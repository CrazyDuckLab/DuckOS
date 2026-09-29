// ptpd/ptpd.go: V1 linuxptp configuration
// model and renderer. The package owns
// the typed contracts for ptp4l and
// phc2sys configurations, the
// deterministic renderer, the validator,
// and the timestamping-capability gate.
//
// DuckOS owns the intent. The runtime is
// a thin mapping to the linuxptp daemons.
package ptpd

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Timestamping is the timestamping class.
type Timestamping string

const (
	TSHardware Timestamping = "hardware"
	TSSoftware Timestamping = "software"
	TSNone     Timestamping = "none"
)

// Valid reports whether t is known.
func (t Timestamping) Valid() bool {
	switch t {
	case TSHardware, TSSoftware, TSNone:
		return true
	}
	return false
}

// Profile is the PTP profile.
type Profile string

const (
	ProfileDefault Profile = "default"
	ProfileHA      Profile = "ha"      // 802.3at / IEEE 1588 HA
	ProfilePower   Profile = "power"   // IEC 61850 9-3
	ProfileTelecom Profile = "telecom" // G.8275.1 / G.8275.2
)

// Valid reports whether p is known.
func (p Profile) Valid() bool {
	switch p {
	case ProfileDefault, ProfileHA, ProfilePower, ProfileTelecom:
		return true
	}
	return false
}

// Clock is a single PTP clock configuration.
type Clock struct {
	Interface string  `json:"interface"`
	Profile   Profile `json:"profile"`
	Domain    int     `json:"domain"`
	Priority1 int     `json:"priority1,omitempty"`
	Priority2 int     `json:"priority2,omitempty"`
	// MasterOnly pins the clock to master
	// role; absent means auto (BMCA).
	MasterOnly bool `json:"masterOnly,omitempty"`
	SlaveOnly  bool `json:"slaveOnly,omitempty"`
	TwoStep    bool `json:"twoStep,omitempty"`
	// UDP/IPv4 / UDP/IPv6 / L2 transport.
	Transport string `json:"transport,omitempty"`
	// FreeRun never adjusts the clock.
	FreeRun bool `json:"freeRun,omitempty"`
	// CapableHW is the detected hardware
	// timestamping capability.
	CapableHW Timestamping `json:"capableHw,omitempty"`
}

var ifnamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,15}$`)

// Valid reports whether c is well-formed.
func (c Clock) Valid() error {
	if !ifnamePattern.MatchString(c.Interface) {
		return errors.New("ptpd: interface is not canonical")
	}
	if !c.Profile.Valid() {
		return errors.New("ptpd: profile is unknown")
	}
	if c.Domain < 0 || c.Domain > 127 {
		return errors.New("ptpd: domain out of range")
	}
	if c.MasterOnly && c.SlaveOnly {
		return errors.New("ptpd: masterOnly and slaveOnly are mutually exclusive")
	}
	if c.Transport != "" {
		switch c.Transport {
		case "udp4", "udp6", "l2":
		default:
			return errors.New("ptpd: transport is unknown")
		}
	}
	if c.CapableHW != "" && !c.CapableHW.Valid() {
		return errors.New("ptpd: capableHw is invalid")
	}
	return nil
}

// Render produces the ptp4l.conf text.
// The output is deterministic.
func Render(c Clock) (string, error) {
	if err := c.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# DuckOS-generated ptp4l.conf\n")
	// Global defaults.
	fmt.Fprintf(&sb, "[global]\nverbose 1\n")
	// Profile-specific defaults.
	switch c.Profile {
	case ProfileDefault:
		sb.WriteString("gmCapable 1\n")
	case ProfileHA:
		sb.WriteString("gmCapable 1\nlogAnnounceInterval -3\n")
	case ProfilePower:
		sb.WriteString("network_transport L2\ndelay_mechanism P2P\n")
	case ProfileTelecom:
		// G.8275.x defaults are applied by
		// the operator-supplied .cfg file.
	}
	if c.Transport != "" {
		switch c.Transport {
		case "udp4":
			sb.WriteString("network_transport UDPv4\n")
		case "udp6":
			sb.WriteString("network_transport UDPv6\n")
		case "l2":
			sb.WriteString("network_transport L2\n")
		}
	}
	if c.MasterOnly {
		sb.WriteString("masterOnly 1\n")
	}
	if c.SlaveOnly {
		sb.WriteString("slaveOnly 1\n")
	}
	if c.TwoStep {
		sb.WriteString("twoStepFlag 1\n")
	}
	if c.Priority1 != 0 {
		fmt.Fprintf(&sb, "priority1 %d\n", c.Priority1)
	}
	if c.Priority2 != 0 {
		fmt.Fprintf(&sb, "priority2 %d\n", c.Priority2)
	}
	if c.FreeRun {
		sb.WriteString("free_running 1\n")
	}
	if c.Domain != 0 {
		fmt.Fprintf(&sb, "domainNumber %d\n", c.Domain)
	}
	switch c.CapableHW {
	case TSHardware:
		sb.WriteString("time_stamping hardware\n")
	case TSSoftware:
		sb.WriteString("time_stamping software\n")
	case TSNone:
		sb.WriteString("time_stamping software\n")
	}
	// Per-interface section.
	fmt.Fprintf(&sb, "\n[%s]\n", c.Interface)
	return sb.String(), nil
}

// PHCSys is a phc2sys configuration entry.
type PHCSys struct {
	Clock  string `json:"clock"`  // /dev/ptpN or CLOCK_REALTIME
	Source string `json:"source"` // /dev/ptpN or CLOCK_REALTIME
	// Pin the source to the system clock
	// for SLAVE/MASTER roles.
	Slave  bool `json:"slave,omitempty"`
	Master bool `json:"master,omitempty"`
	// ForceStep forces a step adjustment
	// when offset exceeds threshold.
	ForceStep bool `json:"forceStep,omitempty"`
	// StepThresholdNs is the offset in ns
	// above which a step is forced.
	StepThresholdNs int `json:"stepThresholdNs,omitempty"`
	// PiProportional / PiIntegral.
	PiProportional int `json:"piProportional,omitempty"`
	PiIntegral     int `json:"piIntegral,omitempty"`
}

// Valid reports whether p is well-formed.
func (p PHCSys) Valid() error {
	if p.Clock == "" {
		return errors.New("ptpd: phc2sys clock is required")
	}
	if p.Source == "" {
		return errors.New("ptpd: phc2sys source is required")
	}
	if p.Slave && p.Master {
		return errors.New("ptpd: phc2sys slave and master are mutually exclusive")
	}
	if p.StepThresholdNs < 0 {
		return errors.New("ptpd: step threshold must be non-negative")
	}
	return nil
}

// RenderPHCSys produces the phc2sys.conf text.
func RenderPHCSys(p PHCSys) (string, error) {
	if err := p.Valid(); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# DuckOS-generated phc2sys.conf\n")
	fmt.Fprintf(&sb, "clock %s\n", p.Clock)
	fmt.Fprintf(&sb, "source %s\n", p.Source)
	if p.Slave {
		sb.WriteString("slave\n")
	}
	if p.Master {
		sb.WriteString("master\n")
	}
	if p.ForceStep {
		sb.WriteString("force_step\n")
	}
	if p.StepThresholdNs > 0 {
		fmt.Fprintf(&sb, "step_threshold %d\n", p.StepThresholdNs)
	}
	if p.PiProportional > 0 {
		fmt.Fprintf(&sb, "pi_proportional %d\n", p.PiProportional)
	}
	if p.PiIntegral > 0 {
		fmt.Fprintf(&sb, "pi_integral %d\n", p.PiIntegral)
	}
	return sb.String(), nil
}

// Capability is the detected hardware
// timestamping capability of an
// interface.
type Capability struct {
	Interface  string       `json:"interface"`
	Capable    Timestamping `json:"capable"`
	DetectedAt time.Time    `json:"detectedAt"`
}

// Manager is the PTP capability store.
type Manager struct {
	clocks       []Clock
	capabilities []Capability
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{}
}

// SetClocks replaces the clock list.
func (m *Manager) SetClocks(cs []Clock) error {
	for _, c := range cs {
		if err := c.Valid(); err != nil {
			return err
		}
	}
	m.clocks = cs
	return nil
}

// Clocks returns the sorted clock list.
func (m *Manager) Clocks() []Clock {
	out := append([]Clock(nil), m.clocks...)
	sort.Slice(out, func(i, k int) bool { return out[i].Interface < out[k].Interface })
	return out
}

// SetCapability records a detected
// capability.
func (m *Manager) SetCapability(c Capability) {
	if c.Capable == "" {
		return
	}
	if c.DetectedAt.IsZero() {
		c.DetectedAt = time.Now().UTC()
	}
	for i, x := range m.capabilities {
		if x.Interface == c.Interface {
			m.capabilities[i] = c
			return
		}
	}
	m.capabilities = append(m.capabilities, c)
}

// Capabilities returns the sorted list.
func (m *Manager) Capabilities() []Capability {
	out := append([]Capability(nil), m.capabilities...)
	sort.Slice(out, func(i, k int) bool { return out[i].Interface < out[k].Interface })
	return out
}
