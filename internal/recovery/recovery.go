// recovery/recovery.go: V1 factory-reset /
// recovery / emergency-management surface.
// The package owns the typed contracts for
// reset kinds, emergency-access windows,
// recovery environment selection, reboot
// reasons, and graceful shutdown ordering.
//
// The runtime is a thin mapping to existing
// state-store wipes, vault reseals, and the
// boot-environment selector.
package recovery

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// ResetKind is the class of reset.
type ResetKind string

const (
	// ResetFull wipes the configuration,
	// vault, identity, and audit log.
	ResetFull ResetKind = "full"
	// ResetNetworkOnly wipes the network
	// configuration but retains identity,
	// vault and audit.
	ResetNetworkOnly ResetKind = "network-only"
	// ResetAdminOnly resets the local
	// administrator credentials. All other
	// state is preserved.
	ResetAdminOnly ResetKind = "admin-only"
	// ResetFromBackup restores from a named
	// backup.
	ResetFromBackup ResetKind = "from-backup"
)

// Valid reports whether k is known.
func (k ResetKind) Valid() bool {
	switch k {
	case ResetFull, ResetNetworkOnly, ResetAdminOnly, ResetFromBackup:
		return true
	}
	return false
}

// EmergencyAccess is a temporary, audited,
// TTL-bounded management window. The
// management API exposes this only when
// normal access has been lost; the window
// auto-expires and never permanently
// disables the regular auth path.
type EmergencyAccess struct {
	ID         string    `json:"id"`
	Reason     string    `json:"reason"`
	Operator   string    `json:"operator"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	AllowedIPs []string  `json:"allowedIPs,omitempty"`
	Token      string    `json:"token,omitempty"` // vault-protected
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether a is well-formed.
func (a EmergencyAccess) Valid() error {
	if !idPattern.MatchString(a.ID) {
		return errors.New("recovery: id is not canonical")
	}
	if a.Reason == "" {
		return errors.New("recovery: reason is required")
	}
	if a.Operator == "" {
		return errors.New("recovery: operator is required")
	}
	if a.CreatedAt.IsZero() || a.ExpiresAt.IsZero() {
		return errors.New("recovery: timestamps are required")
	}
	if !a.ExpiresAt.After(a.CreatedAt) {
		return errors.New("recovery: expiresAt must be after createdAt")
	}
	if a.ExpiresAt.Sub(a.CreatedAt) > 24*time.Hour {
		return errors.New("recovery: emergency access window must be <= 24h")
	}
	return nil
}

// Active reports whether the access is
// still valid at t.
func (a EmergencyAccess) Active(t time.Time) bool {
	return !t.Before(a.CreatedAt) && t.Before(a.ExpiresAt)
}

// Environment is a boot environment. The
// runtime selects one at boot.
type Environment string

const (
	// EnvActive is the currently active
	// release.
	EnvActive Environment = "active"
	// EnvPrevious is the previous release.
	EnvPrevious Environment = "previous"
	// EnvRecovery is the recovery image.
	EnvRecovery Environment = "recovery"
)

// Valid reports whether e is known.
func (e Environment) Valid() bool {
	switch e {
	case EnvActive, EnvPrevious, EnvRecovery:
		return true
	}
	return false
}

// RebootReason is a structured boot/shutdown
// reason. The reason is recorded in the
// reboot history so the operator can audit
// what triggered a restart.
type RebootReason string

const (
	RebootUnknown       RebootReason = "unknown"
	RebootOperator      RebootReason = "operator"
	RebootUpdate        RebootReason = "update"
	RebootHealthFailure RebootReason = "health-failure"
	RebootPowerLoss     RebootReason = "power-loss"
	RebootWatchdog      RebootReason = "watchdog"
	RebootKernel        RebootReason = "kernel"
	RebootMaintenance   RebootReason = "maintenance"
)

// Valid reports whether r is known.
func (r RebootReason) Valid() bool {
	switch r {
	case RebootUnknown, RebootOperator, RebootUpdate, RebootHealthFailure,
		RebootPowerLoss, RebootWatchdog, RebootKernel, RebootMaintenance:
		return true
	}
	return false
}

// RebootEvent is a single boot or shutdown
// event. The Agent retains the last 256
// events.
type RebootEvent struct {
	At     time.Time    `json:"at"`
	Reason RebootReason `json:"reason"`
	Detail string       `json:"detail,omitempty"`
}

const MaxRebootHistory = 256

// ResetRequest is the input to a reset
// operation.
type ResetRequest struct {
	Kind          ResetKind `json:"kind"`
	BackupID      string    `json:"backupId,omitempty"`
	Operator      string    `json:"operator"`
	Reason        string    `json:"reason"`
	ConfirmPhrase string    `json:"confirmPhrase,omitempty"`
	// ConfirmThreshold, when non-zero,
	// requires the caller to repeat the
	// phrase as a sanity check.
	ConfirmThreshold int `json:"confirmThreshold,omitempty"`
}

// Valid reports whether r is well-formed.
func (r ResetRequest) Valid() error {
	if !r.Kind.Valid() {
		return errors.New("recovery: kind is invalid")
	}
	if r.Operator == "" {
		return errors.New("recovery: operator is required")
	}
	if r.Reason == "" {
		return errors.New("recovery: reason is required")
	}
	if r.Kind == ResetFromBackup && r.BackupID == "" {
		return errors.New("recovery: from-backup requires backupId")
	}
	return nil
}

// ResetImpact describes what the reset will
// destroy. It is computed before the reset
// and presented to the operator.
type ResetImpact struct {
	Kind             ResetKind `json:"kind"`
	WipesConfig      bool      `json:"wipesConfig"`
	WipesVault       bool      `json:"wipesVault"`
	WipesIdentity    bool      `json:"wipesIdentity"`
	WipesAudit       bool      `json:"wipesAudit"`
	WipesBackups     bool      `json:"wipesBackups"`
	RequiresPhrase   bool      `json:"requiresPhrase"`
	ManagementImpact string    `json:"managementImpact"`
}

// Impact computes the impact of a reset
// request. The function is pure.
func (r ResetRequest) Impact() ResetImpact {
	out := ResetImpact{Kind: r.Kind, RequiresPhrase: r.ConfirmThreshold > 0}
	switch r.Kind {
	case ResetFull:
		out.WipesConfig = true
		out.WipesVault = true
		out.WipesIdentity = true
		out.WipesAudit = true
		out.WipesBackups = true
		out.ManagementImpact = "loses all management access; recovery environment only"
	case ResetNetworkOnly:
		out.WipesConfig = true
		out.ManagementImpact = "loses network connectivity; management may be impacted"
	case ResetAdminOnly:
		out.WipesIdentity = true
		out.ManagementImpact = "loses local admin credentials; recovery required"
	case ResetFromBackup:
		out.WipesConfig = true
		out.ManagementImpact = "restore preview required before apply"
	}
	return out
}

// Manager is the V1 recovery store. Manager
// is goroutine-safe.
type Manager struct {
	mu      sync.Mutex
	emerg   map[string]EmergencyAccess
	env     Environment
	history []RebootEvent
	clock   func() time.Time
}

// NewManager returns an empty manager with
// the supplied boot environment.
func NewManager(env Environment, clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{emerg: map[string]EmergencyAccess{}, env: env, clock: clock}
}

// SetEnvironment selects the next boot
// environment.
func (m *Manager) SetEnvironment(e Environment) error {
	if !e.Valid() {
		return errors.New("recovery: environment is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.env = e
	return nil
}

// Environment returns the selected boot
// environment.
func (m *Manager) Environment() Environment {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.env
}

// OpenEmergency opens a new emergency access
// window. The window is auto-expired by
// Active() once the clock advances past
// ExpiresAt.
func (m *Manager) OpenEmergency(req EmergencyAccess) (EmergencyAccess, error) {
	if err := req.Valid(); err != nil {
		return EmergencyAccess{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emerg[req.ID] = req
	return req, nil
}

// CloseEmergency revokes a window before
// its natural expiry.
func (m *Manager) CloseEmergency(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.emerg[id]; !ok {
		return errors.New("recovery: unknown emergency id")
	}
	delete(m.emerg, id)
	return nil
}

// ActiveEmergency returns the active
// emergency windows at t. Expired windows
// are removed as a side effect.
func (m *Manager) ActiveEmergency(t time.Time) []EmergencyAccess {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []EmergencyAccess{}
	for id, a := range m.emerg {
		if a.Active(t) {
			out = append(out, a)
		} else {
			delete(m.emerg, id)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.Before(out[k].CreatedAt) })
	return out
}

// RecordReboot appends a reboot event. The
// history is bounded to MaxRebootHistory.
func (m *Manager) RecordReboot(reason RebootReason, detail string) error {
	if !reason.Valid() {
		return errors.New("recovery: reason is invalid")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, RebootEvent{At: m.clock().UTC(), Reason: reason, Detail: detail})
	if len(m.history) > MaxRebootHistory {
		m.history = m.history[len(m.history)-MaxRebootHistory:]
	}
	return nil
}

// RebootHistory returns a copy of the
// reboot history, newest first.
func (m *Manager) RebootHistory() []RebootEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RebootEvent, len(m.history))
	copy(out, m.history)
	sort.Slice(out, func(i, k int) bool { return out[i].At.After(out[k].At) })
	return out
}

// ShutdownStep is a single ordered step of
// a graceful shutdown. The runtime executes
// the steps in order; a failing step
// aborts the shutdown and surfaces the
// failure.
type ShutdownStep struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// ShutdownPlan is the dependency-ordered
// list of steps. The plan is consumed by
// the runtime's shutdown orchestrator.
type ShutdownPlan struct {
	Reason RebootReason   `json:"reason"`
	Detail string         `json:"detail,omitempty"`
	Steps  []ShutdownStep `json:"steps"`
}

// DefaultShutdownPlan returns the V1
// graceful-shutdown ordering. The order is:
//  1. drain management API
//  2. close tunnels
//  3. drain conntrack
//  4. flush state journal
//  5. power down services
func DefaultShutdownPlan(reason RebootReason, detail string) ShutdownPlan {
	return ShutdownPlan{
		Reason: reason,
		Detail: detail,
		Steps: []ShutdownStep{
			{Name: "drain-management-api", Required: true},
			{Name: "close-tunnels", Required: true},
			{Name: "drain-conntrack", Required: true},
			{Name: "flush-state-journal", Required: true},
			{Name: "power-down-services", Required: true},
		},
	}
}
