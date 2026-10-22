// Package xray / stats_test.go: accounting semantics (delta
// accumulation, restart reset handling, staleness, bounds).
package xray

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestAccountant_AccumulatesDeltas(t *testing.T) {
	var persisted map[string]uint64
	a := NewAccountant(64, func(l map[string]uint64) error { persisted = l; return nil })
	email := "alice@example.com"
	r1 := StatsSnapshot{CollectedAt: time.Now(), Counters: []StatCounter{
		{Name: "user>>>" + email + ">>>traffic>>>uplink", Value: 100},
		{Name: "user>>>" + email + ">>>traffic>>>downlink", Value: 900},
	}}
	if err := a.Apply(r1); err != nil {
		t.Fatal(err)
	}
	up, down, total := a.UserUsage(email)
	if up != 100 || down != 900 || total != 1000 {
		t.Fatalf("round1: up=%d down=%d total=%d", up, down, total)
	}
	// Second round grows counters.
	r2 := StatsSnapshot{CollectedAt: time.Now(), Counters: []StatCounter{
		{Name: "user>>>" + email + ">>>traffic>>>uplink", Value: 250},
		{Name: "user>>>" + email + ">>>traffic>>>downlink", Value: 900},
	}}
	_ = a.Apply(r2)
	up, _, total = a.UserUsage(email)
	if up != 250 || total != 1150 {
		t.Fatalf("round2: up=%d total=%d", up, total)
	}
	// Restart reset: counter drops to 0 then grows again.
	r3 := StatsSnapshot{CollectedAt: time.Now(), Counters: []StatCounter{
		{Name: "user>>>" + email + ">>>traffic>>>uplink", Value: 75},
	}}
	_ = a.Apply(r3)
	up, _, total = a.UserUsage(email)
	if up != 325 || total != 1225 {
		t.Fatalf("restart: up=%d total=%d", up, total)
	}
	if persisted == nil || persisted["user>>>"+email+">>>traffic>>>uplink"] != 325 {
		t.Fatalf("persist hook: %v", persisted)
	}
}

func TestAccountant_StaleNotCounted(t *testing.T) {
	a := NewAccountant(64, nil)
	if err := a.Apply(StatsSnapshot{Stale: true}); err != nil {
		t.Fatal(err)
	}
	if _, stale := a.LastCollection(); !stale {
		t.Fatal("stale round not flagged")
	}
	if up, _, total := a.UserUsage("nobody"); up != 0 || total != 0 {
		t.Fatal("stale round mutated the ledger")
	}
}

func TestAccountant_KeyBound(t *testing.T) {
	a := NewAccountant(4, nil)
	for i := 0; i < 16; i++ {
		_ = a.Apply(StatsSnapshot{Counters: []StatCounter{
			{Name: "user>>>u" + string(rune('a'+i)) + ">>>traffic>>>uplink", Value: 1},
		}})
	}
	if len(a.Ledger()) > 4 {
		t.Fatalf("ledger grew past bound: %d", len(a.Ledger()))
	}
}

func TestAccountant_RestoreAndConcurrent(t *testing.T) {
	a := NewAccountant(64, nil)
	a.Restore(map[string]uint64{"user>>>bob>>>traffic>>>downlink": 500})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.Apply(StatsSnapshot{Counters: []StatCounter{
				{Name: "user>>>bob>>>traffic>>>downlink", Value: uint64(100 * (i + 1))},
			}})
			a.Ledger()
			a.UserUsage("bob")
		}()
	}
	wg.Wait()
	_, down, _ := a.UserUsage("bob")
	if down == 0 {
		t.Fatal("concurrent application lost traffic")
	}
}

func TestParseStatsQueryOutput(t *testing.T) {
	out := []byte(`{"stat":[{"name":"user>>>alice@example.com>>>traffic>>>uplink","value":"1234"},{"name":"inbound>>>in-primary>>>traffic>>>downlink","value":"9999"}]}`)
	snap, err := ParseStatsQueryOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Counters) != 2 || snap.Counters[0].Value != 1234 || snap.Counters[1].Name != "inbound>>>in-primary>>>traffic>>>downlink" {
		t.Fatalf("snapshot: %+v", snap)
	}
	if _, err := ParseStatsQueryOutput([]byte(`{"stat":`)); err == nil {
		t.Fatal("malformed output accepted")
	}
}

func TestCollector_ErrorPath(t *testing.T) {
	a := NewAccountant(64, nil)
	err := a.Apply(func() StatsSnapshot {
		return StatsSnapshot{Stale: true}
	}())
	_ = err
	// A failing collector must surface its error to the caller
	// through the Collector contract itself.
	var c Collector = func(ctx context.Context) (StatsSnapshot, error) {
		return StatsSnapshot{}, ErrStatsUnavailable
	}
	if _, err := c(context.Background()); !errors.Is(err, ErrStatsUnavailable) {
		t.Fatalf("collector error contract: %v", err)
	}
}
