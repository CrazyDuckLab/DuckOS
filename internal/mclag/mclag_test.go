package mclag

import "testing"

func TestDomainValid(t *testing.T) {
	good := Domain{ID: "d1", Name: "core", PeerAddress: "10.0.0.2", Role: RolePrimary, Members: []string{"eth0"}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noPeer := Domain{ID: "d1", Role: RolePrimary}
	if err := noPeer.Valid(); err == nil {
		t.Fatal("missing peer must fail")
	}
	badRole := Domain{ID: "d1", PeerAddress: "x", Role: "garbage"}
	if err := badRole.Valid(); err == nil {
		t.Fatal("bad role must fail")
	}
}

func TestManagerPeerLink(t *testing.T) {
	m := NewManager()
	if err := m.Set(Domain{ID: "d1", PeerAddress: "10.0.0.2", Role: RolePrimary, Members: []string{"eth0", "eth1"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPeerLink("d1", "peer0", "up"); err != nil {
		t.Fatal(err)
	}
	d, _ := m.Get("d1")
	if d.PeerLink.State != "up" || d.SplitBrainRisk {
		t.Fatal("up state wrong")
	}
	if err := m.SetPeerLink("d1", "peer0", "down"); err != nil {
		t.Fatal(err)
	}
	d, _ = m.Get("d1")
	if d.PeerLink.State != "down" || !d.SplitBrainRisk {
		t.Fatal("down state wrong")
	}
	if len(d.OrphanPorts) != 2 {
		t.Fatal("orphan ports")
	}
}
