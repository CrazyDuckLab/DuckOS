//go:build linux

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
)

func collectManagementEvidence(c ManagementEvidenceCollector) (ManagementEvidence, error) {
	if c.Netlink == nil || c.ProcNet == nil {
		return ManagementEvidence{}, errors.New("netlink and proc-net sources are required for evidence collection")
	}
	ctx := context.Background()
	if err := ctx.Err(); err != nil {
		return ManagementEvidence{}, err
	}
	links, addresses, err := c.Netlink.SnapshotLinks()
	if err != nil {
		return ManagementEvidence{}, fmt.Errorf("netlink link snapshot: %w", err)
	}
	for i := range links {
		if c.OwnerAllow != nil {
			links[i].Agent = c.OwnerAllow[links[i].Name]
		}
	}
	routes, err := c.Netlink.MainRoutes()
	if err != nil {
		return ManagementEvidence{}, fmt.Errorf("netlink route snapshot: %w", err)
	}
	listeners, err := c.ProcNet.Listeners()
	if err != nil {
		return ManagementEvidence{}, fmt.Errorf("proc-net listener snapshot: %w", err)
	}
	var neighbors []ManagementNeighborEvidence
	if c.LLDPProbe != nil {
		neighbors, err = c.LLDPProbe.Neighbors()
		if err != nil {
			return ManagementEvidence{}, fmt.Errorf("lldp probe: %w", err)
		}
	}
	return ManagementEvidence{
		Links:          links,
		MainRoutes:     routes,
		MainAddresses:  addresses,
		Listeners:      listeners,
		LLDPNeighbors:  neighbors,
		OwnedNeighbors: map[string]bool{},
	}, nil
}

// defaultListenerScanner uses /proc/net/tcp and /proc/net/udp to list local
// listeners with link attribution. It is intentionally simple: the goal
// is to surface a foreign listener, not to be a complete netstat clone.
type defaultListenerScanner struct {
	LinkByIndex map[int]string
}

func (s defaultListenerScanner) Listeners() ([]ManagementListenerEvidence, error) {
	out, err := s.read("/proc/net/tcp", "tcp")
	if err != nil {
		return nil, err
	}
	udp, err := s.read("/proc/net/udp", "udp")
	if err != nil {
		return nil, err
	}
	return append(out, udp...), nil
}

func (s defaultListenerScanner) read(path, proto string) ([]ManagementListenerEvidence, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var listeners []ManagementListenerEvidence
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for _, line := range strings.Split(string(buf[:n]), "\n") {
				if !strings.Contains(line, " 0A ") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 4 {
					continue
				}
				local, port, ok := parseHexAddr(fields[1])
				if !ok {
					continue
				}
				listeners = append(listeners, ManagementListenerEvidence{Link: "", Address: local, Port: port, Proto: proto, Service: guessService(port), Agent: false})
			}
		}
		if err != nil {
			break
		}
	}
	return listeners, nil
}

func parseHexAddr(s string) (netip.Addr, int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return netip.Addr{}, 0, false
	}
	ipBytes, err := hexDecode(parts[0])
	if err != nil {
		return netip.Addr{}, 0, false
	}
	port, err := hexPort(parts[1])
	if err != nil {
		return netip.Addr{}, 0, false
	}
	if len(ipBytes) == 4 {
		return netip.AddrFrom4([4]byte{ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0]}), port, true
	}
	if len(ipBytes) == 16 {
		var b [16]byte
		copy(b[:], ipBytes)
		return netip.AddrFrom16(b), port, true
	}
	return netip.Addr{}, 0, false
}

func hexDecode(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if len(s)%2 != 0 {
		return nil, &net.ParseError{Type: "hex", Text: s}
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, ok1 := hexValue(s[2*i])
		lo, ok2 := hexValue(s[2*i+1])
		if !ok1 || !ok2 {
			return nil, &net.ParseError{Type: "hex", Text: s}
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexValue(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

func hexPort(s string) (int, error) {
	if len(s) == 0 {
		return 0, &net.ParseError{Type: "port", Text: s}
	}
	v := 0
	for i := 0; i < len(s); i++ {
		c, ok := hexValue(s[i])
		if !ok {
			return 0, &net.ParseError{Type: "port", Text: s}
		}
		v = v*16 + int(c)
	}
	return v, nil
}

func guessService(port int) string {
	switch port {
	case 22:
		return "ssh"
	case 80:
		return "web"
	case 443:
		return "web"
	case 8443:
		return "api"
	}
	return ""
}
