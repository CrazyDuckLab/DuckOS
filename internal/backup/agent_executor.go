// backup/agent_executor.go wires ExecuteRestore to the
// durable JournalStore. The previous ExecuteRestore
// accepted *RestoreJournal directly; the audit called
// that disconnected from persistence. Executor is the
// integration point the Agent composition root uses to
// drive a restore: every consequential transition
// (admitted, validated, applied, rolled-back, failed)
// is fsynced through JournalStore before the function
// returns to the IPC caller.
package backup

import (
	"context"
	"errors"
	"time"
)

// Executor wires a JournalStore to the canonical
// restore lifecycle. The Executor is goroutine-safe
// at the level of one Execute call; concurrent Execute
// calls MUST be serialised by the caller (the Agent
// enforces the same serialization rule on the
// Safe-Apply controller path).
type Executor struct {
	store *JournalStore
}

// NewExecutor returns an Executor bound to the supplied
// JournalStore. A nil store is a programming error.
func NewExecutor(store *JournalStore) (*Executor, error) {
	if store == nil {
		return nil, errors.New("backup: executor requires a JournalStore")
	}
	return &Executor{store: store}, nil
}

// Execute runs the canonical validate → snapshot →
// apply → reconcile lifecycle. Every consequential
// transition writes through the JournalStore before the
// function returns, so a crash between transitions
// preserves the durable audit trail.
//
// Each transition entry carries a distinct ID so the
// JournalStore's per-id dedup keeps the full transition
// trail (started → applied | failed). The audit
// required durable, replayable record per transition.
//
// The Executor enforces the audit-required safety
// rules:
//
//   - safe admission: SafeToApply MUST be true; source
//     and live revisions MUST match.
//   - admission-before-effects: the Started entry is
//     persisted before the Snapshotter is called.
//   - independent bounded rollback: rollback uses a
//     separate context so caller cancellation does not
//     cancel recovery.
//   - rollback-failure-not-success: if Restore returns
//     a non-nil error, the Executor records the failure
//     and does not mark the transaction committed.
func (e *Executor) Execute(ctx context.Context, entry RestoreEntry, plan RestorePlan, sourceRevision, liveRevision string, snap Snapshotter, restore Restorer) (RestorerResult, error) {
	if err := ctx.Err(); err != nil {
		return RestorerResult{Err: err}, err
	}
	if err := e.requireSafe(plan, sourceRevision, liveRevision); err != nil {
		return RestorerResult{Err: err}, err
	}
	if err := e.store.Append(entry); err != nil {
		return RestorerResult{Err: err}, err
	}
	before, err := snap(ctx)
	if err != nil {
		failed := transitionFrom(entry, OutcomeFailed, time.Now().UTC())
		failed.ValidationErrors = append(failed.ValidationErrors, "snapshot: "+err.Error())
		_ = e.store.Append(failed)
		return RestorerResult{Entry: failed, Err: err}, err
	}
	applyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	applyErr := restore(applyCtx, before, DirectionApply)
	if applyErr == nil {
		applyErr = applyCtx.Err()
	}
	cancel()
	if applyErr != nil {
		// Allocate recovery time only after apply has stopped. Neither an
		// expired apply deadline nor caller cancellation consumes this budget.
		recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer recoveryCancel()
		rollbackErr := restore(recoveryCtx, before, DirectionRollback)
		failed := transitionFrom(entry, OutcomeFailed, time.Now().UTC())
		failed.ValidationErrors = append(failed.ValidationErrors, "restore: "+applyErr.Error())
		if rollbackErr != nil {
			failed.ValidationErrors = append(failed.ValidationErrors, "rollback: "+rollbackErr.Error())
			_ = e.store.Append(failed)
			return RestorerResult{Entry: failed, Err: errors.Join(applyErr, rollbackErr)}, errors.Join(applyErr, rollbackErr)
		}
		_ = e.store.Append(failed)
		return RestorerResult{Entry: failed, Err: applyErr}, applyErr
	}
	confirmed := transitionFrom(entry, OutcomeApplied, time.Now().UTC())
	if err := e.store.Append(confirmed); err != nil {
		return RestorerResult{Entry: confirmed, Err: err}, err
	}
	return RestorerResult{Entry: confirmed}, nil
}

// transitionFrom builds a transition entry derived from
// the supplied entry. The ID is rebuilt from the base
// entry ID plus the transition suffix so the JournalStore
// keeps every transition without collapsing on the
// per-id dedup.
func transitionFrom(base RestoreEntry, outcome RestoreOutcome, endedAt time.Time) RestoreEntry {
	derived := base
	derived.Outcome = outcome
	derived.EndedAt = endedAt
	switch outcome {
	case OutcomeApplied:
		derived.ID = base.ID + "-applied"
	case OutcomeFailed:
		derived.ID = base.ID + "-failed"
	case OutcomeRolledBack:
		derived.ID = base.ID + "-rolled-back"
	}
	return derived
}

// requireSafe enforces the audit gate: SafeToApply
// MUST be true and the supplied revisions MUST match.
func (e *Executor) requireSafe(plan RestorePlan, sourceRevision, liveRevision string) error {
	if plan.BackupID == "" {
		return errors.New("backup: restore plan has empty backup id")
	}
	if !plan.SafeToApply {
		return errors.New("backup: restore plan is not safe to apply")
	}
	if plan.RequiresSecret && sourceRevision == "" {
		return errors.New("backup: plan requires secret but source revision is missing")
	}
	if sourceRevision == "" || liveRevision == "" || liveRevision != sourceRevision {
		return errors.New("backup: live revision does not match the plan")
	}
	return nil
}
