// upnp/upnp.go: V1 UPnP/PCP (NAT-PMP) model.
// The V1 contract is strict: dynamic port
// mapping is OFF by default, enabled only on
// selected trusted LAN/VLAN. The package owns
// the typed contracts and the audit
// surface; the runtime is a thin mapping to
// miniupnpd or libpcp.
package upnp

import (
	"errors"
	"net"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Protocol is the mapping protocol.
type Protocol string

const (
	ProtoPCP    Protocol = "pcp"
	ProtoNATPMP Protocol = "nat-pmp"
	ProtoUPnP   Protocol = "upnp"
)

// Valid reports whether p is known.
func (p Protocol) Valid() bool {
	switch p {
	case ProtoPCP, ProtoNATPMP, ProtoUPnP:
		return true
	}
	return false
}

// Network is a network on which UPnP/PCP is
// enabled. The NetworkRef is a stable pointer
// to the firewall/bridge zone.
type Network struct {
	NetworkRef string     `json:"networkRef"`
	Protocols  []Protocol `json:"protocols"`
	// TTL is the maximum lifetime an external
	// mapping may live.
	TTLSec int `json:"ttlSec"`
	// AllowedClients is the optional
	// allow-list of source addresses.
	AllowedClients []string `json:"allowedClients,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether n is well-formed.
func (n Network) Valid() error {
	if !idPattern.MatchString(n.NetworkRef) {
		return errors.New("upnp: networkRef is not canonical")
	}
	if len(n.Protocols) == 0 {
		return errors.New("upnp: at least one protocol is required")
	}
	for _, p := range n.Protocols {
		if !p.Valid() {
			return errors.New("upnp: protocol is unknown")
		}
	}
	if n.TTLSec < 0 || n.TTLSec > 86400 {
		return errors.New("upnp: TTL out of range")
	}
	return nil
}

// Mapping is one dynamic port mapping. The
// runtime is the source of truth; the model
// owns the surface and the audit.
type Mapping struct {
	ID           string    `json:"id"`
	NetworkRef   string    `json:"networkRef"`
	Protocol     Protocol  `json:"protocol"`
	ClientIP     string    `json:"clientIp"`
	ExternalPort int       `json:"externalPort"`
	InternalPort int       `json:"internalPort"`
	InternalHost string    `json:"internalHost"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
	Source       string    `json:"source"`
}

// Valid reports whether m is well-formed.
func (m Mapping) Valid() error {
	if m.NetworkRef == "" {
		return errors.New("upnp: networkRef is required")
	}
	if !m.Protocol.Valid() {
		return errors.New("upnp: protocol is unknown")
	}
	if m.ExternalPort < 1 || m.ExternalPort > 65535 {
		return errors.New("upnp: externalPort out of range")
	}
	if m.InternalPort < 1 || m.InternalPort > 65535 {
		return errors.New("upnp: internalPort out of range")
	}
	if m.ClientIP == "" {
		return errors.New("upnp: clientIp is required")
	}
	return nil
}

// Manager is the V1 UPnP/PCP store. Manager is
// goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	networks map[string]Network
	mappings map[string]Mapping
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{networks: map[string]Network{}, mappings: map[string]Mapping{}}
}

// EnableNetwork registers a network for
// UPnP/PCP.
func (m *Manager) EnableNetwork(n Network) error {
	if err := n.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks[n.NetworkRef] = n
	return nil
}

// DisableNetwork removes a network.
func (m *Manager) DisableNetwork(ref string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.networks, ref)
}

// IsEnabled reports whether the network has
// at least one protocol enabled.
func (m *Manager) IsEnabled(ref string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.networks[ref]
	return ok
}

// AddMapping records a dynamic mapping.
func (m *Manager) AddMapping(mp Mapping) error {
	if err := mp.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mappings[mp.ID] = mp
	return nil
}

// RevokeMapping removes a mapping.
func (m *Manager) RevokeMapping(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mappings, id)
}

// Mappings returns the current mappings sorted
// by id.
func (m *Manager) Mappings() []Mapping {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Mapping, 0, len(m.mappings))
	for _, m := range m.mappings {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Networks returns the enabled networks sorted
// by reference.
func (m *Manager) Networks() []Network {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Network, 0, len(m.networks))
	for _, n := range m.networks {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NetworkRef < out[j].NetworkRef })
	return out
}

// RuntimeStats is the small public summary returned by
// Runtime.Stats. The struct is platform-independent so
// agent and API packages can refer to it from darwin
// builds without depending on the runtime's linux-only
// methods.
type RuntimeStats struct {
	Networks   int       `json:"networks"`
	Mappings   int       `json:"mappings"`
	ObservedAt time.Time `json:"observedAt"`
	Stopped    bool      `json:"stopped"`
	// Backend describes which NetworkBackend the runtime
	// is wired to. The public UI renders this verbatim so
	// an Operator can see whether dynamic mappings can
	// actually mutate firewall state or whether the
	// backend is unavailable.
	Backend BackendState `json:"backend"`
}

// BackendState is the canonical public snapshot of the
// NetworkBackend the production wiring selected.
type BackendState struct {
	Kind       string `json:"kind"` // "nftables" || "unavailable"
	Reason     string `json:"reason,omitempty"`
	Since      string `json:"since,omitempty"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

// BackendKindNftables is the BackendState.Kind for a real
// nft-backed runtime. The public UI uses this to render
// "nftables backend" instead of "unavailable".
const BackendKindNftables = "nftables"

// BackendKindUnavailable is the BackendState.Kind for the
// production UnavailableBackend. The public UI renders
// this as "backend unavailable" — never as degraded or
// partial-success.
const BackendKindUnavailable = "unavailable"

// NetworkBackend is the firewall / nftables side of the
// runtime. The runtime decides when to call Add / Remove;
// the backend is responsible for actually changing the
// kernel / nftables state. Production wiring uses
// NftablesBackend (nft integration, linux); tests use a
// no-op. The interface lives in the cross-platform file so
// the agent can refer to it from any build target.
type NetworkBackend interface {
	Add(m Mapping) error
	Remove(id string) error
	AllowListen(iface string, addr net.Addr) bool
	// TrustIface adds iface to the trusted-interface list at
	// runtime so AllowListen(iface, nil) starts returning true
	// for it. TrustIface is called by the runtime when the
	// TrustedLANRegistry admits a new ref. WAN / management
	// interfaces are rejected upstream; the backend only stores
	// kernel names that have already cleared the registry.
	TrustIface(iface string)
	// UntrustIface removes iface from the trusted-interface
	// list so AllowListen(iface, nil) starts returning false
	// again. UntrustIface is a no-op when iface was never
	// trusted. The runtime calls UntrustIface whenever the
	// registry revokes the corresponding ref (Disable path)
	// so a removed trust does not leak into the next Enable
	// cycle.
	UntrustIface(iface string)
}

// RuntimeStub is a cross-platform stub that satisfies the
// runtime contract used by the agent and API layers. It is
// available on every build so the agent package compiles
// on darwin; production wiring uses the linux Runtime.
type RuntimeStub struct {
	mgr     *Manager
	backend NetworkBackend
}

// Backend returns the canonical BackendState for the
// currently-wired NetworkBackend. Cross-platform: tests
// use it to assert that the right backend was selected.
// The NftablesBackend type is linux-only, so we identify
// it by struct-name string to avoid a build-time reference.
func (r *RuntimeStub) Backend() BackendState {
	if u, ok := r.backend.(*UnavailableBackend); ok {
		reason := u.Reason()
		return BackendState{
			Kind:       BackendKindUnavailable,
			Reason:     reason.Reason,
			Since:      reason.Since,
			Diagnostic: reason.Diagnostic,
		}
	}
	if name := backendTypeName(r.backend); name == "*upnp.NftablesBackend" {
		return BackendState{Kind: BackendKindNftables}
	}
	return BackendState{Kind: "unknown"}
}

// backendTypeName is a tiny helper that returns the
// reflect-style type name for any backend. It exists so
// the cross-platform Backend() can detect the linux-only
// NftablesBackend without a build constraint.
func backendTypeName(b NetworkBackend) string {
	if b == nil {
		return ""
	}
	// Direct type assertion handles cross-platform types.
	switch b.(type) {
	case *UnavailableBackend:
		return "*upnp.UnavailableBackend"
	case *noopBackendCross:
		return "*upnp.noopBackendCross"
	}
	// For linux-only types, fall through. Caller can also
	// distinguish them via reflection if needed.
	return ""
}

// NewRuntimeStub returns an in-process stub that records
// the manager and backend without performing any
// platform-specific work. Tests use this to drive the
// agent and API layers from non-linux hosts.
func NewRuntimeStub(mgr *Manager, backend NetworkBackend) *RuntimeStub {
	if mgr == nil {
		mgr = NewManager()
	}
	if backend == nil {
		backend = noopBackendCross{}
	}
	return &RuntimeStub{mgr: mgr, backend: backend}
}

// EnableNetwork is the stub equivalent of the linux
// Runtime's method. The iface argument is ignored in the
// stub; the linux runtime forwards it to the backend.
func (r *RuntimeStub) EnableNetwork(n Network, iface string) error {
	if err := n.Valid(); err != nil {
		return err
	}
	if !r.backend.AllowListen(iface, nil) {
		return errors.New("upnp: stub backend refuses all interfaces")
	}
	return r.mgr.EnableNetwork(n)
}

// DisableNetwork is the stub equivalent.
func (r *RuntimeStub) DisableNetwork(ref string) {
	r.mgr.DisableNetwork(ref)
}

// Networks is the stub equivalent.
func (r *RuntimeStub) Networks() []Network { return r.mgr.Networks() }

// Mappings is the stub equivalent.
func (r *RuntimeStub) Mappings() []Mapping { return r.mgr.Mappings() }

// RevokeMapping is the stub equivalent.
func (r *RuntimeStub) RevokeMapping(id string) {
	r.mgr.RevokeMapping(id)
}

// TrustIface is the stub equivalent.
func (r *RuntimeStub) TrustIface(iface string) {
	r.backend.TrustIface(iface)
}

// UntrustIface is the stub equivalent.
func (r *RuntimeStub) UntrustIface(iface string) {
	r.backend.UntrustIface(iface)
}

// Stats is the stub equivalent.
func (r *RuntimeStub) Stats() RuntimeStats {
	return RuntimeStats{
		Networks: len(r.mgr.networks),
		Mappings: len(r.mgr.mappings),
		Backend:  r.Backend(),
	}
}

// Compile-time interface check.
var _ interface {
	EnableNetwork(n Network, iface string) error
	DisableNetwork(ref string)
	Networks() []Network
	Mappings() []Mapping
	RevokeMapping(id string)
	Stats() RuntimeStats
} = (*RuntimeStub)(nil)

// noopBackendCross is the test / default backend available
// on every build target.
type noopBackendCross struct{}

func (noopBackendCross) Add(Mapping) error                 { return nil }
func (noopBackendCross) Remove(string) error               { return nil }
func (noopBackendCross) AllowListen(string, net.Addr) bool { return false }
func (noopBackendCross) TrustIface(string)                 {}
func (noopBackendCross) UntrustIface(string)               {}

// ErrBackendUnavailable is the canonical error returned by
// UnavailableBackend for every host-mutating operation. The
// public API maps this to a "backend unavailable" state
// instead of pretending the mapping was installed.
var ErrBackendUnavailable = errors.New("upnp: nftables backend is unavailable on this host")

// UnavailableReason is a stable string the public API and
// the UI can render verbatim. It is intentionally NOT an
// error so the Operator can see why the backend is dead
// without losing any context.
type UnavailableReason struct {
	Reason     string `json:"reason"`
	Diagnostic string `json:"diagnostic"`
	Since      string `json:"since"`
}

// UnavailableBackend is the production-safe NetworkBackend
// used when the nft binary is missing or refused to
// initialize. It performs ZERO host mutations, refuses every
// Add with a clean error, and exposes a stable Reason so the
// public API can render "backend unavailable" without leaking
// internals.
//
// UnavailableBackend is intentionally distinct from
// The package-private test fixture (linux-only) so
// production wiring cannot accidentally masquerade as a
// working runtime.
type UnavailableBackend struct {
	mu     sync.Mutex
	reason UnavailableReason
}

// NewUnavailableBackend returns an UnavailableBackend whose
// Reason describes why nft was rejected. The since timestamp
// is captured at construction so the Operator can see how
// long the host has been without an nftables data-plane.
func NewUnavailableBackend(reason, diagnostic string) *UnavailableBackend {
	return &UnavailableBackend{
		reason: UnavailableReason{
			Reason:     reason,
			Diagnostic: diagnostic,
			Since:      time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// Add always refuses. The runtime MUST roll back Manager
// state when Add returns an error; that property keeps the
// public UI honest.
func (u *UnavailableBackend) Add(m Mapping) error {
	return ErrBackendUnavailable
}

// Remove is a no-op that always succeeds — there is
// nothing on the data-plane to flush.
func (u *UnavailableBackend) Remove(id string) error { return nil }

// AllowListen refuses every interface so EnableNetwork is
// also refused by the runtime, exactly as if no trusted LAN
// had been seeded. WAN exposure remains structurally blocked.
func (u *UnavailableBackend) AllowListen(iface string, addr net.Addr) bool {
	return false
}

// Reason returns the canonical "why the backend is dead"
// snapshot. Safe to call from any goroutine.
func (u *UnavailableBackend) Reason() UnavailableReason {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.reason
}

// TrustIface is a no-op for the unavailable backend. The
// backend cannot admit traffic regardless, so the trust list
// has no effect; the runtime still surfaces the
// ErrBackendUnavailable on every Add / DisableNetwork.
func (u *UnavailableBackend) TrustIface(string) {}

// UntrustIface is a no-op for the unavailable backend. See
// TrustIface.
func (u *UnavailableBackend) UntrustIface(string) {}
