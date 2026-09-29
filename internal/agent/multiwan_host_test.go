package agent

import (
	"context"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
)

func sampleTopology() multiwan.Topology {
	g := multiwan.Group{
		ID:       "wan-group",
		Name:     "primary",
		Strategy: multiwan.StrategyActiveActive,
		Members: []multiwan.Member{
			{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10},
			{ID: "wan-2", InterfaceID: "if-wan-2", Weight: 5},
		},
		Probes: []multiwan.Probe{
			{Target: "1.1.1.1", Method: multiwan.HealthMethodICMP, Timeout: 1 * time.Second},
		},
		Interval: 10 * time.Second,
		Table:    200,
		Primary:  "wan-1",
	}
	t, _ := multiwan.Build([]multiwan.Group{g}, time.Now())
	return t
}

func TestMultiWANHostRoundTrip(t *testing.T) {
	h, err := NewMultiWANHost(sampleTopology())
	if err != nil {
		t.Fatal(err)
	}
	topology, sel, health, err := h.MultiWAN(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(topology.Groups))
	}
	if _, ok := sel["wan-group"]; ok {
		t.Fatal("selections should be empty before any probe")
	}
	// health is reported as "unknown" for every member
	// until a probe has run; the API contract is that the
	// map is non-empty but every value is the unknown state.
	if health["wan-1"] != "unknown" {
		t.Fatalf("expected wan-1 unknown, got %s", health["wan-1"])
	}
}

func TestMultiWANHostRecordsAndReads(t *testing.T) {
	h, _ := NewMultiWANHost(sampleTopology())
	selections := h.RecordProbe(multiwan.ProbeResult{Target: "1.1.1.1", Method: multiwan.HealthMethodICMP, OK: true, RTTUS: 50, At: time.Now()})
	if selections["wan-group"].MemberID != "wan-1" {
		t.Fatalf("expected wan-1, got %s", selections["wan-group"].MemberID)
	}
	last := h.LastSelections()
	if last["wan-group"].MemberID != "wan-1" {
		t.Fatalf("LastSelections returned wrong member: %s", last["wan-group"].MemberID)
	}
	health := h.MemberHealth()
	if health["wan-1"] != "up" {
		t.Fatalf("expected wan-1 up, got %s", health["wan-1"])
	}
}

func TestMultiWANHostUpdateTopology(t *testing.T) {
	h, _ := NewMultiWANHost(sampleTopology())
	g := sampleTopology().Groups[0]
	g.ID = "wan-group-2"
	t2, _ := multiwan.Build([]multiwan.Group{g}, time.Now())
	if err := h.UpdateTopology(t2); err != nil {
		t.Fatal(err)
	}
	if got := h.Topology(); len(got.Groups) != 1 || got.Groups[0].ID != "wan-group-2" {
		t.Fatalf("topology not updated: %+v", got)
	}
}

func TestMultiWANHostNilReceiver(t *testing.T) {
	var h *MultiWANHost
	if _, _, _, err := h.MultiWAN(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.Topology().SchemaVersion != "" {
		t.Fatal("nil host produced non-empty topology")
	}
}

func TestMultiWANHostContextCancel(t *testing.T) {
	h, _ := NewMultiWANHost(sampleTopology())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := h.MultiWAN(ctx); err == nil {
		t.Fatal("cancelled context accepted")
	}
}
