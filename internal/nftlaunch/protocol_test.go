package nftlaunch

import (
	"strings"
	"testing"
)

// Acceptance: every batch shape the three DuckOS renderers emit must
// validate. These are the production code paths (fwrules, UPnP,
// network safeapply + forward guard).
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
		"flush chain flat": `flush chain inet duckos_fw forward
`,
		"delete chain flat": `delete chain inet duckos_upnp prerouting
`,
	}
	for name, batch := range cases {
		if err := ValidateBatch([]byte(batch)); err != nil {
			t.Errorf("%s: renderer output must validate: %v", name, err)
		}
	}
}

// Denial: every structurally-owned-bypass attempt must be rejected.
// Security tests here prove denial of the launcher's batch surface.
func TestValidateBatchDenials(t *testing.T) {
	denials := map[string]string{
		"flush ruleset":         "flush ruleset\n",
		"destroy ruleset":       "destroy ruleset\n",
		"smuggled flush":        "add rule inet duckos_fw forward accept; flush ruleset\n",
		"foreign table":         "add table inet eviltab\n",
		"foreign family ip":     "add table ip eviltab\n",
		"foreign family inet6":  "add table inet6 duckos\n",
		"rule into foreign tab": "add rule inet eviltab c accept\n",
		"list in batch":         "list ruleset\n",
		"include":               "include \"/etc/passwd\"\n",
		"define":                "define EVIL = 1.2.3.4\n",
		"monitor":               "monitor rules\n",
		"reset":                 "reset counters table inet duckos\n",
		"element injection":     "add element inet duckos_fw myset { 1.2.3.4 }\n",
		"nested table block":    "table inet duckos { table inet eviltab { } }\n",
		"unbalanced braces":     "table inet duckos { chain input { accept\n",
		"empty":                 "",
		"oversized":             strings.Repeat("add rule inet duckos_fw forward accept\n", MaxStatements+1),
		"rule starts add":       "table inet duckos_fw { chain c { add rule inet duckos_fw c accept } }\n",
		"rule starts include":   "table inet duckos_fw { chain c { include \"/x\" } }\n",
		"variable reference":    "table inet duckos_fw { chain c { $x accept } }\n",
		"delete chain foreign":  "delete chain inet eviltab c\n",
		"no family":             "add table duckos_fw\n",
	}
	for name, batch := range denials {
		if err := ValidateBatch([]byte(batch)); err == nil {
			t.Errorf("%s: must be denied, got accepted", name)
		}
	}
}

// The oversized-batch case above relies on the statement cap; check
// the byte cap separately.
func TestValidateBatchByteCap(t *testing.T) {
	big := strings.Repeat("add rule inet duckos_fw forward accept\n", 1+MaxBatchBytes/40)
	if err := ValidateBatch([]byte(big)); err == nil {
		t.Fatal("byte-oversized batch must be denied")
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

func TestSplitTopSemicolonSeparators(t *testing.T) {
	stmts, err := splitTop("table inet duckos { chain input { type filter hook input priority filter; policy drop; accept } }")
	if err != nil {
		t.Fatalf("chain-body semicolons must split, not fail: %v", err)
	}
	if len(stmts) != 1 {
		t.Fatalf("block must stay one top-level statement, got %d", len(stmts))
	}
	// A stray top-level `;` still splits into fragments, each of
	// which must then pass the allowlist (defence in depth is the
	// fragment validation, not the separator refusal).
	stmts, err = splitTop("add rule inet duckos_fw forward accept; flush ruleset")
	if err != nil {
		t.Fatalf("top-level semicolon must split: %v", err)
	}
	if len(stmts) != 2 {
		t.Fatalf("expected 2 fragments, got %d", len(stmts))
	}
	if err := ValidateBatch([]byte("add rule inet duckos_fw forward accept; flush ruleset\n")); err == nil {
		t.Fatal("smuggled flush ruleset must be denied by fragment validation")
	}
}

func TestRedactTokens(t *testing.T) {
	out := redactTokens([]string{"add", "rule", "inet", "duckos_fw", "forward", "secret-hostname", "accept"})
	if strings.Contains(out, "secret-hostname") {
		t.Fatalf("payload must be redacted, got %q", out)
	}
}
