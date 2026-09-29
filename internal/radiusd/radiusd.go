// radiusd/radiusd.go: V1 FreeRADIUS
// runtime integration. The package owns
// the typed configuration model, the
// deterministic renderer, the validator,
// the lifecycle, and the runtime state
// collector.
//
// DuckOS owns the intent. The runtime is
// a thin mapping to the freeradius daemon
// (validated against the /etc/freeradius
// schema). The package does not shell out
// at runtime; it renders configuration
// text the privileged Agent applies.
package radiusd

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Server is a FreeRADIUS server instance.
type Server struct {
	Name       string   `json:"name"`
	ListenAuth string   `json:"listenAuth,omitempty"` // "1812"
	ListenAcct string   `json:"listenAcct,omitempty"` // "1813"
	Listen     string   `json:"listen,omitempty"`     // "0.0.0.0"
	Clients    []Client `json:"clients,omitempty"`
	Realms     []Realm  `json:"realms,omitempty"`
	LogLevel   int      `json:"logLevel,omitempty"` // 0..3
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,31}$`)

// Valid reports whether s is well-formed.
func (s Server) Valid() error {
	if !namePattern.MatchString(s.Name) {
		return errors.New("radiusd: name is not canonical")
	}
	if s.ListenAuth == "" {
		s.ListenAuth = "1812"
	}
	if s.ListenAcct == "" {
		s.ListenAcct = "1813"
	}
	if s.LogLevel < 0 || s.LogLevel > 3 {
		return errors.New("radiusd: logLevel out of range")
	}
	for _, c := range s.Clients {
		if err := c.Valid(); err != nil {
			return err
		}
	}
	for _, r := range s.Realms {
		if err := r.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Client is a RADIUS client (NAS).
type Client struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Secret    string `json:"secret"` // vault reference
	Shortname string `json:"shortname,omitempty"`
	// NASType is informational ("cisco",
	// "juniper", "other").
	NASType string `json:"nasType,omitempty"`
}

// Valid reports whether c is well-formed.
func (c Client) Valid() error {
	if c.Name == "" {
		return errors.New("radiusd: client name is required")
	}
	if c.Address == "" {
		return errors.New("radiusd: client address is required")
	}
	if c.Secret == "" {
		return errors.New("radiusd: client secret is required")
	}
	return nil
}

// Realm is a per-realm authentication
// configuration.
type Realm struct {
	Name   string `json:"name"`
	Auth   string `json:"auth"`
	Secret string `json:"secret,omitempty"`
	Strip  bool   `json:"strip,omitempty"`
}

// Valid reports whether r is well-formed.
func (r Realm) Valid() error {
	if r.Name == "" {
		return errors.New("radiusd: realm name is required")
	}
	if r.Auth == "" {
		return errors.New("radiusd: realm auth backend is required")
	}
	return nil
}

// Render produces the FreeRADIUS config
// text. The output is deterministic.
func Render(s Server) (string, error) {
	if err := s.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# DuckOS-generated freeradius config\n")
	fmt.Fprintf(&sb, "listen {\n  type = \"auth\"\n  ipaddr = %s\n  port = %s\n}\n", orDefault(s.Listen, "*"), s.ListenAuth)
	fmt.Fprintf(&sb, "listen {\n  type = \"acct\"\n  ipaddr = %s\n  port = %s\n}\n", orDefault(s.Listen, "*"), s.ListenAcct)
	cs := append([]Client(nil), s.Clients...)
	sort.Slice(cs, func(i, k int) bool { return cs[i].Name < cs[k].Name })
	for _, c := range cs {
		fmt.Fprintf(&sb, "client %s {\n  ipaddr = %s\n  secret = %s\n", c.Name, c.Address, c.Secret)
		if c.Shortname != "" {
			fmt.Fprintf(&sb, "  shortname = %s\n", c.Shortname)
		}
		if c.NASType != "" {
			fmt.Fprintf(&sb, "  nas_type = %s\n", c.NASType)
		}
		sb.WriteString("}\n")
	}
	rs := append([]Realm(nil), s.Realms...)
	sort.Slice(rs, func(i, k int) bool { return rs[i].Name < rs[k].Name })
	for _, r := range rs {
		fmt.Fprintf(&sb, "realm %s {\n  auth_pool = %s\n", r.Name, r.Auth)
		if r.Secret != "" {
			fmt.Fprintf(&sb, "  secret = %s\n", r.Secret)
		}
		if r.Strip {
			sb.WriteString("  strip\n")
		}
		sb.WriteString("}\n")
	}
	return sb.String(), nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// CoAState is the CoA / Disconnect state.
type CoAState string

const (
	CoAIdle    CoAState = "idle"
	CoASending CoAState = "sending"
	CoASuccess CoAState = "success"
	CoAFailed  CoAState = "failed"
)

// CoARecord is one CoA / Discommand record.
type CoARecord struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"` // "coa" or "disconnect"
	Client string    `json:"client"`
	User   string    `json:"user,omitempty"`
	State  CoAState  `json:"state"`
	Detail string    `json:"detail,omitempty"`
}

const MaxCoARecords = 1024

// Manager is the V1 RADIUS lifecycle store.
// Manager is goroutine-safe.
type Manager struct {
	mu       sync.Mutex
	config   Server
	previous Server
	coa      []CoARecord
	clock    func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{clock: clock}
}

// Set replaces the candidate config.
func (m *Manager) Set(s Server) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.previous = m.config
	m.config = s
	return nil
}

// Render returns the rendered config text.
func (m *Manager) Render() (string, error) {
	m.mu.Lock()
	c := m.config
	m.mu.Unlock()
	return Render(c)
}

// Rollback restores the previous config.
func (m *Manager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.previous.Name == "" {
		return errors.New("radiusd: no previous config")
	}
	m.config = m.previous
	m.previous = Server{}
	return nil
}

// RecordCoA appends a CoA record.
func (m *Manager) RecordCoA(r CoARecord) {
	if r.Action != "coa" && r.Action != "disconnect" {
		return
	}
	if r.At.IsZero() {
		r.At = m.clock().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.coa = append(m.coa, r)
	if len(m.coa) > MaxCoARecords {
		m.coa = m.coa[len(m.coa)-MaxCoARecords:]
	}
}

// CoARecords returns a copy of the CoA
// history, newest first.
func (m *Manager) CoARecords() []CoARecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CoARecord, len(m.coa))
	copy(out, m.coa)
	sort.SliceStable(out, func(i, k int) bool { return out[i].At.After(out[k].At) })
	return out
}
