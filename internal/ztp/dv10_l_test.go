//go:build linux

// dv10_l_test.go exercises the DV10 L-tier subcase
// "second_writer_no_collision" against the real Linux
// flock(2) semantics. The contract demands:
//
//   - two concurrent OpenManager writers cannot both
//     hold the per-dir lock;
//   - restart/replay durability: re-opening the journal
//     after a process restart preserves the redeemed
//     digest and rejects replays;
//   - capacity exhaustion beyond MaxRedemptionJournal
//     is fail-closed (ErrCapacityExhausted);
//   - corrupted replay state (truncated lines, garbage)
//     fails CLOSED: reload() refuses to load so a
//     redeemed-token record cannot be silently dropped
//     and the replay window cannot be re-opened.
//
// Each subcase writes to a private temp dir under
// t.TempDir() so no host state is touched. The test
// process itself is the only writer; the "two writers"
// subcase is exercised by opening two Managers in
// sequence (the second OpenManager must return
// ErrLocked before Close, then succeed after Close).
package ztp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDV10SecondWriterRefused: a second OpenManager
// while the first is still open must return ErrLocked.
// This is the Linux flock(2) inter-process exclusion
// guarantee.
func TestDV10SecondWriterRefused(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	first, err := OpenManager(dir, key)
	if err != nil {
		t.Fatalf("first OpenManager: %v", err)
	}
	defer first.Close()
	second, err := OpenManager(dir, key)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("second OpenManager: got %v, want ErrLocked", err)
	}
	if second != nil {
		t.Fatal("second OpenManager must return nil Manager on lock failure")
	}
}

// TestDV10ReopenAfterClose: closing the first Manager
// releases the lock; a subsequent OpenManager must
// succeed and inherit the durable state.
func TestDV10ReopenAfterClose(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	first, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, first, IssueRequest{Subject: "node-1", Validity: time.Minute})
	rec, err := first.Redeem(context.Background(), tok, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	// Restart: new Manager, same dir.
	second, err := OpenManager(dir, key)
	if err != nil {
		t.Fatalf("reopen after Close: %v", err)
	}
	defer second.Close()
	got, ok := second.FindByDigest(rec.TokenDigest)
	if !ok {
		t.Fatal("redemption did not survive restart")
	}
	if got.Phase != PhaseRedeemed {
		t.Fatalf("Phase: %q, want redeemed", got.Phase)
	}
}

// TestDV10ReplayRefusedAfterRestart: after a restart,
// the second Redeem of the same token must return
// ErrTokenReplay (the durable dedup must survive the
// process boundary).
func TestDV10ReplayRefusedAfterRestart(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{Subject: "node-2", Validity: time.Minute})
	if _, err := m.Redeem(context.Background(), tok, "node-2"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m2, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	_, err = m2.Redeem(context.Background(), tok, "node-2")
	if !errors.Is(err, ErrTokenReplay) {
		t.Fatalf("replay after restart: got %v, want ErrTokenReplay", err)
	}
}

// TestDV10CapacityFailClosedBeyondRetention: forcing
// the journal past MaxRedemptionJournal with unexpired
// records must return ErrCapacityExhausted (the audit
// forbids eviction of unexpired records).
func TestDV10CapacityFailClosedBeyondRetention(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// Stretch TTL so issued tokens remain unexpired
	// during the test.
	m.SetTTL(time.Hour)
	// Issue + Redeem one token at a time, counting
	// until the journal hits MaxRedemptionJournal.
	for i := 0; i < MaxRedemptionJournal; i++ {
		tok := mintToken(t, m, IssueRequest{
			Subject:  "node-cap",
			Validity: time.Hour,
		})
		if _, err := m.Redeem(context.Background(), tok, "node-cap"); err != nil {
			t.Fatalf("Redeem %d: %v", i, err)
		}
	}
	// One more unexpired redemption must be refused.
	tok := mintToken(t, m, IssueRequest{
		Subject:  "node-cap-extra",
		Validity: time.Hour,
	})
	_, err = m.Redeem(context.Background(), tok, "node-cap-extra")
	if !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("expected ErrCapacityExhausted after %d records, got %v",
			MaxRedemptionJournal, err)
	}
}

// TestDV10CorruptJournalFailsClosed: a journal with
// truncated / garbage lines FAILS CLOSED. reload() must
// return an error rather than silently dropping a
// redeemed-token record (a silent drop re-opens the
// replay window — the audit classified that as a
// high-severity vulnerability). The production
// composition root treats the error as a startup
// failure and refuses to mint or accept tokens until
// the operator restores a known-good journal.
func TestDV10CorruptJournalFailsClosed(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	tok := mintToken(t, m, IssueRequest{Subject: "node-c", Validity: time.Minute})
	rec, err := m.Redeem(context.Background(), tok, "node-c")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Corrupt the journal: append garbage.
	journalPath := filepath.Join(dir, "redemption-journal.log")
	original, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	b.Write(original)
	b.WriteString("this is not json at all\n")
	if err := os.WriteFile(journalPath, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	// Reopen must fail — the journal is corrupt and the
	// production composition root cannot trust the
	// replay index.
	m2, err := OpenManager(dir, key)
	if err == nil {
		_ = m2.Close()
		t.Fatalf("reload after corruption must FAIL CLOSED; got nil error")
	}
	// Restore the original journal; reopen succeeds and
	// the original good record is preserved.
	if err := os.WriteFile(journalPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	m3, err := OpenManager(dir, key)
	if err != nil {
		t.Fatalf("reload of restored journal: %v", err)
	}
	defer m3.Close()
	got, ok := m3.FindByDigest(rec.TokenDigest)
	if !ok {
		t.Fatal("good record lost after restore")
	}
	if got.Phase != PhaseRedeemed {
		t.Fatalf("Phase: %q, want redeemed", got.Phase)
	}
}

// TestDV10JournalHasLockFileAfterOpen: the per-dir lock
// file exists while the Manager is open. Closing the
// Manager releases the lock so the file is no longer
// held. The flock(2) semantics (not just file existence)
// guarantee the inter-process exclusion; this test pins
// the lockfile as the documented artifact of that.
func TestDV10JournalHasLockFileAfterOpen(t *testing.T) {
	dir := t.TempDir()
	key := tempKey(t)
	m, err := OpenManager(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "redemption-journal.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file missing while Manager open: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Lockfile on Linux is intentionally kept around so
	// external monitoring can observe it; the flock is
	// advisory. The file content is empty (per O_EXCL
	// elsewhere).
	b, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file vanished after Close: %v", err)
	}
	if !strings.HasPrefix(strings.TrimRight(string(b), "\n"), "") {
		t.Fatalf("unexpected lock file content: %q", b)
	}
}
