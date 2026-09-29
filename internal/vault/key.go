package vault

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"os"
	"syscall"
)

const masterFilename = "master.key"

// MasterKey belongs to the privileged key custodian. File-backed unattended boot
// does not protect against an attacker who can read both key and encrypted data.
type MasterKey struct{ data []byte }

func (k MasterKey) String() string               { return "[installation master key]" }
func (k MasterKey) GoString() string             { return k.String() }
func (k MasterKey) LogValue() slog.Value         { return slog.StringValue(k.String()) }
func (k MasterKey) MarshalJSON() ([]byte, error) { return nil, ErrRecord }
func (k *MasterKey) Clear()                      { clear(k.data); k.data = nil }

// OpenVault confines the raw master-key handoff to the encrypted store.
func (k *MasterKey) OpenVault(path string) (*Store, error) { return OpenStore(path, k.data) }
func (k *MasterKey) InitializeVault(path string) (*Store, error) {
	return InitializeStore(path, k.data)
}

func keyDirectory(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, ErrRecord
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return nil, ErrRecord
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrRecord
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		root.Close()
		return nil, ErrRecord
	}
	return root, nil
}

// ProvisionMasterKey requires an empty, pre-created 0700 directory belonging to
// the serving identity. It never replaces an existing key or repairs a partial
// key. The Linux Agent must call it as root only during explicit provisioning.
func ProvisionMasterKey(path string) (*MasterKey, error) {
	root, err := keyDirectory(path)
	if err != nil {
		return nil, ErrRecord
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, ErrRecord
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1)
	if !errors.Is(err, io.EOF) || len(entries) != 0 {
		return nil, ErrRecord
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, ErrRecord
	}
	ok := false
	defer func() {
		if !ok {
			clear(key)
		}
	}()
	f, err := root.OpenFile(masterFilename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, ErrRecord
	}
	// A failed write deliberately leaves evidence in place. Never overwrite a
	// potentially durable key with fresh random material on retry.
	n, writeErr := f.Write(key)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(key) || syncErr != nil || closeErr != nil || dir.Sync() != nil {
		return nil, ErrRecord
	}
	ok = true
	return &MasterKey{data: key}, nil
}

func LoadMasterKey(path string) (*MasterKey, error) {
	root, err := keyDirectory(path)
	if err != nil {
		return nil, ErrRecord
	}
	defer root.Close()
	info, err := root.Lstat(masterFilename)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 32 {
		return nil, ErrRecord
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() || st.Nlink != 1 {
		return nil, ErrRecord
	}
	f, err := root.Open(masterFilename)
	if err != nil {
		return nil, ErrRecord
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Mode().Perm() != 0600 || opened.Size() != 32 {
		return nil, ErrRecord
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 || bytes.Equal(key, make([]byte, 32)) {
		clear(key)
		return nil, ErrRecord
	}
	return &MasterKey{data: key}, nil
}
