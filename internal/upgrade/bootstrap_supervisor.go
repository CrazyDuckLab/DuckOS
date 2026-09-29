package upgrade

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnrollInstalledRelease establishes the signed rollback baseline for an
// already installed release. It never extracts or replaces installed files.
// The offline bootstrap package is provided by the installer/operator, not IPC.
func EnrollInstalledRelease(store *ReleaseStore, keys []ed25519.PublicKey, arch, root string, driver ReleaseRuntime, raw []byte) error {
	pkg, err := Decode(raw, multiKeyVerifier(keys))
	if err != nil {
		return err
	}
	if pkg.Manifest.Architecture != arch || pkg.Manifest.InputSchema != 1 || pkg.Manifest.OutputSchema != 1 || len(pkg.Manifest.Dependencies) != 0 {
		return errors.New("upgrade: incompatible bootstrap package")
	}
	if driver == nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return errors.New("upgrade: invalid bootstrap runtime")
	}
	if err := ownedDirectory(root, 0755); err != nil {
		return err
	}
	selected, err := os.Readlink(filepath.Join(root, "current"))
	if err != nil {
		return err
	}
	id := strings.TrimPrefix(selected, "releases/")
	if selected != "releases/"+id || !validStoredReleaseID(id) {
		return errors.New("upgrade: unsafe baseline selection")
	}
	o, err := OpenDurableOrchestrator(store, keys, arch)
	if err != nil {
		return err
	}
	defer o.Close()
	if o.state.Active != "" || o.workflowPending {
		return errors.New("upgrade: bootstrap cannot replace an existing workflow")
	}
	if existing, err := store.ReadPayload(id); err == nil {
		if string(existing) != string(raw) {
			return errors.New("upgrade: conflicting baseline package")
		}
	} else if err := store.Materialize(ReleaseMeta{ID: id, Architecture: arch, SourceVersion: pkg.Manifest.SourceVersion, TargetVersion: pkg.Manifest.TargetVersion}, raw); err != nil {
		return err
	}
	s := &Supervisor{o: o, store: store, root: root, runtime: driver}
	build, err := s.verifiedBuild(id)
	if err != nil {
		return err
	}
	if err := nativeBuildProbe(filepath.Join(root, "releases", id), build); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := driver.Health(ctx, build); err != nil {
		return err
	}
	o.state.Active = id
	o.state.Candidate = ""
	o.workflowPending = true
	if err := o.persistLocked(); err != nil {
		return err
	}
	if err := store.Activate(id); err != nil {
		return err
	}
	o.workflowPending = false
	o.state.RecoveryRequired = false
	o.state.UpdatedAt = time.Now().UTC()
	o.recordHistoryLocked(HistoryEntry{At: o.state.UpdatedAt, To: id, Result: "health-passed", Channel: ChannelStable, Operator: "bootstrap"})
	return o.persistLocked()
}
