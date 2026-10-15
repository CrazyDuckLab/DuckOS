// verify_test.go: regression coverage for exact semantic
// firewall verification, empty-ruleset semantics and scoped
// snapshot decoding (beta.3 release-blocker remediation).
package fwrules

import (
	"encoding/json"
	"strings"
	"testing"
)

func tableObject(name string) string {
	return `{"table":{"family":"inet","name":"` + name + `"}}`
}

func chainObject(table string) string {
	return `{"chain":{"family":"inet","table":"` + table + `","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"accept"}}`
}

func ruleObject(table, chain string, exprs ...string) string {
	return `{"rule":{"family":"inet","table":"` + table + `","chain":"` + chain + `","handle":7,"expr":[` +
		strings.Join(exprs, ",") + `]}}`
}

const (
	commentLAN = `{"comment":"duckos_fw:lan-web"}`
	matchSaddr = `{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"10.0.0.0/8"}}`
	matchDport = `{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}}`
	verdictAcc = `{"verdict":{"verdict":"accept"}}`
	counter    = `{"counter":{"packets":5,"bytes":120}}`
)

func lanWebRule() Rule {
	return Rule{ID: "lan-web", Enabled: true, Action: "accept", Direction: "forward",
		Protocol: "tcp", SrcPrefix: "10.0.0.0/8", DstPort: 443}
}

func objects(raw ...string) []json.RawMessage {
	var out []json.RawMessage
	for _, s := range raw {
		out = append(out, json.RawMessage(s))
	}
	return out
}

func TestDecodeSnapshotObjectsAcceptsOwnedTable(t *testing.T) {
	wire := `{"nftables":[{"metainfo":{"version":"1.0.9"}},` +
		tableObject(TableName) + `,` + chainObject(TableName) + `,` +
		ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc) + `]}`
	objs, err := DecodeSnapshotObjects([]byte(wire))
	if err != nil {
		t.Fatalf("owned snapshot rejected: %v", err)
	}
	if len(objs) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objs))
	}
}

func TestDecodeSnapshotObjectsEmptyOutput(t *testing.T) {
	objs, err := DecodeSnapshotObjects(nil)
	if err != nil || len(objs) != 0 {
		t.Fatalf("empty stdout must decode to an empty list, got %v, %v", objs, err)
	}
}

func TestDecodeSnapshotObjectsRejectsForeignTables(t *testing.T) {
	for name, wire := range map[string]string{
		"network table":  `{"nftables":[` + tableObject("duckos") + `]}`,
		"upnp table":     `{"nftables":[` + tableObject("duckos_upnp") + `]}`,
		"foreign family": `{"nftables":[{"table":{"family":"ip","name":"duckos_fw"}}]}`,
		"foreign chain":  `{"nftables":[` + chainObject("duckos") + `]}`,
		"foreign rule":   `{"nftables":[` + ruleObject("duckos", "forward", verdictAcc) + `]}`,
		"malformed json": `{"nftables":[{"table":}]}`,
		"bad envelope":   `{"nftables":null}`,
		"unknown field":  `{"nftables":[],"extra":1}`,
	} {
		if _, err := DecodeSnapshotObjects([]byte(wire)); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
	}
}

func TestRenderEmptyRulesetRemovesTable(t *testing.T) {
	for name, rules := range map[string][]Rule{
		"nil":        nil,
		"empty":      {},
		"all denied": {{ID: "off", Enabled: false, Action: "drop", Direction: "forward", Protocol: "any"}},
	} {
		batch, err := Render(rules)
		if err != nil {
			t.Fatalf("%s: render: %v", name, err)
		}
		text := string(batch)
		if strings.Contains(text, "add chain") || strings.Contains(text, "add rule") {
			t.Fatalf("%s: empty ruleset must not create chains or rules:\n%s", name, text)
		}
		if !strings.Contains(text, "add table inet duckos_fw") || !strings.Contains(text, "delete table inet duckos_fw") {
			t.Fatalf("%s: empty ruleset must be an idempotent table removal:\n%s", name, text)
		}
	}
}

func TestVerifySnapshotExactState(t *testing.T) {
	rules := []Rule{lanWebRule(),
		{ID: "wan-drop", Enabled: true, Action: "drop", Direction: "forward", Protocol: "any", OutIface: "eth1"}}
	objs := objects(
		tableObject(TableName),
		chainObject(TableName),
		ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc),
		ruleObject(TableName, "forward", `{"match":{"op":"==","left":{"meta":{"key":"oifname"}},"right":"eth1"}}`,
			counter, `{"comment":"duckos_fw:wan-drop"}`, `{"verdict":{"verdict":"drop"}}`),
	)
	if err := VerifySnapshot(rules, objs); err != nil {
		t.Fatalf("exact state must verify: %v", err)
	}
}

func TestVerifySnapshotRejectsDivergence(t *testing.T) {
	base := func() []json.RawMessage {
		return objects(tableObject(TableName), chainObject(TableName),
			ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc))
	}
	cases := map[string]func() []json.RawMessage{
		"table missing": func() []json.RawMessage {
			return objects(chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc))
		},
		"chain missing": func() []json.RawMessage {
			return objects(tableObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc))
		},
		"extra chain": func() []json.RawMessage {
			return append(base(), json.RawMessage(chainObject(TableName)))
		},
		"wrong policy": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName), // placeholder, replaced below
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc))
		},
		"missing rule": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName))
		},
		"extra rule": func() []json.RawMessage {
			return append(base(), json.RawMessage(ruleObject(TableName, "forward", verdictAcc)))
		},
		"wrong order": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc),
				ruleObject(TableName, "forward", verdictAcc))
		},
		"wrong action": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN,
					`{"verdict":{"verdict":"drop"}}`))
		},
		"wrong port": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr,
					`{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":8443}}`,
					counter, commentLAN, verdictAcc))
		},
		"wrong comment": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, `{"comment":"someone-else"}`, verdictAcc))
		},
		"wrong srcprefix": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward",
					`{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"192.168.0.0/16"}}`,
					matchDport, counter, commentLAN, verdictAcc))
		},
		"no verdict": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN))
		},
		"unsupported expr": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject(TableName, "forward", matchSaddr, matchDport, `{"limit":{"rate":1}}`, counter, commentLAN, verdictAcc))
		},
	}
	for name, build := range cases {
		var objs []json.RawMessage
		if name == "wrong policy" {
			objs = objects(tableObject(TableName),
				`{"chain":{"family":"inet","table":"duckos_fw","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"drop"}}`,
				ruleObject(TableName, "forward", matchSaddr, matchDport, counter, commentLAN, verdictAcc))
		} else {
			objs = build()
		}
		if err := VerifySnapshot([]Rule{lanWebRule()}, objs); err == nil {
			t.Fatalf("%s: expected verification failure", name)
		}
	}
}

func TestVerifySnapshotEmptyExpectation(t *testing.T) {
	// An empty expectation with a present owned table is a
	// divergence the adapter catches via "absent"; here a
	// table-only readback must fail because the chain exists.
	if err := VerifySnapshot(nil, objects(tableObject(TableName), chainObject(TableName))); err == nil {
		t.Fatal("empty ruleset with live chain must not verify")
	}
}
