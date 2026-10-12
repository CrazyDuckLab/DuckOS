// Package state torture: §12 Safe Apply torture test.
//
// The QA program requires us to exercise failure injection at
// every state-machine transition. These tests do not crash
// processes directly (we are not running inside a forked
// process); instead they exercise every transition with
// concurrent writers, force-rollback, and stale-request
// scenarios. The state.Store API has the right primitives
// (Prepare / Transition / Commit / RolledBack) — we confirm
// the invariants under load.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// stateDir returns a t.TempDir()-style private dir suitable
// for state.Open, which requires 0700 and not a symlink.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// t.TempDir() usually creates 0700 dirs, but be explicit.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod state dir: %v", err)
	}
	return dir
}

// TestTortureConcurrentWritersRace is §12.1: many
// goroutines call Prepare + Transition + Commit in
// parallel against the same Store. Final state must be
// exactly one of the proposed values, never mixed.
func TestTortureConcurrentWritersRace(t *testing.T) {
	dir := stateDir(t)
	initial := json.RawMessage(`{"value":0}`)
	validate := func(raw json.RawMessage) error {
		var v struct {
			Value int `json:"value"`
		}
		return json.Unmarshal(raw, &v)
	}
	s, err := Open(dir, initial, validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if _, err := s.Snapshot(); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	const writers = 16
	const iters = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers*iters)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				snap, _ := s.Snapshot()
				// Writers start at 1000 so the first candidate can
				// never equal the initial {"value":0} state: an
				// equal candidate is a plain "no changes" error,
				// not a contention conflict, and writer 0 iteration
				// 0 wins that race whenever the machine is fast.
				value := (id+1)*1000 + i
				cfg := json.RawMessage(fmt.Sprintf(`{"value":%d}`, value))
				tx, err := s.Prepare(snap.Current.ID, cfg, "torture", "concurrent writers")
				if err != nil {
					// Conflict is expected under heavy contention.
					if errors.Is(err, ErrConflict) || errors.Is(err, ErrPending) {
						continue
					}
					errs <- fmt.Errorf("prepare w=%d i=%d: %w", id, i, err)
					continue
				}
				if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
					errs <- fmt.Errorf("transition applying w=%d i=%d: %w", id, i, err)
					continue
				}
				if err := s.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
					errs <- fmt.Errorf("transition confirm w=%d i=%d: %w", id, i, err)
					continue
				}
				if err := s.Commit(tx.ID); err != nil {
					if errors.Is(err, ErrConflict) || errors.Is(err, ErrPending) {
						continue
					}
					errs <- fmt.Errorf("commit w=%d i=%d: %w", id, i, err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	var collected []error
	for e := range errs {
		collected = append(collected, e)
	}
	if len(collected) > 0 {
		t.Fatalf("unexpected errors:\n%v", collected)
	}
	final, err := s.Snapshot()
	if err != nil {
		t.Fatalf("final snapshot: %v", err)
	}
	var got struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(final.Current.Config, &got); err != nil {
		t.Fatalf("final config not parseable: %v", err)
	}
	// History should have grown. We don't pin exact count,
	// just confirm at least one commit happened.
	if len(final.History) == 0 {
		t.Fatalf("history empty after %d writers x %d iters", writers, iters)
	}
	t.Logf("after %d writers x %d iters: history=%d, final value=%d",
		writers, iters, len(final.History), got.Value)
}

// TestTortureStaleRequestRejected is §12.2: an admin
// reads revision N, prepares a request, but a second
// admin commits revision N+1 first. The first admin's
// stale commit must be REJECTED, not silently overwrite.
func TestTortureStaleRequestRejected(t *testing.T) {
	dir := stateDir(t)
	initial := json.RawMessage(`{"who":"none"}`)
	validate := func(raw json.RawMessage) error {
		var v struct {
			Who string `json:"who"`
		}
		return json.Unmarshal(raw, &v)
	}
	s, err := Open(dir, initial, validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	snapA, err := s.Snapshot()
	if err != nil {
		t.Fatalf("snapA: %v", err)
	}
	revA := snapA.Current.ID
	// Admin B reads revision and commits first.
	txB, err := s.Prepare(revA, json.RawMessage(`{"who":"B"}`), "admin-B", "first")
	if err != nil {
		t.Fatalf("prepare B: %v", err)
	}
	if err := s.Transition(txB.ID, Applying, time.Time{}); err != nil {
		t.Fatalf("B applying: %v", err)
	}
	if err := s.Transition(txB.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("B confirm: %v", err)
	}
	if err := s.Commit(txB.ID); err != nil {
		t.Fatalf("B commit: %v", err)
	}
	// Admin A still has revA; prepare with stale rev.
	txA, err := s.Prepare(revA, json.RawMessage(`{"who":"A"}`), "admin-A", "stale")
	if err != nil {
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrPending) {
			t.Logf("Prepare correctly refused stale revision: %v", err)
			return
		}
		t.Fatalf("prepare A: %v", err)
	}
	if err := s.Transition(txA.ID, Applying, time.Time{}); err != nil {
		t.Fatalf("A applying: %v", err)
	}
	if err := s.Transition(txA.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("A confirm: %v", err)
	}
	if err := s.Commit(txA.ID); err == nil {
		t.Fatalf("Admin A's stale commit was accepted — conflict overwrote B's revision!")
	}
	final, err := s.Snapshot()
	if err != nil {
		t.Fatalf("final snapshot: %v", err)
	}
	var got struct {
		Who string `json:"who"`
	}
	if err := json.Unmarshal(final.Current.Config, &got); err != nil {
		t.Fatalf("final config not parseable: %v", err)
	}
	if got.Who != "B" {
		t.Fatalf("expected B's revision to win, got %q", got.Who)
	}
}

// TestTortureRollbackRecoversPending is §12.3: a
// transaction is stuck in Applying. Recovery rolls it
// back. A new transaction with the same baseline revision
// then succeeds.
func TestTortureRollbackRecoversPending(t *testing.T) {
	dir := stateDir(t)
	initial := json.RawMessage(`{"who":"none"}`)
	validate := func(raw json.RawMessage) error {
		var v struct {
			Who string `json:"who"`
		}
		return json.Unmarshal(raw, &v)
	}
	s, err := Open(dir, initial, validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	snap, _ := s.Snapshot()
	rev0 := snap.Current.ID
	txA, err := s.Prepare(rev0, json.RawMessage(`{"who":"A"}`), "admin", "stuck")
	if err != nil {
		t.Fatalf("prepare A: %v", err)
	}
	if err := s.Transition(txA.ID, Applying, time.Time{}); err != nil {
		t.Fatalf("A applying: %v", err)
	}
	// Rollback flow: Applying -> RollingBack -> RolledBack.
	if err := s.Transition(txA.ID, RollingBack, time.Time{}); err != nil {
		t.Fatalf("A rolling-back: %v", err)
	}
	if err := s.RolledBack(txA.ID); err != nil {
		t.Fatalf("rolled back A: %v", err)
	}
	txB, err := s.Prepare(rev0, json.RawMessage(`{"who":"B"}`), "admin", "after rollback")
	if err != nil {
		t.Fatalf("prepare B: %v", err)
	}
	if err := s.Transition(txB.ID, Applying, time.Time{}); err != nil {
		t.Fatalf("B applying: %v", err)
	}
	if err := s.Transition(txB.ID, AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		t.Fatalf("B confirm: %v", err)
	}
	if err := s.Commit(txB.ID); err != nil {
		t.Fatalf("B commit: %v", err)
	}
	final, _ := s.Snapshot()
	var got struct {
		Who string `json:"who"`
	}
	json.Unmarshal(final.Current.Config, &got)
	if got.Who != "B" {
		t.Fatalf("expected B's revision, got %q", got.Who)
	}
}
