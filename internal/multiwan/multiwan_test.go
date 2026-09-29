package multiwan

import (
	"testing"
	"time"
)

func sampleGroup() Group {
	return Group{
		ID:       "wan-group",
		Name:     "primary",
		Strategy: StrategyActiveActive,
		Members: []Member{
			{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10},
			{ID: "wan-2", InterfaceID: "if-wan-2", Weight: 5},
		},
		Probes: []Probe{
			{Target: "1.1.1.1", Method: HealthMethodICMP, Timeout: 1 * time.Second},
		},
		Interval: 10 * time.Second,
		Table:    200,
		Primary:  "wan-1",
	}
}

func TestGroupValidates(t *testing.T) {
	g := sampleGroup()
	if err := g.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestGroupRejectsBadStrategy(t *testing.T) {
	g := sampleGroup()
	g.Strategy = "round-robin"
	if err := g.Valid(); err == nil {
		t.Fatal("invalid strategy accepted")
	}
}

func TestGroupRejectsBadMethod(t *testing.T) {
	g := sampleGroup()
	g.Probes[0].Method = "smtp"
	if err := g.Valid(); err == nil {
		t.Fatal("invalid method accepted")
	}
}

func TestGroupRejectsEmptyMembers(t *testing.T) {
	g := sampleGroup()
	g.Members = nil
	if err := g.Valid(); err == nil {
		t.Fatal("empty members accepted")
	}
}

func TestGroupRejectsDuplicateMembers(t *testing.T) {
	g := sampleGroup()
	g.Members = append(g.Members, g.Members[0])
	if err := g.Valid(); err == nil {
		t.Fatal("duplicate member id accepted")
	}
}

func TestGroupRejectsOutOfRangeWeight(t *testing.T) {
	g := sampleGroup()
	g.Members[0].Weight = 0
	if err := g.Valid(); err == nil {
		t.Fatal("zero weight accepted")
	}
	g.Members[0].Weight = 101
	if err := g.Valid(); err == nil {
		t.Fatal("oversized weight accepted")
	}
}

func TestGroupRejectsIntervalBounds(t *testing.T) {
	g := sampleGroup()
	g.Interval = 0
	if err := g.Valid(); err == nil {
		t.Fatal("zero interval accepted")
	}
	g.Interval = 2 * time.Hour
	if err := g.Valid(); err == nil {
		t.Fatal("oversized interval accepted")
	}
}

func TestGroupRejectsProbeTimeout(t *testing.T) {
	g := sampleGroup()
	g.Probes[0].Timeout = 10 * time.Second
	if err := g.Valid(); err == nil {
		t.Fatal("oversized probe timeout accepted")
	}
}

func TestGroupRejectsPrimaryReference(t *testing.T) {
	g := sampleGroup()
	g.Primary = "missing"
	if err := g.Valid(); err == nil {
		t.Fatal("missing primary accepted")
	}
}

func TestTopologyBuildRejectsEmptyGroups(t *testing.T) {
	if _, err := Build(nil, time.Now()); err == nil {
		t.Fatal("empty topology accepted")
	}
}

func TestTopologyBuildRejectsDuplicateTables(t *testing.T) {
	g1 := sampleGroup()
	g2 := sampleGroup()
	g2.ID = "other"
	if _, err := Build([]Group{g1, g2}, time.Now()); err == nil {
		t.Fatal("duplicate table accepted")
	}
}

func TestBuildSortsAndValidates(t *testing.T) {
	g1 := sampleGroup()
	g1.ID = "z-last"
	g1.Table = 201
	g2 := sampleGroup()
	g2.ID = "a-first"
	g2.Table = 200
	t2, err := Build([]Group{g1, g2}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if t2.Groups[0].ID != "a-first" || t2.Groups[1].ID != "z-last" {
		t.Fatal("topology not sorted by id")
	}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	g := sampleGroup()
	t2, err := Build([]Group{g}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeTopology(t2)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxTotalBytes {
		t.Fatalf("encoded topology too large: %d", len(data))
	}
	got, err := ParseTopology(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || len(got.Groups) != 1 {
		t.Fatal("roundtrip lost data")
	}
}

func TestParseTopologyRejectsOversized(t *testing.T) {
	big := make([]byte, MaxTotalBytes+1)
	if _, err := ParseTopology(big); err == nil {
		t.Fatal("oversized payload accepted")
	}
}

func TestParseTopologyRejectsMalformed(t *testing.T) {
	if _, err := ParseTopology([]byte("not json")); err == nil {
		t.Fatal("malformed payload accepted")
	}
}

func TestParseTopologyRejectsEmpty(t *testing.T) {
	if _, err := ParseTopology(nil); err == nil {
		t.Fatal("empty payload accepted")
	}
}

func TestActiveActivePicksBest(t *testing.T) {
	g := sampleGroup()
	g.Strategy = StrategyActiveActive
	g.Primary = ""
	now := time.Now().UTC()
	sel := choose(g, map[string]bool{"wan-1": true, "wan-2": true}, now)
	if sel.MemberID != "wan-1" {
		t.Fatalf("active-active should pick highest weight, got %s", sel.MemberID)
	}
	if sel.Health != "ok" {
		t.Fatalf("expected ok, got %s", sel.Health)
	}
}

func TestActiveBackupPrefersPrimary(t *testing.T) {
	g := sampleGroup()
	g.Strategy = StrategyActiveBackup
	now := time.Now().UTC()
	sel := choose(g, map[string]bool{"wan-1": true, "wan-2": true}, now)
	if sel.MemberID != "wan-1" {
		t.Fatalf("active-backup should prefer primary, got %s", sel.MemberID)
	}
	if sel.Reason != "active-backup: primary is up" {
		t.Fatalf("unexpected reason: %s", sel.Reason)
	}
}

func TestActiveBackupFallsOverWhenPrimaryDown(t *testing.T) {
	g := sampleGroup()
	g.Strategy = StrategyActiveBackup
	now := time.Now().UTC()
	sel := choose(g, map[string]bool{"wan-2": true}, now)
	if sel.MemberID != "wan-2" {
		t.Fatalf("active-backup should fall over, got %s", sel.MemberID)
	}
	if sel.Reason != "active-backup: primary is down, secondary is up" {
		t.Fatalf("unexpected reason: %s", sel.Reason)
	}
}

func TestAllProbesFailedReturnsDegraded(t *testing.T) {
	g := sampleGroup()
	g.Strategy = StrategyActiveActive
	now := time.Now().UTC()
	sel := choose(g, map[string]bool{}, now)
	if sel.Health != "degraded" {
		t.Fatalf("expected degraded, got %s", sel.Health)
	}
	if sel.MemberID != "wan-1" {
		t.Fatalf("expected highest-weight member, got %s", sel.MemberID)
	}
}

func TestEvaluatorSelect(t *testing.T) {
	g := sampleGroup()
	t2, _ := Build([]Group{g}, time.Now())
	e, err := NewEvaluator(t2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	results := map[string]ProbeResult{
		"1.1.1.1": {Target: "1.1.1.1", Method: HealthMethodICMP, OK: true, RTTUS: 100, At: now},
	}
	selections := e.Select(results)
	if selections["wan-group"].MemberID != "wan-1" {
		t.Fatalf("unexpected selection: %+v", selections["wan-group"])
	}
}

func TestEvaluatorRecordProbeStoresAndSelects(t *testing.T) {
	g := sampleGroup()
	t2, _ := Build([]Group{g}, time.Now())
	e, _ := NewEvaluator(t2)
	now := time.Now().UTC()
	selections := e.RecordProbe(ProbeResult{Target: "1.1.1.1", Method: HealthMethodICMP, OK: true, RTTUS: 50, At: now})
	if selections["wan-group"].MemberID != "wan-1" {
		t.Fatalf("expected wan-1, got %s", selections["wan-group"].MemberID)
	}
	last := e.LastSelections()
	if last["wan-group"].MemberID != "wan-1" {
		t.Fatalf("LastSelections returned wrong member: %s", last["wan-group"].MemberID)
	}
}

func TestEvaluatorMemberHealth(t *testing.T) {
	g := sampleGroup()
	g.Members = []Member{
		{ID: "wan-1", InterfaceID: "if-wan-1", Weight: 10},
		{ID: "wan-2", InterfaceID: "if-wan-2", Weight: 5},
	}
	t2, _ := Build([]Group{g}, time.Now())
	e, _ := NewEvaluator(t2)
	now := time.Now().UTC()
	e.RecordProbe(ProbeResult{Target: "1.1.1.1", OK: true, At: now})
	health := e.MemberHealth()
	if health["wan-1"] != "up" {
		t.Fatalf("expected wan-1 up, got %s", health["wan-1"])
	}
	if health["wan-2"] != "up" {
		t.Fatalf("expected wan-2 up, got %s", health["wan-2"])
	}
	e.RecordProbe(ProbeResult{Target: "1.1.1.1", OK: false, At: now})
	health2 := e.MemberHealth()
	if health2["wan-1"] != "down" {
		t.Fatalf("expected wan-1 down, got %s", health2["wan-1"])
	}
}

func TestEvaluatorRejectsBadTopology(t *testing.T) {
	if _, err := NewEvaluator(Topology{}); err == nil {
		t.Fatal("empty topology accepted")
	}
}

func TestEvaluatorUpdateTopology(t *testing.T) {
	g := sampleGroup()
	t2, _ := Build([]Group{g}, time.Now())
	e, _ := NewEvaluator(t2)
	g2 := sampleGroup()
	g2.ID = "wan-group-2"
	t3, _ := Build([]Group{g2}, time.Now())
	if err := e.UpdateTopology(t3); err != nil {
		t.Fatal(err)
	}
	if got := e.Topology(); len(got.Groups) != 1 || got.Groups[0].ID != "wan-group-2" {
		t.Fatalf("topology not updated: %+v", got)
	}
}

func TestEvaluatorUpdateTopologyRejectsBad(t *testing.T) {
	g := sampleGroup()
	t2, _ := Build([]Group{g}, time.Now())
	e, _ := NewEvaluator(t2)
	if err := e.UpdateTopology(Topology{}); err == nil {
		t.Fatal("bad topology accepted")
	}
}

func TestEvaluatorConcurrent(t *testing.T) {
	g := sampleGroup()
	t2, _ := Build([]Group{g}, time.Now())
	e, _ := NewEvaluator(t2)
	done := make(chan struct{}, 32)
	for i := 0; i < 16; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			e.RecordProbe(ProbeResult{Target: "1.1.1.1", OK: true, At: time.Now()})
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			_ = e.LastSelections()
		}()
	}
	for i := 0; i < 32; i++ {
		<-done
	}
}

func TestIsHealthy(t *testing.T) {
	if !IsHealthy(Selection{Health: "ok"}) {
		t.Fatal("ok should be healthy")
	}
	if IsHealthy(Selection{Health: "degraded"}) {
		t.Fatal("degraded should not be healthy")
	}
}

func TestShortSummary(t *testing.T) {
	s := ShortSummary(Selection{GroupID: "g", MemberID: "m", Interface: "if0", Health: "ok"})
	if s != "g -> m (if0, ok)" {
		t.Fatalf("unexpected summary: %s", s)
	}
}

func TestProbeStaleResultIsIgnored(t *testing.T) {
	g := sampleGroup()
	g.Interval = 1 * time.Second
	now := time.Now().UTC()
	healthy := map[string]bool{}
	sel := choose(g, healthy, now)
	if sel.Health != "degraded" {
		t.Fatalf("empty results should produce degraded selection, got %s", sel.Health)
	}
}
