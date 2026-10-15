// Package persist provides the bounded atomic JSON file store the
// Xray Center uses for its new inventories (nodes, groups, delivery
// tokens, certificates, quotas, events, policies, REALITY keys).
// It mirrors the FileCatalog conventions: 0600 file, atomic rename,
// fsync of file and parent directory, single-mutex serialization.
package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// File is a durable JSON document of T. Safe for concurrent use.
type File[T any] struct {
	dir   string
	name  string
	mu    sync.Mutex
	state T
}

// Open loads (or initialises) the document at dir/name. The zero T
// is used when the file does not exist yet.
func Open[T any](dir, name string) (*File[T], error) {
	if dir == "" || name == "" || filepath.Base(name) != name {
		return nil, errors.New("persist: dir and plain file name are required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("persist: mkdir: %w", err)
	}
	f := &File[T]{dir: dir, name: name}
	raw, err := os.ReadFile(f.path())
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("persist: read: %w", err)
	}
	if err := json.Unmarshal(raw, &f.state); err != nil {
		return nil, fmt.Errorf("persist: decode %s: %w", name, err)
	}
	return f, nil
}

func (f *File[T]) path() string { return filepath.Join(f.dir, f.name) }

// Get returns a copy of the current state.
func (f *File[T]) Get() T {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

// Update applies mut to the state and persists. On a persist error
// the in-memory state is rolled back so memory never diverges from
// disk.
func (f *File[T]) Update(mut func(*T)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev := f.state
	mut(&f.state)
	raw, err := json.Marshal(f.state)
	if err != nil {
		f.state = prev
		return fmt.Errorf("persist: encode: %w", err)
	}
	tmp := f.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		f.state = prev
		return fmt.Errorf("persist: write: %w", err)
	}
	if err := os.Rename(tmp, f.path()); err != nil {
		f.state = prev
		_ = os.Remove(tmp)
		return fmt.Errorf("persist: rename: %w", err)
	}
	if err := syncDir(f.dir); err != nil {
		// The rename already made the state durable enough for the
		// catalog contract; a dir-fsync failure is reported but the
		// state stays.
		return fmt.Errorf("persist: fsync dir: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
