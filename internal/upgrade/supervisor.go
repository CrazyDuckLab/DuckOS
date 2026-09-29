package upgrade

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ReleaseRuntime is implemented by the fixed systemd service driver. Release
// identifiers and executable paths are never provided to it by IPC callers.
type ReleaseRuntime interface {
	Restart(context.Context) error
	Stop(context.Context) error
	Health(context.Context, BuildIdentity) error
}

type BuildIdentity struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Target  string `json:"target"`
}

// Supervisor runs outside the replaceable release and retains the workflow
// writer lock when managed services restart or their requesting Agent exits.
type Supervisor struct {
	mu      sync.Mutex
	o       *Orchestrator
	store   *ReleaseStore
	root    string
	runtime ReleaseRuntime
}

func OpenSupervisor(store *ReleaseStore, keys []ed25519.PublicKey, arch, root string, driver ReleaseRuntime) (*Supervisor, error) {
	if driver == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errors.New("upgrade: trusted runtime configuration required")
	}
	if err := ownedDirectory(root, 0755); err != nil {
		return nil, err
	}
	if err := ownedDirectory(filepath.Join(root, "releases"), 0755); err != nil {
		return nil, err
	}
	o, err := OpenDurableOrchestrator(store, keys, arch)
	if err != nil {
		return nil, err
	}
	s := &Supervisor{o: o, store: store, root: root, runtime: driver}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := s.recover(ctx); err != nil {
		_ = o.Close()
		return nil, err
	}
	return s, nil
}

func (s *Supervisor) Close() error  { s.mu.Lock(); defer s.mu.Unlock(); return s.o.Close() }
func (s *Supervisor) State() State  { return s.o.State() }
func (s *Supervisor) Pending() bool { s.o.mu.Lock(); defer s.o.mu.Unlock(); return s.o.workflowPending }

func (s *Supervisor) Stage(plan Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.o.mu.Lock()
	available := s.o.mutationAvailableLocked()
	s.o.mu.Unlock()
	if !available {
		return ErrRecoveryRequired
	}
	// The common preflight authenticates the complete archive before native
	// candidate validation or any effect on the live selection.
	pre := NewPreflight(s.store, s.o.keys, s.o.arch, nil)
	if result := pre.Run(plan); result.Refused {
		return errors.New("upgrade: package preflight refused: " + result.Reason)
	}
	pkg, err := Decode(plan.Payload, multiKeyVerifier(s.o.keys))
	if err != nil {
		return err
	}
	if pkg.Manifest.InputSchema != 1 || pkg.Manifest.OutputSchema != 1 || len(pkg.Manifest.Dependencies) != 0 {
		return errors.New("upgrade: schema/host migration is not supported by this activation path")
	}
	if err := s.prepare(plan.Release.ID, pkg); err != nil {
		return err
	}
	return s.o.Stage(plan)
}

func (s *Supervisor) Activate(ctx context.Context) error { return s.transition(ctx, false) }
func (s *Supervisor) Rollback(ctx context.Context) error { return s.transition(ctx, true) }

func (s *Supervisor) transition(ctx context.Context, rollback bool) (result error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.o
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	if !o.mutationAvailableLocked() {
		o.mu.Unlock()
		return ErrRecoveryRequired
	}
	target := o.state.Candidate
	if rollback {
		target = o.state.Previous
	}
	if target == "" {
		o.mu.Unlock()
		return errors.New("upgrade: no target release")
	}
	before := o.state
	o.mu.Unlock()
	build, err := s.verifiedBuild(target)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Active remains the committed before-image throughout provisional
	// activation. Restart therefore always knows which release to restore.
	o.mu.Lock()
	o.workflowPending = true
	if err := o.persistLocked(); err != nil {
		o.state.RecoveryRequired = true
		o.mu.Unlock()
		return err
	}
	o.mu.Unlock()
	if err = s.selectRelease(target); err == nil {
		err = s.runtime.Restart(ctx)
	}
	if err == nil {
		err = s.runtime.Health(ctx, build)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = s.store.Activate(target)
	}
	if err != nil {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		recoveryErr := s.restoreRuntime(recoveryCtx, before.Active)
		o.mu.Lock()
		defer o.mu.Unlock()
		o.state = before
		o.workflowPending = recoveryErr != nil
		o.state.RecoveryRequired = recoveryErr != nil
		o.recordHistoryLocked(HistoryEntry{At: time.Now().UTC(), From: before.Active, To: target, Result: "activation-failed", Channel: before.Channel, Operator: "system"})
		return errors.Join(err, recoveryErr, o.persistLocked())
	}
	// Journal the confirmed build before updating the metadata anchor. If
	// interrupted here the pending journal still causes rollback, not guessing.
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.Active = target
	o.state.Previous = before.Active
	o.state.Candidate = ""
	o.state.RecoveryRequired = false
	o.workflowPending = false
	o.state.UpdatedAt = time.Now().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.state.UpdatedAt, From: before.Active, To: target, Result: "health-passed", Channel: before.Channel, Operator: "system"})
	if err := o.persistLocked(); err != nil {
		o.state.RecoveryRequired = true
		return errors.Join(err, ErrRecoveryRequired)
	}
	return nil
}

func (s *Supervisor) recover(ctx context.Context) error {
	o := s.o
	if !o.workflowPending && o.state.Active == "" {
		if _, err := os.Lstat(filepath.Join(s.root, "current")); !errors.Is(err, os.ErrNotExist) {
			return errors.New("upgrade: unmanaged current selection")
		}
	}
	if o.workflowPending {
		if err := s.restoreRuntime(ctx, o.state.Active); err != nil {
			return errors.Join(ErrRecoveryRequired, err)
		}
		o.workflowPending = false
		o.state.RecoveryRequired = false
		return o.persistLocked()
	}
	if o.state.Active != "" {
		build, err := s.verifiedBuild(o.state.Active)
		if err != nil {
			return err
		}
		selected, selectErr := os.Readlink(filepath.Join(s.root, "current"))
		if err = s.runtime.Health(ctx, build); err != nil || selectErr != nil || selected != filepath.Join("releases", o.state.Active) {
			if err = s.restoreRuntime(ctx, o.state.Active); err != nil {
				return errors.Join(ErrRecoveryRequired, err)
			}
		}
		o.state.RecoveryRequired = false
		return o.persistLocked()
	}
	return nil
}

func (s *Supervisor) restoreRuntime(ctx context.Context, id string) error {
	if id == "" {
		if err := s.runtime.Stop(ctx); err != nil {
			return err
		}
		path := filepath.Join(s.root, "current")
		if _, err := os.Readlink(path); err == nil {
			if err = os.Remove(path); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := syncReleaseDir(s.root); err != nil {
			return err
		}
		return s.store.Deactivate()
	}
	build, err := s.verifiedBuild(id)
	if err != nil {
		return err
	}
	if err = s.selectRelease(id); err != nil {
		return err
	}
	if err = s.runtime.Restart(ctx); err != nil {
		return err
	}
	if err = s.runtime.Health(ctx, build); err != nil {
		return err
	}
	return s.store.Activate(id)
}

func (s *Supervisor) selectRelease(id string) error {
	if !validStoredReleaseID(id) {
		return errors.New("upgrade: invalid release identity")
	}
	tmp, err := os.CreateTemp(s.root, ".selection-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	defer os.Remove(name)
	if err = os.Symlink(filepath.Join("releases", id), name); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(s.root, "current")); err != nil {
		return err
	}
	return syncReleaseDir(s.root)
}
