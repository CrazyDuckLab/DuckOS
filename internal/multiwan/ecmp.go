// multiwan/ecmp.go: V1 multi-WAN route groups
// with weighted selection, ECMP, failover,
// and per-flow stickiness. The existing
// multiwan package owns the basic model; this
// file adds the V1 enterprise semantics.
package multiwan

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"regexp"
	"sort"
	"sync"
)

// SelectionPolicy is the WAN selection policy.
type SelectionPolicy string

const (
	SelectionFailover SelectionPolicy = "failover"
	SelectionWeighted SelectionPolicy = "weighted"
	SelectionECMP     SelectionPolicy = "ecmp"
	SelectionSticky   SelectionPolicy = "sticky"
)

// Valid reports whether s is a known
// selection.
func (s SelectionPolicy) Valid() bool {
	switch s {
	case SelectionFailover, SelectionWeighted, SelectionECMP, SelectionSticky:
		return true
	}
	return false
}

// GroupMember is a WAN member of a route group.
// (Renamed to avoid collision with the
// existing internal/multiwan.Member type.)
type GroupMember struct {
	WANID  string `json:"wanId"`
	Weight int    `json:"weight"`
	// Priority is the failover order; lower
	// numbers are preferred. Used only when
	// SelectionPolicy == SelectionFailover.
	Priority int `json:"priority"`
}

var wanIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether m is well-formed.
func (m GroupMember) Valid() error {
	if !wanIDPattern.MatchString(m.WANID) {
		return errors.New("multiwan: member WAN id is not canonical")
	}
	if m.Weight < 0 {
		return errors.New("multiwan: weight must be non-negative")
	}
	if m.Priority < 0 {
		return errors.New("multiwan: priority must be non-negative")
	}
	return nil
}

// RouteGroup is the V1 multi-WAN policy. A
// RouteGroup selects one or more WAN members
// for outbound traffic.
type RouteGroup struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Selection SelectionPolicy `json:"selection"`
	Members   []GroupMember   `json:"members"`
	// FailbackEnabled controls automatic
	// recovery to a higher-priority WAN when
	// it comes back up.
	FailbackEnabled bool `json:"failbackEnabled"`
	// RecoveryDelay is the minimum time a
	// higher-priority WAN must stay healthy
	// before the group fails back to it.
	RecoveryDelaySec int `json:"recoveryDelaySec,omitempty"`
	// StickyHashKeys is the set of flow
	// fields used to pin a flow to a WAN
	// (SelectionPolicy == SelectionSticky).
	// The keys are "src", "dst", "sport",
	// "dport", "proto".
	StickyHashKeys []string `json:"stickyHashKeys,omitempty"`
}

// Valid reports whether g is well-formed.
func (g RouteGroup) Valid() error {
	if !wanIDPattern.MatchString(g.ID) {
		return errors.New("multiwan: route group id is not canonical")
	}
	if g.Name == "" {
		return errors.New("multiwan: route group name is required")
	}
	if !g.Selection.Valid() {
		return errors.New("multiwan: selection is unknown")
	}
	if len(g.Members) == 0 {
		return errors.New("multiwan: route group has no members")
	}
	for _, m := range g.Members {
		if err := m.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// FlowKey is a stable key for sticky/ECMP
// hashing. The struct is JSON-serialisable
// for logging.
type FlowKey struct {
	Src   string `json:"src,omitempty"`
	Dst   string `json:"dst,omitempty"`
	Sport int    `json:"sport,omitempty"`
	Dport int    `json:"dport,omitempty"`
	Proto string `json:"proto,omitempty"`
}

// Select returns the WAN ID the group picks
// for the given flow. The function is pure:
// it does not consult runtime health, only
// the static policy and the flow. The runtime
// overlays health-aware failover separately.
func (g RouteGroup) Select(flow FlowKey) string {
	if len(g.Members) == 0 {
		return ""
	}
	switch g.Selection {
	case SelectionFailover:
		return g.byPriority()
	case SelectionWeighted, SelectionECMP, SelectionSticky:
		return g.byHash(flow)
	}
	return g.Members[0].WANID
}

func (g RouteGroup) byPriority() string {
	best := g.Members[0]
	for _, m := range g.Members[1:] {
		if m.Priority < best.Priority {
			best = m
		}
	}
	return best.WANID
}

func (g RouteGroup) byHash(flow FlowKey) string {
	keys := g.StickyHashKeys
	if len(keys) == 0 {
		keys = []string{"src", "dst", "sport", "dport", "proto"}
	}
	h := crc32.NewIEEE()
	for _, k := range keys {
		switch k {
		case "src":
			h.Write([]byte(flow.Src))
		case "dst":
			h.Write([]byte(flow.Dst))
		case "sport":
			binary.Write(h, binary.BigEndian, uint32(flow.Sport))
		case "dport":
			binary.Write(h, binary.BigEndian, uint32(flow.Dport))
		case "proto":
			h.Write([]byte(flow.Proto))
		}
	}
	sum := h.Sum32()
	total := 0
	for _, m := range g.Members {
		total += m.Weight
	}
	if total == 0 {
		return g.Members[0].WANID
	}
	pick := int(sum % uint32(total))
	for _, m := range g.Members {
		if pick < m.Weight {
			return m.WANID
		}
		pick -= m.Weight
	}
	return g.Members[len(g.Members)-1].WANID
}

// Manager is the V1 route group store. The
// Manager is goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	groups map[string]RouteGroup
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{groups: map[string]RouteGroup{}}
}

// Set replaces or inserts a route group.
func (m *Manager) Set(g RouteGroup) error {
	if err := g.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[g.ID] = g
	return nil
}

// Get returns a route group by id.
func (m *Manager) Get(id string) (RouteGroup, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	return g, ok
}

// List returns the route groups sorted by id.
func (m *Manager) List() []RouteGroup {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RouteGroup, 0, len(m.groups))
	for _, g := range m.groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a route group.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, id)
}

// ConntrackImpact is the V1 contract for
// reporting which established connections a
// change will affect. The change impact is
// computed at Safe Apply time and surfaced
// to the operator before Commit.
type ConntrackImpact string

const (
	ConntrackImpactNone        ConntrackImpact = "none"
	ConntrackImpactNewOnly     ConntrackImpact = "new-only"
	ConntrackImpactEstablished ConntrackImpact = "established"
)
