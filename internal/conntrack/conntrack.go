// conntrack/conntrack.go: V1 conntrack
// surface for diagnostics, change impact,
// and bounded kill. The package owns the
// typed contracts; the runtime is a thin
// mapping to the `conntrack` userland
// tool. Safe Apply consults ConntrackImpact
// to report "only new connections" vs
// "established sessions" before a commit.
package conntrack

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Action is the kind of conntrack action.
type Action string

const (
	ActionList  Action = "list"
	ActionKill  Action = "kill"
	ActionStats Action = "stats"
)

// Filter is a conntrack query filter.
type Filter struct {
	Src        string `json:"src,omitempty"`
	Dst        string `json:"dst,omitempty"`
	Sport      int    `json:"sport,omitempty"`
	Dport      int    `json:"dport,omitempty"`
	Proto      string `json:"proto,omitempty"`
	Mark       int    `json:"mark,omitempty"`
	WANID      string `json:"wanId,omitempty"`
	State      string `json:"state,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
}

var addrPattern = regexp.MustCompile(`^[a-zA-Z0-9.:/_-]{1,253}$`)
var protoPattern = regexp.MustCompile(`^(tcp|udp|icmp|icmp6|gre|esp|ah)$`)

// Valid reports whether f is well-formed.
func (f Filter) Valid() error {
	if f.Src != "" && !addrPattern.MatchString(f.Src) {
		return errors.New("conntrack: src is not canonical")
	}
	if f.Dst != "" && !addrPattern.MatchString(f.Dst) {
		return errors.New("conntrack: dst is not canonical")
	}
	if f.Proto != "" && !protoPattern.MatchString(f.Proto) {
		return errors.New("conntrack: proto is unknown")
	}
	if f.Sport < 0 || f.Sport > 65535 {
		return errors.New("conntrack: sport out of range")
	}
	if f.Dport < 0 || f.Dport > 65535 {
		return errors.New("conntrack: dport out of range")
	}
	if f.MaxResults < 0 || f.MaxResults > 10000 {
		return errors.New("conntrack: maxResults out of range")
	}
	return nil
}

// Entry is one conntrack row.
type Entry struct {
	Src      string `json:"src"`
	Dst      string `json:"dst"`
	Sport    int    `json:"sport"`
	Dport    int    `json:"dport"`
	Proto    string `json:"proto"`
	State    string `json:"state"`
	WANID    string `json:"wanId,omitempty"`
	TunnelID string `json:"tunnelId,omitempty"`
	Bytes    uint64 `json:"bytes"`
	Packets  uint64 `json:"packets"`
	AgeSec   int    `json:"ageSec"`
}

// Valid reports whether e is well-formed.
func (e Entry) Valid() error {
	if !addrPattern.MatchString(e.Src) || !addrPattern.MatchString(e.Dst) {
		return errors.New("conntrack: src/dst must be canonical")
	}
	return nil
}

// Stats is the conntrack global counters.
type Stats struct {
	Entries  uint64 `json:"entries"`
	Max      uint64 `json:"max"`
	Searched uint64 `json:"searched,omitempty"`
	Found    uint64 `json:"found,omitempty"`
	Failed   uint64 `json:"failed,omitempty"`
}

// Manager is the V1 conntrack surface. The
// Manager is goroutine-safe.
type Manager struct {
	mu    sync.Mutex
	stats Stats
	kill  chan Filter
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{kill: make(chan Filter, 16)}
}

// SetStats replaces the global counters.
func (m *Manager) SetStats(s Stats) {
	m.mu.Lock()
	m.stats = s
	m.mu.Unlock()
}

// Stats returns a copy of the global counters.
func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// List runs the filter against a virtual
// conntrack table. The function is the
// model used by the runtime; the test
// version uses an in-memory slice.
func (m *Manager) List(table []Entry, f Filter) []Entry {
	out := make([]Entry, 0)
	for _, e := range table {
		if f.Src != "" && f.Src != e.Src {
			continue
		}
		if f.Dst != "" && f.Dst != e.Dst {
			continue
		}
		if f.Sport != 0 && f.Sport != e.Sport {
			continue
		}
		if f.Dport != 0 && f.Dport != e.Dport {
			continue
		}
		if f.Proto != "" && f.Proto != e.Proto {
			continue
		}
		if f.WANID != "" && f.WANID != e.WANID {
			continue
		}
		if f.State != "" && f.State != e.State {
			continue
		}
		out = append(out, e)
		if f.MaxResults > 0 && len(out) >= f.MaxResults {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// KillSelected stages a kill request. The
// runtime applies the kill against the
// kernel. A bounded kill is required by V1:
// a single conntrack kill must not exceed
// MaxResults entries; the package refuses
// anything larger.
func (m *Manager) KillSelected(f Filter) error {
	if err := f.Valid(); err != nil {
		return err
	}
	if f.MaxResults == 0 {
		f.MaxResults = 100
	}
	if f.MaxResults > 1000 {
		return errors.New("conntrack: kill refused past 1000 entries")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case m.kill <- f:
		return nil
	default:
		return errors.New("conntrack: kill queue is full")
	}
}

// ChangeImpact is the V1 contract for the
// Safe Apply conntrack impact report.
type ChangeImpact string

const (
	ImpactNone        ChangeImpact = "none"
	ImpactNewOnly     ChangeImpact = "new-only"
	ImpactEstablished ChangeImpact = "established"
	ImpactAll         ChangeImpact = "all"
)
