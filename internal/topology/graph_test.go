package topology

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuilderBasicGraph(t *testing.T) {
	b := NewBuilder(time.Now().Unix())
	b.AddInterface(InterfaceSpec{
		Name:      "lan0",
		Kind:      NodeInterface,
		State:     "up",
		Addresses: []string{"192.0.2.1/24", "fe80::1/64"},
	})
	b.AddInterface(InterfaceSpec{
		Name:   "br0",
		Kind:   NodeBridge,
		Parent: "lan0",
		State:  "up",
	})
	b.AddInterface(InterfaceSpec{
		Name:   "vlan10",
		Kind:   NodeVLAN,
		Parent: "br0",
		State:  "up",
	})
	b.AddDevice(DeviceSpec{ID: "abcdef", Label: "laptop", Link: "lan0", Role: "trusted"})
	graph := b.Snapshot()
	if len(graph.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) < 3 {
		t.Fatalf("expected at least 3 edges, got %d", len(graph.Edges))
	}
	if err := graph.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestBuilderDeduplicates(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "eth0"})
	b.AddInterface(InterfaceSpec{Name: "eth0"})
	b.AddInterface(InterfaceSpec{Name: "eth0", State: "up"})
	g := b.Snapshot()
	count := 0
	for _, n := range g.Nodes {
		if n.Label == "eth0" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 node, got %d", count)
	}
}

func TestBuilderQuarantineEdge(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "lan0"})
	b.AddDevice(DeviceSpec{ID: "abc", Link: "lan0", Role: "quarantined"})
	g := b.Snapshot()
	hasQuar := false
	for _, e := range g.Edges {
		if e.Kind == EdgeQuarantine {
			hasQuar = true
		}
	}
	if !hasQuar {
		t.Fatal("quarantine edge missing")
	}
}

func TestBuilderWireGuardPeers(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "wg0"})
	b.AddWireGuard(WireGuardSpec{
		Name:  "main",
		Peers: []string{"alice", "bob"},
		State: "ok",
	})
	g := b.Snapshot()
	peer := 0
	for _, n := range g.Nodes {
		if n.Kind == NodeWireGuard && n.State == "peer" {
			peer++
		}
	}
	if peer != 2 {
		t.Fatalf("expected 2 peer nodes, got %d", peer)
	}
}

func TestBuilderXrayUsers(t *testing.T) {
	b := NewBuilder(0)
	b.AddXray(XraySpec{Name: "vless-main", Users: []string{"u1", "u2"}, State: "ok"})
	g := b.Snapshot()
	users := 0
	for _, n := range g.Nodes {
		if n.Kind == NodeXray && n.State == "user" {
			users++
		}
	}
	if users != 2 {
		t.Fatalf("expected 2 user nodes, got %d", users)
	}
}

func TestBuilderRespectsNodeCap(t *testing.T) {
	b := NewBuilder(0)
	for i := 0; i < MaxNodes+16; i++ {
		b.AddInterface(InterfaceSpec{Name: "if" + itoa(i)})
	}
	g := b.Snapshot()
	if len(g.Nodes) != MaxNodes {
		t.Fatalf("expected %d nodes, got %d", MaxNodes, len(g.Nodes))
	}
}

func TestBuilderRespectsDeviceCap(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "lan0"})
	for i := 0; i < MaxDevices+16; i++ {
		b.AddDevice(DeviceSpec{ID: itoa(i), Link: "lan0"})
	}
	g := b.Snapshot()
	count := 0
	for _, n := range g.Nodes {
		if n.Kind == NodeDevice {
			count++
		}
	}
	if count != MaxDevices {
		t.Fatalf("expected %d devices, got %d", MaxDevices, count)
	}
}

func TestValidatesGraph(t *testing.T) {
	g := Graph{SchemaVersion: "wrong"}
	if err := g.Valid(); err == nil {
		t.Fatal("invalid schema accepted")
	}
}

func TestBuilderTruncatesAddresses(t *testing.T) {
	b := NewBuilder(0)
	addrs := make([]string, MaxAddressCount+4)
	for i := range addrs {
		addrs[i] = "10.0.0.1/32"
	}
	b.AddInterface(InterfaceSpec{Name: "x", Addresses: addrs})
	g := b.Snapshot()
	if len(g.Nodes[0].Address) > MaxAddressCount {
		t.Fatalf("expected %d addresses, got %d", MaxAddressCount, len(g.Nodes[0].Address))
	}
}

func TestBuilderRejectsBadMeta(t *testing.T) {
	b := NewBuilder(0)
	m := make(map[string]string, 20)
	for i := 0; i < 20; i++ {
		m["k"+itoa(i)] = "v"
	}
	b.AddInterface(InterfaceSpec{Name: "x", Meta: m})
	g := b.Snapshot()
	if g.Nodes[0].Meta != nil {
		t.Fatal("oversized meta accepted")
	}
}

func TestBuilderConcurrent(t *testing.T) {
	b := NewBuilder(0)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.AddInterface(InterfaceSpec{Name: "if" + itoa(i)})
		}(i)
	}
	wg.Wait()
	if err := b.Snapshot().Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotJSON(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "lan0", State: "up", Addresses: []string{"192.0.2.1/24"}})
	b.AddDevice(DeviceSpec{ID: "abc", Link: "lan0", Role: "trusted"})
	g := b.Snapshot()
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxBytes {
		t.Fatalf("graph exceeded MaxBytes: %d > %d", len(data), MaxBytes)
	}
	out := string(data)
	if !strings.Contains(out, `"schemaVersion":"1"`) {
		t.Fatalf("missing schemaVersion: %s", out)
	}
}

func TestBuilderIgnoresEmptyInputs(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{})
	b.AddDevice(DeviceSpec{})
	b.AddWireGuard(WireGuardSpec{})
	b.AddXray(XraySpec{})
	if g := b.Snapshot(); len(g.Nodes) != 0 {
		t.Fatalf("empty inputs produced nodes: %+v", g)
	}
}

func TestBuilderRejectsBadKind(t *testing.T) {
	b := NewBuilder(0)
	b.AddInterface(InterfaceSpec{Name: "x", Kind: NodeKind("wrong")})
	if g := b.Snapshot(); len(g.Nodes) != 0 {
		t.Fatal("invalid kind accepted")
	}
}

func TestValidatesEdgeToMissingNode(t *testing.T) {
	g := Graph{
		SchemaVersion: SchemaVersion,
		Nodes:         []Node{{ID: "if:lan0", Kind: NodeInterface, Label: "lan0"}},
		Edges:         []Edge{{From: "if:lan0", To: "if:missing", Kind: EdgeMember}},
	}
	if err := g.Valid(); err == nil {
		t.Fatal("missing-node edge accepted")
	}
}

func TestValidatesBadEdgeKind(t *testing.T) {
	g := Graph{
		SchemaVersion: SchemaVersion,
		Nodes:         []Node{{ID: "if:a", Kind: NodeInterface, Label: "a"}, {ID: "if:b", Kind: NodeInterface, Label: "b"}},
		Edges:         []Edge{{From: "if:a", To: "if:b", Kind: EdgeKind("wrong")}},
	}
	if err := g.Valid(); err == nil {
		t.Fatal("bad edge kind accepted")
	}
}
