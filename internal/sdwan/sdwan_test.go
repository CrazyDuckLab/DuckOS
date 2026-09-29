// sdwan/sdwan_test.go pins the V1 SD-WAN
// policy layer.
package sdwan

import "testing"

func TestPolicyValid(t *testing.T) {
	good := Policy{
		ID: "p1", Name: "voice",
		ApplicationClass: ApplicationClass{Name: "voice", DSCP: 46},
		Paths: []Path{
			{ID: "wan1", WAN: "wan1", Cost: 10, Preference: 100},
			{ID: "wan2", WAN: "wan2", Cost: 20, Preference: 50},
		},
		Weights: []Weight{
			{Criteria: CriterionLatency, Weight: 10},
			{Criteria: CriterionLoss, Weight: 5},
		},
		HysteresisMs: 50,
	}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noPath := Policy{ID: "p1", ApplicationClass: ApplicationClass{Name: "x"}}
	if err := noPath.Valid(); err == nil {
		t.Fatal("missing paths must fail")
	}
}

func TestDetectBrownout(t *testing.T) {
	if DetectBrownout(PathScore{Reachable: true, LatencyMs: 10, JitterMs: 1, LossPct: 0}, 100, 50, 5).Brownout {
		t.Fatal("healthy must not be brownout")
	}
	if !DetectBrownout(PathScore{Reachable: false}, 100, 50, 5).Brownout {
		t.Fatal("unreachable must be brownout")
	}
	if !DetectBrownout(PathScore{Reachable: true, LatencyMs: 200, JitterMs: 1, LossPct: 0}, 100, 50, 5).Brownout {
		t.Fatal("high latency must be brownout")
	}
	if !DetectBrownout(PathScore{Reachable: true, LatencyMs: 10, JitterMs: 100, LossPct: 0}, 100, 50, 5).Brownout {
		t.Fatal("high jitter must be brownout")
	}
	if !DetectBrownout(PathScore{Reachable: true, LatencyMs: 10, JitterMs: 1, LossPct: 50}, 100, 50, 5).Brownout {
		t.Fatal("high loss must be brownout")
	}
}

func TestScoreDeterministic(t *testing.T) {
	p := Policy{
		ID: "p1", ApplicationClass: ApplicationClass{Name: "voice"},
		Paths: []Path{
			{ID: "wan1", WAN: "wan1"},
			{ID: "wan2", WAN: "wan2"},
		},
		Weights: []Weight{
			{Criteria: CriterionLatency, Weight: 10},
			{Criteria: CriterionPreference, Weight: 5},
		},
	}
	scores := map[string]PathScore{
		"wan1": {PathID: "wan1", LatencyMs: 5, Reachable: true, Preference: 100},
		"wan2": {PathID: "wan2", LatencyMs: 50, Reachable: true, Preference: 50},
	}
	first, firstScore := Score(p, scores)
	for i := 0; i < 10; i++ {
		got, gotScore := Score(p, scores)
		if got != first || gotScore != firstScore {
			t.Fatalf("score not deterministic")
		}
	}
	if first != "wan1" {
		t.Fatalf("expected wan1, got %s", first)
	}
}

func TestEvaluateHysteresis(t *testing.T) {
	m := NewManager()
	p := Policy{
		ID: "p1", ApplicationClass: ApplicationClass{Name: "voice"},
		Paths:        []Path{{ID: "a", WAN: "wan1"}, {ID: "b", WAN: "wan2"}},
		Weights:      []Weight{{Criteria: CriterionLatency, Weight: 10}},
		HysteresisMs: 1000,
	}
	m.Set(p)
	// First evaluation: best is "a".
	scores := map[string]PathScore{
		"a": {PathID: "a", LatencyMs: 5, Reachable: true},
		"b": {PathID: "b", LatencyMs: 50, Reachable: true},
	}
	h, _ := m.Evaluate("p1", scores, 100, 100, 100)
	if h.CurrentPath != "a" {
		t.Fatalf("current: %s", h.CurrentPath)
	}
	// Small change: still on a (hysteresis).
	scores = map[string]PathScore{
		"a": {PathID: "a", LatencyMs: 5, Reachable: true},
		"b": {PathID: "b", LatencyMs: 6, Reachable: true},
	}
	h, _ = m.Evaluate("p1", scores, 100, 100, 100)
	if h.CurrentPath != "a" {
		t.Fatalf("hysteresis broken: %s", h.CurrentPath)
	}
	// Large change: flips to b.
	scores = map[string]PathScore{
		"a": {PathID: "a", LatencyMs: 5000, Reachable: true},
		"b": {PathID: "b", LatencyMs: 5, Reachable: true},
	}
	h, _ = m.Evaluate("p1", scores, 100, 100, 100)
	if h.CurrentPath != "b" {
		t.Fatalf("large change did not flip: %s", h.CurrentPath)
	}
}
