// observability/observability_test.go pins
// the V1 Doctor surface.
package observability

import "testing"

func TestSeverityValid(t *testing.T) {
	for _, s := range []Severity{SeverityOK, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical} {
		if !s.Valid() {
			t.Fatalf("%s must be valid", s)
		}
	}
	if Severity("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
	if SeverityOK.Order() >= SeverityWarning.Order() {
		t.Fatal("order must be ascending")
	}
}

func TestDoctorAddAndSort(t *testing.T) {
	d := NewDoctor()
	d.Add(Object{Kind: "interface", ID: "eth0", Severity: SeverityOK, Summary: "up"})
	d.Add(Object{Kind: "tunnel", ID: "wg0", Severity: SeverityError, Summary: "handshake stalled"})
	d.Add(Object{Kind: "interface", ID: "eth1", Severity: SeverityWarning, Summary: "CRC rising"})
	d.Add(Object{Kind: "tunnel", ID: "ipsec0", Severity: SeverityCritical, Summary: "rekey failed"})
	snap := d.Snapshot()
	if snap.WorstSeverity != SeverityCritical {
		t.Fatalf("worst: %s", snap.WorstSeverity)
	}
	if snap.Counts[SeverityOK] != 1 || snap.Counts[SeverityError] != 1 || snap.Counts[SeverityWarning] != 1 || snap.Counts[SeverityCritical] != 1 {
		t.Fatalf("counts: %+v", snap.Counts)
	}
	if snap.Objects[0].ID != "ipsec0" {
		t.Fatalf("first must be critical, got %+v", snap.Objects[0])
	}
}

func TestRegistryCollect(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(FuncSource{KindValue: "interface", Fn: func() []Object {
		return []Object{{ID: "eth0", Severity: SeverityOK, Summary: "up"}}
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(FuncSource{KindValue: "tunnel", Fn: func() []Object {
		return []Object{{ID: "wg0", Severity: SeverityWarning, Summary: "slow"}}
	}}); err != nil {
		t.Fatal(err)
	}
	// duplicate kind replaces older source
	if err := r.Register(FuncSource{KindValue: "interface", Fn: func() []Object {
		return []Object{{ID: "eth1", Severity: SeverityError, Summary: "down"}}
	}}); err != nil {
		t.Fatal(err)
	}
	snap := r.Collect()
	if snap.Counts[SeverityError] != 1 {
		t.Fatalf("replace did not happen: %+v", snap)
	}
}

func TestRegistryInvalid(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(FuncSource{KindValue: ""}); err == nil {
		t.Fatal("empty kind must fail")
	}
	if err := r.Register(FuncSource{KindValue: "x", Fn: nil}); err == nil {
		t.Fatal("nil source must fail")
	}
}

func TestDoctorEmpty(t *testing.T) {
	d := NewDoctor()
	snap := d.Snapshot()
	if snap.WorstSeverity != EmptySeverity {
		t.Fatalf("empty worst: %s", snap.WorstSeverity)
	}
}
