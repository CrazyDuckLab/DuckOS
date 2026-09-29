package agent

import (
	"context"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/multiwan"
)

// MultiWANHost is the Agent-facing entry point that owns the
// multi-WAN topology, the probe results and the per-group
// selection. The Host is intentionally read-only: it never
// mutates routing/firewall state. The Engine + Agent
// pipeline is the only place that applies a Selection.
//
// The Host is goroutine-safe: the snapshot methods are
// protected by a mutex and the Evaluator inside the
// multiwan package already serialises its own state.
type MultiWANHost struct {
	mu        sync.Mutex
	evaluator *multiwan.Evaluator
	now       func() time.Time
}

// NewMultiWANHost wires a default Host from a topology. The
// caller must have validated the topology with
// multiwan.Build.
func NewMultiWANHost(t multiwan.Topology) (*MultiWANHost, error) {
	e, err := multiwan.NewEvaluator(t)
	if err != nil {
		return nil, err
	}
	return &MultiWANHost{evaluator: e, now: time.Now}, nil
}

// Topology returns the current topology.
func (h *MultiWANHost) Topology() multiwan.Topology {
	if h == nil {
		return multiwan.Topology{}
	}
	return h.evaluator.Topology()
}

// UpdateTopology atomically replaces the topology.
func (h *MultiWANHost) UpdateTopology(t multiwan.Topology) error {
	if h == nil {
		return nil
	}
	return h.evaluator.UpdateTopology(t)
}

// RecordProbe records a probe result and returns the
// resulting per-group Selection. The method is the canonical
// write path the Agent uses after a probe round.
func (h *MultiWANHost) RecordProbe(r multiwan.ProbeResult) map[string]multiwan.Selection {
	if h == nil {
		return nil
	}
	return h.evaluator.RecordProbe(r)
}

// LastSelections returns a copy of the most recent per-group
// selection. The function is the read path the API exposes.
func (h *MultiWANHost) LastSelections() map[string]multiwan.Selection {
	if h == nil {
		return nil
	}
	return h.evaluator.LastSelections()
}

// MemberHealth returns the per-member health summary.
func (h *MultiWANHost) MemberHealth() map[string]string {
	if h == nil {
		return nil
	}
	return h.evaluator.MemberHealth()
}

// MultiWAN is the canonical read API the Agent IPC handler
// calls. The Host returns the topology, the most recent
// per-group selection and the per-member health. The Host
// never blocks on I/O: a real probe runner would update the
// evaluator from a separate goroutine.
func (h *MultiWANHost) MultiWAN(ctx context.Context) (multiwan.Topology, map[string]multiwan.Selection, map[string]string, error) {
	if h == nil {
		return multiwan.Topology{}, nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return multiwan.Topology{}, nil, nil, err
	}
	t := h.Topology()
	sel := h.LastSelections()
	health := h.MemberHealth()
	return t, sel, health, nil
}
