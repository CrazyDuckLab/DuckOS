package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/wifi"
)

// WifiHost is the Agent-side runtime that owns
// the Wi-Fi read-only surface. The host is
// goroutine-safe; the Agent calls Wifi from
// the loopback handler under a concurrency cap.
// The host does not own a Runner: the Wi-Fi
// foundation is read-only and surfaces the
// capability gate so the operator never sees
// a checkbox the host cannot honour.
type WifiHost struct {
	mu    sync.Mutex
	state *wifiState
}

type wifiState struct {
	topology   wifi.Topology
	health     map[string]wifi.Health
	stations   map[string]int
	capability wifi.Capability
	probedAt   int64
}

// NewWifiHost returns a host backed by an
// empty topology and an "unsupported"
// capability. The constructor is the single
// entry point used by the duck-agent startup.
func NewWifiHost() *WifiHost {
	return &WifiHost{state: &wifiState{topology: wifi.Topology{SchemaVersion: wifi.SchemaVersion}, health: map[string]wifi.Health{}, stations: map[string]int{}, capability: wifi.Capability{Supported: false, Reason: "no wireless capability reported"}}}
}

// NewWifiHostWithState returns a host preloaded
// with the supplied topology and capability.
func NewWifiHostWithState(t wifi.Topology, cap wifi.Capability) (*WifiHost, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	if err := cap.Valid(); err != nil {
		return nil, err
	}
	h := &WifiHost{state: &wifiState{topology: t, health: map[string]wifi.Health{}, stations: map[string]int{}, capability: cap}}
	for _, a := range t.APs {
		if a.Enabled && cap.Supported {
			h.state.health[a.ID] = wifi.HealthEnabled
		} else if !a.Enabled {
			h.state.health[a.ID] = wifi.HealthDisabled
		} else {
			h.state.health[a.ID] = wifi.HealthUnsupported
		}
	}
	return h, nil
}

// Wifi returns the read-only status. The status
// is the topology, the per-AP health, the
// per-AP station count, the host capability
// and the last probe timestamp.
func (h *WifiHost) Wifi(_ context.Context) (wifi.Status, error) {
	if h == nil {
		return wifi.Status{}, errors.New("agent: wifi host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	health := map[string]wifi.Health{}
	for k, v := range h.state.health {
		health[k] = v
	}
	stations := map[string]int{}
	for k, v := range h.state.stations {
		stations[k] = v
	}
	return wifi.Status{Topology: h.state.topology, Health: health, Stations: stations, Capability: h.state.capability, ProbedAt: h.state.probedAt, HasAPs: len(h.state.topology.APs)}, nil
}

// UpdateTopology replaces the agent's view. An
// invalid topology is rejected.
func (h *WifiHost) UpdateTopology(t wifi.Topology) error {
	if err := t.Valid(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.topology = t
	health := map[string]wifi.Health{}
	for _, a := range t.APs {
		if v, ok := h.state.health[a.ID]; ok {
			health[a.ID] = v
		} else if a.Enabled && h.state.capability.Supported {
			health[a.ID] = wifi.HealthEnabled
		} else if !a.Enabled {
			health[a.ID] = wifi.HealthDisabled
		} else {
			health[a.ID] = wifi.HealthUnsupported
		}
	}
	h.state.health = health
	return nil
}

// UpdateCapability replaces the host capability
// report. The capability is the single source
// of truth; the operator never sees a checkbox
// the host cannot honour.
func (h *WifiHost) UpdateCapability(c wifi.Capability) error {
	if err := c.Valid(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.capability = c
	for _, a := range h.state.topology.APs {
		if a.Enabled && c.Supported {
			h.state.health[a.ID] = wifi.HealthEnabled
		} else if !a.Enabled {
			h.state.health[a.ID] = wifi.HealthDisabled
		} else {
			h.state.health[a.ID] = wifi.HealthUnsupported
		}
	}
	return nil
}
