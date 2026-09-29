// explain/expand_test.go pins the V1
// Explain Why expansion.
package explain

import "testing"

func TestClassifyRouteFilterAccept(t *testing.T) {
	rules := []FilterRule{
		{Name: "r1", Action: "modify", Reason: "set metric"},
		{Name: "r2", Action: "accept"},
		{Name: "r3", Action: "accept"},
	}
	out := ClassifyRouteFilter("10.0.0.0/8", rules)
	if len(out) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(out))
	}
	if out[0].Action != "modify" || out[1].Action != "accept" {
		t.Fatalf("actions: %+v", out)
	}
}

func TestClassifyRouteFilterReject(t *testing.T) {
	rules := []FilterRule{{Name: "r1", Action: "reject", Reason: "policy"}}
	out := ClassifyRouteFilter("10.0.0.0/8", rules)
	if len(out) != 1 || out[0].Action != "reject" {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestDetectRouteLoop(t *testing.T) {
	if l := DetectRouteLoop([]string{"a", "b", "c"}); l != nil {
		t.Fatalf("no loop: %+v", l)
	}
	l := DetectRouteLoop([]string{"a", "b", "a"})
	if l == nil {
		t.Fatal("expected loop")
	}
	if l.Level != "l3" {
		t.Fatalf("level: %s", l.Level)
	}
}

func TestDetectL2Loop(t *testing.T) {
	if l := DetectL2Loop([]string{"a", "b", "c"}); l != nil {
		t.Fatalf("no loop: %+v", l)
	}
	l := DetectL2Loop([]string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02", "aa:bb:cc:dd:ee:01"})
	if l == nil {
		t.Fatal("expected l2 loop")
	}
}

func TestBuilderExpand(t *testing.T) {
	b := NewExplainBuilder(Endpoint{Address: "10.0.0.5"}, Endpoint{Address: "1.1.1.1"}, nil)
	b.AddVRF(VRFHop{VRF: "mgmt", Table: 100, Decision: DecisionAllow})
	b.AddBGP(BGPSessionHop{Neighbor: "10.0.0.1", State: "established", FilterHit: "r1"})
	b.AddRouteFilter(RouteFilterHop{Prefix: "10.0.0.0/8", Filter: "r1", Action: "accept"})
	b.AddHop(Hop{Kind: HopRoute, Decision: DecisionAllow, Label: "default"})
	b.AddLoop(LoopHop{Level: "l3", Detail: "recursive"})
	out := b.Build()
	if out.Verdict != DecisionLoop {
		t.Fatalf("verdict: %s", out.Verdict)
	}
	if len(out.Expand.VRF) != 1 || out.Expand.VRF[0].VRF != "mgmt" {
		t.Fatalf("vrf: %+v", out.Expand.VRF)
	}
	if len(out.Expand.BGP) != 1 || out.Expand.BGP[0].Neighbor != "10.0.0.1" {
		t.Fatalf("bgp: %+v", out.Expand.BGP)
	}
	if len(out.Expand.Loop) != 1 {
		t.Fatalf("loop not preserved: %+v", out.Expand.Loop)
	}
}

func TestBuilderHopVerdict(t *testing.T) {
	b := NewExplainBuilder(Endpoint{Address: "10.0.0.5"}, Endpoint{Address: "1.1.1.1"}, nil)
	b.AddHop(Hop{Kind: HopRoute, Decision: DecisionAllow})
	if b.chain.Verdict != DecisionAllow {
		t.Fatalf("verdict: %s", b.chain.Verdict)
	}
	b.AddHop(Hop{Kind: HopFirewall, Decision: DecisionDeny})
	if b.chain.Verdict != DecisionStop {
		t.Fatalf("verdict after deny: %s", b.chain.Verdict)
	}
}

func TestValidServiceEndpoint(t *testing.T) {
	good := ServiceEndpoint{Endpoint: Endpoint{Address: "10.0.0.5", Port: 443}, VRF: "mgmt"}
	if err := ValidServiceEndpoint(good); err != nil {
		t.Fatal(err)
	}
	bad := ServiceEndpoint{Endpoint: Endpoint{Address: "", Port: 70000}}
	if err := ValidServiceEndpoint(bad); err == nil {
		t.Fatal("port out of range must fail")
	}
	bad2 := ServiceEndpoint{Endpoint: Endpoint{Address: "10.0.0.5"}, VRF: "!bad"}
	if err := ValidServiceEndpoint(bad2); err == nil {
		t.Fatal("non-canonical vrf must fail")
	}
}
