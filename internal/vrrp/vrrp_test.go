// vrrp/vrrp_test.go pins the V1 VRRP model.
package vrrp

import "testing"

func TestGroupValid(t *testing.T) {
	good := Group{ID: "vrrp-1", Interface: "lan0", VirtualIP: "192.168.1.1", VRID: 50, Priority: 100, AdvertiseMS: 1000, Preempt: true, State: StateBackup}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	badVRID := Group{ID: "vrrp-1", Interface: "lan0", VirtualIP: "192.168.1.1", VRID: 0, Priority: 100, AdvertiseMS: 1000, State: StateBackup}
	if err := badVRID.Valid(); err == nil {
		t.Fatal("vrid=0 must fail")
	}
	badPriority := Group{ID: "vrrp-1", Interface: "lan0", VirtualIP: "192.168.1.1", VRID: 50, Priority: 0, AdvertiseMS: 1000, State: StateBackup}
	if err := badPriority.Valid(); err == nil {
		t.Fatal("priority=0 must fail")
	}
}

func TestElectionDecide(t *testing.T) {
	// Local priority strictly higher: master.
	e := Election{LocalPriority: 150, RemotePriorities: []int{100, 120}, Preempt: true}
	if got := e.Decide(); got != StateMaster {
		t.Fatalf("got %s, want master", got)
	}
	// No remote advertisement: master (we are
	// the only one).
	e2 := Election{LocalPriority: 100, RemotePriorities: nil, Preempt: true}
	if got := e2.Decide(); got != StateMaster {
		t.Fatalf("got %s, want master", got)
	}
	// Local priority equal to remote and
	// preemption: master.
	e3 := Election{LocalPriority: 100, RemotePriorities: []int{100}, Preempt: true}
	if got := e3.Decide(); got != StateMaster {
		t.Fatalf("got %s, want master with preemption", got)
	}
	// Local priority equal and no preemption:
	// backup.
	e4 := Election{LocalPriority: 100, RemotePriorities: []int{100}, Preempt: false}
	if got := e4.Decide(); got != StateBackup {
		t.Fatalf("got %s, want backup without preemption", got)
	}
	// Local priority lower: backup.
	e5 := Election{LocalPriority: 80, RemotePriorities: []int{120}, Preempt: true}
	if got := e5.Decide(); got != StateBackup {
		t.Fatalf("got %s, want backup", got)
	}
	// Pin priority 255: always master.
	e6 := Election{LocalPriority: 255, RemotePriorities: []int{200}, Preempt: false}
	if got := e6.Decide(); got != StateMaster {
		t.Fatalf("got %s, want master at priority 255", got)
	}
}

func TestManagerSetGetListDelete(t *testing.T) {
	m := NewManager()
	g := Group{ID: "vrrp-1", Interface: "lan0", VirtualIP: "192.168.1.1", VRID: 50, Priority: 100, AdvertiseMS: 1000, State: StateMaster}
	if err := m.Set(g); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("vrrp-1"); !ok {
		t.Fatal("Get must find the group")
	}
	m.Delete("vrrp-1")
	if _, ok := m.Get("vrrp-1"); ok {
		t.Fatal("Get after Delete must miss")
	}
}

func TestGroupTransition(t *testing.T) {
	g := Group{ID: "vrrp-1", Interface: "lan0", VirtualIP: "192.168.1.1", VRID: 50, Priority: 100, AdvertiseMS: 1000, State: StateBackup}
	g2, err := g.Transition(StateMaster, 1234)
	if err != nil {
		t.Fatal(err)
	}
	if g2.State != StateMaster || g2.LastTransition != 1234 {
		t.Fatalf("Transition: %+v", g2)
	}
	if _, err := g.Transition("garbage", 1); err == nil {
		t.Fatal("unknown state must fail")
	}
}
