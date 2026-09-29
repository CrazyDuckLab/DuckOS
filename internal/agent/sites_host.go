package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/CrazyDuckLab/DuckOS/internal/sites"
)

// SitesHost is the Agent-side runtime that owns
// the sites read-only surface. The host is
// goroutine-safe; the Agent calls Sites from
// the loopback handler under a concurrency cap.
// The host does not own a Runner: the sites
// foundation is read-only.
type SitesHost struct {
	mu    sync.Mutex
	state *sitesState
}

// sitesState is the small runtime view the
// host keeps. The State carries the topology
// and a per-site health map. A state is the
// canonical DuckOS pattern.
type sitesState struct {
	topology sites.Topology
	health   map[string]sites.Health
}

// NewSitesHost returns a host backed by an empty
// topology. Use NewSitesHostWithState to
// preload a topology.
func NewSitesHost() *SitesHost {
	return &SitesHost{state: &sitesState{topology: sites.Topology{SchemaVersion: sites.SchemaVersion}, health: map[string]sites.Health{}}}
}

// NewSitesHostWithState returns a host preloaded
// with the supplied topology.
func NewSitesHostWithState(t sites.Topology) (*SitesHost, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	h := NewSitesHost()
	h.state.topology = t
	for _, s := range t.Sites {
		if s.Enabled {
			h.state.health[s.ID] = sites.HealthEnabled
		} else {
			h.state.health[s.ID] = sites.HealthDisabled
		}
	}
	return h, nil
}

// Sites returns the read-only status.
func (h *SitesHost) Sites(_ context.Context) (sites.Status, error) {
	if h == nil {
		return sites.Status{}, errors.New("agent: sites host is nil")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	st := sites.Status{Topology: h.state.topology, Health: map[string]sites.Health{}, ProbedAt: h.state.probedAt(), HasSites: len(h.state.topology.Sites)}
	for k, v := range h.state.health {
		st.Health[k] = v
	}
	return st, nil
}

// UpdateTopology replaces the agent's view.
// An invalid topology is rejected.
func (h *SitesHost) UpdateTopology(t sites.Topology) error {
	if err := t.Valid(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.topology = t
	health := map[string]sites.Health{}
	for _, s := range t.Sites {
		if v, ok := h.state.health[s.ID]; ok {
			health[s.ID] = v
		} else if s.Enabled {
			health[s.ID] = sites.HealthEnabled
		} else {
			health[s.ID] = sites.HealthDisabled
		}
	}
	h.state.health = health
	return nil
}

func (s *sitesState) probedAt() int64 {
	return 0
}
