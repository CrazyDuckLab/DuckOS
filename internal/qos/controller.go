// controller.go defines the bounded contract the
// platform-specific adapter must satisfy. The Agent is
// the only side that holds a Controller; the API/CLI
// never touches the live tc surface.
//
// A Controller is a small, audit-friendly object: it
// takes a Plan, applies it through a Runner, and
// remembers the previous plan so Safe Apply can roll
// back. A Controller is goroutine-safe: the Agent can
// call Apply and Rollback from the same transaction
// state-machine without an extra mutex.

package qos

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ApplyOutcome reports what the Runner actually did.
// The Agent only trusts the Runner when both the run
// returned nil and the post-state is what the plan
// asked for.
type ApplyOutcome struct {
	Applied   int       `json:"applied"`
	Failed    int       `json:"failed"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
	Warnings  []string  `json:"warnings,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Runner is the contract a platform-specific adapter
// must satisfy. The runner is the only side that
// performs the live mutation; tests use a fake. A nil
// Runner is treated as "always unavailable" and the
// Controller refuses to apply or roll back.
type Runner interface {
	// Apply runs the supplied Plan. The Runner is
	// responsible for verifying post-state; the
	// Controller only inspects the outcome.
	Apply(ctx context.Context, p Plan) (ApplyOutcome, error)
	// Rollback restores the supplied previous plan
	// (or a teardown of the current one when
	// previous is empty).
	Rollback(ctx context.Context, p Plan, previous Plan) (ApplyOutcome, error)
	// State returns a redacted summary of the live
	// queueing state. The Controller does not rely
	// on it for correctness; the Engine shows it to
	// the operator for verification.
	State(ctx context.Context) (RunnerState, error)
}

// RunnerState is a small struct the Runner reports.
// The Controller treats it as advisory.
type RunnerState struct {
	Link       string `json:"link"`
	Algorithm  string `json:"algorithm,omitempty"`
	RateKbps   int    `json:"rateKbps,omitempty"`
	HasShapers int    `json:"hasShapers"`
	Error      string `json:"error,omitempty"`
}

// Controller is the bounded Agent-side coordinator.
// The Controller never holds secrets, never mutates
// host state directly and never opens a socket. It
// delegates to a Runner and records the previous plan.
type Controller struct {
	mu       sync.Mutex
	runner   Runner
	current  Plan
	previous Plan
	last     ApplyOutcome
}

// NewController returns a Controller wired to a Runner.
// A nil runner is rejected; the Agent must decide
// between "unavailable" and "test fake" explicitly.
func NewController(r Runner) (*Controller, error) {
	if r == nil {
		return nil, errors.New("qos: controller requires a runner")
	}
	c := &Controller{runner: r}
	if durable, ok := r.(interface{ Plans() (Plan, Plan) }); ok {
		c.current, c.previous = durable.Plans()
	}
	return c, nil
}

func (c *Controller) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if closer, ok := c.runner.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// Current returns the most recently applied Plan. The
// returned Plan is a copy; mutating it does not
// affect the Controller.
func (c *Controller) Current() Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

// Previous returns the Plan that was active before the
// current one. The Controller uses it for rollback.
func (c *Controller) Previous() Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.previous
}

// Last returns the most recent ApplyOutcome. The
// Controller stores the outcome regardless of success
// or failure so the operator can correlate logs.
func (c *Controller) Last() ApplyOutcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// Apply runs a Plan through the Runner. The previous
// Plan is rotated into Previous before the new one
// becomes Current. A failed Apply does not rotate the
// previous plan; the caller can retry.
//
// Guarded execution: when Runner.Apply fails the
// Controller attempts Runner.Rollback so a partially
// applied plan does not leave the live tc surface in an
// indeterminate state. The rollback outcome is folded
// into the returned ApplyOutcome so the caller sees a
// single, structured record of what happened. A
// successful rollback leaves Current and Previous
// unchanged so the next Apply starts from the same
// state the failed attempt tried to leave.
func (c *Controller) Apply(ctx context.Context, p Plan) (ApplyOutcome, error) {
	if err := p.Valid(); err != nil {
		return ApplyOutcome{}, err
	}
	if c == nil {
		return ApplyOutcome{}, errors.New("qos: controller is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	runner := c.runner
	prev := c.current
	out, err := runner.Apply(ctx, p)
	if err != nil {
		// Guarded execution: ask the Runner to restore
		// the previous plan so a partially applied
		// plan does not leave live tc in an
		// indeterminate state. The rollback runs on a
		// bounded context so a runaway Runner cannot
		// stall the recovery path.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		rbOut, rbErr := runner.Rollback(rctx, p, prev)
		cancel()
		if rbErr != nil {
			out.Warnings = append(out.Warnings,
				"apply failed and rollback failed: "+rbErr.Error(),
				"live tc surface may be indeterminate; operator intervention required",
			)
		} else {
			out.Warnings = append(out.Warnings, "apply reverted via guarded rollback")
		}
		if rbOut.Applied > 0 || rbOut.Failed > 0 {
			out.Warnings = append(out.Warnings,
				"rollback summary: applied="+strconv.Itoa(rbOut.Applied)+
					" failed="+strconv.Itoa(rbOut.Failed),
			)
		}
		c.last = out
		return out, errors.Join(err, rbErr)
	}
	c.previous = prev
	c.current = p
	c.last = out
	return out, nil
}

// Rollback asks the Runner to restore the previous
// plan. The current plan becomes the new "previous"
// so a subsequent Rollback restores the one before
// it. Rollback refuses to run when there is no
// current plan; the Runner must never be called with
// an empty present state.
func (c *Controller) Rollback(ctx context.Context) (ApplyOutcome, error) {
	if c == nil {
		return ApplyOutcome{}, errors.New("qos: controller is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	runner := c.runner
	cur := c.current
	prev := c.previous
	if len(cur.Steps) == 0 {
		return ApplyOutcome{}, errors.New("qos: no current plan to roll back")
	}
	out, err := runner.Rollback(ctx, cur, prev)
	if err == nil {
		c.previous = cur
		c.current = prev
	}
	c.last = out
	return out, err
}

// PreviewState calls the Runner's State method. The
// Controller does not cache the result; the API
// surfaces the latest state on demand.
func (c *Controller) PreviewState(ctx context.Context) (RunnerState, error) {
	if c == nil {
		return RunnerState{}, errors.New("qos: controller is nil")
	}
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	return runner.State(ctx)
}

// DiffPlan returns a human-readable summary of what
// would change if next were applied over current.
// The summary is what the GUI/CLI shows as a
// preview. DiffPlan is pure: it does not perform any
// mutation and does not call the Runner.
func DiffPlan(current, next Plan) (DiffResult, error) {
	if err := current.Valid(); err != nil {
		return DiffResult{}, fmt.Errorf("current: %w", err)
	}
	if err := next.Valid(); err != nil {
		return DiffResult{}, fmt.Errorf("next: %w", err)
	}
	curIdx := indexSteps(current.Steps)
	nextIdx := indexSteps(next.Steps)
	added := []Step{}
	removed := []Step{}
	changed := []StepDiff{}
	for k, ns := range nextIdx {
		cs, ok := curIdx[k]
		if !ok {
			added = append(added, ns)
			continue
		}
		if !stepEqual(cs, ns) {
			changed = append(changed, StepDiff{Before: cs, After: ns})
		}
	}
	for k, cs := range curIdx {
		if _, ok := nextIdx[k]; !ok {
			removed = append(removed, cs)
		}
	}
	lessStep := func(a, b Step) bool {
		if a.Link != b.Link {
			return a.Link < b.Link
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Parent != b.Parent {
			return a.Parent < b.Parent
		}
		return a.Handle < b.Handle
	}
	sort.Slice(added, func(i, j int) bool { return lessStep(added[i], added[j]) })
	sort.Slice(removed, func(i, j int) bool { return lessStep(removed[i], removed[j]) })
	sort.Slice(changed, func(i, j int) bool { return lessStep(changed[i].After, changed[j].After) })
	return DiffResult{Added: added, Removed: removed, Changed: changed, Warnings: next.Warnings}, nil
}

// DiffResult is what the API returns to the GUI/CLI.
// The list sizes are bounded; the renderer rejects
// plans with more than MaxSteps.
type DiffResult struct {
	Added    []Step     `json:"added"`
	Removed  []Step     `json:"removed"`
	Changed  []StepDiff `json:"changed"`
	Warnings []string   `json:"warnings,omitempty"`
}

// StepDiff is the before/after for a single step that
// changed. The diff is a deep copy: the caller cannot
// mutate the controller's view.
type StepDiff struct {
	Before Step `json:"before"`
	After  Step `json:"after"`
}

func indexSteps(steps []Step) map[string]Step {
	out := make(map[string]Step, len(steps))
	for _, s := range steps {
		out[stepKey(s)] = s
	}
	return out
}

func stepKey(s Step) string {
	return string(s.Kind) + "|" + s.Link + "|" + s.Parent + "|" + s.Handle
}

func stepEqual(a, b Step) bool {
	if len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}
	return a.Kind == b.Kind && a.Link == b.Link && a.Parent == b.Parent && a.Handle == b.Handle
}
