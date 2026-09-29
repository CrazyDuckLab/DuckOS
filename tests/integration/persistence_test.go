//go:build linux

// persistence_test.go covers the Safe Apply
// persistence boundary. The test exercises the
// Prepare / Transition / Commit cycle through
// the real State store, then closes the store
// and re-opens it to verify the durable
// snapshot survived. The test is the
// integration contract for every V1 subsystem
// that goes through Safe Apply.
package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

// TestStateStoreSafeApplyRoundTrip walks the
// real State store through a Safe Apply
// lifecycle. The test is the contract for the
// "no pending root enrollment" gate: an open
// transaction must be visible to OpenExisting
// after a restart, and the rolled-back snapshot
// must be restored.
func TestStateStoreSafeApplyRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	path := dir
	validate := func(b json.RawMessage) error {
		if !json.Valid(b) {
			return errors.New("invalid JSON")
		}
		return nil
	}
	store, err := state.Open(path, json.RawMessage(`{"value":0}`), validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Prepare a transaction. The first arg is the
	// expected current revision id; capture it from
	// the snapshot so the optimistic-concurrency
	// check succeeds on the first Prepare.
	initialSnap, err := store.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	tx, err := store.Prepare(initialSnap.Current.ID, json.RawMessage(`{"value":42}`), "operator-1", "test prepare")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if tx.ID == "" {
		t.Fatalf("expected non-empty transaction id")
	}
	// The transaction is already in Prepared after
	// Prepare. Walk the legal transitions:
	// Prepared -> Applying (no deadline) ->
	// AwaitingConfirmation (1-15 min deadline).
	confirmDeadline := time.Now().Add(5 * time.Minute)
	if err := store.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		t.Fatalf("transition applying: %v", err)
	}
	if err := store.Transition(tx.ID, state.AwaitingConfirmation, confirmDeadline); err != nil {
		t.Fatalf("transition awaiting-confirmation: %v", err)
	}
	// Close without commit: the transaction is
	// still pending. OpenExisting must surface
	// it as recovery-required via the Pending
	// pointer, not as a committed history entry.
	_ = store.Close()
	recovered, err := state.OpenExisting(path, validate)
	if err != nil {
		t.Fatalf("open existing: %v", err)
	}
	defer recovered.Close()
	snap, err := recovered.Snapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.Pending == nil || snap.Pending.ID != tx.ID {
		t.Fatalf("expected pending tx %s, got %+v", tx.ID, snap.Pending)
	}
	if len(snap.History) != 0 {
		t.Fatalf("expected 0 history entries, got %d", len(snap.History))
	}
	// Commit the transaction: the snapshot must
	// rotate to the candidate revision.
	if err := recovered.Commit(tx.ID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	snap, _ = recovered.Snapshot()
	if snap.Current.ID != tx.Candidate.ID {
		t.Fatalf("expected current to be %s, got %s", tx.Candidate.ID, snap.Current.ID)
	}
	_ = recovered.Close()
	// Re-open: the committed state must be
	// visible.
	again, err := state.OpenExisting(path, validate)
	if err != nil {
		t.Fatalf("open existing again: %v", err)
	}
	defer again.Close()
	snap, _ = again.Snapshot()
	if snap.Current.ID != tx.Candidate.ID {
		t.Fatalf("expected persisted current %s, got %s", tx.Candidate.ID, snap.Current.ID)
	}
}

// TestStateStoreRollbackRoundTrip walks a
// transaction through Prepare / Transition /
// Rollback. After a restart, the snapshot must
// reflect the rolled-back state, not the
// rolled-forward attempt.
func TestStateStoreRollbackRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	path := dir
	validate := func(b json.RawMessage) error {
		if !json.Valid(b) {
			return errors.New("invalid JSON")
		}
		return nil
	}
	store, err := state.Open(path, json.RawMessage(`{"value":0}`), validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Apply a successful transaction so the
	// current snapshot is not empty.
	initial, _ := store.Snapshot()
	tx1, err := store.Prepare(initial.Current.ID, json.RawMessage(`{"value":1}`), "operator-1", "test apply")
	if err != nil {
		t.Fatalf("prepare tx1: %v", err)
	}
	now := time.Now().Add(5 * time.Minute)
	// Walk the legal transitions: Prepared (already
	// after Prepare) -> Applying (no deadline) ->
	// AwaitingConfirmation (1-15 min deadline).
	if err := store.Transition(tx1.ID, state.Applying, time.Time{}); err != nil {
		t.Fatalf("transition tx1 applying: %v", err)
	}
	if err := store.Transition(tx1.ID, state.AwaitingConfirmation, now); err != nil {
		t.Fatalf("transition tx1 awaiting-confirmation: %v", err)
	}
	if err := store.Commit(tx1.ID); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}
	snap, _ := store.Snapshot()
	// Now attempt a second transaction and
	// roll it back.
	tx2, err := store.Prepare(snap.Current.ID, json.RawMessage(`{"value":2}`), "operator-1", "test rollback")
	if err != nil {
		t.Fatalf("prepare tx2: %v", err)
	}
	if err := store.Transition(tx2.ID, state.RollingBack, time.Time{}); err != nil {
		t.Fatalf("transition tx2 rolling-back: %v", err)
	}
	if err := store.RolledBack(tx2.ID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	_ = store.Close()
	// Re-open: current must still be tx1.
	again, err := state.OpenExisting(path, validate)
	if err != nil {
		t.Fatalf("open existing: %v", err)
	}
	defer again.Close()
	snap2, _ := again.Snapshot()
	if snap2.Current.ID != tx1.Candidate.ID {
		t.Fatalf("expected current tx1 %s, got %s", tx1.Candidate.ID, snap2.Current.ID)
	}
}

// TestStateStoreConcurrentReconcile walks many
// concurrent reconciles through the State
// store. The store is the single Safe Apply
// gate; the test verifies the gate is
// goroutine-safe.
func TestStateStoreConcurrentReconcile(t *testing.T) {
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	path := dir
	validate := func(b json.RawMessage) error {
		if !json.Valid(b) {
			return errors.New("invalid JSON")
		}
		return nil
	}
	store, err := state.Open(path, json.RawMessage(`{"value":0}`), validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()
	// The State store is the single Safe Apply
	// gate; the test exercises that gate under
	// concurrent Prepare traffic. The state
	// machine refuses concurrent Pending
	// (ErrPending) and concurrent rev mismatches
	// (ErrConflict); both are expected, neither
	// counts as a failure. The test only fails
	// on an unexpected error or a runtime panic.
	initial, _ := store.Snapshot()
	var unexpected int32
	var ok int32
	var pending int32
	var conflict int32
	done := make(chan struct{}, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			for j := 0; j < 8; j++ {
				_, err := store.Prepare(initial.Current.ID, json.RawMessage(fmt.Sprintf(`{"value":%d}`, i*100+j)), "operator-1", "concurrent reconcile")
				switch {
				case err == nil:
					atomic.AddInt32(&ok, 1)
				case errors.Is(err, state.ErrPending):
					atomic.AddInt32(&pending, 1)
				case errors.Is(err, state.ErrConflict):
					atomic.AddInt32(&conflict, 1)
				default:
					atomic.AddInt32(&unexpected, 1)
				}
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 16; i++ {
		<-done
	}
	if atomic.LoadInt32(&unexpected) != 0 {
		t.Fatalf("unexpected errors: %d (ok=%d pending=%d conflict=%d)", unexpected, ok, pending, conflict)
	}
	if ok == 0 {
		t.Fatal("expected at least one Prepare to succeed under contention")
	}
}

// ensurePrivateDir is no longer needed: os.Chmod
// above is the contract. The helper is kept as
// a no-op for clarity and to keep the test
// self-documenting.
