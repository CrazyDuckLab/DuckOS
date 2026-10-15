package nftlaunch

import (
	"strings"
	"testing"
)

// Acceptance: every batch shape the three DuckOS renderers emit must
// validate. These are the production code paths (fwrules, UPnP,
// network safeapply block form, JSON recovery).
func TestValidateBatchAcceptsRendererOutput(t *testing.T) {
	cases := map[string]string{
		"fwrules non-empty": `# duckos.duckos_fw atomic table replacement
add table inet duckos_fw
delete table inet duckos_fw
add table inet duckos_fw
add chain inet duckos_fw forward { type filter hook forward priority filter ; policy accept ; }
add rule inet duckos_fw forward ip saddr 10.250.0.0/24 tcp dport 443 accept comment "duckos_fw:allow-web"
`,
		"fwrules empty-state": `add table inet duckos_fw
delete table inet duckos_fw
`,
		"upnp atomic replacement": `# duckos.upnp atomic table replacement
add table inet duckos_upnp
delete table inet duckos_upnp
add table inet duckos_upnp
add chain inet duckos_upnp prerouting { type nat hook prerouting priority -100 ; }
add rule inet duckos_upnp prerouting tcp dport 5000 dnat ip to 192.168.7.10:5000 comment "duckos_upnp:m-1"
`,
		"network block form": `destroy table inet duckos
table inet duckos {
 chain input {
  type filter hook input priority filter; policy drop;
  iifname != { "eth0", "lan0" } accept
  iifname "lo" accept
  ct state invalid drop
  ip protocol icmp accept
 }
 chain forward {
  type filter hook forward priority filter; policy accept;
  oifname "wan0" drop
 }
}
`,
		"forward guard": `add table inet duckos
add chain inet duckos duckos_apply_guard { type filter hook forward priority -10; policy drop; }
`,
		"quoted iface in rule": `add rule inet duckos_fw forward iifname "eth0" accept
`,
	}
	for name, batch := range cases {
		if err := ValidateBatch([]byte(batch)); err != nil {
			t.Errorf("%s: renderer output must validate: %v", name, err)
		}
	}
}

// TestF1AuditBypassClasses: adversarial regression tests for the
// parser-differential class the independent audit exploited. Every
// case must be DENIED, and the denial must hold even though the
// batch visually looks like an authorized DuckOS operation.
func TestF1AuditBypassClasses(t *testing.T) {
	denials := map[string]string{
		// The audit's confirmed bypass family: quotes/comments/braces
		// interpreted differently from nft, smuggling a foreign-table
		// command through an authorized-looking statement.
		"quote hiding table cmd":    `add rule inet duckos_fw forward comment "a" ; add table inet eviltab`,
		"quote eating brace":        `table inet duckos_fw { chain c { comment "}" } } add table inet eviltab`,
		"comment eats newline":      `add rule inet duckos_fw forward accept # trailing` + "\ndelete table inet eviltab\n",
		"unterminated string":       `add rule inet duckos_fw forward comment "open`,
		"unterminated string EOF":   `add rule inet duckos_fw forward comment "open\n`,
		"line continuation":         "add rule inet duckos_fw forward accept \\\n; delete table inet eviltab\n",
		"unsupported escape":        `add rule inet duckos_fw forward comment "a\nb" accept`,
		"brace imbalance":           "table inet duckos_fw { chain c { accept\n",
		"nested blocks":             "table inet duckos_fw { chain c { chain d { } } }",
		"deep nesting":              "table inet duckos_fw { chain c { { } } }",
		"quoted table name":         `add table inet "duckos_fw"`,
		"quoted family":             `add table "inet" duckos_fw`,
		"quoted chain in prefix":    `add rule inet duckos_fw "forward" accept`,
		"foreign table":             "add table inet eviltab\n",
		"foreign family":            "add table ip eviltab\n",
		"rule into foreign table":   "add rule inet eviltab c accept\n",
		"smuggled flush ruleset":    "add rule inet duckos_fw forward accept; flush ruleset\n",
		"flush ruleset":             "flush ruleset\n",
		"destroy ruleset":           "destroy ruleset\n",
		"include":                   "include \"/etc/passwd\"\n",
		"define":                    "define EVIL = 1.2.3.4\n",
		"monitor":                   "monitor rules\n",
		"reset":                     "reset counters table inet duckos\n",
		"element injection":         "add element inet duckos_fw myset { 1.2.3.4 }\n",
		"nested table in block":     "table inet duckos_fw { chain c { table inet eviltab { } } }\n",
		"rule starts add":           "table inet duckos_fw { chain c { add rule inet duckos_fw c accept } }\n",
		"rule starts include":       "table inet duckos_fw { chain c { include \"/x\" } }\n",
		"variable reference":        "table inet duckos_fw { chain c { $x accept } }\n",
		"delete chain foreign":      "delete chain inet eviltab c\n",
		"no family":                 "add table duckos_fw\n",
		"chain def extra words":     "add chain inet duckos_fw c type filter { }\n",
		"chain def foreign keyword": "add chain inet duckos_fw c { include x }\n",
		"chain def string":          `add chain inet duckos_fw c { type "filter" }`,
		"empty block":               "table inet duckos_fw { }",
		"table block with rule":     "table inet duckos_fw { accept }",
		"table block extra head":    "table inet duckos_fw x { }",
		"empty statement semi":      "add table inet duckos_fw ;; delete table inet duckos_fw",
		"leading semi":              "; add table inet duckos_fw",
		"empty":                     "",
		"whitespace only":           "   \n\t  ",
	}
	for name, batch := range denials {
		if err := ValidateBatch([]byte(batch)); err == nil {
			t.Errorf("F1 bypass %q: must be denied, got accepted", name)
		}
	}
}

// TestF1OwnedSyntaxStillAccepted pins the exact syntax DuckOS
// renderers emit so the strict lexer cannot regress into over-denial.
func TestF1OwnedSyntaxStillAccepted(t *testing.T) {
	accepted := []string{
		`add rule inet duckos_fw forward iifname != { "lan0", "wan0" } accept`,
		`add rule inet duckos_fw forward tcp dport { 22, 443 } accept comment "duckos_fw:web"`,
		`add rule inet duckos_upnp prerouting tcp dport 8080 dnat ip to 192.168.7.10:80 comment "duckos_upnp:m-1"`,
		"flush chain inet duckos_fw forward",
		"flush table inet duckos_fw",
		"delete chain inet duckos_fw forward",
	}
	for _, batch := range accepted {
		if err := ValidateBatch([]byte(batch)); err != nil {
			t.Errorf("owned syntax must be accepted: %v: %v", batch, err)
		}
	}
}

// TestF1LexerMatchesNftInertStrings pins the lexical agreement with
// nft: separators and braces inside quoted strings are inert STRING
// CONTENT (nft executes one harmless rule with that comment), so the
// batch is accepted as a single owned rule — while the same content
// outside a string is a hard denial (see TestF1AuditBypassClasses).
func TestF1LexerMatchesNftInertStrings(t *testing.T) {
	accepted := []string{
		`add rule inet duckos_fw forward comment "x; delete table inet eviltab"`,
		`add rule inet duckos_fw forward comment "a#b" accept`,
		`add rule inet duckos_fw forward comment "a\"b; flush ruleset" accept`,
		`add rule inet duckos_fw forward {bad} accept`,
	}
	for _, batch := range accepted {
		if err := ValidateBatch([]byte(batch)); err != nil {
			t.Errorf("inert-string owned rule must be accepted: %v: %v", batch, err)
		}
	}
}

func TestValidateBatchByteAndStatementCaps(t *testing.T) {
	if err := ValidateBatch([]byte(strings.Repeat("add rule inet duckos_fw forward accept\n", 1+MaxBatchBytes/40))); err == nil {
		t.Fatal("byte-oversized batch must be denied")
	}
	if err := ValidateBatch([]byte(strings.Repeat("add rule inet duckos_fw forward accept\n", MaxStatements+1))); err == nil {
		t.Fatal("statement-oversized batch must be denied")
	}
}

func TestValidateReadTarget(t *testing.T) {
	for _, tbl := range []string{"duckos", "duckos_fw", "duckos_upnp"} {
		if err := ValidateReadTarget("inet", tbl); err != nil {
			t.Errorf("owned table %s rejected: %v", tbl, err)
		}
	}
	for _, tc := range [][2]string{{"inet", "filter"}, {"ip", "duckos"}, {"inet", ""}, {"", "duckos"}, {"arp", "duckos"}} {
		if err := ValidateReadTarget(tc[0], tc[1]); err == nil {
			t.Errorf("read target %v must be denied", tc)
		}
	}
}

// TestF2JSONStrictness: the JSON wrapper must never authorize a
// different object than the kernel executes.
func TestF2JSONStrictness(t *testing.T) {
	owned := func() string {
		return `{"nftables":[
			{"destroy":{"table":{"family":"inet","name":"duckos"}}},
			{"add":{"table":{"family":"inet","name":"duckos"}}},
			{"add":{"chain":{"family":"inet","table":"duckos","name":"input","type":"filter","hook":"input","prio":0,"policy":"drop"}}},
			{"add":{"rule":{"family":"inet","table":"duckos","chain":"input","expr":[{"accept":null}]}}}
		]}`
	}
	if err := ValidateJSONBatch([]byte(owned())); err != nil {
		t.Fatalf("trusted recovery JSON must validate: %v", err)
	}

	denials := map[string]string{
		"duplicate key in command": `{"nftables":[{"add":{"table":{"family":"inet","name":"duckos"}},{"delete":{"table":{"family":"inet","name":"filter"}}}]}`,
		"duplicate key in object":  `{"nftables":[{"add":{"table":{"family":"inet","name":"duckos","name":"filter"}}}]}`,
		"duplicate nftables key":   `{"nftables":[],"nftables":[]}`,
		"unknown document key":     `{"nftables":[],"meta":1}`,
		"trailing content":         `{"nftables":[]} {}`,
		"unknown verb":             `{"nftables":[{"rename":{"table":{"family":"inet","name":"duckos"}}}]}`,
		"unknown kind":             `{"nftables":[{"add":{"socket":{"family":"inet","table":"duckos"}}}]}`,
		"unknown field":            `{"nftables":[{"add":{"table":{"family":"inet","name":"duckos","evil":1}}}]}`,
		"handle on add":            `{"nftables":[{"add":{"rule":{"family":"inet","table":"duckos","chain":"c","handle":7,"expr":[]}}}]}`,
		"flush chain json":         `{"nftables":[{"flush":{"chain":{"family":"inet","table":"duckos","name":"c"}}}]}`,
		"flush ruleset json":       `{"nftables":[{"flush":{"ruleset":{"family":"inet","name":"x"}}}]}`,
		"two kinds one command":    `{"nftables":[{"add":{"table":{"family":"inet","name":"duckos"},"chain":{"family":"inet","table":"duckos","name":"c"}}}]}`,
		"foreign family json":      `{"nftables":[{"add":{"table":{"family":"ip","name":"duckos"}}}]}`,
		"foreign table json":       `{"nftables":[{"add":{"table":{"family":"inet","name":"filter"}}}]}`,
		"rule into foreign json":   `{"nftables":[{"add":{"rule":{"family":"inet","table":"filter","chain":"c","expr":[]}}}]}`,
		"missing family":           `{"nftables":[{"add":{"table":{"name":"duckos"}}}]}`,
		"missing table on rule":    `{"nftables":[{"add":{"rule":{"family":"inet","chain":"c","expr":[]}}}]}`,
		"kind not object":          `{"nftables":[{"add":{"table":"duckos"}}]}`,
		"payload not object":       `{"nftables":[{"add":7}]}`,
		"entry not object":         `{"nftables":[7]}`,
		"no nftables":              `{"commands":[]}`,
		"nested dup deep":          `{"nftables":[{"add":{"rule":{"family":"inet","table":"duckos","chain":"c","expr":[{"match":{"op":"==","op":"!="}}]}}}]}`,
		"array entries":            `{"nftables":[["add"]]}`,
	}
	for name, doc := range denials {
		if err := ValidateJSONBatch([]byte(doc)); err == nil {
			t.Errorf("F2 bypass %q: must be denied", name)
		}
	}
}

func TestRedactStyleErrors(t *testing.T) {
	// Error text must never echo a full attacker-controlled payload.
	err := ValidateBatch([]byte(`add rule inet duckos_fw forward comment "super-secret-hostname-12345" 1`))
	if err != nil && strings.Contains(err.Error(), "super-secret-hostname") {
		t.Fatalf("error must not echo the payload: %v", err)
	}
}
