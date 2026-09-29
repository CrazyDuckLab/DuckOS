// observability/observability.go: V1
// per-object Doctor surface. The package
// owns the typed contracts for health
// snapshot of every first-class subsystem:
// interfaces, routes, neighbors, ARP/ND,
// FDB, conntrack, firewall, NAT, BGP/OSPF
// sessions, VRRP, tunnels, DNS, DHCP, QoS,
// hardware offload, services, storage, UPS,
// modem, certificates, updates.
//
// The runtime is a thin mapping of the
// existing per-subsystem stores to a
// uniform Doctor snapshot.
package observability

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Severity is the per-object health level.
type Severity string

const (
	SeverityOK       Severity = "ok"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Valid reports whether s is known.
func (s Severity) Valid() bool {
	switch s {
	case SeverityOK, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical:
		return true
	}
	return false
}

// Order returns the severity rank, higher
// is more severe.
func (s Severity) Order() int {
	switch s {
	case SeverityOK:
		return 0
	case SeverityInfo:
		return 1
	case SeverityWarning:
		return 2
	case SeverityError:
		return 3
	case SeverityCritical:
		return 4
	}
	return -1
}

// Object is a single typed object with
// health, evidence, and last-update time.
type Object struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Severity  Severity  `json:"severity"`
	Summary   string    `json:"summary"`
	Details   []string  `json:"details,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Doctor is the V1 doctor snapshot. It
// lists every observed object, sorted by
// severity descending and then by kind/id.
type Doctor struct {
	GeneratedAt time.Time `json:"generatedAt"`
	Objects     []Object  `json:"objects"`
	// Counts is a quick severity histogram.
	Counts map[Severity]int `json:"counts"`
	// WorstSeverity is the highest severity
	// in the snapshot.
	WorstSeverity Severity `json:"worstSeverity"`
}

// EmptySeverity is returned for a Doctor
// with no objects.
const EmptySeverity Severity = SeverityOK

// NewDoctor returns an empty doctor.
func NewDoctor() *Doctor {
	return &Doctor{Counts: map[Severity]int{}, WorstSeverity: EmptySeverity}
}

// Add inserts an object. The doctor's
// worst-severity and counts are updated.
func (d *Doctor) Add(o Object) {
	if !o.Severity.Valid() {
		o.Severity = SeverityInfo
	}
	d.Objects = append(d.Objects, o)
	d.Counts[o.Severity]++
	if o.Severity.Order() > d.WorstSeverity.Order() {
		d.WorstSeverity = o.Severity
	}
}

// Sort orders objects by severity
// descending, then by kind/id ascending.
func (d *Doctor) Sort() {
	sort.Slice(d.Objects, func(i, k int) bool {
		if d.Objects[i].Severity.Order() != d.Objects[k].Severity.Order() {
			return d.Objects[i].Severity.Order() > d.Objects[k].Severity.Order()
		}
		if d.Objects[i].Kind != d.Objects[k].Kind {
			return d.Objects[i].Kind < d.Objects[k].Kind
		}
		return d.Objects[i].ID < d.Objects[k].ID
	})
}

// Snapshot is the read-only view returned
// to the API.
func (d *Doctor) Snapshot() Doctor {
	d.Sort()
	out := Doctor{
		GeneratedAt:   time.Now().UTC(),
		Objects:       append([]Object(nil), d.Objects...),
		Counts:        map[Severity]int{},
		WorstSeverity: d.WorstSeverity,
	}
	for k, v := range d.Counts {
		out.Counts[k] = v
	}
	return out
}

// Source is a subsystem that produces
// per-object health. The Doctor calls
// each registered source in turn. The
// source must be safe for concurrent use.
type Source interface {
	Kind() string
	Collect() []Object
}

// Registry is the typed Source registry.
// Registry is goroutine-safe.
type Registry struct {
	mu      sync.Mutex
	sources []Source
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register appends a source. Sources with
// the same kind replace older ones; this
// keeps the snapshot flat.
func (r *Registry) Register(s Source) error {
	if s == nil {
		return errors.New("observability: nil source")
	}
	if s.Kind() == "" {
		return errors.New("observability: source kind is empty")
	}
	if f, ok := s.(FuncSource); ok && f.Fn == nil {
		return errors.New("observability: FuncSource has nil function")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, x := range r.sources {
		if x.Kind() == s.Kind() {
			r.sources[i] = s
			return nil
		}
	}
	r.sources = append(r.sources, s)
	return nil
}

// Collect runs every source and returns a
// fresh Doctor.
func (r *Registry) Collect() Doctor {
	r.mu.Lock()
	srcs := append([]Source(nil), r.sources...)
	r.mu.Unlock()
	d := NewDoctor()
	for _, s := range srcs {
		for _, o := range s.Collect() {
			if o.Kind == "" {
				o.Kind = s.Kind()
			}
			if o.UpdatedAt.IsZero() {
				o.UpdatedAt = time.Now().UTC()
			}
			d.Add(o)
		}
	}
	return d.Snapshot()
}

// FuncSource adapts a plain function to the
// Source interface. The kind is fixed at
// registration.
type FuncSource struct {
	KindValue string
	Fn        func() []Object
}

// Kind returns the source's kind.
func (f FuncSource) Kind() string { return f.KindValue }

// Collect runs the supplied function.
func (f FuncSource) Collect() []Object { return f.Fn() }
