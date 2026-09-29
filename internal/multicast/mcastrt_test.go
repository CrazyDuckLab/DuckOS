package multicast

import "testing"

func TestProtocolValid(t *testing.T) {
	for _, p := range []Protocol{ProtoPIMSM, ProtoSSM, ProtoIGMP} {
		if !p.Valid() {
			t.Fatalf("%s must be valid", p)
		}
	}
}

func TestManager(t *testing.T) {
	m := NewManager()
	m.SetProtocol(ProtoPIMSM)
	m.AddRP(RP{Address: "10.0.0.1", Group: "239.0.0.0/8"})
	m.SetIGMPSnoop(true)
	m.SetState(State{Entries: []Entry{{Group: "239.0.0.1", Source: "10.0.0.5", Incoming: "eth0", RPF: "10.0.0.1"}}})
	rpf, err := m.RPF("239.0.0.1", "10.0.0.5")
	if err != nil || rpf != "10.0.0.1" {
		t.Fatalf("rpf: %s err: %v", rpf, err)
	}
	if _, err := m.RPF("239.0.0.99", "10.0.0.5"); err == nil {
		t.Fatal("missing rpf must fail")
	}
}

func TestL7Rule(t *testing.T) {
	good := L7Rule{ID: "r1", Match: "*.zoom.us", Class: "video-conf", Source: "sni", Confidence: 80, Limitations: []string{"encrypted"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noMatch := L7Rule{ID: "r1", Class: "x"}
	if err := noMatch.Valid(); err == nil {
		t.Fatal("missing match must fail")
	}
	m := NewL7Manager()
	m.Add(good)
	if len(m.List()) != 1 {
		t.Fatal("list size")
	}
}
