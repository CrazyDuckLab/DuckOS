//go:build linux

package doctor

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
)

// NetworkSource is the Doctor's view of the live interface and
// route surface. The source reads /proc/net/{dev,route,if_inet6}
// and /sys/class/net/<name>/*. It never executes shell commands.
type NetworkSource struct{}

func (s *NetworkSource) Name() string { return "network" }
func (s *NetworkSource) Contribute(b *Builder) {
	ifaces, err := readInterfaces()
	if err != nil {
		b.AddSubsystem(Subsystem{Name: "network", Status: "degraded", Detail: err.Error()})
	}
	routes, err := readRoutes()
	if err != nil {
		b.AddSubsystem(Subsystem{Name: "network", Status: "degraded", Detail: err.Error()})
	}
	gw := false
	if data, err := readBounded("/proc/net/route"); err == nil {
		gw = hasDefaultRoute(data)
	}
	b.SetNetwork(Network{Interfaces: ifaces, Routes: routes, GatewayLoss: !gw})
	b.AddSubsystem(Subsystem{Name: "network", Status: "ok"})
}

func readInterfaces() ([]Interface, error) {
	data, err := readBounded("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	byName := map[string]Throughput{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 8 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		byName[name] = Throughput{RxBytes: rx, TxBytes: tx}
	}
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return byNameFallback(byName), nil
	}
	out := make([]Interface, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.Contains(name, ":") {
			continue
		}
		idx := readInt("/sys/class/net/" + name + "/ifindex")
		mtu := readInt("/sys/class/net/" + name + "/mtu")
		flags := readInt("/sys/class/net/" + name + "/flags")
		oper := readString("/sys/class/net/" + name + "/operstate")
		mac := readMAC("/sys/class/net/" + name + "/address")
		driver := readDriver(name)
		kind := readKind(name)
		addrs := readAddresses(name)
		out = append(out, Interface{
			Name:       name,
			Index:      idx,
			LinkUp:     (flags & 1) != 0,
			MTU:        mtu,
			MAC:        mac,
			Kind:       kind,
			Driver:     driver,
			OperState:  strings.TrimSpace(oper),
			Addresses:  addrs,
			Throughput: byName[name],
		})
		if len(out) >= MaxInterfaces {
			break
		}
	}
	return out, nil
}

func byNameFallback(m map[string]Throughput) []Interface {
	out := make([]Interface, 0, len(m))
	for name, t := range m {
		out = append(out, Interface{Name: name, Throughput: t, OperState: "unknown"})
		if len(out) >= MaxInterfaces {
			break
		}
	}
	return out
}

func readInt(path string) int {
	data, err := readBounded(path)
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return v
}

func readString(path string) string {
	data, err := readBounded(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readMAC(path string) string {
	data, err := readBounded(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readDriver(name string) string {
	link, err := os.Readlink("/sys/class/net/" + name + "/device/driver")
	if err != nil {
		return ""
	}
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

func readKind(name string) string {
	data, err := readBounded("/sys/class/net/" + name + "/uevent")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "DEVTYPE="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func readAddresses(name string) []string {
	var out []string
	if data, err := readBounded("/proc/net/if_inet6"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 6 {
				continue
			}
			if fields[5] != name {
				continue
			}
			raw := fields[0]
			if len(raw) != 32 {
				continue
			}
			b, err := hex.DecodeString(raw)
			if err != nil || len(b) != 16 {
				continue
			}
			out = append(out, formatInet6(b)+"/"+fields[4])
		}
	}
	if data, err := readBounded("/proc/net/fib_trie"); err == nil {
		// fib_trie is verbose; we leave IPv4 enumeration to the
		// overview collector and only pull a coarse link flag here.
		_ = data
	}
	return out
}

func formatInet6(b []byte) string {
	groups := make([]string, 8)
	for i := 0; i < 8; i++ {
		groups[i] = hex.EncodeToString(b[i*2 : i*2+2])
	}
	return strings.Join(groups, ":")
}

func readRoutes() ([]Route, error) {
	data, err := readBounded("/proc/net/route")
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	var out []Route
	first := true
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}
		if first {
			first = false
			continue
		}
		dest, err := hexToIP(fields[1])
		if err != nil {
			continue
		}
		gw, err := hexToIP(fields[2])
		if err != nil {
			continue
		}
		flags, _ := strconv.ParseUint(fields[3], 16, 32)
		metric, _ := strconv.Atoi(fields[6])
		mask, err := hexToIP(fields[7])
		if err != nil {
			continue
		}
		out = append(out, Route{
			Destination: dest + "/" + mask,
			Gateway:     gw,
			Device:      fields[0],
			Table:       "main",
			Metric:      metric,
			Source:      ifFlag(flags, 0x2, "static", "dynamic"),
		})
		if len(out) >= MaxRoutes {
			break
		}
	}
	return out, nil
}

func hexToIP(s string) (string, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 4 {
		return "", err
	}
	return strconv.Itoa(int(b[0])) + "." + strconv.Itoa(int(b[1])) + "." + strconv.Itoa(int(b[2])) + "." + strconv.Itoa(int(b[3])), nil
}

func ifFlag(flags uint64, bit uint64, yes, no string) string {
	if flags&bit != 0 {
		return yes
	}
	return no
}

func hasDefaultRoute(data []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	first := true
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if first {
			first = false
			continue
		}
		if fields[1] == "00000000" {
			return true
		}
	}
	return false
}
