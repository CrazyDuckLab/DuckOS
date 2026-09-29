// localfirst/localfirst_test.go pins the
// V1 local-first contract.
package localfirst

import "testing"

func TestConnectivityValid(t *testing.T) {
	for _, c := range []Connectivity{ConnLocal, ConnOptional, ConnRequired} {
		if !c.Valid() {
			t.Fatalf("%s must be valid", c)
		}
	}
	if Connectivity("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestDeclarationValid(t *testing.T) {
	good := Declaration{Subsystem: SubRouting, Connectivity: ConnLocal, Reason: "kernel + nftables"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	noSub := Declaration{Connectivity: ConnLocal}
	if err := noSub.Valid(); err == nil {
		t.Fatal("missing subsystem must fail")
	}
	badConn := Declaration{Subsystem: SubRouting, Connectivity: "garbage"}
	if err := badConn.Valid(); err == nil {
		t.Fatal("unknown connectivity must fail")
	}
}

func TestManagerSetGetList(t *testing.T) {
	m := NewManager()
	if err := m.Set(Declaration{Subsystem: SubRouting, Connectivity: ConnLocal}); err != nil {
		t.Fatal(err)
	}
	if err := m.Set(Declaration{Subsystem: SubACME, Connectivity: ConnOptional}); err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Get(SubRouting); !ok || got.Connectivity != ConnLocal {
		t.Fatal("get failed")
	}
	if len(m.List()) != 2 {
		t.Fatal("list size")
	}
}

func TestManagerEvaluateLocal(t *testing.T) {
	m := NewManager()
	m.Set(Declaration{Subsystem: SubRouting, Connectivity: ConnLocal})
	m.Set(Declaration{Subsystem: SubFirewall, Connectivity: ConnLocal})
	rep := m.Evaluate()
	if !rep.LocalFirst {
		t.Fatalf("expected local-first, got %+v", rep)
	}
}

func TestManagerEvaluateWithRequired(t *testing.T) {
	m := NewManager()
	m.Set(Declaration{Subsystem: SubRouting, Connectivity: ConnLocal})
	m.Set(Declaration{Subsystem: SubSubscriptions, Connectivity: ConnRequired})
	m.Set(Declaration{Subsystem: SubACME, Connectivity: ConnOptional})
	rep := m.Evaluate()
	if rep.LocalFirst {
		t.Fatal("required dependency must break local-first")
	}
	if len(rep.Violations) != 1 || rep.Violations[0] != SubSubscriptions {
		t.Fatalf("violations: %v", rep.Violations)
	}
	if len(rep.OptionalDeps) != 1 || rep.OptionalDeps[0] != SubACME {
		t.Fatalf("optional: %v", rep.OptionalDeps)
	}
}
