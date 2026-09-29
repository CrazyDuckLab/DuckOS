// maintenance/maintenance.go: V1
// maintenance-mode contract. The package
// owns the typed suppression set, the
// auto-expiry, and the audit trail.
//
// The runtime is a thin mapping to a
// global toggle consulted by the watchdog
// and the auto-remediation path.
package maintenance

import (
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

// SuppressionKind is the class of
// behaviour maintenance mode suppresses.
type SuppressionKind string

const (
	SupFailoverReaction    SuppressionKind = "failover-reaction"
	SupWatchdogAction      SuppressionKind = "watchdog-action"
	SupAutoRemediation     SuppressionKind = "auto-remediation"
	SupScheduledDisruptive SuppressionKind = "scheduled-disruptive"
)

// Valid reports whether k is known.
func (k SuppressionKind) Valid() bool {
	switch k {
	case SupFailoverReaction, SupWatchdogAction, SupAutoRemediation, SupScheduledDisruptive:
		return true
	}
	return false
}

// CoreSecuritySuppressions enumerates
// the suppression kinds that affect core
// security. The Manager refuses to enable
// a maintenance window that would suppress
// any of these.
var CoreSecuritySuppressions = map[SuppressionKind]bool{}

// Session is an active maintenance
// window. The window auto-expires.
type Session struct {
	ID           string            `json:"id"`
	Operator     string            `json:"operator"`
	Reason       string            `json:"reason"`
	Suppressions []SuppressionKind `json:"suppressions"`
	CreatedAt    time.Time         `json:"createdAt"`
	ExpiresAt    time.Time         `json:"expiresAt"`
	AuditTrail   []AuditEntry      `json:"audit,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether s is well-formed.
func (s Session) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("maintenance: id is not canonical")
	}
	if s.Operator == "" {
		return errors.New("maintenance: operator is required")
	}
	if s.Reason == "" {
		return errors.New("maintenance: reason is required")
	}
	if len(s.Suppressions) == 0 {
		return errors.New("maintenance: at least one suppression is required")
	}
	for _, k := range s.Suppressions {
		if !k.Valid() {
			return errors.New("maintenance: suppression kind is unknown: " + string(k))
		}
	}
	if s.CreatedAt.IsZero() || s.ExpiresAt.IsZero() {
		return errors.New("maintenance: timestamps are required")
	}
	if !s.ExpiresAt.After(s.CreatedAt) {
		return errors.New("maintenance: expiresAt must be after createdAt")
	}
	if s.ExpiresAt.Sub(s.CreatedAt) > 24*time.Hour {
		return errors.New("maintenance: maintenance window must be <= 24h")
	}
	return nil
}

// AuditEntry is one audit record on a
// maintenance session.
type AuditEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Detail string    `json:"detail,omitempty"`
}

// Manager is the V1 maintenance store.
// Manager is goroutine-safe.
type Manager struct {
	mu     sync.Mutex
	active map[string]Session
	clock  func() time.Time
}

// NewManager returns an empty manager.
func NewManager(clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{active: map[string]Session{}, clock: clock}
}

// Open starts a new maintenance window.
// The window is auto-expired by Active()
// once the clock advances past ExpiresAt.
func (m *Manager) Open(s Session) (Session, error) {
	if err := s.Valid(); err != nil {
		return Session{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active[s.ID] = s
	return s, nil
}

// Close revokes a window before its
// natural expiry. The action is recorded
// in the audit trail.
func (m *Manager) Close(id, operator, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.active[id]
	if !ok {
		return errors.New("maintenance: unknown id")
	}
	s.AuditTrail = append(s.AuditTrail, AuditEntry{At: m.clock().UTC(), Action: "closed", Detail: "operator=" + operator + " reason=" + reason})
	delete(m.active, id)
	return nil
}

// AppendAudit records an audit entry on an
// active session.
func (m *Manager) AppendAudit(id, action, detail string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.active[id]
	if !ok {
		return errors.New("maintenance: unknown id")
	}
	s.AuditTrail = append(s.AuditTrail, AuditEntry{At: m.clock().UTC(), Action: action, Detail: detail})
	m.active[id] = s
	return nil
}

// Active returns the active sessions at t.
// Expired sessions are removed as a side
// effect and recorded in their audit
// trail.
func (m *Manager) Active(t time.Time) []Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Session{}
	for id, s := range m.active {
		if t.Before(s.ExpiresAt) && !t.Before(s.CreatedAt) {
			out = append(out, s)
		} else {
			s.AuditTrail = append(s.AuditTrail, AuditEntry{At: t.UTC(), Action: "expired"})
			delete(m.active, id)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.Before(out[k].CreatedAt) })
	return out
}

// IsSuppressed reports whether the
// supplied kind is suppressed by an
// active session at t.
func (m *Manager) IsSuppressed(kind SuppressionKind, t time.Time) bool {
	for _, s := range m.Active(t) {
		for _, k := range s.Suppressions {
			if k == kind {
				return true
			}
		}
	}
	return false
}
