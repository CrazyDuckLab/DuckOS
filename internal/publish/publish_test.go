// publish/publish_test.go pins the V1 Publish
// Service intent model.
package publish

import "testing"

func TestIntentValid(t *testing.T) {
	good := Intent{ID: "blog", Service: "blog", WANID: "wan1", ExternalPort: 443, InternalHost: "192.168.1.10", InternalPort: 8080, Protocol: ProtoTCP, CreatedBy: "op-1"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Intent{ID: "blog", Service: "blog", WANID: "wan1", ExternalPort: 70000, InternalHost: "192.168.1.10", InternalPort: 8080, Protocol: ProtoTCP}
	if err := bad.Valid(); err == nil {
		t.Fatal("externalPort out of range must fail")
	}
	noWAN := Intent{ID: "blog", Service: "blog", ExternalPort: 443, InternalHost: "192.168.1.10", InternalPort: 8080, Protocol: ProtoTCP}
	if err := noWAN.Valid(); err == nil {
		t.Fatal("wanId required must fail")
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusActive, StatusTested, StatusExpired, StatusDisabled, StatusFailed, "garbage"} {
		want := s != "garbage"
		if got := s.Valid(); got != want {
			t.Errorf("%s.Valid=%v want %v", s, got, want)
		}
	}
}

func TestComputePlanFlagsHighRiskPorts(t *testing.T) {
	ssh := Intent{ID: "ssh", Service: "ssh", WANID: "wan1", ExternalPort: 22, InternalHost: "10.0.0.1", InternalPort: 22, Protocol: ProtoTCP}
	if p := ComputePlan(ssh); p.ManagementRisk != "high" {
		t.Fatalf("ssh risk: %q", p.ManagementRisk)
	}
	blog := Intent{ID: "blog", Service: "blog", WANID: "wan1", ExternalPort: 443, InternalHost: "10.0.0.1", InternalPort: 8080, Protocol: ProtoTCP}
	if p := ComputePlan(blog); p.ManagementRisk != "low" {
		t.Fatalf("blog risk: %q", p.ManagementRisk)
	}
	if p := ComputePlan(blog); p.ConntrackImpact != "new-only" {
		t.Fatalf("ConntrackImpact: %q", p.ConntrackImpact)
	}
}

func TestManagerCRUD(t *testing.T) {
	m := NewManager()
	i := Intent{ID: "blog", Service: "blog", WANID: "wan1", ExternalPort: 443, InternalHost: "10.0.0.1", InternalPort: 8080, Protocol: ProtoTCP, CreatedBy: "op-1"}
	p := Published{Intent: i, Plan: ComputePlan(i), Status: StatusActive}
	if err := m.Set(p); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("blog"); !ok {
		t.Fatal("Get must find")
	}
	if len(m.List()) != 1 {
		t.Fatal("List count")
	}
	m.Delete("blog")
	if _, ok := m.Get("blog"); ok {
		t.Fatal("Get after Delete must miss")
	}
}
