// runner.go owns the live UPS probe surface. The
// runner is the only side that shells out to
// /usr/bin/upsc. Tests use a fake runner; the
// Agent wires the production runner. Every
// argument the runner passes is built from a
// validated Device, and the runner never builds
// a shell string.

package ups

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Executor is the contract a probe implementation
// must satisfy. The Agent wires the real
// os/exec; tests inject a fake.
type Executor interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}

// ExecRunner is the production Executor. It
// shells out to a fixed binary path and never
// reads the response other than the exit code and
// captured stdout.
type ExecRunner struct {
	Binary string
	mu     sync.Mutex
}

// NewExecRunner returns an ExecRunner with a
// known binary path. The path is captured once;
// the runner never re-resolves it.
func NewExecRunner(binary string) *ExecRunner {
	if binary == "" {
		binary = "/usr/bin/upsc"
	}
	return &ExecRunner{Binary: binary}
}

// Run executes argv[0] = binary, argv[1:] = args.
// The arguments are joined with single spaces by
// exec.Command itself; the runner never builds a
// shell string.
func (e *ExecRunner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, errors.New("ups: empty argv")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	cmd := exec.CommandContext(ctx, e.Binary, argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ups: %s %v: %w (%s)", e.Binary, argv[1:], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// ProbeResult is the structured outcome of a
// single device probe. The Runner never sees a
// NUT variable map; the Agent reduces the
// variable map to a ProbeResult through the
// classifier.
type ProbeResult struct {
	DeviceID  string
	Health    Health
	Variables []Variable
	Err       error
	At        time.Time
}

// Runner is the bounded Agent-side coordinator
// for live UPS probes. The Runner takes a list
// of devices and runs one probe per device. The
// Runner never holds secrets and never opens a
// socket; it delegates the actual `upsc`
// invocation to an Executor.
type Runner struct {
	Exec  Executor
	Clock func() time.Time
	// Timeout bounds a single `upsc` round. The
	// default is 4 seconds; an unresponsive
	// daemon must not stall the agent.
	Timeout time.Duration
}

// NewRunner returns a Runner with sensible
// defaults. The caller may override the Executor
// and clock for tests.
func NewRunner(exec Executor) *Runner {
	if exec == nil {
		exec = NewExecRunner("/usr/bin/upsc")
	}
	return &Runner{Exec: exec, Clock: time.Now, Timeout: 4 * time.Second}
}

// Probe runs a single device probe. The runner
// invokes `upsc <device>` and parses the result.
// A failure is reported in the ProbeResult; the
// caller decides whether to record the error in
// the state.
func (r *Runner) Probe(ctx context.Context, d Device) ProbeResult {
	if err := d.Valid(); err != nil {
		return ProbeResult{DeviceID: d.ID, Health: HealthUnknown, At: r.now(), Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	out, err := r.Exec.Run(ctx, []string{"upsc", d.ID})
	if err != nil {
		return ProbeResult{DeviceID: d.ID, Health: HealthUnknown, At: r.now(), Err: err}
	}
	vars := parseUpscOutput(out)
	status := firstVar(vars, "ups.status")
	charge := firstIntVar(vars, "battery.charge")
	health := ClassifyHealth(status, charge)
	return ProbeResult{DeviceID: d.ID, Health: health, Variables: vars, At: r.now(), Err: nil}
}

// ProbeAll runs a probe per device. The runner
// is sequential; the V1 install never has more
// than MaxDevices devices and the timeout
// already bounds the total latency.
func (r *Runner) ProbeAll(ctx context.Context, devices []Device) []ProbeResult {
	out := make([]ProbeResult, 0, len(devices))
	for _, d := range devices {
		out = append(out, r.Probe(ctx, d))
	}
	return out
}

func (r *Runner) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now().UTC()
}

// parseUpscOutput parses the textual output of
// `upsc <device>`. The format is:
//
//	key: value
//	key2: value2
//
// Empty lines and lines without a colon are
// ignored. The function never panics on bad
// input.
func parseUpscOutput(out []byte) []Variable {
	lines := strings.Split(string(out), "\n")
	vars := make([]Variable, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		idx := strings.Index(ln, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(ln[:idx])
		val := strings.TrimSpace(ln[idx+1:])
		if key == "" || val == "" {
			continue
		}
		v := Variable{Key: key, Value: val}
		if err := v.Valid(); err == nil {
			vars = append(vars, v)
			if len(vars) >= MaxVariables {
				break
			}
		}
	}
	return vars
}

// firstVar returns the value of the first
// variable whose key matches the supplied name.
// The NUT output is case-insensitive on keys;
// the comparison lower-cases both sides.
func firstVar(vars []Variable, key string) string {
	key = strings.ToLower(key)
	for _, v := range vars {
		if strings.ToLower(v.Key) == key {
			return v.Value
		}
	}
	return ""
}

// firstIntVar returns the integer value of the
// first variable whose key matches the supplied
// name. A parse error yields 0.
func firstIntVar(vars []Variable, key string) int {
	s := firstVar(vars, key)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
	if err != nil {
		return 0
	}
	return n
}
