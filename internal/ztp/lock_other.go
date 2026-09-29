//go:build !linux

package ztp

import "os"

// On non-Linux platforms the flock(2) inter-process lock
// is unavailable. OpenManager still uses the lockfile as
// a marker so concurrent callers on the same dir get a
// best-effort ErrLocked via file-based O_EXCL semantics.
func (m *Manager) acquireLock() error {
	f, err := os.OpenFile(m.lockPath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return ErrLocked
		}
		return err
	}
	m.lockFile = f
	return nil
}

func (m *Manager) releaseLock() error {
	if m.lockFile == nil {
		return nil
	}
	err := m.lockFile.Close()
	_ = os.Remove(m.lockPath)
	m.lockFile = nil
	return err
}
