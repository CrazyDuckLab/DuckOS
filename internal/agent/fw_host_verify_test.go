//go:build linux

// fw_host_verify_test.go: regression tests for the firewall
// adapter Verify path — exact semantic verification, canonical
// empty ruleset and fail-closed readback error classification
// (beta.3 release-blocker remediation). Requires Linux: the
// adapter and nft readback are Agent-only code paths.
package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/fwrules"
)

func fwrObj(raw string) json.RawMessage { return json.RawMessage(raw) }

func fwrSnapshot() []json.RawMessage {
	return []json.RawMessage{
		fwrObj(`{"table":{"family":"inet","name":"duckos_fw"}}`),
		fwrObj(`{"chain":{"family":"inet","table":"duckos_fw","name":"forward","type":"filter","hook":"forward","prio":0,"policy":"accept"}}`),
		fwrObj(`{"rule":{"family":"inet","table":"duckos_fw","chain":"forward","handle":9,"expr":[` +
			`{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"10.0.0.0/8"}},` +
			`{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}},` +
			`{"counter":{"packets":1,"bytes":64}},` +
			`{"comment":"duckos_fw:lan-web"},` +
			`{"verdict":{"verdict":"accept"}}]}}`),
	}
}

func fwrCandidate(t *testing.T, rules ...fwrules.Rule) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(fwrules.Wire{Version: 1, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func fwrAdapter(readback func(string) ([]json.RawMessage, bool, error)) *fwrulesAdapter {
	return &fwrulesAdapter{nftBin: "/usr/sbin/nft", readbackFn: readback}
}

func lanWeb() fwrules.Rule {
	return fwrules.Rule{ID: "lan-web", Enabled: true, Action: "accept", Direction: "forward",
		Protocol: "tcp", SrcPrefix: "10.0.0.0/8", DstPort: 443}
}

func TestFwrulesVerifyExactState(t *testing.T) {
	a := fwrAdapter(func(string) ([]json.RawMessage, bool, error) { return fwrSnapshot(), false, nil })
	if err := a.Verify(t.Context(), fwrCandidate(t, lanWeb())); err != nil {
		t.Fatalf("exact kernel state must verify: %v", err)
	}
}

func TestFwrulesVerifyRejectsDivergence(t *testing.T) {
	wrong := fwrSnapshot()
	wrong[2] = fwrObj(`{"rule":{"family":"inet","table":"duckos_fw","chain":"forward","handle":9,"expr":[` +
		`{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"10.0.0.0/8"}},` +
		`{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":8443}},` +
		`{"comment":"duckos_fw:lan-web"},` +
		`{"verdict":{"verdict":"accept"}}]}}`)
	a := fwrAdapter(func(string) ([]json.RawMessage, bool, error) { return wrong, false, nil })
	if err := a.Verify(t.Context(), fwrCandidate(t, lanWeb())); err == nil {
		t.Fatal("diverging kernel rule must fail verification (count-only check would pass)")
	}
}

func TestFwrulesVerifyEmptyCandidateRequiresAbsentTable(t *testing.T) {
	a := fwrAdapter(func(string) ([]json.RawMessage, bool, error) { return nil, true, nil })
	if err := a.Verify(t.Context(), fwrCandidate(t)); err != nil {
		t.Fatalf("absent table with empty candidate must verify: %v", err)
	}
	stillThere := fwrAdapter(func(string) ([]json.RawMessage, bool, error) { return fwrSnapshot(), false, nil })
	if err := stillThere.Verify(t.Context(), fwrCandidate(t)); err == nil {
		t.Fatal("surviving table with empty candidate must fail verification")
	}
}

func TestFwrulesVerifyReadbackErrorIsNeverEmptySuccess(t *testing.T) {
	a := fwrAdapter(func(string) ([]json.RawMessage, bool, error) {
		return nil, false, errors.New("nft exited 1: permission denied")
	})
	if err := a.Verify(t.Context(), fwrCandidate(t)); err == nil || !strings.Contains(err.Error(), "readback error") {
		t.Fatalf("readback error must fail closed with context, got %v", err)
	}
	// The same error with an empty candidate: still an error —
	// a failed observation must never decode to "empty state".
	if err := a.Verify(t.Context(), fwrCandidate(t)); err == nil {
		t.Fatal("observation failure must not verify as empty")
	}
}

func TestFwrulesVerifyAbsentTableWithRulesFails(t *testing.T) {
	a := fwrAdapter(func(string) ([]json.RawMessage, bool, error) { return nil, true, nil })
	err := a.Verify(t.Context(), fwrCandidate(t, lanWeb()))
	if err == nil {
		t.Fatal("absent table with non-empty candidate must fail")
	}
}
