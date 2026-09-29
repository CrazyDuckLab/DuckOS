// upgrade/healthcheck.go: V1 health-check
// executor. The executor runs a list of
// HealthChecks against the running Agent and
// returns a verdict. The orchestrator wires
// the verdict into the automatic
// previous-release rollback.
//
// The executor is a thin layer over an
// injected Runner; no shell is invoked by
// the package itself.
package upgrade

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// CheckRunner runs a single HealthCheck. The
// Agent supplies the implementation: command
// probe, HTTP probe, or service health probe.
// The runner must not block longer than its
// own per-check timeout; the executor adds a
// global timeout envelope.
type CheckRunner interface {
	Run(check HealthCheck) error
}

// CheckResult is the outcome of one check.
type CheckResult struct {
	Name    string        `json:"name"`
	Passed  bool          `json:"passed"`
	Error   string        `json:"error,omitempty"`
	Elapsed time.Duration `json:"elapsed"`
}

var checkNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Executor is the V1 health-check executor.
type Executor struct {
	mu      sync.Mutex
	runner  CheckRunner
	timeout time.Duration
}

// NewExecutor returns an Executor that uses
// the supplied runner. timeout is the
// per-check timeout; zero means no per-check
// timeout.
func NewExecutor(runner CheckRunner, timeout time.Duration) *Executor {
	if runner == nil {
		runner = noopRunner{}
	}
	return &Executor{runner: runner, timeout: timeout}
}

// RunAll runs every check serially and
// returns the per-check result plus a
// summary. The first failing check aborts
// the run. The function never panics on a
// failing runner; the error is recorded in
// the per-check result.
func (e *Executor) RunAll(checks []HealthCheck) ([]CheckResult, bool) {
	ordered := append([]HealthCheck(nil), checks...)
	sort.Slice(ordered, func(i, k int) bool { return ordered[i].Name < ordered[k].Name })
	results := make([]CheckResult, 0, len(ordered))
	allPassed := len(ordered) > 0
	for _, c := range ordered {
		if !checkNamePattern.MatchString(c.Name) {
			results = append(results, CheckResult{Name: c.Name, Passed: false, Error: "name is not canonical"})
			allPassed = false
			continue
		}
		if err := c.Validate(); err != nil {
			results = append(results, CheckResult{Name: c.Name, Passed: false, Error: err.Error()})
			allPassed = false
			continue
		}
		started := time.Now()
		runErr := e.runOne(c)
		elapsed := time.Since(started)
		cr := CheckResult{Name: c.Name, Passed: runErr == nil, Elapsed: elapsed}
		if runErr != nil {
			cr.Error = runErr.Error()
			allPassed = false
		}
		results = append(results, cr)
		if runErr != nil {
			break
		}
	}
	return results, allPassed
}

func (e *Executor) runOne(c HealthCheck) error {
	if e.timeout == 0 {
		return e.runner.Run(c)
	}
	done := make(chan error, 1)
	go func() { done <- e.runner.Run(c) }()
	select {
	case err := <-done:
		return err
	case <-time.After(e.timeout):
		return errors.New("upgrade: check exceeded timeout")
	}
}

type noopRunner struct{}

func (noopRunner) Run(_ HealthCheck) error { return errors.New("upgrade: health runner unavailable") }
