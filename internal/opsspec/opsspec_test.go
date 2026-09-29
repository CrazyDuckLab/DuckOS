// G22 specialised operations catalogue torture regression.
package opsspec

import (
	"testing"
)

func TestKind_Valid(t *testing.T) {
	for _, k := range []Kind{KindDNS, KindDHCP, KindAAA, KindPortal, KindFlow, KindHardware} {
		if !k.Valid() {
			t.Errorf("%q must be valid", k)
		}
	}
	if Kind("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestRisk_Valid(t *testing.T) {
	for _, r := range []Risk{RiskLow, RiskMedium, RiskHigh} {
		if !r.Valid() {
			t.Errorf("%q must be valid", r)
		}
	}
	if Risk("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestSpec_Validate(t *testing.T) {
	s := Spec{Kind: KindDNS, Action: DNSReloadZone, Risk: RiskLow}
	if err := s.Validate(); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	// Bad kind
	bad := Spec{Kind: Kind("garbage"), Action: DNSReloadZone, Risk: RiskLow}
	if err := bad.Validate(); err == nil {
		t.Fatalf("bad kind must be rejected")
	}
	// Bad action
	bad2 := Spec{Kind: KindDNS, Action: Action("X"), Risk: RiskLow}
	if err := bad2.Validate(); err == nil {
		t.Fatalf("bad action must be rejected")
	}
	// Bad risk
	bad3 := Spec{Kind: KindDNS, Action: DNSReloadZone, Risk: Risk("garbage")}
	if err := bad3.Validate(); err == nil {
		t.Fatalf("bad risk must be rejected")
	}
}

func TestSpec_Key(t *testing.T) {
	s := Spec{Kind: KindDNS, Action: DNSReloadZone}
	if s.Key() != "dns:dns.reload-zone" {
		t.Fatalf("Key drift: %q", s.Key())
	}
}

func TestCatalogue_DefaultIsPopulated(t *testing.T) {
	c := NewCatalogue()
	if c.Counters().Total < 13 {
		t.Fatalf("expected at least 13 default specs, got %d", c.Counters().Total)
	}
}

func TestCatalogue_Get(t *testing.T) {
	c := NewCatalogue()
	spec, ok := c.Get(KindDNS, DNSReloadZone)
	if !ok || spec.Risk != RiskLow {
		t.Fatalf("drift: %+v ok=%v", spec, ok)
	}
}

func TestCatalogue_AddReplaces(t *testing.T) {
	c := NewCatalogue()
	if err := c.Add(Spec{Kind: KindDNS, Action: DNSReloadZone, Risk: RiskHigh}); err != nil {
		t.Fatal(err)
	}
	spec, _ := c.Get(KindDNS, DNSReloadZone)
	if spec.Risk != RiskHigh {
		t.Fatalf("Add must replace: %+v", spec)
	}
}

func TestCatalogue_AddRejectsInvalid(t *testing.T) {
	c := NewCatalogue()
	if err := c.Add(Spec{Kind: Kind("garbage"), Action: DNSReloadZone}); err == nil {
		t.Fatalf("invalid kind must be rejected")
	}
}

func TestCatalogue_Remove(t *testing.T) {
	c := NewCatalogue()
	c.Remove(KindDNS, DNSReloadZone)
	if _, ok := c.Get(KindDNS, DNSReloadZone); ok {
		t.Fatalf("Remove must delete")
	}
	c.Remove(KindDNS, DNSReloadZone) // idempotent
}

func TestCatalogue_ByKindSorted(t *testing.T) {
	c := NewCatalogue()
	dns := c.ByKind(KindDNS)
	if len(dns) < 3 {
		t.Fatalf("expected >= 3 dns specs, got %d", len(dns))
	}
	for i := 1; i < len(dns); i++ {
		if dns[i-1].Action > dns[i].Action {
			t.Fatalf("not sorted at %d", i)
		}
	}
}

func TestCatalogue_AllSorted(t *testing.T) {
	c := NewCatalogue()
	all := c.All()
	for i := 1; i < len(all); i++ {
		if all[i-1].Kind == all[i].Kind && all[i-1].Action > all[i].Action {
			t.Fatalf("action order broken at %d", i)
		}
		if all[i-1].Kind > all[i].Kind {
			t.Fatalf("kind order broken at %d", i)
		}
	}
}

func TestCatalogue_Counters(t *testing.T) {
	c := NewCatalogue()
	cn := c.Counters()
	if cn.Total == 0 {
		t.Fatalf("expected non-zero counters")
	}
	if cn.ByKind[KindDNS] == 0 {
		t.Fatalf("expected DNS specs")
	}
	if cn.ByRisk[RiskHigh] == 0 {
		t.Fatalf("expected high-risk specs")
	}
}
