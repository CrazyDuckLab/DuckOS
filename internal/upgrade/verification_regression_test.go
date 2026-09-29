package upgrade

import (
	"context"
	"testing"
)

type verificationRunner func(HealthCheck) error

func (f verificationRunner) Run(c HealthCheck) error { return f(c) }

func TestVerificationCanceledOrMissingHealthRunnerCannotSucceed(t *testing.T) {
	check := HealthCheck{Name: "agent", Command: "status"}
	if got := CancellableHealthCheck(context.Background(), nil, check); got.Passed || got.Error == "" {
		t.Fatal("missing runner accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := make(chan struct{}, 1)
	got := CancellableHealthCheck(ctx, verificationRunner(func(HealthCheck) error { called <- struct{}{}; return nil }), check)
	if got.Passed || got.Error == "" {
		t.Fatal("canceled health check accepted")
	}
	select {
	case <-called:
		t.Fatal("already canceled check started work")
	default:
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	got = CancellableHealthCheck(ctx, verificationRunner(func(HealthCheck) error { cancel(); return nil }), check)
	if got.Passed {
		t.Fatal("late success overrode cancellation")
	}
}
