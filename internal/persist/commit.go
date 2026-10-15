// Package persist / commit.go: the commit-outcome model.
//
// UQ-02 root cause: a directory-fsync failure AFTER a successful
// rename was reported like any other storage error, so callers
// rolled back their in-memory state — and the next mutation then
// wrote the older, more permissive snapshot over the revoked state
// that the rename had already placed on disk. Atomic visibility is
// NOT durable-commit acknowledgement, and an uncertain outcome is
// NOT evidence that the previous state remains authoritative.
//
// Commit outcomes:
//
//	CommitOK            — tmp write + fsync + rename + dir fsync all
//	                      succeeded: durably committed (to the extent
//	                      fsync guarantees durability).
//	CommitNotCommitted  — the failure happened BEFORE the rename: the
//	                      previous file is untouched and remains the
//	                      only authoritative state; callers roll back.
//	CommitUncertain     — the rename succeeded and the new bytes ARE
//	                      visible in the directory, but directory
//	                      metadata durability could not be confirmed
//	                      (dir fsync failed). Callers MUST keep the
//	                      NEW (restrictive) state, MUST NOT roll back,
//	                      MUST NOT issue further commits until the
//	                      outcome is reconciled against the disk.
package persist

import (
	"errors"
	"fmt"
	"os"
)

// CommitOutcome classifies one Update attempt.
type CommitOutcome int

const (
	CommitOK CommitOutcome = iota
	CommitNotCommitted
	CommitUncertain
)

func (c CommitOutcome) String() string {
	switch c {
	case CommitOK:
		return "committed"
	case CommitNotCommitted:
		return "not_committed"
	default:
		return "commit_uncertain"
	}
}

// ErrUncertain wraps post-rename failures. errors.Is identifies the
// degraded class; the wrapped cause carries the storage detail.
var ErrUncertain = errors.New("persist: commit outcome uncertain (rename applied, directory durability unconfirmed)")

// syncDirFn is the directory-fsync boundary. Production binds the
// real syscall; fault injection substitutes the result. Keeping the
// boundary explicit makes the post-rename uncertainty testable on
// every platform (the Linux seccomp qualification exercises the
// same boundary natively).
var syncDirFn = syncDir

// Update applies mut and commits atomically, reporting the commit
// phase of any failure. On CommitNotCommitted the in-memory state
// is rolled back; on CommitUncertain the in-memory state KEEPS the
// new content (it matches what the rename most plausibly left on
// disk, and it is the caller's duty to treat uncertainty as a
// degraded, fail-closed condition).
func (f *File[T]) Update(mut func(*T)) (CommitOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev := f.state
	mut(&f.state)
	raw, err := jsonMarshal(f.state)
	if err != nil {
		f.state = prev
		return CommitNotCommitted, fmt.Errorf("persist: encode: %w", err)
	}
	tmp := f.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return CommitNotCommitted, fmt.Errorf("persist: write: %w", err)
	}
	// Permission enforcement before durability: a wrong-mode file
	// must never become the committed state.
	if err := os.Chmod(tmp, 0o600); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return CommitNotCommitted, fmt.Errorf("persist: chmod: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return CommitNotCommitted, fmt.Errorf("persist: fsync file: %w", err)
	}
	if err := closeTmp(tmp); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return CommitNotCommitted, fmt.Errorf("persist: close: %w", err)
	}
	if err := os.Rename(tmp, f.path()); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return CommitNotCommitted, fmt.Errorf("persist: rename: %w", err)
	}
	// Post-rename: the new content is what the directory now shows.
	// Only the durability of that visibility is in question.
	if err := syncDirFn(f.dir); err != nil {
		return CommitUncertain, fmt.Errorf("%w: %s", ErrUncertain, err)
	}
	return CommitOK, nil
}
