package ipv6trans

import "testing"

func TestTransitionKindValid(t *testing.T) {
	for _, k := range []TransitionKind{Trans464XLAT, TransDSLite, Trans6rd, TransMAPE, TransMAPT, TransNPTv6} {
		if !k.Valid() {
			t.Fatalf("%s must be valid", k)
		}
	}
}

func TestCapabilitySupports(t *testing.T) {
	c := Capability{Transitions: []TransitionKind{Trans464XLAT, TransDSLite}}
	if !c.Supports(Trans464XLAT) {
		t.Fatal("must support 464xlat")
	}
	if c.Supports(TransMAPE) {
		t.Fatal("must not support MAPE")
	}
}

func TestCGNATPoolValid(t *testing.T) {
	good := CGNATPool{Name: "p1", Addresses: []string{"203.0.113.1", "203.0.113.2"}, PortBlockSize: 64}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noName := CGNATPool{Addresses: []string{"x"}, PortBlockSize: 64}
	if err := noName.Valid(); err == nil {
		t.Fatal("missing name must fail")
	}
}

func TestCGNATManager(t *testing.T) {
	m := NewCGNATManager()
	m.AddPool(CGNATPool{Name: "p1", Addresses: []string{"203.0.113.1"}, PortBlockSize: 64})
	m.SetExhaustion(ExhaustionState{FreeAddresses: 1, FreePortBlocks: 100, Subscribers: 50})
	if p, ok := m.GetPool("p1"); !ok || p.Name != "p1" {
		t.Fatal("get pool failed")
	}
	if m.Exhaustion().Subscribers != 50 {
		t.Fatal("exhaustion not set")
	}
}

func TestTransitionManager(t *testing.T) {
	m := NewTransitionManager()
	m.SetCapability(Capability{Transitions: []TransitionKind{Trans6rd}})
	m.AddPrefix(Prefix{Kind: Trans6rd, Prefix: "2001:db8::/32", BR: "10.0.0.1"})
	if len(m.Prefixes()) != 1 {
		t.Fatal("prefixes")
	}
}
