// tunnels/tunnels.go: V1 tunnel model covering
// IPsec/IKEv2, classic tunnels (GRE, GRETAP,
// IPIP, VTI), and overlay networks (VXLAN
// with optional EVPN). The model is the
// contract the future UI binds to; the
// runtime is a thin mapping to strongSwan
// (charon) for IPsec, `ip tunnel` for
// classic, and `ip link add type vxlan` for
// overlays. The MPLS/L3VPN surface is also
// captured here as a typed model.
//
// The V1 contract is: every tunnel is owned
// by a vault-managed secret; secrets never
// appear in logs; every tunnel reports
// configured vs observed (handshake for
// WireGuard, SA state for IPsec).
package tunnels

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Kind enumerates the V1 tunnel kinds.
type Kind string

const (
	KindIPsecSite Kind = "ipsec-site"
	KindIPsecRA   Kind = "ipsec-ra"
	KindGRE       Kind = "gre"
	KindGRETAP    Kind = "gretap"
	KindIPIP      Kind = "ipip"
	KindVTI       Kind = "vti"
	KindVXLAN     Kind = "vxlan"
	KindMPLS      Kind = "mpls"
)

// Valid reports whether k is known.
func (k Kind) Valid() bool {
	switch k {
	case KindIPsecSite, KindIPsecRA, KindGRE, KindGRETAP, KindIPIP, KindVTI, KindVXLAN, KindMPLS:
		return true
	}
	return false
}

// State is the runtime state of a tunnel.
type State string

const (
	StateDown      State = "down"
	StateUp        State = "up"
	StateHandshake State = "handshake"
	StateError     State = "error"
)

// Valid reports whether s is known.
func (s State) Valid() bool {
	switch s {
	case StateDown, StateUp, StateHandshake, StateError:
		return true
	}
	return false
}

// Tunnel is the V1 tunnel model. The
// per-kind-specific fields are embedded as
// sub-configs; only the relevant sub-config
// is set for a given Kind.
type Tunnel struct {
	ID         string `json:"id"`
	Kind       Kind   `json:"kind"`
	Name       string `json:"name"`
	VRF        string `json:"vrf,omitempty"`
	State      State  `json:"state"`
	LastChange int64  `json:"lastChange"`
	// SecretRef is a vault reference to the
	// per-tunnel key material. The runtime
	// resolves it through the VaultResolver.
	SecretRef string `json:"secretRef,omitempty"`
	// Local / Remote are the canonical
	// endpoint addresses.
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
	// MTU is the tunnel MTU. Zero means
	// "kernel default".
	MTU int `json:"mtu,omitempty"`
	// IPSec holds IPsec-specific config.
	IPSec *IPSecConfig `json:"ipsec,omitempty"`
	// VXLAN holds VXLAN-specific config.
	VXLAN *VXLANConfig `json:"vxlan,omitempty"`
	// MPLS holds MPLS-specific config.
	MPLS *MPLSConfig `json:"mpls,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether t is well-formed.
func (t Tunnel) Valid() error {
	if !idPattern.MatchString(t.ID) {
		return errors.New("tunnels: id is not canonical")
	}
	if !t.Kind.Valid() {
		return errors.New("tunnels: kind is unknown")
	}
	if !t.State.Valid() {
		return errors.New("tunnels: state is unknown")
	}
	if t.Kind == KindIPsecSite || t.Kind == KindIPsecRA {
		if t.IPSec == nil {
			return errors.New("tunnels: IPsec tunnel requires an IPsec config")
		}
		if err := t.IPSec.Valid(); err != nil {
			return err
		}
	}
	if t.Kind == KindVXLAN {
		if t.VXLAN == nil {
			return errors.New("tunnels: VXLAN requires a vxlan config")
		}
		if err := t.VXLAN.Valid(); err != nil {
			return err
		}
	}
	if t.Kind == KindMPLS {
		if t.MPLS == nil {
			return errors.New("tunnels: MPLS requires an mpls config")
		}
		if err := t.MPLS.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// IPSecConfig is the IPsec/IKEv2 configuration.
// The runtime uses modern crypto defaults;
// the model surfaces only the operator
// knobs the userland needs.
type IPSecConfig struct {
	// IKEVersion is 1 (IKEv1) or 2 (IKEv2).
	IKEVersion int `json:"ikeVersion"`
	// AuthMethod is "psk" or "cert".
	AuthMethod string `json:"authMethod"`
	// PSKRef / CertRef are vault references.
	PSKRef  string `json:"pskRef,omitempty"`
	CertRef string `json:"certRef,omitempty"`
	// RemoteCandidates is the list of
	// candidate peer addresses; the
	// runtime picks the first reachable.
	RemoteCandidates []string `json:"remoteCandidates,omitempty"`
	// LocalNetworks / RemoteNetworks are
	// the protected selectors.
	LocalNetworks  []string `json:"localNetworks,omitempty"`
	RemoteNetworks []string `json:"remoteNetworks,omitempty"`
	// Proposals is a list of proposal
	// strings. The runtime is responsible
	// for modern defaults; the operator
	// can pin to a specific proposal only
	// when a peer demands it.
	Proposals []string `json:"proposals,omitempty"`
}

// Valid reports whether c is well-formed.
func (c IPSecConfig) Valid() error {
	if c.IKEVersion != 1 && c.IKEVersion != 2 {
		return errors.New("tunnels: IKE version must be 1 or 2")
	}
	if c.AuthMethod != "psk" && c.AuthMethod != "cert" {
		return errors.New("tunnels: authMethod must be psk or cert")
	}
	if c.AuthMethod == "psk" && c.PSKRef == "" {
		return errors.New("tunnels: PSK auth requires a PSK reference")
	}
	if c.AuthMethod == "cert" && c.CertRef == "" {
		return errors.New("tunnels: cert auth requires a cert reference")
	}
	if len(c.LocalNetworks) == 0 || len(c.RemoteNetworks) == 0 {
		return errors.New("tunnels: IPsec requires local and remote networks")
	}
	return nil
}

// VXLANConfig is the VXLAN overlay
// configuration.
type VXLANConfig struct {
	VNI      int    `json:"vni"`
	Local    string `json:"local"`
	Group    string `json:"group,omitempty"`
	Remote   string `json:"remote,omitempty"`
	Dev      string `json:"dev"`
	Port     int    `json:"port"`
	Bridge   string `json:"bridge,omitempty"`
	Learning bool   `json:"learning"`
	// EVPNEnabled toggles EVPN integration.
	EVPNEnabled bool `json:"evpnEnabled"`
	// EVPNVNI is the EVPN-specific VNI when
	// EVPNEnabled is true. The V1 contract
	// is that the runtime wires the EVPN
	// control plane when this is set; a
	// placeholder value reports the
	// capability.
	EVPNVNI int `json:"evpnVni,omitempty"`
}

// Valid reports whether c is well-formed.
func (c VXLANConfig) Valid() error {
	if c.VNI < 1 || c.VNI > 16777215 {
		return errors.New("tunnels: VNI must be 1..16777215")
	}
	if c.Local == "" {
		return errors.New("tunnels: local is required")
	}
	if c.Group == "" && c.Remote == "" {
		return errors.New("tunnels: VXLAN needs a group or a remote")
	}
	if c.Dev == "" {
		return errors.New("tunnels: dev is required")
	}
	if c.Port < 0 || c.Port > 65535 {
		return errors.New("tunnels: port out of range")
	}
	if c.EVPNEnabled && c.EVPNVNI == 0 {
		return errors.New("tunnels: EVPNEnabled requires EVPNVNI")
	}
	return nil
}

// MPLSConfig is the bounded MPLS/L3VPN
// configuration. The V1 contract is the
// model; the kernel/userspace dataplane
// capability is detected at runtime and
// reported as a capability flag.
type MPLSConfig struct {
	// LabelMin / LabelMax bound the dynamic
	// label range. Static labels are not in
	// the V1 model.
	LabelMin int `json:"labelMin"`
	LabelMax int `json:"labelMax"`
	// L3VPNVRFs is the list of VRF names
	// that participate in L3VPN. The
	// runtime detects whether the kernel
	// supports per-VRF label forwarding.
	L3VPNVRFs []string `json:"l3vpnVrfs,omitempty"`
}

// Valid reports whether c is well-formed.
func (c MPLSConfig) Valid() error {
	if c.LabelMin < 16 || c.LabelMin > 1048575 {
		return errors.New("tunnels: labelMin out of range")
	}
	if c.LabelMax < c.LabelMin || c.LabelMax > 1048575 {
		return errors.New("tunnels: labelMax must be >= labelMin")
	}
	return nil
}

// Manager is the V1 tunnel store. Manager is
// goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	tunnels map[string]Tunnel
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{tunnels: map[string]Tunnel{}}
}

// Set replaces or inserts a tunnel.
func (m *Manager) Set(t Tunnel) error {
	if err := t.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tunnels[t.ID] = t
	return nil
}

// Get returns a tunnel by id.
func (m *Manager) Get(id string) (Tunnel, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tunnels[id]
	return t, ok
}

// List returns the tunnels sorted by id.
func (m *Manager) List() []Tunnel {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a tunnel.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tunnels, id)
}

// HasCapability reports whether the manager
// has at least one tunnel of the given kind.
// The future UI uses this to show or hide
// overlay features.
func (m *Manager) HasCapability(k Kind) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tunnels {
		if t.Kind == k {
			return true
		}
	}
	return false
}
