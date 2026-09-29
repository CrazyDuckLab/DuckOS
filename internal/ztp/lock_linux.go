//go:build linux

package ztp

import (
	"os"
	"syscall"
)

// acquireLock takes an exclusive flock(2) on the per-dir
// lock file. A second OpenManager call on the same dir
// returns ErrLocked so the contract never silently
// accepts a second writer.
func (m *Manager) acquireLock() error {
	f, err := os.OpenFile(m.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return ErrLocked
	}
	m.lockFile = f
	return nil
}

func (m *Manager) releaseLock() error {
	if m.lockFile == nil {
		return nil
	}
	// Best-effort: unlock before close so other processes
	// can observe the release immediately.
	_ = syscall.Flock(int(m.lockFile.Fd()), syscall.LOCK_UN)
	err := m.lockFile.Close()
	m.lockFile = nil
	return err
}
