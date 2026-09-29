// hostapd/hostapd.go: V1 hostapd
// configuration model and renderer. The
// package owns the typed contracts for
// 802.11 access-point configuration, the
// deterministic hostapd.conf renderer,
// the validator, and the radio-capability
// gate.
//
// DuckOS owns the intent. The runtime is
// a thin mapping to the hostapd daemon
// (validated against the man page schema).
package hostapd

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Band is the 802.11 band.
type Band string

const (
	Band2G Band = "2g"
	Band5G Band = "5g"
	Band6G Band = "6g"
)

// Valid reports whether b is known.
func (b Band) Valid() bool { return b == Band2G || b == Band5G || b == Band6G }

// Security is the AP security mode.
type Security string

const (
	SecWPA2Personal   Security = "wpa2-personal"
	SecWPA3Personal   Security = "wpa3-personal"
	SecWPA2Enterprise Security = "wpa2-enterprise"
	SecWPA3Enterprise Security = "wpa3-enterprise"
	SecOpen           Security = "open"
)

// Valid reports whether s is known.
func (s Security) Valid() bool {
	switch s {
	case SecWPA2Personal, SecWPA3Personal, SecWPA2Enterprise,
		SecWPA3Enterprise, SecOpen:
		return true
	}
	return false
}

// BSS is one 802.11 access point.
type BSS struct {
	Name       string   `json:"name"`
	Interface  string   `json:"interface"`
	Driver     string   `json:"driver"` // nl80211 / hostap / bsd
	SSID       string   `json:"ssid"`
	Band       Band     `json:"band"`
	Channel    int      `json:"channel"`
	HT_CAPAB   []string `json:"htCapab,omitempty"`
	VHT_CAPAB  []string `json:"vhtCapab,omitempty"`
	Security   Security `json:"security"`
	Passphrase string   `json:"passphrase,omitempty"` // vault ref in production
	AuthServer string   `json:"authServer,omitempty"`
	AuthPort   int      `json:"authPort,omitempty"`
	AuthSecret string   `json:"authSecret,omitempty"` // vault ref
	ACLSecret  string   `json:"aclSecret,omitempty"`  // vault ref
	VLAN       int      `json:"vlan,omitempty"`
	// WMM enables Wi-Fi Multimedia.
	WMM bool `json:"wmm,omitempty"`
	// IEE80211w enables Protected Management
	// Frames (PMF) when Security is WPA3.
	IEE80211w int `json:"ieee80211w,omitempty"`
	// Bridge attaches the AP to a bridge.
	Bridge string `json:"bridge,omitempty"`
	// HideESSID makes the SSID broadcast
	// invisible.
	HideESSID bool `json:"hideEssid,omitempty"`
}

var bssNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)
var ifnamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether b is well-formed.
func (b BSS) Valid() error {
	if !bssNamePattern.MatchString(b.Name) {
		return errors.New("hostapd: name is not canonical")
	}
	if !ifnamePattern.MatchString(b.Interface) {
		return errors.New("hostapd: interface is not canonical")
	}
	if b.SSID == "" {
		return errors.New("hostapd: ssid is required")
	}
	if !b.Band.Valid() {
		return errors.New("hostapd: band is unknown")
	}
	if b.Channel < 1 || b.Channel > 233 {
		return errors.New("hostapd: channel out of range")
	}
	if !b.Security.Valid() {
		return errors.New("hostapd: security is unknown")
	}
	if (b.Security == SecWPA2Personal || b.Security == SecWPA3Personal) && len(b.Passphrase) < 8 {
		return errors.New("hostapd: passphrase too short (>=8)")
	}
	if (b.Security == SecWPA2Enterprise || b.Security == SecWPA3Enterprise) && (b.AuthServer == "" || b.AuthPort == 0) {
		return errors.New("hostapd: enterprise security requires authServer + authPort")
	}
	return nil
}

// Config is the hostapd configuration.
type Config struct {
	BSSes []BSS `json:"bsses"`
	// LogLevel is 0..5.
	LogLevel int `json:"logLevel,omitempty"`
}

// Valid reports whether c is well-formed.
func (c Config) Valid() error {
	if len(c.BSSes) == 0 {
		return errors.New("hostapd: at least one BSS is required")
	}
	for _, b := range c.BSSes {
		if err := b.Valid(); err != nil {
			return err
		}
	}
	if c.LogLevel < 0 || c.LogLevel > 5 {
		return errors.New("hostapd: logLevel out of range")
	}
	return nil
}

// Render produces the hostapd.conf text.
// The output is deterministic.
func Render(c Config) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# DuckOS-generated hostapd.conf\n")
	sb.WriteString("# Do not edit by hand.\n")
	if c.LogLevel > 0 {
		fmt.Fprintf(&sb, "logger_syslog=-1\nlogger_syslog_level=%d\n", c.LogLevel)
	}
	bsses := append([]BSS(nil), c.BSSes...)
	sort.Slice(bsses, func(i, k int) bool { return bsses[i].Name < bsses[k].Name })
	first := true
	for i, b := range bsses {
		if first {
			fmt.Fprintf(&sb, "interface=%s\n", b.Interface)
			fmt.Fprintf(&sb, "driver=%s\n", b.Driver)
			first = false
		} else {
			fmt.Fprintf(&sb, "bss=%s\n", b.Interface)
		}
		fmt.Fprintf(&sb, "ssid=%s\n", b.SSID)
		if b.HideESSID {
			sb.WriteString("ignore_broadcast_ssid=1\n")
		}
		fmt.Fprintf(&sb, "hw_mode=%s\n", bandToHw(b.Band))
		fmt.Fprintf(&sb, "channel=%d\n", b.Channel)
		if b.WMM {
			sb.WriteString("wmm_enabled=1\n")
		}
		if b.VHT_CAPAB == nil && b.Band == Band5G {
			fmt.Fprintf(&sb, "ieee80211ac=1\n")
		}
		switch b.Security {
		case SecOpen:
			// nothing
		case SecWPA2Personal:
			sb.WriteString("wpa=2\n")
			sb.WriteString("wpa_key_mgmt=WPA-PSK\n")
			sb.WriteString("wpa_pairwise=CCMP\n")
			fmt.Fprintf(&sb, "wpa_passphrase=%s\n", b.Passphrase)
		case SecWPA3Personal:
			sb.WriteString("wpa=2\n")
			sb.WriteString("wpa_key_mgmt=SAE\n")
			sb.WriteString("wpa_pairwise=CCMP\n")
			fmt.Fprintf(&sb, "sae_password=%s\n", b.Passphrase)
		case SecWPA2Enterprise:
			sb.WriteString("wpa=2\n")
			sb.WriteString("wpa_key_mgmt=WPA-EAP\n")
			fmt.Fprintf(&sb, "auth_server_addr=%s\n", b.AuthServer)
			fmt.Fprintf(&sb, "auth_server_port=%d\n", b.AuthPort)
			fmt.Fprintf(&sb, "auth_server_shared_secret=%s\n", b.AuthSecret)
		case SecWPA3Enterprise:
			sb.WriteString("wpa=2\n")
			sb.WriteString("wpa_key_mgmt=WPA-EAP-SUITE-B-192\n")
			fmt.Fprintf(&sb, "auth_server_addr=%s\n", b.AuthServer)
			fmt.Fprintf(&sb, "auth_server_port=%d\n", b.AuthPort)
		}
		if b.IEE80211w > 0 {
			fmt.Fprintf(&sb, "ieee80211w=%d\n", b.IEE80211w)
		}
		if b.VLAN > 0 {
			fmt.Fprintf(&sb, "dynamic_vlan=1\nvlan_naming=1\nvlan_bridge=%s\n", b.Bridge)
		}
		if b.Bridge != "" {
			fmt.Fprintf(&sb, "bridge=%s\n", b.Bridge)
		}
		// '!' is the hostapd BSS separator. hostapd 2.10 rejects
		// a trailing '!' after the last BSS block ("invalid line '!'"),
		// so emit it only between blocks, never after the final one.
		if i+1 < len(bsses) {
			sb.WriteString("!\n")
		}
	}
	return sb.String(), nil
}

func bandToHw(b Band) string {
	switch b {
	case Band2G:
		return "g"
	case Band5G, Band6G:
		return "a"
	}
	return "g"
}

// Station is one observed client.
type Station struct {
	MAC         string    `json:"mac"`
	Signal      int       `json:"signal"`
	ConnectedAt time.Time `json:"connectedAt"`
	BSS         string    `json:"bss"`
}

// Manager is the runtime state store.
type Manager struct {
	mu       sync.Mutex
	config   Config
	previous Config
	stations map[string]Station
	clock    func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{stations: map[string]Station{}, clock: clock}
}

// Set replaces the candidate config.
func (m *Manager) Set(c Config) error {
	if err := c.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.previous = m.config
	m.config = c
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
	if len(m.previous.BSSes) == 0 {
		return errors.New("hostapd: no previous config")
	}
	m.config = m.previous
	m.previous = Config{}
	return nil
}

// SetStation records a station observation.
func (m *Manager) SetStation(s Station) {
	if s.MAC == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.ConnectedAt.IsZero() {
		s.ConnectedAt = m.clock().UTC()
	}
	m.stations[s.MAC] = s
}

// Stations returns the observed stations.
func (m *Manager) Stations() []Station {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Station, 0, len(m.stations))
	for _, s := range m.stations {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].MAC < out[k].MAC })
	return out
}
