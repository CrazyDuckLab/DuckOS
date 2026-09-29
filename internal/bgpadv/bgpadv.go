// bgpadv/bgpadv.go: V1 advanced BGP
// surface. The package owns the typed
// contracts for standard / extended /
// large communities, RPKI ROA validation,
// BMP export targets, BGP FlowSpec with
// strict guardrails, route-reflector
// clients, graceful-shutdown community,
// max-prefix, and GTSM.
//
// The runtime is a thin mapping to the
// existing internal/frr package.
package bgpadv

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Community is a standard or extended
// community.
type Community string

// LargeCommunity is the RFC 8092 large
// community triple.
type LargeCommunity struct {
	ASN    uint32 `json:"asn"`
	Local1 uint32 `json:"local1"`
	Local2 uint32 `json:"local2"`
}

// Valid reports whether l is well-formed.
func (l LargeCommunity) Valid() error {
	if l.ASN == 0 {
		return errors.New("bgpadv: large community ASN is required")
	}
	return nil
}

// String returns the canonical
// ASN:L1:L2 form.
func (l LargeCommunity) String() string {
	return uintStr(l.ASN) + ":" + uintStr(l.Local1) + ":" + uintStr(l.Local2)
}

func uintStr(n uint32) string {
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

// Well-known communities.
const (
	CommNoExport         Community = "no-export"
	CommNoAdvertise      Community = "no-advertise"
	CommNoExportSubconf  Community = "no-export-subconf"
	CommGracefulShutdown Community = "graceful-shutdown"
	CommBlackhole        Community = "blackhole"
)

// Valid reports whether c is well-known.
func (c Community) Valid() bool {
	switch c {
	case CommNoExport, CommNoAdvertise, CommNoExportSubconf,
		CommGracefulShutdown, CommBlackhole:
		return true
	}
	return false
}

// RPKIState is the ROA validation state.
type RPKIState string

const (
	RPKIValid    RPKIState = "valid"
	RPKIInvalid  RPKIState = "invalid"
	RPKINotFound RPKIState = "not-found"
	RPKIUnknown  RPKIState = "unknown"
)

// Valid reports whether s is known.
func (s RPKIState) Valid() bool {
	switch s {
	case RPKIValid, RPKIInvalid, RPKINotFound, RPKIUnknown:
		return true
	}
	return false
}

// ROA is a single ROA record.
type ROA struct {
	ASN       uint32    `json:"asn"`
	Prefix    string    `json:"prefix"`
	MaxLength int       `json:"maxLength"`
	Source    string    `json:"source"` // cache source
	FetchedAt time.Time `json:"fetchedAt"`
}

// Valid reports whether r is well-formed.
func (r ROA) Valid() error {
	if r.ASN == 0 {
		return errors.New("bgpadv: ROA ASN is required")
	}
	if r.Prefix == "" {
		return errors.New("bgpadv: ROA prefix is required")
	}
	if r.MaxLength < 0 || r.MaxLength > 128 {
		return errors.New("bgpadv: ROA maxLength out of range")
	}
	return nil
}

// RPKIResult is the ROA validation result
// for one prefix.
type RPKIResult struct {
	Prefix string    `json:"prefix"`
	ASN    uint32    `json:"asn"`
	State  RPKIState `json:"state"`
	// Reason is the explanation when the
	// state is invalid or not-found.
	Reason string `json:"reason,omitempty"`
}

// RPKIServer is the RTR cache.
type RPKIServer struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
}

// RPKIManager is the typed RPKI cache.
type RPKIManager struct {
	mu     sync.Mutex
	roas   []ROA
	server RPKIServer
}

// NewRPKIManager returns an empty manager.
func NewRPKIManager() *RPKIManager { return &RPKIManager{} }

// SetServer replaces the RTR server config.
func (m *RPKIManager) SetServer(s RPKIServer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.server = s
}

// SetROAs replaces the ROA cache.
func (m *RPKIManager) SetROAs(rs []ROA) error {
	for _, r := range rs {
		if err := r.Valid(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roas = rs
	return nil
}

// ROAs returns a sorted copy of the cache.
func (m *RPKIManager) ROAs() []ROA {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]ROA(nil), m.roas...)
	sort.Slice(out, func(i, k int) bool { return out[i].Prefix < out[k].Prefix })
	return out
}

// Validate returns the RPKI state for the
// supplied (prefix, ASN) pair against the
// cache. The lookup is deterministic.
func (m *RPKIManager) Validate(prefix string, asn uint32) RPKIResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.roas {
		if r.Prefix == prefix && r.ASN == asn {
			return RPKIResult{Prefix: prefix, ASN: asn, State: RPKIValid, Reason: "ROA matches origin"}
		}
	}
	// If any ROA exists for the prefix with
	// a different ASN, the prefix is invalid.
	for _, r := range m.roas {
		if r.Prefix == prefix {
			return RPKIResult{Prefix: prefix, ASN: asn, State: RPKIInvalid, Reason: "ROA exists with different origin ASN"}
		}
	}
	return RPKIResult{Prefix: prefix, ASN: asn, State: RPKINotFound, Reason: "no ROA matches the prefix"}
}

// BMPTarget is a BMP collector endpoint.
type BMPTarget struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	Enabled bool   `json:"enabled"`
	// StatIntervalSec is the BMP stat
	// interval.
	StatIntervalSec int `json:"statIntervalSec,omitempty"`
}

// Valid reports whether b is well-formed.
func (b BMPTarget) Valid() error {
	if b.Address == "" {
		return errors.New("bgpadv: BMP address is required")
	}
	if b.Port < 1 || b.Port > 65535 {
		return errors.New("bgpadv: BMP port out of range")
	}
	return nil
}

// FlowSpec is a BGP FlowSpec rule.
type FlowSpec struct {
	ID      string `json:"id"`
	Comment string `json:"comment,omitempty"`
	// Match is a set of typed match
	// criteria.
	Match FlowSpecMatch `json:"match"`
	// Action is the action to apply.
	Action FlowSpecAction `json:"action"`
	// SourceASNs is the set of ASNs
	// allowed to install this rule. The
	// guardrail: a FlowSpec rule is
	// applied only when its origin ASN
	// belongs to SourceASNs.
	SourceASNs []uint32 `json:"sourceAsns"`
	// ExpiresAt is the expiry timestamp.
	ExpiresAt time.Time `json:"expiresAt"`
	// PreviewOnly is the safe-review
	// mode: the rule is shown in the
	// looking glass but never installed.
	PreviewOnly bool `json:"previewOnly,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether f is well-formed.
func (f FlowSpec) Valid() error {
	if !idPattern.MatchString(f.ID) {
		return errors.New("bgpadv: flowspec id is not canonical")
	}
	if f.Action.Kind == "" {
		return errors.New("bgpadv: action is required")
	}
	if !f.ExpiresAt.IsZero() && !f.ExpiresAt.After(time.Now()) {
		return errors.New("bgpadv: expiresAt must be in the future")
	}
	return nil
}

// FlowSpecMatch is a typed match.
type FlowSpecMatch struct {
	DestPrefix   string `json:"destPrefix,omitempty"`
	SrcPrefix    string `json:"srcPrefix,omitempty"`
	Protocol     string `json:"protocol,omitempty"` // tcp/udp/icmp
	DstPort      int    `json:"dstPort,omitempty"`
	SrcPort      int    `json:"srcPort,omitempty"`
	DSCP         int    `json:"dscp,omitempty"`
	PacketLength int    `json:"packetLength,omitempty"`
}

// FlowSpecActionKind is the FlowSpec action.
type FlowSpecActionKind string

const (
	FSActionRateLimit FlowSpecActionKind = "rate-limit"
	FSActionDiscard   FlowSpecActionKind = "discard"
	FSActionRedirect  FlowSpecActionKind = "redirect"
	FSActionAccept    FlowSpecActionKind = "accept"
)

// Valid reports whether k is known.
func (k FlowSpecActionKind) Valid() bool {
	switch k {
	case FSActionRateLimit, FSActionDiscard, FSActionRedirect, FSActionAccept:
		return true
	}
	return false
}

// FlowSpecAction is a typed action.
type FlowSpecAction struct {
	Kind FlowSpecActionKind `json:"kind"`
	// RateKbps is the rate-limit for the
	// rate-limit action.
	RateKbps int `json:"rateKbps,omitempty"`
	// RedirectTarget is the redirect
	// target.
	RedirectTarget string `json:"redirectTarget,omitempty"`
}

// FlowSpecManager is the typed FlowSpec
// store.
type FlowSpecManager struct {
	mu    sync.Mutex
	rules map[string]FlowSpec
	clock func() time.Time
}

// NewFlowSpecManager returns an empty
// manager.
func NewFlowSpecManager(clock func() time.Time) *FlowSpecManager {
	if clock == nil {
		clock = time.Now
	}
	return &FlowSpecManager{rules: map[string]FlowSpec{}, clock: clock}
}

// Add inserts a rule.
func (m *FlowSpecManager) Add(f FlowSpec) error {
	if err := f.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[f.ID]; ok {
		return errors.New("bgpadv: flowspec id already exists")
	}
	m.rules[f.ID] = f
	return nil
}

// Revoke removes a rule.
func (m *FlowSpecManager) Revoke(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[id]; !ok {
		return errors.New("bgpadv: unknown flowspec id")
	}
	delete(m.rules, id)
	return nil
}

// SweepExpired walks the rules and removes
// every expired one. The function returns
// the IDs that were removed.
func (m *FlowSpecManager) SweepExpired() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock()
	out := []string{}
	for id, f := range m.rules {
		if f.ExpiresAt.IsZero() {
			continue
		}
		if now.After(f.ExpiresAt) {
			out = append(out, id)
			delete(m.rules, id)
		}
	}
	sort.Strings(out)
	return out
}

// List returns every rule sorted by id.
func (m *FlowSpecManager) List() []FlowSpec {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]FlowSpec, 0, len(m.rules))
	for _, f := range m.rules {
		out = append(out, f)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// OriginAllowed reports whether the origin
// ASN is allowed to install the rule.
func (m *FlowSpecManager) OriginAllowed(id string, asn uint32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.rules[id]
	if !ok {
		return false
	}
	for _, x := range f.SourceASNs {
		if x == asn {
			return true
		}
	}
	return false
}

// PrefixLen is a helper for matching.
// FlowSpec rules are often matched against
// the longuest prefix; the helper returns
// the prefix length in bits.
func PrefixLen(prefix string) int {
	for i := 0; i < len(prefix); i++ {
		if prefix[i] == '/' {
			n := 0
			for j := i + 1; j < len(prefix); j++ {
				if prefix[j] < '0' || prefix[j] > '9' {
					return -1
				}
				n = n*10 + int(prefix[j]-'0')
			}
			return n
		}
	}
	return -1
}
