// ipv6/ipv6.go: V1 IPv6 parity model. The
// package owns the typed contracts for
// SLAAC, Router-Advertisement, DHCPv6,
// Prefix Delegation, and the NAT64/DNS64
// discovery helpers. The runtime is a thin
// mapping to `ip -6 addr`, `ip -6 route`,
// and the radvd/dhclient userland.
//
// The V1 contract is: every place the V1
// codebase references an IPv4 address, it
// also accepts an IPv6 form; no access-policy
// or tunnel leaks through IPv6.
package ipv6

import (
	"encoding/binary"
	"errors"
	"regexp"
	"sort"
	"sync"
)

// AddressMode is the per-interface IPv6
// address acquisition mode.
type AddressMode string

const (
	ModeSLAAC   AddressMode = "slaac"
	ModeDHCPv6  AddressMode = "dhcpv6"
	ModeStatic  AddressMode = "static"
	ModePDChild AddressMode = "pd-child"
)

// Valid reports whether m is known.
func (m AddressMode) Valid() bool {
	switch m {
	case ModeSLAAC, ModeDHCPv6, ModeStatic, ModePDChild:
		return true
	}
	return false
}

// PDState is the Prefix Delegation state.
type PDState string

const (
	PDIdle     PDState = "idle"
	PDRequest  PDState = "requesting"
	PDBound    PDState = "bound"
	PDExpired  PDState = "expired"
	PDRenewing PDState = "renewing"
)

// Valid reports whether s is known.
func (s PDState) Valid() bool {
	switch s {
	case PDIdle, PDRequest, PDBound, PDExpired, PDRenewing:
		return true
	}
	return false
}

// Assignment is one delegated prefix assignment.
type Assignment struct {
	Interface      string  `json:"interface"`
	Prefix         string  `json:"prefix"`
	PrefixLen      int     `json:"prefixLen"`
	State          PDState `json:"state"`
	ValidUntil     int64   `json:"validUntil"`
	PreferredUntil int64   `json:"preferredUntil"`
}

// Valid reports whether a is well-formed.
func (a Assignment) Valid() error {
	if a.Interface == "" {
		return errors.New("ipv6: interface is required")
	}
	if a.Prefix == "" {
		return errors.New("ipv6: prefix is required")
	}
	if a.PrefixLen < 0 || a.PrefixLen > 128 {
		return errors.New("ipv6: prefix length out of range")
	}
	if !a.State.Valid() {
		return errors.New("ipv6: state is unknown")
	}
	return nil
}

// PDConfig is the per-interface Prefix
// Delegation configuration.
type PDConfig struct {
	Interface  string `json:"interface"`
	UpstreamIf string `json:"upstreamIf"`
	// IAID is the identity-association id
	// for the upstream PD request.
	IAID uint32 `json:"iaid"`
	// PrefixHint is an optional hint sent to
	// the upstream; many ISPs ignore it.
	PrefixHint string `json:"prefixHint,omitempty"`
	// State is the current state of the
	// upstream negotiation.
	State PDState `json:"state"`
	// Assignments is the set of downstream
	// prefix assignments derived from the
	// upstream.
	Assignments []Assignment `json:"assignments"`
}

var ifacePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether c is well-formed.
func (c PDConfig) Valid() error {
	if !ifacePattern.MatchString(c.Interface) {
		return errors.New("ipv6: interface is not canonical")
	}
	if c.UpstreamIf != "" && !ifacePattern.MatchString(c.UpstreamIf) {
		return errors.New("ipv6: upstreamIf is not canonical")
	}
	if !c.State.Valid() {
		return errors.New("ipv6: state is unknown")
	}
	for _, a := range c.Assignments {
		if err := a.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// PrefixFormat is the contract for a delegated
// prefix string. The format is "2001:db8::/64".
// ValidatePrefix reports whether the format is
// canonical and the prefix length is in [0,128].
func ValidatePrefix(p string) error {
	slash := -1
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			slash = i
			break
		}
	}
	if slash < 0 {
		return errors.New("ipv6: prefix must contain /N")
	}
	prefix := p[:slash]
	plen := 0
	for _, c := range p[slash+1:] {
		if c < '0' || c > '9' {
			return errors.New("ipv6: prefix length must be numeric")
		}
		plen = plen*10 + int(c-'0')
	}
	if plen < 0 || plen > 128 {
		return errors.New("ipv6: prefix length out of range")
	}
	if _, err := parseIPv6(prefix); err != nil {
		return err
	}
	return nil
}

// parseIPv6 parses a canonical IPv6 address.
// It is a small subset of net.ParseIP that
// handles zero-compressed addresses; the
// real parsing lives in the runtime.
func parseIPv6(s string) ([]byte, error) {
	// Reject obvious bad forms. The "::" form
	// is the unspecified address (all zeros)
	// and is acceptable as a wildcard.
	if len(s) == 0 {
		return nil, errors.New("ipv6: address is empty")
	}
	if s == "::" {
		return make([]byte, 16), nil
	}
	// Compressed form: detect "::" at most once.
	double := -1
	for i := 0; i+1 < len(s); i++ {
		if s[i] == ':' && s[i+1] == ':' {
			if double >= 0 {
				return nil, errors.New("ipv6: multiple :: in address")
			}
			double = i
		}
	}
	groups := splitGroups(s)
	want := 8
	if double >= 0 {
		want = 8 - len(groups) + 1
		if want < 0 {
			return nil, errors.New("ipv6: too many groups")
		}
	}
	if double < 0 && len(groups) != 8 {
		return nil, errors.New("ipv6: address must have 8 groups (or be :: compressed)")
	}
	out := make([]byte, 16)
	pos := 0
	for _, g := range groups {
		if g == "" {
			continue
		}
		if len(g) > 4 {
			return nil, errors.New("ipv6: group exceeds 4 hex digits")
		}
		var n uint16
		for _, c := range g {
			var d byte
			switch {
			case c >= '0' && c <= '9':
				d = byte(c - '0')
			case c >= 'a' && c <= 'f':
				d = byte(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = byte(c-'A') + 10
			default:
				return nil, errors.New("ipv6: invalid hex digit")
			}
			n = n<<4 | uint16(d)
		}
		binary.BigEndian.PutUint16(out[pos:], n)
		pos += 2
	}
	if double >= 0 {
		pos = 16 - (len(groups)-(stringsCount(s, "::")*8-(8-len(groups))+1))*2
	}
	_ = pos
	return out, nil
}

func splitGroups(s string) []string {
	out := []string{}
	cur := ""
	for _, c := range s {
		if c == ':' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	if cur != "" || s[len(s)-1] == ':' {
		out = append(out, cur)
	}
	return out
}

func stringsCount(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

// NAT64Config is the V1 NAT64/DNS64 model.
type NAT64Config struct {
	Enabled    bool   `json:"enabled"`
	Prefix     string `json:"prefix"`
	JoolBinary string `json:"joolBinary,omitempty"`
	// TAYv4 is the IPv4 address pool.
	TAYv4 string `json:"tayV4,omitempty"`
}

// Valid reports whether c is well-formed.
func (c NAT64Config) Valid() error {
	if !c.Enabled {
		return nil
	}
	if c.Prefix == "" {
		return errors.New("ipv6: NAT64 prefix is required when enabled")
	}
	if err := ValidatePrefix(c.Prefix); err != nil {
		return err
	}
	return nil
}

// DNS64Config is the V1 DNS64 model.
type DNS64Config struct {
	Enabled  bool   `json:"enabled"`
	Prefix   string `json:"prefix"`
	Upstream string `json:"upstream,omitempty"`
}

// Valid reports whether c is well-formed.
func (c DNS64Config) Valid() error {
	if !c.Enabled {
		return nil
	}
	if c.Prefix == "" {
		return errors.New("ipv6: DNS64 prefix is required when enabled")
	}
	if err := ValidatePrefix(c.Prefix); err != nil {
		return err
	}
	return nil
}

// Manager is the V1 IPv6 store. Manager is
// goroutine-safe.
type Manager struct {
	mu    sync.Mutex
	pd    map[string]PDConfig // keyed by interface
	nat64 NAT64Config
	dns64 DNS64Config
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{pd: map[string]PDConfig{}}
}

// SetPD replaces or inserts the PD config for
// an interface.
func (m *Manager) SetPD(c PDConfig) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pd[c.Interface] = c
	return nil
}

// PD returns the PD config for an interface.
func (m *Manager) PD(iface string) (PDConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.pd[iface]
	return c, ok
}

// ListPD returns every PD config sorted by
// interface.
func (m *Manager) ListPD() []PDConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PDConfig, 0, len(m.pd))
	for _, c := range m.pd {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface < out[j].Interface })
	return out
}

// SetNAT64 replaces the NAT64 config.
func (m *Manager) SetNAT64(c NAT64Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	m.nat64 = c
	m.mu.Unlock()
	return nil
}

// NAT64 returns the NAT64 config.
func (m *Manager) NAT64() NAT64Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nat64
}

// SetDNS64 replaces the DNS64 config.
func (m *Manager) SetDNS64(c DNS64Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	m.dns64 = c
	m.mu.Unlock()
	return nil
}

// DNS64 returns the DNS64 config.
func (m *Manager) DNS64() DNS64Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dns64
}
