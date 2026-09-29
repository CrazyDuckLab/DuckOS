// publish/publish.go: V1 Publish Service
// intent. The operator describes a service
// they want to expose (e.g. "blog on TCP
// 192.168.1.10:8080 from WAN1"); DuckOS
// computes the visible firewall/NAT/route
// effects and runs the Safe Apply.
//
// The package owns the typed contract: a
// Publish is a typed intent with a Plan
// (preview), a Status (applied/tested/expired),
// and an Expiry. The runtime mapping is
// downstream of the Safe Apply gate.
package publish

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Protocol is the IP protocol.
type Protocol string

const (
	ProtoTCP Protocol = "tcp"
	ProtoUDP Protocol = "udp"
)

// Valid reports whether p is known.
func (p Protocol) Valid() bool { return p == ProtoTCP || p == ProtoUDP }

// Intent describes the operator's goal.
type Intent struct {
	ID           string   `json:"id"`
	Service      string   `json:"service"`
	Description  string   `json:"description,omitempty"`
	WANID        string   `json:"wanId"`
	ExternalPort int      `json:"externalPort"`
	InternalHost string   `json:"internalHost"`
	InternalPort int      `json:"internalPort"`
	Protocol     Protocol `json:"protocol"`
	// SourceNetworks is the optional
	// allow-list of source addresses.
	SourceNetworks []string `json:"sourceNetworks,omitempty"`
	// ExpiresAt is the optional expiry. A
	// zero value means the intent is
	// persistent until the operator removes
	// it.
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	// CertificateID is the optional managed
	// certificate. The runtime may also
	// surface REALITY/TLS via the Xray
	// pipeline; this field is for the
	// reverse-proxy / web-server path.
	CertificateID string `json:"certificateId,omitempty"`
	// CreatedBy records the operator that
	// authored the intent.
	CreatedBy string `json:"createdBy"`
	// CreatedAt is the time the intent was
	// authored.
	CreatedAt time.Time `json:"createdAt"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether i is well-formed.
func (i Intent) Valid() error {
	if !idPattern.MatchString(i.ID) {
		return errors.New("publish: id is not canonical")
	}
	if i.Service == "" {
		return errors.New("publish: service is required")
	}
	if i.WANID == "" {
		return errors.New("publish: wanId is required")
	}
	if !i.Protocol.Valid() {
		return errors.New("publish: protocol is unknown")
	}
	if i.ExternalPort < 1 || i.ExternalPort > 65535 {
		return errors.New("publish: externalPort out of range")
	}
	if i.InternalPort < 1 || i.InternalPort > 65535 {
		return errors.New("publish: internalPort out of range")
	}
	if i.InternalHost == "" {
		return errors.New("publish: internalHost is required")
	}
	for _, s := range i.SourceNetworks {
		if s == "" {
			return errors.New("publish: empty source network")
		}
	}
	return nil
}

// Status is the runtime status of a published
// service.
type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusTested   Status = "tested"
	StatusExpired  Status = "expired"
	StatusDisabled Status = "disabled"
	StatusFailed   Status = "failed"
)

// Valid reports whether s is known.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusActive, StatusTested, StatusExpired, StatusDisabled, StatusFailed:
		return true
	}
	return false
}

// Plan is the Safe Apply preview of an intent.
type Plan struct {
	IntentID        string    `json:"intentId"`
	FirewallRules   []string  `json:"firewallRules"`
	NATRules        []string  `json:"natRules"`
	Routes          []string  `json:"routes"`
	CertificateDep  string    `json:"certificateDep,omitempty"`
	ConntrackImpact string    `json:"conntrackImpact"`
	ManagementRisk  string    `json:"managementRisk"`
	GeneratedAt     time.Time `json:"generatedAt"`
}

// ComputePlan returns the visible effects of
// an intent. The function is pure: it never
// inspects the kernel, only the intent and
// the model's risk heuristics.
func ComputePlan(i Intent) Plan {
	rules := []string{
		"accept in wan=" + i.WANID + " proto=" + string(i.Protocol) + " dport=" + itoa(i.ExternalPort),
		"dnat to " + i.InternalHost + ":" + itoa(i.InternalPort),
	}
	nat := []string{
		"snat to " + i.WANID + " from " + i.InternalHost,
	}
	risk := "low"
	if i.ExternalPort == 22 || i.ExternalPort == 23 || i.ExternalPort == 3389 {
		risk = "high"
	}
	return Plan{
		IntentID:        i.ID,
		FirewallRules:   rules,
		NATRules:        nat,
		Routes:          []string{},
		CertificateDep:  i.CertificateID,
		ConntrackImpact: "new-only",
		ManagementRisk:  risk,
		GeneratedAt:     time.Now().UTC(),
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Published is the persisted record.
type Published struct {
	Intent  Intent `json:"intent"`
	Plan    Plan   `json:"plan"`
	Status  Status `json:"status"`
	Applied int64  `json:"applied"`
}

// Manager is the V1 publish store. Manager is
// goroutine-safe.
type Manager struct {
	mu   sync.Mutex
	byID map[string]Published
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{byID: map[string]Published{}}
}

// Set replaces or inserts a published record.
func (m *Manager) Set(p Published) error {
	if err := p.Intent.Valid(); err != nil {
		return err
	}
	if !p.Status.Valid() {
		return errors.New("publish: status is unknown")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[p.Intent.ID] = p
	return nil
}

// Get returns a published record.
func (m *Manager) Get(id string) (Published, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.byID[id]
	return p, ok
}

// List returns the published records sorted
// by id.
func (m *Manager) List() []Published {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Published, 0, len(m.byID))
	for _, p := range m.byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Intent.ID < out[j].Intent.ID })
	return out
}

// Delete removes a published record.
func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.byID, id)
}

// PlanJSON is a JSON round-trip helper for
// the API surface.
func PlanJSON(p Plan) ([]byte, error) { return json.Marshal(p) }
