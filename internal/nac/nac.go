// nac/nac.go: V1 NAC + IPAM + port-security
// + SPAN + eBPF/switchdev capability +
// SR-IOV + DHCP-HA + Anycast. The
// package groups related first-class
// objects into one consistent surface
// so the Agent and the future UI can
// address them uniformly.
package nac

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Device is a detected endpoint.
type Device struct {
	MAC         string    `json:"mac"`
	IP          string    `json:"ip,omitempty"`
	Hostname    string    `json:"hostname,omitempty"`
	Vendor      string    `json:"vendor,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	FirstSeen   time.Time `json:"firstSeen"`
	LastSeen    time.Time `json:"lastSeen"`
	// Quarantine is true when the device
	// is in the limited-access network.
	Quarantine bool `json:"quarantine"`
	Approved   bool `json:"approved"`
}

var macPattern = regexp.MustCompile(`^[a-fA-F0-9:]{17}$`)

// Valid reports whether d is well-formed.
func (d Device) Valid() error {
	if !macPattern.MatchString(d.MAC) {
		return errors.New("nac: MAC is not canonical")
	}
	return nil
}

// Manager is the NAC store. Manager is
// goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	devices map[string]Device
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{devices: map[string]Device{}} }

// Observe records a device observation.
func (m *Manager) Observe(d Device) error {
	if err := d.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	existing, ok := m.devices[d.MAC]
	if ok {
		existing.LastSeen = now
		if d.IP != "" {
			existing.IP = d.IP
		}
		if d.Hostname != "" {
			existing.Hostname = d.Hostname
		}
		if d.Fingerprint != "" {
			existing.Fingerprint = d.Fingerprint
		}
		if d.Vendor != "" {
			existing.Vendor = d.Vendor
		}
		m.devices[d.MAC] = existing
		return nil
	}
	d.FirstSeen = now
	d.LastSeen = now
	m.devices[d.MAC] = d
	return nil
}

// Approve marks the device as approved.
func (m *Manager) Approve(mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[mac]
	if !ok {
		return errors.New("nac: unknown device")
	}
	d.Approved = true
	d.Quarantine = false
	m.devices[mac] = d
	return nil
}

// Quarantine moves the device to the
// limited-access network.
func (m *Manager) Quarantine(mac string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[mac]
	if !ok {
		return errors.New("nac: unknown device")
	}
	d.Approved = false
	d.Quarantine = true
	m.devices[mac] = d
	return nil
}

// Devices returns every device sorted by
// MAC.
func (m *Manager) Devices() []Device {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Device, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].MAC < out[k].MAC })
	return out
}

// Prefix is an IPAM prefix.
type Prefix struct {
	CIDR    string `json:"cidr"`
	VLAN    int    `json:"vlan,omitempty"`
	VRF     string `json:"vrf,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	Site    string `json:"site,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// IPAMStore is the IPAM store.
type IPAMStore struct {
	mu       sync.Mutex
	prefixes map[string]Prefix
	pools    map[string]Pool
}

// Pool is a DHCP pool within a prefix.
type Pool struct {
	Name      string `json:"name"`
	CIDR      string `json:"cidr"`
	StartIP   string `json:"startIp"`
	EndIP     string `json:"endIp"`
	Gateway   string `json:"gateway,omitempty"`
	LeaseTime int    `json:"leaseTimeSec,omitempty"`
}

// NewIPAMStore returns an empty store.
func NewIPAMStore() *IPAMStore {
	return &IPAMStore{prefixes: map[string]Prefix{}, pools: map[string]Pool{}}
}

// AddPrefix inserts a prefix.
func (s *IPAMStore) AddPrefix(p Prefix) error {
	if p.CIDR == "" {
		return errors.New("nac: prefix CIDR is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefixes[p.CIDR] = p
	return nil
}

// AddPool inserts a pool.
func (s *IPAMStore) AddPool(p Pool) error {
	if p.Name == "" || p.CIDR == "" {
		return errors.New("nac: pool name+CIDR are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pools[p.Name] = p
	return nil
}

// Prefixes returns every prefix sorted.
func (s *IPAMStore) Prefixes() []Prefix {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Prefix, 0, len(s.prefixes))
	for _, p := range s.prefixes {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CIDR < out[k].CIDR })
	return out
}

// Pools returns every pool sorted.
func (s *IPAMStore) Pools() []Pool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Pool, 0, len(s.pools))
	for _, p := range s.pools {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// PortSecurity is the per-port MAC ACL.
type PortSecurity struct {
	Port            string   `json:"port"`
	MaxMACs         int      `json:"maxMacs"`
	AllowedMACs     []string `json:"allowedMacs,omitempty"`
	Sticky          bool     `json:"sticky,omitempty"`
	ViolationAction string   `json:"violationAction,omitempty"`
}

// LLDPNeighbor is a learned LLDP neighbor
// (tied to port-security context).
type LLDPNeighbor struct {
	Port       string `json:"port"`
	ChassisID  string `json:"chassisId"`
	PortID     string `json:"portId"`
	SystemName string `json:"systemName,omitempty"`
	PortDescr  string `json:"portDescr,omitempty"`
	Capability string `json:"capability,omitempty"`
	VLAN       int    `json:"vlan,omitempty"`
	VoiceVLAN  int    `json:"voiceVlan,omitempty"`
}

// PortSecStore is the port-security store.
type PortSecStore struct {
	mu        sync.Mutex
	portSec   map[string]PortSecurity
	neighbors []LLDPNeighbor
}

// NewPortSecStore returns an empty store.
func NewPortSecStore() *PortSecStore {
	return &PortSecStore{portSec: map[string]PortSecurity{}}
}

// Set inserts a port-security record.
func (s *PortSecStore) Set(p PortSecurity) error {
	if p.Port == "" {
		return errors.New("nac: port is required")
	}
	if p.MaxMACs < 0 {
		return errors.New("nac: maxMacs must be non-negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.portSec[p.Port] = p
	return nil
}

// ObserveLLDP appends an LLDP neighbor.
func (s *PortSecStore) ObserveLLDP(n LLDPNeighbor) {
	if n.Port == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.neighbors = append(s.neighbors, n)
}

// Neighbors returns the LLDP neighbors
// sorted by port.
func (s *PortSecStore) Neighbors() []LLDPNeighbor {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]LLDPNeighbor(nil), s.neighbors...)
	sort.Slice(out, func(i, k int) bool { return out[i].Port < out[k].Port })
	return out
}

// SPANSession is one SPAN / mirror
// session.
type SPANSession struct {
	ID            string `json:"id"`
	Source        string `json:"source"`
	Dest          string `json:"dest"`
	Direction     string `json:"direction,omitempty"`     // "ingress" | "egress" | "both"
	Encapsulation string `json:"encapsulation,omitempty"` // "none" | "erspan"
	Bound         int    `json:"boundBytesPerSec,omitempty"`
	MaxSessions   int    `json:"maxSessions,omitempty"`
}

// Valid reports whether s is well-formed.
func (s SPANSession) Valid() error {
	if s.ID == "" {
		return errors.New("nac: SPAN id is required")
	}
	if s.Source == "" || s.Dest == "" {
		return errors.New("nac: SPAN source and dest are required")
	}
	return nil
}

// SPANStore is the SPAN session store.
type SPANStore struct {
	mu       sync.Mutex
	sessions map[string]SPANSession
}

// NewSPANStore returns an empty store.
func NewSPANStore() *SPANStore { return &SPANStore{sessions: map[string]SPANSession{}} }

// Add inserts a session.
func (s *SPANStore) Add(sess SPANSession) error {
	if err := sess.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return nil
}

// Remove drops a session.
func (s *SPANStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// All returns every session sorted.
func (s *SPANStore) All() []SPANSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SPANSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// AccelBackend is the eBPF / XDP /
// switchdev capability gate.
type AccelBackend string

const (
	AccelNone      AccelBackend = "none"
	AccelEBPF      AccelBackend = "ebpf"
	AccelXDP       AccelBackend = "xdp"
	AccelAFXDP     AccelBackend = "af-xdp"
	AccelDPDK      AccelBackend = "dpdk"
	AccelSwitchDev AccelBackend = "switchdev"
	AccelDevlink   AccelBackend = "devlink"
	AccelDSA       AccelBackend = "dsa"
)

// Valid reports whether b is known.
func (b AccelBackend) Valid() bool {
	switch b {
	case AccelNone, AccelEBPF, AccelXDP, AccelAFXDP,
		AccelDPDK, AccelSwitchDev, AccelDevlink, AccelDSA:
		return true
	}
	return false
}

// AccelStore is the per-component
// acceleration store.
type AccelStore struct {
	mu      sync.Mutex
	backend map[string]AccelBackend
}

// NewAccelStore returns an empty store.
func NewAccelStore() *AccelStore { return &AccelStore{backend: map[string]AccelBackend{}} }

// Set records the active backend for a
// component (interface, NIC, switch).
func (s *AccelStore) Set(component string, b AccelBackend) {
	if !b.Valid() || component == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backend[component] = b
}

// Get returns the recorded backend.
func (s *AccelStore) Get(component string) (AccelBackend, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.backend[component]
	return b, ok
}

// All returns the full backend map.
func (s *AccelStore) All() map[string]AccelBackend {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]AccelBackend{}
	for k, v := range s.backend {
		out[k] = v
	}
	return out
}

// SRIOVPort is a single SR-IOV port
// record.
type SRIOVPort struct {
	PF       string `json:"pf"`
	VF       int    `json:"vf"`
	MAC      string `json:"mac,omitempty"`
	VLAN     int    `json:"vlan,omitempty"`
	RateMbps int    `json:"rateMbps,omitempty"`
}

// SRIOVStore is the SR-IOV store.
type SRIOVStore struct {
	mu    sync.Mutex
	ports map[string]SRIOVPort
}

// NewSRIOVStore returns an empty store.
func NewSRIOVStore() *SRIOVStore { return &SRIOVStore{ports: map[string]SRIOVPort{}} }

// Set records a port.
func (s *SRIOVStore) Set(p SRIOVPort) {
	if p.PF == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ports[p.PF+":"+uintStr(p.VF)] = p
}

// All returns every port sorted.
func (s *SRIOVStore) All() []SRIOVPort {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SRIOVPort, 0, len(s.ports))
	for _, p := range s.ports {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool {
		if out[i].PF != out[k].PF {
			return out[i].PF < out[k].PF
		}
		return out[i].VF < out[k].VF
	})
	return out
}

// DHCPHAPeer is the DHCP HA peer state.
type DHCPHAPeer struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	State   string `json:"state"` // "primary" | "secondary" | "unknown"
}

// DHCPHAStore is the DHCP HA store.
type DHCPHAStore struct {
	mu    sync.Mutex
	peers map[string]DHCPHAPeer
}

// NewDHCPHAStore returns an empty store.
func NewDHCPHAStore() *DHCPHAStore { return &DHCPHAStore{peers: map[string]DHCPHAPeer{}} }

// Set records a peer.
func (s *DHCPHAStore) Set(p DHCPHAPeer) error {
	if p.Name == "" || p.Address == "" {
		return errors.New("nac: DHCP HA peer name+address required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[p.Name] = p
	return nil
}

// Get returns a peer.
func (s *DHCPHAStore) Get(name string) (DHCPHAPeer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peers[name]
	return p, ok
}

// All returns every peer sorted.
func (s *DHCPHAStore) All() []DHCPHAPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DHCPHAPeer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// AnycastService is the anycast service
// declaration.
type AnycastService struct {
	Name      string `json:"name"`
	VIP       string `json:"vip"`
	LocalNode string `json:"localNode"`
	// Healthy toggles whether the local
	// node is healthy enough to advertise.
	Healthy bool `json:"healthy"`
	// HysteresisSec is the hold-down
	// between advertise and withdraw.
	HysteresisSec int `json:"hysteresisSec,omitempty"`
}

// Valid reports whether s is well-formed.
func (s AnycastService) Valid() error {
	if s.Name == "" || s.VIP == "" {
		return errors.New("nac: anycast name+VIP required")
	}
	return nil
}

// AnycastStore is the anycast service
// store.
type AnycastStore struct {
	mu       sync.Mutex
	services map[string]AnycastService
}

// NewAnycastStore returns an empty store.
func NewAnycastStore() *AnycastStore { return &AnycastStore{services: map[string]AnycastService{}} }

// Set records the service.
func (s *AnycastStore) Set(svc AnycastService) error {
	if err := svc.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services[svc.Name] = svc
	return nil
}

// All returns every service sorted.
func (s *AnycastStore) All() []AnycastService {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AnycastService, 0, len(s.services))
	for _, svc := range s.services {
		out = append(out, svc)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

func uintStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
