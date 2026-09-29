//go:build !linux

// xray/runtime_other.go returns a refusal
// constructor on non-Linux platforms. The
// real Linux implementation lives in
// runtime_linux.go and is selected through
// the build tag above.
package xray

import "context"

// ErrUnsupported is returned on non-Linux
// platforms.
var ErrUnsupported = errorString("xray: runtime is Linux-only")

type errorString string

func (e errorString) Error() string { return string(e) }

// Runtime is a stub on non-Linux.
type Runtime struct{}

// NewRuntime refuses on non-Linux.
func NewRuntime() (*Runtime, error) { return nil, ErrUnsupported }

// State is a stub.
type State struct{ Active bool }

// State is a stub.
func (r *Runtime) State() State { return State{} }

// Apply is a stub.
func (r *Runtime) Apply(ctx context.Context, cfg Config) error { return ErrUnsupported }

// Stop is a stub.
func (r *Runtime) Stop() error { return ErrUnsupported }

// Rollback is a stub.
func (r *Runtime) Rollback() error { return ErrUnsupported }
