// G05 live health observation torture regression.
package livehealth

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func sampleObs(comp, metric string, at time.Time) Observation {
	return Observation{
		Component:  comp,
		Metric:     metric,
		Value:      42,
		Unit:       "count",
		Severity:   SeverityOK,
		ObservedAt: at,
		Source:     "doctor",
	}
}

func TestSeverity_Valid(t *testing.T) {
	for _, s := range []Severity{SeverityOK, SeverityInfo, SeverityWarning, SeverityCritical} {
		if !s.Valid() {
			t.Errorf("%q must be valid", s)
		}
	}
	if Severity("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestObservation_Validate(t *testing.T) {
	o := sampleObs("network.firewall", "input-drops", time.Now().UTC())
	if err := o.Validate(); err != nil {
		t.Fatalf("valid observation: %v", err)
	}
	// Bad component
	o2 := sampleObs("Bad", "input-drops", time.Now().UTC())
	if err := o2.Validate(); err == nil {
		t.Fatalf("bad component must be rejected")
	}
	// Bad metric
	o3 := sampleObs("network.firewall", "Bad-Metric", time.Now().UTC())
	if err := o3.Validate(); err == nil {
		t.Fatalf("bad metric must be rejected")
	}
	// Zero observedAt
	o4 := sampleObs("network.firewall", "input-drops", time.Time{})
	if err := o4.Validate(); err == nil {
		t.Fatalf("zero observedAt must be rejected")
	}
}

func TestObservation_Key(t *testing.T) {
	o := sampleObs("net-1", "x", time.Now())
	if got := o.Key(); got != "net-1:x" {
		t.Fatalf("Key drift: %q", got)
	}
}

func TestStore_AppendAndLatest(t *testing.T) {
	s := NewStore()
	if err := s.Append(sampleObs("net-1", "drops", time.Now().UTC())); err != nil {
		t.Fatalf("Append: %v", err)
	}
	latest, ok := s.Latest("net-1", "drops")
	if !ok || latest.Metric != "drops" {
		t.Fatalf("drift: %+v", latest)
	}
}

func TestStore_AppendRejectsInvalid(t *testing.T) {
	s := NewStore()
	if err := s.Append(sampleObs("Bad", "x", time.Now().UTC())); err == nil {
		t.Fatalf("invalid component must be rejected")
	}
	if err := s.Append(Observation{Component: "net-1", Metric: "x", Severity: "garbage", ObservedAt: time.Now()}); err == nil {
		t.Fatalf("invalid severity must be rejected")
	}
}

func TestStore_TrimsToMax(t *testing.T) {
	s := NewStore()
	s.SetMax(3)
	base := time.Now().UTC()
	for i := 0; i < 10; i++ {
		if err := s.Append(sampleObs("net-1", "drops", base.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatal(err)
		}
	}
	hist := s.History("net-1", "drops")
	if len(hist) != 3 {
		t.Fatalf("expected 3 entries after trim, got %d", len(hist))
	}
	// Newest first
	if hist[0].ObservedAt != base.Add(9*time.Second) {
		t.Fatalf("newest must be first, got %v", hist[0].ObservedAt)
	}
}

func TestStore_StaleKeys(t *testing.T) {
	s := NewStore()
	s.SetStale(50 * time.Millisecond)
	now := time.Now().UTC()
	_ = s.Append(sampleObs("frsh-1", "x", now))
	time.Sleep(60 * time.Millisecond)
	_ = s.Append(sampleObs("frsh-1", "x", time.Now().UTC()))
	_ = s.Append(sampleObs("stale-comp", "y", now.Add(-time.Second)))
	stale := s.StaleKeys()
	if len(stale) != 1 || stale[0] != "stale-comp:y" {
		t.Fatalf("StaleKeys drift: %v", stale)
	}
}

func TestStore_Counters(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	_ = s.Append(sampleObs("aaa", "x", now))
	_ = s.Append(sampleObs("aaa", "y", now))
	_ = s.Append(sampleObs("bbb", "z", now))
	c := s.Counters()
	if c.Keys != 3 || c.Total != 3 {
		t.Fatalf("counter drift: %+v", c)
	}
}

func TestStore_Purge(t *testing.T) {
	s := NewStore()
	_ = s.Append(sampleObs("aaa", "x", time.Now().UTC()))
	s.Purge("aaa", "x")
	if _, ok := s.Latest("aaa", "x"); ok {
		t.Fatalf("purged observation must not survive")
	}
}

func TestStore_LatestForComponent(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	_ = s.Append(sampleObs("aaa", "x", now))
	_ = s.Append(sampleObs("aaa", "y", now.Add(time.Second)))
	_ = s.Append(sampleObs("bbb", "z", now))
	latest, ok := s.LatestForComponent("aaa")
	if !ok || latest.Metric != "y" {
		t.Fatalf("LatestForComponent drift: %+v", latest)
	}
}

func TestStore_AllKeysIsSorted(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	_ = s.Append(sampleObs("z-1", "x-1", now))
	_ = s.Append(sampleObs("aaa", "x", now))
	_ = s.Append(sampleObs("m-1", "y-1", now))
	keys := s.AllKeys()
	if want := []string{"aaa:x", "m-1:y-1", "z-1:x-1"}; !equalStrings(keys, want) {
		t.Fatalf("keys drift: %v want %v", keys, want)
	}
}

func TestStore_ConcurrentAppend(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	now := time.Now().UTC()
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = s.Append(sampleObs("comp-1", "metric", now.Add(time.Duration(i)*time.Microsecond)))
		}(i)
	}
	wg.Wait()
	c := s.Counters()
	if c.Total != 32 {
		t.Fatalf("expected 32 total observations, got %d", c.Total)
	}
}

func TestStore_HistoryIsCopy(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	_ = s.Append(sampleObs("aaa", "x", now))
	h1 := s.History("aaa", "x")
	h1[0].Value = 999
	h2 := s.History("aaa", "x")
	if h2[0].Value == 999 {
		t.Fatalf("History must return a copy")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// silence unused import warnings for strings.
var _ = strings.TrimSpace
