// observe/drift.go: V1 desired vs observed
// state model. The Engine separates the
// canonical, configured state from the
// observed runtime state; drift between the
// two is a first-class object the future UI
// renders explicitly.
//
// A drift is an observable difference between
// a configured object and the kernel/service
// state. Common causes:
//   - operator ran `ip` or `nft` by hand
//   - a service crashed
//   - a tunnel went down
//   - an external system (peer, upstream)
//     changed
//
// The package owns the Drift model. A Drift
// has a target (kind, id), the configured
// intent, the observed state, the reason, and
// the reconciliation policy.
package observe

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// DriftKind enumerates the V1 drift kinds.
type DriftKind string

const (
	DriftLink     DriftKind = "link"
	DriftAddress  DriftKind = "address"
	DriftRoute    DriftKind = "route"
	DriftFirewall DriftKind = "firewall"
	DriftNAT      DriftKind = "nat"
	DriftTunnel   DriftKind = "tunnel"
	DriftService  DriftKind = "service"
	DriftDNS      DriftKind = "dns"
	DriftDHCP     DriftKind = "dhcp"
	DriftBGP      DriftKind = "bgp"
	DriftOSPF     DriftKind = "ospf"
	DriftVRRP     DriftKind = "vrrp"
)

// ReconciliationPolicy is what the operator
// chose to do with a drift.
type ReconciliationPolicy string

const (
	PolicyAutoFix  ReconciliationPolicy = "auto-fix"
	PolicyAlert    ReconciliationPolicy = "alert"
	PolicyManual   ReconciliationPolicy = "manual"
	PolicyDisabled ReconciliationPolicy = "disabled"
)

// Drift is a single observed-vs-desired
// delta. Configured / Observed are canonical
// JSON of the relevant slice of state.
type Drift struct {
	Kind        DriftKind            `json:"kind"`
	Target      string               `json:"target"`
	Configured  json.RawMessage      `json:"configured,omitempty"`
	Observed    json.RawMessage      `json:"observed,omitempty"`
	Reason      string               `json:"reason"`
	Severity    string               `json:"severity"`
	Policy      ReconciliationPolicy `json:"policy"`
	DetectedAt  time.Time            `json:"detectedAt"`
	LastError   string               `json:"lastError,omitempty"`
	LastApplyAt time.Time            `json:"lastApplyAt,omitempty"`
}

var targetPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,127}$`)

// Valid reports whether d is well-formed.
func (d Drift) Valid() error {
	if d.Kind == "" {
		return errors.New("drift: kind is required")
	}
	if !targetPattern.MatchString(d.Target) {
		return errors.New("drift: target is not canonical")
	}
	switch d.Policy {
	case PolicyAutoFix, PolicyAlert, PolicyManual, PolicyDisabled:
	default:
		return errors.New("drift: policy is invalid")
	}
	if d.DetectedAt.IsZero() {
		return errors.New("drift: detectedAt is required")
	}
	return nil
}

// Store is the in-memory + durable store of
// drifts. Store is goroutine-safe.
type Store struct {
	mu    sync.Mutex
	drift map[string]Drift // keyed by kind + target
	clock func() time.Time
}

// NewStore returns an empty store.
func NewStore(clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	return &Store{drift: map[string]Drift{}, clock: clock}
}

func key(k DriftKind, target string) string { return string(k) + "|" + target }

// Set replaces or inserts a drift. The store
// rejects an invalid drift.
func (s *Store) Set(d Drift) error {
	if err := d.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.DetectedAt.IsZero() {
		d.DetectedAt = s.clock().UTC()
	}
	s.drift[key(d.Kind, d.Target)] = d
	return nil
}

// Get returns a drift by kind+target.
func (s *Store) Get(k DriftKind, target string) (Drift, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drift[key(k, target)]
	return d, ok
}

// List returns all drifts, sorted by severity
// then target.
func (s *Store) List() []Drift {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Drift, 0, len(s.drift))
	for _, d := range s.drift {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// Delete removes a drift.
func (s *Store) Delete(k DriftKind, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.drift, key(k, target))
}

// Report is the public surface a Doctor
// snapshot or a runtime check feeds into the
// store.
type Report struct {
	At     time.Time             `json:"at"`
	ByKind map[DriftKind][]Drift `json:"byKind"`
}

// Detect computes the desired/observed delta
// for a single object. The function is a thin
// helper; the real detectors live in the
// subsystems that own the data.
func Detect(configured, observed json.RawMessage) (equal bool, reason string) {
	if string(configured) == string(observed) {
		return true, ""
	}
	return false, "configured != observed"
}
