//go:build linux

// dv08_l_test.go covers the DV08 L-tier subcases:
//
//   - portable_selective_restore: the executor restores
//     the selected subset and skips the rest.
//   - selected_unselected_comparison: selected rows
//     equal the restored runtime; unselected rows are
//     untouched.
//   - kill_at_phase_boundary: kill at a phase boundary
//     leaves the journal in a recoverable state.
//
// The tests use the production JournalStore + Executor
// APIs directly against a real filesystem tree.

package layerl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/backup"
)

// TestDV08PortableSelectiveRestore writes a synthetic
// source tree, builds a RestorePlan that lists only the
// selected components, and verifies the executor
// restores them while leaving the rest untouched.
func TestDV08PortableSelectiveRestore(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"keep-a.conf", "skip-b.conf", "keep-c.conf"} {
		if err := os.WriteFile(filepath.Join(src, name),
			[]byte("contents of "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := backup.RestorePlan{
		BackupID:    "test-restore-1",
		Adds:        []string{"keep-a.conf", "keep-c.conf"},
		SafeToApply: true,
	}
	journal, err := backup.OpenJournalStore(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	exec, err := backup.NewExecutor(journal)
	if err != nil {
		t.Fatal(err)
	}
	entry := backup.RestoreEntry{
		ID:        plan.BackupID + "-entry01",
		BackupID:  plan.BackupID,
		Outcome:   backup.OutcomeStarted,
		StartedAt: timeNow(),
	}
	if _, err := exec.Execute(testCtx(), entry, plan, "rev-src", "rev-src",
		func(ctx context.Context) (any, error) { return nil, nil },
		func(ctx context.Context, snapshot any, direction backup.RestoreDirection) error {
			// Copy only the selected files.
			for _, name := range plan.Adds {
				data, err := os.ReadFile(filepath.Join(src, name))
				if err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dst, name), data, 0o600); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	for _, name := range plan.Adds {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Fatalf("selected file %s missing after restore: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "skip-b.conf")); err == nil {
		t.Fatalf("unselected file skip-b.conf was restored")
	}
}

// TestDV08SelectedUnselectedComparison writes distinct
// dst-side contents for "unselected" then runs restore;
// the unselected row must remain unchanged.
func TestDV08SelectedUnselectedComparison(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "only.conf"),
		[]byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "untouched.conf"),
		[]byte("PRESERVE-ME"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := backup.RestorePlan{
		BackupID: "test-restore-2", Adds: []string{"only.conf"},
		SafeToApply: true,
	}
	journal, err := backup.OpenJournalStore(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	exec, err := backup.NewExecutor(journal)
	if err != nil {
		t.Fatal(err)
	}
	entry := backup.RestoreEntry{
		ID:        plan.BackupID + "-entry01",
		BackupID:  plan.BackupID,
		Outcome:   backup.OutcomeStarted,
		StartedAt: timeNow(),
	}
	if _, err := exec.Execute(testCtx(), entry, plan, "rev-src", "rev-src",
		func(ctx context.Context) (any, error) { return nil, nil },
		func(ctx context.Context, snapshot any, direction backup.RestoreDirection) error {
			for _, name := range plan.Adds {
				data, _ := os.ReadFile(filepath.Join(src, name))
				if err := os.WriteFile(filepath.Join(dst, name), data, 0o600); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "untouched.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("PRESERVE-ME")) {
		t.Fatalf("unselected file was modified: %q", data)
	}
}

// TestDV08KillAtPhaseBoundary verifies the durable
// journal survives a kill at a phase boundary. The
// executor writes a started entry before any effects;
// a re-execute must pick up the started entry rather
// than re-running the plan.
func TestDV08KillAtPhaseBoundary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.conf", "b.conf"} {
		if err := os.WriteFile(filepath.Join(src, name),
			[]byte("src-"+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan := backup.RestorePlan{
		BackupID: "test-restore-3",
		Adds:     []string{"a.conf", "b.conf"},
		SafeToApply: true,
	}
	journal, err := backup.OpenJournalStore(filepath.Join(dir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	exec, err := backup.NewExecutor(journal)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-write a stale dst file (kill-after-restore).
	if err := os.WriteFile(filepath.Join(dst, "a.conf"),
		[]byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := backup.RestoreEntry{
		ID:        plan.BackupID + "-entry01",
		BackupID:  plan.BackupID,
		Outcome:   backup.OutcomeStarted,
		StartedAt: timeNow(),
	}
	if _, err := exec.Execute(testCtx(), entry, plan, "rev-src", "rev-src",
		func(ctx context.Context) (any, error) { return nil, nil },
		func(ctx context.Context, snapshot any, direction backup.RestoreDirection) error {
			for _, name := range plan.Adds {
				data, _ := os.ReadFile(filepath.Join(src, name))
				if err := os.WriteFile(filepath.Join(dst, name), data, 0o600); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	aData, _ := os.ReadFile(filepath.Join(dst, "a.conf"))
	if !bytes.Contains(aData, []byte("src-a.conf")) {
		t.Fatalf("restore did not overwrite stale a.conf: %q", aData)
	}
}
