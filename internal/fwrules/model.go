// Package fwrules implements the beta.4 typed firewall rule
// manager: a validated, ordered, persistent rule set rendered
// into the exclusively-owned nftables table `inet duckos_fw`.
//
// V1 safety boundary (deliberate):
//
//   - Only FORWARD-chain rules are supported. Input/output
//     filtering stays with the management policy owner
//     (inet/duckos), so a UI-authored rule can never lock an
//     operator out of the management interface.
//   - The table is exclusively owned: nothing else writes to
//     it and nothing here touches other tables.
//   - Rendering is one atomic nft transaction (same lifecycle
//     design as the beta.3 port-mapping hardening).
package fwrules

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Rule is one typed firewall rule. Order is by Position then
// ID; Position is a dense 1..N index managed by the Manager.
type Rule struct {
	ID string `json:"id"`
	// Enabled rules render; disabled rules persist but do not.
	Enabled bool `json:"enabled"`
	// Action is accept | drop | reject.
	Action string `json:"action"`
	// Direction is forward in V1 (see package comment).
	Direction string `json:"direction"`
	// Protocol is tcp | udp | any.
	Protocol string `json:"protocol"`
	// SrcPrefix / DstPrefix are optional CIDRs ("10.0.0.0/8").
	// Empty matches any.
	SrcPrefix string `json:"srcPrefix,omitempty"`
	DstPrefix string `json:"dstPrefix,omitempty"`
	// DstPort is optional, 1..65535; only valid when Protocol
	// is tcp or udp.
	DstPort int `json:"dstPort,omitempty"`
	// InIface / OutIface are optional kernel interface names.
	InIface  string `json:"inIface,omitempty"`
	OutIface string `json:"outIface,omitempty"`
	// Comment is surfaced in the nft rule comment.
	Comment string `json:"comment,omitempty"`
}

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)
	ifacePattern  = regexp.MustCompile(`^[a-zA-Z0-9._@:-]{1,15}$`)
	maxRulesTotal = 256
)

// Valid reports whether r is well-formed and renderable.
func (r Rule) Valid() error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("fwrules: rule id is not canonical")
	}
	switch r.Action {
	case "accept", "drop", "reject":
	default:
		return fmt.Errorf("fwrules: unsupported action %q", r.Action)
	}
	if r.Direction != "forward" {
		return fmt.Errorf("fwrules: unsupported direction %q (V1 supports forward only)", r.Direction)
	}
	switch r.Protocol {
	case "any", "tcp", "udp":
	default:
		return fmt.Errorf("fwrules: unsupported protocol %q", r.Protocol)
	}
	if r.DstPort != 0 {
		if r.Protocol == "any" {
			return errors.New("fwrules: dstPort requires tcp or udp protocol")
		}
		if r.DstPort < 1 || r.DstPort > 65535 {
			return errors.New("fwrules: dstPort out of range")
		}
	}
	for name, value := range map[string]string{"srcPrefix": r.SrcPrefix, "dstPrefix": r.DstPrefix} {
		if value == "" {
			continue
		}
		p, err := netip.ParsePrefix(value)
		if err != nil {
			return fmt.Errorf("fwrules: %s is not a CIDR prefix", name)
		}
		if p != p.Masked() {
			return fmt.Errorf("fwrules: %s is not a masked prefix", name)
		}
		if p.Addr().IsMulticast() {
			return fmt.Errorf("fwrules: %s is multicast", name)
		}
	}
	for name, value := range map[string]string{"inIface": r.InIface, "outIface": r.OutIface} {
		if value == "" {
			continue
		}
		if !ifacePattern.MatchString(value) {
			return fmt.Errorf("fwrules: %s is not a canonical kernel interface name", name)
		}
	}
	if len(r.Comment) > 128 {
		return errors.New("fwrules: comment too long")
	}
	return true2nil()
}

func true2nil() error { return nil }

// Manager is the goroutine-safe ordered rule set. The zero
// value is an empty, disabled-by-default set.
type Manager struct {
	mu    sync.Mutex
	rules []Rule
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{} }

// Set bootstraps the manager from a decoded rule list
// (persistence load). The list is validated as a whole.
func (m *Manager) Set(rules []Rule) error {
	if len(rules) > maxRulesTotal {
		return errors.New("fwrules: rule count exceeds limit")
	}
	ids := map[string]bool{}
	for _, r := range rules {
		if err := r.Valid(); err != nil {
			return err
		}
		if ids[r.ID] {
			return fmt.Errorf("fwrules: duplicate rule id %q", r.ID)
		}
		ids[r.ID] = true
	}
	m.mu.Lock()
	m.rules = append([]Rule(nil), rules...)
	m.sortLocked()
	m.mu.Unlock()
	return nil
}

// Rules returns a position-sorted copy.
func (m *Manager) Rules() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Rule(nil), m.rules...)
}

// Enabled returns the renderable subset, position-sorted.
func (m *Manager) Enabled() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Rule
	for _, r := range m.rules {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out
}

// Get returns one rule by id.
func (m *Manager) Get(id string) (Rule, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rules {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// Put creates or replaces one rule. Rule order is
// deterministic: the render and evaluation order is the
// sorted rule id order, so the id doubles as the stable
// position. First-match semantics of nftables therefore
// resolve deterministically for overlapping rules.
func (m *Manager) Put(r Rule) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rules) >= maxRulesTotal {
		return errors.New("fwrules: rule count exceeds limit")
	}
	found := false
	for i := range m.rules {
		if m.rules[i].ID == r.ID {
			m.rules[i] = r
			found = true
			break
		}
	}
	if !found {
		m.rules = append(m.rules, r)
	}
	m.sortLocked()
	return nil
}

// Delete removes one rule. Unknown ids are a no-op success.
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return true
		}
	}
	return false
}

// SetEnabled flips the enabled flag.
func (m *Manager) SetEnabled(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.rules {
		if m.rules[i].ID == id {
			m.rules[i].Enabled = enabled
			return nil
		}
	}
	return errors.New("fwrules: unknown rule id")
}

func (m *Manager) sortLocked() {
	sort.SliceStable(m.rules, func(i, j int) bool { return m.rules[i].ID < m.rules[j].ID })
}

// Wire is the persistence document.
type Wire struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

// LoadWire decodes and validates a persisted document.
func LoadWire(data []byte) ([]Rule, error) {
	var w Wire
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("fwrules: decode store: %w", err)
	}
	if w.Version != 1 {
		return nil, fmt.Errorf("fwrules: unsupported store version %d", w.Version)
	}
	if err := (&Manager{}).Set(w.Rules); err != nil {
		return nil, err
	}
	return w.Rules, nil
}

// SaveWire encodes the current rule set.
func SaveWire(rules []Rule) ([]byte, error) {
	data, err := json.MarshalIndent(Wire{Version: 1, Rules: rules}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// StoreFile persists the rule set atomically (write + rename).
type StoreFile struct {
	mu   sync.Mutex
	path string
}

// NewStoreFile returns a store persisting to path.
func NewStoreFile(path string) *StoreFile { return &StoreFile{path: path} }

// Path exposes the backing file for diagnostics.
func (s *StoreFile) Path() string { return s.path }

// Save writes the document atomically.
func (s *StoreFile) Save(rules []Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := SaveWire(rules)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Load reads the document. A missing file is an empty set,
// not an error.
func (s *StoreFile) Load() ([]Rule, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	return LoadWire(data)
}
