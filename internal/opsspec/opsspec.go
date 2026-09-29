// opsspec/opsspec.go: G22 specialized DNS/DHCP/AAA/portal/
// flow/hardware operations catalogue.
//
// G22 wants one canonical place where the Agent declares
// the operations it can perform on a per-domain basis
// (DNS zone reload, DHCP lease flush, RADIUS CoA, portal
// session eviction, flow rule install, hardware port
// reset). The catalogue is read-only metadata that the
// API and CLI expose; the actual privileged execution
// remains on the Agent composition root.
//
// Every operation is identified by a canonical (Kind,
// Action) pair. The catalogue carries:
//   - Preconditions: which readiness rows must be
//     "supported" for the operation to be admissible
//   - Risk: a normalised "low" / "medium" / "high" label
//   - Audit: the audit categories the operation
//     contributes to
//
// The catalogue never exposes payloads or secrets; it is
// a metadata index the UI can use to render an honest
// "available actions" menu and the CLI can use to drive
// scripted operations.
package opsspec

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// SchemaVersion is the catalogue schema version.
const SchemaVersion = 1

// Risk enumerates the operation risk levels.
type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Valid reports whether r is a known risk level.
func (r Risk) Valid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

// Kind enumerates the canonical operation kinds.
type Kind string

const (
	KindDNS      Kind = "dns"
	KindDHCP     Kind = "dhcp"
	KindAAA      Kind = "aaa"
	KindPortal   Kind = "portal"
	KindFlow     Kind = "flow"
	KindHardware Kind = "hardware"
)

// Valid reports whether k is a known kind.
func (k Kind) Valid() bool {
	switch k {
	case KindDNS, KindDHCP, KindAAA, KindPortal, KindFlow, KindHardware:
		return true
	}
	return false
}

// Action enumerates the canonical action names per
// kind. New actions must be appended to preserve the
// wire-format contract.
type Action string

const (
	// DNS actions
	DNSReloadZone Action = "dns.reload-zone"
	DNSFlushCache Action = "dns.flush-cache"
	DNSSignZone   Action = "dns.sign-zone"

	// DHCP actions
	DHCPFlushLeases  Action = "dhcp.flush-leases"
	DHCPRescanSubnet Action = "dhcp.rescan-subnet"

	// AAA actions
	AAACoA        Action = "aaa.coa"
	AAADisconnect Action = "aaa.disconnect"

	// Portal actions
	PortalEvictSession Action = "portal.evict-session"
	PortalRotateToken  Action = "portal.rotate-token"

	// Flow actions
	FlowInstallRule Action = "flow.install-rule"
	FlowRemoveRule  Action = "flow.remove-rule"

	// Hardware actions
	HardwarePortReset  Action = "hardware.port-reset"
	HardwarePortPoE    Action = "hardware.port-poe"
	HardwareRadioReset Action = "hardware.radio-reset"
)

// Spec is one catalogue entry.
type Spec struct {
	Kind        Kind   `json:"kind"`
	Action      Action `json:"action"`
	Description string `json:"description"`
	Risk        Risk   `json:"risk"`
	// Preconditions lists the readiness feature keys the
	// operation requires. Empty means "no precondition".
	Preconditions []string `json:"preconditions,omitempty"`
	// Audit lists the canonical audit categories the
	// operation contributes to.
	Audit []string `json:"audit,omitempty"`
	// RBAC is the minimum identity.Permission the caller
	// must hold. The string is a stable canonical name;
	// the Agent maps it onto the identity package at
	// execution time.
	RBAC string `json:"rbac,omitempty"`
}

var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,63}$`)

// Validate reports whether s is internally consistent.
func (s Spec) Validate() error {
	if !s.Kind.Valid() {
		return errors.New("opsspec: kind is not canonical")
	}
	if !actionPattern.MatchString(string(s.Action)) {
		return errors.New("opsspec: action is not canonical")
	}
	if !s.Risk.Valid() {
		return errors.New("opsspec: risk is not canonical")
	}
	if len(s.Description) > 256 {
		return errors.New("opsspec: description too long")
	}
	for _, p := range s.Preconditions {
		if len(p) > 64 {
			return errors.New("opsspec: precondition too long")
		}
	}
	for _, a := range s.Audit {
		if len(a) > 64 {
			return errors.New("opsspec: audit category too long")
		}
	}
	if len(s.RBAC) > 64 {
		return errors.New("opsspec: rbac too long")
	}
	return nil
}

// Key returns the canonical (kind, action) pair.
func (s Spec) Key() string { return string(s.Kind) + ":" + string(s.Action) }

// Catalogue is the goroutine-safe catalogue store.
type Catalogue struct {
	mu    sync.RWMutex
	entry map[string]Spec
}

// NewCatalogue returns the canonical catalogue seeded
// with the G22 default entries.
func NewCatalogue() *Catalogue {
	c := &Catalogue{entry: map[string]Spec{}}
	for _, spec := range defaultSpecs() {
		c.entry[spec.Key()] = spec
	}
	return c
}

// Add inserts or replaces a spec.
func (c *Catalogue) Add(s Spec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entry[s.Key()] = s
	return nil
}

// Remove deletes the spec for the supplied key.
func (c *Catalogue) Remove(kind Kind, action Action) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entry, string(kind)+":"+string(action))
}

// Get returns the spec for the supplied key.
func (c *Catalogue) Get(kind Kind, action Action) (Spec, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.entry[string(kind)+":"+string(action)]
	return s, ok
}

// ByKind returns every spec for the supplied kind
// sorted by action.
func (c *Catalogue) ByKind(kind Kind) []Spec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []Spec
	for _, s := range c.entry {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}

// All returns every spec sorted by (kind, action).
func (c *Catalogue) All() []Spec {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Spec, 0, len(c.entry))
	for _, s := range c.entry {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Action < out[j].Action
	})
	return out
}

// Counters summarises the catalogue.
type Counters struct {
	Total  int          `json:"total"`
	ByKind map[Kind]int `json:"byKind"`
	ByRisk map[Risk]int `json:"byRisk"`
}

// Counters returns the current counts.
func (c *Catalogue) Counters() Counters {
	c.mu.RLock()
	defer c.mu.RUnlock()
	byKind := map[Kind]int{}
	byRisk := map[Risk]int{}
	for _, s := range c.entry {
		byKind[s.Kind]++
		byRisk[s.Risk]++
	}
	return Counters{Total: len(c.entry), ByKind: byKind, ByRisk: byRisk}
}

func defaultSpecs() []Spec {
	return []Spec{
		// DNS
		{Kind: KindDNS, Action: DNSReloadZone, Description: "Reload a DNS zone file", Risk: RiskLow, Preconditions: []string{"network.dns.bind"}, Audit: []string{"dns"}, RBAC: "configure-network"},
		{Kind: KindDNS, Action: DNSFlushCache, Description: "Flush a DNS resolver cache", Risk: RiskLow, Audit: []string{"dns"}, RBAC: "configure-network"},
		{Kind: KindDNS, Action: DNSSignZone, Description: "Re-sign a DNSSEC zone", Risk: RiskMedium, Preconditions: []string{"network.dns.bind"}, Audit: []string{"dns", "security"}, RBAC: "configure-network"},
		// DHCP
		{Kind: KindDHCP, Action: DHCPFlushLeases, Description: "Flush DHCP leases for a subnet", Risk: RiskMedium, Preconditions: []string{"network.dhcp.isc"}, Audit: []string{"dhcp"}, RBAC: "configure-network"},
		{Kind: KindDHCP, Action: DHCPRescanSubnet, Description: "Rescan a DHCP subnet for changes", Risk: RiskLow, Preconditions: []string{"network.dhcp.isc"}, Audit: []string{"dhcp"}, RBAC: "configure-network"},
		// AAA
		{Kind: KindAAA, Action: AAACoA, Description: "Send a RADIUS Change-of-Authorization", Risk: RiskMedium, Preconditions: []string{"aaa.radius"}, Audit: []string{"aaa", "security"}, RBAC: "configure-network"},
		{Kind: KindAAA, Action: AAADisconnect, Description: "Disconnect a RADIUS session", Risk: RiskMedium, Preconditions: []string{"aaa.radius"}, Audit: []string{"aaa", "security"}, RBAC: "configure-network"},
		// Portal
		{Kind: KindPortal, Action: PortalEvictSession, Description: "Evict a captive-portal session", Risk: RiskMedium, Audit: []string{"portal"}, RBAC: "configure-network"},
		{Kind: KindPortal, Action: PortalRotateToken, Description: "Rotate a captive-portal token", Risk: RiskMedium, Audit: []string{"portal", "security"}, RBAC: "configure-network"},
		// Flow
		{Kind: KindFlow, Action: FlowInstallRule, Description: "Install a flow rule", Risk: RiskHigh, Audit: []string{"flow", "security"}, RBAC: "configure-network"},
		{Kind: KindFlow, Action: FlowRemoveRule, Description: "Remove a flow rule", Risk: RiskMedium, Audit: []string{"flow", "security"}, RBAC: "configure-network"},
		// Hardware
		{Kind: KindHardware, Action: HardwarePortReset, Description: "Reset a switch / router port", Risk: RiskHigh, Audit: []string{"hardware"}, RBAC: "configure-network"},
		{Kind: KindHardware, Action: HardwarePortPoE, Description: "Toggle PoE on a port", Risk: RiskMedium, Audit: []string{"hardware"}, RBAC: "configure-network"},
		{Kind: KindHardware, Action: HardwareRadioReset, Description: "Reset a Wi-Fi radio", Risk: RiskHigh, Preconditions: []string{"wifi.ap"}, Audit: []string{"hardware", "wifi"}, RBAC: "configure-network"},
	}
}
