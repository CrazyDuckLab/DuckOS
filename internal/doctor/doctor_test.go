package doctor

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestBuilder() *Builder {
	return NewBuilder("test", "duckos-dev", "boot-1", time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), 123.45)
}

type stubSource struct {
	name string
	fn   func(*Builder)
}

func (s *stubSource) Name() string          { return s.name }
func (s *stubSource) Contribute(b *Builder) { s.fn(b) }

func TestBuilderSnapshotIsBounded(t *testing.T) {
	b := newTestBuilder()
	b.SetSystem(System{Hostname: strings.Repeat("a", 1024), Kernel: "linux", OS: "debian", Architecture: "amd64"})
	storage := make([]Storage, MaxStorage+8)
	for i := range storage {
		storage[i] = Storage{Mountpoint: "/mnt", Filesystem: "ext4", Device: strings.Repeat("/dev/", 30), TotalBytes: 1, AvailableBytes: 1, UsedBytes: 0}
	}
	b.SetResources(Resources{
		CPULogicalCores: 4,
		LoadAverage:     []float64{0.1, 0.2, 0.3, 0.4},
		Memory:          Memory{TotalBytes: 1 << 30, AvailableBytes: 1 << 30, UsedBytes: 1 << 30},
		Storage:         storage,
	})
	snap := b.Snapshot()
	if len(snap.Resources.Storage) != MaxStorage {
		t.Fatalf("expected %d storage entries, got %d", MaxStorage, len(snap.Resources.Storage))
	}
	if len(snap.Resources.LoadAverage) != 3 {
		t.Fatalf("expected 3 load entries, got %d", len(snap.Resources.LoadAverage))
	}
	if len(snap.System.Hostname) > 64 {
		t.Fatalf("hostname not truncated: %d", len(snap.System.Hostname))
	}
	if len(snap.Resources.Storage[0].Device) > 128 {
		t.Fatalf("device path not truncated: %d", len(snap.Resources.Storage[0].Device))
	}
}

func TestBuilderNetworkCaps(t *testing.T) {
	b := newTestBuilder()
	ifaces := make([]Interface, MaxInterfaces+4)
	for i := range ifaces {
		ifaces[i] = Interface{Name: "if" + strings.Repeat("x", 64), Index: i, LinkUp: true, MTU: 1500, MAC: "01:02:03:04:05:06"}
	}
	routes := make([]Route, MaxRoutes+4)
	for i := range routes {
		routes[i] = Route{Destination: "10.0.0.0/24", Device: "if0", Metric: i}
	}
	b.SetNetwork(Network{Interfaces: ifaces, Routes: routes})
	snap := b.Snapshot()
	if len(snap.Network.Interfaces) != MaxInterfaces {
		t.Fatalf("expected %d interfaces, got %d", MaxInterfaces, len(snap.Network.Interfaces))
	}
	if len(snap.Network.Routes) != MaxRoutes {
		t.Fatalf("expected %d routes, got %d", MaxRoutes, len(snap.Network.Routes))
	}
	if snap.Network.Interfaces[0].Name == "" || len(snap.Network.Interfaces[0].Name) > 32 {
		t.Fatalf("interface name not truncated: %q", snap.Network.Interfaces[0].Name)
	}
}

func TestBuilderRejectsBadMAC(t *testing.T) {
	b := newTestBuilder()
	b.SetNetwork(Network{Interfaces: []Interface{{Name: "x", MAC: "not-a-mac"}}})
	snap := b.Snapshot()
	if snap.Network.Interfaces[0].MAC != "" {
		t.Fatalf("expected empty MAC, got %q", snap.Network.Interfaces[0].MAC)
	}
}

func TestBuilderIssuesDedupe(t *testing.T) {
	b := newTestBuilder()
	b.AddIssue(Issue{Component: "x", Code: "y", Severity: "error", Message: "m"})
	b.AddIssue(Issue{Component: "x", Code: "y", Severity: "warning", Message: "other"})
	b.AddIssue(Issue{Component: "x", Code: "z", Severity: "critical", Message: "z"})
	snap := b.Snapshot()
	if len(snap.Issues) != 2 {
		t.Fatalf("expected 2 issues after dedup, got %d", len(snap.Issues))
	}
	if snap.Issues[0].Severity != "critical" {
		t.Fatalf("expected critical first, got %q", snap.Issues[0].Severity)
	}
}

func TestBuilderIssueCap(t *testing.T) {
	b := newTestBuilder()
	for i := 0; i < MaxIssues+8; i++ {
		b.AddIssue(Issue{Component: "c", Code: string(rune('a'+(i%26))) + string(rune('A'+((i/26)%26))), Severity: "info", Message: "m"})
	}
	snap := b.Snapshot()
	if len(snap.Issues) != MaxIssues {
		t.Fatalf("expected %d issues, got %d", MaxIssues, len(snap.Issues))
	}
}

func TestBuilderLoggingTail(t *testing.T) {
	b := newTestBuilder()
	tail := make([]string, MaxLogLines+8)
	for i := range tail {
		tail[i] = "line"
	}
	b.SetLogging(Logging{Tail: tail})
	snap := b.Snapshot()
	if len(snap.Logging.Tail) != MaxLogLines {
		t.Fatalf("expected %d tail lines, got %d", MaxLogLines, len(snap.Logging.Tail))
	}
}

func TestBuilderSubsystem(t *testing.T) {
	b := newTestBuilder()
	b.AddSubsystem(Subsystem{Name: "x", Status: "ok"})
	b.AddSubsystem(Subsystem{Name: "y", Status: "weird"})
	b.AddSubsystem(Subsystem{Name: ""})
	snap := b.Snapshot()
	if len(snap.Subsystem) != 2 {
		t.Fatalf("expected 2 subsystems, got %d", len(snap.Subsystem))
	}
	if snap.Subsystem[0].Name != "x" || snap.Subsystem[1].Status != "unknown" {
		t.Fatalf("subsystem normalisation failed: %#v", snap.Subsystem)
	}
}

func TestBuilderConcurrentSources(t *testing.T) {
	b := newTestBuilder()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.AddSubsystem(Subsystem{Name: "src", Status: "ok", Detail: string(rune('a' + (i % 26)))})
			b.AddIssue(Issue{Component: "c", Code: string(rune('A' + (i % 26))), Severity: "info", Message: "m"})
		}(i)
	}
	wg.Wait()
	snap := b.Snapshot()
	if len(snap.Subsystem) > 64 {
		t.Fatalf("subsystem ring exceeded: %d", len(snap.Subsystem))
	}
}

func TestSnapshotValidates(t *testing.T) {
	b := newTestBuilder()
	snap := b.Snapshot()
	if err := snap.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotJSON(t *testing.T) {
	b := newTestBuilder()
	b.SetSystem(System{Hostname: "router", Kernel: "6.5", OS: "debian", Architecture: "amd64"})
	b.SetResources(Resources{CPULogicalCores: 4, Memory: Memory{TotalBytes: 1, AvailableBytes: 1, UsedBytes: 0}})
	b.SetNetwork(Network{Interfaces: []Interface{{Name: "lo", Index: 1, LinkUp: true, MTU: 65536, Addresses: []string{"127.0.0.1/8"}}}})
	b.SetFirewall(Firewall{Backend: "nftables", Status: "available", Tables: 1, Chains: 4, Rules: 12})
	b.SetVault(Vault{Available: true, State: "open", KeyID: "abc", Records: 5, Tombstones: 1, MasterExists: true})
	b.SetIdentity(Identity{Enrolled: true, UserCount: 1, SessionCount: 1, AuditLines: 10})
	b.SetDevices(Devices{Total: 12, Quarantined: 1, PerLink: map[string]int{"lan0": 12}, PerRole: map[string]int{"trusted": 11, "quarantined": 1}})
	b.SetWireGuard(WireGuard{Servers: 1, Peers: 4, Status: "ok"})
	b.SetXray(Xray{Servers: 1, Users: 2, Profiles: 1, Subscribes: 0, Status: "ok"})
	b.SetLogging(Logging{Subsystems: 6, RingSize: 512, RingUsed: 12, Subscribers: 2})
	b.AddSubsystem(Subsystem{Name: "vault", Status: "ok"})
	b.AddSubsystem(Subsystem{Name: "network", Status: "ok"})
	b.AddIssue(Issue{Component: "vault", Code: "ready", Severity: "info", Message: "ok"})
	snap := b.Snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxBytes {
		t.Fatalf("snapshot exceeded MaxBytes: %d > %d", len(data), MaxBytes)
	}
	out := string(data)
	if !strings.Contains(out, `"schemaVersion":"1"`) {
		t.Fatalf("missing schemaVersion in JSON: %s", out)
	}
	if !strings.Contains(out, `"duckos":"duckos-dev"`) {
		t.Fatalf("missing duckos version in JSON: %s", out)
	}
	if strings.Contains(out, "secret") || strings.Contains(out, "password") {
		t.Fatalf("snapshot JSON unexpectedly contains secret-like tokens: %s", out)
	}
}

func TestBuilderIssueSeverityFilter(t *testing.T) {
	b := newTestBuilder()
	b.AddIssue(Issue{Component: "a", Code: "a", Severity: "broken", Message: "m"})
	b.AddIssue(Issue{Component: "b", Code: "b", Severity: "critical", Message: "m"})
	snap := b.Snapshot()
	if snap.Issues[0].Code != "b" {
		t.Fatalf("expected critical first, got %+v", snap.Issues[0])
	}
	if snap.Issues[1].Severity != "info" {
		t.Fatalf("expected severity demoted to info, got %q", snap.Issues[1].Severity)
	}
}
