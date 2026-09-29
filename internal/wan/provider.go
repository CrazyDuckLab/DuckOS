// wan/provider.go: V1 WAN provider abstraction.
// Every DuckOS WAN is a Provider: a typed
// object with a mode (DHCP, Static, PPPoE,
// L2TP), hardware/configured/effective MAC
// addresses, MTU/MRU, and MSS clamping
// settings. The package owns the Provider
// model and the MAC model.
//
// The runtime is split: this file is the
// model; the Linux runtime is in
// internal/network. A real L2TP/PPPoE client
// is large; the V1 contract is the model and
// the safe abstractions, the live Linux
// runtime is qualified per provider.
package wan

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Mode is the WAN acquisition mode.
type Mode string

const (
	ModeDHCP   Mode = "dhcp"
	ModeStatic Mode = "static"
	ModePPPoE  Mode = "pppoe"
	ModeL2TP   Mode = "l2tp"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeDHCP, ModeStatic, ModePPPoE, ModeL2TP:
		return true
	}
	return false
}

// MAC is a canonical unicast MAC. The zero
// value is the special "use hardware MAC"
// sentinel.
type MAC string

var macPattern = regexp.MustCompile(`^([0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}$`)

// ParseMAC normalises a MAC string and
// returns it as a MAC. The function refuses
// broadcast/multicast bytes (lowest bit of
// the first octet must be 0) and refuses
// all-zero / all-one sentinels.
func ParseMAC(s string) (MAC, error) {
	if !macPattern.MatchString(s) {
		return "", errors.New("wan: MAC must be 6 hex octets")
	}
	first, err := hexByte(s[:2])
	if err != nil {
		return "", err
	}
	if first&0x01 != 0 {
		return "", errors.New("wan: MAC must be unicast")
	}
	if s == "00:00:00:00:00:00" || s == "ff:ff:ff:ff:ff:ff" {
		return "", errors.New("wan: MAC must not be all-zero or all-one")
	}
	return MAC(s), nil
}

func hexByte(s string) (byte, error) {
	var out byte
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			out = out<<4 | byte(c-'0')
		case c >= 'a' && c <= 'f':
			out = out<<4 | byte(c-'a'+10)
		case c >= 'A' && c <= 'F':
			out = out<<4 | byte(c-'A'+10)
		default:
			return 0, errors.New("wan: invalid hex")
		}
	}
	return out, nil
}

// Source is the configured MAC source. The
// runtime resolves the effective MAC by
// combining the link's hardware MAC with the
// configured override.
type Source string

const (
	// MACSourceHardware means: use the NIC's
	// hardware MAC; ignore any operator
	// override.
	MACSourceHardware Source = "hardware"
	// MACSourceOverride means: use the
	// operator-supplied MAC exactly.
	MACSourceOverride Source = "override"
	// MACSourceCloneCurrent means: clone the
	// MAC of the device currently connected
	// to the LAN.
	MACSourceCloneCurrent Source = "clone-current"
	// MACSourceCloneRecent means: clone from a
	// recently observed client MAC.
	MACSourceCloneRecent Source = "clone-recent"
)

// Valid reports whether s is a known MAC
// source.
func (s Source) Valid() bool {
	switch s {
	case MACSourceHardware, MACSourceOverride, MACSourceCloneCurrent, MACSourceCloneRecent:
		return true
	}
	return false
}

// MACConfig is the per-WAN MAC override.
type MACConfig struct {
	Source  Source `json:"source"`
	Address MAC    `json:"address,omitempty"`
}

// Valid reports whether m is a well-formed
// MACConfig.
func (m MACConfig) Valid() error {
	if !m.Source.Valid() {
		return errors.New("wan: MAC source is unknown")
	}
	if m.Source == MACSourceOverride && m.Address == "" {
		return errors.New("wan: MACSourceOverride requires an address")
	}
	if m.Address != "" {
		if _, err := ParseMAC(string(m.Address)); err != nil {
			return err
		}
	}
	return nil
}

// MTUConfig is the per-WAN MTU/MRU/MSS clamp.
type MTUConfig struct {
	// MTU is the interface MTU. Zero means
	// "use the kernel default".
	MTU int `json:"mtu,omitempty"`
	// MRU is the PPPoE receive-unit. Zero
	// means "match MTU".
	MRU int `json:"mru,omitempty"`
	// MSSClamp enables MSS clamping on the
	// WAN firewall chain.
	MSSClamp bool `json:"mssClamp,omitempty"`
	// MSSClampPMTU enables PMTU-aware MSS
	// clamping. Only effective when
	// MSSClamp is true.
	MSSClampPMTU bool `json:"mssClampPMTU,omitempty"`
}

// Valid reports whether m is well-formed.
func (m MTUConfig) Valid() error {
	if m.MTU < 0 || m.MTU > 9000 {
		return errors.New("wan: MTU out of bounds")
	}
	if m.MRU < 0 || m.MRU > 9000 {
		return errors.New("wan: MRU out of bounds")
	}
	if m.MSSClampPMTU && !m.MSSClamp {
		return errors.New("wan: MSSClampPMTU requires MSSClamp")
	}
	return nil
}

// StaticConfig is the static IP parameters.
type StaticConfig struct {
	Address    string   `json:"address"`
	Gateway    string   `json:"gateway"`
	DNSServers []string `json:"dnsServers,omitempty"`
}

// PPPoEConfig holds the PPPoE credentials and
// service name.
type PPPoEConfig struct {
	Username    string `json:"username"`
	PasswordRef string `json:"passwordRef"` // vault ref
	ServiceName string `json:"serviceName,omitempty"`
	ACName      string `json:"acName,omitempty"`
}

// L2TPConfig holds the L2TP/IPSec parameters.
type L2TPConfig struct {
	Server      string `json:"server"`
	Username    string `json:"username"`
	PasswordRef string `json:"passwordRef"`
	PSKRef      string `json:"pskRef,omitempty"`
}

// Provider is one WAN acquisition plan. A
// Provider always carries a Mode, a MAC
// configuration, and a Mode-specific sub-
// configuration.
type Provider struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Link    string        `json:"link"`
	Mode    Mode          `json:"mode"`
	MAC     MACConfig     `json:"mac"`
	MTU     MTUConfig     `json:"mtu"`
	Static  *StaticConfig `json:"static,omitempty"`
	PPPoE   *PPPoEConfig  `json:"pppoe,omitempty"`
	L2TP    *L2TPConfig   `json:"l2tp,omitempty"`
	Enabled bool          `json:"enabled"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether p is well-formed.
func (p Provider) Valid() error {
	if !idPattern.MatchString(p.ID) {
		return errors.New("wan: id is not canonical")
	}
	if !linkPattern.MatchString(p.Link) {
		return errors.New("wan: link is not canonical")
	}
	if !p.Mode.Valid() {
		return errors.New("wan: mode is unknown")
	}
	if err := p.MAC.Valid(); err != nil {
		return err
	}
	if err := p.MTU.Valid(); err != nil {
		return err
	}
	switch p.Mode {
	case ModeStatic:
		if p.Static == nil {
			return errors.New("wan: mode=static requires a static config")
		}
	case ModePPPoE:
		if p.PPPoE == nil {
			return errors.New("wan: mode=pppoe requires a PPPoE config")
		}
		if p.PPPoE.Username == "" || p.PPPoE.PasswordRef == "" {
			return errors.New("wan: PPPoE requires username and password reference")
		}
	case ModeL2TP:
		if p.L2TP == nil {
			return errors.New("wan: mode=l2tp requires an L2TP config")
		}
		if p.L2TP.Server == "" || p.L2TP.Username == "" || p.L2TP.PasswordRef == "" {
			return errors.New("wan: L2TP requires server, username, password reference")
		}
	}
	return nil
}

// JSON encodes a Provider for durable storage.
func (p Provider) JSON() ([]byte, error) {
	if err := p.Valid(); err != nil {
		return nil, fmt.Errorf("wan: %w", err)
	}
	return json.Marshal(p)
}

// DecodeProvider parses a JSON-encoded
// Provider.
func DecodeProvider(raw []byte) (Provider, error) {
	var p Provider
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	if err := p.Valid(); err != nil {
		return p, err
	}
	return p, nil
}
