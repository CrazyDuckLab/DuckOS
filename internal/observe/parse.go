package observe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"math"
	"net"
	"strconv"
	"strings"
)

func parseUptime(data []byte) (float64, error) {
	f := strings.Fields(string(data))
	if len(f) < 1 {
		return 0, errors.New("missing uptime")
	}
	n, err := strconv.ParseFloat(f[0], 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, errors.New("invalid uptime")
	}
	return n, nil
}
func parseCPU(data []byte) (total, idle uint64, err error) {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	f := strings.Fields(string(line))
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, errors.New("missing aggregate CPU")
	}
	// Guest counters are already included in user and nice.
	for i := 1; i < len(f) && i <= 8; i++ {
		n, e := strconv.ParseUint(f[i], 10, 64)
		if e != nil || math.MaxUint64-total < n {
			return 0, 0, errors.New("invalid CPU counter")
		}
		total += n
		if i == 4 || i == 5 {
			idle += n
		}
	}
	return total, idle, nil
}
func cpuUsage(t1, i1, t2, i2 uint64) *float64 {
	if t2 <= t1 || i2 < i1 || i2-i1 > t2-t1 {
		return nil
	}
	v := 100 * (1 - float64(i2-i1)/float64(t2-t1))
	return &v
}
func parseLoad(data []byte) ([]float64, error) {
	f := strings.Fields(string(data))
	if len(f) < 3 {
		return nil, errors.New("missing load averages")
	}
	r := make([]float64, 3)
	for i := range r {
		n, err := strconv.ParseFloat(f[i], 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return nil, errors.New("invalid load average")
		}
		r[i] = n
	}
	return r, nil
}
func parseMemory(data []byte) (model.Memory, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) == 0 || (f[0] != "MemTotal:" && f[0] != "MemAvailable:") {
			continue
		}
		if len(f) != 3 || f[2] != "kB" {
			return model.Memory{}, errors.New("invalid memory unit")
		}
		n, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil || n > math.MaxUint64/1024 {
			return model.Memory{}, errors.New("invalid memory value")
		}
		values[f[0]] = n * 1024
	}
	total, okt := values["MemTotal:"]
	available, oka := values["MemAvailable:"]
	if scanner.Err() != nil || !okt || !oka || total == 0 || available > total {
		return model.Memory{}, errors.New("incomplete memory state")
	}
	return model.Memory{TotalBytes: total, AvailableBytes: available, UsedBytes: total - available}, nil
}
func parseOS(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			if s, err := strconv.Unquote(v); err == nil {
				return s
			}
			return strings.Trim(v, "'\"")
		}
	}
	return ""
}
func parseDNS(data []byte) (model.DNS, error) {
	d := model.DNS{Nameservers: []string{}, SearchDomains: []string{}, Source: "/etc/resolv.conf"}
	s := bufio.NewScanner(bytes.NewReader(data))
	for s.Scan() {
		line := strings.SplitN(strings.SplitN(s.Text(), "#", 2)[0], ";", 2)[0]
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "nameserver":
			if net.ParseIP(strings.SplitN(f[1], "%", 2)[0]) == nil {
				return d, errors.New("invalid resolver address")
			}
			d.Nameservers = append(d.Nameservers, f[1])
		case "search":
			d.SearchDomains = append([]string{}, f[1:]...)
		case "domain":
			d.SearchDomains = []string{f[1]}
		}
		if len(d.Nameservers) > 64 || len(d.SearchDomains) > 64 {
			return d, errors.New("resolver limit")
		}
	}
	return d, s.Err()
}
func scalarString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var n uint64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatUint(n, 10), nil
	}
	return "", errors.New("invalid route scalar")
}
func parseRoutes(data []byte, family string) ([]model.Route, error) {
	var rows []struct {
		Dst      string          `json:"dst"`
		Gateway  string          `json:"gateway"`
		Dev      string          `json:"dev"`
		Protocol json.RawMessage `json:"protocol"`
		Scope    json.RawMessage `json:"scope"`
		Table    json.RawMessage `json:"table"`
		Metric   uint64          `json:"metric"`
		Type     string          `json:"type"`
		Nexthops json.RawMessage `json:"nexthops"`
		NHID     json.RawMessage `json:"nhid"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	if len(rows) > 4096 {
		return nil, errors.New("route limit")
	}
	r := make([]model.Route, 0, len(rows))
	unsupported := false
	for _, row := range rows {
		if row.Type != "" && row.Type != "unicast" && row.Type != "local" && row.Type != "broadcast" && row.Type != "multicast" {
			unsupported = true
			continue
		}
		if len(row.Nexthops) != 0 || len(row.NHID) != 0 {
			unsupported = true
			continue
		}
		p, err := scalarString(row.Protocol)
		if err != nil {
			return nil, err
		}
		s, err := scalarString(row.Scope)
		if err != nil {
			return nil, err
		}
		table, err := scalarString(row.Table)
		if err != nil {
			return nil, err
		}
		// iproute2 omits the main-table attribute.
		if table == "" {
			table = "main"
		}
		r = append(r, model.Route{Family: family, Destination: row.Dst, Gateway: row.Gateway, Device: row.Dev, Protocol: p, Scope: s, Table: table, Metric: row.Metric})
	}
	if unsupported {
		return r, errors.New("route inventory contains unsupported multipath or special route types")
	}
	return r, nil
}

var ServiceNames = []string{"duck-api.service", "duck-agent.service", "ssh.service", "systemd-networkd.service", "systemd-resolved.service", "nftables.service"}

func parseServices(data []byte) ([]model.Service, error) {
	allowed := map[string]bool{}
	for _, name := range ServiceNames {
		allowed[name] = true
	}
	r := []model.Service{}
	seen := map[string]bool{}
	for _, block := range strings.Split(strings.TrimSpace(string(data)), "\n\n") {
		f := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			k, v, ok := strings.Cut(line, "=")
			if ok {
				f[k] = v
			}
		}
		name := f["Id"]
		if !allowed[name] || seen[name] || f["LoadState"] == "" || f["ActiveState"] == "" || f["SubState"] == "" {
			return nil, errors.New("invalid service state")
		}
		seen[name] = true
		r = append(r, model.Service{Name: name, LoadState: f["LoadState"], ActiveState: f["ActiveState"], SubState: f["SubState"]})
	}
	if len(r) != len(ServiceNames) {
		return r, errors.New("incomplete service state")
	}
	return r, nil
}
func parseNFT(data []byte) (model.NFTables, error) {
	var d struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return model.NFTables{Status: "unavailable"}, err
	}
	if d.NFTables == nil {
		return model.NFTables{Status: "unavailable"}, errors.New("missing nftables list")
	}
	r := model.NFTables{Status: "available"}
	for _, row := range d.NFTables {
		if _, ok := row["table"]; ok {
			r.Tables++
		}
		if _, ok := row["chain"]; ok {
			r.Chains++
		}
		if _, ok := row["rule"]; ok {
			r.Rules++
		}
	}
	return r, nil
}
func filesystemName(kind int64) string {
	switch uint64(kind) {
	case 0xef53:
		return "ext"
	case 0x58465342:
		return "xfs"
	case 0x9123683e:
		return "btrfs"
	case 0x794c7630:
		return "overlay"
	case 0x01021994:
		return "tmpfs"
	default:
		return fmt.Sprintf("linux:0x%x", kind)
	}
}
