package oam

import "testing"

func TestCFM(t *testing.T) {
	m := NewManager(nil)
	m.SetMD(CFMMD{Name: "core", Level: 3})
	m.SetMA(CFMMA{Name: "ma1", MDName: "core", CCMInterval: 1000000000})
	m.RecordCFM(CFMState{MEP: "mep1", MDName: "core", MAName: "ma1", State: "up"})
	if len(m.CFMAll()) != 1 {
		t.Fatal("cfm")
	}
}

func TestRing(t *testing.T) {
	m := NewRingManager(nil)
	m.Set(RingState{RingID: "r1", State: "protected", RPLOwner: true, RPLPort: "eth0", BlockedPort: "eth1"})
	if r, ok := m.Get("r1"); !ok || r.State != "protected" {
		t.Fatal("ring")
	}
	if err := m.Set(RingState{RingID: "!!bad", State: "garbage"}); err == nil {
		t.Fatal("bad ring must fail")
	}
}
