//go:build linux

// adapter_linux_test.go covers the Linux runner with
// a fake Executor. Real tc execution is exercised
// under tests/integration/qos_linux_test.go against
// a real Debian 13 host; this file is the unit-level
// contract coverage.
package qos

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeExec records every Run call. The Runner must
// never execute anything that did not pass through
// this struct.
type fakeExec struct {
	mu      sync.Mutex
	calls   [][]string
	failOn  int
	results []error
}

func (f *fakeExec) Run(_ context.Context, argv []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := len(f.calls)
	f.calls = append(f.calls, append([]string(nil), argv...))
	if f.failOn > 0 && idx+1 == f.failOn {
		if idx < len(f.results) {
			return f.results[idx]
		}
		return errors.New("synthetic fail")
	}
	if idx < len(f.results) {
		return f.results[idx]
	}
	return nil
}

func (f *fakeExec) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = append([]string(nil), c...)
	}
	return out
}

// planFor returns a small Plan suitable for the
// adapter tests. The Plan is built by the renderer
// so the adapter test does not rely on hand-rolled
// internals.
func planFor(t *testing.T) Plan {
	t.Helper()
	sh := Shaper{ID: "wan1", Link: "wan1", Direction: DirectionEgress, Algorithm: AlgorithmCAKE, RateKbps: 1000, BurstKbyte: 32, Priority: PriorityVoice}
	tp, err := Build([]Shaper{sh}, nil, nil, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	p, err := Render(tp)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return p
}

// TestLinuxRunnerApply exercises the happy path:
// every Step produces a single Exec call with a
// fixed argv shape and no shell metacharacters.
func TestLinuxRunnerApply(t *testing.T) {
	exec := &fakeExec{}
	runner := &LinuxRunner{Exec: exec, Clock: func() time.Time { return time.Unix(0, 0) }, MaxDuration: time.Second}
	out, err := runner.Apply(context.Background(), planFor(t))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if out.Applied == 0 {
		t.Fatalf("expected applied > 0")
	}
	if out.Error != "" {
		t.Fatalf("expected no error, got %q", out.Error)
	}
	calls := exec.Calls()
	if len(calls) != out.Applied {
		t.Fatalf("expected %d calls, got %d", out.Applied, len(calls))
	}
	// First call must be a qdisc add on dev wan1.
	first := calls[0]
	if first[0] != "tc" || first[1] != "qdisc" || first[2] != "add" || first[3] != "dev" || first[4] != "wan1" {
		t.Fatalf("unexpected first call: %v", first)
	}
	for _, c := range calls {
		for _, a := range c {
			if a == "" {
				t.Fatalf("empty argument in call: %v", c)
			}
			if strings.ContainsAny(a, " \t\n\r\"';\\") {
				t.Fatalf("shell metacharacter in argv: %v", c)
			}
		}
	}
}

// TestLinuxRunnerApplyReportsFailureOnFirstError
// covers the error path: the first failing Step
// stops the apply and the outcome reports the
// remaining steps as failed.
func TestLinuxRunnerApplyReportsFailureOnFirstError(t *testing.T) {
	exec := &fakeExec{failOn: 1, results: []error{errors.New("synthetic")}}
	runner := &LinuxRunner{Exec: exec, Clock: func() time.Time { return time.Unix(0, 0) }, MaxDuration: time.Second}
	p := planFor(t)
	out, err := runner.Apply(context.Background(), p)
	if err == nil {
		t.Fatalf("expected apply to fail")
	}
	if out.Applied != 0 {
		t.Fatalf("expected applied=0, got %d", out.Applied)
	}
	if out.Failed != len(p.Steps) {
		t.Fatalf("expected failed=%d, got %d", len(p.Steps), out.Failed)
	}
	if !strings.Contains(out.Error, "synthetic") {
		t.Fatalf("expected error to mention synthetic, got %q", out.Error)
	}
}

// A plan alone is not ownership evidence; raw rollback must make no calls.
// Actual teardown/reapply and failure recovery are covered by OwnedRunner tests.
func TestLinuxRunnerRollbackRequiresDurableOwnership(t *testing.T) {
	executor := &fakeExec{}
	runner := NewLinuxRunner(executor)
	out, err := runner.Rollback(context.Background(), planFor(t), planFor(t))
	if !errors.Is(err, ErrRecoveryRequired) || out.Applied != 0 || len(executor.Calls()) != 0 {
		t.Fatalf("unproven ownership permitted mutation: outcome=%+v err=%v", out, err)
	}
}

// TestLinuxRunnerArgvShape checks the argv shape
// for a qdisc and a class. The renderer never
// produces a Step with an empty Parent/Handle, but
// the adapter must still copy whatever the Step
// carries.
func TestLinuxRunnerArgvShape(t *testing.T) {
	exec := &fakeExec{}
	runner := &LinuxRunner{Exec: exec, Clock: func() time.Time { return time.Unix(0, 0) }, MaxDuration: time.Second}
	steps := []Step{
		{Kind: StepQdisc, Link: "wan1", Parent: "root", Handle: "root", Args: []string{"cake", "bandwidth=1000kbit"}},
		{Kind: StepClass, Link: "wan1", Parent: "root", Handle: "1:1", Args: []string{"cake", "diffservvoice"}},
		{Kind: StepFilter, Link: "wan1", Args: []string{"protocol", "ip", "fwmark", "0x7/0xff", "flowid", "1:1"}},
	}
	for _, s := range steps {
		argv := runner.argvFor(s)
		if argv[0] != "tc" {
			t.Fatalf("expected argv[0]=tc, got %q", argv[0])
		}
		if argv[1] != string(s.Kind) || argv[2] != "add" || argv[3] != "dev" || argv[4] != s.Link {
			t.Fatalf("unexpected argv prefix: %v", argv)
		}
	}
}

// TestNewExecRunnerDefaultsToSbinTc covers the
// default binary path: the production constructor
// must produce a usable ExecRunner.
func TestNewExecRunnerDefaultsToSbinTc(t *testing.T) {
	exec := NewExecRunner("")
	if exec.Binary == "" {
		t.Fatalf("expected non-empty default binary path")
	}
}

// TestLinuxRunnerStateFailsCleanly ensures a State
// call without an ExecRunner still returns a
// structured RunnerState. The Agent surfaces the
// state to the operator; the field contract must
// hold even on the empty path.
func TestLinuxRunnerStateFailsCleanly(t *testing.T) {
	runner := &LinuxRunner{Exec: &fakeExec{}}
	got, err := runner.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if got.HasShapers != 0 {
		t.Fatalf("expected hasShapers=0, got %d", got.HasShapers)
	}
}

// TestLinuxRunnerContextCancelled covers the
// propagation of the context deadline: a tc round
// trip past the MaxDuration is reported as a
// failure.
func TestLinuxRunnerContextCancelled(t *testing.T) {
	exec := &fakeExec{results: []error{context.Canceled}}
	runner := &LinuxRunner{Exec: exec, Clock: func() time.Time { return time.Unix(0, 0) }, MaxDuration: time.Second}
	_, err := runner.Apply(context.Background(), planFor(t))
	if err == nil {
		t.Fatalf("expected apply to fail on cancelled context")
	}
}
