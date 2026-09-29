package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validator(raw json.RawMessage) error {
	var v struct {
		Value int `json:"value"`
	}
	return decode(raw, &v)
}
func openTest(t *testing.T, path string) *Store {
	t.Helper()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, json.RawMessage(`{"value":0}`), validator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func prepareTest(t *testing.T, s *Store, n int) Transaction {
	t.Helper()
	snap, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.Prepare(snap.Current.ID, json.RawMessage(fmt.Sprintf(`{"value":%d}`, n)), "test-owner", "test change")
	if err != nil {
		t.Fatal(err)
	}
	return tx
}
func TestDurableTransactionAndHistory(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	for i := 1; i <= 40; i++ {
		tx := prepareTest(t, s, i)
		if err := s.Commit(tx.ID); err == nil {
			t.Fatal("committed unverified transaction")
		}
		if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
			t.Fatal(err)
		}
		if err := s.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := s.Commit(tx.ID); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s = openTest(t, path)
	snap, _ := s.Snapshot()
	if string(snap.Current.Config) != `{"value":40}` || len(snap.History) != 32 || len(snap.Outcomes) != 32 || snap.Pending != nil {
		t.Fatalf("incorrect persisted state: %+v", snap)
	}
	if snap.Outcomes[0].Result != "committed" || snap.Outcomes[0].Candidate != snap.Current.ID || snap.Outcomes[0].Before != snap.History[0].ID {
		t.Fatal("commit receipt does not describe committed transition")
	}
}
func TestPendingSurvivesRestartWithoutConfirmation(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	tx := prepareTest(t, s, 1)
	if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, path)
	snap, _ := s.Snapshot()
	if snap.Pending == nil || snap.Pending.Phase != Applying || string(snap.Current.Config) != `{"value":0}` {
		t.Fatal("lost recovery evidence")
	}
	if err := s.Transition(tx.ID, RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RolledBack(tx.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Snapshot()
	if snap.Pending != nil || string(snap.Current.Config) != `{"value":0}` {
		t.Fatal("rollback changed committed state")
	}
	s.Close()
	s = openTest(t, path)
	snap, _ = s.Snapshot()
	if len(snap.Outcomes) != 1 || snap.Outcomes[0].TransactionID != tx.ID || snap.Outcomes[0].Result != "rolled-back" || snap.Outcomes[0].Candidate != tx.Candidate.ID || snap.Outcomes[0].Before != snap.Current.ID {
		t.Fatal("rollback receipt did not survive restart")
	}
}

func TestVersionOneJournalUpgradesOnlyOnMutation(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	snap, _ := s.Snapshot()
	s.Close()
	snap.Version = 1
	payload, _ := json.Marshal(snap)
	data, _ := json.Marshal(envelope{Digest: digest(payload), Payload: payload})
	file := filepath.Join(path, "state.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, path)
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(data) {
		t.Fatal("read rewrote legacy journal", err)
	}
	tx := prepareTest(t, s, 1)
	s.Close()
	s = openTest(t, path)
	snap, _ = s.Snapshot()
	if snap.Version != 2 || snap.Pending == nil || snap.Pending.ID != tx.ID || len(snap.Outcomes) != 0 {
		t.Fatal("upgrade lost transaction state")
	}
}

func TestInvalidOutcomeReceiptRejected(t *testing.T) {
	s := openTest(t, t.TempDir())
	tx := prepareTest(t, s, 1)
	if err := s.Transition(tx.ID, RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RolledBack(tx.ID); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Snapshot){
		"unknown result":    func(v *Snapshot) { v.Outcomes[0].Result = "probably-success" },
		"invalid identity":  func(v *Snapshot) { v.Outcomes[0].TransactionID = "invalid" },
		"missing timestamp": func(v *Snapshot) { v.Outcomes[0].CompletedAt = time.Time{} },
		"duplicate receipt": func(v *Snapshot) { v.Outcomes = append(v.Outcomes, v.Outcomes[0]) },
		"legacy receipt":    func(v *Snapshot) { v.Version = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			v, _ := s.Snapshot()
			change(&v)
			if err := s.check(v); err == nil {
				t.Fatal("malformed receipt accepted")
			}
		})
	}
}
func TestExclusiveWriterAndConcurrencyPreconditions(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	if other, err := Open(path, nil, validator); err == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
	if _, err := s.Prepare("wrong", json.RawMessage(`{"value":1}`), "owner", "reason"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	tx := prepareTest(t, s, 1)
	if _, err := s.Prepare(tx.Before, json.RawMessage(`{"value":2}`), "owner", "reason"); !errors.Is(err, ErrPending) {
		t.Fatal(err)
	}
	if err := s.Transition("wrong", Applying, time.Time{}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(time.Minute)); err == nil {
		t.Fatal("skipped applying")
	}
}
func TestValidationAndIsolation(t *testing.T) {
	s := openTest(t, t.TempDir())
	snap, _ := s.Snapshot()
	for _, raw := range []string{`{"unknown":1}`, `{"value":1} {}`, `{`, `{"value":"x"}`} {
		if _, err := s.Prepare(snap.Current.ID, json.RawMessage(raw), "owner", "reason"); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	tx := prepareTest(t, s, 1)
	tx.Candidate.Config[0] = 'x'
	snap.Current.Config[0] = 'x'
	after, _ := s.Snapshot()
	if after.Current.Config[0] != '{' || after.Pending.Candidate.Config[0] != '{' {
		t.Fatal("caller mutated store")
	}
}
func TestCorruptionNeverInitializes(t *testing.T) {
	for _, data := range []string{`{`, `{"digest":"wrong","payload":{}}`, `{"digest":"x","payload":{},"extra":true}`} {
		path := t.TempDir()
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "state.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if s, err := Open(path, json.RawMessage(`{"value":0}`), validator); err == nil {
			s.Close()
			t.Fatal("replaced corrupt state")
		}
		got, _ := os.ReadFile(filepath.Join(path, "state.json"))
		if string(got) != data {
			t.Fatal("damaged recovery evidence")
		}
	}
}
func TestRejectUnsafeFiles(t *testing.T) {
	for _, name := range []string{"writer.lock", "state.json", "state.tmp"} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir()
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "unowned")
			if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(path, name)); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(path, json.RawMessage(`{"value":0}`), validator); err == nil {
				s.Close()
				t.Fatal("followed symlink")
			}
			got, _ := os.ReadFile(target)
			if string(got) != "preserve" {
				t.Fatal("overwrote target")
			}
		})
	}
}
func TestDeadlineAndRecoveryPhases(t *testing.T) {
	s := openTest(t, t.TempDir())
	tx := prepareTest(t, s, 1)
	if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []time.Time{time.Time{}, time.Now().Add(-time.Second), time.Now().Add(time.Hour)} {
		if err := s.Transition(tx.ID, AwaitingConfirmation, deadline); err == nil {
			t.Fatal("accepted invalid deadline")
		}
	}
	if err := s.Transition(tx.ID, RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(tx.ID, RecoveryRequired, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(tx.ID); err == nil {
		t.Fatal("confirmed failed rollback")
	}
	if err := s.Transition(tx.ID, RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedStagingPreservesCommittedSnapshot(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	before, _ := s.Snapshot()
	if err := os.Mkdir(filepath.Join(path, "state.tmp"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prepare(before.Current.ID, json.RawMessage(`{"value":1}`), "owner", "test"); err == nil {
		t.Fatal("staging failure hidden")
	}
	after, err := s.Snapshot()
	if err != nil || after.Current.ID != before.Current.ID || after.Pending != nil {
		t.Fatal("failed staging changed state")
	}
	s.Close()
	s = openTest(t, path)
	after, err = s.Snapshot()
	if err != nil || after.Current.ID != before.Current.ID {
		t.Fatal("durable before-state lost")
	}
}

// TestSeedCurrentFreshJournal pins the canonical happy path: a
// journal with no Pending, no Outcomes, and no History accepts
// the caller's revision as the new Current.
func TestSeedCurrentFreshJournal(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	r, err := s.revision(json.RawMessage(`{"value":42}`), "agent", "live kernel capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCurrent(r); err != nil {
		t.Fatalf("seed rejected on fresh journal: %v", err)
	}
	snap, _ := s.Snapshot()
	if snap.Current.ID != r.ID {
		t.Fatalf("current not seeded: got %s want %s", snap.Current.ID, r.ID)
	}
}

// TestSeedCurrentRejectsWithCommittedHistory is the safety
// invariant: a journal that has received a committed Apply must
// never be silently re-seeded. Re-seeding would orphan the
// committed receipt and could mask a real divergence.
func TestSeedCurrentRejectsWithCommittedHistory(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	tx := prepareTest(t, s, 1)
	if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Commit(tx.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot()
	r, err := s.revision(json.RawMessage(`{"value":99}`), "agent", "stale re-seed attempt")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCurrent(r); !errors.Is(err, ErrConflict) {
		t.Fatalf("seed must refuse journal with committed history; got %v", err)
	}
	after, _ := s.Snapshot()
	if after.Current.ID != before.Current.ID {
		t.Fatalf("committed current lost: %s -> %s", before.Current.ID, after.Current.ID)
	}
}

// TestSeedCurrentAcceptsRolledBackOutcomes is the operational
// invariant the engine's bootstrap relies on: rolled-back
// outcomes from rejected Apply attempts are noise that does
// NOT block re-seeding. The journal's Current was the
// placeholder and the agent's live Current has now diverged.
func TestSeedCurrentAcceptsRolledBackOutcomes(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	// Run a full Prepare→Transition→RolledBack cycle so a
	// "rolled-back" outcome lands in the journal. RejectRequest
	// requires the journal to have no Pending, so the cycle
	// must complete before the rolled-back noise is visible.
	tx := prepareTest(t, s, 1)
	if err := s.Transition(tx.ID, RollingBack, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RolledBack(tx.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot()
	if len(snap.Outcomes) != 1 || snap.Outcomes[0].Result != "rolled-back" {
		t.Fatalf("rolled-back outcome missing: %+v", snap.Outcomes)
	}
	r, err := s.revision(json.RawMessage(`{"value":7}`), "agent", "live kernel capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCurrent(r); err != nil {
		t.Fatalf("seed must accept re-seeding over rolled-back noise: %v", err)
	}
	after, _ := s.Snapshot()
	if after.Current.ID != r.ID {
		t.Fatalf("current not seeded: got %s want %s", after.Current.ID, r.ID)
	}
	// Old rolled-back outcomes must still be visible — re-seeding
	// rewrites Current, not history.
	if len(after.Outcomes) != 1 || after.Outcomes[0].Result != "rolled-back" {
		t.Fatalf("rolled-back outcome lost: %+v", after.Outcomes)
	}
}

// TestSeedCurrentRejectsWithPending pins the in-flight invariant:
// a journal that is mid-transaction must never be re-seeded.
func TestSeedCurrentRejectsWithPending(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	prepareTest(t, s, 1) // creates a Pending transaction
	r, err := s.revision(json.RawMessage(`{"value":5}`), "agent", "live kernel capture")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SeedCurrent(r); !errors.Is(err, ErrConflict) {
		t.Fatalf("seed must refuse journal with pending tx; got %v", err)
	}
}

// TestSeedCurrentRejectsInvalidRevision guards the validator
// pass-through: a malformed revision (wrong hex length, bad
// validator) must be refused the same as Prepare would.
func TestSeedCurrentRejectsInvalidRevision(t *testing.T) {
	path := t.TempDir()
	s := openTest(t, path)
	bad := Revision{ID: strings.Repeat("z", 64), Config: json.RawMessage(`{"value":1}`), Actor: "x", Reason: "y", CreatedAt: time.Now().UTC()}
	if err := s.SeedCurrent(bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("malformed revision id must be rejected: %v", err)
	}
}
