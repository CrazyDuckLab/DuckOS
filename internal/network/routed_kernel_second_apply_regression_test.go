package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// unwrapDeletePayload returns the inner chain/rule payload of a delete
// command so the fake executor can compare it against owned objects.
func unwrapDeletePayload(raw json.RawMessage) json.RawMessage {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return raw
	}
	for _, payload := range envelope {
		return payload
	}
	return raw
}

// matchesDelete returns true if `existing` (an owned object envelope of the
// form {"<kind>":{<body>}}) is the object addressed by a delete command
// payload. The delete payload is the inner body — {"family":..., "table":...}
// for a chain delete, or {"family":..., "table":..., "chain":...} for a rule
// delete — so we compare it against the inner body of the envelope. nft
// matches delete addresses by name (and family/table); the delete payload is
// a subset of the object's body, so we treat a match as every key in the
// payload being equal in the owned body.
func matchesDelete(existing json.RawMessage, target json.RawMessage) bool {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(existing, &envelope) != nil {
		return false
	}
	for _, payload := range envelope {
		var owned, want map[string]any
		if json.Unmarshal(payload, &owned) != nil || json.Unmarshal(target, &want) != nil {
			return false
		}
		for k, v := range want {
			got, ok := owned[k]
			if !ok {
				return false
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", v) {
				return false
			}
		}
		return true
	}
	return false
}

// TestSecondApplyFingerprintAfterFirstCommit reproduces the exact failure
// observed on the witness VM.
//
// Evidence (recovered from /var/lib/duckos-agent/network/state.json):
//   - First commit's agent journal shows the second Apply's Capture snapshot
//     containing table / chains / rules but NO duckos_apply_guard chain.
//   - kernel.Apply errored at firewall.Verify(ctx, guard) with
//     "managed firewall differs from recorded kernel state".
//
// Root cause (this test pins it): guardSnapshot appends the guard chain at
// the end of the snapshot's Objects slice. Real nft --json list table groups
// chains before rules; the guard chain is a chain, so it sits among the
// chains in the listing, not after the rules. The kernel records `guard`
// (chain at end) and compares it against live nft (chain in the chains
// group). Fingerprints disagree and the transaction rolls back.
//
// The fake executor simulates production nft behaviour: `--json list table
// inet duckos` returns chains sorted by handle then rules sorted by handle,
// so the recovered witness output shape is faithfully reproduced.
//
// The test must FAIL on the current HEAD (the bug is present). It must
// PASS once the lifecycle is fixed.
func TestSecondApplyFingerprintAfterFirstCommit(t *testing.T) {
	ctx := context.Background()
	candidate := fixture()
	beforeSnapshot, err := DecodeNFTSnapshot([]byte(firstCommittedSnapshot))
	if err != nil {
		t.Fatalf("decode first committed snapshot: %v", err)
	}

	live := beforeSnapshot
	captured := []string{}

	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--json list tables":
			if len(live.Objects) == 0 {
				return []byte(`{"nftables":[]}`), nil
			}
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			// Production nft returns chains grouped together (sorted by handle)
			// and rules grouped afterwards (sorted by handle). The guard chain
			// is a chain, so it sits among the other chains regardless of the
			// order it was added to the batch.
			return listInHandleOrder(live), nil
		case "--json --check --file -", "--check --file -":
			return nil, nil
		case "--json --file -":
			var batch struct {
				NFTables []map[string]json.RawMessage `json:"nftables"`
			}
			if json.Unmarshal(input, &batch) != nil {
				return nil, fmt.Errorf("invalid recovery JSON")
			}
			// RestoreBatch emits destroy+add and resets the table; a pure delete
			// (ReleaseForwardGuard) must leave existing live state alone.
			hasDestroy := false
			for _, command := range batch.NFTables {
				if object, ok := command["destroy"]; ok && len(object) > 0 {
					var probe map[string]json.RawMessage
					if json.Unmarshal(object, &probe) == nil {
						if _, isTable := probe["table"]; isTable {
							hasDestroy = true
						}
					}
				}
			}
			if hasDestroy {
				live = NFTSnapshot{Objects: []json.RawMessage{}}
			}
			for _, command := range batch.NFTables {
				if object, ok := command["add"]; ok {
					live.Objects = append(live.Objects, object)
				}
				if object, ok := command["delete"]; ok {
					target := unwrapDeletePayload(object)
					keep := live.Objects[:0]
					for _, existing := range live.Objects {
						if matchesDelete(existing, target) {
							continue
						}
						keep = append(keep, existing)
					}
					live.Objects = keep
				}
			}
		case "--file -":
			captured = append(captured, string(input))
			if !strings.HasSuffix(string(input), forwardGuardBatch) {
				return nil, fmt.Errorf("unguarded candidate batch")
			}
			// The candidate batch is `destroy table + table { ... } + add table
			// + add chain guard`. In real nft the post-state is the candidate's
			// render plus the guard chain. For this regression we drive a
			// second Apply whose candidate renders to the same rules as the
			// captured before, so the post-state equals guardSnapshot(before).
			newLive, err := guardSnapshot(beforeSnapshot)
			if err != nil {
				return nil, err
			}
			live = newLive
		default:
			return nil, fmt.Errorf("unexpected op: %v", args)
		}
		return nil, nil
	}}

	// ---- Phase 1: Capture(before) on the second Apply ----
	firstCapture, err := h.Capture(ctx)
	if err != nil {
		t.Fatalf("first Capture: %v", err)
	}
	if verifyForwardGuard(firstCapture) == nil {
		t.Fatalf("first Capture unexpectedly contained the guard chain")
	}
	fpFirstCapture := fingerprintOf(t, firstCapture)

	// ---- Phase 2: InstallForwardGuard(before) ----
	guarded, err := h.InstallForwardGuard(ctx, firstCapture)
	if err != nil {
		t.Fatalf("InstallForwardGuard: %v", err)
	}
	if verifyForwardGuard(guarded) != nil {
		t.Fatalf("InstallForwardGuard produced no guard")
	}
	if !hasGuardChain(guarded) {
		t.Fatalf("InstallForwardGuard returned snapshot without guard")
	}
	liveAfterInstall := captureOf(t, h)
	if !hasGuardChain(liveAfterInstall) {
		t.Fatalf("InstallForwardGuard lost the guard chain from live state")
	}
	if fingerprintOf(t, liveAfterInstall) == fpFirstCapture {
		t.Fatalf("InstallForwardGuard did not change the live fingerprint (guard not installed)")
	}
	// The recorded `guarded` snapshot and the live nft state must agree on
	// fingerprint, otherwise the next phase's Apply cannot end with
	// firewall.Verify(ctx, guard) succeeding.
	if err := h.Verify(ctx, guarded); err != nil {
		fpGuarded := fingerprintOf(t, guarded)
		fpLive := fingerprintOf(t, liveAfterInstall)
		t.Fatalf("InstallForwardGuard's recorded guard fingerprint disagrees with live nft:\n  guarded (recorded) = %s\n  live (nft)        = %s", fpGuarded, fpLive)
	}

	// ---- Phase 3: ApplyGuarded(candidate, guarded) ----
	postApply, err := h.ApplyGuarded(ctx, candidate, guarded)
	if err != nil {
		t.Fatalf("ApplyGuarded: %v", err)
	}
	if verifyForwardGuard(postApply) != nil {
		t.Fatalf("ApplyGuarded produced no guard")
	}
	if !hasGuardChain(postApply) {
		t.Fatalf("ApplyGuarded returned snapshot without guard")
	}
	liveAfterApply := captureOf(t, h)
	if !hasGuardChain(liveAfterApply) {
		t.Fatalf("ApplyGuarded did not install the guard chain in live")
	}

	// ---- Phase 4: firewall.Verify(ctx, guard) — the failure point on the VM ----
	if err := h.Verify(ctx, postApply); err != nil {
		fpPostApply := fingerprintOf(t, postApply)
		fpLive := fingerprintOf(t, liveAfterApply)
		t.Fatalf("second Apply firewall.Verify mismatch (this is the live VM failure):\n  recorded fingerprint = %s\n  live fingerprint     = %s\n  Apply ran %d --file - call(s)", fpPostApply, fpLive, len(captured))
	}
}

// TestReleaseForwardGuardAfterFirstApplyLifecycle is the second piece of the
// regression: after a successful ApplyGuarded and a ReleaseForwardGuard, the
// live state must equal the captured before-snapshot (modulo the guard
// chain), so the next Capture can produce a snapshot that the next Apply's
// verifyForwardGuard accepts and that the next Apply's final Verify
// succeeds.
func TestReleaseForwardGuardAfterFirstApplyLifecycle(t *testing.T) {
	ctx := context.Background()
	candidate := fixture()
	beforeSnapshot, err := DecodeNFTSnapshot([]byte(firstCommittedSnapshot))
	if err != nil {
		t.Fatalf("decode first committed snapshot: %v", err)
	}

	live := beforeSnapshot
	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--json list tables":
			if len(live.Objects) == 0 {
				return []byte(`{"nftables":[]}`), nil
			}
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			return listInHandleOrder(live), nil
		case "--json --check --file -", "--check --file -":
			return nil, nil
		case "--json --file -":
			var batch struct {
				NFTables []map[string]json.RawMessage `json:"nftables"`
			}
			if json.Unmarshal(input, &batch) != nil {
				return nil, fmt.Errorf("invalid recovery JSON")
			}
			hasDestroy := false
			for _, command := range batch.NFTables {
				if object, ok := command["destroy"]; ok && len(object) > 0 {
					var probe map[string]json.RawMessage
					if json.Unmarshal(object, &probe) == nil {
						if _, isTable := probe["table"]; isTable {
							hasDestroy = true
						}
					}
				}
			}
			if hasDestroy {
				live = NFTSnapshot{Objects: []json.RawMessage{}}
			}
			for _, command := range batch.NFTables {
				if object, ok := command["add"]; ok {
					live.Objects = append(live.Objects, object)
				}
				if object, ok := command["delete"]; ok {
					target := unwrapDeletePayload(object)
					keep := live.Objects[:0]
					for _, existing := range live.Objects {
						if matchesDelete(existing, target) {
							continue
						}
						keep = append(keep, existing)
					}
					live.Objects = keep
				}
			}
		case "--file -":
			// See TestSecondApplyFingerprintAfterFirstCommit: real nft applies
			// the candidate batch (destroy + table + add chain) so the post
			// state equals guardSnapshot(beforeSnapshot) for our regression
			// candidate.
			newLive, err := guardSnapshot(beforeSnapshot)
			if err != nil {
				return nil, err
			}
			live = newLive
		}
		return nil, nil
	}}

	firstCapture, err := h.Capture(ctx)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	guarded, err := h.InstallForwardGuard(ctx, firstCapture)
	if err != nil {
		t.Fatalf("InstallForwardGuard: %v", err)
	}
	if _, err := h.ApplyGuarded(ctx, candidate, guarded); err != nil {
		t.Fatalf("ApplyGuarded: %v", err)
	}
	if err := h.ReleaseForwardGuard(ctx); err != nil {
		t.Fatalf("ReleaseForwardGuard: %v", err)
	}

	postRelease, err := h.Capture(ctx)
	if err != nil {
		t.Fatalf("post-release Capture: %v", err)
	}
	if verifyForwardGuard(postRelease) == nil {
		t.Fatalf("guard chain still present after ReleaseForwardGuard")
	}
	if err := h.Verify(ctx, firstCapture); err != nil {
		t.Fatalf("post-release live != first committed state: %v", err)
	}
}

// TestGuardSnapshotOrdering is the focused unit test for the lifecycle
// defect. It does not exercise the firewall host at all — it directly
// verifies that guardSnapshot places the guard chain in the position that
// real nft places it (chains group, before rules group). This is the
// targeted fix-spec test the fix has to satisfy.
func TestGuardSnapshotOrdering(t *testing.T) {
	before, err := DecodeNFTSnapshot([]byte(firstCommittedSnapshot))
	if err != nil {
		t.Fatalf("decode first committed snapshot: %v", err)
	}

	guarded, err := guardSnapshot(before)
	if err != nil {
		t.Fatalf("guardSnapshot: %v", err)
	}

	if !hasGuardChain(guarded) {
		t.Fatalf("guard chain missing from guarded snapshot")
	}

	// Find the first rule and the last chain in the guarded snapshot.
	firstRuleIndex := -1
	lastChainIndex := -1
	for i, raw := range guarded.Objects {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			t.Fatalf("invalid guarded object at %d: %v", i, err)
		}
		switch {
		case probe["chain"] != nil:
			lastChainIndex = i
		case probe["rule"] != nil:
			if firstRuleIndex == -1 {
				firstRuleIndex = i
			}
		}
	}

	if firstRuleIndex == -1 {
		t.Fatalf("guarded snapshot has no rules")
	}
	if lastChainIndex == -1 {
		t.Fatalf("guarded snapshot has no chains")
	}

	// The guard chain must live among the chains (lastChainIndex), not after
	// the rules (firstRuleIndex). Production nft --json list table returns
	// chains grouped before rules; the recorded snapshot must match that
	// shape or its fingerprint will never equal the live fingerprint.
	if lastChainIndex >= firstRuleIndex {
		t.Fatalf("guard chain ended up after the first rule: lastChainIndex=%d firstRuleIndex=%d (rules and chains must be split so chains are before rules)", lastChainIndex, firstRuleIndex)
	}

	// The guard chain is the last chain; verify the chain at lastChainIndex
	// is in fact the guard.
	lastChainRaw := guarded.Objects[lastChainIndex]
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(lastChainRaw, &probe); err != nil {
		t.Fatalf("invalid last chain: %v", err)
	}
	var chain struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(probe["chain"], &chain); err != nil {
		t.Fatalf("invalid chain body: %v", err)
	}
	if chain.Name != forwardGuardName {
		t.Fatalf("expected guard chain at lastChainIndex, got %q", chain.Name)
	}
}

// firstCommittedSnapshot mirrors the live nftables state observed on the
// witness VM after the first commit's ReleaseForwardGuard: chains in handle
// order (input=1 forward=2 prerouting=3 postrouting=4), then input rules
// (handles 6..15), then forward rules (16, 17). The guard chain is NOT
// present — this is the post-ReleaseForwardGuard steady state.
const firstCommittedSnapshot = `{"nftables":[{"metainfo":{"json_schema_version":1}},{"table":{"family":"inet","name":"duckos","handle":152}},{"chain":{"family":"inet","table":"duckos","name":"input","handle":1,"type":"filter","hook":"input","prio":0,"policy":"drop"}},{"chain":{"family":"inet","table":"duckos","name":"forward","handle":2,"type":"filter","hook":"forward","prio":0,"policy":"drop"}},{"chain":{"family":"inet","table":"duckos","name":"prerouting","handle":3,"type":"nat","hook":"prerouting","prio":-100,"policy":"accept"}},{"chain":{"family":"inet","table":"duckos","name":"postrouting","handle":4,"type":"nat","hook":"postrouting","prio":100,"policy":"accept"}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":6,"expr":[{"match":{"op":"!=","left":{"meta":{"key":"iifname"}},"right":{"set":["eth1","eth2"]}}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":7,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"lo"}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":8,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"eth0"}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":9,"expr":[{"ct":{"state":{"key":"state"},"dir":"src","val":["established","related"]}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":10,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"eth2"}},{"match":{"op":"==","left":{"meta":{"l4proto":"tcp"}},"right":"tcp"}},{"tcp":{"dport":{"set":[22,443]}}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":11,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"eth2"}},{"match":{"op":"==","left":{"meta":{"l4proto":"udp"}},"right":"udp"}},{"udp":{"dport":{"set":[53]}}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":13,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"eth2"}},{"match":{"op":"==","left":{"meta":{"l4proto":"tcp"}},"right":"tcp"}},{"tcp":{"dport":{"set":[53]}}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":14,"expr":[{"counter":null},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"input","handle":15,"expr":[{"match":{"op":"==","left":{"meta":{"l4proto":"icmp"}},"right":"icmp"}},{"counter":null},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"forward","handle":16,"expr":[{"ct":{"state":{"key":"state"},"dir":"src","val":["established","related"]}},{"accept":null}]}},{"rule":{"family":"inet","table":"duckos","chain":"forward","handle":17,"expr":[{"counter":null},{"drop":null}]}}]}`

// handleAndBody is the per-object representation used by listInHandleOrder.
type handleAndBody struct {
	handle uint64
	body   json.RawMessage
}

// listInHandleOrder serialises the in-memory snapshot the same way production
// nft does: table first, then chains sorted by handle, then rules sorted by
// handle. The guard chain — when present — sits among the chains in handle
// order, regardless of the order the apply batch added it.
func listInHandleOrder(live NFTSnapshot) []byte {
	var tableObj *handleAndBody
	var chains []handleAndBody
	var rules []handleAndBody
	for _, raw := range live.Objects {
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		kind := ""
		for k := range probe {
			kind = k
		}
		entry := handleAndBody{handle: readHandle(probe[kind]), body: raw}
		switch kind {
		case "table":
			tableObj = &entry
		case "chain":
			chains = append(chains, entry)
		case "rule":
			rules = append(rules, entry)
		}
	}
	sortByHandle(chains)
	sortByHandle(rules)
	objects := []json.RawMessage{}
	if tableObj != nil {
		objects = append(objects, tableObj.body)
	}
	for _, c := range chains {
		objects = append(objects, c.body)
	}
	for _, r := range rules {
		objects = append(objects, r.body)
	}
	out, _ := json.Marshal(map[string]any{"nftables": objects})
	return out
}

func readHandle(raw json.RawMessage) uint64 {
	var inner map[string]any
	if json.Unmarshal(raw, &inner) != nil {
		return 0
	}
	switch v := inner["handle"].(type) {
	case float64:
		return uint64(v)
	case int:
		return uint64(v)
	case uint64:
		return v
	}
	return 0
}

func sortByHandle(s []handleAndBody) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].handle > s[j].handle; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func hasGuardChain(snap NFTSnapshot) bool {
	for _, raw := range snap.Objects {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		chainRaw, ok := probe["chain"]
		if !ok {
			continue
		}
		var chain struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(chainRaw, &chain) == nil && chain.Name == forwardGuardName {
			return true
		}
	}
	return false
}

func captureOf(t *testing.T, h *FirewallHost) NFTSnapshot {
	t.Helper()
	snap, err := h.Capture(context.Background())
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	return snap
}

func fingerprintOf(t *testing.T, snap NFTSnapshot) string {
	t.Helper()
	fp, err := snap.Fingerprint()
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return fp
}