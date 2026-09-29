// multicast/mcastrt.go: V1 multicast
// routing surface. Package owns typed
// contracts for PIM-SM, SSM, RP
// configuration, (*,G)/(S,G) state, RPF
// lookup, and the L7 application
// classification surface.
package multicast

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Protocol is the multicast routing
// protocol.
type Protocol string

const (
	ProtoPIMSM Protocol = "pim-sm"
	ProtoSSM   Protocol = "ssm"
	ProtoIGMP  Protocol = "igmp"
)

// Valid reports whether p is known.
func (p Protocol) Valid() bool { return p == ProtoPIMSM || p == ProtoSSM || p == ProtoIGMP }

// RP is a rendezvous point.
type RP struct {
	Address string `json:"address"`
	// Group is the optional group prefix.
	Group string `json:"group,omitempty"`
}

var addrPattern = regexp.MustCompile(`^[0-9a-fA-F:.]+$`)

// Valid reports whether r is well-formed.
func (r RP) Valid() error {
	if !addrPattern.MatchString(r.Address) {
		return errors.New("multicast: RP address is not canonical")
	}
	if r.Group != "" && !addrPattern.MatchString(r.Group) {
		return errors.New("multicast: RP group is not canonical")
	}
	return nil
}

// Entry is one (*,G) or (S,G) state.
type Entry struct {
	Group    string   `json:"group"`
	Source   string   `json:"source,omitempty"`
	Incoming string   `json:"incoming,omitempty"`
	Outgoing []string `json:"outgoing,omitempty"`
	// RPF is the RPF neighbour.
	RPF string `json:"rpf,omitempty"`
}

// State is the multicast state.
type State struct {
	Entries []Entry `json:"entries"`
}

// Manager is the multicast store.
type Manager struct {
	mu        sync.Mutex
	protocol  Protocol
	rps       []RP
	igmpsnoop bool
	state     State
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{} }

// SetProtocol records the active
// protocol.
func (m *Manager) SetProtocol(p Protocol) {
	if !p.Valid() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.protocol = p
}

// AddRP inserts an RP.
func (m *Manager) AddRP(r RP) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rps = append(m.rps, r)
	return nil
}

// SetIGMPSnoop toggles IGMP snooping.
func (m *Manager) SetIGMPSnoop(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.igmpsnoop = v
}

// SetState replaces the runtime state.
func (m *Manager) SetState(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
}

// RPF performs a simple RPF lookup: the
// incoming interface is the supplied
// value when it is non-empty, else the
// RPF neighbor.
func (m *Manager) RPF(group, source string) (string, error) {
	if group == "" {
		return "", errors.New("multicast: group is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.state.Entries {
		if e.Group == group && e.Source == source {
			if e.RPF != "" {
				return e.RPF, nil
			}
			return e.Incoming, nil
		}
	}
	return "", errors.New("multicast: RPF not found")
}

// Snapshot returns a copy of the current
// state.
func (m *Manager) Snapshot() (Protocol, []RP, bool, State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rps := append([]RP(nil), m.rps...)
	sort.Slice(rps, func(i, k int) bool { return rps[i].Address < rps[k].Address })
	return m.protocol, rps, m.igmpsnoop, m.state
}

// L7Rule is an application-classification
// rule.
type L7Rule struct {
	ID    string `json:"id"`
	Match string `json:"match"` // SNI / domain / port signature
	Class string `json:"class"`
	// Source is the rule source: dns /
	// sni / signature.
	Source      string   `json:"source"`
	Confidence  int      `json:"confidence"`
	Notes       string   `json:"notes,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether r is well-formed.
func (r L7Rule) Valid() error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("multicast: L7 id is not canonical")
	}
	if r.Match == "" {
		return errors.New("multicast: L7 match is required")
	}
	if r.Class == "" {
		return errors.New("multicast: L7 class is required")
	}
	if r.Confidence < 0 || r.Confidence > 100 {
		return errors.New("multicast: L7 confidence out of range")
	}
	return nil
}

// L7Manager is the L7 classification store.
type L7Manager struct {
	mu    sync.Mutex
	rules map[string]L7Rule
}

// NewL7Manager returns an empty manager.
func NewL7Manager() *L7Manager { return &L7Manager{rules: map[string]L7Rule{}} }

// Add inserts a rule.
func (m *L7Manager) Add(r L7Rule) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[r.ID] = r
	return nil
}

// List returns every rule sorted.
func (m *L7Manager) List() []L7Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]L7Rule, 0, len(m.rules))
	for _, r := range m.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}
