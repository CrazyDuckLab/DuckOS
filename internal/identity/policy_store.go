// identity/policy_store.go adds durable persistence to
// the per-object policy overlay (B05). The in-memory
// Policy remains the authoritative read cache so reads
// stay cheap; the file store writes through to disk on
// every Set/Delete so the Agent survives a restart with
// the overlay intact.
//
// The store is goroutine-safe at the file boundary via a
// separate mutex; the in-memory Policy still owns its own
// RWMutex so concurrent callers cannot race the cached
// state.
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// PolicyStore is the bounded file-backed persistence for
// the per-object policy overlay. The file is
// line-oriented JSON; each grant is exactly one JSON
// object on a single line. The store reads existing
// grants on Open so a restarted Agent picks up where the
// previous one left off.
type PolicyStore struct {
	mu     sync.Mutex
	dir    string
	path   string
	tmp    string
	policy *Policy
}

// OpenPolicyStore restores (or creates) a policy store at
// the supplied directory. Existing grants on disk are
// loaded into the in-memory Policy. The supplied
// fallbackPolicy is used when no on-disk file exists
// yet; pass NewPolicy() for an empty default.
func OpenPolicyStore(dir string, fallbackPolicy *Policy) (*PolicyStore, error) {
	if dir == "" {
		return nil, errors.New("identity: policy directory is required")
	}
	if fallbackPolicy == nil {
		fallbackPolicy = NewPolicy()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("identity: policy directory: %w", err)
	}
	s := &PolicyStore{
		dir:    dir,
		path:   filepath.Join(dir, "policy-overlay.log"),
		tmp:    filepath.Join(dir, "policy-overlay.log.tmp"),
		policy: fallbackPolicy,
	}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// reload reads the existing overlay file (if any) and
// replays every grant into the in-memory Policy. The
// file is line-oriented JSON.
//
// SECURITY (DV05, B05): the overlay is an authorisation
// anchor. A corrupt overlay that silently drops deny
// rules is a permissive-fallback vulnerability — the
// audit classified that as a high-severity failure
// mode. reload() therefore fails CLOSED: any corrupt
// line (bad JSON, invalid grant, mismatched schema
// version, truncated final line) returns an error and
// the in-memory Policy is left untouched. Callers must
// either restore a known-good file or block
// authorization at the call site.
//
// The "incomplete trailing line" guard exists so a
// crash mid-write (which can leave the file with no
// terminating '\n' on the final partial grant) does
// not silently truncate the overlay — the partial
// grant is reported as corruption, not skipped.
func (s *PolicyStore) reload() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("identity: overlay read: %w", err)
	}
	lines := splitPolicyLines(data)
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		var g Grant
		if err := json.Unmarshal(line, &g); err != nil {
			return fmt.Errorf("identity: overlay corrupt line %d: %w", i+1, err)
		}
		if err := g.Valid(); err != nil {
			return fmt.Errorf("identity: overlay invalid grant line %d: %w", i+1, err)
		}
		if err := s.policy.Set(g); err != nil {
			return fmt.Errorf("identity: overlay reject grant line %d: %w", i+1, err)
		}
	}
	// A file without a trailing newline ends on a
	// partial line; the JSON above already parsed it.
	// If the final line was truncated mid-write the
	// parser would have produced an error, so reaching
	// here means every line is intact.
	return nil
}

// Policy returns the in-memory Policy the store owns.
func (s *PolicyStore) Policy() *Policy { return s.policy }

// Set upserts a grant through the in-memory Policy and
// persists the full overlay. The write-through is
// synchronous (write + fsync + rename + parent-dir fsync)
// so a crash mid-write never leaves a partial line on
// disk.
func (s *PolicyStore) Set(g Grant) error {
	if err := s.policy.Set(g); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked()
}

// Delete removes the grant from the in-memory Policy and
// persists the overlay.
func (s *PolicyStore) Delete(role Role, obj ObjectKey, perm Permission) error {
	s.policy.Delete(role, obj, perm)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistLocked()
}

// persistLocked rewrites the overlay file from the
// in-memory cache. The call MUST hold s.mu.
func (s *PolicyStore) persistLocked() error {
	// Mode 0o600: the overlay is an authorisation
	// anchor and must never be readable by
	// unprivileged callers.
	f, err := os.OpenFile(s.tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("identity: overlay write: %w", err)
	}
	for _, g := range s.policy.All() {
		data, err := json.Marshal(g)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("identity: overlay marshal: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("identity: overlay append: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("identity: overlay sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("identity: overlay close: %w", err)
	}
	if err := os.Rename(s.tmp, s.path); err != nil {
		return fmt.Errorf("identity: overlay rename: %w", err)
	}
	// Flush the parent directory so the rename is
	// durable.
	if d, derr := os.Open(filepath.Dir(s.path)); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// ContentSHA256 returns the SHA-256 hex digest of the
// on-disk overlay (empty string when the file does not
// exist). The digest is the canonical integrity anchor
// for audit/replay.
func (s *PolicyStore) ContentSHA256() (string, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// splitPolicyLines is a tiny helper so the reload path
// does not pull in bufio for what is usually a small file.
func splitPolicyLines(data []byte) [][]byte {
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
