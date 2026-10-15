//go:build linux

// fw_live_test.go: LIVE nftables qualification against the real
// kernel (beta.3 remediation). Skipped unless DUCKOS_LIVE_NFT=1,
// euid 0, and /usr/sbin/nft exist. Designed to run inside a
// dedicated network namespace (`ip netns exec`) so the host
// ruleset and the management interface are never touched: the
// suite only ever creates and destroys the owned duckos_fw table
// and explicitly cleans it in cleanupOwnedTable.
package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/fwrules"
	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

const liveNFT = "/usr/sbin/nft"

func skipUnlessLive(t *testing.T) {
	t.Helper()
	if os.Getenv("DUCKOS_LIVE_NFT") != "1" {
		t.Skip("live nft qualification disabled (set DUCKOS_LIVE_NFT=1)")
	}
	if os.Geteuid() != 0 {
		t.Skip("live nft qualification requires root")
	}
	if _, err := os.Stat(liveNFT); err != nil {
		t.Skip("nft binary not available")
	}
}

func runNFTLive(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.Command(liveNFT, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("nft %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// cleanupOwnedTable removes the owned table between and after
// tests; absence is not an error.
func cleanupOwnedTable(t *testing.T) {
	t.Helper()
	_ = exec.Command(liveNFT, "delete", "table", "inet", fwrules.TableName).Run()
	t.Cleanup(func() { _ = exec.Command(liveNFT, "delete", "table", "inet", fwrules.TableName).Run() })
}

func liveAdapter(t *testing.T) *fwrulesAdapter {
	t.Helper()
	return &fwrulesAdapter{nftBin: liveNFT, stageDir: t.TempDir()}
}

func liveCandidate(t *testing.T, rules ...fwrules.Rule) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(fwrules.Wire{Version: 1, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func liveRule(id, comment string) fwrules.Rule {
	return fwrules.Rule{ID: id, Enabled: true, Action: "accept", Direction: "forward",
		Protocol: "tcp", SrcPrefix: "10.250.0.0/24", DstPort: 443, Comment: comment}
}

func mustReadback(t *testing.T) ([]json.RawMessage, bool) {
	t.Helper()
	objs, absent, err := readbackFwrTable(liveNFT)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	return objs, absent
}

func TestLiveApplyVerifyExact(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	a := liveAdapter(t)
	rules := []fwrules.Rule{liveRule("allow-web", "LAN web"), {ID: "wan-drop", Enabled: true,
		Action: "drop", Direction: "forward", Protocol: "any", OutIface: "lo"}}
	// Apply signature: (ctx, current, candidate).
	if err := a.Apply(t.Context(), json.RawMessage("{}"), liveCandidate(t, rules...)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := a.Verify(t.Context(), liveCandidate(t, rules...)); err != nil {
		t.Fatalf("exact state must verify against the real kernel: %v", err)
	}
	objs, absent := mustReadback(t)
	if absent || len(objs) == 0 {
		t.Fatal("table must be present with objects after apply")
	}
}

func TestLiveVerifyRejectsForeignDrift(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	a := liveAdapter(t)
	rules := []fwrules.Rule{liveRule("allow-web", "")}
	if err := a.Apply(t.Context(), json.RawMessage("{}"), liveCandidate(t, rules...)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Out-of-band mutation: same number of rules, different port —
	// count-only verification would pass; semantic must fail.
	runNFTLive(t, "add", "rule", "inet", fwrules.TableName, "forward",
		"tcp", "dport", "8443", "accept", "comment", "\"duckos_fw:allow-web\"")
	if err := a.Verify(t.Context(), liveCandidate(t, rules...)); err == nil {
		t.Fatal("diverging kernel state must fail verification")
	}
}

func TestLiveEmptyCandidateRemovesTable(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	a := liveAdapter(t)
	if err := a.Apply(t.Context(), json.RawMessage("{}"), liveCandidate(t, liveRule("allow-web", ""))); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := a.Apply(t.Context(), json.RawMessage("{}"), liveCandidate(t)); err != nil {
		t.Fatalf("empty apply: %v", err)
	}
	if _, absent := mustReadback(t); !absent {
		t.Fatal("owned table must be absent after empty apply")
	}
	if err := a.Verify(t.Context(), liveCandidate(t)); err != nil {
		t.Fatalf("empty candidate must verify on absent table: %v", err)
	}
}

func TestLiveRestoreKnownGood(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	a := liveAdapter(t)
	first := []fwrules.Rule{liveRule("allow-web", "")}
	second := []fwrules.Rule{liveRule("allow-web", ""), {ID: "block-guest", Enabled: true,
		Action: "drop", Direction: "forward", Protocol: "udp", DstPort: 53, SrcPrefix: "10.250.1.0/24"}}
	if err := a.Apply(t.Context(), json.RawMessage("{}"), liveCandidate(t, first...)); err != nil {
		t.Fatalf("apply first: %v", err)
	}
	// Restore re-applies `before`; the candidate documents the
	// failed change being rolled back.
	if err := a.Restore(t.Context(), liveCandidate(t, second...), liveCandidate(t, first...)); err != nil {
		t.Fatalf("restore to known-good: %v", err)
	}
	if err := a.Verify(t.Context(), liveCandidate(t, second...)); err != nil {
		t.Fatalf("restored state must verify: %v", err)
	}
	// Rollback to empty removes the table entirely.
	if err := a.Restore(t.Context(), liveCandidate(t), liveCandidate(t, second...)); err != nil {
		t.Fatalf("restore to empty: %v", err)
	}
	if _, absent := mustReadback(t); !absent {
		t.Fatal("restore to empty must leave the table absent")
	}
}

func TestLiveReadbackAbsentClassification(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	if _, absent, err := readbackFwrTable(liveNFT); err != nil || !absent {
		t.Fatalf("missing owned table must classify as absent, got absent=%v err=%v", absent, err)
	}
}

// TestLiveSafeApplyLifecycle exercises the journal-first pipeline
// against the real kernel: admit → apply → confirm; a second change
// rolled back explicitly; pending-change crash recovery on reopen;
// and automatic rollback on confirm-window expiry.
func TestLiveSafeApplyLifecycle(t *testing.T) {
	skipUnlessLive(t)
	cleanupOwnedTable(t)
	dir := t.TempDir()
	storePath, stageDir := filepath.Join(dir, "store"), filepath.Join(dir, "stage")
	// The journal path is a private, pre-existing directory.
	for _, d := range []string{storePath, stageDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("dir %s: %v", d, err)
		}
	}
	svc, err := NewFirewallRulesService(storePath, stageDir, liveNFT, acceptAccess{})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	rule1 := liveRule("allow-web", "")
	if _, err := svc.worker.SubmitRequest("", "", liveCandidate(t, rule1), "test", "live-admit", FirewallConfirmWindow); err != nil {
		t.Fatalf("admit: %v", err)
	}
	waitKernelRule(t, "allow-web")
	if err := svc.controller.Confirm(waitPendingID(t, svc)); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// Explicit rollback of a second change.
	rule2 := fwrules.Rule{ID: "block-guest", Enabled: true, Action: "drop",
		Direction: "forward", Protocol: "udp", DstPort: 53}
	if _, err := svc.worker.SubmitRequest("", "", liveCandidate(t, rule1, rule2), "test", "live-admit-2", FirewallConfirmWindow); err != nil {
		t.Fatalf("admit 2: %v", err)
	}
	waitKernelRule(t, "block-guest")
	if err := svc.controller.Rollback(waitPendingID(t, svc)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	waitKernelWithoutRule(t, "block-guest")

	// Crash recovery: admit a third change and abandon the process
	// (Close without confirm); reopening the journal must restore
	// the last known-good state and expire the pending transaction.
	pending := fwrules.Rule{ID: "crash-rule", Enabled: true, Action: "reject",
		Direction: "forward", Protocol: "any"}
	if _, err := svc.worker.SubmitRequest("", "", liveCandidate(t, rule1, pending), "test", "live-admit-3", FirewallConfirmWindow); err != nil {
		t.Fatalf("admit 3: %v", err)
	}
	waitKernelRule(t, "crash-rule")
	svc.Close()
	svc2, err := NewFirewallRulesService(storePath, stageDir, liveNFT, acceptAccess{})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(svc2.Close)
	deadline := time.Now().Add(5 * time.Second)
	for {
		rules := kernelRuleComments(t)
		if !strings.Contains(rules, "crash-rule") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pending transaction not recovered after restart; kernel still has: %s", rules)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := liveAdapter(t).Verify(t.Context(), liveCandidate(t, rule1)); err != nil {
		t.Fatalf("recovered state must equal last confirmed: %v", err)
	}
	svc2.Close()

	// Automatic rollback when the confirmation window expires.
	store2 := filepath.Join(dir, "store2")
	if err := os.MkdirAll(store2, 0o700); err != nil {
		t.Fatalf("store2: %v", err)
	}
	validate := func(raw json.RawMessage) error { _, err := fwrules.LoadWire(raw); return err }
	empty, _ := json.Marshal(fwrules.Wire{Version: 1, Rules: []fwrules.Rule{}})
	st, err := state.Open(store2, empty, validate)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	stage2 := filepath.Join(dir, "stage2")
	if err := os.MkdirAll(stage2, 0o700); err != nil {
		t.Fatalf("stage2: %v", err)
	}
	adapter := &fwrulesAdapter{nftBin: liveNFT, stageDir: stage2}
	ctl, err := safeapply.Open(st, adapter)
	if err != nil {
		t.Fatalf("controller: %v", err)
	}
	w := safeapply.NewWorker(ctl)
	if _, err := w.SubmitRequest("", "", liveCandidate(t, liveRule("timeout-rule", "")), "test", "timeout", 2*time.Second); err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitKernelRule(t, "timeout-rule")
	deadline = time.Now().Add(10 * time.Second)
	for {
		snap, err := ctl.Status()
		if err == nil && snap.Pending == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("confirm timeout never rolled the change back: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	waitKernelWithoutRule(t, "timeout-rule")
	ctl.Close()
	st.Close()
}

func waitKernelRule(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if strings.Contains(kernelRuleComments(t), "duckos_fw:"+id) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("rule %s never reached the kernel: %s", id, kernelRuleComments(t))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitKernelWithoutRule(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if !strings.Contains(kernelRuleComments(t), "duckos_fw:"+id) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("rule %s still in the kernel after removal", id)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func kernelRuleComments(t *testing.T) string {
	t.Helper()
	out, err := exec.Command(liveNFT, "-j", "list", "table", "inet", fwrules.TableName).Output()
	if err != nil {
		return ""
	}
	var wire struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal(out, &wire) != nil {
		return ""
	}
	var comments []string
	for _, entry := range wire.NFTables {
		var rule struct {
			Expr []map[string]json.RawMessage `json:"expr"`
		}
		if json.Unmarshal(entry["rule"], &rule) != nil {
			continue
		}
		for _, expr := range rule.Expr {
			var c struct {
				Comment string `json:"comment"`
			}
			if json.Unmarshal(expr["comment"], &c) == nil && c.Comment != "" {
				comments = append(comments, c.Comment)
			}
		}
	}
	return strings.Join(comments, ",")
}

func waitPendingID(t *testing.T, svc *FirewallRulesService) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, _, err := svc.worker.Status()
		if err == nil && job.TransactionID != "" {
			return job.TransactionID
		}
		if time.Now().After(deadline) {
			t.Fatalf("no pending transaction id: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
