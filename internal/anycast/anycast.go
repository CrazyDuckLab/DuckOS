// anycast/anycast.go: Anycast evidence contract.
//
// An Anycast VIP is one virtual IP advertised by a set of
// nodes. The operator needs to know three things for
// each VIP:
//
//  1. Desired: the configured VIP + hysteresis + scope.
//  2. Health: the latest probe result and its observedAt.
//  3. Advertisement: actual routing/BGP announcement
//     evidence (whether a peer accepted it, hold-down
//     status, withdrawal timestamp).
//
// This package is the bounded evidence store. Producers
// feed Desired / Health / Advertisement records; the
// Store trims by retention; consumers query. The store
// never grows unbounded.
//
// Per UX spec, the "Healthy boolean" must never be
// equated with a successful probe or announcement. The
// Evidence fields carry the granular source / timestamp
// the UI needs to render an honest "advertised" vs
// "configured-but-withdrawn" badge.
package anycast

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SchemaVersion is the contract schema version.
const SchemaVersion = 1

// MaxVIPs is the global retention ceiling.
const MaxVIPs = 32

// MaxEvidencePerVIP caps the per-VIP evidence ring.
const MaxEvidencePerVIP = 64

// MaxAge is the per-record age ceiling.
const MaxAge = 24 * time.Hour

// HysteresisState enumerates the desired advertisement
// state for a VIP.
type HysteresisState string

const (
	HysteresisAnnounce  HysteresisState = "announce"
	HysteresisWithdrawn HysteresisState = "withdrawn"
	HysteresisHoldDown  HysteresisState = "hold-down"
)

// Valid reports whether s is a known hysteresis state.
func (s HysteresisState) Valid() bool {
	switch s {
	case HysteresisAnnounce, HysteresisWithdrawn, HysteresisHoldDown:
		return true
	}
	return false
}

// ProbeOutcome enumerates the canonical probe outcomes.
type ProbeOutcome string

const (
	ProbeSucceeded ProbeOutcome = "succeeded"
	ProbeFailed    ProbeOutcome = "failed"
	ProbeUnknown   ProbeOutcome = "unknown"
)

// Valid reports whether o is a known outcome.
func (o ProbeOutcome) Valid() bool {
	switch o {
	case ProbeSucceeded, ProbeFailed, ProbeUnknown:
		return true
	}
	return false
}

// VIPIdentifier identifies one Anycast VIP. The IP
// string is canonical IPv4 or IPv6; the name is the
// operator-facing label.
type VIPIdentifier struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)
var ipPattern = regexp.MustCompile(`^([0-9]{1,3}\.){3}[0-9]{1,3}$|^[0-9a-fA-F:]+$`)

// Valid reports whether v is well-formed.
func (v VIPIdentifier) Valid() bool {
	if !namePattern.MatchString(v.Name) {
		return false
	}
	return ipPattern.MatchString(v.IP)
}

// DesiredRecord is the configured VIP intent.
type DesiredRecord struct {
	VIP               VIPIdentifier   `json:"vip"`
	State             HysteresisState `json:"state"`
	HysteresisSeconds int             `json:"hysteresisSeconds"`
	Scope             string          `json:"scope,omitempty"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	UpdatedBy         string          `json:"updatedBy,omitempty"`
}

// Validate reports whether d is internally consistent.
func (d DesiredRecord) Validate() error {
	if !d.VIP.Valid() {
		return errors.New("anycast: vip is not canonical")
	}
	if !d.State.Valid() {
		return errors.New("anycast: state is not canonical")
	}
	if d.HysteresisSeconds < 0 || d.HysteresisSeconds > 3600 {
		return errors.New("anycast: hysteresisSeconds out of [0,3600]")
	}
	if len(d.Scope) > 64 {
		return errors.New("anycast: scope too long")
	}
	if len(d.UpdatedBy) > 64 {
		return errors.New("anycast: updatedBy too long")
	}
	if d.UpdatedAt.IsZero() {
		return errors.New("anycast: updatedAt is zero")
	}
	return nil
}

// HealthRecord is the latest health probe evidence.
type HealthRecord struct {
	VIP        VIPIdentifier `json:"vip"`
	Outcome    ProbeOutcome  `json:"outcome"`
	ObservedAt time.Time     `json:"observedAt"`
	Probe      string        `json:"probe,omitempty"`
	Note       string        `json:"note,omitempty"`
}

// Validate reports whether h is internally consistent.
func (h HealthRecord) Validate() error {
	if !h.VIP.Valid() {
		return errors.New("anycast: vip is not canonical")
	}
	if !h.Outcome.Valid() {
		return errors.New("anycast: outcome is not canonical")
	}
	if h.ObservedAt.IsZero() {
		return errors.New("anycast: observedAt is zero")
	}
	if len(h.Probe) > 64 {
		return errors.New("anycast: probe too long")
	}
	if len(h.Note) > 256 {
		return errors.New("anycast: note too long")
	}
	return nil
}

// AdvertisementRecord is the actual routing/BGP evidence.
type AdvertisementRecord struct {
	VIP VIPIdentifier `json:"vip"`
	// Active is true when the local node is currently
	// announcing the VIP to its peers.
	Active     bool      `json:"active"`
	ObservedAt time.Time `json:"observedAt"`
	// Peers lists the canonical peer names that accepted
	// the announcement.
	Peers []string `json:"peers,omitempty"`
	// WithdrawnAt is the wall-clock time of the last
	// successful withdrawal. Zero when currently
	// advertising.
	WithdrawnAt time.Time `json:"withdrawnAt,omitempty"`
	Note        string    `json:"note,omitempty"`
}

// Validate reports whether a is internally consistent.
func (a AdvertisementRecord) Validate() error {
	if !a.VIP.Valid() {
		return errors.New("anycast: vip is not canonical")
	}
	if a.ObservedAt.IsZero() {
		return errors.New("anycast: observedAt is zero")
	}
	for _, p := range a.Peers {
		if len(p) > 64 {
			return errors.New("anycast: peer name too long")
		}
	}
	if len(a.Note) > 256 {
		return errors.New("anycast: note too long")
	}
	return nil
}

// Evidence is the per-VIP composite the API renders.
type Evidence struct {
	VIP           VIPIdentifier        `json:"vip"`
	Desired       *DesiredRecord       `json:"desired,omitempty"`
	LatestHealth  *HealthRecord        `json:"latestHealth,omitempty"`
	Advertisement *AdvertisementRecord `json:"advertisement,omitempty"`
}

// Store is the goroutine-safe bounded Anycast evidence store.
type Store struct {
	mu        sync.RWMutex
	desired   map[string]DesiredRecord
	health    map[string][]HealthRecord
	advertise map[string][]AdvertisementRecord
	maxVIPs   int
	maxPerVIP int
	maxAge    time.Duration
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		desired:   map[string]DesiredRecord{},
		health:    map[string][]HealthRecord{},
		advertise: map[string][]AdvertisementRecord{},
		maxVIPs:   MaxVIPs,
		maxPerVIP: MaxEvidencePerVIP,
		maxAge:    MaxAge,
	}
}

// SetLimits overrides the retention caps.
func (s *Store) SetLimits(maxVIPs, maxPerVIP int, maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if maxVIPs > 0 {
		s.maxVIPs = maxVIPs
	}
	if maxPerVIP > 0 {
		s.maxPerVIP = maxPerVIP
	}
	if maxAge > 0 {
		s.maxAge = maxAge
	}
}

// SetDesired upserts the desired record. The store
// enforces the global VIP cap by trimming the oldest.
func (s *Store) SetDesired(d DesiredRecord) error {
	if err := d.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = time.Now().UTC()
	}
	s.desired[d.VIP.Name] = d
	if s.maxVIPs > 0 && len(s.desired) > s.maxVIPs {
		s.trimDesiredLocked()
	}
	return nil
}

// AppendHealth records a new health observation. The
// per-VIP ring is trimmed to maxPerVIP.
func (s *Store) AppendHealth(h HealthRecord) error {
	if err := h.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := append(s.health[h.VIP.Name], h)
	if s.maxPerVIP > 0 && len(ring) > s.maxPerVIP {
		ring = ring[len(ring)-s.maxPerVIP:]
	}
	s.health[h.VIP.Name] = ring
	return nil
}

// AppendAdvertisement records a new advertisement event.
func (s *Store) AppendAdvertisement(a AdvertisementRecord) error {
	if err := a.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ring := append(s.advertise[a.VIP.Name], a)
	if s.maxPerVIP > 0 && len(ring) > s.maxPerVIP {
		ring = ring[len(ring)-s.maxPerVIP:]
	}
	s.advertise[a.VIP.Name] = ring
	return nil
}

// Evidence returns the composite view for a VIP.
func (s *Store) Evidence(vip VIPIdentifier) Evidence {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ev := Evidence{VIP: vip}
	if d, ok := s.desired[vip.Name]; ok && d.VIP.IP == vip.IP {
		ev.Desired = &d
	}
	if ring := s.health[vip.Name]; len(ring) > 0 {
		last := ring[len(ring)-1]
		ev.LatestHealth = &last
	}
	if ring := s.advertise[vip.Name]; len(ring) > 0 {
		last := ring[len(ring)-1]
		ev.Advertisement = &last
	}
	return ev
}

// AllVIPs returns every observed VIP identifier sorted
// by name.
func (s *Store) AllVIPs() []VIPIdentifier {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]VIPIdentifier, 0, len(s.desired))
	for _, d := range s.desired {
		out = append(out, d.VIP)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AllEvidence returns every composite view sorted by
// VIP name.
func (s *Store) AllEvidence() []Evidence {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Evidence, 0, len(s.desired))
	for _, d := range s.desired {
		vip := d.VIP
		ev := Evidence{VIP: vip, Desired: &d}
		if ring := s.health[vip.Name]; len(ring) > 0 {
			last := ring[len(ring)-1]
			ev.LatestHealth = &last
		}
		if ring := s.advertise[vip.Name]; len(ring) > 0 {
			last := ring[len(ring)-1]
			ev.Advertisement = &last
		}
		out = append(out, ev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VIP.Name < out[j].VIP.Name })
	return out
}

// Counters summarises the store.
type Counters struct {
	VIPs      int `json:"vips"`
	Health    int `json:"health"`
	Advertise int `json:"advertise"`
}

// Counters returns the current counts.
func (s *Store) Counters() Counters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	health := 0
	for _, ring := range s.health {
		health += len(ring)
	}
	advertise := 0
	for _, ring := range s.advertise {
		advertise += len(ring)
	}
	return Counters{VIPs: len(s.desired), Health: health, Advertise: advertise}
}

// GCExpired removes evidence older than the configured
// age ceiling. Returns the total number of records
// removed across all rings.
func (s *Store) GCExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	removed := 0
	for k, ring := range s.health {
		kept := ring[:0]
		for _, h := range ring {
			if now.Sub(h.ObservedAt) <= s.maxAge {
				kept = append(kept, h)
			} else {
				removed++
			}
		}
		if len(kept) == 0 {
			delete(s.health, k)
		} else {
			s.health[k] = kept
		}
	}
	for k, ring := range s.advertise {
		kept := ring[:0]
		for _, a := range ring {
			if now.Sub(a.ObservedAt) <= s.maxAge {
				kept = append(kept, a)
			} else {
				removed++
			}
		}
		if len(kept) == 0 {
			delete(s.advertise, k)
		} else {
			s.advertise[k] = kept
		}
	}
	return removed
}

// trimDesiredLocked removes the oldest desired records
// beyond the global cap. Caller must hold s.mu.
func (s *Store) trimDesiredLocked() {
	if len(s.desired) <= s.maxVIPs {
		return
	}
	type kv struct {
		name string
		old  time.Time
	}
	var all []kv
	for name, d := range s.desired {
		all = append(all, kv{name: name, old: d.UpdatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].old.Before(all[j].old) })
	for _, k := range all[:len(all)-s.maxVIPs] {
		delete(s.desired, k.name)
		delete(s.health, k.name)
		delete(s.advertise, k.name)
	}
}
