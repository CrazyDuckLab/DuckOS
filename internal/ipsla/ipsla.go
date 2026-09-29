// ipsla/ipsla.go: V1 IP SLA / active
// measurement + Track + SD-WAN surface.
// The package owns the typed contracts for
// probes (ICMP / TCP / HTTP / DNS / gateway /
// tunnel), the per-probe thresholds and
// hysteresis state machine, the Track
// objects that drive downstream consumers
// (static-route state, VRRP priority,
// Multi-WAN, SD-WAN, tunnel failover,
// service health), and the SD-WAN policy
// layer with brownout detection, path
// scoring, and per-policy health.
package ipsla

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// ProbeKind is the probe class.
type ProbeKind string

const (
	ProbeICMP   ProbeKind = "icmp"
	ProbeTCP    ProbeKind = "tcp"
	ProbeHTTP   ProbeKind = "http"
	ProbeHTTPS  ProbeKind = "https"
	ProbeDNS    ProbeKind = "dns"
	ProbeGW     ProbeKind = "gateway"
	ProbeTunnel ProbeKind = "tunnel"
)

// Valid reports whether k is known.
func (k ProbeKind) Valid() bool {
	switch k {
	case ProbeICMP, ProbeTCP, ProbeHTTP, ProbeHTTPS, ProbeDNS,
		ProbeGW, ProbeTunnel:
		return true
	}
	return false
}

// Probe is the typed probe configuration.
type Probe struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Kind     ProbeKind     `json:"kind"`
	Target   string        `json:"target"`
	Port     int           `json:"port,omitempty"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	// Threshold* are the failure /
	// recovery thresholds. A probe is
	// considered down after ThresholdDown
	// consecutive failures and up after
	// ThresholdUp consecutive successes.
	ThresholdDown int `json:"thresholdDown"`
	ThresholdUp   int `json:"thresholdUp"`
	// HoldDown is the minimum time the
	// probe must remain in a state before
	// it can flip again. The hold-down
	// prevents oscillation.
	HoldDown time.Duration `json:"holdDown"`
	// History is the maximum number of
	// samples retained.
	History int `json:"history"`
	// VRF is the routing context.
	VRF string `json:"vrf,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var targetPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,253}$`)

// Valid reports whether p is well-formed.
func (p Probe) Valid() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("ipsla: id is not canonical")
	}
	if !p.Kind.Valid() {
		return errors.New("ipsla: kind is unknown")
	}
	if !targetPattern.MatchString(p.Target) {
		return errors.New("ipsla: target is not canonical")
	}
	if p.Interval < time.Second || p.Interval > 10*time.Minute {
		return errors.New("ipsla: interval must be 1s..10m")
	}
	if p.Timeout < 100*time.Millisecond || p.Timeout > p.Interval {
		return errors.New("ipsla: timeout must be 0.1s..interval")
	}
	if p.ThresholdDown < 1 || p.ThresholdDown > 100 {
		return errors.New("ipsla: thresholdDown must be 1..100")
	}
	if p.ThresholdUp < 1 || p.ThresholdUp > 100 {
		return errors.New("ipsla: thresholdUp must be 1..100")
	}
	if p.HoldDown < 0 || p.HoldDown > time.Hour {
		return errors.New("ipsla: holdDown must be 0..1h")
	}
	if p.History < 0 || p.History > 10000 {
		return errors.New("ipsla: history must be 0..10000")
	}
	if p.Kind == ProbeTCP || p.Kind == ProbeHTTP || p.Kind == ProbeHTTPS {
		if p.Port < 1 || p.Port > 65535 {
			return errors.New("ipsla: port required for tcp/http probes")
		}
	}
	return nil
}

// State is the probe state.
type State string

const (
	StateUp      State = "up"
	StateDown    State = "down"
	StateUnknown State = "unknown"
)

// Valid reports whether s is known.
func (s State) Valid() bool { return s == StateUp || s == StateDown || s == StateUnknown }

// Sample is one probe result.
type Sample struct {
	At     time.Time `json:"at"`
	OK     bool      `json:"ok"`
	RTTMs  float64   `json:"rttMs"`
	Detail string    `json:"detail,omitempty"`
}

// ProbeState is the per-probe runtime
// state.
type ProbeState struct {
	ID            string    `json:"id"`
	State         State     `json:"state"`
	ConsecFails   int       `json:"consecFails"`
	ConsecSuccess int       `json:"consecSuccess"`
	LatencyMs     float64   `json:"latencyMs"`
	JitterMs      float64   `json:"jitterMs"`
	LossPct       float64   `json:"lossPct"`
	LastFlipped   time.Time `json:"lastFlipped"`
	HoldExpires   time.Time `json:"holdExpires"`
	History       []Sample  `json:"history,omitempty"`
}

// Manager is the V1 IP SLA store.
// Manager is goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	probes map[string]Probe
	states map[string]*ProbeState
	clock  func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{probes: map[string]Probe{}, states: map[string]*ProbeState{}, clock: clock}
}

// Set replaces the probe configuration.
func (m *Manager) Set(p Probe) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probes[p.ID] = p
	if _, ok := m.states[p.ID]; !ok {
		m.states[p.ID] = &ProbeState{ID: p.ID, State: StateUnknown}
	}
	return nil
}

// Get returns a copy of the probe.
func (m *Manager) Get(id string) (Probe, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.probes[id]
	return p, ok
}

// State returns a copy of the probe state.
func (m *Manager) State(id string) (ProbeState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[id]
	if !ok {
		return ProbeState{}, false
	}
	out := *s
	out.History = append([]Sample(nil), s.History...)
	return out, true
}

// States returns every state sorted by id.
func (m *Manager) States() []ProbeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ProbeState, 0, len(m.states))
	for _, s := range m.states {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// Ingest applies a probe sample to the
// state machine. The function updates the
// state, the consecutive counters, the
// latency / jitter / loss, and applies the
// configured hold-down.
func (m *Manager) Ingest(id string, s Sample) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	probe, ok := m.probes[id]
	if !ok {
		return StateUnknown, errors.New("ipsla: unknown probe id")
	}
	ps := m.states[id]
	if ps == nil {
		ps = &ProbeState{ID: id, State: StateUnknown}
		m.states[id] = ps
	}
	now := s.At
	if now.IsZero() {
		now = m.clock().UTC()
	}
	s.At = now
	// Append the sample to the history.
	ps.History = append(ps.History, s)
	if probe.History > 0 && len(ps.History) > probe.History {
		ps.History = ps.History[len(ps.History)-probe.History:]
	}
	// Recompute latency / jitter / loss over
	// the retained history.
	computeStats(ps)
	// Update consecutive counters.
	if s.OK {
		ps.ConsecFails = 0
		ps.ConsecSuccess++
	} else {
		ps.ConsecSuccess = 0
		ps.ConsecFails++
	}
	// Hold-down: skip state flip while in
	// hold.
	if !ps.HoldExpires.IsZero() && now.Before(ps.HoldExpires) {
		return ps.State, nil
	}
	// Apply thresholds.
	switch ps.State {
	case StateUnknown:
		if ps.ConsecSuccess >= probe.ThresholdUp {
			ps.State = StateUp
			ps.LastFlipped = now
			ps.HoldExpires = now.Add(probe.HoldDown)
		} else if ps.ConsecFails >= probe.ThresholdDown {
			ps.State = StateDown
			ps.LastFlipped = now
			ps.HoldExpires = now.Add(probe.HoldDown)
		}
	case StateUp:
		if ps.ConsecFails >= probe.ThresholdDown {
			ps.State = StateDown
			ps.LastFlipped = now
			ps.HoldExpires = now.Add(probe.HoldDown)
		}
	case StateDown:
		if ps.ConsecSuccess >= probe.ThresholdUp {
			ps.State = StateUp
			ps.LastFlipped = now
			ps.HoldExpires = now.Add(probe.HoldDown)
		}
	}
	return ps.State, nil
}

func computeStats(ps *ProbeState) {
	if len(ps.History) == 0 {
		return
	}
	var sum, sum2, last, prev float64
	losses := 0
	count := 0
	for i, s := range ps.History {
		sum += s.RTTMs
		sum2 += s.RTTMs * s.RTTMs
		if !s.OK {
			losses++
		}
		if i > 0 {
			prev = last
			last = s.RTTMs
			diff := last - prev
			if diff < 0 {
				diff = -diff
			}
			ps.JitterMs = (ps.JitterMs*float64(i-1) + diff) / float64(i)
		}
		count++
	}
	ps.LatencyMs = sum / float64(count)
	if count > 1 {
		variance := (sum2 - sum*sum/float64(count)) / float64(count-1)
		if variance > 0 {
			ps.JitterMs = sqrt(variance)
		}
	}
	ps.LossPct = float64(losses) * 100 / float64(count)
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Track is a Track object that may drive
// downstream consumers.
type Track struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ProbeID string `json:"probeId"`
	// NotState is the inverted state; the
	// track is "up" when the probe is in
	// NotState.
	NotState State `json:"notState"`
}

// Valid reports whether t is well-formed.
func (t Track) Valid() error {
	if !idPattern.MatchString(t.ID) {
		return errors.New("ipsla: track id is not canonical")
	}
	if t.ProbeID == "" {
		return errors.New("ipsla: track probeId is required")
	}
	if !t.NotState.Valid() {
		return errors.New("ipsla: track notState is invalid")
	}
	return nil
}
