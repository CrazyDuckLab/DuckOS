package qos

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type finalCancelRunner struct {
	cancel             context.CancelFunc
	rollbackErr        error
	rollbackContextErr error
}

func (r *finalCancelRunner) Apply(context.Context, Plan) (ApplyOutcome, error) {
	r.cancel()
	return ApplyOutcome{}, errors.New("apply interrupted")
}
func (r *finalCancelRunner) Rollback(ctx context.Context, _, _ Plan) (ApplyOutcome, error) {
	r.rollbackContextErr = ctx.Err()
	return ApplyOutcome{}, r.rollbackErr
}
func (r *finalCancelRunner) State(context.Context) (RunnerState, error) { return RunnerState{}, nil }

func TestFinalVerificationQoSRollbackCancellationAndTruth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &finalCancelRunner{cancel: cancel, rollbackErr: errors.New("restore failed")}
	c, err := NewController(r)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Apply(ctx, samplePlan(t))
	if err == nil {
		t.Fatal("failed runtime reported success")
	}
	if r.rollbackContextErr != nil {
		t.Error("rollback inherited canceled request")
	}
	if strings.Contains(strings.Join(out.Warnings, " "), "apply reverted via guarded rollback") {
		t.Error("failed rollback reported restoration")
	}
}
