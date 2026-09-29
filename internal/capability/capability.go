// capability/capability.go: V1 hardware
// capability detection surface. The
// package owns the typed contract for
// per-feature detection and the
// "unsupported" action reporting.
//
// The runtime is a thin mapping to the
// existing internal/hardware manager and
// to runtime probes (ethtool, lsmod,
// lspci, virt-what, modetest).
package capability

import (
	"errors"
	"sort"
	"sync"
)

// Feature is the canonical V1 feature key.
type Feature string

const (
	FeatureNICOffload     Feature = "nic-offload"
	FeatureWiFi           Feature = "wifi"
	FeatureSFP            Feature = "sfp"
	FeaturePoE            Feature = "poe"
	FeatureModem          Feature = "modem"
	FeatureStorage        Feature = "storage"
	FeatureVirtualization Feature = "virtualization"
	FeatureUSB            Feature = "usb"
	FeatureBluetooth      Feature = "bluetooth"
	FeatureTPM            Feature = "tpm"
	FeatureGPS            Feature = "gps"
	FeatureCrypto         Feature = "hardware-crypto"
	FeatureNTP            Feature = "ntp"
	FeaturePTP            Feature = "ptp"
	FeatureUPS            Feature = "ups"
)

// AllFeatures returns every canonical
// feature key. The list is sorted for
// stable output.
func AllFeatures() []Feature {
	return []Feature{
		FeatureBluetooth, FeatureCrypto, FeatureGPS, FeatureModem,
		FeatureNICOffload, FeatureNTP, FeaturePTP, FeaturePoE,
		FeatureSFP, FeatureStorage, FeatureTPM, FeatureUPS,
		FeatureUSB, FeatureVirtualization, FeatureWiFi,
	}
}

// Status is the per-feature status.
type Status string

const (
	// StatusSupported means the feature is
	// present and operational.
	StatusSupported Status = "supported"
	// StatusUnsupported means the feature
	// is absent on this hardware.
	StatusUnsupported Status = "unsupported"
	// StatusDisabled means the feature is
	// present but the operator disabled it.
	StatusDisabled Status = "disabled"
	// StatusUnknown means the runtime could
	// not determine the feature's state.
	StatusUnknown Status = "unknown"
)

// Valid reports whether s is known.
func (s Status) Valid() bool {
	switch s {
	case StatusSupported, StatusUnsupported, StatusDisabled, StatusUnknown:
		return true
	}
	return false
}

// Report is the per-feature detection
// result.
type Report struct {
	Feature Feature `json:"feature"`
	Status  Status  `json:"status"`
	Detail  string  `json:"detail,omitempty"`
	Source  string  `json:"source,omitempty"`
}

// Probe is the runtime hook the manager
// invokes to detect a feature. The hook
// returns a Report; StatusUnknown is the
// safe default.
type Probe func() Report

// Manager is the V1 capability store.
// Manager is goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	probes  map[Feature]Probe
	reports map[Feature]Report
}

// NewManager returns an empty manager.
func NewManager() *Manager {
	return &Manager{probes: map[Feature]Probe{}, reports: map[Feature]Report{}}
}

// RegisterProbe attaches a probe for the
// supplied feature. The probe is run on
// the next Detect call.
func (m *Manager) RegisterProbe(f Feature, p Probe) error {
	if p == nil {
		return errors.New("capability: nil probe")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.probes[f] = p
	return nil
}

// SetReport overrides the runtime result
// for a feature. Used by callers that have
// authoritative out-of-band knowledge.
func (m *Manager) SetReport(r Report) {
	if !r.Status.Valid() {
		r.Status = StatusUnknown
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports[r.Feature] = r
}

// Detect runs every registered probe and
// returns the union of fresh and cached
// reports. Probes that panic are caught by
// the recover in runProbe; the resulting
// Status is StatusUnknown.
func (m *Manager) Detect() []Report {
	m.mu.Lock()
	probes := make(map[Feature]Probe, len(m.probes))
	for k, v := range m.probes {
		probes[k] = v
	}
	cached := make(map[Feature]Report, len(m.reports))
	for k, v := range m.reports {
		cached[k] = v
	}
	m.mu.Unlock()
	out := make([]Report, 0, len(probes))
	for f, p := range probes {
		r := runProbe(p)
		if r.Feature == "" {
			r.Feature = f
		}
		if !r.Status.Valid() {
			r.Status = StatusUnknown
		}
		out = append(out, r)
	}
	for f, r := range cached {
		// do not overwrite a fresh probe
		// result with a stale cached value
		hasProbe := false
		for _, x := range out {
			if x.Feature == f {
				hasProbe = true
				break
			}
		}
		if !hasProbe {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Feature < out[k].Feature })
	return out
}

func runProbe(p Probe) (out Report) {
	defer func() {
		if recover() != nil {
			out = Report{Status: StatusUnknown, Detail: "probe panicked"}
		}
	}()
	return p()
}

// ReportFor returns the cached report for
// a feature, or StatusUnknown if the
// feature has not been detected yet.
func (m *Manager) ReportFor(f Feature) Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.reports[f]; ok {
		return r
	}
	return Report{Feature: f, Status: StatusUnknown}
}

// ActionResult is the public output of
// "is this action supported?". Callers
// surface the Reason and the Missing
// capability in the API response.
type ActionResult struct {
	Supported bool      `json:"supported"`
	Reason    string    `json:"reason,omitempty"`
	Missing   []Feature `json:"missing,omitempty"`
	Disabled  []Feature `json:"disabled,omitempty"`
}

// CheckAction reports whether the supplied
// list of features is satisfied. A
// "supported" feature with a probe result
// of StatusSupported is satisfied. A
// StatusDisabled is reported separately
// (the action is supported but the
// feature is operator-disabled). A
// StatusUnsupported or StatusUnknown
// feature is reported as Missing.
func (m *Manager) CheckAction(features []Feature) ActionResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := ActionResult{Supported: true}
	for _, f := range features {
		rep, ok := m.reports[f]
		if !ok {
			r.Supported = false
			r.Missing = append(r.Missing, f)
			continue
		}
		switch rep.Status {
		case StatusSupported:
			// satisfied
		case StatusDisabled:
			r.Disabled = append(r.Disabled, f)
		case StatusUnsupported, StatusUnknown:
			r.Supported = false
			r.Missing = append(r.Missing, f)
		}
	}
	sort.Slice(r.Missing, func(i, k int) bool { return r.Missing[i] < r.Missing[k] })
	sort.Slice(r.Disabled, func(i, k int) bool { return r.Disabled[i] < r.Disabled[k] })
	if !r.Supported {
		r.Reason = "missing capabilities: " + joinFeatures(r.Missing)
	}
	return r
}

func joinFeatures(in []Feature) string {
	out := ""
	for i, f := range in {
		if i > 0 {
			out += ","
		}
		out += string(f)
	}
	return out
}
