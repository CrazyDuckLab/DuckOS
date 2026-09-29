// storagewatch/storagewatch.go: V1
// storage-failure surface. The package
// owns the typed contract for storage
// near-full, full, and read-only state,
// the priority ordering between core
// networking and optional telemetry,
// the bounded-log policy, and the update
// preflight refusal path.
//
// The runtime is a thin mapping to
// statfs / statvfs and the existing
// bounded log ring buffer.
package storagewatch

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Mode is the storage mode.
type Mode string

const (
	// ModeNormal is the healthy state.
	ModeNormal Mode = "normal"
	// ModeNearFull is the warning state
	// (>= NearFullPct).
	ModeNearFull Mode = "near-full"
	// ModeFull is the critical state
	// (>= 100% reported or no space).
	ModeFull Mode = "full"
	// ModeReadOnly is the read-only
	// state.
	ModeReadOnly Mode = "read-only"
)

// Valid reports whether m is known.
func (m Mode) Valid() bool {
	switch m {
	case ModeNormal, ModeNearFull, ModeFull, ModeReadOnly:
		return true
	}
	return false
}

// DefaultNearFullPct is the default
// near-full threshold (90%).
const DefaultNearFullPct = 90

// PathStat is the per-path storage
// statistics.
type PathStat struct {
	Path       string    `json:"path"`
	TotalBytes int64     `json:"totalBytes"`
	FreeBytes  int64     `json:"freeBytes"`
	UsedBytes  int64     `json:"usedBytes"`
	UsedPct    int       `json:"usedPct"`
	ReadOnly   bool      `json:"readOnly"`
	Mode       Mode      `json:"mode"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Compute derives the Mode from the
// supplied path stat. nearFullPct is the
// warning threshold.
func (s PathStat) Compute(nearFullPct int) PathStat {
	if s.TotalBytes <= 0 {
		return s
	}
	if s.ReadOnly {
		s.Mode = ModeReadOnly
		return s
	}
	used := s.UsedBytes
	if used < 0 {
		used = 0
	}
	s.UsedPct = int(used * 100 / s.TotalBytes)
	if s.UsedPct >= 100 {
		s.Mode = ModeFull
		return s
	}
	if nearFullPct > 0 && s.UsedPct >= nearFullPct {
		s.Mode = ModeNearFull
		return s
	}
	s.Mode = ModeNormal
	return s
}

// Valid reports whether s is well-formed.
func (s PathStat) Valid() error {
	if s.Path == "" {
		return errors.New("storagewatch: path is required")
	}
	if s.TotalBytes < 0 {
		return errors.New("storagewatch: totalBytes must be non-negative")
	}
	if s.FreeBytes < 0 {
		return errors.New("storagewatch: freeBytes must be non-negative")
	}
	if !s.Mode.Valid() {
		return errors.New("storagewatch: mode is invalid")
	}
	return nil
}

// Priority is the per-subsystem priority
// when storage is degraded. A higher
// priority subsystem keeps its logs and
// configuration; lower-priority subsystems
// drop them.
type Priority string

const (
	// PriorityCore covers routing,
	// firewall, core network.
	PriorityCore Priority = "core"
	// PriorityManagement covers local
	// management API/UI.
	PriorityManagement Priority = "management"
	// PriorityTelemetry covers optional
	// telemetry, logging, audit, and
	// remote backup.
	PriorityTelemetry Priority = "telemetry"
)

// Valid reports whether p is known.
func (p Priority) Valid() bool {
	switch p {
	case PriorityCore, PriorityManagement, PriorityTelemetry:
		return true
	}
	return false
}

// Order returns the priority rank. Higher
// rank keeps its data when storage is
// degraded.
func (p Priority) Order() int {
	switch p {
	case PriorityCore:
		return 3
	case PriorityManagement:
		return 2
	case PriorityTelemetry:
		return 1
	}
	return 0
}

// Subsystem is a typed per-subsystem
// storage declaration.
type Subsystem struct {
	Name         string   `json:"name"`
	Priority     Priority `json:"priority"`
	BoundedBytes int64    `json:"boundedBytes"`
}

// Valid reports whether s is well-formed.
func (s Subsystem) Valid() error {
	if s.Name == "" {
		return errors.New("storagewatch: subsystem name is required")
	}
	if !s.Priority.Valid() {
		return errors.New("storagewatch: priority is invalid")
	}
	if s.BoundedBytes < 0 {
		return errors.New("storagewatch: boundedBytes must be non-negative")
	}
	return nil
}

// Manager is the V1 storage store.
// Manager is goroutine-safe.
type Manager struct {
	mu        sync.Mutex
	stats     map[string]PathStat
	subsystem map[string]Subsystem
	clock     func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{stats: map[string]PathStat{}, subsystem: map[string]Subsystem{}, clock: clock}
}

// SetStat replaces a per-path stat.
func (m *Manager) SetStat(s PathStat) {
	if s.Path == "" {
		return
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = m.clock().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats[s.Path] = s
}

// SetSubsystem registers a subsystem
// declaration.
func (m *Manager) SetSubsystem(s Subsystem) error {
	if err := s.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subsystem[s.Name] = s
	return nil
}

// Stats returns the per-path stats sorted
// by path.
func (m *Manager) Stats() []PathStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PathStat, 0, len(m.stats))
	for _, s := range m.stats {
		out = append(out, s)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Path < out[k].Path })
	return out
}

// WorstPath is the path with the most
// degraded mode, or empty when no path
// is set.
func (m *Manager) WorstPath() (PathStat, bool) {
	stats := m.Stats()
	var worst PathStat
	for _, s := range stats {
		if !worst.Mode.Valid() || s.Mode == ModeReadOnly || (worst.Mode != ModeReadOnly && worst.Mode == ModeNormal) {
			worst = s
		}
	}
	return worst, worst.Path != ""
}

// PreflightResult is the result of an
// update preflight.
type PreflightResult struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason,omitempty"`
	FreeBytes     int64  `json:"freeBytes"`
	RequiredBytes int64  `json:"requiredBytes"`
	Path          string `json:"path"`
}

// UpdatePreflight refuses an update when
// the candidate payload cannot fit on
// the supplied path. The runtime supplies
// the path's free-bytes.
func (m *Manager) UpdatePreflight(path string, requiredBytes int64) PreflightResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	stat, ok := m.stats[path]
	if !ok {
		return PreflightResult{Allowed: false, Reason: "path is not tracked", Path: path}
	}
	if stat.Mode == ModeReadOnly {
		return PreflightResult{Allowed: false, Reason: "path is read-only", Path: path, FreeBytes: stat.FreeBytes, RequiredBytes: requiredBytes}
	}
	if stat.FreeBytes < requiredBytes {
		return PreflightResult{Allowed: false, Reason: "insufficient free space", Path: path, FreeBytes: stat.FreeBytes, RequiredBytes: requiredBytes}
	}
	return PreflightResult{Allowed: true, Path: path, FreeBytes: stat.FreeBytes, RequiredBytes: requiredBytes}
}

// DegradationPlan is the per-subsystem
// degradation order when storage is
// degraded. Subsystems are ordered by
// Priority ascending (telemetry first to
// drop, core last to keep).
type DegradationPlan struct {
	DropOrder []string `json:"dropOrder"`
	KeepOrder []string `json:"keepOrder"`
	WorstPath string   `json:"worstPath"`
	WorstMode Mode     `json:"worstMode"`
}

// PlanDegradation returns the order in
// which subsystems must drop their data
// when the worst path is degraded.
func (m *Manager) PlanDegradation() DegradationPlan {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := make([]Subsystem, 0, len(m.subsystem))
	for _, s := range m.subsystem {
		subs = append(subs, s)
	}
	sort.Slice(subs, func(i, k int) bool { return subs[i].Priority.Order() < subs[k].Priority.Order() })
	plan := DegradationPlan{}
	for _, s := range subs {
		plan.DropOrder = append(plan.DropOrder, s.Name)
	}
	// Keep order is the reverse of drop
	// order.
	for i := len(subs) - 1; i >= 0; i-- {
		plan.KeepOrder = append(plan.KeepOrder, subs[i].Name)
	}
	for _, s := range m.stats {
		if plan.WorstPath == "" || modeRank(s.Mode) > modeRank(modeFromString(plan.WorstMode)) {
			plan.WorstPath = s.Path
			plan.WorstMode = s.Mode
		}
	}
	return plan
}

func modeRank(m Mode) int {
	switch m {
	case ModeReadOnly:
		return 4
	case ModeFull:
		return 3
	case ModeNearFull:
		return 2
	case ModeNormal:
		return 1
	}
	return 0
}

func modeFromString(s Mode) Mode { return s }
