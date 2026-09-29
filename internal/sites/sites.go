// Package sites owns the bounded reverse-proxy
// and dynamic-DNS surface the DuckOS V1
// contract requires.
//
// A Site is a public-facing listener bound to a
// local target. The Site model is the contract
// the GUI/CLI talks to; the renderer produces a
// deterministic Plan the operator can review
// before any daemon is touched. The Plan never
// runs a daemon directly: the Agent applies the
// plan through a Runner that resolves a reverse
// proxy, nftables NAT and a DDNS updater.
//
// The package is split into a model layer (this
// file), a renderer (render.go) and a controller
// stub (controller.go). The model layer is the
// only one the GUI/CLI touches.
package sites

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// jsonMarshalStable is the single JSON encoder
// for the package.
func jsonMarshalStable(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func jsonNewDecoder(data []byte) *json.Decoder {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec
}

// SchemaVersion is the wire/JSON schema version
// this model serialises. Bumping it requires a
// migration story.
const SchemaVersion = "1"

// Hard caps. A V1 install never needs more than
// a handful of sites; the bound is policy, not a
// guess.
const (
	MaxSites         = 32
	MaxNameLen       = 64
	MaxDomainLen     = 253
	MaxHostnameLen   = 253
	MaxProviderLen   = 64
	MaxNotes         = 64
	MaxPlanBytes     = 64 << 10
	MaxTopologyBytes = 32 << 10
)

// idPattern is the canonical DuckOS id
// alphabet. Lowercase, dashes, no colons, no
// leading digit.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// linkPattern is the canonical link/interface
// id alphabet used by the network config
// layer. The Site listener references the same
// id space.
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// domainPattern is a bounded DNS hostname
// pattern. The full RFC-1123 alphabet is too
// permissive; the pattern allows lowercase
// letters, digits, dashes and dots, and rejects
// leading or trailing dots or dashes.
var domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// Protocol enumerates the supported transport
// protocols. Unknown protocols are rejected.
type Protocol string

const (
	ProtocolTCP  Protocol = "tcp"
	ProtocolUDP  Protocol = "udp"
	ProtocolHTTP Protocol = "http"
	ProtocolTLS  Protocol = "tls"
)

func isValidProtocol(p Protocol) bool {
	switch p {
	case ProtocolTCP, ProtocolUDP, ProtocolHTTP, ProtocolTLS:
		return true
	}
	return false
}

// DDNSProvider enumerates the supported DDNS
// providers. The package never talks to a
// provider directly; the Agent owns the wire
// protocol.
type DDNSProvider string

const (
	DDNSProviderNone       DDNSProvider = ""
	DDNSProviderDuckDNS    DDNSProvider = "duckdns"
	DDNSProviderCloudflare DDNSProvider = "cloudflare"
	DDNSProviderNoIP       DDNSProvider = "noip"
	DDNSProviderRoute53    DDNSProvider = "route53"
)

func isValidProvider(p DDNSProvider) bool {
	switch p {
	case DDNSProviderNone, DDNSProviderDuckDNS, DDNSProviderCloudflare, DDNSProviderNoIP, DDNSProviderRoute53:
		return true
	}
	return false
}

// Listener is the public-facing front end of a
// site. InterfaceID is the DuckOS link id; Port
// is the TCP/UDP port; Protocol is the
// transport. The wildcard "0.0.0.0" address is
// implied when InterfaceID is empty.
type Listener struct {
	InterfaceID string   `json:"interfaceId,omitempty"`
	Port        int      `json:"port"`
	Protocol    Protocol `json:"protocol"`
}

// Valid enforces the per-listener contract.
func (l Listener) Valid() error {
	if l.InterfaceID != "" && !linkPattern.MatchString(l.InterfaceID) {
		return errors.New("sites: listener interfaceId is not canonical")
	}
	if l.Port < 1 || l.Port > 65535 {
		return errors.New("sites: listener port is out of range")
	}
	if !isValidProtocol(l.Protocol) {
		return errors.New("sites: listener protocol is not canonical")
	}
	return nil
}

// Target is the local back end of a site.
// Protocol must match the listener protocol
// for TCP/UDP; HTTP/TLS allow a different
// target protocol (e.g. listen on TLS, talk to
// local HTTP).
type Target struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Protocol Protocol `json:"protocol"`
}

// Valid enforces the per-target contract. The
// host is a DNS name or an IP address; the
// renderer refuses shell metacharacters.
func (t Target) Valid() error {
	if t.Host == "" {
		return errors.New("sites: target host is empty")
	}
	if len(t.Host) > MaxHostnameLen {
		return errors.New("sites: target host is oversized")
	}
	if strings.ContainsAny(t.Host, " \t\n\r\"';\\`$") {
		return errors.New("sites: target host contains shell metacharacter")
	}
	if t.Port < 1 || t.Port > 65535 {
		return errors.New("sites: target port is out of range")
	}
	if !isValidProtocol(t.Protocol) {
		return errors.New("sites: target protocol is not canonical")
	}
	return nil
}

// TLS is the optional TLS configuration of a
// site. CertRef and KeyRef are opaque
// references the Vault resolves; the package
// never embeds the key bytes.
type TLS struct {
	Enabled bool   `json:"enabled"`
	CertRef string `json:"certRef,omitempty"`
	KeyRef  string `json:"keyRef,omitempty"`
	SNI     string `json:"sni,omitempty"`
}

// Valid enforces the per-tls contract. An
// enabled TLS site without a CertRef/KeyRef
// is rejected at Validate time; the renderer
// never produces a Step that references an
// empty ref.
func (t TLS) Valid() error {
	if !t.Enabled {
		return nil
	}
	if t.CertRef == "" {
		return errors.New("sites: tls certRef is required when tls is enabled")
	}
	if t.KeyRef == "" {
		return errors.New("sites: tls keyRef is required when tls is enabled")
	}
	if len(t.CertRef) > 128 || len(t.KeyRef) > 128 {
		return errors.New("sites: tls ref is oversized")
	}
	if strings.ContainsAny(t.CertRef, " \t\n\r\"';\\") || strings.ContainsAny(t.KeyRef, " \t\n\r\"';\\") {
		return errors.New("sites: tls ref contains shell metacharacter")
	}
	if t.SNI != "" && !domainPattern.MatchString(t.SNI) {
		return errors.New("sites: tls sni is not a canonical hostname")
	}
	return nil
}

// DDNS is the optional dynamic-DNS
// configuration of a site. The package never
// embeds the provider secret; the Agent owns
// the secret store and resolves the provider
// through the Vault.
type DDNS struct {
	Provider    DDNSProvider `json:"provider"`
	Hostname    string       `json:"hostname,omitempty"`
	RefreshMins int          `json:"refreshMins,omitempty"`
	SecretRef   string       `json:"secretRef,omitempty"`
}

// Valid enforces the per-ddns contract. A
// provider of "" disables DDNS; a non-empty
// provider requires a hostname and a
// refresh interval.
func (d DDNS) Valid() error {
	if !isValidProvider(d.Provider) {
		return errors.New("sites: ddns provider is not canonical")
	}
	if d.Provider == DDNSProviderNone {
		return nil
	}
	if d.Hostname == "" {
		return errors.New("sites: ddns hostname is required when provider is set")
	}
	if !domainPattern.MatchString(d.Hostname) {
		return errors.New("sites: ddns hostname is not a canonical hostname")
	}
	if d.RefreshMins < 1 || d.RefreshMins > 1440 {
		return errors.New("sites: ddns refreshMins is out of range")
	}
	if d.SecretRef != "" {
		if len(d.SecretRef) > 128 {
			return errors.New("sites: ddns secretRef is oversized")
		}
		if strings.ContainsAny(d.SecretRef, " \t\n\r\"';\\") {
			return errors.New("sites: ddns secretRef contains shell metacharacter")
		}
	}
	return nil
}

// Site is the canonical reverse-proxy object.
// The Site carries one listener, one target,
// optional TLS, optional DDNS and an optional
// description.
type Site struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Domain      string   `json:"domain"`
	Listener    Listener `json:"listener"`
	Target      Target   `json:"target"`
	TLS         TLS      `json:"tls"`
	DDNS        DDNS     `json:"ddns"`
	Description string   `json:"description,omitempty"`
	Enabled     bool     `json:"enabled"`
}

// Valid enforces the per-site contract.
func (s Site) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("sites: site id is not canonical")
	}
	if s.Name == "" || len(s.Name) > MaxNameLen {
		return errors.New("sites: site name is empty or oversized")
	}
	if !domainPattern.MatchString(s.Domain) {
		return errors.New("sites: site domain is not a canonical hostname")
	}
	if err := s.Listener.Valid(); err != nil {
		return err
	}
	if err := s.Target.Valid(); err != nil {
		return err
	}
	if err := s.TLS.Valid(); err != nil {
		return err
	}
	if err := s.DDNS.Valid(); err != nil {
		return err
	}
	if s.Description != "" && len(s.Description) > 256 {
		return errors.New("sites: site description is oversized")
	}
	return nil
}

// Topology is the canonical Sites plan.
type Topology struct {
	SchemaVersion string `json:"schemaVersion"`
	Sites         []Site `json:"sites"`
	GeneratedAt   int64  `json:"generated"`
}

// Valid enforces the topology contract.
func (t Topology) Valid() error {
	if t.SchemaVersion != SchemaVersion {
		return errors.New("sites: schema version mismatch")
	}
	if len(t.Sites) == 0 {
		return errors.New("sites: topology has no sites")
	}
	if len(t.Sites) > MaxSites {
		return errors.New("sites: too many sites")
	}
	seen := map[string]bool{}
	for _, s := range t.Sites {
		if seen[s.ID] {
			return errors.New("sites: duplicate site id")
		}
		seen[s.ID] = true
		if err := s.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Build sorts a Topology and validates it.
func Build(sites []Site, now time.Time) (Topology, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].ID < sites[j].ID })
	t := Topology{SchemaVersion: SchemaVersion, Sites: sites, GeneratedAt: now.Unix()}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// EncodeTopology serialises a Topology.
func EncodeTopology(t Topology) ([]byte, error) {
	if err := t.Valid(); err != nil {
		return nil, err
	}
	data, err := jsonMarshalStable(t)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTopologyBytes {
		return nil, errors.New("sites: topology exceeds maximum serialised size")
	}
	return data, nil
}

// ParseTopology decodes a JSON payload.
func ParseTopology(data []byte) (Topology, error) {
	var t Topology
	dec := jsonNewDecoder(data)
	if err := dec.Decode(&t); err != nil {
		return Topology{}, err
	}
	if err := t.Valid(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// Health is the canonical site health state.
type Health string

// Status is the runtime view the Agent keeps.
// The Status carries the topology, the
// per-site health and the last probe
// timestamp. The Status is what the API
// returns to the GUI/CLI.
type Status struct {
	Topology  Topology          `json:"topology"`
	Health    map[string]Health `json:"health"`
	ProbedAt  int64             `json:"probedAt"`
	LastError string            `json:"lastError,omitempty"`
	HasSites  int               `json:"hasSites"`
}

const (
	HealthUnknown    Health = "unknown"
	HealthEnabled    Health = "enabled"
	HealthDisabled   Health = "disabled"
	HealthConflicted Health = "conflicted"
	HealthError      Health = "error"
)

func isValidHealth(h Health) bool {
	switch h {
	case HealthUnknown, HealthEnabled, HealthDisabled, HealthConflicted, HealthError:
		return true
	}
	return false
}

// SiteSummary is the small struct the CLI
// prints. It is intentionally tiny: the
// operator asks for "what is published and
// where", not the full TLS/DDNS state.
type SiteSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Listener string `json:"listener"`
	Target   string `json:"target"`
	TLS      bool   `json:"tls"`
	DDNS     bool   `json:"ddns"`
	Health   Health `json:"health"`
}

// Summarise returns a deterministic, per-site
// summary.
func Summarise(t Topology, health map[string]Health) []SiteSummary {
	out := make([]SiteSummary, 0, len(t.Sites))
	for _, s := range t.Sites {
		h := HealthUnknown
		if v, ok := health[s.ID]; ok {
			h = v
		} else if !s.Enabled {
			h = HealthDisabled
		}
		out = append(out, SiteSummary{ID: s.ID, Name: s.Name, Domain: s.Domain, Listener: formatListener(s.Listener), Target: formatTarget(s.Target), TLS: s.TLS.Enabled, DDNS: s.DDNS.Provider != DDNSProviderNone, Health: h})
	}
	return out
}

func formatListener(l Listener) string {
	if l.InterfaceID == "" {
		return formatPort(l.Port, l.Protocol)
	}
	return l.InterfaceID + ":" + formatPort(l.Port, l.Protocol)
}

func formatTarget(t Target) string {
	return t.Host + ":" + formatPort(t.Port, t.Protocol)
}

func formatPort(port int, p Protocol) string {
	return strings.ToLower(string(p)) + "/" + itoa(port)
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
