// Regression test for B-013 (BETA.2): the launchers declare
// After=duck-agent.service, so at boot the launcher sockets may not
// exist yet when the agent's one-shot WireGuard startup reconciliation
// dials them. Before the fix a single dial failure left the WireGuard
// service disabled for the whole boot. reconcileWithBackoff must retry
// a failing reconciliation and succeed once the dependency appears;
// exhausted attempts must surface the last error.

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReconcileWithBackoffRetriesUntilSuccess(t *testing.T) {
	calls := 0
	original := reconcileBackoff
	reconcileBackoff = []time.Duration{0, 0, 0, 0, 0}
	defer func() { reconcileBackoff = original }()
	err := reconcileWithBackoff(func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("dial unix /run/duckos-agent/wg-launcher.sock: connect: no such file or directory")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestReconcileWithBackoffExhaustsAndReturnsLastError(t *testing.T) {
	original := reconcileBackoff
	reconcileBackoff = []time.Duration{0, 0, 0, 0, 0}
	defer func() { reconcileBackoff = original }()
	calls := 0
	sentinel := errors.New("dial unix /run/duckos-agent/wg-launcher.sock: connect: connection refused")
	err := reconcileWithBackoff(func(ctx context.Context) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected last error surfaced, got %v", err)
	}
	if calls != len(reconcileBackoff) {
		t.Fatalf("expected %d attempts, got %d", len(reconcileBackoff), calls)
	}
}

func TestReconcileWithBackoffContextBound(t *testing.T) {
	original := reconcileBackoff
	reconcileBackoff = []time.Duration{0}
	defer func() { reconcileBackoff = original }()
	err := reconcileWithBackoff(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(45 * time.Second):
			return nil
		}
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected per-attempt 30s deadline, got %v", err)
	}
}
