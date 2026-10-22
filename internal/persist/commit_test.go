// Package persist / commit_test.go: UQ-02 commit-phase fault
// boundaries. The directory-fsync boundary is exercised through the
// real syscall path with an injected result; pre-rename failures
// are covered by file_test.go (write/rename rollback). Physical
// power-loss durability cannot be demonstrated on a test host and
// is stated as such in the qualification report.
package persist

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type snap struct {
	Status string `json:"status"`
}

// Post-rename dir-fsync failure: outcome UNCERTAIN, the new bytes
// are what the directory shows, memory keeps the new (restrictive)
// state, and the error carries no storage paths.
func TestCommit_DirFsyncFailureIsUncertainAndFileShowsNewState(t *testing.T) {
	dir := t.TempDir()
	f, ferr := Open[snap](dir, "state.json")
	if ferr != nil {
		t.Fatal(ferr)
	}
	if _, err := f.Update(func(s *snap) { s.Status = "active" }); err != nil {
		t.Fatal(err)
	}
	restore := SyncDirForTest(func(string) error { return errors.New("injected EIO") })
	outcome, uncertainErr := f.Update(func(s *snap) { s.Status = "revoked" })
	restore()
	if outcome != CommitUncertain || uncertainErr == nil || !errors.Is(uncertainErr, ErrUncertain) {
		t.Fatalf("outcome=%v err=%v", outcome, uncertainErr)
	}
	reopened, err := Open[snap](dir, "state.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get().Status; got != "revoked" {
		t.Fatalf("disk_status=%s, want the post-rename state", got)
	}
	if got := f.Get().Status; got != "revoked" {
		t.Fatalf("memory_status=%s, want the retained restrictive state", got)
	}
	if strings.Contains(uncertainErr.Error(), dir) {
		t.Fatal("uncertain error leaks the storage path")
	}
	// Healthy again: the next commit is a normal OK.
	restore2 := SyncDirForTest(func(string) error { return nil })
	outcome2, err2 := f.Update(func(s *snap) { s.Status = "revoked-confirmed" })
	restore2()
	if outcome2 != CommitOK || err2 != nil {
		t.Fatalf("post-recovery commit: %v %v", outcome2, err2)
	}
}

// Pre-rename tmp fsync failure class: covered end-to-end by
// file_test.go TestUpdateRollbackOnPersistFailure (rename failure).
// This pins the outcome classification for that test's scenario.
func TestCommit_PreRenameFailureClassification(t *testing.T) {
	dir := t.TempDir()
	f, ferr := Open[snap](dir, "state.json")
	if ferr != nil {
		t.Fatal(ferr)
	}
	if _, err := f.Update(func(s *snap) { s.Status = "active" }); err != nil {
		t.Fatal(err)
	}
	// Replace the destination with a directory so the final rename
	// fails (same arrangement as file_test.go's rollback test).
	if err := os.Remove(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "state.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := f.Update(func(s *snap) { s.Status = "revoked" })
	if outcome != CommitNotCommitted || err == nil {
		t.Fatalf("outcome=%v err=%v", outcome, err)
	}
	if got := f.Get().Status; got != "active" {
		t.Fatalf("memory_status=%s after NOT_COMMITTED, want rollback", got)
	}
}

// mkdirAt places a directory at the given file name inside the
// store directory (forcing the final rename to fail). The path
// derivation mirrors File.path() for tests.
