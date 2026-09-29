// Package wifi owns the bounded Wi-Fi AP/Station
// model the DuckOS V1 contract requires.
//
// The package is a model layer: it carries the
// canonical AP object, the canonical Station
// object and the Capability gate that reports
// whether the host actually supports Wi-Fi
// management. The renderer and the runner are
// out of scope for the foundation; the Agent
// only surfaces the model through the loopback
// API.
//
// The capability gate is the contract: an AP
// that the host cannot run is reported as
// "unsupported" rather than silently accepted.
// The operator never sees a checkbox that the
// host cannot honour.
package wifi

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
// this model serialises.
const SchemaVersion = "1"

// Hard caps. A V1 install never needs more than
// a handful of APs and a few dozen stations.
const (
	MaxAPs           = 8
	MaxStations      = 256
	MaxSSIDLen       = 32
	MaxNameLen       = 64
	MaxInterfaceLen  = 64
	MaxDriverLen     = 64
	MaxNotes         = 64
	MaxTopologyBytes = 32 << 10
	MaxStatusBytes   = 32 << 10
)

// idPattern is the canonical DuckOS id
// alphabet.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// interfacePattern is the canonical
// link/interface id alphabet used by the network
// config layer. The AP binds to a Linux
// wireless interface; the pattern keeps the
// AP and the network layer in lockstep.
var interfacePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// ssidPattern is the canonical SSID alphabet.
// SSIDs are 1-32 octets; the printable subset
// (excluding NUL, control characters and
// shell metacharacters) is bounded.
var ssidPattern = regexp.MustCompile(`^[\x20-\x7e]{1,32}$`)

// countryPattern is a 2-letter ISO 3166-1
// alpha-2 country code. The pattern is
// deliberately strict: regulatory codes that
// do not match the pattern are rejected.
var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// Band enumerates the supported Wi-Fi bands.
type Band string

const (
	Band2G Band = "2g"
	Band5G Band = "5g"
	Band6G Band = "6g"
)

func isValidBand(b Band) bool {
	switch b {
	case Band2G, Band5G, Band6G:
		return true
	}
	return false
}

// Security enumerates the supported Wi-Fi
// security modes. WEP and "open" are admitted
// for completeness but the renderer reports
// them as legacy and never recommends them.
type Security string

const (
	SecurityOpen           Security = "open"
	SecurityWPA2PSK        Security = "wpa2-psk"
	SecurityWPA3SAE        Security = "wpa3-sae"
	SecurityWPA2WPA3       Security = "wpa2-wpa3"
	SecurityWPA3Enterprise Security = "wpa3-enterprise"
)

func isValidSecurity(s Security) bool {
	switch s {
	case SecurityOpen, SecurityWPA2PSK, SecurityWPA3SAE, SecurityWPA2WPA3, SecurityWPA3Enterprise:
		return true
	}
	return false
}

// IsLegacy reports whether the security mode is
// legacy. The GUI/CLI uses the predicate to
// colour the row.
func IsLegacy(s Security) bool { return s == SecurityOpen }

// Capability is the bounded host-side report
// for Wi-Fi support. The Capability is the
// single source of truth: a V1 install with no
// wireless hardware reports Capability{false}
// and every AP is reported as "unsupported".
type Capability struct {
	Supported bool   `json:"supported"`
	Driver    string `json:"driver,omitempty"`
	Interface string `json:"interface,omitempty"`
	Bands     []Band `json:"bands,omitempty"`
	RegDomain string `json:"regDomain,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Valid enforces the per-capability contract.
func (c Capability) Valid() error {
	if c.Supported {
		if c.Driver == "" {
			return errors.New("wifi: capability driver is empty when supported")
		}
		if c.Interface != "" && !interfacePattern.MatchString(c.Interface) {
			return errors.New("wifi: capability interface is not canonical")
		}
		if len(c.Driver) > MaxDriverLen {
			return errors.New("wifi: capability driver is oversized")
		}
		if c.RegDomain != "" && !countryPattern.MatchString(c.RegDomain) {
			return errors.New("wifi: capability regDomain is not a canonical country code")
		}
	}
	return nil
}

// AP is the canonical Wi-Fi access point. The
// Interface field is the Linux wireless
// interface the AP binds to; an empty
// Interface means "any supported wireless
// interface". The PassphraseRef is an opaque
// Vault reference; the package never embeds the
// passphrase bytes.
type AP struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SSID          string   `json:"ssid"`
	Interface     string   `json:"interface,omitempty"`
	Band          Band     `json:"band"`
	Channel       int      `json:"channel"`
	Security      Security `json:"security"`
	PassphraseRef string   `json:"passphraseRef,omitempty"`
	Country       string   `json:"country,omitempty"`
	Enabled       bool     `json:"enabled"`
	Description   string   `json:"description,omitempty"`
	Hidden        bool     `json:"hidden,omitempty"`
}

// Valid enforces the per-ap contract. An AP
// that the host cannot run (no Interface, no
// supported band, no supported security) is
// still admitted at this layer: the Capability
// gate decides whether the AP is runnable.
func (a AP) Valid() error {
	if !idPattern.MatchString(a.ID) {
		return errors.New("wifi: ap id is not canonical")
	}
	if a.Name == "" || len(a.Name) > MaxNameLen {
		return errors.New("wifi: ap name is empty or oversized")
	}
	if !ssidPattern.MatchString(a.SSID) {
		return errors.New("wifi: ap ssid is not canonical")
	}
	if a.Interface != "" && !interfacePattern.MatchString(a.Interface) {
		return errors.New("wifi: ap interface is not canonical")
	}
	if !isValidBand(a.Band) {
		return errors.New("wifi: ap band is not canonical")
	}
	if !isValidSecurity(a.Security) {
		return errors.New("wifi: ap security is not canonical")
	}
	if a.Security != SecurityOpen && a.PassphraseRef == "" {
		return errors.New("wifi: passphraseRef is required for non-open security")
	}
	if a.PassphraseRef != "" {
		if len(a.PassphraseRef) > 128 {
			return errors.New("wifi: passphraseRef is oversized")
		}
		if strings.ContainsAny(a.PassphraseRef, " \t\n\r\"';\\") {
			return errors.New("wifi: passphraseRef contains shell metacharacter")
		}
	}
	if a.Country != "" && !countryPattern.MatchString(a.Country) {
		return errors.New("wifi: ap country is not a canonical country code")
	}
	if !validChannel(a.Band, a.Channel) {
		return errors.New("wifi: ap channel is not allowed for the band")
	}
	if a.Description != "" && len(a.Description) > 256 {
		return errors.New("wifi: ap description is oversized")
	}
	return nil
}

// validChannel reports whether the supplied
// channel is in the canonical DuckOS allow-
// list. The allow-list is conservative; an AP
// on a non-canonical channel is rejected at
// Validate time.
func validChannel(b Band, c int) bool {
	switch b {
	case Band2G:
		return c >= 1 && c <= 13
	case Band5G:
		return c == 36 || c == 40 || c == 44 || c == 48 ||
			c == 52 || c == 56 || c == 60 || c == 64 ||
			c == 100 || c == 104 || c == 108 || c == 112 ||
			c == 116 || c == 120 || c == 124 || c == 128 ||
			c == 132 || c == 136 || c == 140 || c == 144 ||
			c == 149 || c == 153 || c == 157 || c == 161 ||
			c == 165
	case Band6G:
		return c >= 1 && c <= 233
	}
	return false
}

// Station is the canonical Wi-Fi client record.
// The AP field is the canonical DuckOS AP id
// the station is associated with. The MAC is
// the canonical 6-octet MAC address. Signal is
// the dBm value as reported by the kernel; the
// renderer reports "<noise" as "weak".
type Station struct {
	ID          string `json:"id"`
	APID        string `json:"apId"`
	MAC         string `json:"mac"`
	Hostname    string `json:"hostname,omitempty"`
	SignalDbm   int    `json:"signalDbm"`
	ConnectedAt int64  `json:"connectedAt"`
}

// Valid enforces the per-station contract.
func (s Station) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("wifi: station id is not canonical")
	}
	if !idPattern.MatchString(s.APID) {
		return errors.New("wifi: station apId is not canonical")
	}
	if !macPattern.MatchString(s.MAC) {
		return errors.New("wifi: station mac is not canonical")
	}
	if s.SignalDbm < -120 || s.SignalDbm > 0 {
		return errors.New("wifi: station signal out of range")
	}
	if s.Hostname != "" && len(s.Hostname) > 64 {
		return errors.New("wifi: station hostname is oversized")
	}
	if s.ConnectedAt < 0 {
		return errors.New("wifi: station connectedAt is negative")
	}
	return nil
}

var macPattern = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)

// Topology is the canonical Wi-Fi plan. The
// Topology carries the configured APs; the
// runtime view is a separate Status.
type Topology struct {
	SchemaVersion string `json:"schemaVersion"`
	APs           []AP   `json:"aps"`
	GeneratedAt   int64  `json:"generated"`
}

// Valid enforces the topology contract.
func (t Topology) Valid() error {
	if t.SchemaVersion != SchemaVersion {
		return errors.New("wifi: schema version mismatch")
	}
	if len(t.APs) > MaxAPs {
		return errors.New("wifi: too many aps")
	}
	seen := map[string]bool{}
	for _, a := range t.APs {
		if seen[a.ID] {
			return errors.New("wifi: duplicate ap id")
		}
		seen[a.ID] = true
		if err := a.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Build sorts a Topology and validates it.
func Build(aps []AP, now time.Time) (Topology, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sort.Slice(aps, func(i, j int) bool { return aps[i].ID < aps[j].ID })
	t := Topology{SchemaVersion: SchemaVersion, APs: aps, GeneratedAt: now.Unix()}
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
		return nil, errors.New("wifi: topology exceeds maximum serialised size")
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

// Health is the canonical AP health state.
type Health string

const (
	HealthUnknown     Health = "unknown"
	HealthEnabled     Health = "enabled"
	HealthDisabled    Health = "disabled"
	HealthUnsupported Health = "unsupported"
	HealthConflicted  Health = "conflicted"
)

func isValidHealth(h Health) bool {
	switch h {
	case HealthUnknown, HealthEnabled, HealthDisabled, HealthUnsupported, HealthConflicted:
		return true
	}
	return false
}

// APSummary is the small struct the CLI prints.
type APSummary struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	SSID     string   `json:"ssid"`
	Band     Band     `json:"band"`
	Channel  int      `json:"channel"`
	Security Security `json:"security"`
	Health   Health   `json:"health"`
	Stations int      `json:"stations"`
}

// Summarise returns a deterministic, per-AP
// summary.
func Summarise(t Topology, health map[string]Health, stations map[string]int) []APSummary {
	out := make([]APSummary, 0, len(t.APs))
	for _, a := range t.APs {
		h := HealthUnknown
		if v, ok := health[a.ID]; ok {
			h = v
		} else if !a.Enabled {
			h = HealthDisabled
		}
		out = append(out, APSummary{ID: a.ID, Name: a.Name, SSID: a.SSID, Band: a.Band, Channel: a.Channel, Security: a.Security, Health: h, Stations: stations[a.ID]})
	}
	return out
}

// Status is the runtime view the Agent keeps.
// The Status carries the topology, the
// per-AP health, the per-AP station count and
// the host capability report.
type Status struct {
	Topology   Topology          `json:"topology"`
	Health     map[string]Health `json:"health"`
	Stations   map[string]int    `json:"stations"`
	Capability Capability        `json:"capability"`
	ProbedAt   int64             `json:"probedAt"`
	LastError  string            `json:"lastError,omitempty"`
	HasAPs     int               `json:"hasAps"`
}

// EncodeStatus serialises a Status using the
// stable encoder. The output is bounded by
// MaxStatusBytes.
func (s Status) Encode() ([]byte, error) {
	data, err := jsonMarshalStable(s)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxStatusBytes {
		return nil, errors.New("wifi: status exceeds maximum serialised size")
	}
	return data, nil
}
