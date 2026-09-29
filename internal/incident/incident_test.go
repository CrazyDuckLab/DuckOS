// incident/incident_test.go pins the V1
// 30-second incident view.
package incident

import (
	"testing"
	"time"
)

func TestSeverityValid(t *testing.T) {
	for _, s := range []Severity{SevOK, SevInfo, SevWarning, SevError, SevCritical} {
		if !s.Valid() {
			t.Fatalf("%s must be valid", s)
		}
	}
	if Severity("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
	if SevCritical.Order() <= SevOK.Order() {
		t.Fatal("order must be ascending")
	}
}

func TestBuilderBuild(t *testing.T) {
	now := time.Now()
	srcs := FuncSources{
		HealthFn: func() []HealthObject {
			return []HealthObject{
				{Kind: "interface", ID: "eth0", Severity: SevOK, Summary: "up"},
				{Kind: "tunnel", ID: "wg0", Severity: SevError, Summary: "stalled"},
			}
		},
		ChangesFn: func(since time.Time) []ChangeEvent {
			return []ChangeEvent{
				{At: now.Add(-time.Minute), Actor: "admin", Kind: "set", Object: "wg0", Summary: "changed"},
			}
		},
		TimelineFn: func(since time.Time) []TimelineEvent {
			return []TimelineEvent{
				{At: now, Source: "tunnel", Severity: SevError, Summary: "no handshake"},
			}
		},
		CheckpointsFn: func() []Checkpoint {
			return []Checkpoint{{ID: "cp-1", Label: "before wg0 change", CreatedAt: now.Add(-2 * time.Minute), Operator: "admin"}}
		},
	}
	b := NewBuilder(srcs, 64, func() time.Time { return now })
	v, err := b.Build(now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if v.WorstSeverity != SevError {
		t.Fatalf("worst: %s", v.WorstSeverity)
	}
	if len(v.Unhealthy) != 1 || v.Unhealthy[0].ID != "wg0" {
		t.Fatalf("unhealthy: %+v", v.Unhealthy)
	}
	if len(v.RecentChanges) != 1 {
		t.Fatalf("changes: %d", len(v.RecentChanges))
	}
	if len(v.Timeline) != 1 {
		t.Fatalf("timeline: %d", len(v.Timeline))
	}
	if len(v.Checkpoints) != 1 {
		t.Fatalf("checkpoints: %d", len(v.Checkpoints))
	}
	if v.RollbackHint == "" {
		t.Fatal("rollback hint must be set on error severity")
	}
}

func TestBuilderNilSources(t *testing.T) {
	b := NewBuilder(nil, 64, nil)
	if _, err := b.Build(time.Now()); err == nil {
		t.Fatal("nil sources must fail")
	}
}

func TestBuilderHistoryCap(t *testing.T) {
	now := time.Now()
	srcs := FuncSources{
		HealthFn: func() []HealthObject { return nil },
		ChangesFn: func(since time.Time) []ChangeEvent {
			out := []ChangeEvent{}
			for i := 0; i < 10; i++ {
				out = append(out, ChangeEvent{At: now.Add(-time.Duration(i) * time.Minute), Summary: "x"})
			}
			return out
		},
	}
	b := NewBuilder(srcs, 3, func() time.Time { return now })
	v, _ := b.Build(now.Add(-time.Hour))
	if len(v.RecentChanges) != 3 {
		t.Fatalf("history cap: %d", len(v.RecentChanges))
	}
}

func TestBuilderNoRollbackHintOnWarning(t *testing.T) {
	now := time.Now()
	srcs := FuncSources{
		HealthFn: func() []HealthObject {
			return []HealthObject{{Kind: "iface", ID: "eth1", Severity: SevWarning, Summary: "CRC rising"}}
		},
		CheckpointsFn: func() []Checkpoint { return []Checkpoint{{ID: "cp-1", Label: "x", CreatedAt: now}} },
	}
	b := NewBuilder(srcs, 64, func() time.Time { return now })
	v, _ := b.Build(now.Add(-time.Hour))
	if v.WorstSeverity != SevWarning {
		t.Fatalf("worst: %s", v.WorstSeverity)
	}
	if v.RollbackHint != "" {
		t.Fatal("warning must not set rollback hint")
	}
}
