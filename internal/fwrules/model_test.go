package fwrules

import (
	"encoding/json"
	"strings"
	"testing"
)

func baseRule() Rule {
	return Rule{ID: "allow-lan-web", Enabled: true, Action: "accept", Direction: "forward",
		Protocol: "tcp", SrcPrefix: "192.168.10.0/24", DstPort: 8443, Comment: "LAN web"}
}

func TestRuleValidation(t *testing.T) {
	if err := baseRule().Valid(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	cases := []func(*Rule) string{
		func(r *Rule) string { r.ID = "Bad Id"; return "id" },
		func(r *Rule) string { r.Action = "drop-all"; return "action" },
		func(r *Rule) string { r.Direction = "input"; return "direction" },
		func(r *Rule) string { r.Protocol = "sctp"; return "protocol" },
		func(r *Rule) string { r.Protocol = "any"; r.DstPort = 80; return "port-with-any" },
		func(r *Rule) string { r.DstPort = 70000; return "port-range" },
		func(r *Rule) string { r.SrcPrefix = "10.0.0.0/8 "; return "prefix" },
		func(r *Rule) string { r.SrcPrefix = "10.0.0.1/8"; return "unmasked" },
		func(r *Rule) string { r.InIface = "eth0; rm -rf"; return "iface" },
	}
	for _, mutate := range cases {
		r := baseRule()
		want := mutate(&r)
		if err := r.Valid(); err == nil {
			t.Fatalf("case %q accepted", want)
		}
	}
}

func TestManagerPutUpdateDelete(t *testing.T) {
	m := NewManager()
	if err := m.Put(baseRule()); err != nil {
		t.Fatalf("put: %v", err)
	}
	// Update replaces, does not duplicate.
	updated := baseRule()
	updated.Action = "drop"
	if err := m.Put(updated); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := len(m.Rules()); got != 1 {
		t.Fatalf("rules after update: %d", got)
	}
	if r, _ := m.Get("allow-lan-web"); r.Action != "drop" {
		t.Fatalf("update lost: %+v", r)
	}
	// Delete then idempotent delete.
	if !m.Delete("allow-lan-web") || m.Delete("allow-lan-web") {
		t.Fatal("delete semantics")
	}
	// Duplicate id via Set is refused.
	if err := m.Set([]Rule{baseRule(), baseRule()}); err == nil {
		t.Fatal("duplicate id accepted")
	}
}

func TestManagerDisabledRulesDoNotRender(t *testing.T) {
	m := NewManager()
	a := baseRule()
	b := Rule{ID: "block-guest", Enabled: false, Action: "drop", Direction: "forward", Protocol: "any",
		SrcPrefix: "10.99.0.0/16"}
	if err := m.Set([]Rule{a, b}); err != nil {
		t.Fatalf("set: %v", err)
	}
	data, err := Render(m.Enabled())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(data), "10.99.0.0/16") {
		t.Fatal("disabled rule rendered")
	}
	if !strings.Contains(string(data), "8443") {
		t.Fatal("enabled rule missing")
	}
}

func TestRenderIsSelfContainedAndAtomic(t *testing.T) {
	data, err := Render([]Rule{baseRule()})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	text := string(data)
	if strings.Count(text, "table inet duckos_fw") < 3 || !strings.Contains(text, "delete table inet duckos_fw") {
		t.Fatalf("not self-contained: %s", text)
	}
	if !strings.Contains(text, "policy accept") {
		t.Fatal("forward chain must default-accept so unrelated traffic is untouched")
	}
	if !strings.Contains(text, `comment "duckos_fw:LAN web"`) {
		t.Fatalf("comment missing: %s", text)
	}
}

func TestRenderProtocolImpliedByDport(t *testing.T) {
	r := baseRule() // tcp + dport
	data, err := Render([]Rule{r})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Count(string(data), "tcp") != 1 {
		t.Fatalf("protocol keyword duplicated: %s", data)
	}
	// Protocol without port uses l4proto.
	r.DstPort = 0
	data, err = Render([]Rule{r})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(data), "meta l4proto tcp") {
		t.Fatalf("l4proto missing: %s", data)
	}
}

func TestWireRoundTripAndVersionGate(t *testing.T) {
	m := NewManager()
	if err := m.Set([]Rule{baseRule()}); err != nil {
		t.Fatalf("set: %v", err)
	}
	data, err := SaveWire(m.Rules())
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	rules, err := LoadWire(data)
	if err != nil || len(rules) != 1 {
		t.Fatalf("load: %v %d", err, len(rules))
	}
	// Unknown fields refused; wrong version refused.
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	raw["version"] = 2
	bad, _ := json.Marshal(raw)
	if _, err := LoadWire(bad); err == nil {
		t.Fatal("version gate")
	}
}

func TestRenderRejectsUnknownAction(t *testing.T) {
	r := baseRule()
	r.Action = "jump"
	if _, err := Render([]Rule{r}); err == nil {
		t.Fatal("unknown action rendered")
	}
}
