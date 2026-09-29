// multiwan/ecmp_test.go pins the V1 multi-WAN
// selection model.
package multiwan

import "testing"

func TestGroupMemberValid(t *testing.T) {
	if err := (GroupMember{WANID: "../bad"}).Valid(); err == nil {
		t.Fatal("non-canonical id must fail")
	}
	if err := (GroupMember{WANID: "wan-1", Weight: -1}).Valid(); err == nil {
		t.Fatal("negative weight must fail")
	}
}

func TestRouteGroupValid(t *testing.T) {
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionFailover, Members: []GroupMember{{WANID: "wan-1", Priority: 1}, {WANID: "wan-2", Priority: 2}}}
	if err := g.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionPolicy("garbage"), Members: []GroupMember{{WANID: "wan-1"}}}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown selection must fail")
	}
	empty := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionFailover, Members: nil}
	if err := empty.Valid(); err == nil {
		t.Fatal("empty members must fail")
	}
}

func TestFailoverSelectsLowestPriority(t *testing.T) {
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionFailover, Members: []GroupMember{{WANID: "wan-1", Priority: 2}, {WANID: "wan-2", Priority: 1}, {WANID: "wan-3", Priority: 3}}}
	if got := g.Select(FlowKey{}); got != "wan-2" {
		t.Fatalf("Select = %q, want wan-2", got)
	}
}

func TestWeightedSelectsStablyForSameFlow(t *testing.T) {
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionWeighted, Members: []GroupMember{{WANID: "wan-1", Weight: 1}, {WANID: "wan-2", Weight: 1}, {WANID: "wan-3", Weight: 1}}}
	a := g.Select(FlowKey{Src: "10.0.0.1", Dst: "8.8.8.8", Sport: 12345, Dport: 443, Proto: "tcp"})
	b := g.Select(FlowKey{Src: "10.0.0.1", Dst: "8.8.8.8", Sport: 12345, Dport: 443, Proto: "tcp"})
	if a != b {
		t.Fatalf("same flow must hash to the same WAN: %q vs %q", a, b)
	}
	if a == "" {
		t.Fatal("weighted select must produce a WAN id")
	}
}

func TestWeightedDistributesOverManyFlows(t *testing.T) {
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionWeighted, Members: []GroupMember{{WANID: "wan-1", Weight: 1}, {WANID: "wan-2", Weight: 1}}}
	hits := map[string]int{}
	for i := 0; i < 1000; i++ {
		w := g.Select(FlowKey{Src: "10.0.0.1", Dst: "8.8.8.8", Sport: 1000 + i, Dport: 443, Proto: "tcp"})
		hits[w]++
	}
	if hits["wan-1"] < 400 || hits["wan-2"] < 400 {
		t.Fatalf("weighted distribution skewed: %+v", hits)
	}
}

func TestStickyRespectsConfiguredHashKeys(t *testing.T) {
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionSticky, StickyHashKeys: []string{"dst"}, Members: []GroupMember{{WANID: "wan-1", Weight: 1}, {WANID: "wan-2", Weight: 1}}}
	// Same dst must always pick the same WAN.
	first := g.Select(FlowKey{Dst: "8.8.8.8"})
	for i := 0; i < 100; i++ {
		if got := g.Select(FlowKey{Dst: "8.8.8.8", Sport: i}); got != first {
			t.Fatalf("sticky with dst-only key must be stable")
		}
	}
}

func TestManagerSetGetListDelete(t *testing.T) {
	m := NewManager()
	g := RouteGroup{ID: "rg-1", Name: "default", Selection: SelectionFailover, Members: []GroupMember{{WANID: "wan-1"}}}
	if err := m.Set(g); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("rg-1"); !ok {
		t.Fatal("Get after Set must find the group")
	}
	if len(m.List()) != 1 {
		t.Fatal("List must contain the group")
	}
	m.Delete("rg-1")
	if _, ok := m.Get("rg-1"); ok {
		t.Fatal("Get after Delete must miss")
	}
}
