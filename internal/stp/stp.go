// stp/stp.go: V1 spanning-tree / storm
// protection / MAC-flap detection model.
// The package owns the typed contracts; the
// runtime is a thin mapping to `bridge stp`
// and the kernel FDB.
//
// The detection surface is the part the future
// UI and the incident-experience engine
// consume.
package stp

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Mode is the spanning-tree protocol.
type Mode string

const (
	ModeSTP  Mode = "stp"
	ModeRSTP Mode = "rstp"
	ModeMSTP Mode = "mstp"
)

// Valid reports whether m is known.
func (m Mode) Valid() bool {
	switch m {
	case ModeSTP, ModeRSTP, ModeMSTP:
		return true
	}
	return false
}

// Bridge is the per-bridge STP configuration.
type Bridge struct {
	Bridge    string `json:"bridge"`
	Mode      Mode   `json:"mode"`
	Priority  int    `json:"priority"`
	HelloTime int    `json:"helloTimeSec"`
	MaxAge    int    `json:"maxAgeSec"`
	FwdDelay  int    `json:"forwardDelaySec"`
}

// Port is the per-port STP configuration.
type Port struct {
	Bridge    string `json:"bridge"`
	Link      string `json:"link"`
	Priority  int    `json:"priority"`
	PathCost  int    `json:"pathCost"`
	Edge      bool   `json:"edge"`
	BPDUGuard bool   `json:"bpduGuard"`
	RootGuard bool   `json:"rootGuard"`
	LoopGuard bool   `json:"loopGuard"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether b is well-formed.
func (b Bridge) Valid() error {
	if !idPattern.MatchString(b.Bridge) {
		return errors.New("stp: bridge is not canonical")
	}
	if !b.Mode.Valid() {
		return errors.New("stp: mode is unknown")
	}
	if b.Priority < 0 || b.Priority > 61440 {
		return errors.New("stp: priority out of range")
	}
	if b.HelloTime <= 0 {
		return errors.New("stp: hello time must be positive")
	}
	if b.MaxAge <= 0 {
		return errors.New("stp: max age must be positive")
	}
	if b.FwdDelay <= 0 {
		return errors.New("stp: forward delay must be positive")
	}
	return nil
}

// Valid reports whether p is well-formed.
func (p Port) Valid() error {
	if !linkPattern.MatchString(p.Link) {
		return errors.New("stp: link is not canonical")
	}
	if p.Priority < 0 || p.Priority > 240 {
		return errors.New("stp: port priority out of range")
	}
	if p.PathCost < 0 {
		return errors.New("stp: path cost must be non-negative")
	}
	return nil
}

// StormConfig is the per-port storm control
// configuration.
type StormConfig struct {
	BroadcastPPS  int `json:"broadcastPps,omitempty"`
	MulticastPPS  int `json:"multicastPps,omitempty"`
	UnknownUniPPS int `json:"unknownUnicastPps,omitempty"`
	// CPUProtection enables a per-port queue
	// discipline.
	CPUProtection bool `json:"cpuProtection"`
}

// Valid reports whether s is well-formed.
func (s StormConfig) Valid() error {
	if s.BroadcastPPS < 0 || s.MulticastPPS < 0 || s.UnknownUniPPS < 0 {
		return errors.New("stp: PPS must be non-negative")
	}
	return nil
}

// MACMove is a single MAC-flap observation.
type MACMove struct {
	MAC      string    `json:"mac"`
	FromPort string    `json:"fromPort"`
	ToPort   string    `json:"toPort"`
	VLAN     int       `json:"vlan"`
	At       time.Time `json:"at"`
}

// MacFlap is the aggregated flap detection
// record. The runtime updates Rate over a
// sliding window; the model exposes the
// current and peak values.
type MacFlap struct {
	MAC          string    `json:"mac"`
	WindowStart  time.Time `json:"windowStart"`
	Rate         int       `json:"rate"`
	PeakRate     int       `json:"peakRate"`
	LastMove     MACMove   `json:"lastMove"`
	Ports        []string  `json:"ports"`
	VLANs        []int     `json:"vlans"`
	DetectedLoop bool      `json:"detectedLoop"`
}

// Manager is the V1 STP / storm / flap store.
// Manager is goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	bridges map[string]Bridge
	ports   map[string]Port // key: bridge/link
	storm   map[string]StormConfig
	flaps   map[string]*MacFlap
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{bridges: map[string]Bridge{}, ports: map[string]Port{}, storm: map[string]StormConfig{}, flaps: map[string]*MacFlap{}}
}

// SetBridge replaces or inserts a bridge.
func (m *Manager) SetBridge(b Bridge) error {
	if err := b.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bridges[b.Bridge] = b
	return nil
}

// SetPort replaces or inserts a port.
func (m *Manager) SetPort(p Port) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ports[p.Bridge+"/"+p.Link] = p
	return nil
}

// SetStorm replaces or inserts storm config.
func (m *Manager) SetStorm(link string, s StormConfig) error {
	if err := s.Valid(); err != nil {
		return err
	}
	if !linkPattern.MatchString(link) {
		return errors.New("stp: link is not canonical")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storm[link] = s
	return nil
}

// RecordMove records a MAC move. The manager
// aggregates moves per MAC and detects the
// "same MAC moves rapidly between ports"
// signature.
func (m *Manager) RecordMove(move MACMove, window time.Duration, flapThreshold int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	flap, ok := m.flaps[move.MAC]
	if !ok {
		flap = &MacFlap{MAC: move.MAC, WindowStart: move.At}
		m.flaps[move.MAC] = flap
	}
	// Roll the window: if the previous move
	// is older than the window, reset.
	if move.At.Sub(flap.WindowStart) > window {
		flap.WindowStart = move.At
		flap.Rate = 0
		flap.Ports = nil
		flap.VLANs = nil
	}
	flap.Rate++
	if flap.Rate > flap.PeakRate {
		flap.PeakRate = flap.Rate
	}
	flap.LastMove = move
	flap.Ports = appendUnique(flap.Ports, move.FromPort)
	flap.Ports = appendUnique(flap.Ports, move.ToPort)
	flap.VLANs = appendUniqueInt(flap.VLANs, move.VLAN)
	flap.DetectedLoop = flap.Rate >= flapThreshold
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func appendUniqueInt(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// Flaps returns a copy of the detected flaps
// sorted by Rate (descending).
func (m *Manager) Flaps() []MacFlap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MacFlap, 0, len(m.flaps))
	for _, f := range m.flaps {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rate > out[j].Rate })
	return out
}
