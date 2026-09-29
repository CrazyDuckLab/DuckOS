// vrrp/vrrp.go: V1 VRRP first-class model.
// VRRP enables multiple routers to share a
// virtual IP; one is the active router, the
// others are standby. The model owns the
// per-group configuration, the priority
// rules, and the preemption policy.
//
// The runtime is platform-agnostic; the
// Linux implementation lives in
// internal/network. The V1 contract is the
// model and the bounded state machine.
package vrrp

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// State is the high-level state of a VRRP
// group on a single router.
type State string

const (
	StateInitialize State = "initialize"
	StateBackup     State = "backup"
	StateMaster     State = "master"
	StateDown       State = "down"
)

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	switch s {
	case StateInitialize, StateBackup, StateMaster, StateDown:
		return true
	}
	return false
}

// Group is one VRRP group. A router may run
// multiple groups per link.
type Group struct {
	ID          string `json:"id"`
	Interface   string `json:"interface"`
	VirtualIP   string `json:"virtualIp"`
	VRID        int    `json:"vrid"`
	Priority    int    `json:"priority"`
	AdvertiseMS int    `json:"advertiseMs"`
	Preempt     bool   `json:"preempt"`
	// AcceptPriority reduces the local
	// priority when the upstream link is
	// down. The value is subtracted from the
	// configured priority; the result is
	// clamped to [1, 254].
	AcceptPriority int `json:"acceptPriority"`
	// State is the current observed state.
	// The Agent updates it through the
	// runtime.
	State State `json:"state"`
	// LastTransition is the unix-second of
	// the most recent state change.
	LastTransition int64 `json:"lastTransition"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether g is well-formed.
func (g Group) Valid() error {
	if !idPattern.MatchString(g.ID) {
		return errors.New("vrrp: id is not canonical")
	}
	if !linkPattern.MatchString(g.Interface) {
		return errors.New("vrrp: interface is not canonical")
	}
	if g.VRID < 1 || g.VRID > 255 {
		return errors.New("vrrp: vrid must be 1..255")
	}
	if g.Priority < 1 || g.Priority > 254 {
		return errors.New("vrrp: priority must be 1..254")
	}
	if g.AdvertiseMS <= 0 {
		return errors.New("vrrp: advertise interval must be positive")
	}
	if g.AcceptPriority < 0 || g.AcceptPriority > 253 {
		return errors.New("vrrp: acceptPriority out of range")
	}
	if !g.State.Valid() {
		return errors.New("vrrp: state is unknown")
	}
	return nil
}

// Election is the V1 election algorithm. The
// router with the highest priority wins. A
// priority of 255 pins the router as the
// active. Preempt controls whether a backup
// takes over from a lower-priority active.
type Election struct {
	// LocalPriority is the current effective
	// priority (after AcceptPriority offset).
	LocalPriority int
	// RemotePriorities is the set of priorities
	// observed in the most recent advertisement
	// from each peer.
	RemotePriorities []int
	// Preempt mirrors the group setting.
	Preempt bool
	// ActivePriority is the priority of the
	// current active. A backup takes over only
	// if Preempt is true and its own priority
	// is strictly higher.
	ActivePriority int
}

// Decide returns the new local state. The
// function is pure: it does not consult the
// network, only the election snapshot.
func (e Election) Decide() State {
	if e.LocalPriority >= 255 {
		return StateMaster
	}
	bestRemote := 0
	for _, p := range e.RemotePriorities {
		if p > bestRemote {
			bestRemote = p
		}
	}
	if bestRemote == 0 {
		// No peer advertisement: become
		// active if the local priority is the
		// highest known.
		return StateMaster
	}
	if e.LocalPriority > bestRemote {
		return StateMaster
	}
	if e.LocalPriority == bestRemote && e.Preempt {
		return StateMaster
	}
	return StateBackup
}

// Manager is the V1 VRRP store. The Manager is
// goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	groups map[string]Group
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{groups: map[string]Group{}}
}

// Set replaces or inserts a group.
func (m *Manager) Set(g Group) error {
	if err := g.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[g.ID] = g
	return nil
}

// Get returns a group by id.
func (m *Manager) Get(id string) (Group, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	return g, ok
}

// List returns the groups sorted by id.
func (m *Manager) List() []Group {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Group, 0, len(m.groups))
	for _, g := range m.groups {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a group.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.groups, id)
}

// Transition returns a new Group with the
// given state and last-transition timestamp.
// The function does not mutate the input.
func (g Group) Transition(to State, now int64) (Group, error) {
	if !to.Valid() {
		return g, errors.New("vrrp: target state is unknown")
	}
	g.State = to
	g.LastTransition = now
	return g, nil
}
