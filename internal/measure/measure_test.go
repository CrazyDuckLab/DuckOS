// G20 measurement quality/history torture regression.
package measure

import (
	"strings"
	"testing"
	"time"
)

func sample(metric string, at time.Time, quality float64) Sample {
	return Sample{
		Metric:  metric,
		Value:   42.5,
		Unit:    "ms",
		Quality: quality,
		Method:  MethodProbe,
		At:      at,
		Source:  "doctor",
	}
}

func TestMethod_Valid(t *testing.T) {
	for _, m := range []Method{MethodProbe, MethodPassive, MethodExternal} {
		if !m.Valid() {
			t.Errorf("%q must be valid", m)
		}
	}
	if Method("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestSample_Validate(t *testing.T) {
	s := sample("wan.latency", time.Now().UTC(), 0.9)
	if err := s.Validate(); err != nil {
		t.Fatalf("valid sample: %v", err)
	}
	// Bad metric
	bad := sample("X", time.Now().UTC(), 0.9)
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad metric must be rejected")
	}
	// Bad method
	bad2 := sample("wan.latency", time.Now().UTC(), 0.9)
	bad2.Method = Method("garbage")
	if err := bad2.Validate(); err == nil {
		t.Fatalf("bad method must be rejected")
	}
	// Quality out of range
	bad3 := sample("wan.latency", time.Now().UTC(), 1.5)
	if err := bad3.Validate(); err == nil {
		t.Fatalf("quality > 1 must be rejected")
	}
	bad4 := sample("wan.latency", time.Now().UTC(), -0.1)
	if err := bad4.Validate(); err == nil {
		t.Fatalf("quality < 0 must be rejected")
	}
	// Zero at
	bad5 := sample("wan.latency", time.Time{}, 0.5)
	if err := bad5.Validate(); err == nil {
		t.Fatalf("zero at must be rejected")
	}
}

func TestStore_AppendAndLatest(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	if err := s.Append(sample("wan.latency", now, 0.9)); err != nil {
		t.Fatal(err)
	}
	latest, ok := s.Latest("wan.latency")
	if !ok || latest.Quality != 0.9 {
		t.Fatalf("drift: %+v", latest)
	}
}

func TestStore_AppendRejectsInvalid(t *testing.T) {
	s := NewStore()
	if err := s.Append(sample("X", time.Now().UTC(), 0.5)); err == nil {
		t.Fatalf("invalid metric must be rejected")
	}
}

func TestStore_TrimsToMax(t *testing.T) {
	s := NewStore()
	s.SetLimits(3, time.Hour)
	base := time.Now().UTC()
	for i := 0; i < 10; i++ {
		if err := s.Append(sample("wan.latency", base.Add(time.Duration(i)*time.Millisecond), 0.5)); err != nil {
			t.Fatal(err)
		}
	}
	hist := s.History("wan.latency")
	if len(hist) != 3 {
		t.Fatalf("expected 3 samples after trim, got %d", len(hist))
	}
	if hist[0].At != base.Add(9*time.Millisecond) {
		t.Fatalf("newest must be first, got %v", hist[0].At)
	}
}

func TestStore_GCExpired(t *testing.T) {
	s := NewStore()
	s.SetLimits(64, 50*time.Millisecond)
	now := time.Now().UTC()
	_ = s.Append(sample("wan.latency", now.Add(-time.Second), 0.5))
	_ = s.Append(sample("wan.latency", now, 0.5))
	removed := s.GCExpired()
	if removed != 1 {
		t.Fatalf("expected 1 expired sample removed, got %d", removed)
	}
	if c := s.Counters(); c.Samples != 1 {
		t.Fatalf("expected 1 surviving sample, got %+v", c)
	}
}

func TestStore_GCExpiredRemovesEmptyMetrics(t *testing.T) {
	s := NewStore()
	s.SetLimits(64, 50*time.Millisecond)
	now := time.Now().UTC()
	_ = s.Append(sample("wan.latency", now.Add(-time.Second), 0.5))
	s.GCExpired()
	if len(s.AllMetrics()) != 0 {
		t.Fatalf("empty metrics must be removed, got %v", s.AllMetrics())
	}
}

func TestStore_AllMetricsSorted(t *testing.T) {
	s := NewStore()
	now := time.Now().UTC()
	_ = s.Append(sample("zeta.latency", now, 0.5))
	_ = s.Append(sample("alpha.latency", now, 0.5))
	_ = s.Append(sample("mu.loss", now, 0.5))
	if got := s.AllMetrics(); !equalStrings(got, []string{"alpha.latency", "mu.loss", "zeta.latency"}) {
		t.Fatalf("AllMetrics drift: %v", got)
	}
}

func TestStore_QualitySummary(t *testing.T) {
	s := NewStore()
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		q := float64(i+1) / 10.0
		_ = s.Append(sample("wan.latency", base.Add(time.Duration(i)*time.Millisecond), q))
	}
	stats, ok := s.QualitySummary("wan.latency", 3)
	if !ok {
		t.Fatalf("expected summary")
	}
	if stats.Count != 3 {
		t.Fatalf("count drift: %d", stats.Count)
	}
	if stats.Min != 0.3 || stats.Max != 0.5 {
		t.Fatalf("min/max drift: %+v", stats)
	}
	if stats.Average < 0.39 || stats.Average > 0.41 {
		t.Fatalf("avg drift: %v", stats.Average)
	}
}

func TestStore_QualitySummaryUnknownMetric(t *testing.T) {
	s := NewStore()
	if _, ok := s.QualitySummary("missing", 10); ok {
		t.Fatalf("missing metric must return ok=false")
	}
}

func TestStore_QualitySummaryAllSamples(t *testing.T) {
	s := NewStore()
	base := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_ = s.Append(sample("wan.latency", base.Add(time.Duration(i)*time.Millisecond), float64(i+1)/10.0))
	}
	stats, ok := s.QualitySummary("wan.latency", 0)
	if !ok {
		t.Fatalf("expected summary")
	}
	if stats.Count != 3 {
		t.Fatalf("count drift: %d", stats.Count)
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

// silence unused import warning for strings.
var _ = strings.TrimSpace
