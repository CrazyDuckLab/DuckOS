//go:build linux

package observe

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/model"
)

func (c *Collector) Overview(ctx context.Context) (model.Overview, error) {
	o := emptyOverview(c.Environment)
	o.System.Architecture = runtime.GOARCH
	if runtime.GOARCH == "arm" {
		o.System.Architecture = "armv7"
	}
	if name, err := os.Hostname(); err == nil {
		o.System.Hostname = name
	} else {
		issue(&o, "system", "hostname_unavailable", "Hostname could not be read.")
	}
	if data, err := readBounded("/proc/sys/kernel/osrelease"); err == nil {
		o.System.Kernel = strings.TrimSpace(string(data))
	} else {
		issue(&o, "system", "kernel_unavailable", "Kernel version could not be read.")
	}
	if data, err := readBounded("/etc/os-release"); err == nil {
		o.System.OS = parseOS(data)
	}
	if o.System.OS == "" {
		issue(&o, "system", "os_unavailable", "Operating system identification could not be read.")
	}
	if data, err := readBounded("/proc/uptime"); err == nil {
		if value, e := parseUptime(data); e == nil {
			o.System.UptimeSeconds = value
		} else {
			issue(&o, "system", "uptime_unavailable", "Uptime could not be parsed.")
		}
	} else {
		issue(&o, "system", "uptime_unavailable", "Uptime could not be read.")
	}
	o.CPU.LogicalCores = runtime.NumCPU()
	if data, err := readBounded("/proc/loadavg"); err == nil {
		if values, e := parseLoad(data); e == nil {
			o.CPU.LoadAverage = values
		}
	}
	if len(o.CPU.LoadAverage) != 3 {
		issue(&o, "cpu", "load_unavailable", "System load averages could not be read.")
	}
	first, firstErr := readBounded("/proc/stat")
	timer := time.NewTimer(100 * time.Millisecond)
	select {
	case <-ctx.Done():
		timer.Stop()
		return model.Overview{}, ctx.Err()
	case <-timer.C:
	}
	second, secondErr := readBounded("/proc/stat")
	if firstErr == nil && secondErr == nil {
		t1, i1, e1 := parseCPU(first)
		t2, i2, e2 := parseCPU(second)
		if e1 == nil && e2 == nil {
			o.CPU.UsagePercent = cpuUsage(t1, i1, t2, i2)
		}
	}
	if o.CPU.UsagePercent == nil {
		issue(&o, "cpu", "usage_unavailable", "A valid CPU counter interval could not be measured.")
	}
	if data, err := readBounded("/proc/meminfo"); err == nil {
		if memory, e := parseMemory(data); e == nil {
			o.Memory = memory
		}
	}
	if o.Memory.TotalBytes == 0 {
		issue(&o, "memory", "memory_unavailable", "Memory counters could not be read.")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil && stat.Bsize > 0 {
		unit := uint64(stat.Bsize)
		o.Storage = append(o.Storage, model.Storage{Mountpoint: "/", Filesystem: filesystemName(int64(stat.Type)), TotalBytes: stat.Blocks * unit, AvailableBytes: stat.Bavail * unit, UsedBytes: (stat.Blocks - stat.Bfree) * unit})
	} else {
		issue(&o, "storage", "storage_unavailable", "Root filesystem counters could not be read.")
	}
	if interfaces, err := collectInterfaces(); err == nil {
		o.Interfaces = interfaces
	} else {
		issue(&o, "interfaces", "interfaces_unavailable", "Interface inventory could not be read within its limits.")
	}
	for _, family := range []struct{ arg, name string }{{"-4", "ipv4"}, {"-6", "ipv6"}} {
		data, err := runFixed(ctx, "/usr/sbin/ip", "-j", family.arg, "route", "show", "table", "all")
		if err == nil {
			rows, e := parseRoutes(data, family.name)
			o.Routes = append(o.Routes, rows...)
			if e == nil {
				continue
			}
			issue(&o, "routes", "routes_"+family.name+"_partial", "The "+family.name+" routing inventory is incomplete: unsupported special/multipath entries or invalid data were omitted.")
			continue
		}
		issue(&o, "routes", "routes_"+family.name+"_unavailable", "The "+family.name+" routing tables could not be read within their limits.")
	}
	if data, err := readBounded("/etc/resolv.conf"); err == nil {
		if dns, e := parseDNS(data); e == nil {
			o.DNS = dns
		} else {
			issue(&o, "dns", "dns_unavailable", "Resolver configuration could not be parsed.")
		}
	} else {
		issue(&o, "dns", "dns_unavailable", "Resolver configuration could not be read.")
	}
	if len(o.DNS.Nameservers) == 0 {
		issue(&o, "dns", "no_nameservers", "No nameservers were observed in /etc/resolv.conf; upstream resolver state is not inferred.")
	}
	args := []string{"show", "--no-pager", "--property=Id,LoadState,ActiveState,SubState"}
	args = append(args, ServiceNames...)
	data, commandErr := runFixed(ctx, "/usr/bin/systemctl", args...)
	services, parseErr := parseServices(data)
	if parseErr == nil {
		o.Services = services
	}
	if commandErr != nil || parseErr != nil {
		issue(&o, "services", "services_unavailable", "Allowlisted systemd service state could not be read completely.")
	}
	data, commandErr = runFixed(ctx, "/usr/sbin/nft", "-j", "list", "ruleset")
	if commandErr == nil {
		if nft, err := parseNFT(data); err == nil {
			o.NFTables = nft
		}
	}
	if o.NFTables.Status == "unavailable" {
		issue(&o, "nftables", "nftables_unavailable", "nftables ruleset is unavailable, denied, or exceeds the observation limit.")
	}
	if err := ctx.Err(); err != nil {
		return model.Overview{}, err
	}
	// Timestamp denotes completion of this bounded collection interval.
	o.ObservedAt = time.Now().UTC()
	return o, nil
}

func collectInterfaces() ([]model.Interface, error) {
	items, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	if len(items) > 256 {
		return nil, errors.New("interface limit")
	}
	r := make([]model.Interface, 0, len(items))
	totalAddresses := 0
	for _, item := range items {
		state := "down"
		if item.Flags&net.FlagUp != 0 {
			state = "up"
		}
		entry := model.Interface{Name: item.Name, Index: item.Index, MTU: item.MTU, MAC: item.HardwareAddr.String(), State: state, Addresses: []model.Address{}}
		addresses, err := item.Addrs()
		if err != nil {
			return nil, err
		}
		totalAddresses += len(addresses)
		if totalAddresses > 1024 {
			return nil, errors.New("address limit")
		}
		for _, address := range addresses {
			ip, network, err := net.ParseCIDR(address.String())
			if err != nil {
				return nil, err
			}
			prefix, _ := network.Mask.Size()
			family := "ipv6"
			if ip.To4() != nil {
				family = "ipv4"
			}
			entry.Addresses = append(entry.Addresses, model.Address{Address: ip.String(), Family: family, PrefixLength: prefix})
		}
		r = append(r, entry)
	}
	sort.Slice(r, func(i, j int) bool { return r[i].Index < r[j].Index })
	return r, nil
}
