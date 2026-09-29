package canary

import "testing"

func TestCanary(t *testing.T) {
	s := NewCanaryStore()
	s.Add(Canary{ID: "c1", Scope: ScopeVRF, Target: "vrf1"})
	s.Promote("c1", "ok")
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
	if err := s.Add(Canary{ID: "!!bad", Scope: ScopeVRF, Target: "v"}); err == nil {
		t.Fatal("bad id must fail")
	}
}

func TestPlanTopo(t *testing.T) {
	in := TxnPlan{ID: "p1", Steps: []TxnStep{
		{Name: "interface", Kind: "interface"},
		{Name: "address", Kind: "address", DependsOn: []string{"interface"}},
		{Name: "route", Kind: "route", DependsOn: []string{"address"}},
		{Name: "firewall", Kind: "firewall", DependsOn: []string{"interface"}},
	}}
	out, err := PlanTopo(in)
	if err != nil {
		t.Fatal(err)
	}
	if out.Steps[0].Name != "interface" {
		t.Fatal("interface must be first")
	}
	cyc := TxnPlan{ID: "p2", Steps: []TxnStep{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}}
	if _, err := PlanTopo(cyc); err == nil {
		t.Fatal("cycle must fail")
	}
}

func TestIncident(t *testing.T) {
	s := NewIncidentStore()
	s.Open(Incident{ID: "i1"})
	s.Correlate("i1", "wan1 down", "wan1")
	s.Correlate("i1", "dns fail", "dns")
	s.Close("i1")
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestPathCapture(t *testing.T) {
	c := NewPathCapture()
	c.CaptureBefore(PathSnapshot{Kind: "route", ObjectID: "default", Decision: "wan1"})
	c.CaptureAfter(PathSnapshot{Kind: "route", ObjectID: "default", Decision: "wan2"})
	d := c.Diff()
	if len(d.Changed) != 1 {
		t.Fatalf("changed: %v", d.Changed)
	}
}

func TestStaleDetector(t *testing.T) {
	d := NewStaleDetector()
	d.Add(StaleObject{Kind: "firewall", ID: "r1", Reason: "unused"})
	if len(d.All()) != 1 {
		t.Fatal("all")
	}
}

func TestActionCatalog(t *testing.T) {
	c := NewActionCatalog()
	c.Register(DestructiveAction{Name: "flush-conntrack", Scope: "conntrack", RequiresConfirm: true, ManagementTouched: false})
	if _, ok := c.Get("flush-conntrack"); !ok {
		t.Fatal("get")
	}
}

func TestRetention(t *testing.T) {
	r := NewRetentionStore()
	r.SetTiers([]RetentionTier{{Name: "raw", Resolution: "1s", MaxAgeDays: 1, MaxBytes: 1 << 30}, {Name: "min", Resolution: "1m", MaxAgeDays: 30, MaxBytes: 10 << 30}})
	if len(r.Tiers()) != 2 {
		t.Fatal("tiers")
	}
}

func TestDecommission(t *testing.T) {
	s := NewDecommissionStore()
	s.Add(Decommission{ID: "d1", Kind: "wan", ObjectID: "wan1", Dependencies: []string{"fw:r1"}})
	s.Advance("d1", "migrate")
	s.Advance("d1", "disable")
	s.Advance("d1", "remove")
	s.Advance("d1", "cleanup")
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestLab(t *testing.T) {
	s := NewLabStore()
	if err := s.Add(Lab{ID: "lab1", Name: "test", ProductionSafe: false, Networks: []string{"eth0"}}); err == nil {
		t.Fatal("lab touching eth0 must fail")
	}
	s.Add(Lab{ID: "lab1", Name: "test", Networks: []string{"veth-test"}})
	if len(s.All()) != 1 {
		t.Fatal("all")
	}
}

func TestAccRunner(t *testing.T) {
	r := NewAccRunner()
	r.Record(ScenarioRun{ID: "1", ScenarioID: "1.first-boot", Result: ResultPass})
	r.Record(ScenarioRun{ID: "2", ScenarioID: "26.storage-full", Result: ResultFail})
	counts := r.Counts()
	if counts[ResultPass] != 1 || counts[ResultFail] != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	if err := r.Record(ScenarioRun{ID: "3", ScenarioID: "x", Result: "garbage"}); err == nil {
		t.Fatal("bad result must fail")
	}
}
