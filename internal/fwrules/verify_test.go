// verify_test.go: regression coverage for exact semantic
// firewall verification, empty-ruleset semantics and scoped
// snapshot decoding (beta.3 release-blocker remediation).
// Fixtures mirror REAL `nft -j list table` output (Debian 13,
// nftables 1.1.3): comments are rule-level fields, terminal
// verdicts are bare {"drop":null} keys, prefixes are
// {"prefix":{addr,len}} objects.
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

func ruleObject(comment string, exprs ...string) string {
	return `{"rule":{"family":"inet","table":"` + TableName + `","chain":"forward","handle":7,"comment":"` + comment + `","expr":[` +
		strings.Join(exprs, ",") + `]}}`
}

const (
	matchSaddr = `{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":{"prefix":{"addr":"10.0.0.0","len":8}}}}`
	matchDport = `{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}}`
	verdictAcc = `{"accept":null}`
	verdictDrp = `{"drop":null}`
	counter    = `{"counter":{"bytes":120,"packets":5}}`
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
	wire := `{"nftables":[{"metainfo":{"version":"1.1.3","json_schema_version":1}},` +
		tableObject(TableName) + `,` + chainObject(TableName) + `,` +
		ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc) + `]}`
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
		"foreign rule":   `{"nftables":[{"rule":{"family":"inet","table":"duckos","chain":"forward","expr":[{"accept":null}]}}]}`,
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
		ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc),
		ruleObject("duckos_fw:wan-drop",
			`{"match":{"op":"==","left":{"meta":{"key":"oifname"}},"right":"eth1"}}`,
			counter, verdictDrp),
	)
	if err := VerifySnapshot(rules, objs); err != nil {
		t.Fatalf("exact state must verify: %v", err)
	}
}

func TestVerifySnapshotRejectsDivergence(t *testing.T) {
	base := func() []json.RawMessage {
		return objects(tableObject(TableName), chainObject(TableName),
			ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc))
	}
	cases := map[string]func() []json.RawMessage{
		"table missing": func() []json.RawMessage {
			return objects(chainObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc))
		},
		"chain missing": func() []json.RawMessage {
			return objects(tableObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc))
		},
		"extra chain": func() []json.RawMessage {
			return append(base(), json.RawMessage(chainObject(TableName)))
		},
		"wrong policy": func() []json.RawMessage {
			return objects(tableObject(TableName),
				`{"chain":{"family":"inet","table":"duckos_fw","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"drop"}}`,
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc))
		},
		"missing rule": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName))
		},
		"extra rule": func() []json.RawMessage {
			return append(base(), json.RawMessage(ruleObject("someone-else", verdictAcc)))
		},
		"wrong order": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:wan-drop", verdictDrp),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictAcc))
		},
		"wrong action": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter, verdictDrp))
		},
		"wrong port": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr,
					`{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":8443}}`,
					counter, verdictAcc))
		},
		"wrong comment": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("someone-else", matchSaddr, matchDport, counter, verdictAcc))
		},
		"wrong srcprefix": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:lan-web",
					`{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":{"prefix":{"addr":"192.168.0.0","len":16}}}}`,
					matchDport, counter, verdictAcc))
		},
		"no verdict": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, counter))
		},
		"unsupported expr": func() []json.RawMessage {
			return objects(tableObject(TableName), chainObject(TableName),
				ruleObject("duckos_fw:lan-web", matchSaddr, matchDport, `{"limit":{"rate":1}}`, counter, verdictAcc))
		},
	}
	for name, build := range cases {
		if err := VerifySnapshot([]Rule{lanWebRule()}, build()); err == nil {
			t.Fatalf("%s: expected verification failure", name)
		}
	}
}

func TestVerifySnapshotEmptyExpectation(t *testing.T) {
	// Canonical empty state: Render removes the whole table, so a
	// readback containing any owned object is a divergence.
	if err := VerifySnapshot(nil, objects(tableObject(TableName), chainObject(TableName))); err == nil {
		t.Fatal("empty ruleset with live chain must not verify")
	}
}
