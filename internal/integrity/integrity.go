// integrity/integrity.go: V1 platform
// integrity + lifecycle surface. The
// package groups secure boot, TPM, system
// integrity, SBOM, kernel/driver/firmware
// lifecycle, hardware health, and storage
// operations.
package integrity

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SecureBootState is the platform secure
// boot state.
type SecureBootState string

const (
	SecureBootSupported   SecureBootState = "supported"
	SecureBootActive      SecureBootState = "active"
	SecureBootDegraded    SecureBootState = "degraded"
	SecureBootUnsupported SecureBootState = "unsupported"
)

// Valid reports whether s is known.
func (s SecureBootState) Valid() bool {
	switch s {
	case SecureBootSupported, SecureBootActive, SecureBootDegraded, SecureBootUnsupported:
		return true
	}
	return false
}

// TPMState is the TPM / measured-boot
// state.
type TPMState struct {
	Present   bool      `json:"present"`
	PCR0      string    `json:"pcr0,omitempty"`
	PCR7      string    `json:"pcr7,omitempty"`
	Sealed    bool      `json:"sealed"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Integrity is the platform integrity
// record.
type Integrity struct {
	SecureBoot SecureBootState `json:"secureBoot"`
	TPM        TPMState        `json:"tpm"`
	// ReleaseHash is the expected hash
	// of the immutable release payload.
	ReleaseHash string `json:"releaseHash"`
	// ObservedHash is the runtime hash.
	ObservedHash string `json:"observedHash"`
	// Verified is true when the hashes
	// match.
	Verified  bool      `json:"verified"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Valid reports whether i is well-formed.
func (i Integrity) Valid() error {
	if !i.SecureBoot.Valid() {
		return errors.New("integrity: secureBoot state is invalid")
	}
	return nil
}

// IntegrityStore is the integrity store.
type IntegrityStore struct {
	mu sync.Mutex
	i  Integrity
}

// NewIntegrityStore returns an empty
// store.
func NewIntegrityStore() *IntegrityStore { return &IntegrityStore{} }

// Set replaces the record.
func (s *IntegrityStore) Set(i Integrity) error {
	if err := i.Valid(); err != nil {
		return err
	}
	i.UpdatedAt = time.Now().UTC()
	i.Verified = i.ReleaseHash != "" && i.ReleaseHash == i.ObservedHash
	s.mu.Lock()
	defer s.mu.Unlock()
	s.i = i
	return nil
}

// Get returns a copy.
func (s *IntegrityStore) Get() Integrity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.i
}

// SBOMComponent is one entry in the
// release SBOM.
type SBOMComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// License is informational.
	License string `json:"license,omitempty"`
	// Origin describes the source.
	Origin string `json:"origin,omitempty"`
	// Hash is the artifact hash.
	Hash string `json:"hash,omitempty"`
}

var sbomNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:+@-]{0,127}$`)

// Valid reports whether c is well-formed.
func (c SBOMComponent) Valid() error {
	if !sbomNamePattern.MatchString(c.Name) {
		return errors.New("integrity: SBOM name is not canonical")
	}
	if c.Version == "" {
		return errors.New("integrity: SBOM version is required")
	}
	return nil
}

// SBOMStore is the SBOM store.
type SBOMStore struct {
	mu  sync.Mutex
	cmp map[string]SBOMComponent
}

// NewSBOMStore returns an empty store.
func NewSBOMStore() *SBOMStore {
	return &SBOMStore{cmp: map[string]SBOMComponent{}}
}

// Add inserts a component.
func (s *SBOMStore) Add(c SBOMComponent) error {
	if err := c.Valid(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cmp[c.Name+":"+c.Version] = c
	return nil
}

// All returns every component sorted.
func (s *SBOMStore) All() []SBOMComponent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SBOMComponent, 0, len(s.cmp))
	for _, c := range s.cmp {
		out = append(out, c)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// KernelModule is one kernel/driver
// record.
type KernelModule struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Required toggles the strict
	// requirement: a missing module
	// aborts the candidate release.
	Required bool `json:"required"`
	// Detected is the runtime presence.
	Detected bool `json:"detected"`
}

// Firmware is one firmware record.
type Firmware struct {
	Device  string `json:"device"`
	Vendor  string `json:"vendor,omitempty"`
	Version string `json:"version"`
	// Supported is the per-platform
	// support flag.
	Supported bool `json:"supported"`
}

// KernelFWStore is the kernel/firmware
// store.
type KernelFWStore struct {
	mu       sync.Mutex
	modules  map[string]KernelModule
	firmware map[string]Firmware
}

// NewKernelFWStore returns an empty store.
func NewKernelFWStore() *KernelFWStore {
	return &KernelFWStore{modules: map[string]KernelModule{}, firmware: map[string]Firmware{}}
}

// SetModule inserts a module.
func (s *KernelFWStore) SetModule(m KernelModule) {
	if m.Name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules[m.Name] = m
}

// SetFirmware inserts a firmware record.
func (s *KernelFWStore) SetFirmware(f Firmware) {
	if f.Device == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.firmware[f.Device] = f
}

// MissingRequired returns the list of
// required modules that are not currently
// detected.
func (s *KernelFWStore) MissingRequired() []KernelModule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []KernelModule{}
	for _, m := range s.modules {
		if m.Required && !m.Detected {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// Modules returns every module sorted.
func (s *KernelFWStore) Modules() []KernelModule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]KernelModule, 0, len(s.modules))
	for _, m := range s.modules {
		out = append(out, m)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}

// FirmwareAll returns every firmware
// record.
func (s *KernelFWStore) FirmwareAll() []Firmware {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Firmware, 0, len(s.firmware))
	for _, f := range s.firmware {
		out = append(out, f)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Device < out[k].Device })
	return out
}

// Temp is a temperature reading.
type Temp struct {
	Sensor string  `json:"sensor"`
	C      float64 `json:"c"`
}

// Fan is a fan reading.
type Fan struct {
	Sensor string `json:"sensor"`
	RPM    int    `json:"rpm"`
}

// SMART is the SMART health record.
type SMART struct {
	Device      string `json:"device"`
	Health      string `json:"health"` // "ok" | "degraded" | "failing"
	TempC       int    `json:"tempC"`
	WearPct     int    `json:"wearPct,omitempty"`
	Reallocated int    `json:"reallocated,omitempty"`
}

// HWHealth is the per-component health
// store.
type HWHealth struct {
	mu    sync.Mutex
	temps []Temp
	fans  []Fan
	smart map[string]SMART
}

// NewHWHealth returns an empty store.
func NewHWHealth() *HWHealth {
	return &HWHealth{smart: map[string]SMART{}}
}

// ObserveTemp records a temperature.
func (h *HWHealth) ObserveTemp(t Temp) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.temps = append(h.temps, t)
}

// ObserveFan records a fan reading.
func (h *HWHealth) ObserveFan(f Fan) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fans = append(h.fans, f)
}

// SetSMART records the SMART state.
func (h *HWHealth) SetSMART(s SMART) {
	if s.Device == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.smart[s.Device] = s
}

// Temps returns the recent temperatures.
func (h *HWHealth) Temps() []Temp {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Temp(nil), h.temps...)
}

// Fans returns the fan readings.
func (h *HWHealth) Fans() []Fan {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Fan(nil), h.fans...)
}

// SMARTAll returns every SMART record.
func (h *HWHealth) SMARTAll() []SMART {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]SMART, 0, len(h.smart))
	for _, s := range h.smart {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Device < out[k].Device })
	return out
}

// Disk is a single storage device.
type Disk struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes"`
	FSType     string `json:"fsType,omitempty"`
	MountPoint string `json:"mountPoint,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
}

// RAID is one RAID array.
type RAID struct {
	Name    string   `json:"name"`
	Level   string   `json:"level"`
	State   string   `json:"state"` // "ok" | "degraded" | "rebuilding" | "failed"
	Members []string `json:"members,omitempty"`
}

// StorageStore is the storage store.
type StorageStore struct {
	mu    sync.Mutex
	disks map[string]Disk
	raids map[string]RAID
}

// NewStorageStore returns an empty store.
func NewStorageStore() *StorageStore {
	return &StorageStore{disks: map[string]Disk{}, raids: map[string]RAID{}}
}

// SetDisk inserts a disk.
func (s *StorageStore) SetDisk(d Disk) {
	if d.Path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disks[d.Path] = d
}

// SetRAID inserts a RAID.
func (s *StorageStore) SetRAID(r RAID) {
	if r.Name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raids[r.Name] = r
}

// Disks returns every disk.
func (s *StorageStore) Disks() []Disk {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Disk, 0, len(s.disks))
	for _, d := range s.disks {
		out = append(out, d)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Path < out[k].Path })
	return out
}

// RAIDs returns every RAID.
func (s *StorageStore) RAIDs() []RAID {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RAID, 0, len(s.raids))
	for _, r := range s.raids {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Name < out[k].Name })
	return out
}
