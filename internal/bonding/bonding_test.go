// bonding/bonding_test.go pins the V1 link
// aggregation contract.
package bonding

import "testing"

func TestBondValid(t *testing.T) {
	good := Bond{ID: "bond-1", Name: "bond0", Mode: ModeActiveBackup, MIIMonitor: 100, Members: []Member{{Link: "eth0", Priority: 100}, {Link: "eth1", Priority: 50}}, ActiveMember: "eth0"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	lacp := Bond{ID: "bond-1", Name: "bond0", Mode: ModeLACP, HashMode: HashLayer34, MIIMonitor: 100, Members: []Member{{Link: "eth0", LacpPort: 1}, {Link: "eth1", LacpPort: 2}}}
	if err := lacp.Valid(); err != nil {
		t.Fatal(err)
	}
	badMode := Bond{ID: "bond-1", Name: "bond0", Mode: "garbage", MIIMonitor: 100, Members: []Member{{Link: "eth0"}, {Link: "eth1"}}}
	if err := badMode.Valid(); err == nil {
		t.Fatal("unknown mode must fail")
	}
	lacpNoHash := Bond{ID: "bond-1", Name: "bond0", Mode: ModeLACP, MIIMonitor: 100, Members: []Member{{Link: "eth0"}, {Link: "eth1"}}}
	if err := lacpNoHash.Valid(); err == nil {
		t.Fatal("LACP without hash mode must fail")
	}
	oneMember := Bond{ID: "bond-1", Name: "bond0", Mode: ModeActiveBackup, MIIMonitor: 100, Members: []Member{{Link: "eth0"}}}
	if err := oneMember.Valid(); err == nil {
		t.Fatal("a bond needs at least two members")
	}
}

func TestManagerSetGetListDelete(t *testing.T) {
	m := NewManager()
	b := Bond{ID: "bond-1", Name: "bond0", Mode: ModeActiveBackup, MIIMonitor: 100, Members: []Member{{Link: "eth0"}, {Link: "eth1"}}}
	if err := m.Set(b); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("bond-1"); !ok {
		t.Fatal("Get must find the bond")
	}
	if len(m.List()) != 1 {
		t.Fatal("List must contain the bond")
	}
	m.Delete("bond-1")
	if _, ok := m.Get("bond-1"); ok {
		t.Fatal("Get after Delete must miss")
	}
}
