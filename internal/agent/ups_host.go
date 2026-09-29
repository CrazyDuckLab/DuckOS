package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/ups"
)

// UPSProberFactory produces a Runner bound to a
// probe surface. The factory is the only place
// that resolves a Runner; tests inject a fake.
type UPSProberFactory func() *ups.Runner

// UPSHost is the Agent-side runtime that owns
// the read-only UPS state and the on-demand
// probe surface. The host is goroutine-safe;
// the Agent calls UPS and Probe from the
// loopback handler under a concurrency cap.
type UPSHost struct {
	mu     sync.Mutex
	state  *ups.State
	runner *ups.Runner
}

// NewUPSHost returns a host backed by an empty
// topology. The runner is produced through the
// supplied factory; a factory error is reported
// and the host is left in a "no runner" state.
func NewUPSHost(factory UPSProberFactory) (*UPSHost, error) {
	if factory == nil {
		return nil, errors.New("agent: ups host requires a prober factory")
	}
	r := factory()
	return &UPSHost{state: ups.NewState(ups.Topology{SchemaVersion: ups.SchemaVersion}), runner: r}, nil
}

// NewUPSHostWithState returns a host preloaded
// with the supplied topology. The factory is
// the same shape as NewUPSHost.
func NewUPSHostWithState(initial ups.Topology, factory UPSProberFactory) (*UPSHost, error) {
	h, err := NewUPSHost(factory)
	if err != nil {
		return nil, err
	}
	if err := h.state.UpdateTopology(initial); err != nil {
		return nil, err
	}
	return h, nil
}

// UPS returns the read-only status. The status
// is the topology, the per-device health, the
// per-device variables and the last probe
// timestamp. The state copy is the contract
// the API returns to the GUI/CLI.
func (h *UPSHost) UPS(_ context.Context) (ups.Status, error) {
	if h == nil {
		return ups.Status{}, errors.New("agent: ups host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state.Status(), nil
}

// Probe runs a single probe pass over every
// device in the topology. The runner is the
// only side that calls the live daemon; the
// host updates the state with the per-device
// result.
func (h *UPSHost) Probe(ctx context.Context) error {
	if h == nil {
		return errors.New("agent: ups host is nil")
	}
	if h.runner == nil {
		return errors.New("ups: runner is not configured")
	}
	h.mu.Lock()
	devices := h.state.Topology().Devices
	runner := h.runner
	h.mu.Unlock()
	results := runner.ProbeAll(ctx, devices)
	for _, r := range results {
		if r.Err != nil {
			h.state.MarkError(r.Err.Error())
			continue
		}
		h.state.RecordProbe(r.DeviceID, r.Health, r.Variables, r.At)
	}
	return nil
}
