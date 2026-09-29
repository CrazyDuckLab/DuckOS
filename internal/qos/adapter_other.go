//go:build !linux

// Package main owns the non-Linux stub for the qos
// LinuxRunner. The Agent refuses to instantiate the
// stub on Linux: a non-Linux build cannot apply tc
// plans and the constructor reflects that.
package qos

import (
	"context"
	"time"
)

// Executor is a no-op placeholder on non-Linux
// builds. The model layer and renderer are
// platform-independent; only the runner is gated.
type Executor interface {
	Run(ctx context.Context, argv []string) error
}

// LinuxRunner is the non-Linux stub. Calling Apply
// or Rollback returns ErrUnavailable so the Agent
// can surface "qos requires linux" to the operator
// without crashing.
type LinuxRunner struct{}

// NewLinuxRunner returns the non-Linux stub. The
// exec argument is ignored; the caller passes nil on
// non-Linux builds.
func NewLinuxRunner(_ Executor) *LinuxRunner { return &LinuxRunner{} }

// Apply is not supported on non-Linux platforms.
func (r *LinuxRunner) Apply(_ context.Context, _ Plan) (ApplyOutcome, error) {
	return ApplyOutcome{StartedAt: time.Unix(0, 0), EndedAt: time.Unix(0, 0), Error: ErrUnavailable.Error()}, ErrUnavailable
}

// Rollback is not supported on non-Linux platforms.
func (r *LinuxRunner) Rollback(_ context.Context, _ Plan, _ Plan) (ApplyOutcome, error) {
	return ApplyOutcome{StartedAt: time.Unix(0, 0), EndedAt: time.Unix(0, 0), Error: ErrUnavailable.Error()}, ErrUnavailable
}

// State is not supported on non-Linux platforms.
func (r *LinuxRunner) State(_ context.Context) (RunnerState, error) {
	return RunnerState{Error: ErrUnavailable.Error()}, ErrUnavailable
}
