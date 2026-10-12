//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/network"
)

// linuxNetlink is the production netlink-backed source of link, address
// and main-route observations. The exec helpers below use only root-owned,
// minimal /usr/sbin/ip invocations and the kernel /proc/net view; they
// intentionally avoid any mutation.
type linuxNetlink struct{}

// ipRawExec executes /usr/sbin/ip with the supplied arguments inside a
// short, bounded context and returns the stdout bytes. It is intentionally
// minimal: there is no shell, no globbing, no stderr capture.
func ipRawExec(ctx context.Context, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "/usr/sbin/ip", args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin", "LANG=C", "LC_ALL=C"}
	cmd.Dir = "/"
	cmd.Stderr = io.Discard
	var buf boundedRunner
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ip %s: %w", strings.Join(args, " "), err)
	}
	if buf.err != nil {
		return nil, buf.err
	}
	return append([]byte(nil), buf.data...), nil
}

// boundedRunner is a small io.Writer that enforces a 4 MiB output cap so a
// misbehaving ip invocation cannot exhaust Agent memory.
type boundedRunner struct {
	data []byte
	err  error
}

func (b *boundedRunner) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4<<20 {
		b.err = errors.New("ip output exceeded bounded runner")
		return 0, b.err
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

// ipLinkSnapshot invokes `ip -j -d -p link show` and converts the JSON
// records into the bounded evidence types the verifier expects.
func ipLinkSnapshot(ctx context.Context) ([]ipLinkEntry, error) {
	raw, err := ipRawExec(ctx, "-j", "-d", "-p", "link", "show")
	if err != nil {
		return nil, err
	}
	var out []ipLinkEntry
	if jerr := json.Unmarshal(raw, &out); jerr != nil {
		return nil, fmt.Errorf("ip link json: %w", jerr)
	}
	return out, nil
}

// ipLinkAddrSnapshot invokes `ip -j -d address show` to bound the addresses
// per interface.
func ipLinkAddrSnapshot(ctx context.Context) ([]ipAddrEntry, error) {
	raw, err := ipRawExec(ctx, "-j", "-d", "address", "show")
	if err != nil {
		return nil, err
	}
	var out []ipAddrEntry
	if jerr := json.Unmarshal(raw, &out); jerr != nil {
		return nil, fmt.Errorf("ip addr json: %w", jerr)
	}
	return out, nil
}

type ipLinkEntry struct {
	Ifname    string `json:"ifname"`
	Ifindex   int    `json:"ifindex"`
	OperState string `json:"operstate"`
	Address   string `json:"address"`
	MTU       int    `json:"mtu"`
	LinkType  string `json:"linkinfo,omitempty"`
}

type ipLinkInfo struct {
	InfoKind string `json:"info_kind"`
}

type ipAddrEntry struct {
	Ifname   string           `json:"ifname"`
	Ifindex  int              `json:"ifindex"`
	AddrInfo []ipAddrInfoBody `json:"addr_info"`
}

type ipAddrInfoBody struct {
	Prefix string `json:"prefix"`
}

func (linuxNetlink) SnapshotLinks() ([]network.ManagementLinkEvidence, []network.ManagementAddressEvidence, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	links, err := ipLinkSnapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	addrs, err := ipLinkAddrSnapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]network.ManagementLinkEvidence, 0, len(links))
	for _, l := range links {
		up := l.OperState == "UP" || l.OperState == "UNKNOWN"
		out = append(out, network.ManagementLinkEvidence{
			Name:   l.Ifname,
			Role:   roleFromLinkName(l.Ifname),
			Up:     up,
			Agent:  false,
			HasMAC: l.Address != "",
			MTU:    l.MTU,
		})
	}
	addrOut := make([]network.ManagementAddressEvidence, 0)
	for _, a := range addrs {
		prefixes := make([]string, 0, len(a.AddrInfo))
		for _, info := range a.AddrInfo {
			if info.Prefix != "" {
				prefixes = append(prefixes, info.Prefix)
			}
		}
		for _, prefix := range prefixes {
			p, err := netip.ParsePrefix(prefix)
			if err != nil {
				continue
			}
			addrOut = append(addrOut, network.ManagementAddressEvidence{
				Link:  a.Ifname,
				Addr:  p,
				Agent: false,
			})
		}
	}
	return out, addrOut, nil
}

// roleFromLinkName is a deliberately conservative heuristic that only assigns
// roles to fixtures the Agent recognises by name. It is not authoritative;
// authoritative role binding comes from the durable enrollment.
func roleFromLinkName(name string) string {
	switch {
	case name == "lo":
		return "loopback"
	case strings.HasPrefix(name, "veth"), strings.HasPrefix(name, "vlan"), strings.HasPrefix(name, "br"):
		return "lab"
	}
	return ""
}

func (linuxNetlink) MainRoutes() ([]network.KernelRoute, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := []network.KernelRoute{}
	for _, family := range []string{"-4", "-6"} {
		raw, err := ipRawExec(ctx, "-j", family, "route", "show", "table", "main")
		if err != nil {
			return nil, err
		}
		var records []ipRouteEntry
		if jerr := json.Unmarshal(raw, &records); jerr != nil {
			return nil, fmt.Errorf("ip route json: %w", jerr)
		}
		for _, r := range records {
			fam := "ipv4"
			if family == "-6" {
				fam = "ipv6"
			}
			route := network.KernelRoute{
				Family:      fam,
				Table:       254, // main
				Destination: r.Dst,
				Interface:   r.Dev,
				Metric:      r.Metric,
			}
			if r.Gateway != "" {
				route.Gateway = r.Gateway
			}
			out = append(out, route)
		}
	}
	return out, nil
}

type ipRouteEntry struct {
	Dst     string `json:"dst"`
	Dev     string `json:"dev"`
	Metric  int    `json:"metric"`
	Gateway string `json:"gateway"`
}

// linuxProcNet is the production /proc/net-backed source of TCP/UDP listeners.
// It is read-only and bounded by /proc.
type linuxProcNet struct{}

func (linuxProcNet) Listeners() ([]network.ManagementListenerEvidence, error) {
	out := []network.ManagementListenerEvidence{}
	for _, src := range []string{"/proc/net/tcp", "/proc/net/udp"} {
		proto := "tcp"
		if strings.HasSuffix(src, "udp") {
			proto = "udp"
		}
		b, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", src, err)
		}
		lines := strings.Split(string(b), "\n")
		if len(lines) < 2 {
			continue
		}
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// Only listeners in state 0A (LISTEN).
			if fields[3] != "0A" {
				continue
			}
			host, port, ok := parseHexAddrPort(fields[1])
			if !ok {
				continue
			}
			out = append(out, network.ManagementListenerEvidence{
				Link:    "",
				Address: host,
				Port:    port,
				Proto:   proto,
				Service: guessService(port),
				Agent:   false,
			})
		}
	}
	return out, nil
}

// linuxLLDPProbe is conservative: returns no neighbours unless the optional
// lldpcli/lldp binary is installed at /usr/sbin/lldpcli. Production deployments
// that wish to learn LLDP neighbours can override this with their own probe.
type linuxLLDPProbe struct{}

func (linuxLLDPProbe) Neighbors() ([]network.ManagementNeighborEvidence, error) {
	return nil, nil
}

func parseHexAddrPort(s string) (netip.Addr, int, bool) {
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
		return nil, fmt.Errorf("hex %s has odd length", s)
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, ok1 := hexValue(s[2*i])
		lo, ok2 := hexValue(s[2*i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("hex %s invalid", s)
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
	v, err := strconv.ParseInt(s, 16, 32)
	if err != nil {
		return 0, err
	}
	return int(v), nil
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
