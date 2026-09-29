package network

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestForwardGuardPreservesOwnedInputAndRejectsCollisions(t *testing.T) {
	before, err := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	if err != nil {
		t.Fatal(err)
	}
	guarded, err := guardSnapshot(before)
	if err != nil || verifyForwardGuard(guarded) != nil || len(guarded.Objects) != len(before.Objects)+1 {
		t.Fatal(guarded, err)
	}
	// Every original object must still appear in the guarded snapshot, and the
	// guard chain must be the only addition. The guard chain is inserted at
	// the boundary between the last chain and the first rule so the recorded
	// fingerprint agrees with real nft --json list table output, so the
	// before.Objects indices in guarded.Objects shift by at most one when
	// the guard is inserted before a rule.
	guardedRaw := map[string]struct{}{}
	for _, raw := range guarded.Objects {
		guardedRaw[string(raw)] = struct{}{}
	}
	for _, raw := range before.Objects {
		if _, ok := guardedRaw[string(raw)]; !ok {
			t.Fatal("management chain lost from guarded snapshot")
		}
	}
	guardCount := 0
	for _, raw := range guarded.Objects {
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		var chain struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(probe["chain"], &chain) == nil && chain.Name == forwardGuardName {
			guardCount++
		}
	}
	if guardCount != 1 {
		t.Fatalf("expected exactly one guard chain in guarded snapshot, found %d", guardCount)
	}
	guarded.Objects[0][0] = ' '
	if !json.Valid(before.Objects[0]) {
		t.Fatal("guard snapshot aliases journal memory")
	}
	guarded, _ = guardSnapshot(before)
	if _, err := guardSnapshot(guarded); err == nil {
		t.Fatal("existing barrier adopted without recovery")
	}
	for _, replacement := range []string{strings.Replace(forwardGuardChain, `"policy":"drop"`, `"policy":"accept"`, 1), strings.Replace(forwardGuardChain, `"hook":"forward"`, `"hook":"input"`, 1), strings.Replace(forwardGuardChain, `"prio":-10`, `"prio":0`, 1), strings.Replace(forwardGuardChain, `"prio":-10`, `"prio":-10,"device":"outside"`, 1)} {
		bad := NFTSnapshot{Objects: append([]json.RawMessage(nil), guarded.Objects...)}
		bad.Objects[len(bad.Objects)-1] = json.RawMessage(replacement)
		if verifyForwardGuard(bad) == nil {
			t.Fatal("invalid barrier accepted")
		}
	}
	guarded.Objects = append(guarded.Objects, json.RawMessage(`{"rule":{"family":"inet","table":"duckos","chain":"duckos_apply_guard","expr":[{"accept":null}]}}`))
	if verifyForwardGuard(guarded) == nil {
		t.Fatal("barrier with accept rule trusted")
	}
	empty, err := guardSnapshot(NFTSnapshot{})
	if err != nil || len(empty.Objects) != 2 || verifyForwardGuard(empty) != nil {
		t.Fatal("empty initial state cannot be guarded", err)
	}
}

func rawEqual(a, b json.RawMessage) bool {
	var am, bm map[string]json.RawMessage
	if json.Unmarshal(a, &am) != nil || json.Unmarshal(b, &bm) != nil {
		return string(a) == string(b)
	}
	if len(am) != len(bm) {
		return false
	}
	for k, v := range am {
		if string(v) != string(bm[k]) {
			return false
		}
	}
	return true
}

func TestForwardGuardRealExecutorBoundary(t *testing.T) {
	before, _ := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	live := before
	mutations := 0
	corrupt, fail := false, false
	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--json list tables":
			if len(live.Objects) == 0 {
				return []byte(`{"nftables":[]}`), nil
			}
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			return json.Marshal(map[string]any{"nftables": live.Objects})
		case "--json --check --file -", "--check --file -":
			return nil, nil
		case "--json --file -":
			var batch struct {
				NFTables []map[string]json.RawMessage `json:"nftables"`
			}
			if json.Unmarshal(input, &batch) != nil {
				t.Fatal("invalid recovery JSON")
			}
			live = NFTSnapshot{Objects: []json.RawMessage{}}
			for _, command := range batch.NFTables {
				if object, ok := command["add"]; ok {
					live.Objects = append(live.Objects, object)
				}
				if object, ok := command["delete"]; ok {
					keep := live.Objects[:0]
					for _, existing := range live.Objects {
						if !rawEqual(existing, object) {
							keep = append(keep, existing)
						}
					}
					live.Objects = keep
				}
			}
		case "--file -":
			if !strings.HasSuffix(string(input), forwardGuardBatch) {
				t.Fatal("unguarded candidate batch")
			}
			if strings.Contains(string(input), "chain input") {
				live, _ = guardSnapshot(before)
			} else {
				live, _ = guardSnapshot(NFTSnapshot{})
			}
			if corrupt {
				live.Objects[len(live.Objects)-1] = json.RawMessage(strings.Replace(forwardGuardChain, `"policy":"drop"`, `"policy":"accept"`, 1))
			}
		default:
			t.Fatal("unexpected guard command", args)
		}
		mutations++
		if fail {
			return nil, errors.New("synthetic lost acknowledgement")
		}
		return nil, nil
	}}
	ctx := context.Background()
	guarded, err := h.InstallForwardGuard(ctx, before)
	if err != nil || mutations != 1 || h.Verify(ctx, guarded) != nil {
		t.Fatal("guard installation failed", err)
	}
	updated, err := h.ApplyGuarded(ctx, fixture(), guarded)
	if err != nil || verifyForwardGuard(updated) != nil {
		t.Fatal("candidate lost guard", err)
	}
	count := mutations
	if _, err := h.ApplyGuarded(ctx, fixture(), before); err == nil || mutations != count {
		t.Fatal("unguarded before-state admitted")
	}
	updated, err = h.ApplyGuarded(ctx, Config{Version: 1}, updated)
	if err != nil || len(updated.Objects) != 2 || verifyForwardGuard(updated) != nil {
		t.Fatal("removal released guard prematurely", err)
	}
	corrupt = true
	if _, err := h.ApplyGuarded(ctx, fixture(), updated); err == nil {
		t.Fatal("wrong kernel guard reported success")
	}
	corrupt = false
	if err := h.Restore(ctx, before); err != nil {
		t.Fatal(err)
	}
	fail = true
	if _, err := h.InstallForwardGuard(ctx, before); err == nil {
		t.Fatal("lost acknowledgement reported success")
	}
	fail = false
	if err := h.Restore(ctx, before); err != nil {
		t.Fatal("lost guard acknowledgement not recoverable", err)
	}
}

// Exercise the production host method with a stateful executor double.
// Kernel semantics are covered separately in a disposable Linux namespace.
func TestReleaseForwardGuardRemovesChainWithoutDestroyingTable(t *testing.T) {
	before, err := DecodeNFTSnapshot([]byte(syntheticNFTTable))
	if err != nil {
		t.Fatal(err)
	}
	live, err := guardSnapshot(before)
	if err != nil {
		t.Fatal(err)
	}
	deletes := 0
	h := &FirewallHost{run: func(_ context.Context, args []string, input []byte) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--json list tables":
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			return json.Marshal(map[string]any{"nftables": live.Objects})
		case "--json --file -":
			if !strings.Contains(string(input), `"delete":{"chain":`) || strings.Contains(string(input), `"destroy"`) {
				t.Fatalf("unsafe delete: %s", input)
			}
			if deletes > 0 {
				return nil, errors.New("chain no longer exists")
			}
			deletes++
			live = before
			return nil, nil
		}
		t.Fatalf("unexpected operation: %v", args)
		return nil, nil
	}}
	for i := 0; i < 2; i++ {
		if err := h.ReleaseForwardGuard(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if deletes != 1 {
		t.Fatalf("delete count %d", deletes)
	}
	if err := h.Verify(context.Background(), before); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseForwardGuardRefusesWhenGuardHoldsNonDropRules pins the
// refusal half of Blocker 3: ReleaseForwardGuard must not delete a
// barrier that still owns any non-drop rule, because that would
// re-enable forwarding mid-apply.
func TestReleaseForwardGuardRefusesWhenGuardHoldsNonDropRules(t *testing.T) {
	corrupt := NFTSnapshot{Objects: []json.RawMessage{
		json.RawMessage(`{"table":{"family":"inet","name":"duckos"}}`),
		json.RawMessage(`{"chain":{"family":"inet","table":"duckos","name":"` + forwardGuardName + `","type":"filter","hook":"forward","prio":-10,"policy":"drop"}}`),
		json.RawMessage(`{"rule":{"family":"inet","table":"duckos","chain":"` + forwardGuardName + `","expr":[{"accept":null}]}}`),
	}}
	live := corrupt
	h := &FirewallHost{run: func(_ context.Context, args []string, _ []byte) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "--json list tables":
			return []byte(`{"nftables":[{"table":{"family":"inet","name":"duckos"}}]}`), nil
		case "--json list table inet duckos":
			return json.Marshal(map[string]any{"nftables": live.Objects})
		}
		return nil, nil
	}}
	if err := h.ReleaseForwardGuard(context.Background()); err == nil {
		t.Fatal("ReleaseForwardGuard must refuse when the barrier holds a non-drop rule")
	}
}
