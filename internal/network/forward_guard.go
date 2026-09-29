package network

import (
	"context"
	"encoding/json"
	"errors"
)

const forwardGuardName = "duckos_apply_guard"
const forwardGuardChain = `{"chain":{"family":"inet","table":"duckos","name":"duckos_apply_guard","type":"filter","hook":"forward","prio":-10,"policy":"drop"}}`
const forwardGuardBatch = "add table inet duckos\nadd chain inet duckos duckos_apply_guard { type filter hook forward priority -10; policy drop; }\n"

// guardSnapshot preserves the enrolled table's input/output rules and adds an
// unconditional forward barrier. A pre-existing reserved name means recovery
// or a conflict, not permission to adopt the object.
//
// The guard chain is a chain and nft --json list table groups every chain
// together (in handle order) before the rules. The recorded snapshot must
// match that exact shape or NFTSnapshot.Fingerprint of `guarded` will never
// equal NFTSnapshot.Fingerprint of the live nft readback — which is the
// witness-side disagreement that rolls the second Safe Apply back. The
// barrier is therefore inserted at the boundary between the last chain and
// the first rule, which is also where nft places it after the Apply.
func guardSnapshot(before NFTSnapshot) (NFTSnapshot, error) {
	if err := before.Validate(); err != nil {
		return NFTSnapshot{}, err
	}
	guarded := NFTSnapshot{Objects: []json.RawMessage{}}
	for _, raw := range before.Objects {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return NFTSnapshot{}, errors.New("invalid guard before-state")
		}
		for kind, body := range object {
			var item struct {
				Name  string `json:"name"`
				Chain string `json:"chain"`
			}
			if json.Unmarshal(body, &item) != nil || kind == "chain" && item.Name == forwardGuardName || kind == "rule" && item.Chain == forwardGuardName {
				return NFTSnapshot{}, errors.New("reserved forwarding guard requires recovery")
			}
		}
		guarded.Objects = append(guarded.Objects, append(json.RawMessage(nil), raw...))
	}
	hasTable := false
	for _, raw := range guarded.Objects {
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) == nil {
			if _, ok := probe["table"]; ok {
				hasTable = true
				break
			}
		}
	}
	if !hasTable {
		guarded.Objects = append([]json.RawMessage{json.RawMessage(`{"table":{"family":"inet","name":"duckos"}}`)}, guarded.Objects...)
	}
	// Insert the guard chain at the end of the chain group (just before the
	// first rule), matching the position nft --json list table reports. The
	// previous implementation appended at the very end, which made the
	// recorded snapshot disagree with the live readback on every Safe Apply
	// after the first one and forced an unnecessary rollback.
	insertAt := len(guarded.Objects)
	for i, raw := range guarded.Objects {
		var probe map[string]json.RawMessage
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		if _, ok := probe["rule"]; ok {
			insertAt = i
			break
		}
	}
	guarded.Objects = append(guarded.Objects[:insertAt], append([]json.RawMessage{json.RawMessage(forwardGuardChain)}, guarded.Objects[insertAt:]...)...)
	return guarded, guarded.Validate()
}

// verifyForwardGuard rejects any rule in the barrier chain, including an accept
// inserted before its drop policy. Handles are kernel identity, not semantics.
func verifyForwardGuard(s NFTSnapshot) error {
	if err := s.Validate(); err != nil {
		return err
	}
	count := 0
	for _, raw := range s.Objects {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return errors.New("invalid guarded snapshot")
		}
		if body, ok := object["rule"]; ok {
			var rule struct {
				Chain string `json:"chain"`
			}
			if json.Unmarshal(body, &rule) != nil || rule.Chain == forwardGuardName {
				return errors.New("forwarding guard contains unexpected rules")
			}
		}
		body, ok := object["chain"]
		if !ok {
			continue
		}
		var chain struct {
			Name, Family, Table, Type, Hook, Policy string
			Priority                                int `json:"prio"`
		}
		if json.Unmarshal(body, &chain) != nil {
			return errors.New("invalid guard chain")
		}
		if chain.Name != forwardGuardName {
			continue
		}
		count++
		if chain.Family != "inet" || chain.Table != "duckos" || chain.Type != "filter" || chain.Hook != "forward" || chain.Priority != -10 || chain.Policy != "drop" {
			return errors.New("forwarding barrier semantics differ")
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil {
			return errors.New("invalid guard chain fields")
		}
		for key := range fields {
			switch key {
			case "name", "family", "table", "type", "hook", "prio", "policy", "handle":
			default:
				return errors.New("unsupported forwarding barrier attribute")
			}
		}
	}
	if count != 1 {
		return errors.New("exactly one forwarding barrier is required")
	}
	return nil
}

// InstallForwardGuard requires durable before-state and exclusive enrolled
// firewall ownership from its caller. The batch is atomic within inet/duckos.
// It does not attest absence of external flow offload or bypass packet paths.
func (h *FirewallHost) InstallForwardGuard(ctx context.Context, before NFTSnapshot) (NFTSnapshot, error) {
	guarded, err := guardSnapshot(before)
	if err != nil {
		return NFTSnapshot{}, err
	}
	if err := h.Verify(ctx, before); err != nil {
		return NFTSnapshot{}, err
	}
	if err := h.Restore(ctx, guarded); err != nil {
		return NFTSnapshot{}, err
	}
	return guarded, nil
}

// ApplyGuarded atomically changes managed filtering/NAT while retaining the
// forwarding barrier. Releasing it is a separate normal Apply after composite
// route/link/DNS verification; the controller must not confirm while it remains.
func (h *FirewallHost) ApplyGuarded(ctx context.Context, c Config, before NFTSnapshot) (NFTSnapshot, error) {
	if err := verifyForwardGuard(before); err != nil {
		return NFTSnapshot{}, err
	}
	return h.apply(ctx, c, before, true)
}

// ReleaseForwardGuard removes the duckos_apply_guard chain the Safe Apply
// Capture stage installed. It is idempotent and safe to call from both
// the success path (after a confirmed Apply) and the rollback path
// (after RestoreCaptured). It refuses to release a barrier that holds
// any non-drop rule — that would re-enable forwarding mid-apply and is
// never what the caller asked for. The next Safe Apply starts with a
// clean table so the kernel.Capture stage never trips
// "reserved forwarding guard requires recovery" on a previous
// failure's residue.
func (h *FirewallHost) ReleaseForwardGuard(ctx context.Context) error {
	if h == nil {
		return errors.New("firewall host is not wired")
	}
	obs, err := h.Capture(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, raw := range obs.Objects {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			return errors.New("invalid live guard envelope")
		}
		for kind, body := range object {
			if kind == "chain" {
				var chain struct{ Name string }
				if json.Unmarshal(body, &chain) != nil {
					return errors.New("invalid guard chain")
				}
				found = found || chain.Name == forwardGuardName
			}
			if kind != "rule" {
				continue
			}
			var item struct {
				Chain string `json:"chain"`
			}
			if json.Unmarshal(body, &item) != nil {
				return errors.New("invalid guard rule envelope")
			}
			if item.Chain == forwardGuardName {
				return errors.New("cannot release guard that still holds non-drop rules")
			}
		}
	}
	// Restore replaces the whole owned table, so the ordinary rollback
	// already removed the barrier. Absence must be a true no-op.
	if !found {
		return nil
	}
	if err := verifyForwardGuard(obs); err != nil {
		return err
	}
	// This is a fixed chain operation, not a chain-only table snapshot.
	if _, err := h.run(ctx, []string{"--json", "--file", "-"}, []byte(`{"nftables":[{"delete":{"chain":{"family":"inet","table":"duckos","name":"duckos_apply_guard"}}}]}`)); err != nil {
		return errors.New("forwarding guard release outcome uncertain")
	}
	want := NFTSnapshot{}
	for _, raw := range obs.Objects {
		var object map[string]json.RawMessage
		_ = json.Unmarshal(raw, &object)
		// Only chain objects can carry the reserved guard name. Rules and
		// tables never match; the previous implementation leaked `chain`
		// across iterations and accidentally filtered every rule out of the
		// post-release snapshot, which then failed Verify against live.
		if body, ok := object["chain"]; ok {
			var chain struct{ Name string }
			if json.Unmarshal(body, &chain) == nil && chain.Name == forwardGuardName {
				continue
			}
		}
		want.Objects = append(want.Objects, raw)
	}
	return h.Verify(ctx, want)
}
