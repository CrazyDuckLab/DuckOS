package healthauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const PrivatePath = "/etc/duckos/updater-health.key"
const PublicPath = "/etc/duckos/updater-health.pub"

func readKey(path string, private bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("health key unavailable")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm() != 0600) {
		return nil, errors.New("unsafe health key")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("health key unreadable")
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return nil, errors.New("health key changed")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 130))
	if err != nil {
		return nil, errors.New("health key unreadable")
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid health key")
	}
	return key, nil
}
func LoadPrivate(path string) (ed25519.PrivateKey, error) {
	seed, err := readKey(path, true)
	if err != nil {
		return nil, err
	}
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed), nil
}
func LoadPublic(path string) (ed25519.PublicKey, error) {
	raw, err := readKey(path, false)
	return ed25519.PublicKey(raw), err
}

// Provision creates a new key pair exclusively. Existing files are never
// replaced; interrupted provisioning requires explicit operator recovery.
func Provision(private, public string) error {
	if os.Geteuid() != 0 || filepath.Dir(private) != filepath.Dir(public) {
		return errors.New("health key provisioning requires root and one trusted directory")
	}
	dir, err := os.Open(filepath.Dir(private))
	if err != nil {
		return err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || info.Mode().Perm()&0022 != 0 {
		return errors.New("unsafe health key directory")
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	defer clear(key)
	for _, entry := range []struct {
		path string
		raw  []byte
		mode os.FileMode
	}{{private, key.Seed(), 0600}, {public, pub, 0644}} {
		f, err := os.OpenFile(entry.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, entry.mode)
		if err != nil {
			return err
		}
		if err := f.Chmod(entry.mode); err != nil {
			f.Close()
			return err
		}
		_, writeErr := f.WriteString(hex.EncodeToString(entry.raw) + "\n")
		syncErr := f.Sync()
		closeErr := f.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			return err
		}
		if err := dir.Sync(); err != nil {
			return err
		}
	}
	return nil
}
