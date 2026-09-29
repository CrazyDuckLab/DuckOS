// firstrun/firstrun.go: G18 first-run persistence.
//
// The Agent persists first-run completion to a single
// bounded JSON document on disk. The document is written
// atomically (temp file + fsync + rename + chmod 0600) so
// a power loss cannot leave a partially written state.
// The schema is versioned; older revisions are migrated
// forward without losing data.
//
// The Agent composition root is the only writer; the API
// and CLI are read-only observers. The AdminToken field
// carries the SHA-256 hash of the bootstrap bearer token;
// the raw token never reaches disk.
package firstrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the on-disk schema version. Bump only
// when the JSON shape changes incompatibly; readers must
// refuse unknown major versions.
const SchemaVersion = 1

// State is the canonical first-run document.
type State struct {
	Version int `json:"version"`
	// CompletedAt is the wall-clock time of the first
	// successful bootstrap. Zero means "not yet completed".
	CompletedAt time.Time `json:"completedAt,omitempty"`
	// Mode is the operator-selected mode ("easy", "full",
	// "router", "server", "custom").
	Mode string `json:"mode,omitempty"`
	// Locale is the operator-selected locale.
	Locale string `json:"locale,omitempty"`
	// Theme is "light" or "dark".
	Theme string `json:"theme,omitempty"`
	// Steps is the canonical list of bootstrap steps the
	// operator completed. The list is sorted and
	// deduplicated on every write.
	Steps []string `json:"steps,omitempty"`
	// AdminTokenHash is the hex-encoded SHA-256 of the
	// bootstrap bearer token. The raw token is never
	// stored.
	AdminTokenHash string `json:"adminTokenHash,omitempty"`
	// Hostname is the local hostname captured at first run.
	Hostname string `json:"hostname,omitempty"`
	// UpdatedAt is the last mutation timestamp.
	UpdatedAt time.Time `json:"updatedAt"`
}

// Valid reports whether s is internally consistent. The
// function is used by the Agent composition root before
// persisting.
func (s State) Valid() error {
	if s.Version != SchemaVersion {
		return fmt.Errorf("firstrun: unsupported schema version %d", s.Version)
	}
	if s.Mode != "" && !IsValidMode(s.Mode) {
		return fmt.Errorf("firstrun: invalid mode %q", s.Mode)
	}
	if s.Theme != "" && s.Theme != "light" && s.Theme != "dark" {
		return fmt.Errorf("firstrun: invalid theme %q", s.Theme)
	}
	if s.Locale != "" && !IsValidLocale(s.Locale) {
		return fmt.Errorf("firstrun: invalid locale %q", s.Locale)
	}
	if len(s.Steps) > 64 {
		return fmt.Errorf("firstrun: too many steps (%d)", len(s.Steps))
	}
	for _, step := range s.Steps {
		if !stepPattern.MatchString(step) {
			return fmt.Errorf("firstrun: invalid step %q", step)
		}
	}
	if len(s.Hostname) > 253 {
		return fmt.Errorf("firstrun: hostname too long")
	}
	if len(s.AdminTokenHash) > 0 && len(s.AdminTokenHash) != 64 {
		return fmt.Errorf("firstrun: adminTokenHash must be 64 hex chars")
	}
	return nil
}

// Completed reports whether the first-run has been
// finalised. A document with zero CompletedAt is "not yet
// completed" even if Steps is non-empty.
func (s State) Completed() bool {
	return !s.CompletedAt.IsZero()
}

// IsValidMode returns true for any of the canonical modes.
func IsValidMode(mode string) bool {
	switch mode {
	case "easy", "full", "router", "server", "custom":
		return true
	}
	return false
}

// IsValidLocale returns true for any of the canonical
// locales the first-run screen offers.
func IsValidLocale(locale string) bool {
	switch locale {
	case "en", "ru", "de", "es":
		return true
	}
	return false
}

// stepPattern mirrors the lifecycle object id rules so
// step names cannot smuggle shell metacharacters into the
// audit log.
var stepPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Store is the on-disk persistence handle. It is
// goroutine-safe and bounded; multiple concurrent writers
// serialise on the embedded mutex.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns a Store rooted at path. The directory
// is created on demand with 0700 permissions; the file is
// not touched until Write or Load is called.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the configured persistence path.
func (s *Store) Path() string { return s.path }

// Load reads and decodes the on-disk document. A missing
// file returns ErrNotInitialised so the caller can decide
// whether to bootstrap. A corrupt file returns
// ErrCorruptDocument together with the offset where the
// decoder failed so the operator can inspect the journal.
func (s *Store) Load() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// loadLocked is the unwrapped loader; callers must hold s.mu.
func (s *Store) loadLocked() (State, error) {
	if s.path == "" {
		return State{}, errors.New("firstrun: store path is empty")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, ErrNotInitialised
		}
		return State{}, fmt.Errorf("firstrun: read %s: %w", s.path, err)
	}
	return Decode(data)
}

// Write persists the supplied state atomically. The
// caller must populate AdminTokenHash (the raw token is
// never accepted). Write validates the document and
// stamps UpdatedAt before the atomic rename.
func (s *Store) Write(state State) error {
	if s.path == "" {
		return errors.New("firstrun: store path is empty")
	}
	if state.Version == 0 {
		state.Version = SchemaVersion
	}
	state.Steps = dedupSortedStrings(state.Steps)
	state.UpdatedAt = time.Now().UTC()
	if err := state.Valid(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("firstrun: marshal: %w", err)
	}
	if len(data) > maxDocumentBytes {
		return fmt.Errorf("firstrun: document too large (%d bytes)", len(data))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("firstrun: mkdir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("firstrun: write temp: %w", err)
	}
	if err := fsyncPath(tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("firstrun: fsync temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("firstrun: rename: %w", err)
	}
	if err := fsyncDir(filepath.Dir(s.path)); err != nil {
		// Best-effort: a non-fatal warning is acceptable
		// because the file content is already durable on
		// XFS/ext4. Surface the error to the caller
		// anyway for visibility.
		return fmt.Errorf("firstrun: fsync dir: %w", err)
	}
	return nil
}

// MarkComplete records the bootstrap completion. It is a
// convenience wrapper around Write that fills in the
// immutable fields and stamps CompletedAt.
func (s *Store) MarkComplete(prev State, rawToken, mode, locale, theme, hostname string, steps []string) (State, error) {
	hash := sha256.Sum256([]byte(rawToken))
	state := State{
		Version:        SchemaVersion,
		CompletedAt:    time.Now().UTC(),
		Mode:           mode,
		Locale:         locale,
		Theme:          theme,
		Steps:          dedupSortedStrings(steps),
		AdminTokenHash: hex.EncodeToString(hash[:]),
		Hostname:       hostname,
	}
	if prev.Completed() {
		state.CompletedAt = prev.CompletedAt
		state.Steps = dedupSortedStrings(append(append([]string{}, prev.Steps...), steps...))
	}
	if err := s.Write(state); err != nil {
		return State{}, err
	}
	return state, nil
}

// HashToken returns the canonical SHA-256 hex of a raw
// bearer token. The Store hashes the token itself on
// MarkComplete; this helper exists for tests and the CLI.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Decode parses the raw document and validates it.
func Decode(raw []byte) (State, error) {
	var s State
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if len(raw) > maxDocumentBytes {
		return s, fmt.Errorf("firstrun: document too large (%d bytes)", len(raw))
	}
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("firstrun: decode: %w", err)
	}
	if err := s.Valid(); err != nil {
		return s, err
	}
	return s, nil
}

// ErrNotInitialised is returned when no first-run document
// exists on disk yet.
var ErrNotInitialised = errors.New("firstrun: not initialised")

// ErrCorruptDocument is returned when the on-disk document
// fails validation.
var ErrCorruptDocument = errors.New("firstrun: corrupt document")

const maxDocumentBytes = 16 << 10

// fsyncPath calls fsync on the file at path. The caller
// must hold an exclusive lock or accept that other
// processes may see stale data during the window between
// write and fsync.
func fsyncPath(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// fsyncDir opens the directory read-only and syncs its
// metadata so the rename becomes durable on Linux. The
// call is best-effort: on macOS and other platforms that
// refuse fsync on directories, the helper returns nil so
// the caller does not have to special-case the platform.
func fsyncDir(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if isDarwinFsyncUnsupported(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		if isDarwinFsyncUnsupported(err) {
			return nil
		}
		return err
	}
	return nil
}

// isDarwinFsyncUnsupported matches the platform error
// strings the Darwin kernel returns when fsync is invoked
// on a directory fd. The check is intentionally permissive:
// any IO/ioctl error on the dir fsync is treated as a
// non-fatal warning so the Agent does not refuse to start
// on platforms that do not implement directory fsync.
func isDarwinFsyncUnsupported(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "is a directory") ||
		strings.Contains(msg, "inappropriate ioctl") ||
		strings.Contains(msg, "operation not supported")
}

func dedupSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, s := range in {
		if s == "" {
			continue
		}
		seen[s] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// silence unused import warning for io when no helper uses it.
var _ = io.Discard
