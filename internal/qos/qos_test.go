// qos_test.go exercises the model and renderer. The
// Linux adapter is tested in adapter_linux_test.go; the
// controller is exercised in controller_test.go.
package qos

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestShaperValid exercises the per-shaper validation
// path: each algorithm has a slightly different set of
// accepted fields and the test names the failure mode.
func TestShaperValid(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(s *Shaper)
		wantErr string
	}{
		{"base cake", func(s *Shaper) {}, ""},
		{"base fq_codel", func(s *Shaper) { s.Algorithm = AlgorithmFQCoDel }, ""},
		{"bad id", func(s *Shaper) { s.ID = "1nope" }, "id is not canonical"},
		{"bad link", func(s *Shaper) { s.Link = "9bad" }, "link is not canonical"},
		{"bad direction", func(s *Shaper) { s.Direction = "sideways" }, "direction is not canonical"},
		{"bad algorithm", func(s *Shaper) { s.Algorithm = "fifo" }, "algorithm is not canonical"},
		{"rate zero", func(s *Shaper) { s.RateKbps = 0 }, "rate out of range"},
		{"rate huge", func(s *Shaper) { s.RateKbps = MaxRateKbps + 1 }, "rate out of range"},
		{"burst negative", func(s *Shaper) { s.BurstKbyte = -1 }, "burst out of range"},
		{"burst huge", func(s *Shaper) { s.BurstKbyte = MaxBurstKbytes + 1 }, "burst out of range"},
		{"cake priority bogus", func(s *Shaper) { s.Algorithm = AlgorithmCAKE; s.Priority = "premium" }, "priority tier is not canonical"},
		{"cake ingress refused", func(s *Shaper) { s.Algorithm = AlgorithmCAKE; s.Ingress = true }, "cake does not support ingress"},
		{"fq_codel priority refused", func(s *Shaper) { s.Algorithm = AlgorithmFQCoDel; s.Priority = PriorityVoice }, "fq_codel does not honour priority"},
		{"fq_codel overhead refused", func(s *Shaper) { s.Algorithm = AlgorithmFQCoDel; s.Overhead = 18 }, "fq_codel does not honour overhead"},
		{"ingress only fq_codel", func(s *Shaper) { s.Algorithm = AlgorithmCAKE; s.Direction = DirectionIngress }, "only fq_codel admits ingress"},
		{"description oversized", func(s *Shaper) { s.Description = strings.Repeat("a", 129) }, "description is oversized"},
		{"ok cake priority voice", func(s *Shaper) { s.Algorithm = AlgorithmCAKE; s.Priority = PriorityVoice }, ""},
		{"ok fq_codel ingress", func(s *Shaper) { s.Algorithm = AlgorithmFQCoDel; s.Direction = DirectionIngress }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 50_000, BurstKbyte: 256}
			tc.mutate(&s)
			err := s.Valid()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

// TestClassAndFilter cover the per-element validation
// of the supporting types. The renderer relies on this
// gate so a malformed class/filter is rejected before
// the plan is built.
func TestClassAndFilter(t *testing.T) {
	cases := []struct {
		name    string
		c       Class
		wantErr string
	}{
		{"ok", Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7}, ""},
		{"bad id", Class{ID: "9", ShaperID: "wan1", Name: "Voice", MarkBits: 7}, "id is not canonical"},
		{"bad name", Class{ID: "voice", ShaperID: "wan1", Name: "", MarkBits: 7}, "class name is empty or oversized"},
		{"mark too big", Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: MaxMarkBits + 1}, "markBits out of range"},
		{"share too big", Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7, RateShare: 101}, "rateShare out of range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.Valid()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
	filterCases := []struct {
		name    string
		f       Filter
		wantErr string
	}{
		{"ok", Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7}, ""},
		{"bad protocol", Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7, Protocol: "tcp"}, "filter protocol is not canonical"},
		{"ok proto all", Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7, Protocol: "all"}, ""},
		{"ok proto ipv6", Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7, Protocol: "ipv6"}, ""},
		{"mark too big", Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: MaxMarkBits + 1}, "filter mark out of range"},
	}
	for _, tc := range filterCases {
		t.Run("filter/"+tc.name, func(t *testing.T) {
			err := tc.f.Valid()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestTopologyValid covers the topology-level checks.
// The renderer relies on the (link, direction) uniqueness
// rule so two shapers cannot compete for the same
// interface root.
func TestTopologyValid(t *testing.T) {
	shaper1 := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 50_000, BurstKbyte: 256}
	shaper2 := Shaper{ID: "lan1", Link: "lan1", Direction: DirectionEgress, Algorithm: AlgorithmFQCoDel, RateKbps: 1_000_000, BurstKbyte: 1024}
	if err := (Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1, shaper2}}).Valid(); err != nil {
		t.Fatalf("expected base topology to validate, got %v", err)
	}
	bad := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1, {ID: "wan1-dup", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 10_000}}}
	if err := bad.Valid(); err == nil || !strings.Contains(err.Error(), "duplicate (link, direction)") {
		t.Fatalf("expected duplicate link/direction, got %v", err)
	}
	dupID := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1, {ID: "wan1", Link: "wan2", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 10_000}}}
	if err := dupID.Valid(); err == nil || !strings.Contains(err.Error(), "duplicate shaper id") {
		t.Fatalf("expected duplicate shaper id, got %v", err)
	}
	noShapers := Topology{SchemaVersion: SchemaVersion}
	if err := noShapers.Valid(); err == nil || !strings.Contains(err.Error(), "no shapers") {
		t.Fatalf("expected empty topology error, got %v", err)
	}
	wrongVersion := Topology{SchemaVersion: "0", Shapers: []Shaper{shaper1}}
	if err := wrongVersion.Valid(); err == nil || !strings.Contains(err.Error(), "schema version mismatch") {
		t.Fatalf("expected schema version error, got %v", err)
	}
	cl := Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7}
	orphan := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1}, Classes: []Class{{ID: "voice", ShaperID: "missing", Name: "X", MarkBits: 1}}}
	if err := orphan.Valid(); err == nil || !strings.Contains(err.Error(), "missing shaper") {
		t.Fatalf("expected orphan class error, got %v", err)
	}
	fl := Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7}
	ok := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1}, Classes: []Class{cl}, Filters: []Filter{fl}}
	if err := ok.Valid(); err != nil {
		t.Fatalf("expected full topology to validate, got %v", err)
	}
	filterOrphan := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1}, Classes: []Class{cl}, Filters: []Filter{{ID: "f1", ShaperID: "missing", ClassID: "voice", Mark: 7}}}
	if err := filterOrphan.Valid(); err == nil || !strings.Contains(err.Error(), "missing shaper") {
		t.Fatalf("expected orphan filter shaper error, got %v", err)
	}
	filterOrphanClass := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{shaper1}, Classes: []Class{cl}, Filters: []Filter{{ID: "f1", ShaperID: "wan1", ClassID: "missing", Mark: 7}}}
	if err := filterOrphanClass.Valid(); err == nil || !strings.Contains(err.Error(), "missing class") {
		t.Fatalf("expected orphan filter class error, got %v", err)
	}
}

// TestBuildSortsAndSeals verifies the Build helper
// produces a topology that is byte-stable across runs.
// The Engine relies on this so a serialised plan is
// cacheable.
func TestBuildSortsAndSeals(t *testing.T) {
	s2 := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 50_000, BurstKbyte: 256}
	s1 := Shaper{ID: "lan1", Link: "lan1", Direction: DirectionEgress, Algorithm: AlgorithmFQCoDel, RateKbps: 1_000_000, BurstKbyte: 1024}
	t1, err := Build([]Shaper{s2, s1}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if t1.GeneratedAt != 1 {
		t.Fatalf("expected GeneratedAt=1, got %d", t1.GeneratedAt)
	}
	if t1.Shapers[0].ID != "lan1" || t1.Shapers[1].ID != "wan1" {
		t.Fatalf("expected sorted shapers, got %+v", t1.Shapers)
	}
	if _, err := Build(nil, nil, nil, time.Now()); err == nil {
		t.Fatalf("expected build to fail with no shapers")
	}
	if _, err := Build([]Shaper{s1}, nil, nil, time.Now()); err != nil {
		t.Fatalf("expected single-shaper build to succeed: %v", err)
	}
}

// TestRenderDeterministic produces the same Plan bytes
// for the same input. The test is the regression for
// "Preview must not jitter".
func TestRenderDeterministic(t *testing.T) {
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 50_000, BurstKbyte: 256, Priority: PriorityVoice}
	cl := Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7}
	fl := Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7}
	tp, err := Build([]Shaper{sh}, []Class{cl}, []Filter{fl}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	p1, err := Render(tp)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	p2, err := Render(tp)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(p1.Steps) == 0 {
		t.Fatalf("expected non-empty plan")
	}
	d1, _ := p1.Encode()
	d2, _ := p2.Encode()
	if !bytes.Equal(d1, d2) {
		t.Fatalf("render must be deterministic")
	}
	// verify the textual output is sensible
	text := p1.ToText()
	for _, want := range []string{"# step", "tc qdisc add dev wan1", "tc class add dev wan1", "tc filter add dev wan1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected plan text to contain %q, got %q", want, text)
		}
	}
}

// TestRenderFailsOnInvalid confirms the renderer is a
// pure function of a Valid topology.
func TestRenderFailsOnInvalid(t *testing.T) {
	bad := Topology{SchemaVersion: "0", Shapers: []Shaper{{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1_000}}}
	if _, err := Render(bad); err == nil {
		t.Fatalf("expected render to fail on bad topology")
	}
}

// TestRenderFailsOnShellMetacharacter ensures the
// renderer never emits a tc argument with a space or
// shell metacharacter. The Linux adapter relies on
// this; a stray space would let a hostile id break out
// of the tc argument list.
func TestRenderFailsOnShellMetacharacter(t *testing.T) {
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1_000, Priority: "voice\nrm -rf /"}
	if err := sh.Valid(); err == nil {
		t.Fatalf("expected bad priority to fail validation")
	}
	// Force-build a topology with a bad priority to
	// confirm the renderer catches it via Valid even
	// when the priority slips through. The behaviour
	// must be: refuse.
	tp := Topology{SchemaVersion: SchemaVersion, Shapers: []Shaper{sh}}
	if _, err := Render(tp); err == nil {
		t.Fatalf("expected renderer to refuse bad priority")
	}
}

// TestEncodeDecodeRoundtrip makes sure the engine
// and agent can pass a topology over the wire without
// surprises.
func TestEncodeDecodeRoundtrip(t *testing.T) {
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 50_000, BurstKbyte: 256, Priority: PriorityVoice, Description: "primary"}
	cl := Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7}
	fl := Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7}
	tp, err := Build([]Shaper{sh}, []Class{cl}, []Filter{fl}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	data, err := EncodeTopology(tp)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	parsed, err := ParseTopology(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !equalTopology(tp, parsed) {
		t.Fatalf("roundtrip mismatch")
	}
	// unknown field must be rejected
	unknown := []byte(`{"schemaVersion":"1","shapers":[{"id":"x","link":"x","direction":"egress","algorithm":"cake","rateKbps":1,"burstKbyte":0,"bogus":1}]}`)
	if _, err := ParseTopology(unknown); err == nil {
		t.Fatalf("expected parse to fail on unknown field")
	}
	// truncated
	if _, err := ParseTopology(data[:10]); err == nil {
		t.Fatalf("expected parse to fail on truncated input")
	}
}

func equalTopology(a, b Topology) bool {
	da, _ := json.Marshal(a)
	db, _ := json.Marshal(b)
	return bytes.Equal(da, db)
}

// TestStateThreadsafe exercises the runtime State. The
// Agent reads Status under one lock and updates the
// topology under another; the test must not race.
func TestStateThreadsafe(t *testing.T) {
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1_000, BurstKbyte: 32}
	tp, err := Build([]Shaper{sh}, nil, nil, time.Now())
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	st := NewState(Topology{})
	if err := st.UpdateTopology(tp); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	st.MarkApplied(time.Unix(7, 0))
	st.MarkError("boom")
	got := st.Status()
	if got.LastApply != 7 {
		t.Fatalf("expected lastApply=7, got %d", got.LastApply)
	}
	if got.LastError != "boom" {
		t.Fatalf("expected lastError=boom, got %q", got.LastError)
	}
	if got.HasShapers != 1 {
		t.Fatalf("expected HasShapers=1, got %d", got.HasShapers)
	}
	// bad update is rejected and old topology preserved
	bad := Topology{SchemaVersion: "0"}
	if err := st.UpdateTopology(bad); err == nil {
		t.Fatalf("expected bad update to be rejected")
	}
	if !st.Topology().HasShapers() {
		t.Fatalf("expected previous topology to be preserved")
	}
	// long error gets truncated
	st.MarkError(strings.Repeat("x", 1024))
	if len(st.Status().LastError) > 256 {
		t.Fatalf("expected error to be truncated, got %d", len(st.Status().LastError))
	}
}

// TestSummarise checks the per-shaper summary the CLI
// prints. The summary must use the same id ordering the
// topology uses, so a CLI "selections" view and a
// detail view line up.
func TestSummarise(t *testing.T) {
	s1 := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1_000}
	s2 := Shaper{ID: "lan1", Link: "lan1", Direction: DirectionEgress, Algorithm: AlgorithmFQCoDel, RateKbps: 2_000}
	cl := Class{ID: "voice", ShaperID: "wan1", Name: "Voice", MarkBits: 7}
	fl := Filter{ID: "f1", ShaperID: "wan1", ClassID: "voice", Mark: 7}
	tp, err := Build([]Shaper{s1, s2}, []Class{cl}, []Filter{fl}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	sum := Summarise(tp)
	if len(sum) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(sum))
	}
	if sum[0].ID != "lan1" || sum[1].ID != "wan1" {
		t.Fatalf("expected sorted summaries, got %+v", sum)
	}
	if sum[1].Class != 1 || sum[1].Filter != 1 {
		t.Fatalf("expected class/filter counts to be aggregated, got %+v", sum[1])
	}
}

// TestIngressWarning confirms the renderer emits a
// warning for ingress shaping and that the warning
// propagates into the Plan and the Plan's Warnings
// list.
func TestIngressWarning(t *testing.T) {
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionIngress, Algorithm: AlgorithmFQCoDel, RateKbps: 1_000}
	tp, err := Build([]Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	p, err := Render(tp)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(p.Warnings) == 0 {
		t.Fatalf("expected ingress warning")
	}
	if !strings.Contains(p.Warnings[0], "ingress") {
		t.Fatalf("expected warning about ingress, got %q", p.Warnings[0])
	}
}

// TestPlanArgMetachars checks the Plan's argument
// safety net. A Step with a metacharacter in Args must
// be rejected at Valid time.
func TestPlanArgMetachars(t *testing.T) {
	good := Step{Kind: StepQdisc, Link: "wan1", Parent: "root", Handle: "root", Args: []string{"cake", "bandwidth=1000kbit"}}
	bad := Step{Kind: StepQdisc, Link: "wan1", Parent: "root", Handle: "root", Args: []string{"cake", "bandwidth=1000; rm -rf /"}}
	plan := Plan{SchemaVersion: SchemaVersion, Steps: []Step{good}}
	if err := plan.Valid(); err != nil {
		t.Fatalf("expected plan to validate, got %v", err)
	}
	plan.Steps = append(plan.Steps, bad)
	if err := plan.Valid(); err == nil || !strings.Contains(err.Error(), "shell metacharacter") {
		t.Fatalf("expected plan to reject metacharacter, got %v", err)
	}
	// encode refuses to write a plan that does not
	// validate, even when the bytes fit.
	if _, err := plan.Encode(); err == nil {
		t.Fatalf("expected encode to refuse bad plan")
	}
}

// TestRenderToTextIsBounded ensures the textual view
// does not bloat past the plan byte cap. A renderer that
// emits an unbounded comment would defeat the API byte
// limit.
func TestRenderToTextIsBounded(t *testing.T) {
	shapers := make([]Shaper, 0, 4)
	for i := 0; i < 4; i++ {
		shapers = append(shapers, Shaper{ID: "s" + string(rune('a'+i)), Link: "s" + string(rune('a'+i)), Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1000})
	}
	tp, err := Build(shapers, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	p, err := Render(tp)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	text := p.ToText()
	if len(text) > MaxPlanBytes {
		t.Fatalf("plan text %d exceeds cap %d", len(text), MaxPlanBytes)
	}
}

// TestBuildRespectsMax verifies the per-shaper and
// per-topology caps. The renderer is a pure function of
// the model; pushing the cap is rejected at Build.
func TestBuildRespectsMax(t *testing.T) {
	shapers := make([]Shaper, 0, MaxShapers+1)
	for i := 0; i < MaxShapers+1; i++ {
		shapers = append(shapers, Shaper{ID: "s" + string(rune('a'+(i%26))) + "_" + string(rune('a'+(i/26))), Link: "wan" + string(rune('a'+(i%26))), Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1000})
	}
	if _, err := Build(shapers, nil, nil, time.Now()); err == nil {
		t.Fatalf("expected build to fail over max shapers")
	}
}

// TestShaperFieldsImmutability is a small guardrail
// that catches accidental field renames in JSON. The
// wire format is part of the contract; the test pins
// the names.
func TestShaperFieldsImmutability(t *testing.T) {
	raw, err := json.Marshal(Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32, Priority: PriorityVoice, Description: "d"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"id":"wan1"`, `"link":"wan1"`, `"direction":"egress"`, `"algorithm":"cake"`, `"rateKbps":1000`, `"burstKbyte":32`, `"priority":"voice"`} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("expected %q in %s", want, raw)
		}
	}
}
