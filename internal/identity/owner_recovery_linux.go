package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// RecoverOwner is offline, local recovery. The API must be stopped first: its
// exclusive writer lock prevents replacing identities under a running service.
// Only the canonical identity file is replaced; no service commands are run.
func RecoverOwner(path, backups string) (backup string, err error) {
	if os.Geteuid() != 0 {
		return "", errors.New("Owner recovery requires effective root")
	}
	return recoverOwner(path, backups)
}

func recoverOwner(path, backups string) (backup string, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("identity directory must be private and not a symlink")
	}
	owner := info.Sys().(*syscall.Stat_t)
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	lock, err := root.OpenFile("writer.lock", os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	li, err := lock.Stat()
	if err != nil {
		return "", err
	}
	if !li.Mode().IsRegular() || li.Sys().(*syscall.Stat_t).Nlink != 1 {
		return "", errors.New("unsafe identity lock")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return "", errors.New("stop duck-api before Owner recovery")
	}
	if err = os.Mkdir(backups, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	bi, err := os.Lstat(backups)
	if err != nil {
		return "", err
	}
	if !bi.IsDir() || bi.Mode().Perm()&0077 != 0 || int(bi.Sys().(*syscall.Stat_t).Uid) != os.Geteuid() {
		return "", errors.New("backup directory must be private and owned by recovery operator")
	}
	if err = syncRecoveryDir(filepath.Dir(backups)); err != nil {
		return "", err
	}
	backup, err = os.MkdirTemp(backups, "owner-"+time.Now().UTC().Format("20060102T150405Z")+"-")
	if err != nil {
		return "", err
	}
	// A completed manifest and directory fsync precede any canonical mutation.
	dir, err := root.Open(".")
	if err != nil {
		return backup, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return backup, err
	}
	hashes := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if name != "state.json" && name != "writer.lock" && name != "state.tmp" && name != "owner-recovery.tmp" {
			return backup, fmt.Errorf("unrecognized identity entry %q; refusing incomplete backup", name)
		}
		f, e := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if e != nil {
			return backup, e
		}
		fi, e := f.Stat()
		if e != nil {
			f.Close()
			return backup, e
		}
		if !fi.Mode().IsRegular() || fi.Size() > 32<<20 || fi.Sys().(*syscall.Stat_t).Nlink != 1 {
			f.Close()
			return backup, errors.New("unsafe identity file")
		}
		raw, e := io.ReadAll(io.LimitReader(f, 32<<20+1))
		f.Close()
		if e != nil {
			return backup, e
		}
		if len(raw) > 32<<20 {
			return backup, errors.New("identity file too large")
		}
		if e = writeRecoveryFile(filepath.Join(backup, name), raw); e != nil {
			return backup, e
		}
		digest := sha256.Sum256(raw)
		hashes[name] = hex.EncodeToString(digest[:])
	}
	if _, ok := hashes["state.json"]; !ok {
		return backup, errors.New("existing identity state required")
	}
	manifest, _ := json.Marshal(struct {
		Source string
		SHA256 map[string]string
	}{path, hashes})
	if err = writeRecoveryFile(filepath.Join(backup, "manifest.json"), manifest); err != nil {
		return backup, err
	}
	if err = syncRecoveryDir(backup); err != nil {
		return backup, err
	}
	if err = syncRecoveryDir(backups); err != nil {
		return backup, err
	}
	// Commit in a private staging store. A crash here leaves the live store intact.
	stage, err := os.MkdirTemp(backup, "stage-")
	if err != nil {
		return backup, err
	}
	raw, err := os.ReadFile(filepath.Join(backup, "state.json"))
	if err != nil {
		return backup, err
	}
	if err = writeRecoveryFile(filepath.Join(stage, "state.json"), raw); err != nil {
		return backup, err
	}
	s, err := Open(stage)
	if err != nil {
		return backup, err
	}
	next := cloneDB(s.db)
	next.OwnerRecovery = true
	next.Users = nil
	for _, u := range s.db.Users {
		if u.User.Role != Owner {
			next.Users = append(next.Users, u)
		}
	}
	actor := randomID()
	if len(actor) != 32 {
		s.Close()
		return backup, errors.New("recovery entropy unavailable")
	}
	err = s.save(next, actor, "owner.recovery", actor)
	s.Close()
	if err != nil {
		return backup, err
	}
	raw, err = os.ReadFile(filepath.Join(stage, "state.json"))
	if err != nil {
		return backup, err
	}
	// A previous interrupted pre-rename temp is safe to replace under the lock.
	if err = root.Remove("owner-recovery.tmp"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return backup, err
	}
	f, err := root.OpenFile("owner-recovery.tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return backup, err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Chown(int(owner.Uid), int(owner.Gid))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return backup, err
	}
	if err = root.Rename("owner-recovery.tmp", "state.json"); err != nil {
		return backup, err
	}
	if err = dir.Sync(); err != nil {
		return backup, fmt.Errorf("identity commit durability uncertain; backup %s: %w", backup, err)
	}
	return backup, nil
}

func writeRecoveryFile(path string, raw []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	e := f.Close()
	if err == nil {
		err = e
	}
	return err
}
func syncRecoveryDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
