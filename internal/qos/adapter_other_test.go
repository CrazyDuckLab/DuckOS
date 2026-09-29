//go:build !linux

// adapter_other_test.go covers the non-Linux stub.
// The stub refuses to apply or roll back; the Agent
// must surface a clean "unavailable on this platform"
// diagnostic rather than a panic.
package qos

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestLinuxRunnerUnavailableOnNonLinux confirms the
// non-Linux stub returns ErrUnavailable on every
// method. The model and renderer are
// platform-independent and the tests run on Darwin;
// the stub exists to keep the contract honest.
func TestLinuxRunnerUnavailableOnNonLinux(t *testing.T) {
	runner := NewLinuxRunner(nil)
	if _, err := runner.Apply(context.Background(), Plan{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := runner.Rollback(context.Background(), Plan{}, Plan{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if _, err := runner.State(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

// TestLinuxRunnerApplyOutcomeCarriesError verifies
// the ApplyOutcome includes a non-empty Error
// string so the API can surface a meaningful
// message.
func TestLinuxRunnerApplyOutcomeCarriesError(t *testing.T) {
	runner := NewLinuxRunner(nil)
	out, err := runner.Apply(context.Background(), Plan{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if !strings.Contains(out.Error, "unavailable") {
		t.Fatalf("expected outcome to mention unavailability, got %q", out.Error)
	}
}
