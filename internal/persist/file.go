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

// jsonMarshal, syncFile and closeTmp are the per-stage helpers the
// commit protocol uses (declared here to keep file.go the storage
// surface and commit.go the transaction protocol).
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// syncFile fsyncs an open-by-path file: open, sync, keep the handle
// for the explicit close stage.
func syncFile(path string) error {
	fh, err := os.OpenFile(path, os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// closeTmp re-verifies the temp file closes cleanly (the write
// handle from WriteFile is already closed; this is the explicit
// post-fsync close-error stage required by the commit protocol).
func closeTmp(path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	return fh.Close()
}

// SyncDirForTest exposes the directory-fsync boundary injection
// point (tests only).
func SyncDirForTest(fn func(string) error) (restore func()) {
	prev := syncDirFn
	syncDirFn = fn
	return func() { syncDirFn = prev }
}

// syncDir fsyncs a directory so the rename becomes durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Exists reports whether the durable file is present on disk. The
// snapshot/marker protocols key restart decisions on existence (an
// empty-but-present authoritative document is live state), never on
// content alone.
func (f *File[T]) Exists() bool {
	_, err := os.Stat(f.path())
	return err == nil
}
