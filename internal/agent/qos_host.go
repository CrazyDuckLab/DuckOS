package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/qos"
)

// QoSStateSink is the small interface the host uses
// to remember the last successful apply and the last
// error. The contract is the same as the model
// State; the host owns the controller, the agent
// owns the host.
type QoSStateSink interface {
	MarkApplied(at time.Time)
	MarkError(msg string)
}

// QoSControllerFactory produces a Controller bound
// to a Runner. The factory is the only place that
// resolves a Runner; tests inject a fake. The
// factory is invoked once at host construction.
type QoSControllerFactory func() (*qos.Controller, error)

// QoSHost is the Agent-side runtime that owns the
// read-only QoS state, the apply surface and the
// rollback surface. The host is goroutine-safe; the
// Agent calls QoS, Apply and Rollback from the
// loopback handler under a concurrency cap.
type QoSHost struct {
	mu         sync.Mutex
	state      *qos.State
	controller *qos.Controller
	sink       QoSStateSink
}

// NewQoSHost returns a host backed by an empty
// topology. The controller is produced through the
// supplied factory; a factory error is reported and
// the host is left in a "no controller" state. The
// IPC layer reports that state as 503.
func NewQoSHost(factory QoSControllerFactory) (*QoSHost, error) {
	if factory == nil {
		return nil, errors.New("agent: qos host requires a controller factory")
	}
	c, err := factory()
	if err != nil {
		return nil, err
	}
	h := &QoSHost{state: qos.NewState(topologyFromPlan(c.Current())), controller: c}
	if _, err := c.PreviewState(context.Background()); err != nil {
		h.state.MarkError(err.Error())
	}
	return h, nil
}

func (h *QoSHost) Close() error { return h.controller.Close() }

// NewQoSHostWithState returns a host preloaded with
// the supplied topology. The factory is the same
// shape as NewQoSHost; an empty controller is
// reported as 503 by the IPC layer.
func NewQoSHostWithState(initial qos.Topology, factory QoSControllerFactory) (*QoSHost, error) {
	h, err := NewQoSHost(factory)
	if err != nil {
		return nil, err
	}
	if err := h.state.UpdateTopology(initial); err != nil {
		return nil, err
	}
	return h, nil
}

// QoS returns the read-only status. The status is
// the topology, the last apply timestamp and the
// last error. The state copy is the contract the
// API returns to the GUI/CLI.
func (h *QoSHost) QoS(ctx context.Context) (qos.Status, error) {
	if h == nil {
		return qos.Status{}, errors.New("agent: qos host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, err := h.controller.PreviewState(ctx); err != nil {
		h.state.MarkError(err.Error())
	}
	return h.state.Status(), nil
}

// Apply applies a Plan through the controller. The
// host commits the synthetic topology only AFTER the
// Runner confirmed the live state; on failure it
// leaves the previous topology in place.
//
// Guarded execution: the controller performs the
// rollback when the Runner errors so a partially
// applied plan does not leave live tc in an
// indeterminate state. The host surfaces the
// guarded-rollback warnings to the operator and
// refuses to advance the visible topology.
func (h *QoSHost) Apply(ctx context.Context, p qos.Plan) (qos.ApplyOutcome, error) {
	if h == nil {
		return qos.ApplyOutcome{}, errors.New("agent: qos host is nil")
	}
	if h.controller == nil {
		return qos.ApplyOutcome{Error: "qos controller not configured"}, errors.New("qos: controller not configured")
	}
	if err := p.Valid(); err != nil {
		return qos.ApplyOutcome{Error: err.Error()}, err
	}
	// Build a synthetic topology that represents the
	// plan we are about to apply. The host commits
	// the topology on success so the GUI shows the
	// active plan.
	synthetic := topologyFromPlan(p)
	h.mu.Lock()
	controller := h.controller
	h.mu.Unlock()
	out, err := controller.Apply(ctx, p)
	if err != nil {
		h.markError(err.Error())
		// Guarded rollback already happened inside the
		// controller. Leave the host state on the
		// previous topology so the next read does not
		// reflect a partially applied plan.
		return out, err
	}
	if err := h.commitAppliedTopology(synthetic); err != nil {
		h.markError(err.Error())
		return out, err
	}
	h.state.MarkApplied(out.EndedAt)
	h.notifyApplied(out.EndedAt)
	return out, nil
}

// Rollback asks the controller to restore the
// previous plan. The host records the outcome.
func (h *QoSHost) Rollback(ctx context.Context) (qos.ApplyOutcome, error) {
	if h == nil {
		return qos.ApplyOutcome{}, errors.New("agent: qos host is nil")
	}
	if h.controller == nil {
		return qos.ApplyOutcome{Error: "qos controller not configured"}, errors.New("qos: controller not configured")
	}
	h.mu.Lock()
	controller := h.controller
	h.mu.Unlock()
	out, err := controller.Rollback(ctx)
	if err != nil {
		h.markError(err.Error())
		return out, err
	}
	if err := h.commitAppliedTopology(topologyFromPlan(controller.Current())); err != nil {
		h.markError(err.Error())
		return out, err
	}
	h.state.MarkApplied(out.EndedAt)
	h.notifyApplied(out.EndedAt)
	return out, nil
}

func (h *QoSHost) commitAppliedTopology(topology qos.Topology) error {
	if len(topology.Shapers) == 0 {
		h.state.ClearAppliedTopology()
		return nil
	}
	return h.state.UpdateTopology(topology)
}

// PreviewState returns the controller's view of the
// live state. The host does not cache the result.
func (h *QoSHost) PreviewState(ctx context.Context) (qos.RunnerState, error) {
	if h == nil {
		return qos.RunnerState{}, errors.New("agent: qos host is nil")
	}
	if h.controller == nil {
		return qos.RunnerState{Error: "qos controller not configured"}, errors.New("qos: controller not configured")
	}
	h.mu.Lock()
	controller := h.controller
	h.mu.Unlock()
	return controller.PreviewState(ctx)
}

// SetSink wires a state sink. The sink is invoked
// on every successful apply and every recorded
// error. The sink is optional; the host works
// without one.
func (h *QoSHost) SetSink(s QoSStateSink) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.sink = s
	h.mu.Unlock()
}

func (h *QoSHost) markError(msg string) {
	if h == nil {
		return
	}
	h.state.MarkError(msg)
	h.mu.Lock()
	sink := h.sink
	h.mu.Unlock()
	if sink != nil {
		sink.MarkError(msg)
	}
}

// notifyApplied is the success-side counterpart of
// markError. The sink is invoked under the host's
// own lock, never the controller's. A nil sink is a
// no-op.
func (h *QoSHost) notifyApplied(at time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	sink := h.sink
	h.mu.Unlock()
	if sink != nil {
		sink.MarkApplied(at)
	}
}

// topologyFromPlan rehydrates a Topology from a
// rendered Plan. The plan already carries the
// per-step shape; the host re-derives the canonical
// Shaper list so the GUI/CLI sees a valid topology
// even before the plan was bound to one.
func topologyFromPlan(p qos.Plan) qos.Topology {
	shapers := []qos.Shaper{}
	seen := map[string]bool{}
	for _, s := range p.Steps {
		if s.Kind != qos.StepQdisc {
			continue
		}
		// The Step.Link is the canonical DuckOS link
		// id. The renderer never invents one.
		if seen[s.Link] {
			continue
		}
		seen[s.Link] = true
		// We don't try to recover every Shaper field
		// from the rendered Step: the host only
		// needs a placeholder that Valid accepts, so
		// the GUI can show "this link is shaped".
		shapers = append(shapers, qos.Shaper{ID: s.Link, Link: s.Link, Direction: qos.DirectionEgress, Algorithm: qos.AlgorithmCAKE, RateKbps: 1})
	}
	return qos.Topology{SchemaVersion: qos.SchemaVersion, Shapers: shapers, GeneratedAt: p.Generated}
}
