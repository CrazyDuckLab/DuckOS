// H11 backup/restore lifecycle torture regression.
package backup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func newRestoreEntry(id, backupID string) RestoreEntry {
	return RestoreEntry{
		ID:        id,
		BackupID:  backupID,
		Outcome:   OutcomeStarted,
		StartedAt: time.Now().UTC(),
	}
}

func validRestorePlan() RestorePlan {
	return RestorePlan{
		BackupID:    "backup-1",
		SafeToApply: true,
	}
}

func TestRestoreEntry_Validate(t *testing.T) {
	e := newRestoreEntry("restore-001", "backup-1")
	if err := e.Valid(); err != nil {
		t.Fatalf("valid entry: %v", err)
	}
	bad := newRestoreEntry("X", "backup-1")
	if err := bad.Valid(); err == nil {
		t.Fatalf("bad id must be rejected")
	}
	bad2 := newRestoreEntry("restore-002", "")
	if err := bad2.Valid(); err == nil {
		t.Fatalf("missing backupId must be rejected")
	}
	bad3 := newRestoreEntry("restore-003", "backup-1")
	bad3.StartedAt = time.Time{}
	if err := bad3.Valid(); err == nil {
		t.Fatalf("zero startedAt must be rejected")
	}
}

func TestRestoreOutcome_Valid(t *testing.T) {
	for _, o := range []RestoreOutcome{OutcomeStarted, OutcomeValidated, OutcomeApplied, OutcomeRolledBack, OutcomeFailed} {
		if !o.Valid() {
			t.Errorf("%q must be valid", o)
		}
	}
	if RestoreOutcome("garbage").Valid() {
		t.Errorf("garbage must be invalid")
	}
}

func TestJournal_AppendAndGet(t *testing.T) {
	j := NewRestoreJournal()
	if err := j.Append(newRestoreEntry("restore-001", "backup-1")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, ok := j.Get("restore-001")
	if !ok {
		t.Fatalf("entry must be retrievable")
	}
	if got.BackupID != "backup-1" {
		t.Fatalf("drift: %+v", got)
	}
}

func TestJournal_RejectsInvalid(t *testing.T) {
	j := NewRestoreJournal()
	bad := newRestoreEntry("X", "backup-1")
	if err := j.Append(bad); err == nil {
		t.Fatalf("invalid entry must be rejected")
	}
}

func TestJournal_TrimToMax(t *testing.T) {
	j := NewRestoreJournal()
	j.SetMax(3)
	for i := 0; i < 5; i++ {
		_ = j.Append(newRestoreEntry("restore-"+string(rune('a'+i)), "backup-1"))
	}
	if len(j.All()) != 3 {
		t.Fatalf("expected 3 entries after trim, got %d", len(j.All()))
	}
}

func TestJournal_Counters(t *testing.T) {
	j := NewRestoreJournal()
	e1 := newRestoreEntry("restore-001", "backup-1")
	e1.Outcome = OutcomeApplied
	e2 := newRestoreEntry("restore-002", "backup-1")
	e2.Outcome = OutcomeFailed
	_ = j.Append(e1)
	_ = j.Append(e2)
	c := j.Counters()
	if c.Total != 2 {
		t.Fatalf("counter drift: %+v", c)
	}
	if c.ByOutcome[OutcomeApplied] != 1 {
		t.Fatalf("applied counter drift: %+v", c)
	}
}

func TestValidateRestorePlan_RejectsMissingBackupID(t *testing.T) {
	plan := validRestorePlan()
	plan.BackupID = ""
	failures := ValidateRestorePlan(plan, "rev-1", "rev-1")
	if len(failures) == 0 {
		t.Fatalf("empty backupId must yield a failure")
	}
}

func TestValidateRestorePlan_RejectsUnsafeRemove(t *testing.T) {
	plan := validRestorePlan()
	plan.SafeToApply = false
	plan.Removes = []string{"network.interface/eth0"}
	failures := ValidateRestorePlan(plan, "rev-1", "rev-1")
	if len(failures) == 0 {
		t.Fatalf("unsafe remove must yield a failure")
	}
}

func TestValidateRestorePlan_RejectsUnsafeModify(t *testing.T) {
	plan := validRestorePlan()
	plan.SafeToApply = false
	plan.Modifies = []string{"network.interface/eth0"}
	failures := ValidateRestorePlan(plan, "rev-1", "rev-1")
	if len(failures) == 0 {
		t.Fatalf("unsafe modify must yield a failure")
	}
}

func TestValidateRestorePlan_RejectsSecretWithoutSource(t *testing.T) {
	plan := validRestorePlan()
	plan.RequiresSecret = true
	failures := ValidateRestorePlan(plan, "", "rev-1")
	if len(failures) == 0 {
		t.Fatalf("requiresSecret without source revision must yield a failure")
	}
}

func TestValidateRestorePlan_RejectsRevisionMismatch(t *testing.T) {
	plan := validRestorePlan()
	failures := ValidateRestorePlan(plan, "rev-1", "rev-9")
	if len(failures) == 0 {
		t.Fatalf("revision mismatch must yield a failure")
	}
	if !containsRevisionFailure(failures) {
		t.Fatalf("revision mismatch must surface the sentinel failure")
	}
}

func TestExecuteRestore_Success(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return "snapshot", nil }
	restorer := func(_ context.Context, _ any, _ RestoreDirection) error { return nil }
	e := newRestoreEntry("restore-success", "backup-1")
	entry := ExecuteRestore(context.Background(), j, e, validRestorePlan(), "rev-1", "rev-1", snap, restorer)
	if entry.Err != nil {
		t.Fatalf("ExecuteRestore: %v", entry.Err)
	}
	if entry.Entry.Outcome != OutcomeApplied {
		t.Fatalf("outcome drift: %s", entry.Entry.Outcome)
	}
	// Journal records every transition.
	all := j.All()
	if len(all) < 1 {
		t.Fatalf("journal must carry at least one entry")
	}
}

func TestExecuteRestore_ValidationFailureRefuses(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return "snapshot", nil }
	restorer := func(_ context.Context, _ any, _ RestoreDirection) error { return nil }
	plan := validRestorePlan()
	plan.SafeToApply = false
	plan.Removes = []string{"x"}
	e := newRestoreEntry("restore-validate-fail", "backup-1")
	res := ExecuteRestore(context.Background(), j, e, plan, "rev-1", "rev-1", snap, restorer)
	if !errors.Is(res.Err, ErrRestoreValidation) {
		t.Fatalf("expected ErrRestoreValidation, got %v", res.Err)
	}
	if res.Entry.Outcome != OutcomeFailed {
		t.Fatalf("outcome drift: %s", res.Entry.Outcome)
	}
}

func TestExecuteRestore_RevisionMismatchReturnsRevisionErr(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return "snapshot", nil }
	restorer := func(_ context.Context, _ any, _ RestoreDirection) error { return nil }
	e := newRestoreEntry("restore-revision-mismatch", "backup-1")
	res := ExecuteRestore(context.Background(), j, e, validRestorePlan(), "rev-1", "rev-9", snap, restorer)
	if !errors.Is(res.Err, ErrRevisionMismatch) {
		t.Fatalf("expected ErrRevisionMismatch, got %v", res.Err)
	}
	if res.Entry.Outcome != OutcomeFailed {
		t.Fatalf("outcome drift: %s", res.Entry.Outcome)
	}
}

func TestExecuteRestore_ApplyFailureRollsBack(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return "snapshot", nil }
	var directionCalls atomic.Int32
	restorer := func(_ context.Context, _ any, d RestoreDirection) error {
		directionCalls.Add(1)
		if d == DirectionApply {
			return errors.New("apply failed")
		}
		return nil
	}
	e := newRestoreEntry("restore-apply-fail", "backup-1")
	res := ExecuteRestore(context.Background(), j, e, validRestorePlan(), "rev-1", "rev-1", snap, restorer)
	if res.Err == nil {
		t.Fatalf("expected apply error")
	}
	if res.Entry.Outcome != OutcomeRolledBack {
		t.Fatalf("outcome must be rolled-back, got %s", res.Entry.Outcome)
	}
	if directionCalls.Load() != 2 {
		t.Fatalf("rollback must run after apply, calls=%d", directionCalls.Load())
	}
}

func TestExecuteRestore_RollbackFailureSurfacesBothErrors(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return "snapshot", nil }
	restorer := func(_ context.Context, _ any, d RestoreDirection) error {
		if d == DirectionApply {
			return errors.New("apply boom")
		}
		return errors.New("rollback boom")
	}
	e := newRestoreEntry("restore-double-fail", "backup-1")
	res := ExecuteRestore(context.Background(), j, e, validRestorePlan(), "rev-1", "rev-1", snap, restorer)
	if !errors.Is(res.Err, ErrRestoreExecution) {
		t.Fatalf("expected ErrRestoreExecution, got %v", res.Err)
	}
	if res.Entry.Outcome != OutcomeFailed {
		t.Fatalf("outcome must be failed, got %s", res.Entry.Outcome)
	}
	if res.Entry.Reason == "" {
		t.Fatalf("failed entry must carry a reason")
	}
}

func TestExecuteRestore_NilCallbacksRejected(t *testing.T) {
	j := NewRestoreJournal()
	res := ExecuteRestore(context.Background(), j, newRestoreEntry("restore-nil", "backup-1"), validRestorePlan(), "rev-1", "rev-1", nil, nil)
	if res.Err == nil {
		t.Fatalf("nil callbacks must be rejected")
	}
}

func TestExecuteRestore_NilJournalRejected(t *testing.T) {
	res := ExecuteRestore(context.Background(), nil, newRestoreEntry("restore-nil-journal", "backup-1"), validRestorePlan(), "rev-1", "rev-1", nil, nil)
	if res.Err == nil {
		t.Fatalf("nil journal must be rejected")
	}
}

func TestExecuteRestore_SnapshotFailureRecorded(t *testing.T) {
	j := NewRestoreJournal()
	snap := func(_ context.Context) (any, error) { return nil, errors.New("snap failed") }
	restorer := func(_ context.Context, _ any, _ RestoreDirection) error { return nil }
	e := newRestoreEntry("restore-snap-fail", "backup-1")
	res := ExecuteRestore(context.Background(), j, e, validRestorePlan(), "rev-1", "rev-1", snap, restorer)
	if res.Err == nil {
		t.Fatalf("expected snapshot error")
	}
	if res.Entry.Outcome != OutcomeFailed {
		t.Fatalf("outcome must be failed, got %s", res.Entry.Outcome)
	}
}
