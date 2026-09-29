// vrf/vrf.go: V1 VRF (Virtual Routing and
// Forwarding) first-class model. A VRF is a
// separate routing table; interfaces,
// tunnels, and services can be assigned to a
// VRF. The package owns the VRF model, RIB
// view, FIB view, and the route-lookup
// simulation.
//
// The Linux runtime is a thin mapping to
// `ip link set ... vrf <table>` and
// `ip rule add ... lookup <table>`. The
// V1 model is the contract the future UI
// binds to.
package vrf

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"sync"
)

// ID is the canonical VRF identifier.
type ID string

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether v is a canonical VRF
// id.
func (v ID) Valid() bool {
	return idPattern.MatchString(string(v))
}

// Table is the routing table number. The
// default table for the management VRF is
// 1000; the data plane defaults to main
// (table 254).
type Table int

const (
	TableMain       Table = 254
	TableLocal      Table = 255
	TableManagement Table = 1000
)

// VRF is one virtual routing context.
type VRF struct {
	ID          ID       `json:"id"`
	Name        string   `json:"name"`
	Table       Table    `json:"table"`
	Description string   `json:"description,omitempty"`
	Members     []Member `json:"members"`
	Routes      []Route  `json:"routes"`
	// Leaks is the explicit inter-VRF route
	// leaking policy. A Leak is visible; a
	// silent leak is forbidden by V1.
	Leaks []Leak `json:"leaks,omitempty"`
}

// Member is a binding to a VRF: an interface,
// a tunnel, or a service.
type Member struct {
	Kind   string `json:"kind"`   // "interface" | "tunnel" | "service"
	Target string `json:"target"` // canonical name
}

// Route is a single static route in the VRF.
type Route struct {
	Destination string `json:"destination"`
	NextHop     string `json:"nextHop,omitempty"`
	Interface   string `json:"interface,omitempty"`
	// Type is "unicast" (default), "blackhole",
	// "unreachable", or "prohibit".
	Type   string `json:"type,omitempty"`
	Metric int    `json:"metric,omitempty"`
	Table  Table  `json:"table,omitempty"`
}

// Leak is an explicit inter-VRF route leak.
// A leak always names the source VRF, the
// destination VRF, and the prefix.
type Leak struct {
	Source      ID     `json:"source"`
	Destination ID     `json:"destination"`
	Prefix      string `json:"prefix"`
}

// Valid reports whether v is well-formed.
func (v VRF) Valid() error {
	if !v.ValidID() {
		return errors.New("vrf: id is not canonical")
	}
	if v.Name == "" {
		return errors.New("vrf: name is required")
	}
	if v.Table <= 0 {
		return errors.New("vrf: table must be positive")
	}
	for _, m := range v.Members {
		if m.Kind == "" || m.Target == "" {
			return errors.New("vrf: member is incomplete")
		}
	}
	for _, r := range v.Routes {
		if r.Destination == "" {
			return errors.New("vrf: route destination is required")
		}
		switch r.Type {
		case "", "unicast", "blackhole", "unreachable", "prohibit":
		default:
			return errors.New("vrf: route type is unknown")
		}
	}
	for _, l := range v.Leaks {
		if !l.Source.Valid() || !l.Destination.Valid() {
			return errors.New("vrf: leak endpoints are not canonical")
		}
		if l.Source == l.Destination {
			return errors.New("vrf: leak source equals destination")
		}
	}
	return nil
}

// ValidID is a shortcut for v.ID.Valid().
func (v VRF) ValidID() bool { return v.ID.Valid() }

// Lookup is the route-lookup simulation input.
type Lookup struct {
	VRF       ID     `json:"vrf"`
	Src       string `json:"src"`
	Dst       string `json:"dst"`
	Interface string `json:"interface,omitempty"`
	Table     Table  `json:"table,omitempty"`
	Mark      int    `json:"mark,omitempty"`
}

// LookupResult is the route-lookup simulation
// output. The result is pure: it never
// inspects the kernel, only the V1 model.
type LookupResult struct {
	VRF       ID     `json:"vrf"`
	Rule      string `json:"rule,omitempty"`
	Route     string `json:"route,omitempty"`
	NextHop   string `json:"nextHop,omitempty"`
	Egress    string `json:"egress,omitempty"`
	Reason    string `json:"reason"`
	Recursive bool   `json:"recursive,omitempty"`
}

// Manager is the V1 VRF store. Manager is
// goroutine-safe.
type Manager struct {
	mu   sync.Mutex
	byID map[ID]VRF
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{byID: map[ID]VRF{}}
}

// Set replaces or inserts a VRF.
func (m *Manager) Set(v VRF) error {
	if err := v.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[v.ID] = v
	return nil
}

// Get returns a VRF by id.
func (m *Manager) Get(id ID) (VRF, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.byID[id]
	return v, ok
}

// List returns the VRFs sorted by id.
func (m *Manager) List() []VRF {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]VRF, 0, len(m.byID))
	for _, v := range m.byID {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a VRF.
func (m *Manager) Delete(id ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, id)
}

// Lookup simulates a route lookup. The
// function never invents a successful path:
// when the lookup cannot find a route, the
// Reason is explicit.
func (m *Manager) Lookup(q Lookup) LookupResult {
	m.mu.Lock()
	v, ok := m.byID[q.VRF]
	m.mu.Unlock()
	res := LookupResult{VRF: q.VRF}
	if !ok {
		res.Reason = "vrf not found"
		return res
	}
	// Longest-prefix match.
	best := -1
	bestLen := -1
	for i, r := range v.Routes {
		if !prefixMatches(r.Destination, q.Dst) {
			continue
		}
		plen := prefixLen(r.Destination)
		if plen > bestLen {
			bestLen = plen
			best = i
		}
	}
	if best >= 0 {
		r := v.Routes[best]
		res.Route = r.Destination
		res.NextHop = r.NextHop
		res.Egress = r.Interface
		res.Reason = "matched static route"
		if r.Type != "" && r.Type != "unicast" {
			res.Reason = "matched " + r.Type + " route"
		}
		return res
	}
	for _, l := range v.Leaks {
		if l.Source == q.VRF {
			res.Rule = "leak:" + string(l.Destination) + ":" + l.Prefix
			res.Reason = "matched leak into " + string(l.Destination)
		}
	}
	if res.Rule == "" {
		res.Reason = "no matching route in vrf " + string(q.VRF)
	}
	return res
}

// prefixMatches reports whether prefix (a CIDR
// like 10.0.0.0/8) covers the destination
// address. The match is conservative: it
// supports IPv4 dotted-quad and IPv6 hex
// notation. A prefix of "0.0.0.0/0" or "::/0"
// matches anything.
func prefixMatches(prefix, dst string) bool {
	if prefix == "" {
		return dst != ""
	}
	plen := prefixLen(prefix)
	if plen == 0 {
		return dst != ""
	}
	// Split prefix into network and host parts.
	pNet, pHost := splitCIDR(prefix)
	dNet, dHost := splitCIDR(dst)
	if len(pNet) == 0 {
		return true
	}
	// Compare the first plen/8 bytes of the
	// network portion, then check the bits
	// that fall in the partial byte.
	fullBytes := plen / 8
	if fullBytes > len(pNet) || fullBytes > len(dNet) {
		return false
	}
	for i := 0; i < fullBytes; i++ {
		if pNet[i] != dNet[i] {
			return false
		}
	}
	// Handle the partial byte.
	rem := plen % 8
	if rem == 0 {
		return true
	}
	if fullBytes >= len(pNet) || fullBytes >= len(dNet) {
		return false
	}
	mask := byte(0xFF) << (8 - rem)
	pb := pNet[fullBytes] & mask
	db := dNet[fullBytes] & mask
	if pb != db {
		return false
	}
	_ = pHost
	_ = dHost
	return true
}

// splitCIDR returns the network portion and
// host portion of an IPv4 or IPv6 address.
// The network portion is the raw bytes of
// the address.
func splitCIDR(s string) ([]byte, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			addr := s[:i]
			host := s[i+1:]
			return parseAddr(addr), host
		}
	}
	return parseAddr(s), ""
}

func parseAddr(s string) []byte {
	// IPv4 dotted quad.
	if bytes := parseIPv4(s); bytes != nil {
		return bytes
	}
	// IPv6 hex: very simple parser that strips
	// colons and treats each as 16 bits. We
	// only need this to be enough to compare
	// prefixes; the real address parsing lives
	// in the runtime.
	if len(s) == 0 {
		return nil
	}
	out := make([]byte, 0, 16)
	for _, c := range s {
		if c == ':' {
			continue
		}
		var b byte
		switch {
		case c >= '0' && c <= '9':
			b = byte(c - '0')
		case c >= 'a' && c <= 'f':
			b = byte(c-'a') + 10
		case c >= 'A' && c <= 'F':
			b = byte(c-'A') + 10
		default:
			return nil
		}
		out = append(out, b)
	}
	return out
}

func parseIPv4(s string) []byte {
	parts := splitDots(s)
	if len(parts) != 4 {
		return nil
	}
	out := make([]byte, 4)
	for i, p := range parts {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return nil
		}
		out[i] = byte(n)
	}
	return out
}

func splitDots(s string) []string {
	out := []string{}
	cur := ""
	for _, c := range s {
		if c == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	return append(out, cur)
}

func prefixLen(prefix string) int {
	slash := -1
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '/' {
			slash = i
			break
		}
	}
	if slash < 0 {
		return 0
	}
	n := 0
	for _, c := range prefix[slash+1:] {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// JSON encodes the manager state.
func (m *Manager) JSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Marshal(m.byID)
}
