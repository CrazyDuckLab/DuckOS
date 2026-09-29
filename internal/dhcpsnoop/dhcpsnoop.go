// dhcpsnoop/dhcpsnoop.go: V1 DHCP snooping
// + DAI + IPv6 RA Guard model. The package
// owns the typed contracts for the trusted
// and untrusted port classification, the
// lease-to-port bindings, the DAI table, and
// the rogue-RA detection shape.
//
// The runtime is a thin mapping to nftables /
// bridge nf_tables sets and the kernel FDB;
// the model is what the future UI and the
// DHCP server consult.
package dhcpsnoop

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// TrustClass is the per-port trust level.
type TrustClass string

const (
	TrustTrusted    TrustClass = "trusted"
	TrustUntrusted  TrustClass = "untrusted"
	TrustDHCPClient TrustClass = "dhcp-client"
)

// Valid reports whether t is known.
func (t TrustClass) Valid() bool {
	switch t {
	case TrustTrusted, TrustUntrusted, TrustDHCPClient:
		return true
	}
	return false
}

// PortClass is the per-port classification.
type PortClass struct {
	Bridge string     `json:"bridge"`
	Link   string     `json:"link"`
	Class  TrustClass `json:"class"`
}

var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether p is well-formed.
func (p PortClass) Valid() error {
	if !linkPattern.MatchString(p.Bridge) {
		return errors.New("dhcpsnoop: bridge is not canonical")
	}
	if !linkPattern.MatchString(p.Link) {
		return errors.New("dhcpsnoop: link is not canonical")
	}
	if !p.Class.Valid() {
		return errors.New("dhcpsnoop: class is unknown")
	}
	return nil
}

// Lease is a DHCP-observed binding. The agent
// uses the binding to detect ARP-spoofing and
// rogue DHCP servers.
type Lease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Bridge   string `json:"bridge"`
	Link     string `json:"link"`
	VLAN     int    `json:"vlan"`
	Expiry   int64  `json:"expiry"`
	Trusted  bool   `json:"trusted"`
	Observed int64  `json:"observed"`
}

// Valid reports whether l is well-formed.
func (l Lease) Valid() error {
	if l.MAC == "" || l.IP == "" {
		return errors.New("dhcpsnoop: lease must have mac and ip")
	}
	if !linkPattern.MatchString(l.Link) {
		return errors.New("dhcpsnoop: link is not canonical")
	}
	return nil
}

// ARPBinding is the DAI-validated entry.
type ARPBinding struct {
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
	Bridge string `json:"bridge"`
	Link   string `json:"link"`
	VLAN   int    `json:"vlan"`
}

// Valid reports whether a is well-formed.
func (a ARPBinding) Valid() error {
	if a.MAC == "" || a.IP == "" {
		return errors.New("dhcpsnoop: ARP binding must have mac and ip")
	}
	return nil
}

// RAObservation is one observed IPv6 RA.
type RAObservation struct {
	Source    string `json:"source"` // link-local source
	Prefix    string `json:"prefix"` // announced prefix
	Link      string `json:"link"`
	At        int64  `json:"at"`
	Flags     string `json:"flags"`
	Trusted   bool   `json:"trusted"`
	RogueFlag bool   `json:"rogueFlag"`
}

// Manager is the V1 DHCP-snooping store.
// Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	ports    map[string]PortClass       // bridge/link
	leases   map[string]Lease           // mac
	bindings map[string]ARPBinding      // ip
	ras      map[string][]RAObservation // link
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{ports: map[string]PortClass{}, leases: map[string]Lease{}, bindings: map[string]ARPBinding{}, ras: map[string][]RAObservation{}}
}

// SetPort replaces or inserts a port class.
func (m *Manager) SetPort(p PortClass) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ports[p.Bridge+"/"+p.Link] = p
	return nil
}

// SetLease replaces or inserts a lease.
func (m *Manager) SetLease(l Lease) error {
	if err := l.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leases[l.MAC] = l
	m.bindings[l.IP] = ARPBinding{MAC: l.MAC, IP: l.IP, Bridge: l.Bridge, Link: l.Link, VLAN: l.VLAN}
	return nil
}

// ClassOf returns the trust class of a port.
func (m *Manager) ClassOf(bridge, link string) (TrustClass, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.ports[bridge+"/"+link]
	return p.Class, ok
}

// LeaseForMAC returns the lease for a MAC.
func (m *Manager) LeaseForMAC(mac string) (Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.leases[mac]
	return l, ok
}

// BindingForIP returns the DAI binding for an IP.
func (m *Manager) BindingForIP(ip string) (ARPBinding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.bindings[ip]
	return b, ok
}

// RecordRA stores a router-advertisement
// observation. RAs from a non-trusted link are
// flagged as rogue.
func (m *Manager) RecordRA(bridge, link string, ra RAObservation) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cls, ok := m.ports[bridge+"/"+link]
	if !ok {
		ra.Trusted = false
	} else {
		ra.Trusted = cls.Class == TrustTrusted
	}
	ra.RogueFlag = !ra.Trusted
	m.ras[link] = append(m.ras[link], ra)
}

// RogueRAs returns the RAs that were flagged
// as rogue, sorted by At (newest first).
func (m *Manager) RogueRAs() []RAObservation {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RAObservation
	for _, list := range m.ras {
		for _, ra := range list {
			if ra.RogueFlag {
				out = append(out, ra)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	return out
}
