//go:build linux

// wan/observe_linux.go: the kernel observation collector.
// Reads link state and addresses through the standard net
// interfaces and the IPv4 default route from /proc/net/route.
// Purely read-only: nothing here mutates networking.
package wan

import (
	"bufio"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"
)

// Observe collects the state of one link (or all links when
// link is empty). Missing links return an observation with
// Up=false rather than an error so the API can show honest
// state.
func Observe(link string) []Observation {
	var out []Observation
	now := time.Now().UTC().Format(time.RFC3339)
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	defaults := defaultRoutesV4()
	for _, ifi := range ifaces {
		if link != "" && ifi.Name != link {
			continue
		}
		if link == "" && isLoopbackLike(ifi.Name) {
			continue
		}
		obs := Observation{
			Link:       ifi.Name,
			Up:         ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagRunning != 0,
			MTU:        ifi.MTU,
			Hardware:   ifi.HardwareAddr.String(),
			ObservedAt: now,
		}
		addrs, err := ifi.Addrs()
		if err == nil {
			for _, a := range addrs {
				ipnet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				if ip4 := ipnet.IP.To4(); ip4 != nil {
					obs.IPv4 = append(obs.IPv4, (&net.IPNet{IP: ip4, Mask: ipnet.Mask}).String())
				} else {
					obs.IPv6 = append(obs.IPv6, ipnet.String())
				}
			}
		}
		if gw, ok := defaults[ifi.Name]; ok && gw.IsValid() {
			obs.DefaultRouteV4 = "via " + gw.String()
		}
		out = append(out, obs)
	}
	return out
}

// defaultRoutesV4 parses /proc/net/route into a
// device -> (gateway, device) map for the default
// destination (00000000).
func defaultRoutesV4() map[string](netip.Addr) {
	out := map[string]netip.Addr{}
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		raw, err := hex.DecodeString(fields[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		// little-endian on the wire
		addr, ok := netip.AddrFromSlice([]byte{raw[3], raw[2], raw[1], raw[0]})
		if !ok {
			continue
		}
		out[fields[0]] = addr
	}
	return out
}

func isLoopbackLike(name string) bool {
	return name == "lo" || strings.HasPrefix(name, "lo")
}

// ErrObserveUnsupported marks non-linux builds; never used on
// linux but kept so the two build variants stay symmetric.
var ErrObserveUnsupported = errors.New("wan: observation unsupported on this build")
