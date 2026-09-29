// management/plane.go: V1 management plane
// model. The management plane is the set of
// interfaces, VLANs, VRFs, and tunnels the
// Agent binds its web/API/SSH listeners to.
// The data plane is everything else.
//
// The package owns:
//   - the canonical ManagementPolicy (which
//     interfaces are management-capable,
//     which are management-only, which are
//     OOB)
//   - the bind constraint used by the
//     listener when accepting connections
//   - the commit-confirmed path for changing
//     the management IP without losing
//     access
package management

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// AccessRole is the kind of access the
// management plane allows.
type AccessRole string

const (
	RoleWeb AccessRole = "web"
	RoleAPI AccessRole = "api"
	RoleSSH AccessRole = "ssh"
	RoleAll AccessRole = "all"
)

// PathKind is the kind of management path.
type PathKind string

const (
	PathInterface PathKind = "interface"
	PathVLAN      PathKind = "vlan"
	PathVRF       PathKind = "vrf"
	PathTunnel    PathKind = "tunnel"
	PathNetwork   PathKind = "network"
)

// Path is a single allowed management path.
type Path struct {
	Kind           PathKind     `json:"kind"`
	Target         string       `json:"target"`
	Roles          []AccessRole `json:"roles"`
	OOBOnly        bool         `json:"oobOnly"`
	ManagementOnly bool         `json:"managementOnly"`
	Comment        string       `json:"comment,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,63}$`)

// Valid reports whether p is well-formed.
func (p Path) Valid() error {
	if !idPattern.MatchString(p.Target) {
		return errors.New("management: path target is not canonical")
	}
	switch p.Kind {
	case PathInterface, PathVLAN, PathVRF, PathTunnel, PathNetwork:
	default:
		return errors.New("management: path kind is unknown")
	}
	for _, r := range p.Roles {
		switch r {
		case RoleWeb, RoleAPI, RoleSSH, RoleAll:
		default:
			return errors.New("management: role is unknown")
		}
	}
	return nil
}

// Policy is the management plane configuration.
type Policy struct {
	// OOBInterface is the dedicated
	// out-of-band interface, when present.
	// OOB interfaces are management-only and
	// cannot be repurposed for data plane
	// traffic without an explicit operator
	// override.
	OOBInterface string `json:"oobInterface,omitempty"`
	// Paths is the set of management paths.
	Paths []Path `json:"paths"`
	// AllowManagementOverDataPlane is the
	// explicit "remote VPS" toggle; when
	// true, a single-interface server is
	// allowed to bind management to its only
	// NIC.
	AllowManagementOverDataPlane bool `json:"allowManagementOverDataPlane"`
	// CommitConfirmedWindow is how long a
	// commit-confirmed transaction waits for
	// the operator's explicit Confirm. A
	// missed confirmation rolls back.
	CommitConfirmedWindow string `json:"commitConfirmedWindow,omitempty"`
}

// Manager is the V1 management-plane gate. The
// Manager is goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	policy Policy
}

// NewManager returns a Manager with the given
// starting policy.
func NewManager(p Policy) *Manager {
	return &Manager{policy: p}
}

// Policy returns a copy of the current policy.
func (m *Manager) Policy() Policy {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.policy
	out.Paths = append([]Path(nil), m.policy.Paths...)
	return out
}

// Set replaces the policy. The Manager
// validates every Path before committing.
func (m *Manager) Set(p Policy) error {
	for _, path := range p.Paths {
		if err := path.Valid(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.policy = p
	m.mu.Unlock()
	return nil
}

// AddPath registers a new management path.
func (m *Manager) AddPath(p Path) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.policy.Paths {
		if existing.Kind == p.Kind && existing.Target == p.Target {
			return errors.New("management: path already exists")
		}
	}
	m.policy.Paths = append(m.policy.Paths, p)
	return nil
}

// RemovePath drops a management path.
func (m *Manager) RemovePath(kind PathKind, target string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.policy.Paths {
		if p.Kind == kind && p.Target == target {
			m.policy.Paths = append(m.policy.Paths[:i], m.policy.Paths[i+1:]...)
			return nil
		}
	}
	return errors.New("management: path not found")
}

// Allows reports whether a connection from
// the given (kind, target) is permitted for
// the given role.
func (m *Manager) Allows(kind PathKind, target, role string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.policy.OOBInterface != "" && kind == PathInterface && target == m.policy.OOBInterface {
		// OOB interface is always permitted
		// for the management roles.
		return true
	}
	for _, p := range m.policy.Paths {
		if p.Kind != kind || p.Target != target {
			continue
		}
		for _, r := range p.Roles {
			if r == AccessRole(role) || r == RoleAll {
				return true
			}
		}
	}
	return false
}

// ListPaths returns the registered paths
// sorted by (kind, target) for stable output.
func (m *Manager) ListPaths() []Path {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]Path(nil), m.policy.Paths...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Target < out[j].Target
	})
	return out
}
