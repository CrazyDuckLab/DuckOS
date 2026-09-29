// conntrack/conntrack_test.go pins the V1
// conntrack surface.
package conntrack

import "testing"

func TestFilterValid(t *testing.T) {
	good := Filter{Src: "10.0.0.1", Dst: "8.8.8.8", Sport: 443, Dport: 55555, Proto: "tcp", WANID: "wan1"}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Filter{Proto: "garbage"}
	if err := bad.Valid(); err == nil {
		t.Fatal("unknown proto must fail")
	}
	huge := Filter{MaxResults: 20000}
	if err := huge.Valid(); err == nil {
		t.Fatal("maxResults > 10000 must fail")
	}
}

func TestManagerListFilters(t *testing.T) {
	m := NewManager()
	table := []Entry{
		{Src: "10.0.0.1", Dst: "8.8.8.8", Sport: 12345, Dport: 443, Proto: "tcp", Bytes: 1000},
		{Src: "10.0.0.1", Dst: "1.1.1.1", Sport: 12346, Dport: 443, Proto: "tcp", Bytes: 500},
		{Src: "192.168.1.1", Dst: "8.8.8.8", Sport: 12347, Dport: 80, Proto: "tcp", Bytes: 200},
	}
	tcp443 := m.List(table, Filter{Proto: "tcp", Dport: 443})
	if len(tcp443) != 2 {
		t.Fatalf("expected 2 tcp/443, got %d", len(tcp443))
	}
	src := m.List(table, Filter{Src: "10.0.0.1"})
	if len(src) != 2 {
		t.Fatalf("expected 2 from 10.0.0.1, got %d", len(src))
	}
	none := m.List(table, Filter{Src: "10.0.0.99"})
	if len(none) != 0 {
		t.Fatalf("expected 0, got %d", len(none))
	}
}

func TestManagerKillBounded(t *testing.T) {
	m := NewManager()
	if err := m.KillSelected(Filter{Src: "10.0.0.1", MaxResults: 50}); err != nil {
		t.Fatal(err)
	}
	if err := m.KillSelected(Filter{MaxResults: 2000}); err == nil {
		t.Fatal("kill > 1000 must fail")
	}
	// Refuse unparseable.
	if err := m.KillSelected(Filter{Proto: "garbage"}); err == nil {
		t.Fatal("garbage proto must fail")
	}
}

func TestChangeImpactConstants(t *testing.T) {
	// Document the contract: Safe Apply maps
	// conntrack impact to a small enum.
	if ImpactNewOnly == ImpactEstablished {
		t.Fatal("enum members must differ")
	}
}
