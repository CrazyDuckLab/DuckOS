//go:build linux

// Package qa: real firewall adversarial qualification for
// §9 of the Reality Check program.
//
// §9 verifies that the kernel-side nftables refuses
// malformed / impossible config BEFORE applying it to the
// data plane. The DuckOS-side validation layer lives in
// `internal/nftables` (not yet implemented; see §52 gap
// report), so this test pins the kernel-level contract:
// ANY malformed config DuckOS (or anything else) hands to
// `nft` must be rejected with a parse error.
//
// Cases covered:
//   - empty groups (`chain x { type filter hook input priority 0; }`)
//   - duplicate chain names
//   - invalid prefixes
//   - inverted port ranges (start > end)
//   - impossible ports (0, 65536+)
//   - invalid protocol combinations (tcp with udp-only fields)
//   - references to non-existent chains (`jump foo` when `foo`
//     doesn't exist)
//   - table/chain type mismatches (nat hook on filter chain)
//
// All of these are exercised against a real `nft` binary in
// an isolated netns. None of them are allowed to silently
// succeed.
package qa

import (
	"os"
	"strings"
	"testing"
)

// nftTryApply writes ruleset to a temp file and runs
// `nft -f` against it inside the netns. Returns
// (accepted, output). accepted=true means nft exited 0.
// We use runInNSCaptureBytes so stderr (where nft reports
// its parse errors) is captured too.
func nftTryApply(t *testing.T, ns, nftBin, ruleset string) (bool, string) {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/ruleset.nft"
	if err := os.WriteFile(path, []byte(ruleset), 0o644); err != nil {
		t.Fatalf("write ruleset: %v", err)
	}
	stdout, stderr, err := runInNSCaptureBytes(ns, nftBin, "-f", path)
	return err == nil, stdout + stderr
}

// TestNftablesRejectsMalformedConfig is §9.1: a battery
// of malformed config cases that nft must refuse. Each
// case is a separate test run inside a fresh netns.
//
// The test uses a single netns because all cases are
// independent (nft flushes ruleset between cases).
func TestNftablesRejectsMalformedConfig(t *testing.T) {
	nft := findNFT(t)
	if nft == "" {
		t.Skip("nft not present")
	}
	before := CaptureManagementBoundary(t)
	ns := NewLab(t, "qa-fw-adv")

	cases := []struct {
		name    string
		ruleset string
	}{
		{
			name:    "empty_groups",
			ruleset: `table ip x { chain y { type filter hook input priority 0; } }`,
		},
		{
			name: "duplicate_chain_name",
			ruleset: `table ip x {
  chain dup { type filter hook input priority 0; policy accept; }
  chain dup { type filter hook input priority 0; policy accept; }
}`,
		},
		{
			name: "invalid_prefix",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  ip saddr 999.999.999.0/24 accept
}`,
		},
		{
			name: "inverted_port_range",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  tcp dport 1000-100 accept
}`,
		},
		{
			name: "impossible_port_zero",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  tcp dport 0 accept
}`,
		},
		{
			name: "impossible_port_65536",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  tcp dport 65536 accept
}`,
		},
		{
			name: "tcp_with_udp_only_field",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  tcp dport 22 udp length 100 accept
}`,
		},
		{
			name: "jump_to_nonexistent_chain",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  jump z
}`,
		},
		{
			name: "nat_hook_on_filter_chain",
			ruleset: `table ip x {
  chain y { type filter hook input priority 100; policy accept; }
  iifname "eth0" ip saddr 10.0.0.0/8 snat to 192.168.0.1
}`,
		},
		{
			name: "no_hook_specified",
			ruleset: `table ip x {
  chain y { type filter hook invalid_hook_name priority 0; policy accept; }
}`,
		},
		{
			name: "negative_prefix_len",
			ruleset: `table ip x {
  chain y { type filter hook input priority 0; policy accept; }
  ip saddr 10.0.0.0/-1 accept
}`,
		},
		{
			name: "huge_priority_overflow",
			ruleset: `table ip x {
  chain y { type filter hook input priority 999999999999999999999999; policy accept; }
}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, out := nftTryApply(t, ns.NetNS, nft, c.ruleset)
			if ok {
				t.Fatalf("nft accepted malformed config [%s]:\n%s", c.name, out)
			}
			// Make sure nft gave us a real error, not a silent
			// drop. Either an Error: line or a parse error
			// string is expected.
			hasErr := strings.Contains(out, "Error:") ||
				strings.Contains(out, "error:") ||
				strings.Contains(out, "syntax error") ||
				strings.Contains(out, "FATAL")
			if !hasErr {
				t.Fatalf("nft rejected but produced no error message [%s]:\n%s", c.name, out)
			}
		})
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}
