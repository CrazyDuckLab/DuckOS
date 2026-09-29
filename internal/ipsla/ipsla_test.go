// ipsla/ipsla_test.go pins the V1 IP SLA
// state machine and Track objects.
package ipsla

import (
	"testing"
	"time"
)

func TestProbeValid(t *testing.T) {
	good := Probe{ID: "p1", Name: "gateway", Kind: ProbeICMP, Target: "1.1.1.1", Interval: 5 * time.Second, Timeout: time.Second, ThresholdDown: 3, ThresholdUp: 3, HoldDown: 10 * time.Second, History: 100}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noTarget := Probe{ID: "p1", Kind: ProbeICMP, Interval: 5 * time.Second, Timeout: time.Second, ThresholdDown: 3, ThresholdUp: 3}
	if err := noTarget.Valid(); err == nil {
		t.Fatal("missing target must fail")
	}
	noPort := Probe{ID: "p1", Kind: ProbeHTTP, Target: "example.com", Interval: 5 * time.Second, Timeout: time.Second, ThresholdDown: 3, ThresholdUp: 3}
	if err := noPort.Valid(); err == nil {
		t.Fatal("http without port must fail")
	}
	badThresh := Probe{ID: "p1", Kind: ProbeICMP, Target: "1.1.1.1", Interval: 5 * time.Second, Timeout: time.Second, ThresholdDown: 0, ThresholdUp: 1}
	if err := badThresh.Valid(); err == nil {
		t.Fatal("thresholdDown 0 must fail")
	}
}

func TestManagerStateMachine(t *testing.T) {
	m := NewManager(nil)
	probe := Probe{ID: "p1", Kind: ProbeICMP, Target: "1.1.1.1", Interval: time.Second, Timeout: 500 * time.Millisecond, ThresholdDown: 2, ThresholdUp: 2, HoldDown: 5 * time.Second}
	if err := m.Set(probe); err != nil {
		t.Fatal(err)
	}
	// Initial state: Unknown.
	ps, _ := m.State("p1")
	if ps.State != StateUnknown {
		t.Fatalf("initial: %s", ps.State)
	}
	// Two consecutive successes -> Up.
	m.Ingest("p1", Sample{OK: true, RTTMs: 10})
	state, _ := m.Ingest("p1", Sample{OK: true, RTTMs: 12})
	if state != StateUp {
		t.Fatalf("expected up, got %s", state)
	}
	// Two consecutive failures while in hold-down
	// do NOT change the state.
	m.Ingest("p1", Sample{OK: false})
	state, _ = m.Ingest("p1", Sample{OK: false})
	if state != StateUp {
		t.Fatalf("hold-down broken: state=%s", state)
	}
	// Move the clock past the hold-down by
	// directly clearing HoldExpires.
	m.mu.Lock()
	m.states["p1"].HoldExpires = time.Time{}
	m.mu.Unlock()
	// Now two failures flip the state.
	state, _ = m.Ingest("p1", Sample{OK: false})
	state, _ = m.Ingest("p1", Sample{OK: false})
	if state != StateDown {
		t.Fatalf("expected down, got %s", state)
	}
}

func TestManagerStats(t *testing.T) {
	m := NewManager(nil)
	m.Set(Probe{ID: "p1", Kind: ProbeICMP, Target: "1.1.1.1", Interval: time.Second, Timeout: 500 * time.Millisecond, ThresholdDown: 3, ThresholdUp: 3, History: 100})
	for i, ok := range []bool{true, true, false, true, true} {
		m.Ingest("p1", Sample{OK: ok, RTTMs: float64(10 + i)})
	}
	ps, _ := m.State("p1")
	if ps.LossPct != 20 {
		t.Fatalf("loss: %f", ps.LossPct)
	}
	if ps.LatencyMs < 10 || ps.LatencyMs > 20 {
		t.Fatalf("latency: %f", ps.LatencyMs)
	}
}

func TestTrackValid(t *testing.T) {
	good := Track{ID: "t1", Name: "wan1-down", ProbeID: "p1", NotState: StateDown}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noProbe := Track{ID: "t1", Name: "x"}
	if err := noProbe.Valid(); err == nil {
		t.Fatal("missing probeId must fail")
	}
	badState := Track{ID: "t1", ProbeID: "p1", NotState: "garbage"}
	if err := badState.Valid(); err == nil {
		t.Fatal("unknown state must fail")
	}
}
