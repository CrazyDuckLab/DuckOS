// qos2/qos2_test.go pins the V1
// hierarchical QoS surface.
package qos2

import (
	"strings"
	"testing"
)

func TestClassValid(t *testing.T) {
	good := Class{ID: "voice", Parent: "root", Kind: QDiscHTB, RateKbps: 1000, CeilKbps: 5000, BurstBytes: 32 * 1024, Priority: 1, DSCPFilter: []int{46}}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	badRate := Class{ID: "x", Kind: QDiscHTB, CeilKbps: 100, RateKbps: 200}
	if err := badRate.Valid(); err == nil {
		t.Fatal("ceil<rate must fail")
	}
	badDSCP := Class{ID: "x", Kind: QDiscHTB, DSCPFilter: []int{99}}
	if err := badDSCP.Valid(); err == nil {
		t.Fatal("bad DSCP must fail")
	}
}

func TestPolicyValid(t *testing.T) {
	good := Policy{
		ID: "wan1", Scope: ScopeWAN, Target: "wan1", Enabled: true,
		Classes: []Class{
			{ID: "root", Kind: QDiscHTB, RateKbps: 100000, CeilKbps: 100000},
			{ID: "voice", Parent: "root", Kind: QDiscHTB, RateKbps: 1000, CeilKbps: 5000, Priority: 1, DSCPFilter: []int{46}},
		},
	}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	missing := Policy{ID: "x", Scope: ScopeWAN, Target: "y"}
	if err := missing.Valid(); err == nil {
		t.Fatal("missing classes must fail")
	}
	badParent := Policy{
		ID: "x", Scope: ScopeWAN, Target: "y",
		Classes: []Class{{ID: "a", Parent: "missing", Kind: QDiscHTB}},
	}
	if err := badParent.Valid(); err == nil {
		t.Fatal("unknown parent must fail")
	}
	dup := Policy{
		ID: "x", Scope: ScopeWAN, Target: "y",
		Classes: []Class{
			{ID: "a", Kind: QDiscHTB},
			{ID: "a", Kind: QDiscHTB},
		},
	}
	if err := dup.Valid(); err == nil {
		t.Fatal("duplicate id must fail")
	}
}

func TestRenderDeterministic(t *testing.T) {
	p := Policy{
		ID: "wan1", Scope: ScopeWAN, Target: "wan1", Enabled: true,
		Classes: []Class{
			{ID: "root", Kind: QDiscHTB, RateKbps: 100000, CeilKbps: 100000},
			{ID: "voice", Parent: "root", Kind: QDiscHTB, RateKbps: 1000, CeilKbps: 5000, Priority: 1, DSCPFilter: []int{46}},
			{ID: "data", Parent: "root", Kind: QDiscFQCoDel, Priority: 5},
		},
	}
	first, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	hash := hashOf(first)
	for i := 0; i < 10; i++ {
		got, err := Render(p)
		if err != nil {
			t.Fatal(err)
		}
		if hashOf(got) != hash {
			t.Fatalf("render not deterministic")
		}
	}
	if !strings.Contains(first, "htb") || !strings.Contains(first, "fq_codel") {
		t.Fatalf("missing qdisc types: %s", first)
	}
}

func hashOf(s string) string {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	if h == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for h > 0 {
		i--
		buf[i] = byte('0' + h%10)
		h /= 10
	}
	return string(buf[i:])
}
