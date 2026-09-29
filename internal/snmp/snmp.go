// snmp/snmp.go: V1 SNMPv3 model. The package
// owns the typed contracts for secure
// defaults (auth + priv), the optional
// legacy v2c compatibility toggle, and the
// trap/inform destination list. The runtime
// is a thin mapping to net-snmp; the model
// is the contract.
package snmp

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// Version is the SNMP version.
type Version string

const (
	VersionV3  Version = "v3"
	VersionV2c Version = "v2c"
)

// Valid reports whether v is known.
func (v Version) Valid() bool { return v == VersionV3 || v == VersionV2c }

// AuthProtocol is the V1 auth protocol.
type AuthProtocol string

const (
	AuthSHA256 AuthProtocol = "sha256"
	AuthSHA1   AuthProtocol = "sha1"
)

// Valid reports whether a is known.
func (a AuthProtocol) Valid() bool { return a == AuthSHA256 || a == AuthSHA1 }

// PrivProtocol is the privacy protocol.
type PrivProtocol string

const (
	PrivAES128 PrivProtocol = "aes128"
	PrivAES192 PrivProtocol = "aes192"
	PrivAES256 PrivProtocol = "aes256"
)

// Valid reports whether p is known.
func (p PrivProtocol) Valid() bool {
	switch p {
	case PrivAES128, PrivAES192, PrivAES256:
		return true
	}
	return false
}

// User is an SNMPv3 user.
type User struct {
	Username  string       `json:"username"`
	AuthProto AuthProtocol `json:"authProto"`
	AuthVault string       `json:"authVault"`
	PrivProto PrivProtocol `json:"privProto"`
	PrivVault string       `json:"privVault"`
	ReadView  string       `json:"readView,omitempty"`
	WriteView string       `json:"writeView,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether u is well-formed.
func (u User) Valid() error {
	if !idPattern.MatchString(u.Username) {
		return errors.New("snmp: username is not canonical")
	}
	if !u.AuthProto.Valid() {
		return errors.New("snmp: authProto is unknown")
	}
	if u.AuthVault == "" {
		return errors.New("snmp: auth password must be a vault reference")
	}
	if !u.PrivProto.Valid() {
		return errors.New("snmp: privProto is unknown")
	}
	if u.PrivVault == "" {
		return errors.New("snmp: priv password must be a vault reference")
	}
	return nil
}

// Trap is a trap/inform destination.
type Trap struct {
	Host    string  `json:"host"`
	Port    int     `json:"port"`
	Version Version `json:"version"`
	// Community is the v2c community; empty
	// for v3.
	Community string `json:"community,omitempty"`
	// Inform toggles inform (acknowledged)
	// vs trap (fire-and-forget).
	Inform bool `json:"inform"`
}

// Valid reports whether t is well-formed.
func (t Trap) Valid() error {
	if t.Host == "" {
		return errors.New("snmp: host is required")
	}
	if !t.Version.Valid() {
		return errors.New("snmp: version is unknown")
	}
	if t.Version == VersionV2c && t.Community == "" {
		return errors.New("snmp: v2c requires a community")
	}
	if t.Port < 1 || t.Port > 65535 {
		return errors.New("snmp: port out of range")
	}
	return nil
}

// Manager is the V1 SNMP store.
type Manager struct {
	mu    sync.Mutex
	users map[string]User
	traps map[string]Trap
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{users: map[string]User{}, traps: map[string]Trap{}}
}

// SetUser replaces or inserts a user.
func (m *Manager) SetUser(u User) error {
	if err := u.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.Username] = u
	return nil
}

// SetTrap replaces or inserts a trap.
func (m *Manager) SetTrap(t Trap) error {
	if err := t.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traps[t.Host+":"+itoa(t.Port)] = t
	return nil
}

// Users returns the users sorted by username.
func (m *Manager) Users() []User {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

// Traps returns the traps sorted by host.
func (m *Manager) Traps() []Trap {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Trap, 0, len(m.traps))
	for _, t := range m.traps {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func itoa(n int) string {
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
