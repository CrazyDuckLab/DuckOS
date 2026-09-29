package backup

import (
	"context"
	"testing"
)

func TestVerificationUnsafeAdditionAndOpaqueRevisionRejected(t *testing.T) {
	p := validRestorePlan()
	p.SafeToApply = false
	p.Adds = []string{"network.firewall.rule/allow-wan"}
	if len(ValidateRestorePlan(p, "rev-1", "rev-1")) == 0 {
		t.Error("unsafe addition accepted")
	}
	p = validRestorePlan()
	for _, pair := range [][2]string{{"rev-9", "rev-10"}, {"ffffffff", "aaaaaaaa"}, {"", "rev-1"}, {"rev-1", ""}} {
		if len(ValidateRestorePlan(p, pair[0], pair[1])) == 0 {
			t.Errorf("nonmatching/missing revision accepted: %q", pair)
		}
	}
}

func TestVerificationRollbackSurvivesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rolledBack := false
	result := ExecuteRestore(ctx, NewRestoreJournal(), newRestoreEntry("restore-cancel", "backup-1"), validRestorePlan(), "rev-1", "rev-1",
		func(context.Context) (any, error) { return "before", nil },
		func(ctx context.Context, _ any, direction RestoreDirection) error {
			if direction == DirectionApply {
				cancel()
				return ctx.Err()
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, bounded := ctx.Deadline(); !bounded {
				t.Error("rollback has no deadline")
			}
			rolledBack = true
			return nil
		})
	if !rolledBack || result.Entry.Outcome != OutcomeRolledBack {
		t.Fatal("canceled apply prevented rollback")
	}
}

func TestVerificationRestoreRejectsCanceledRequestAndWrongBackup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx context.Context
		id  string
	}{{ctx, "backup-1"}, {nil, "backup-1"}, {context.Background(), "other-backup"}} {
		result := ExecuteRestore(tc.ctx, NewRestoreJournal(), newRestoreEntry("restore-invalid", tc.id), validRestorePlan(), "rev-1", "rev-1",
			func(context.Context) (any, error) { t.Fatal("invalid restore reached snapshot"); return nil, nil },
			func(context.Context, any, RestoreDirection) error {
				t.Fatal("invalid restore reached mutation")
				return nil
			})
		if result.Err == nil {
			t.Fatal("invalid restore accepted")
		}
	}
}
