// localfirst/localfirst.go: V1
// local-first / cloud-independent
// contract. The package owns the typed
// surface that lets the runtime and the
// future UI answer:
//
//	"Does this operation require cloud /
//	 internet connectivity?"
//
// The V1 contract: every subsystem that
// could be tempted to depend on DuckOS
// cloud services must declare its
// Connectivity requirement. The Manager
// rolls the requirements up so the API
// can answer "is the system fully local-
// first at this moment?".
package localfirst

import (
	"errors"
	"sort"
	"sync"
)

// Connectivity is the dependency class.
type Connectivity string

const (
	// ConnLocal means the operation is
	// fully local: routing, firewall,
	// management, recovery, diagnostics,
	// local auth.
	ConnLocal Connectivity = "local"
	// ConnOptional means the operation
	// works without internet but accepts
	// updates from DuckOS cloud when
	// available.
	ConnOptional Connectivity = "optional"
	// ConnRequired means the operation
	// requires DuckOS cloud services.
	// V1 forbids this for the core
	// subsystems; ConnRequired is
	// declared only for features that are
	// explicitly cloud-only.
	ConnRequired Connectivity = "required"
)

// Valid reports whether c is known.
func (c Connectivity) Valid() bool {
	switch c {
	case ConnLocal, ConnOptional, ConnRequired:
		return true
	}
	return false
}

// Subsystem is a first-class subsystem.
type Subsystem string

const (
	SubRouting       Subsystem = "routing"
	SubFirewall      Subsystem = "firewall"
	SubManagement    Subsystem = "management"
	SubRecovery      Subsystem = "recovery"
	SubDiagnostics   Subsystem = "diagnostics"
	SubAuth          Subsystem = "auth"
	SubVPN           Subsystem = "vpn"
	SubXray          Subsystem = "xray"
	SubDDNS          Subsystem = "ddns"
	SubACME          Subsystem = "acme"
	SubSubscriptions Subsystem = "subscriptions"
	SubUpdates       Subsystem = "updates"
)

// AllSubsystems returns every canonical
// subsystem key sorted.
func AllSubsystems() []Subsystem {
	return []Subsystem{
		SubACME, SubAuth, SubDDNS, SubDiagnostics, SubFirewall,
		SubManagement, SubRecovery, SubRouting, SubSubscriptions,
		SubUpdates, SubVPN, SubXray,
	}
}

// Declaration binds a subsystem to its
// connectivity requirement and the
// runtime reason.
type Declaration struct {
	Subsystem    Subsystem    `json:"subsystem"`
	Connectivity Connectivity `json:"connectivity"`
	Reason       string       `json:"reason,omitempty"`
}

// Valid reports whether d is well-formed.
func (d Declaration) Valid() error {
	if d.Subsystem == "" {
		return errors.New("localfirst: subsystem is required")
	}
	if !d.Connectivity.Valid() {
		return errors.New("localfirst: connectivity is invalid")
	}
	return nil
}

// Manager is the V1 local-first store.
// Manager is goroutine-safe.
type Manager struct {
	mu    sync.Mutex
	decls map[Subsystem]Declaration
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{decls: map[Subsystem]Declaration{}}
}

// Set replaces the declaration for a
// subsystem.
func (m *Manager) Set(d Declaration) error {
	if err := d.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decls[d.Subsystem] = d
	return nil
}

// Get returns the declaration for a
// subsystem.
func (m *Manager) Get(s Subsystem) (Declaration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.decls[s]
	return d, ok
}

// List returns every declaration sorted by
// subsystem.
func (m *Manager) List() []Declaration {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Declaration, 0, len(m.decls))
	for _, d := range m.decls {
		out = append(out, d)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Subsystem < out[k].Subsystem })
	return out
}

// CloudIndependentReport is the result of
// the "is the system fully cloud-
// independent at this moment?" check.
type CloudIndependentReport struct {
	LocalFirst   bool        `json:"localFirst"`
	Violations   []Subsystem `json:"violations,omitempty"`
	OptionalDeps []Subsystem `json:"optionalDeps,omitempty"`
}

// Evaluate returns the cloud-independence
// report. A system is local-first when no
// subsystem declares ConnRequired.
func (m *Manager) Evaluate() CloudIndependentReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	rep := CloudIndependentReport{LocalFirst: true}
	for _, d := range m.decls {
		switch d.Connectivity {
		case ConnRequired:
			rep.LocalFirst = false
			rep.Violations = append(rep.Violations, d.Subsystem)
		case ConnOptional:
			rep.OptionalDeps = append(rep.OptionalDeps, d.Subsystem)
		}
	}
	sort.Slice(rep.Violations, func(i, k int) bool { return rep.Violations[i] < rep.Violations[k] })
	sort.Slice(rep.OptionalDeps, func(i, k int) bool { return rep.OptionalDeps[i] < rep.OptionalDeps[k] })
	return rep
}
