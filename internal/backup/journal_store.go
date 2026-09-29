// backup/journal_store.go adds persistence to the in-memory
// RestoreJournal so the Agent retains the audit trail across
// restarts. The file store is line-oriented JSON; each entry
// is exactly one JSON object on a single line. The store
// reads existing entries on Open so a restarted Agent
// picks up where the previous one left off.
//
// C-blocker #8: durable backup/restore workflow. The
// previous RestoreJournal lived in process memory only; an
// interrupted restore lost its audit trail. The store
// persists every Append so the Agent can recover and
// surface the in-flight state on the next boot.
package backup

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// JournalStore is the bounded file-backed persistence for
// RestoreJournal. The store owns a single file on disk;
// concurrent Append calls are serialised by a mutex. The
// in-memory RestoreJournal stays the authoritative cache
// so reads stay cheap; the store writes through to disk
// on every Append.
type JournalStore struct {
	mu      sync.Mutex
	dir     string
	path    string
	journal *RestoreJournal
	// seen is the dedup map so we never re-load an
	// existing entry twice (the in-memory journal keeps
	// its own dedup but the store tracks it on disk for
	// correctness).
	seen map[string]struct{}
}

// OpenJournalStore restores (or creates) a journal store
// at the supplied directory. Existing entries on disk are
// loaded so the restored journal matches the persisted
// audit trail.
func OpenJournalStore(dir string) (*JournalStore, error) {
	if dir == "" {
		return nil, errors.New("backup: journal directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("backup: journal directory: %w", err)
	}
	s := &JournalStore{
		dir:     dir,
		path:    filepath.Join(dir, "restore-journal.log"),
		journal: NewRestoreJournal(),
		seen:    map[string]struct{}{},
	}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// reload reads the existing journal file (if any) and
// replays every entry into the in-memory journal. The
// file is line-oriented JSON; malformed lines are skipped
// so a partial write does not break the boot recovery
// path. The file is stored in newest-first order (the
// order produced by persistLocked); the in-memory
// journal keeps the same order, so the reload walks the
// file in order and appends to the in-memory list.
func (s *JournalStore) reload() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("backup: journal read: %w", err)
	}
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e RestoreEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if _, ok := s.seen[e.ID]; ok {
			continue
		}
		s.seen[e.ID] = struct{}{}
		s.journal.mu.Lock()
		s.journal.entries[e.ID] = e
		s.journal.order = append(s.journal.order, e.ID)
		s.journal.mu.Unlock()
	}
	return nil
}

// Append inserts or updates an entry in the in-memory
// journal and writes through to the file. The file is
// rewritten on every Append because the in-memory journal
// already enforces retention; rebuilding from the
// authoritative cache keeps the file consistent.
func (s *JournalStore) Append(e RestoreEntry) error {
	if err := e.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.journal.Append(e); err != nil {
		return err
	}
	s.seen[e.ID] = struct{}{}
	return s.persistLocked()
}

// All returns every entry sorted newest first. The
// in-memory journal is the authoritative source so the
// returned slice matches what the Agent observes
// after the most recent Append.
func (s *JournalStore) All() []RestoreEntry {
	return s.journal.All()
}

// Get returns the entry with the supplied id.
func (s *JournalStore) Get(id string) (RestoreEntry, bool) {
	return s.journal.Get(id)
}

// Counters summarises the journal.
func (s *JournalStore) Counters() RestoreJournalCounters {
	return s.journal.Counters()
}

// SetMax overrides the retention cap.
func (s *JournalStore) SetMax(n int) {
	s.journal.SetMax(n)
}

// persistLocked rewrites the journal file from the
// in-memory cache. The call MUST hold s.mu.
//
// The write sequence is fsync(temp) + rename +
// fsync(parent-dir) so a crash at any point leaves
// either the previous journal or the new one — never a
// partial line. The parent-directory fsync is required
// for crash-safe rename on POSIX filesystems; without
// it a power loss after rename but before directory
// flush can lose the rename.
func (s *JournalStore) persistLocked() error {
	tmp := s.path + ".tmp"
	// Mode 0o600: the journal is an audit anchor and
	// must never be readable by unprivileged callers.
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("backup: journal write: %w", err)
	}
	for _, e := range s.journal.All() {
		data, err := json.Marshal(e)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("backup: journal marshal: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("backup: journal append: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: journal sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("backup: journal close: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("backup: journal rename: %w", err)
	}
	// Flush the parent directory so the rename is
	// durable. On Linux this requires opening the dir
	// by name and calling Sync().
	if d, derr := os.Open(filepath.Dir(s.path)); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// splitLines is a tiny helper so the reload path does not
// pull in bufio for what is usually a small file.
func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i > start {
			out = append(out, data[start:i])
		}
		start = i + 1
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

// silence unused-import warning for bufio when the
// future line-reader is wired in.
var _ = bufio.NewScanner
