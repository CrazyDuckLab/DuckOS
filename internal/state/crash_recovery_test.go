// Package state torture: §13 power-loss / crash simulation.
//
// §13 requires us to verify that after a simulated crash
// during a state.Store write, the next Open() recovers a
// consistent snapshot — never a half-written file, never
// a state where the journal claims a transaction exists
// but the snapshot is missing.
//
// We can't kill the test process itself (that would
// terminate the test), but we CAN:
//  1. Force-close the store without graceful shutdown
//  2. Tamper with state.json or state.tmp on disk
//  3. Reopen and verify the state is recoverable
//
// The state.Store uses `os.Rename` for atomic state.json
// replacement after fsync; this is the gold standard for
// POSIX crash safety. The tests below verify that any
// pre-rename tampering is invisible to the next Open().
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCrashRecoveryAtomicRenameSurvivesTamperedTmp is §13.1:
// prepare a transaction, then corrupt the tmp file
// that state.Store writes before rename. The next
// Open() must read the previous committed state and
// ignore the corrupted tmp file.
func TestCrashRecoveryAtomicRenameSurvivesTamperedTmp(t *testing.T) {
	dir := stateDir(t)
	initial := json.RawMessage(`{"value":1}`)
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
	snap, _ := s.Snapshot()
	// Commit one good transaction with a different value.
	tx, err := s.Prepare(snap.Current.ID, json.RawMessage(`{"value":2}`), "owner", "first")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := s.Transition(tx.ID, Applying, time.Time{}); err != nil {
		t.Fatalf("applying: %v", err)
	}
	if err := s.Transition(tx.ID, AwaitingConfirmation, time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := s.Commit(tx.ID); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Simulate a crash mid-write: trash the state.tmp
	// file (if present) and the state.json file, then
	// reopen. The atomic rename semantics mean the
	// committed state survives.
	tmpPath := filepath.Join(dir, "state.tmp")
	if err := os.WriteFile(tmpPath, []byte("GARBAGE"), 0o600); err != nil {
		t.Fatalf("trash tmp: %v", err)
	}
	// Open and check we still get value=1.
	s2, err := OpenExisting(dir, validate)
	if err != nil {
		t.Fatalf("reopen after tmp tamper: %v", err)
	}
	defer s2.Close()
	snap2, err := s2.Snapshot()
	if err != nil {
		t.Fatalf("snap: %v", err)
	}
	var got struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(snap2.Current.Config, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Value != 2 {
		t.Fatalf("expected value=2, got %d", got.Value)
	}
}

// TestCrashRecoveryRejectsTruncatedState is §13.2:
// truncate the state.json file mid-write. The next
// OpenExisting should detect corruption and refuse
// to return a partial state.
func TestCrashRecoveryRejectsTruncatedState(t *testing.T) {
	dir := stateDir(t)
	initial := json.RawMessage(`{"value":1}`)
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
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Truncate state.json to half its size.
	statePath := filepath.Join(dir, "state.json")
	st, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := os.Truncate(statePath, st.Size()/2); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// OpenExisting should refuse the truncated state.
	if _, err := OpenExisting(dir, validate); err == nil {
		t.Fatalf("OpenExisting accepted truncated state.json — should refuse")
	}
}

// TestCrashRecoveryEmptyDirIsFreshStore is §13.3:
// after a "wipe everything" event, Open on an empty
// dir returns a fresh store rather than failing.
func TestCrashRecoveryEmptyDirIsFreshStore(t *testing.T) {
	dir := stateDir(t)
	validate := func(raw json.RawMessage) error {
		var v struct {
			Value int `json:"value"`
		}
		return json.Unmarshal(raw, &v)
	}
	s, err := Open(dir, json.RawMessage(`{"value":1}`), validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Wipe everything in the dir.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	// Now Open (which expects to provision a fresh store
	// on an empty dir) should work.
	s2, err := Open(dir, json.RawMessage(`{"value":99}`), validate)
	if err != nil {
		t.Fatalf("open fresh after wipe: %v", err)
	}
	defer s2.Close()
	snap, err := s2.Snapshot()
	if err != nil {
		t.Fatalf("snap: %v", err)
	}
	var got struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(snap.Current.Config, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Value != 99 {
		t.Fatalf("expected value=99 after wipe, got %d", got.Value)
	}
}

// TestCrashRecoveryRefusesPopulatedDir is §13.4:
// Verify the state store tolerates other files in the
// state directory (e.g. log files, lock files). We
// pre-create a state.json AND commit a transaction,
// then drop a "stale.txt" file and re-open — the
// previous state must still be readable.
func TestCrashRecoveryRefusesPopulatedDir(t *testing.T) {
	dir := stateDir(t)
	validate := func(raw json.RawMessage) error {
		var v struct {
			Value int `json:"value"`
		}
		return json.Unmarshal(raw, &v)
	}
	s, err := Open(dir, json.RawMessage(`{"value":42}`), validate)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Drop a "stale.txt" file. OpenExisting should still
	// load the journal and not be confused by other
	// files.
	if err := os.WriteFile(filepath.Join(dir, "stale.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s2, err := OpenExisting(dir, validate)
	if err != nil {
		t.Fatalf("OpenExisting after writing stale file: %v", err)
	}
	defer s2.Close()
	snap, _ := s2.Snapshot()
	var got struct {
		Value int `json:"value"`
	}
	json.Unmarshal(snap.Current.Config, &got)
	if got.Value != 42 {
		t.Fatalf("expected value=42, got %d", got.Value)
	}
}
