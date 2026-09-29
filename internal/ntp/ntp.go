// ntp/ntp.go: V1 NTP/PTP time-source
// surface. The package owns the typed
// contracts for NTP sources, drift, last
// sync, server (where supported), and PTP.
// The runtime is a thin mapping to
// chrony/ntpd and linuxptp.
//
// The V1 contract: every time source is
// described by an explicit trust level
// that the existing internal/timeintegrity
// state machine consults. A source is
// promoted to "trusted" only when its
// stratum, drift, and reachability all
// match the policy.
package ntp

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SourceKind is the time-source class.
type SourceKind string

const (
	SourceNTP    SourceKind = "ntp"
	SourcePTP    SourceKind = "ptp"
	SourceGPS    SourceKind = "gps"
	SourceManual SourceKind = "manual"
)

// Valid reports whether k is known.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceNTP, SourcePTP, SourceGPS, SourceManual:
		return true
	}
	return false
}

// Source is a single time source.
type Source struct {
	Name    string     `json:"name"`
	Kind    SourceKind `json:"kind"`
	Address string     `json:"address,omitempty"` // host or device path
	Stratum int        `json:"stratum,omitempty"`
	IBurst  bool       `json:"iburst,omitempty"`
	Prefer  bool       `json:"prefer,omitempty"`
	// MinStratum is the minimum acceptable
	// stratum for this source.
	MinStratum int `json:"minStratum,omitempty"`
	// MaxDriftMs is the maximum acceptable
	// drift in milliseconds.
	MaxDriftMs int  `json:"maxDriftMs,omitempty"`
	Enabled    bool `json:"enabled"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether s is well-formed.
func (s Source) Valid() error {
	if !namePattern.MatchString(s.Name) {
		return errors.New("ntp: name is not canonical")
	}
	if !s.Kind.Valid() {
		return errors.New("ntp: kind is unknown")
	}
	if s.Kind == SourceNTP && s.Address == "" {
		return errors.New("ntp: NTP source requires an address")
	}
	if s.Stratum < 0 || s.Stratum > 16 {
		return errors.New("ntp: stratum out of range")
	}
	if s.MinStratum < 0 {
		return errors.New("ntp: minStratum must be non-negative")
	}
	if s.MaxDriftMs < 0 {
		return errors.New("ntp: maxDriftMs must be non-negative")
	}
	return nil
}

// Status is a runtime status snapshot for
// a source.
type Status struct {
	Name         string    `json:"name"`
	Reachable    bool      `json:"reachable"`
	Stratum      int       `json:"stratum"`
	OffsetMs     float64   `json:"offsetMs"`
	DriftMs      float64   `json:"driftMs"`
	LastSync     time.Time `json:"lastSync"`
	Trusted      bool      `json:"trusted"`
	FailureCount int       `json:"failureCount"`
}

// ServerConfig is the optional local NTP
// server configuration. The runtime enables
// the listener only when at least one
// upstream source is also enabled.
type ServerConfig struct {
	Listen  string   `json:"listen,omitempty"`
	Allow   []string `json:"allow,omitempty"`
	Stratum int      `json:"stratum,omitempty"`
}

// Valid reports whether c is well-formed.
func (c ServerConfig) Valid() error {
	if c.Stratum < 0 || c.Stratum > 16 {
		return errors.New("ntp: server stratum out of range")
	}
	return nil
}

// PTPConfig is the PTP configuration. PTP
// is exposed as a typed contract; the
// runtime uses linuxptp or equivalent.
type PTPConfig struct {
	Interface string `json:"interface"`
	Domain    int    `json:"domain"`
	Transport string `json:"transport"` // "udp4" | "udp6" | "l2"
	Mode      string `json:"mode"`      // "master" | "slave" | "boundary"
	Priority1 int    `json:"priority1,omitempty"`
	Priority2 int    `json:"priority2,omitempty"`
}

// Valid reports whether c is well-formed.
func (c PTPConfig) Valid() error {
	if c.Interface == "" {
		return errors.New("ntp: PTP interface is required")
	}
	switch c.Transport {
	case "udp4", "udp6", "l2":
	case "":
		// empty defaults to udp4
	default:
		return errors.New("ntp: PTP transport is unknown")
	}
	switch c.Mode {
	case "master", "slave", "boundary":
	case "":
		// empty defaults to slave
	default:
		return errors.New("ntp: PTP mode is unknown")
	}
	return nil
}

// Manager is the V1 time-source store.
// Manager is goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	sources map[string]Source
	status  map[string]Status
	server  ServerConfig
	ptp     []PTPConfig
	clock   func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{sources: map[string]Source{}, status: map[string]Status{}, clock: clock}
}

// SetSource replaces or inserts a source.
func (m *Manager) SetSource(s Source) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[s.Name] = s
	return nil
}

// RemoveSource drops a source.
func (m *Manager) RemoveSource(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sources[name]; !ok {
		return errors.New("ntp: unknown source")
	}
	delete(m.sources, name)
	delete(m.status, name)
	return nil
}

// Sources returns the configured sources
// sorted by name.
func (m *Manager) Sources() []Source {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Source, 0, len(m.sources))
	for _, s := range m.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// SetStatus replaces a source's runtime
// status.
func (m *Manager) SetStatus(st Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st.LastSync.IsZero() {
		st.LastSync = m.clock().UTC()
	}
	m.status[st.Name] = st
}

// Statuses returns the runtime status
// sorted by name.
func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.status))
	for _, s := range m.status {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// SetServer replaces the server config.
func (m *Manager) SetServer(c ServerConfig) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.server = c
	return nil
}

// Server returns a copy of the server config.
func (m *Manager) Server() ServerConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.server
}

// SetPTP replaces the PTP configuration
// list.
func (m *Manager) SetPTP(cs []PTPConfig) error {
	for _, c := range cs {
		if err := c.Valid(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ptp = append([]PTPConfig(nil), cs...)
	return nil
}

// PTP returns a copy of the PTP list.
func (m *Manager) PTP() []PTPConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PTPConfig(nil), m.ptp...)
}

// TrustDecision is the output of the trust
// policy: at least one source is reachable
// and inside the configured drift and
// stratum thresholds.
type TrustDecision struct {
	Trusted       bool     `json:"trusted"`
	BestSource    string   `json:"bestSource,omitempty"`
	FailingSource []string `json:"failingSources,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// EvaluateTrust inspects the runtime status
// list against the configured source policy
// and returns the trust decision. The
// decision drives the existing
// timeintegrity promotion.
func (m *Manager) EvaluateTrust() TrustDecision {
	m.mu.Lock()
	defer m.mu.Unlock()
	dec := TrustDecision{}
	for _, s := range m.sources {
		if !s.Enabled {
			continue
		}
		st, ok := m.status[s.Name]
		if !ok {
			dec.FailingSource = append(dec.FailingSource, s.Name)
			continue
		}
		if !st.Reachable {
			dec.FailingSource = append(dec.FailingSource, s.Name)
			continue
		}
		if s.MinStratum > 0 && st.Stratum > s.MinStratum {
			dec.FailingSource = append(dec.FailingSource, s.Name)
			continue
		}
		if s.MaxDriftMs > 0 && absF(st.DriftMs) > float64(s.MaxDriftMs) {
			dec.FailingSource = append(dec.FailingSource, s.Name)
			continue
		}
		// prefer source
		if dec.BestSource == "" || s.Prefer {
			dec.BestSource = s.Name
			dec.Trusted = true
		}
	}
	sort.Strings(dec.FailingSource)
	if !dec.Trusted {
		dec.Reason = "no source passed the trust policy"
	}
	return dec
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
