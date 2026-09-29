// journal_store_test.go pins the durable audit-trail
// behaviour of the JournalStore (C-blocker #8). A
// restart MUST surface the previously written entries
// so an interrupted Agent picks up where it left off.
package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// sampleEntry returns a canonical RestoreEntry the
// tests can mutate. The entry is self-consistent so the
// in-memory journal's validation accepts it.
func sampleEntry(id string) RestoreEntry {
	return RestoreEntry{
		ID:             id,
		BackupID:       "backup-0001",
		Outcome:        OutcomeStarted,
		SourceRevision: "rev-abc",
		StartedAt:      time.Now().UTC(),
	}
}

// TestJournalStorePersistsAcrossRestart proves that an
// entry written through the store is visible after the
// store is reopened on the same directory.
func TestJournalStorePersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := sampleEntry("entry-aaaaaaaa")
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	// Reopen the store and verify the entry survives.
	s2, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(e.ID)
	if !ok {
		t.Fatalf("entry %q did not survive restart", e.ID)
	}
	if got.BackupID != e.BackupID {
		t.Fatalf("BackupID: %q, want %q", got.BackupID, e.BackupID)
	}
}

// TestJournalStoreFileModeIs600 pins the audit-trail
// permission contract. The journal is an audit anchor;
// it MUST never be readable by unprivileged callers.
func TestJournalStoreFileModeIs600(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(sampleEntry("entry-aaaaaaaa")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "restore-journal.log"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode: %v, want 0600", info.Mode().Perm())
	}
}

// TestJournalStoreRewriteKeepsOrder proves the
// write-through behaviour preserves the most-recent-first
// order on disk. Reload must reproduce that order.
func TestJournalStoreRewriteKeepsOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"entry-zzzzzzzz", "entry-aaaaaaaa", "entry-mmmmmmmm"} {
		if err := s.Append(sampleEntry(id)); err != nil {
			t.Fatal(err)
		}
	}
	s2, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := s2.All()
	if len(all) != 3 {
		t.Fatalf("len: %d, want 3", len(all))
	}
	want := []string{"entry-mmmmmmmm", "entry-aaaaaaaa", "entry-zzzzzzzz"}
	for i, e := range all {
		if e.ID != want[i] {
			t.Fatalf("[%d]: %q, want %q", i, e.ID, want[i])
		}
	}
}

// TestJournalStoreMalformedLinesIgnored proves the
// reload path skips malformed lines so a partial write
// does not break the boot recovery path.
func TestJournalStoreMalformedLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "restore-journal.log")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	good := sampleEntry("entry-aaaaaaaa")
	line, _ := json.Marshal(good)
	body := append([]byte("{not-json\n"), line...)
	body = append(body, '\n')
	body = append(body, []byte("also-not-json\n")...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get("entry-aaaaaaaa")
	if !ok {
		t.Fatal("good entry was not loaded after malformed lines")
	}
	if got.BackupID != good.BackupID {
		t.Fatalf("BackupID: %q, want %q", got.BackupID, good.BackupID)
	}
}
