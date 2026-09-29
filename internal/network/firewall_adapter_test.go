package network

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

type syntheticFirewallKernel struct {
	live         NFTSnapshot
	applyError   bool
	restoreError bool
	applies      int
	releases     int
}

func (h *syntheticFirewallKernel) Check(context.Context, Config) error          { return nil }
func (h *syntheticFirewallKernel) Capture(context.Context) (NFTSnapshot, error) { return h.live, nil }
func (h *syntheticFirewallKernel) Apply(_ context.Context, _ Config, _ NFTSnapshot) (NFTSnapshot, error) {
	h.applies++
	h.live, _ = DecodeNFTSnapshot([]byte(syntheticNFTTable))
	if h.applyError {
		return NFTSnapshot{}, errors.New("synthetic lost kernel acknowledgement")
	}
	return h.live, nil
}
func (h *syntheticFirewallKernel) Verify(_ context.Context, want NFTSnapshot) error {
	a, _ := h.live.Fingerprint()
	b, _ := want.Fingerprint()
	if a != b {
		return errors.New("synthetic drift")
	}
	return nil
}
func (h *syntheticFirewallKernel) Restore(_ context.Context, want NFTSnapshot) error {
	if h.restoreError {
		return errors.New("synthetic restore failure")
	}
	h.live = want
	return nil
}
func (h *syntheticFirewallKernel) Release(_ context.Context, target NFTSnapshot) error {
	h.releases++
	if len(target.Objects) == 0 {
		return nil
	}
	// Mirror the real kernel host: drop the targeted objects from
	// the synthetic live snapshot.
	next := NFTSnapshot{}
	for _, raw := range h.live.Objects {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) != nil {
			continue
		}
		drop := false
		for _, targetRaw := range target.Objects {
			var tObject map[string]json.RawMessage
			if json.Unmarshal(targetRaw, &tObject) != nil {
				continue
			}
			for kind, body := range tObject {
				if object[kind] != nil && jsonEqual(object[kind], body) {
					drop = true
					break
				}
			}
			if drop {
				break
			}
		}
		if !drop {
			next.Objects = append(next.Objects, raw)
		}
	}
	h.live = next
	return nil
}
func jsonEqual(a, b json.RawMessage) bool { return string(a) == string(b) }

func firewallTestStore(t *testing.T, path string) *state.Store {
	t.Helper()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(fixture())
	s, err := state.Open(path, raw, func(raw json.RawMessage) error { _, err := Decode(raw); return err })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFirewallControllerRestoresUncertainMutation(t *testing.T) {
	for _, restoreFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "restored", true: "recovery-required"}[restoreFailure], func(t *testing.T) {
			h := &syntheticFirewallKernel{applyError: true, restoreError: restoreFailure}
			s := firewallTestStore(t, t.TempDir())
			c, err := safeapply.Open(s, &FirewallAdapter{host: h})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			before, _ := s.Snapshot()
			next := fixture()
			next.Firewall.Forwards[0].Port = 9443
			raw, _ := json.Marshal(next)
			if _, err := c.Apply(before.Current.ID, raw, "owner", "synthetic failure", 2*time.Minute); err == nil {
				t.Fatal("uncertain mutation reported success")
			}
			after, _ := s.Snapshot()
			if after.Current.ID != before.Current.ID || h.applies != 1 {
				t.Fatal("committed or replayed uncertain mutation")
			}
			if restoreFailure {
				if after.Pending == nil || after.Pending.Phase != state.RecoveryRequired {
					t.Fatal("restore failure lost durable recovery gate")
				}
			} else if after.Pending != nil || len(h.live.Objects) != 0 || len(after.Outcomes) != 1 {
				t.Fatal("before-state was not restored")
			}
		})
	}
}

func TestFirewallAdapterRestartUsesDurableEvidence(t *testing.T) {
	path := t.TempDir()
	s := firewallTestStore(t, path)
	h := &syntheticFirewallKernel{}
	a := &FirewallAdapter{host: h}
	before, _ := s.Snapshot()
	next := fixture()
	next.Firewall.Forwards[0].Port = 9443
	raw, _ := json.Marshal(next)
	evidence, err := a.Capture(context.Background(), before.Current.Config, raw)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.PrepareWithRecovery(before.Current.ID, raw, "owner", "synthetic process death", evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := a.Apply(context.Background(), before.Current.Config, raw); err != nil {
		t.Fatal(err)
	}
	// No graceful controller shutdown: model death after mutation, before ack.
	s.Close()
	s = firewallTestStore(t, path)
	c, err := safeapply.Open(s, &FirewallAdapter{host: h})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	after, _ := s.Snapshot()
	if after.Pending != nil || after.Current.ID != before.Current.ID || len(h.live.Objects) != 0 {
		t.Fatal("startup did not restore captured kernel state")
	}
}

func TestFirewallAdapterRefusesUncapturedOrNonFirewallMutation(t *testing.T) {
	h := &syntheticFirewallKernel{}
	a := &FirewallAdapter{host: h}
	before := fixture()
	raw, _ := json.Marshal(before)
	if err := a.Apply(context.Background(), raw, raw); err == nil {
		t.Fatal("uncaptured mutation admitted")
	}
	before.Interfaces[0].MTU = 1400
	next, _ := json.Marshal(before)
	if err := a.Preflight(context.Background(), raw, next); err == nil {
		t.Fatal("link changes silently ignored")
	}
	if h.applies != 0 {
		t.Fatal("invalid request reached kernel")
	}
}
