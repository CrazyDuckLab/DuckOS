package agent

import (
	"context"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/topology"
)

type stubInterface struct {
	specs []topology.InterfaceSpec
}

func (s *stubInterface) Interfaces(context.Context) ([]topology.InterfaceSpec, error) {
	return s.specs, nil
}

type stubDevice struct {
	specs []topology.DeviceSpec
}

func (s *stubDevice) Devices(context.Context) ([]topology.DeviceSpec, error) {
	return s.specs, nil
}

type stubWireGuard struct {
	specs []topology.WireGuardSpec
}

func (s *stubWireGuard) WireGuardServers(context.Context) ([]topology.WireGuardSpec, error) {
	return s.specs, nil
}

type stubXrayTopology struct {
	specs []topology.XraySpec
}

func (s *stubXrayTopology) XrayServers(context.Context) ([]topology.XraySpec, error) {
	return s.specs, nil
}

func TestTopologyHostAggregatesProviders(t *testing.T) {
	h := NewTopologyHost()
	h.interfaces = &stubInterface{specs: []topology.InterfaceSpec{{Name: "lan0", Kind: topology.NodeInterface, State: "up"}}}
	h.devices = &stubDevice{specs: []topology.DeviceSpec{{ID: "abc", Link: "lan0", Role: "trusted"}}}
	h.wireguard = &stubWireGuard{specs: []topology.WireGuardSpec{{Name: "main", Peers: []string{"p1"}}}}
	h.xray = &stubXrayTopology{specs: []topology.XraySpec{{Name: "vless", Users: []string{"u1"}}}}
	graph, err := h.Topology(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) < 5 {
		t.Fatalf("expected at least 5 nodes, got %d", len(graph.Nodes))
	}
}

func TestTopologyHostNilProviders(t *testing.T) {
	h := NewTopologyHost()
	graph, err := h.Topology(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 0 {
		t.Fatalf("expected empty graph, got %d nodes", len(graph.Nodes))
	}
}

func TestTopologyHostContextCancel(t *testing.T) {
	h := NewTopologyHost()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Topology(ctx); err == nil {
		t.Fatal("cancelled context accepted")
	}
}

func TestTopologyHostNilReceiver(t *testing.T) {
	var h *TopologyHost
	graph, err := h.Topology(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if graph.SchemaVersion != "" {
		t.Fatal("nil host produced non-empty graph")
	}
}

func TestTopologyHostTimestampStable(t *testing.T) {
	fixed := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	h := NewTopologyHost()
	h.now = func() time.Time { return fixed }
	h.interfaces = &stubInterface{specs: []topology.InterfaceSpec{{Name: "lan0"}}}
	graph, _ := h.Topology(context.Background())
	if graph.Generated != fixed.Unix() {
		t.Fatalf("expected %d, got %d", fixed.Unix(), graph.Generated)
	}
}
