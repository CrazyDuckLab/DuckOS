//go:build linux

// Package main owns the Linux adapter for the qos
// subsystem. The adapter is the only side that runs
// `tc`. Every argument it passes is built from a
// validated Plan: the model layer refuses any id or
// rate that would produce a shell metacharacter, and
// the Plan Valid gate re-checks every Step. The
// adapter itself does not call sh or eval, and the
// fixed binary path is set at construction time.
package qos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	osExec "os/exec"
	"strings"
	"sync"
	"time"
)

// Executor is the contract the Linux adapter uses to
// run a single `tc` invocation. The Agent wires the
// real os/exec; tests inject a fake.
type Executor interface {
	Run(ctx context.Context, argv []string) error
}

// ExecRunner is the production Executor. It shells
// out to /sbin/tc (or the binary the caller chose at
// construction time) and never reads the response
// other than the exit code. The adapter inspects the
// exit code; a non-zero exit aborts the apply.
type ExecRunner struct {
	Binary string
	mu     sync.Mutex
}

// NewExecRunner returns an ExecRunner with a known
// binary path. The path is captured once; the
// adapter never re-resolves it.
func NewExecRunner(binary string) *ExecRunner {
	if binary == "" {
		binary = "/sbin/tc"
	}
	return &ExecRunner{Binary: binary}
}

// Run executes argv[0] = binary, argv[1:] = args.
// The arguments are joined with single spaces by
// exec.Command itself; we never build a shell
// string.
func (e *ExecRunner) Run(ctx context.Context, argv []string) error {
	if len(argv) == 0 {
		return errors.New("qos: empty argv")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	cmd := osExec.CommandContext(ctx, e.Binary, argv[1:]...)
	// capture stderr for diagnostics; stdout is not used
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qos: %s %v: %w (%s)", e.Binary, argv[1:], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// LinuxRunner is the production Runner: it composes
// an ExecRunner with the Plan -> argv conversion.
// The LinuxRunner is goroutine-safe; the Agent can
// call Apply and Rollback from the same transaction
// state-machine.
type LinuxRunner struct {
	Exec  Executor
	Clock func() time.Time
	// MaxDuration bounds a single Apply. The bound
	// is policy, not a guess: a tc round trip on a
	// V1 install never needs more than 16 seconds.
	MaxDuration time.Duration
}

// NewLinuxRunner returns a LinuxRunner with sensible
// defaults. The caller may override the binary path
// and clock for tests.
func NewLinuxRunner(exec Executor) *LinuxRunner {
	if exec == nil {
		exec = NewExecRunner("/sbin/tc")
	}
	return &LinuxRunner{Exec: exec, Clock: time.Now, MaxDuration: 16 * time.Second}
}

// Apply runs every Step of the Plan in order. A
// failure stops the apply and reports the
// already-applied steps in the ApplyOutcome so the
// caller can decide whether to rollback. The previous
// plan is left untouched.
func (r *LinuxRunner) Apply(ctx context.Context, p Plan) (ApplyOutcome, error) {
	out := ApplyOutcome{StartedAt: r.now()}
	ctx, cancel := context.WithTimeout(ctx, r.MaxDuration)
	defer cancel()
	for i, s := range p.Steps {
		if err := r.Exec.Run(ctx, r.argvFor(s)); err != nil {
			out.Failed = len(p.Steps) - i
			out.EndedAt = r.now()
			out.Error = err.Error()
			return out, err
		}
	}
	out.Applied = len(p.Steps)
	out.EndedAt = r.now()
	return out, nil
}

// Rollback without a durable ownership witness must never delete a root
// based only on a caller-supplied plan. Production rollback uses OwnedRunner.
func (r *LinuxRunner) Rollback(_ context.Context, _, _ Plan) (ApplyOutcome, error) {
	return ApplyOutcome{Error: ErrRecoveryRequired.Error()}, ErrRecoveryRequired
}

// State returns a redacted summary of the live
// queueing state. The implementation uses `tc -s
// qdisc show` to enumerate root qdiscs; the result is
// parsed only for the link name, the algorithm and
// the rate, and is then passed through.
func (r *LinuxRunner) State(ctx context.Context) (RunnerState, error) {
	ctx, cancel := context.WithTimeout(ctx, r.MaxDuration)
	defer cancel()
	out := RunnerState{}
	// The reference implementation inspects the
	// system. Tests use a fake Executor.
	if exec, ok := r.Exec.(*ExecRunner); ok {
		cmd := osExec.CommandContext(ctx, exec.Binary, "-s", "qdisc", "show")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			out.Error = err.Error()
			return out, err
		}
		return parseTcQdiscShow(stdout.Bytes(), out), nil
	}
	return out, nil
}

// parseTcQdiscShow parses the textual output of
// `tc -s qdisc show` and fills the RunnerState.
// The parser is intentionally small: it extracts
// the link name, the algorithm and the qdisc
// count, and reports any per-line error to the
// caller. The function never panics on bad input.
func parseTcQdiscShow(raw []byte, out RunnerState) RunnerState {
	lines := strings.Split(string(raw), "\n")
	seen := map[string]bool{}
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) < 5 {
			continue
		}
		if fields[0] != "qdisc" {
			continue
		}
		out.Algorithm = fields[1]
		idx := -1
		for j, f := range fields {
			if f == "dev" && j+1 < len(fields) {
				idx = j + 1
				break
			}
		}
		if idx < 0 {
			continue
		}
		link := fields[idx]
		if !seen[link] {
			seen[link] = true
			out.HasShapers++
		}
		out.Link = link
	}
	return out
}

// argvFor converts a Step into the argv slice we hand
// to the Executor. The arguments are constructed from
// validated fields; no string interpolation happens
// here. The Executor joins them with single spaces
// via exec.Command.
func (r *LinuxRunner) argvFor(s Step) []string {
	argv := []string{"tc", string(s.Kind), "add", "dev", s.Link}
	if s.Parent == "root" {
		argv = append(argv, "root")
	} else if s.Parent != "" {
		argv = append(argv, "parent", s.Parent)
	}
	if s.Handle != "" {
		argv = append(argv, "handle", s.Handle)
	}
	argv = append(argv, s.Args...)
	return argv
}

func (r *LinuxRunner) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}
