// safeapply/lock.go: V1 maintenance /
// exclusive config session. A session holds
// a single writer lock for a bounded window;
// the lock blocks Apply, Stage, and Rollback
// from any other operator. The session is
// explicit: the operator starts it, the
// operator ends it, and the Agent auto-expires
// the session when the window elapses.
package safeapply

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"sync"
	"time"
)

// MaxSessionWindow is the upper bound on a
// single session. Anything longer than an
// hour is almost certainly a forgotten lock.
const MaxSessionWindow = time.Hour

// MinSessionWindow is the floor. A session
// shorter than 30 seconds is operationally
// useless.
const MinSessionWindow = 30 * time.Second

// SessionID is the durable identifier.
type SessionID string

// Session is the live maintenance state.
type Session struct {
	ID        SessionID `json:"id"`
	Owner     string    `json:"owner"`
	Reason    string    `json:"reason"`
	StartedAt time.Time `json:"startedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

var reasonPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,127}$`)

// LockManager is the V1 session gate. A
// LockManager is goroutine-safe.
type LockManager struct {
	mu     sync.Mutex
	active *Session
	clock  func() time.Time
}

// NewLockManager returns an empty lock
// manager.
func NewLockManager(clock func() time.Time) *LockManager {
	if clock == nil {
		clock = time.Now
	}
	return &LockManager{clock: clock}
}

// Begin starts a new session. The LockManager
// refuses to begin a new session while an
// active one exists; the operator must first
// End the previous one.
func (m *LockManager) Begin(owner, reason string, window time.Duration) (Session, error) {
	if !reasonPattern.MatchString(reason) {
		return Session{}, errors.New("lock: reason is not canonical")
	}
	if window < MinSessionWindow || window > MaxSessionWindow {
		return Session{}, errors.New("lock: window must be 30s..1h")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != nil && m.clock().Before(m.active.ExpiresAt) {
		return Session{}, errors.New("lock: a session is already active")
	}
	now := m.clock().UTC()
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	s := Session{ID: SessionID(hex.EncodeToString(buf)), Owner: owner, Reason: reason, StartedAt: now, ExpiresAt: now.Add(window)}
	m.active = &s
	return s, nil
}

// End terminates the active session. The
// owner must match.
func (m *LockManager) End(id SessionID, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return errors.New("lock: no active session")
	}
	if m.active.ID != id {
		return errors.New("lock: session id does not match")
	}
	if m.active.Owner != owner {
		return errors.New("lock: only the owner can end the session")
	}
	m.active = nil
	return nil
}

// Active returns the active session, if any.
// An expired session is reported as
// non-active.
func (m *LockManager) Active() (Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return Session{}, false
	}
	if m.clock().After(m.active.ExpiresAt) {
		m.active = nil
		return Session{}, false
	}
	return *m.active, true
}

// Authorize returns nil when no active session
// is in place, or when the caller's session
// owns the lock. A caller without the lock is
// refused.
func (m *LockManager) Authorize(sess SessionID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return nil
	}
	if m.clock().After(m.active.ExpiresAt) {
		m.active = nil
		return nil
	}
	if m.active.ID != sess {
		return errors.New("lock: session is not the lock owner")
	}
	return nil
}
