package network

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPolicyInventoryPreservesNumericSelectorsAndGuards(t *testing.T) {
	raw := `[{"priority":0,"src":"all","table":"255","protocol":"2"},{"priority":10000,"src":"all","dst":"198.51.100.0","dstlen":24,"table":"254","suppress_prefixlen":23,"protocol":"4"},{"priority":10001,"src":"all","dst":"198.51.100.0","dstlen":24,"action":"unreachable","protocol":"4"},{"priority":20000,"src":"198.51.100.20","table":"10000","protocol":"4"},{"priority":20001,"src":"198.51.100.20","action":"7","protocol":"4"},{"priority":30000,"src":"all","fwmark":"0x1","table":"52"}]`
	rules, err := decodePolicyInventory([]byte(raw), "ipv4")
	if err != nil || len(rules) != 4 || rules[0].SuppressPrefixLength != 23 || rules[2].From != "198.51.100.20/32" || !rules[3].Unreachable || rules[3].Table != 0 {
		t.Fatal(rules, err)
	}
	s := PolicySnapshot{Version: 1, Rules: rules}
	encoded, _ := json.Marshal(s)
	decoded, err := DecodePolicySnapshot(encoded)
	if err != nil || !equalPolicy(s, decoded) {
		t.Fatal(decoded, err)
	}
	for _, bad := range []string{strings.Replace(raw, `"priority":20000`, `"priority":20000,"fwmark":1`, 1), strings.Replace(raw, `"action":"7"`, `"action":"prohibit"`, 1), strings.Replace(raw, `"table":"10000"`, `"table":"52"`, 1), strings.Replace(raw, `"src":"198.51.100.20"`, `"src":"198.51.100.20","srclen":24`, 1), strings.Replace(raw, `"priority":20001`, `"priority":20000`, 1), `[{"src":"all"}]`, `null`} {
		if _, err := decodePolicyInventory([]byte(bad), "ipv4"); err == nil {
			t.Fatal("unreproducible inventory accepted", bad)
		}
	}
}

func TestPolicySnapshotCompilesSharedInstructions(t *testing.T) {
	c := policyFixture()
	s, err := PolicySnapshotForConfig(c)
	if err != nil || len(s.Rules) != 8 {
		t.Fatal(s, err)
	}
	for _, r := range s.Rules {
		if r.Protocol != 4 || r.Unreachable != (r.Priority%2 == 1) {
			t.Fatal(r)
		}
		args := strings.Join(policyArguments("add", r), " ")
		if strings.Contains(args, "flush") || !strings.Contains(args, "protocol 4") {
			t.Fatal(args)
		}
	}
	c.PolicyRules[0].From = "0.0.0.0/0"
	s, err = PolicySnapshotForConfig(c)
	if err != nil {
		t.Fatal("wildcard policy lost", err)
	}
	for _, r := range s.Rules {
		if r.Priority >= 20000 && r.From != "" {
			t.Fatal("wildcard not normalized", r)
		}
	}
	a := KernelPolicyRule{Family: "ipv6", Priority: 20002, From: "2001:db8::/64", To: "2001:db8:2::9/128", Table: 10001, SuppressPrefixLength: -1, Protocol: 4}
	raw := `[{"priority":20002,"src":"2001:db8::","srclen":64,"dst":"2001:db8:2::9","table":"10001","protocol":"4"}]`
	rules, err := decodePolicyInventory([]byte(raw), "ipv6")
	if err != nil || !reflect.DeepEqual(rules, []KernelPolicyRule{a}) {
		t.Fatal(rules, err)
	}
}

func TestPolicySnapshotRejectsCorruptRecovery(t *testing.T) {
	s, _ := PolicySnapshotForConfig(policyFixture())
	for _, change := range []func(*PolicySnapshot){func(s *PolicySnapshot) { s.Version = 2 }, func(s *PolicySnapshot) { s.Rules[0].Priority = 32766 }, func(s *PolicySnapshot) { s.Rules[0].SuppressPrefixLength = -1 }, func(s *PolicySnapshot) { s.Rules[1].Table = 254 }, func(s *PolicySnapshot) { s.Rules[0].Protocol = 256 }, func(s *PolicySnapshot) { s.Rules = append(s.Rules, s.Rules[0]) }} {
		x := PolicySnapshot{Version: s.Version, Rules: append([]KernelPolicyRule(nil), s.Rules...)}
		change(&x)
		raw, _ := json.Marshal(x)
		if _, err := DecodePolicySnapshot(raw); err == nil {
			t.Fatal("corrupt snapshot accepted", x)
		}
	}
}
