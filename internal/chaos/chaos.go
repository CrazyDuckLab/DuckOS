// chaos/chaos.go: V1 dev/alpha chaos +
// readiness + telemetry privacy + crash
// evidence + perf/IRQ + NTS + TR-369 +
// clone protection + SSH keys + console
// recovery surface.
package chaos

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// Readiness is the per-feature
// readiness.
type Readiness string

const (
	ReadySupported    Readiness = "supported"
	ReadyExperimental Readiness = "experimental"
	ReadyHWPending    Readiness = "hardware-pending"
	ReadyCredPending  Readiness = "credential-pending"
	ReadyUnsupported  Readiness = "unsupported"
	ReadyDegraded     Readiness = "degraded"
)

// Valid reports whether r is known.
func (r Readiness) Valid() bool {
	switch r {
	case ReadySupported, ReadyExperimental, ReadyHWPending,
		ReadyCredPending, ReadyUnsupported, ReadyDegraded:
		return true
	}
	return false
}

// FeatureReadiness is one feature
// record.
type FeatureReadiness struct {
	Feature   string    `json:"feature"`
	State     Readiness `json:"state"`
	Note      string    `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ReadinessStore is the feature readiness
// store.
type ReadinessStore struct {
	mu sync.Mutex
	r  map[string]FeatureReadiness
}

// NewReadinessStore returns an empty
// store.
func NewReadinessStore() *ReadinessStore {
	return &ReadinessStore{r: map[string]FeatureReadiness{}}
}

// Set records a feature readiness.
func (s *ReadinessStore) Set(f FeatureReadiness) {
	if f.Feature == "" || !f.State.Valid() {
		return
	}
	f.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[f.Feature] = f
}

// Get returns a feature readiness.
func (s *ReadinessStore) Get(feature string) (FeatureReadiness, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.r[feature]
	return r, ok
}

// All returns every feature sorted.
func (s *ReadinessStore) All() []FeatureReadiness {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FeatureReadiness, 0, len(s.r))
	for _, r := range s.r {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Feature < out[k].Feature })
	return out
}

// Fault is a chaos / fault-injection
// declaration.
type Fault struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "service-kill" | "link-down" | "packet-loss" | "packet-delay" | "dns-failure" | "gateway-failure" | "disk-full" | "read-only" | "time-untrusted" | "corrupt-candidate" | "update-interrupt" | "daemon-crash"
	Target string `json:"target"`
	// Magnitude is the per-fault
	// magnitude (loss %, delay ms,
	// bytes freed, etc.).
	Magnitude string `json:"magnitude,omitempty"`
	// Isolated is true when the fault is
	// isolated from production.
	Isolated bool `json:"isolated,omitempty"`
}

// Valid reports whether f is well-formed.
func (f Fault) Valid() error {
	if f.ID == "" || f.Kind == "" || f.Target == "" {
		return errors.New("chaos: fault id+kind+target required")
	}
	if !f.Isolated {
		return errors.New("chaos: fault must be isolated from production")
	}
	return nil
}

// FaultManager is the fault store.
type FaultManager struct {
	mu     sync.Mutex
	faults map[string]Fault
}

// NewFaultManager returns an empty store.
func NewFaultManager() *FaultManager {
	return &FaultManager{faults: map[string]Fault{}}
}

// Add inserts a fault.
func (m *FaultManager) Add(f Fault) error {
	if err := f.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faults[f.ID] = f
	return nil
}

// All returns every fault sorted.
func (m *FaultManager) All() []Fault {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Fault, 0, len(m.faults))
	for _, f := range m.faults {
		out = append(out, f)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// TelemetryPref is the beta-telemetry
// privacy preference.
type TelemetryPref struct {
	Enabled   bool   `json:"enabled"`
	UserAgent string `json:"userAgent,omitempty"`
	// CrashReports toggles crash
	// reporting.
	CrashReports bool `json:"crashReports"`
	// CompatReports toggles compatibility
	// reporting.
	CompatReports bool `json:"compatReports"`
	// AllowedEndpoints is the
	// allow-list of telemetry endpoints.
	AllowedEndpoints []string  `json:"allowedEndpoints,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Valid reports whether p is well-formed.
func (p TelemetryPref) Valid() error {
	// Even when enabled, secrets are
	// forbidden in userAgent.
	if p.UserAgent != "" {
		for _, ch := range p.UserAgent {
			if ch == ' ' {
				return errors.New("chaos: userAgent must not contain spaces")
			}
		}
	}
	return nil
}

// TelemetryStore is the telemetry
// preference store.
type TelemetryStore struct {
	mu sync.Mutex
	p  TelemetryPref
}

// NewTelemetryStore returns an empty
// store.
func NewTelemetryStore() *TelemetryStore {
	return &TelemetryStore{p: TelemetryPref{Enabled: false, CrashReports: false, CompatReports: false}}
}

// Set replaces the preference.
func (s *TelemetryStore) Set(p TelemetryPref) error {
	if err := p.Valid(); err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.p = p
	return nil
}

// Get returns a copy.
func (s *TelemetryStore) Get() TelemetryPref {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p
}

// CrashEvidence is one crash record.
type CrashEvidence struct {
	ID            string    `json:"id"`
	Component     string    `json:"component"`
	Version       string    `json:"version"`
	Reason        string    `json:"reason"`
	At            time.Time `json:"at"`
	RecentEvents  []string  `json:"recentEvents,omitempty"`
	ResourceState string    `json:"resourceState,omitempty"`
	ActiveTxn     string    `json:"activeTxn,omitempty"`
	// Candidate is the candidate release
	// id active at crash time.
	Candidate string `json:"candidate,omitempty"`
}

// CrashStore is the crash-evidence store.
type CrashStore struct {
	mu      sync.Mutex
	crashes []CrashEvidence
	// MaxHistory caps the retention.
	MaxHistory int
}

// NewCrashStore returns an empty store.
func NewCrashStore() *CrashStore {
	return &CrashStore{MaxHistory: 64}
}

// Record appends a crash record.
func (s *CrashStore) Record(c CrashEvidence) error {
	if c.ID == "" {
		return errors.New("chaos: crash id required")
	}
	if c.At.IsZero() {
		c.At = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.crashes = append(s.crashes, c)
	if s.MaxHistory > 0 && len(s.crashes) > s.MaxHistory {
		s.crashes = s.crashes[len(s.crashes)-s.MaxHistory:]
	}
	return nil
}

// All returns every crash sorted newest
// first.
func (s *CrashStore) All() []CrashEvidence {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CrashEvidence, len(s.crashes))
	copy(out, s.crashes)
	sort.Slice(out, func(i, k int) bool { return out[i].At.After(out[k].At) })
	return out
}

// IRQCPUPerf is the IRQ / CPU / NUMA
// performance record.
type IRQCPUPerf struct {
	IRQAffinity  map[string]string `json:"irqAffinity,omitempty"`
	RSS          map[string]string `json:"rss,omitempty"`
	RPS          map[string]string `json:"rps,omitempty"`
	XPS          map[string]string `json:"xps,omitempty"`
	CPUAffinity  map[string]string `json:"cpuAffinity,omitempty"`
	NUMALocality map[string]string `json:"numaLocality,omitempty"`
	// AutoTune toggles automatic
	// capability-aware tuning.
	AutoTune bool `json:"autoTune,omitempty"`
	// Reversible ensures the operator
	// can roll back the tuning.
	Reversible bool `json:"reversible"`
}

// PerfStore is the perf store.
type PerfStore struct {
	mu sync.Mutex
	r  IRQCPUPerf
}

// NewPerfStore returns an empty store.
func NewPerfStore() *PerfStore { return &PerfStore{r: IRQCPUPerf{Reversible: true}} }

// Set replaces the perf record.
func (s *PerfStore) Set(p IRQCPUPerf) {
	p.Reversible = true
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r = p
}

// Get returns a copy.
func (s *PerfStore) Get() IRQCPUPerf {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r
}

// NTSState is the Network Time Security
// state.
type NTSState struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server,omitempty"`
	// Authenticated toggles NTS
	// authentication.
	Authenticated bool `json:"authenticated"`
	// LastSync is the last successful
	// authenticated sync.
	LastSync time.Time `json:"lastSync,omitempty"`
	// Quality is the per-sync quality.
	Quality string `json:"quality,omitempty"`
}

// NTSStore is the NTS store.
type NTSStore struct {
	mu sync.Mutex
	s  NTSState
}

// NewNTSStore returns an empty store.
func NewNTSStore() *NTSStore { return &NTSStore{} }

// Set records the NTS state.
func (s *NTSStore) Set(st NTSState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s = st
}

// Get returns a copy.
func (s *NTSStore) Get() NTSState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s
}

// USPState is the TR-369 / USP optional
// CPE state.
type USPState struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
	// AuthMethod is "mtls" or "psk".
	AuthMethod string `json:"authMethod,omitempty"`
	// Audited toggles audit logging.
	Audited bool `json:"audited,omitempty"`
	// LocalFirst toggles local-first
	// behaviour when USP is offline.
	LocalFirst bool `json:"localFirst"`
}

// Valid reports whether u is well-formed.
func (u USPState) Valid() error {
	if !u.Enabled {
		return nil
	}
	if u.Endpoint == "" {
		return errors.New("chaos: USP endpoint required when enabled")
	}
	if u.AuthMethod != "mtls" && u.AuthMethod != "psk" {
		return errors.New("chaos: USP auth method must be mtls or psk")
	}
	if !u.LocalFirst {
		return errors.New("chaos: USP must remain local-first when disabled or unreachable")
	}
	return nil
}

// USPStore is the USP store.
type USPStore struct {
	mu sync.Mutex
	s  USPState
}

// NewUSPStore returns an empty store.
func NewUSPStore() *USPStore { return &USPStore{} }

// Set records the USP state.
func (s *USPStore) Set(u USPState) error {
	if err := u.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.s = u
	return nil
}

// Get returns a copy.
func (s *USPStore) Get() USPState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s
}

// SSHHostKey is the SSH host-key
// lifecycle record.
type SSHHostKey struct {
	Type        string    `json:"type"` // "ed25519" | "rsa" | "ecdsa"
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"createdAt"`
	RotatedFrom string    `json:"rotatedFrom,omitempty"`
}

// SSHKeyStore is the SSH key store.
type SSHKeyStore struct {
	mu   sync.Mutex
	keys map[string]SSHHostKey
}

// NewSSHKeyStore returns an empty store.
func NewSSHKeyStore() *SSHKeyStore { return &SSHKeyStore{keys: map[string]SSHHostKey{}} }

// Set records a key.
func (s *SSHKeyStore) Set(k SSHHostKey) {
	if k.Type == "" || k.Fingerprint == "" {
		return
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[k.Type] = k
}

// Get returns a key.
func (s *SSHKeyStore) Get(t string) (SSHHostKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[t]
	return k, ok
}

// All returns every key.
func (s *SSHKeyStore) All() []SSHHostKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SSHHostKey, 0, len(s.keys))
	for _, k := range s.keys {
		out = append(out, k)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Type < out[k].Type })
	return out
}

// CloneProtection is the per-class
// identity policy.
type CloneProtection struct {
	Class             string `json:"class"` // "machine-id" | "ssh-host-key" | "instance-id" | "one-time-token" | "enrollment-id"
	RegenerateOnClone bool   `json:"regenerateOnClone"`
	// Portable toggles whether the
	// object survives a clone.
	Portable bool `json:"portable,omitempty"`
	// Description explains the class.
	Description string `json:"description,omitempty"`
}

// CloneStore is the clone-protection
// store.
type CloneStore struct {
	mu sync.Mutex
	r  map[string]CloneProtection
}

// NewCloneStore returns an empty store.
func NewCloneStore() *CloneStore { return &CloneStore{r: map[string]CloneProtection{}} }

// Set records a class.
func (s *CloneStore) Set(c CloneProtection) {
	if c.Class == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.r[c.Class] = c
}

// Get returns a class.
func (s *CloneStore) Get(class string) (CloneProtection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.r[class]
	return c, ok
}

// All returns every class.
func (s *CloneStore) All() []CloneProtection {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]CloneProtection, 0, len(s.r))
	for _, c := range s.r {
		out = append(out, c)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Class < out[k].Class })
	return out
}

// ConsoleCommand is one serial/console
// recovery command.
type ConsoleCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Mutating toggles whether the
	// command changes system state.
	Mutating bool `json:"mutating"`
	// Allowed toggles whether the
	// command is allowed in the
	// console UI.
	Allowed bool `json:"allowed"`
}

var cmdPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// Valid reports whether c is well-formed.
func (c ConsoleCommand) Valid() error {
	if !cmdPattern.MatchString(c.Name) {
		return errors.New("chaos: command name is not canonical")
	}
	if c.Description == "" {
		return errors.New("chaos: command description is required")
	}
	return nil
}

// ConsoleCatalog is the console
// command catalog.
type ConsoleCatalog struct {
	mu   sync.Mutex
	cmds map[string]ConsoleCommand
}

// NewConsoleCatalog returns an empty
// store.
func NewConsoleCatalog() *ConsoleCatalog {
	return &ConsoleCatalog{cmds: map[string]ConsoleCommand{}}
}

// Register inserts a command.
func (c *ConsoleCatalog) Register(cmd ConsoleCommand) error {
	if err := cmd.Valid(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cmds[cmd.Name] = cmd
	return nil
}

// All returns every command sorted.
func (c *ConsoleCatalog) All() []ConsoleCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ConsoleCommand, 0, len(c.cmds))
	for _, cmd := range c.cmds {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}
