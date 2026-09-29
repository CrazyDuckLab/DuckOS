// hardware/hardware.go: V1 hardware
// abstraction package. It owns the typed
// contracts for LLDP advertisement, NIC
// capability detection, PoE control,
// hardware-offload reporting, RADIUS
// server reference, USB modem summary, and
// UPS/NUT summary. The runtime is a thin
// mapping to lldpd, ethtool, the PoE
// driver, hostapd, freeradius, ModemManager,
// and the NUT client.
//
// The V1 contract is: every typed object
// reports its capability as a typed value
// (true/false/error) so the future UI can
// surface "supported" vs "not detected" vs
// "explicitly disabled" honestly.
package hardware

import (
	"errors"
	"regexp"
	"sort"
	"sync"
)

// LLDPMode is the LLDP advertisement mode.
type LLDPMode string

const (
	LLDPRxOnly LLDPMode = "rx-only"
	LLDPTxRx   LLDPMode = "tx-rx"
	LLDPOff    LLDPMode = "off"
)

// Valid reports whether m is known.
func (m LLDPMode) Valid() bool {
	switch m {
	case LLDPRxOnly, LLDPTxRx, LLDPOff:
		return true
	}
	return false
}

// LLDPSettings is the per-link LLDP config.
type LLDPSettings struct {
	Link        string   `json:"link"`
	Mode        LLDPMode `json:"mode"`
	SystemName  string   `json:"systemName,omitempty"`
	ChassisID   string   `json:"chassisId,omitempty"`
	PortDescr   string   `json:"portDescr,omitempty"`
	VLANEnabled bool     `json:"vlanEnabled,omitempty"`
}

var linkPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether l is well-formed.
func (l LLDPSettings) Valid() error {
	if !linkPattern.MatchString(l.Link) {
		return errors.New("hardware: link is not canonical")
	}
	if !l.Mode.Valid() {
		return errors.New("hardware: LLDP mode is unknown")
	}
	return nil
}

// NICCapabilities is the typed NIC capability
// report.
type NICCapabilities struct {
	Link      string       `json:"link"`
	SpeedMbps int          `json:"speedMbps"`
	Offloads  OffloadState `json:"offloads"`
	// SFP is the SFP/SFP+ module telemetry
	// when present.
	SFP *SFPInfo `json:"sfp,omitempty"`
	// Flaps counts link-state changes since
	// boot.
	Flaps int `json:"flaps"`
	// CRCErrors is the running counter.
	CRCErrors uint64 `json:"crcErrors"`
}

// OffloadState is the per-offload enable bit.
type OffloadState struct {
	ChecksumOffload bool `json:"checksumOffload"`
	TSO             bool `json:"tso"`
	UO              bool `json:"uo"`
	LRO             bool `json:"lro"`
	RxVLANOffload   bool `json:"rxVlanOffload"`
	TxVLANOffload   bool `json:"txVlanOffload"`
	RxHash          bool `json:"rxHash"`
	FlowDirector    bool `json:"flowDirector"`
}

// SFPInfo is the SFP/SFP+ telemetry.
type SFPInfo struct {
	Vendor     string  `json:"vendor,omitempty"`
	Model      string  `json:"model,omitempty"`
	Serial     string  `json:"serial,omitempty"`
	Wavelength int     `json:"wavelengthNm,omitempty"`
	RXPower    float64 `json:"rxPowerDbm,omitempty"`
	TXPower    float64 `json:"txPowerDbm,omitempty"`
	TempC      float64 `json:"tempC,omitempty"`
}

// PoEClass is the IEEE 802.3 PoE class.
type PoEClass int

const (
	PoEClass0 PoEClass = 0
	PoEClass1 PoEClass = 1
	PoEClass2 PoEClass = 2
	PoEClass3 PoEClass = 3
	PoEClass4 PoEClass = 4
	PoEClass5 PoEClass = 5
	PoEClass6 PoEClass = 6
	PoEClass7 PoEClass = 7
	PoEClass8 PoEClass = 8
)

// PoEState is the per-port PoE state.
type PoEState struct {
	Port       string   `json:"port"`
	Enabled    bool     `json:"enabled"`
	Delivering bool     `json:"delivering"`
	Class      PoEClass `json:"class"`
	PowerW     float64  `json:"powerW"`
	BudgetW    float64  `json:"budgetW"`
	Failure    string   `json:"failure,omitempty"`
}

// Valid reports whether p is well-formed.
func (p PoEState) Valid() error {
	if !linkPattern.MatchString(p.Port) {
		return errors.New("hardware: port is not canonical")
	}
	return nil
}

// PoEWatchdog is a per-port bounded power-
// cycle policy. Watchdogs must never create
// reboot loops.
type PoEWatchdog struct {
	Port        string `json:"port"`
	ProbeTarget string `json:"probeTarget"` // address/host to probe
	ProbeMethod string `json:"probeMethod"` // "icmp" | "tcp"
	MaxCycles   int    `json:"maxCyclesPerHour"`
	CooldownSec int    `json:"cooldownSec"`
	Enabled     bool   `json:"enabled"`
}

// Valid reports whether w is well-formed.
func (w PoEWatchdog) Valid() error {
	if !linkPattern.MatchString(w.Port) {
		return errors.New("hardware: port is not canonical")
	}
	if w.ProbeTarget == "" {
		return errors.New("hardware: probe target is required")
	}
	if w.MaxCycles < 0 || w.MaxCycles > 60 {
		return errors.New("hardware: maxCyclesPerHour must be 0..60")
	}
	if w.CooldownSec < 0 {
		return errors.New("hardware: cooldown must be non-negative")
	}
	return nil
}

// RADIUSServer is an external RADIUS server
// reference.
type RADIUSServer struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	AuthPort  int    `json:"authPort"`
	AcctPort  int    `json:"acctPort"`
	SecretRef string `json:"secretRef"` // vault ref
}

// Valid reports whether r is well-formed.
func (r RADIUSServer) Valid() error {
	if r.Host == "" {
		return errors.New("hardware: host is required")
	}
	if r.SecretRef == "" {
		return errors.New("hardware: secret must come from the vault")
	}
	if r.AuthPort < 1 || r.AuthPort > 65535 {
		return errors.New("hardware: authPort out of range")
	}
	if r.AcctPort < 1 || r.AcctPort > 65535 {
		return errors.New("hardware: acctPort out of range")
	}
	return nil
}

// ModemSummary is the USB modem summary.
type ModemSummary struct {
	Path   string `json:"path"`
	Vendor string `json:"vendor,omitempty"`
	Model  string `json:"model,omitempty"`
	// Signal is the modem signal strength
	// (0..100).
	Signal     int    `json:"signal"`
	Technology string `json:"technology,omitempty"` // "LTE" | "5G" | "3G"
	Operator   string `json:"operator,omitempty"`
	IP         string `json:"ip,omitempty"`
	State      string `json:"state,omitempty"`
}

// UPSSummary is the NUT surface.
type UPSSummary struct {
	Name          string  `json:"name"`
	Host          string  `json:"host,omitempty"`
	Model         string  `json:"model,omitempty"`
	BatteryPct    float64 `json:"batteryPct"`
	LoadPct       float64 `json:"loadPct"`
	RuntimeSec    int     `json:"runtimeSec"`
	InputVoltage  float64 `json:"inputVoltage,omitempty"`
	OutputVoltage float64 `json:"outputVoltage,omitempty"`
	State         string  `json:"state"` // "online" | "on-battery" | "low-battery" | "charging"
}

// Capability is the V1 capability surface.
type Capability struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

// Manager is the V1 hardware store. Manager is
// goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	lldp   map[string]LLDPSettings
	nic    map[string]NICCapabilities
	poe    map[string]PoEState
	poeWd  map[string]PoEWatchdog
	radius map[string]RADIUSServer
	modems map[string]ModemSummary
	ups    map[string]UPSSummary
	caps   []Capability
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{lldp: map[string]LLDPSettings{}, nic: map[string]NICCapabilities{}, poe: map[string]PoEState{}, poeWd: map[string]PoEWatchdog{}, radius: map[string]RADIUSServer{}, modems: map[string]ModemSummary{}, ups: map[string]UPSSummary{}}
}

// SetLLDP replaces or inserts an LLDP setting.
func (m *Manager) SetLLDP(s LLDPSettings) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lldp[s.Link] = s
	return nil
}

// SetNIC replaces or inserts a NIC capability.
func (m *Manager) SetNIC(n NICCapabilities) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nic[n.Link] = n
}

// SetPoEState replaces or inserts a PoE state.
func (m *Manager) SetPoEState(p PoEState) error {
	if err := p.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.poe[p.Port] = p
	return nil
}

// SetPoEWatchdog replaces or inserts a PoE
// watchdog policy.
func (m *Manager) SetPoEWatchdog(w PoEWatchdog) error {
	if err := w.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.poeWd[w.Port] = w
	return nil
}

// SetRADIUS replaces or inserts a RADIUS server.
func (m *Manager) SetRADIUS(r RADIUSServer) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.radius[r.Name] = r
	return nil
}

// SetModem replaces or inserts a modem summary.
func (m *Manager) SetModem(m2 ModemSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modems[m2.Path] = m2
}

// SetUPS replaces or inserts a UPS summary.
func (m *Manager) SetUPS(u UPSSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ups[u.Name] = u
}

// SetCapabilities replaces the capability
// list.
func (m *Manager) SetCapabilities(caps []Capability) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.caps = append([]Capability(nil), caps...)
}

// Snapshot returns a copy of the entire
// hardware surface.
type Snapshot struct {
	LLDP         []LLDPSettings    `json:"lldp"`
	NIC          []NICCapabilities `json:"nic"`
	PoE          []PoEState        `json:"poe"`
	PoEWatchdogs []PoEWatchdog     `json:"poeWatchdogs"`
	RADIUS       []RADIUSServer    `json:"radius"`
	Modems       []ModemSummary    `json:"modems"`
	UPS          []UPSSummary      `json:"ups"`
	Capabilities []Capability      `json:"capabilities"`
}

// Snapshot returns the current snapshot.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Snapshot{}
	for _, v := range m.lldp {
		out.LLDP = append(out.LLDP, v)
	}
	sort.Slice(out.LLDP, func(i, j int) bool { return out.LLDP[i].Link < out.LLDP[j].Link })
	for _, v := range m.nic {
		out.NIC = append(out.NIC, v)
	}
	sort.Slice(out.NIC, func(i, j int) bool { return out.NIC[i].Link < out.NIC[j].Link })
	for _, v := range m.poe {
		out.PoE = append(out.PoE, v)
	}
	sort.Slice(out.PoE, func(i, j int) bool { return out.PoE[i].Port < out.PoE[j].Port })
	for _, v := range m.poeWd {
		out.PoEWatchdogs = append(out.PoEWatchdogs, v)
	}
	sort.Slice(out.PoEWatchdogs, func(i, j int) bool { return out.PoEWatchdogs[i].Port < out.PoEWatchdogs[j].Port })
	for _, v := range m.radius {
		out.RADIUS = append(out.RADIUS, v)
	}
	sort.Slice(out.RADIUS, func(i, j int) bool { return out.RADIUS[i].Name < out.RADIUS[j].Name })
	for _, v := range m.modems {
		out.Modems = append(out.Modems, v)
	}
	sort.Slice(out.Modems, func(i, j int) bool { return out.Modems[i].Path < out.Modems[j].Path })
	for _, v := range m.ups {
		out.UPS = append(out.UPS, v)
	}
	sort.Slice(out.UPS, func(i, j int) bool { return out.UPS[i].Name < out.UPS[j].Name })
	out.Capabilities = append([]Capability(nil), m.caps...)
	return out
}
