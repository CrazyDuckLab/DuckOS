package upgrade

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

var ErrRecoveryRequired = errors.New("upgrade: recovery_required")

type workflowJournal struct {
	Version int   `json:"version"`
	State   State `json:"state"`
	Pending bool  `json:"pending"`
}

// OpenDurableOrchestrator owns one writer for the workflow. Interrupted
// activation is never promoted to healthy by reading release metadata.
func OpenDurableOrchestrator(store *ReleaseStore, keys []ed25519.PublicKey, arch string) (*Orchestrator, error) {
	if store == nil {
		return nil, errors.New("upgrade: release store required")
	}
	lock, err := os.OpenFile(filepath.Join(store.dir, "workflow.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	o := NewOrchestrator(store, keys, arch, nil)
	o.workflowPath = filepath.Join(store.dir, "workflow.json")
	o.workflowLock = lock
	ok := false
	defer func() {
		if !ok {
			_ = o.Close()
		}
	}()
	f, err := os.OpenFile(o.workflowPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err == nil {
		info, statErr := f.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			f.Close()
			return nil, ErrRecoveryRequired
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		var journal workflowJournal
		if readErr != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &journal) != nil || journal.Version != 1 || !journal.State.Channel.Valid() || !journal.State.Policy.Valid() || len(journal.State.History) > MaxHistory {
			return nil, ErrRecoveryRequired
		}
		for _, id := range []string{journal.State.Active, journal.State.Previous, journal.State.Candidate} {
			if id != "" && store.VerifyFingerprint(id) != nil {
				return nil, ErrRecoveryRequired
			}
		}
		o.state = journal.State
		o.workflowPending = journal.Pending
		if !journal.Pending {
			active, readErr := os.Readlink(filepath.Join(store.dir, "active"))
			if errors.Is(readErr, os.ErrNotExist) {
				active = ""
				readErr = nil
			}
			if readErr != nil || active != o.state.Active {
				return nil, ErrRecoveryRequired
			}
		}
		// No runtime witness is available at this layer. Persisted Active and
		// interrupted phases require runtime reconciliation before mutations.
		if journal.Pending || o.state.Active != "" {
			o.state.RecoveryRequired = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		// Existing selections without their journal are not a new installation.
		for _, name := range []string{"active", "previous"} {
			if _, err := os.Lstat(filepath.Join(store.dir, name)); !errors.Is(err, os.ErrNotExist) {
				return nil, ErrRecoveryRequired
			}
		}
	}
	if err := o.persistLocked(); err != nil {
		return nil, err
	}
	ok = true
	return o, nil
}

func (o *Orchestrator) Close() error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.workflowLock == nil {
		return nil
	}
	err := o.workflowLock.Close()
	o.workflowLock = nil
	return err
}

func (o *Orchestrator) persistLocked() error {
	if o.workflowPath == "" {
		return nil
	}
	if o.workflowLock == nil {
		return ErrRecoveryRequired
	}
	raw, err := json.Marshal(workflowJournal{Version: 1, State: o.state, Pending: o.workflowPending})
	if err != nil || len(raw) > 1<<20 {
		return ErrRecoveryRequired
	}
	f, err := os.CreateTemp(filepath.Dir(o.workflowPath), ".workflow-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), o.workflowPath); err != nil {
		return err
	}
	return syncReleaseDir(filepath.Dir(o.workflowPath))
}

func (o *Orchestrator) persistOutcomeLocked(outcome *error) {
	if o.workflowPath != "" && o.workflowPending {
		o.state.RecoveryRequired = true
		*outcome = errors.Join(*outcome, ErrRecoveryRequired)
	}
	if err := o.persistLocked(); err != nil {
		o.state.RecoveryRequired = true
		*outcome = errors.Join(*outcome, ErrRecoveryRequired, err)
	}
}

func (o *Orchestrator) mutationAvailableLocked() bool {
	return !o.state.RecoveryRequired && (o.workflowPath == "" || (o.workflowLock != nil && !o.workflowPending))
}
