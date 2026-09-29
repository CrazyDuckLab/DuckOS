// Package testfixture contains synthetic TEST data only. Runtime packages must
// not import it; fixtures are never a Linux collector fallback.
package testfixture

import (
	"github.com/CrazyDuckLab/DuckOS/internal/model"
	"time"
)

func Overview() model.Overview {
	u := 25.0
	return model.Overview{
		SchemaVersion: "1", ObservedAt: time.Now().UTC(), Environment: "test",
		System:     model.System{Hostname: "router.test", Kernel: "test-kernel", OS: "Debian GNU/Linux 13", Architecture: "amd64", UptimeSeconds: 12345},
		CPU:        model.CPU{LogicalCores: 4, UsagePercent: &u, LoadAverage: []float64{1, .5, .25}},
		Memory:     model.Memory{TotalBytes: 2048, AvailableBytes: 1024, UsedBytes: 1024},
		Storage:    []model.Storage{{Mountpoint: "/", Filesystem: "ext", TotalBytes: 4096, AvailableBytes: 2048, UsedBytes: 1024}},
		Interfaces: []model.Interface{{Name: "test0", Index: 2, MTU: 1500, MAC: "02:00:00:00:00:01", State: "up", Addresses: []model.Address{{Address: "192.0.2.1", Family: "ipv4", PrefixLength: 24}, {Address: "2001:db8::1", Family: "ipv6", PrefixLength: 64}}}},
		Routes:     []model.Route{{Family: "ipv4", Destination: "default", Gateway: "192.0.2.254", Device: "test0", Table: "main", Protocol: "static", Scope: "global"}},
		DNS:        model.DNS{Nameservers: []string{"192.0.2.53"}, SearchDomains: []string{"example.test"}, Source: "/etc/resolv.conf"},
		Services:   []model.Service{{Name: "duck-agent.service", LoadState: "loaded", ActiveState: "active", SubState: "running"}},
		NFTables:   model.NFTables{Status: "available", Tables: 1, Chains: 2, Rules: 3}, Issues: []model.Issue{},
	}
}
