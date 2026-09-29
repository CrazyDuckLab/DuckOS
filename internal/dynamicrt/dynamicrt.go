// dynamicrt/dynamicrt.go: V1 dynamic-routing
// data model. The package owns the typed
// contracts for OSPF, BGP, and BFD sessions,
// plus the Looking-Glass surface the future
// UI and CLI use to answer "why was this
// route accepted/rejected/preferred?".
//
// The runtime is split:
//   - this file: pure model + looking glass
//   - the OSPF/BGP daemon integration is
//     env-gated; the V1 contract is the model
//     and the filter-trace shape.
//
// A real OSPF/BGP daemon is large; the V1
// contract is the typed wire model and the
// visible looking-glass surface, not the
// userland daemon itself.
package dynamicrt

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Protocol is the routing protocol.
type Protocol string

const (
	ProtoOSPF Protocol = "ospf"
	ProtoBGP  Protocol = "bgp"
)

// AS is the autonomous-system number. The
// zero value is "unspecified".
type AS int64

// Session is a dynamic-routing session with a
// remote peer. The OSPF/BGP code paths share
// the same surface.
type Session struct {
	ID       string   `json:"id"`
	Protocol Protocol `json:"protocol"`
	// LocalAS is the local autonomous system.
	LocalAS AS `json:"localAS"`
	// RemoteAS is the remote AS. For OSPF
	// this is informational.
	RemoteAS AS `json:"remoteAS"`
	// VRF is the routing context. Empty
	// string means the default VRF.
	VRF string `json:"vrf,omitempty"`
	// Peer is the canonical peer address
	// (IPv4 or IPv6).
	Peer string `json:"peer"`
	// Interface is the local interface the
	// session binds to.
	Interface string `json:"interface,omitempty"`
	// State is the current state of the
	// session. The model surfaces it; the
	// runtime updates it.
	State SessionState `json:"state"`
	// Since is the time the session entered
	// its current state.
	Since int64 `json:"since"`
	// BFDEnabled toggles BFD-style fast
	// failure detection.
	BFDEnabled bool `json:"bfdEnabled"`
}

// SessionState is the high-level state of a
// session.
type SessionState string

const (
	StateIdle        SessionState = "idle"
	StateConnect     SessionState = "connect"
	StateActive      SessionState = "active"
	StateEstablished SessionState = "established"
	StateDown        SessionState = "down"
)

// Valid reports whether s is a known state.
func (s SessionState) Valid() bool {
	switch s {
	case StateIdle, StateConnect, StateActive, StateEstablished, StateDown:
		return true
	}
	return false
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether s is well-formed.
func (s Session) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("dynamicrt: session id is not canonical")
	}
	if s.Protocol != ProtoOSPF && s.Protocol != ProtoBGP {
		return errors.New("dynamicrt: protocol is unknown")
	}
	if s.LocalAS <= 0 {
		return errors.New("dynamicrt: localAS must be positive")
	}
	if s.Protocol == ProtoBGP && s.RemoteAS <= 0 {
		return errors.New("dynamicrt: BGP requires a remoteAS")
	}
	if s.Peer == "" {
		return errors.New("dynamicrt: peer is required")
	}
	if !s.State.Valid() {
		return errors.New("dynamicrt: state is unknown")
	}
	return nil
}

// Route is a route received from a dynamic
// routing session. The Looking Glass shows
// these alongside the static RIB.
type Route struct {
	Protocol    Protocol `json:"protocol"`
	SessionID   string   `json:"sessionId"`
	Network     string   `json:"network"`
	NextHop     string   `json:"nextHop"`
	Metric      int      `json:"metric"`
	LocalPref   int      `json:"localPref,omitempty"`
	ASPath      []AS     `json:"asPath,omitempty"`
	Communities []string `json:"communities,omitempty"`
	// Accepted is true when the route passed
	// the import filter. RejectedRoutes carry
	// the Reason instead.
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
	// BestPath is true when the route is the
	// current best path.
	BestPath bool `json:"bestPath"`
}

// Filter is one prefix-list / route-map entry
// used to filter routes.
type Filter struct {
	ID       string            `json:"id"`
	Sequence int               `json:"sequence"`
	Action   FilterAction      `json:"action"`
	Match    map[string]string `json:"match"`
	Comment  string            `json:"comment,omitempty"`
}

// FilterAction is what a filter does on a
// match.
type FilterAction string

const (
	FilterAccept   FilterAction = "accept"
	FilterReject   FilterAction = "reject"
	FilterModifyLP FilterAction = "modify-local-pref"
)

// Valid reports whether a is known.
func (a FilterAction) Valid() bool {
	switch a {
	case FilterAccept, FilterReject, FilterModifyLP:
		return true
	}
	return false
}

// Valid reports whether f is well-formed.
func (f Filter) Valid() error {
	if !idPattern.MatchString(f.ID) {
		return errors.New("dynamicrt: filter id is not canonical")
	}
	if f.Sequence < 0 {
		return errors.New("dynamicrt: sequence must be non-negative")
	}
	if !f.Action.Valid() {
		return errors.New("dynamicrt: action is unknown")
	}
	return nil
}

// TraceStep is one step in the filter trace.
// The looking glass renders the trace
// chronologically.
type TraceStep struct {
	FilterID string `json:"filterId"`
	Decision string `json:"decision"` // "accept" | "reject" | "modify"
	Reason   string `json:"reason,omitempty"`
}

// FilterTrace is the looking-glass output for
// "why was this route accepted/rejected/
// modified/preferred".
type FilterTrace struct {
	Route  Route       `json:"route"`
	Steps  []TraceStep `json:"steps"`
	Result string      `json:"result"`
	Reason string      `json:"reason,omitempty"`
}

// LookingGlass is the V1 dynamic-routing
// view. The Looking Glass is a read-only
// facade the future UI and CLI consume.
type LookingGlass struct {
	mu       sync.Mutex
	sessions map[string]Session
	routes   map[string][]Route // keyed by session id
	filters  map[string]Filter
}

// NewLookingGlass returns an empty looking
// glass.
func NewLookingGlass() *LookingGlass {
	return &LookingGlass{sessions: map[string]Session{}, routes: map[string][]Route{}, filters: map[string]Filter{}}
}

// SetSession replaces or inserts a session.
func (l *LookingGlass) SetSession(s Session) error {
	if err := s.Valid(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sessions[s.ID] = s
	return nil
}

// SetRoute replaces a session's routes.
func (l *LookingGlass) SetRoute(sessionID string, routes []Route) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.routes[sessionID] = routes
	return nil
}

// SetFilter replaces or inserts a filter.
func (l *LookingGlass) SetFilter(f Filter) error {
	if err := f.Valid(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.filters[f.ID] = f
	return nil
}

// Sessions returns a copy of the sessions
// sorted by id.
func (l *LookingGlass) Sessions() []Session {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Session, 0, len(l.sessions))
	for _, s := range l.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Session returns one session by id.
func (l *LookingGlass) Session(id string) (Session, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.sessions[id]
	return s, ok
}

// Routes returns the routes of a session,
// sorted by (network, best-path first).
func (l *LookingGlass) Routes(sessionID string) []Route {
	l.mu.Lock()
	defer l.mu.Unlock()
	routes := l.routes[sessionID]
	out := make([]Route, len(routes))
	copy(out, routes)
	sort.Slice(out, func(i, j int) bool {
		if out[i].BestPath != out[j].BestPath {
			return out[i].BestPath
		}
		return out[i].Network < out[j].Network
	})
	return out
}

// Filters returns the registered filters.
func (l *LookingGlass) Filters() []Filter {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.filtersSorted()
}

// filtersSorted returns the registered
// filters sorted by (Sequence, ID). The
// caller must hold l.mu.
func (l *LookingGlass) filtersSorted() []Filter {
	out := make([]Filter, 0, len(l.filters))
	for _, f := range l.filters {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sequence != out[j].Sequence {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Trace runs the looking-glass filter trace
// for one route. The trace is a deterministic
// walk through the registered filters: the
// first matching filter wins. The trace
// distinguishes accept / reject / modify
// decisions; a "no match" result carries the
// implicit "default accept" reason.
func (l *LookingGlass) Trace(sessionID string, network string) FilterTrace {
	l.mu.Lock()
	routes := l.routes[sessionID]
	l.mu.Unlock()
	var route Route
	for _, r := range routes {
		if r.Network == network {
			route = r
			break
		}
	}
	trace := FilterTrace{Route: route, Steps: []TraceStep{}}
	if route.Network == "" {
		trace.Result = "not-found"
		trace.Reason = "no route matches the network"
		return trace
	}
	// Walk the filters in deterministic
	// sequence order. Filters() returns the
	// canonical sorted slice; we reuse the
	// same ordering so the trace output is
	// stable across runs and across
	// goroutines.
	for _, f := range l.filtersSorted() {
		if matchesFilter(f, route) {
			step := TraceStep{FilterID: f.ID}
			switch f.Action {
			case FilterAccept:
				step.Decision = "accept"
				trace.Steps = append(trace.Steps, step)
				trace.Result = "accepted"
				trace.Reason = "matched accept filter " + f.ID
				return trace
			case FilterReject:
				step.Decision = "reject"
				trace.Steps = append(trace.Steps, step)
				trace.Result = "rejected"
				trace.Reason = "matched reject filter " + f.ID
				return trace
			case FilterModifyLP:
				step.Decision = "modify"
				step.Reason = "local-pref modified"
				trace.Steps = append(trace.Steps, step)
			}
		}
	}
	if route.Accepted {
		trace.Result = "accepted"
		trace.Reason = "no matching filter; default-accept applies"
	} else {
		trace.Result = "rejected"
		trace.Reason = "no matching filter; default-reject applies"
	}
	return trace
}

func matchesFilter(f Filter, r Route) bool {
	for k, v := range f.Match {
		switch k {
		case "network":
			if r.Network != v {
				return false
			}
		case "protocol":
			if string(r.Protocol) != v {
				return false
			}
		}
	}
	return true
}
