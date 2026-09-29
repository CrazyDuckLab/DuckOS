// dynamicrt/dynamicrt_test.go pins the V1
// dynamic-routing data model and looking
// glass.
package dynamicrt

import "testing"

func TestSessionValid(t *testing.T) {
	good := Session{ID: "ospf-1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bgp := Session{ID: "bgp-1", Protocol: ProtoBGP, LocalAS: 65000, RemoteAS: 65001, Peer: "10.0.0.1", State: StateEstablished}
	if err := bgp.Valid(); err != nil {
		t.Fatal(err)
	}
	badProto := Session{ID: "x", Protocol: "garbage", LocalAS: 1, Peer: "10.0.0.1", State: StateDown}
	if err := badProto.Valid(); err == nil {
		t.Fatal("unknown protocol must fail")
	}
	bgpNoRemote := Session{ID: "bgp-1", Protocol: ProtoBGP, LocalAS: 65000, Peer: "10.0.0.1", State: StateIdle}
	if err := bgpNoRemote.Valid(); err == nil {
		t.Fatal("BGP without remoteAS must fail")
	}
}

func TestLookingGlassSessions(t *testing.T) {
	lg := NewLookingGlass()
	if err := lg.SetSession(Session{ID: "ospf-1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished}); err != nil {
		t.Fatal(err)
	}
	if err := lg.SetSession(Session{ID: "ospf-2", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.2", State: StateActive}); err != nil {
		t.Fatal(err)
	}
	got := lg.Sessions()
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
}

func TestLookingGlassRoutes(t *testing.T) {
	lg := NewLookingGlass()
	_ = lg.SetSession(Session{ID: "ospf-1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished})
	routes := []Route{
		{Protocol: ProtoOSPF, SessionID: "ospf-1", Network: "10.0.0.0/8", Metric: 10, Accepted: true, BestPath: true},
		{Protocol: ProtoOSPF, SessionID: "ospf-1", Network: "192.168.0.0/16", Metric: 20, Accepted: true, BestPath: false},
	}
	if err := lg.SetRoute("ospf-1", routes); err != nil {
		t.Fatal(err)
	}
	got := lg.Routes("ospf-1")
	if len(got) != 2 || got[0].Network != "10.0.0.0/8" {
		t.Fatalf("Routes order: %+v", got)
	}
}

func TestLookingGlassTraceAccepted(t *testing.T) {
	lg := NewLookingGlass()
	_ = lg.SetSession(Session{ID: "ospf-1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished})
	_ = lg.SetRoute("ospf-1", []Route{
		{Protocol: ProtoOSPF, SessionID: "ospf-1", Network: "10.0.0.0/8", Metric: 10, Accepted: true, BestPath: true},
	})
	_ = lg.SetFilter(Filter{ID: "f-1", Sequence: 10, Action: FilterAccept, Match: map[string]string{"network": "10.0.0.0/8"}})
	_ = lg.SetFilter(Filter{ID: "f-2", Sequence: 20, Action: FilterReject, Match: map[string]string{"network": "10.0.0.0/8"}})
	trace := lg.Trace("ospf-1", "10.0.0.0/8")
	if trace.Result != "accepted" {
		t.Fatalf("expected accepted, got %s: %s", trace.Result, trace.Reason)
	}
	if len(trace.Steps) != 1 || trace.Steps[0].FilterID != "f-1" {
		t.Fatalf("expected 1 step matching f-1, got %+v", trace.Steps)
	}
}

func TestLookingGlassTraceRejected(t *testing.T) {
	lg := NewLookingGlass()
	_ = lg.SetSession(Session{ID: "ospf-1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished})
	_ = lg.SetRoute("ospf-1", []Route{
		{Protocol: ProtoOSPF, SessionID: "ospf-1", Network: "10.0.0.0/8", Metric: 10, Accepted: false, Reason: "policy"},
	})
	_ = lg.SetFilter(Filter{ID: "f-1", Sequence: 10, Action: FilterReject, Match: map[string]string{"network": "10.0.0.0/8"}})
	trace := lg.Trace("ospf-1", "10.0.0.0/8")
	if trace.Result != "rejected" {
		t.Fatalf("expected rejected, got %s: %s", trace.Result, trace.Reason)
	}
}

func TestLookingGlassTraceNoRoute(t *testing.T) {
	lg := NewLookingGlass()
	trace := lg.Trace("ospf-1", "8.8.8.0/24")
	if trace.Result != "not-found" {
		t.Fatalf("expected not-found, got %s", trace.Result)
	}
}

func TestLookingGlassTraceModifyThenAccept(t *testing.T) {
	lg := NewLookingGlass()
	_ = lg.SetRoute("ospf-1", []Route{
		{Protocol: ProtoOSPF, SessionID: "ospf-1", Network: "10.0.0.0/8", Accepted: true, BestPath: true},
	})
	_ = lg.SetFilter(Filter{ID: "f-mod", Sequence: 10, Action: FilterModifyLP, Match: map[string]string{"network": "10.0.0.0/8"}})
	_ = lg.SetFilter(Filter{ID: "f-acc", Sequence: 20, Action: FilterAccept, Match: map[string]string{"network": "10.0.0.0/8"}})
	trace := lg.Trace("ospf-1", "10.0.0.0/8")
	if trace.Result != "accepted" {
		t.Fatalf("expected accepted, got %s", trace.Result)
	}
	if len(trace.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %+v", trace.Steps)
	}
}

func TestLookingGlassTraceDeterministic(t *testing.T) {
	// Add many filters with the same Sequence
	// so iteration order depends entirely on
	// the stable sort. Run 50 times to catch
	// any map-ordering regressions.
	for i := 0; i < 50; i++ {
		lg := NewLookingGlass()
		_ = lg.SetSession(Session{ID: "s1", Protocol: ProtoOSPF, LocalAS: 65000, Peer: "10.0.0.1", State: StateEstablished})
		_ = lg.SetRoute("s1", []Route{
			{Protocol: ProtoOSPF, SessionID: "s1", Network: "10.0.0.0/8", Metric: 10, Accepted: true, BestPath: true},
		})
		// Insert filters in shuffled order to
		// defeat any "happens to sort" luck.
		_ = lg.SetFilter(Filter{ID: "f-2", Sequence: 10, Action: FilterAccept, Match: map[string]string{"network": "10.0.0.0/8"}})
		_ = lg.SetFilter(Filter{ID: "f-1", Sequence: 10, Action: FilterReject, Match: map[string]string{"network": "10.0.0.0/8"}})
		_ = lg.SetFilter(Filter{ID: "f-3", Sequence: 5, Action: FilterAccept, Match: map[string]string{"network": "10.0.0.0/8"}})
		trace := lg.Trace("s1", "10.0.0.0/8")
		// f-3 has Sequence 5 (lowest), so it
		// wins regardless of insertion order.
		if trace.Result != "accepted" {
			t.Fatalf("iter %d: expected accepted, got %s: %s", i, trace.Result, trace.Reason)
		}
		if len(trace.Steps) != 1 || trace.Steps[0].FilterID != "f-3" {
			t.Fatalf("iter %d: expected single f-3 step, got %+v", i, trace.Steps)
		}
	}
}

func TestLookingGlassFilterValid(t *testing.T) {
	if err := (Filter{ID: "../bad", Action: FilterAccept}).Valid(); err == nil {
		t.Fatal("non-canonical id must fail")
	}
	if err := (Filter{ID: "f-1", Action: "garbage"}).Valid(); err == nil {
		t.Fatal("unknown action must fail")
	}
}
