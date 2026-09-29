package backup

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecutorRollsBackAfterApplyFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := OpenJournalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := NewExecutor(store)
	if err != nil {
		t.Fatal(err)
	}
	var rolledBack atomic.Bool
	entry := RestoreEntry{ID: "restore-0001", BackupID: "backup-0001", Outcome: OutcomeStarted, StartedAt: time.Now().UTC()}
	plan := RestorePlan{BackupID: "backup-0001", SafeToApply: true}
	_, err = exec.Execute(context.Background(), entry, plan, "rev-1", "rev-1",
		func(context.Context) (any, error) { return "snapshot", nil },
		func(_ context.Context, _ any, d RestoreDirection) error {
			if d == DirectionRollback {
				rolledBack.Store(true)
				return nil
			}
			return errors.New("apply failed")
		})
	if err == nil {
		t.Fatal("apply failure unexpectedly succeeded")
	}
	if !rolledBack.Load() {
		t.Fatal("executor did not call rollback after apply failure")
	}
	if _, ok := store.Get("restore-0001-failed"); !ok {
		t.Fatal("failed transition was not durably recorded")
	}
}

func TestH09RollbackGetsFreshContextAfterCanceledApply(t *testing.T) {
	for _, returnError := range []bool{true, false} {
		t.Run(map[bool]string{true: "apply-error", false: "late-success"}[returnError], func(t *testing.T) {
			store, err := OpenJournalStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			executor, err := NewExecutor(store)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rolledBack := false
			entry := RestoreEntry{ID: "restore-h09-context", BackupID: "backup-h09", Outcome: OutcomeStarted, StartedAt: time.Now().UTC()}
			_, err = executor.Execute(ctx, entry, RestorePlan{BackupID: entry.BackupID, SafeToApply: true}, "rev-1", "rev-1",
				func(context.Context) (any, error) { return "before", nil },
				func(operation context.Context, _ any, direction RestoreDirection) error {
					if direction == DirectionApply {
						cancel()
						if returnError {
							return operation.Err()
						}
						return nil
					}
					if operation.Err() != nil {
						t.Fatal("rollback inherited cancellation")
					}
					deadline, ok := operation.Deadline()
					if !ok || time.Until(deadline) < 25*time.Second {
						t.Fatal("rollback has no fresh bounded budget")
					}
					rolledBack = true
					return nil
				})
			if !errors.Is(err, context.Canceled) || !rolledBack {
				t.Fatalf("canceled apply was not undone: %v, %v", err, rolledBack)
			}
			if _, ok := store.Get(entry.ID + "-applied"); ok {
				t.Fatal("canceled apply reported committed")
			}
		})
	}
}
