// lifecycle/dependencies_test.go pins the V1
// dependency-aware deletion preflight.
package lifecycle

import "testing"

func TestObjectValid(t *testing.T) {
	for _, tc := range []struct {
		o    Object
		want bool
	}{
		{Object{Kind: KindInterface, ID: "eth0"}, true},
		{Object{Kind: KindCertificate, ID: "cert-1"}, true},
		{Object{Kind: KindXrayServer, ID: "vless-1"}, true},
		{Object{Kind: KindXrayUser, ID: "user-1"}, true},
		{Object{}, false},
		{Object{Kind: KindInterface, ID: ""}, false},
		{Object{Kind: KindInterface, ID: "../etc/passwd"}, false},
		{Object{Kind: "garbage", ID: "x"}, false},
	} {
		if got := tc.o.Valid(); got != tc.want {
			t.Errorf("Object(%+v).Valid() = %v, want %v", tc.o, got, tc.want)
		}
	}
}

func TestGraphAddRejectsSelfReference(t *testing.T) {
	g := NewGraph()
	ref := Reference{Dependent: Object{Kind: KindInterface, ID: "eth0"}, Target: Object{Kind: KindInterface, ID: "eth0"}}
	if err := g.Add(ref); err != ErrSelfReference {
		t.Fatalf("Add self-ref: %v, want ErrSelfReference", err)
	}
}

func TestGraphAddRejectsInvalid(t *testing.T) {
	g := NewGraph()
	if err := g.Add(Reference{Dependent: Object{Kind: KindInterface, ID: "eth0"}, Target: Object{Kind: "garbage", ID: "x"}}); err != ErrInvalidObject {
		t.Fatalf("Add invalid target: %v", err)
	}
}

func TestGraphDependents(t *testing.T) {
	g := NewGraph()
	cert := Object{Kind: KindCertificate, ID: "cert-1"}
	site := Object{Kind: KindSite, ID: "site-blog"}
	sub := Object{Kind: KindSubscription, ID: "sub-blog"}
	if err := g.Add(Reference{Dependent: site, Target: cert, Reason: "binds 443/tls"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Reference{Dependent: sub, Target: cert, Reason: "publishes for domain"}); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(Reference{Dependent: sub, Target: site, Reason: "exposes site"}); err != nil {
		t.Fatal(err)
	}
	deps := g.Dependents(cert)
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependents of cert, got %d: %+v", len(deps), deps)
	}
	// Order is (Kind, ID) ascending; "site" < "subscription".
	if deps[0].Dependent.Kind != KindSite {
		t.Fatalf("deps[0] kind = %s, want %s", deps[0].Dependent.Kind, KindSite)
	}
	if deps[1].Dependent.Kind != KindSubscription {
		t.Fatalf("deps[1] kind = %s, want %s", deps[1].Dependent.Kind, KindSubscription)
	}
}

func TestGraphTargets(t *testing.T) {
	g := NewGraph()
	iface := Object{Kind: KindInterface, ID: "wan1"}
	site := Object{Kind: KindSite, ID: "site-blog"}
	if err := g.Add(Reference{Dependent: site, Target: iface, Reason: "uplink"}); err != nil {
		t.Fatal(err)
	}
	tgts := g.Targets(site)
	if len(tgts) != 1 || tgts[0].Target != iface {
		t.Fatalf("Targets: %+v", tgts)
	}
}

func TestGraphPreflightCascadeBlocker(t *testing.T) {
	g := NewGraph()
	wan := Object{Kind: KindWAN, ID: "wan-1"}
	tun := Object{Kind: KindTunnel, ID: "wg-1"}
	if err := g.Add(Reference{Dependent: tun, Target: wan, Reason: "uplink"}); err != nil {
		t.Fatal(err)
	}
	plan, err := g.Preflight(wan)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasDependents {
		t.Fatal("Preflight must report dependents")
	}
	if len(plan.Dependents) != 1 {
		t.Fatalf("plan has %d dependents, want 1", len(plan.Dependents))
	}
}

func TestGraphPreflightRefusesInvalid(t *testing.T) {
	g := NewGraph()
	if _, err := g.Preflight(Object{}); err != ErrInvalidObject {
		t.Fatalf("Preflight: %v, want ErrInvalidObject", err)
	}
}

func TestGraphRemove(t *testing.T) {
	g := NewGraph()
	wan := Object{Kind: KindWAN, ID: "wan-1"}
	tun := Object{Kind: KindTunnel, ID: "wg-1"}
	ref := Reference{Dependent: tun, Target: wan, Reason: "uplink"}
	if err := g.Add(ref); err != nil {
		t.Fatal(err)
	}
	g.Remove(ref)
	if deps := g.Dependents(wan); len(deps) != 0 {
		t.Fatalf("after Remove, expected 0 dependents, got %+v", deps)
	}
}
