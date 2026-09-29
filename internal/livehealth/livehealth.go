// livehealth/livehealth.go: G05 live health observations.
//
// A live observation is one timestamped probe result
// against one component (e.g. "nft input drop rate last
// 5 seconds" or "frr bgp peer established count"). The
// Store keeps a bounded ring of recent observations so
// the UI can render a per-component freshness badge and
// the CLI can stream the latest measurement without
// triggering a fresh probe.
//
// All observations are pure data: no callbacks, no
// timers, no I/O. The Agent composition root feeds
// observations from existing subsystems (doctor,
// readiness, daemon probes). The Store never grows
// unbounded: per-component retention is bounded by
// MaxObservationsPerComponent.
package livehealth

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SchemaVersion is the on-disk schema version. Bumped
// only on incompatible envelope changes.
const SchemaVersion = 1

// MaxObservationsPerComponent caps the per-component
// retention. Older entries fall off the end.
const MaxObservationsPerComponent = 64

// Severity enumerates the canonical observation severity
// levels. New levels must be appended to preserve the
// wire-format contract.
type Severity string

const (
	SeverityOK       Severity = "ok"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Valid reports whether s is a known severity.
func (s Severity) Valid() bool {
	switch s {
	case SeverityOK, SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

// Observation is one timestamped probe result.
type Observation struct {
	Component string `json:"component"`
	// Metric is the canonical measurement name within
	// the component (e.g. "bgp-peers-up", "input-drops").
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	// Unit is the measurement unit ("count", "pps",
	// "pct", "ms", "bytes").
	Unit string `json:"unit,omitempty"`
	// Severity is the classification the producer
	// attached to the observation.
	Severity Severity `json:"severity"`
	// ObservedAt is the wall-clock time (UTC) the probe
	// finished.
	ObservedAt time.Time `json:"observedAt"`
	// Note is an optional operator-facing explanation.
	Note string `json:"note,omitempty"`
	// Source is the subsystem that produced the
	// observation (e.g. "doctor", "netlink").
	Source string `json:"source,omitempty"`
}

// componentPattern mirrors the lifecycle id rules so
// observation components cannot smuggle shell
// metacharacters into the audit log.
var componentPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

// metricPattern is a less strict version that allows
// dotted metric names (e.g. "bgp.peers.up").
var metricPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Validate reports whether the observation is
// internally consistent.
func (o Observation) Validate() error {
	if !componentPattern.MatchString(o.Component) {
		return errors.New("livehealth: component is not canonical")
	}
	if !metricPattern.MatchString(o.Metric) {
		return errors.New("livehealth: metric is not canonical")
	}
	if !o.Severity.Valid() {
		return errors.New("livehealth: severity is not canonical")
	}
	if o.ObservedAt.IsZero() {
		return errors.New("livehealth: observedAt is zero")
	}
	if len(o.Unit) > 16 {
		return errors.New("livehealth: unit too long")
	}
	if len(o.Source) > 32 {
		return errors.New("livehealth: source too long")
	}
	if len(o.Note) > 256 {
		return errors.New("livehealth: note too long")
	}
	return nil
}

// Key returns the canonical (component, metric) pair.
func (o Observation) Key() string {
	return o.Component + ":" + o.Metric
}

// Store is the goroutine-safe bounded observation store.
type Store struct {
	mu           sync.RWMutex
	observations map[string][]Observation
	maxPer       int
	staleAfter   time.Duration
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		observations: map[string][]Observation{},
		maxPer:       MaxObservationsPerComponent,
		staleAfter:   90 * time.Second,
	}
}

// SetMax overrides the per-key retention ceiling.
func (s *Store) SetMax(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 {
		n = MaxObservationsPerComponent
	}
	s.maxPer = n
}

// SetStale overrides the staleness threshold used by
// Latest.
func (s *Store) SetStale(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d <= 0 {
		d = 90 * time.Second
	}
	s.staleAfter = d
}

// Append inserts an observation. The per-key ring is
// trimmed to maxPer. An invalid observation returns the
// validation error without mutating state.
func (s *Store) Append(o Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	key := o.Key()
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := append(s.observations[key], o)
	if s.maxPer > 0 && len(ring) > s.maxPer {
		ring = ring[len(ring)-s.maxPer:]
	}
	s.observations[key] = ring
	return nil
}

// Latest returns the most recent observation for the
// supplied key. ok is false when no observation has
// been recorded.
func (s *Store) Latest(component, metric string) (Observation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := component + ":" + metric
	ring := s.observations[key]
	if len(ring) == 0 {
		return Observation{}, false
	}
	return ring[len(ring)-1], true
}

// History returns the bounded ring for the supplied
// key, newest first. The slice is a copy.
func (s *Store) History(component, metric string) []Observation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := component + ":" + metric
	ring := s.observations[key]
	out := make([]Observation, len(ring))
	for i, o := range ring {
		out[len(ring)-1-i] = o
	}
	return out
}

// AllKeys returns every observed (component, metric)
// pair sorted.
func (s *Store) AllKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.observations))
	for k := range s.observations {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// StaleKeys returns the keys whose latest observation
// is older than the configured threshold.
func (s *Store) StaleKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	var out []string
	for k, ring := range s.observations {
		if len(ring) == 0 {
			continue
		}
		last := ring[len(ring)-1]
		if now.Sub(last.ObservedAt) > s.staleAfter {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Counters summarises the store.
type Counters struct {
	Keys      int `json:"keys"`
	StaleKeys int `json:"staleKeys"`
	Total     int `json:"total"`
}

// Counters returns the current counts.
func (s *Store) Counters() Counters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, ring := range s.observations {
		total += len(ring)
	}
	now := time.Now().UTC()
	stale := 0
	for _, ring := range s.observations {
		if len(ring) == 0 {
			continue
		}
		last := ring[len(ring)-1]
		if now.Sub(last.ObservedAt) > s.staleAfter {
			stale++
		}
	}
	return Counters{Keys: len(s.observations), StaleKeys: stale, Total: total}
}

// Purge removes every observation for the supplied key.
func (s *Store) Purge(component, metric string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.observations, component+":"+metric)
}

// LatestForComponent returns the newest observation for
// any metric of the supplied component.
func (s *Store) LatestForComponent(component string) (Observation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best Observation
	found := false
	for k, ring := range s.observations {
		if !startsWith(k, component+":") || len(ring) == 0 {
			continue
		}
		last := ring[len(ring)-1]
		if !found || last.ObservedAt.After(best.ObservedAt) {
			best = last
			found = true
		}
	}
	return best, found
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
