// ddns/ddns.go: V1 DDNS provider model. The
// package owns the typed contracts for
// provider abstraction (Cloudflare, common
// providers, custom secure HTTP), the
// dependency on the selected WAN, and the
// CGNAT awareness flag.
//
// The runtime drives the actual update. The
// V1 contract is that DDNS never pretends to
// enable inbound access if the public
// routable IP is unavailable; the State
// surfaces the effective public IP and
// whether the address is CGNAT.
package ddns

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// ProviderKind is the V1 provider family.
type ProviderKind string

const (
	ProviderCloudflare   ProviderKind = "cloudflare"
	ProviderRoute53      ProviderKind = "route53"
	ProviderHetzner      ProviderKind = "hetzner"
	ProviderDigitalOcean ProviderKind = "digitalocean"
	ProviderCustom       ProviderKind = "custom"
)

// Valid reports whether k is known.
func (k ProviderKind) Valid() bool {
	switch k {
	case ProviderCloudflare, ProviderRoute53, ProviderHetzner, ProviderDigitalOcean, ProviderCustom:
		return true
	}
	return false
}

// Record is the DDNS record DuckOS maintains.
// The credentials live in the vault.
type Record struct {
	ID       string       `json:"id"`
	Hostname string       `json:"hostname"`
	Provider ProviderKind `json:"provider"`
	// VaultRef is the credential reference.
	VaultRef string `json:"vaultRef"`
	// ZoneID is the DNS zone (provider-
	// specific). Empty when not applicable.
	ZoneID string `json:"zoneId,omitempty"`
	// WANID is the WAN whose public IP is
	// used. The record follows the active
	// IP of this WAN.
	WANID string `json:"wanId"`
	// IntervalSec is the update interval.
	// Zero means "only on change".
	IntervalSec int `json:"intervalSec,omitempty"`
	// TTL is the record TTL.
	TTL int `json:"ttl"`
	// CGNATAware when true refuses to update
	// if the detected public IP is a known
	// CGNAT carrier-grade IP range.
	CGNATAware bool `json:"cgnatAware"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var hostPattern = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// Valid reports whether r is well-formed.
func (r Record) Valid() error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("ddns: id is not canonical")
	}
	if !hostPattern.MatchString(r.Hostname) {
		return errors.New("ddns: hostname is not a canonical FQDN")
	}
	if !r.Provider.Valid() {
		return errors.New("ddns: provider is unknown")
	}
	if r.VaultRef == "" {
		return errors.New("ddns: credentials must come from the vault")
	}
	if r.WANID == "" {
		return errors.New("ddns: wanId is required")
	}
	if r.TTL < 0 || r.TTL > 86400 {
		return errors.New("ddns: TTL out of range")
	}
	return nil
}

// State is the runtime state of a record.
type State struct {
	RecordID         string    `json:"recordId"`
	Hostname         string    `json:"hostname"`
	CurrentAddress   string    `json:"currentAddress"`
	DetectedPublic   string    `json:"detectedPublic"`
	CGNATDetected    bool      `json:"cgnatDetected"`
	LastUpdate       time.Time `json:"lastUpdate"`
	LastSuccess      time.Time `json:"lastSuccess"`
	LastError        string    `json:"lastError,omitempty"`
	ConsecutiveFails int       `json:"consecutiveFails"`
}

// Manager is the V1 DDNS store. Manager is
// goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	records map[string]Record
	states  map[string]State
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{records: map[string]Record{}, states: map[string]State{}}
}

// SetRecord replaces or inserts a record.
func (m *Manager) SetRecord(r Record) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[r.ID] = r
	return nil
}

// SetState replaces the runtime state of a
// record.
func (m *Manager) SetState(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[s.RecordID] = s
}

// GetRecord returns a record by id.
func (m *Manager) GetRecord(id string) (Record, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	return r, ok
}

// GetState returns the runtime state.
func (m *Manager) GetState(id string) (State, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.states[id]
	return s, ok
}

// List returns the records sorted by id.
func (m *Manager) List() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Delete removes a record and its state.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, id)
	delete(m.states, id)
}

// CanEnableForPublicIP reports whether a
// record may be enabled for the given
// detected public IP. CGNAT-aware records
// refuse well-known CGNAT ranges.
func (m *Manager) CanEnableForPublicIP(recordID, publicIP string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[recordID]
	if !ok {
		return false
	}
	if !r.CGNATAware {
		return true
	}
	return !isWellKnownCGNAT(publicIP)
}

// isWellKnownCGNAT is a conservative
// heuristic. Real CGNAT detection is a
// network measurement; this helper only
// filters obvious reserved ranges so the
// operator gets a visible warning.
func isWellKnownCGNAT(ip string) bool {
	// 100.64.0.0/10 is the CGNAT reserved
	// range per RFC 6598.
	if len(ip) < 7 {
		return false
	}
	if ip[:8] == "100.64.0." || ip[:8] == "100.127." {
		return true
	}
	if len(ip) >= 8 && ip[:7] == "100.64." {
		return true
	}
	return false
}
