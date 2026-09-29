// identity/session_continuity.go: G23 session / reconnect
// continuity overlay.
//
// The base Service in service.go already tracks
// LastSeen per Session. This file adds the explicit
// continuity surface the API and CLI need:
//
//	Touch(token)         — heartbeat; advances LastSeen
//	                      without changing ExpiresAt.
//
//	Renew(token, ttl)    — extend the session lifetime
//	                      and return a fresh bearer token
//	                      preserving the same role and
//	                      session id. The old token is
//	                      logged out atomically.
//
//	BoundSessions()      — bounded per-user session
//	                      history (last N sessions per
//	                      user); prevents unbounded
//	                      growth on long-running nodes.
//
//	ReconnectReport()    — per-session evidence: age,
//	                      idle time, lifetime used,
//	                      remaining lifetime.
//
// All operations are goroutine-safe. Touch and Renew
// honour the configured SessionTTL window so callers
// cannot extend sessions beyond the policy.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

// SessionTTL is the default per-session lifetime. The
// API uses 8 hours; the loopback CLI may override via
// Service.SessionTTL.
const SessionTTL = 8 * time.Hour

// MaxSessionsPerUser bounds the per-user history. Older
// sessions beyond the cap are dropped on Renew.
const MaxSessionsPerUser = 16

// SessionContinuity is the small surface the API and CLI
// depend on. The base Service satisfies it.
type SessionContinuity interface {
	Touch(token string) error
	Renew(token string, ttl time.Duration) (string, Session, error)
	BoundSessions(token string) ([]Session, error)
	ReconnectReport(token string) (ReconnectEvidence, error)
}

// Touch updates the session's LastSeen timestamp. The
// caller must present a valid bearer token. Touch does
// not extend ExpiresAt; use Renew for that.
func (s *Service) Touch(token string) error {
	if s == nil {
		return ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.session(token)
	return err
}

// Renew issues a fresh bearer token for the supplied
// session, preserving the same role, user and session id.
// The old token is logged out atomically. The new token
// inherits a fresh ExpiresAt; the lifetime is clamped
// to the supplied ttl (positive) or to SessionTTL when
// the caller passes zero or a negative value.
func (s *Service) Renew(token string, ttl time.Duration) (string, Session, error) {
	if s == nil {
		return "", Session{}, ErrUnauthorized
	}
	if ttl <= 0 {
		ttl = SessionTTL
	}
	if ttl > SessionTTL {
		return "", Session{}, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := sha256.Sum256([]byte(token))
	session, err := s.session(token)
	if err != nil {
		return "", Session{}, err
	}
	// Generate a new bearer token.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", Session{}, errors.New("identity: cannot generate bearer token")
	}
	newToken := hex.EncodeToString(buf)
	newHash := sha256.Sum256([]byte(newToken))
	session.ExpiresAt = s.now().Add(ttl)
	session.LastSeen = s.now()
	s.sessions[newHash] = session
	delete(s.sessions, hash)
	// Drop oldest sessions for this user beyond MaxSessionsPerUser.
	s.trimSessionsLocked(session.User.Username)
	return newToken, session, nil
}

// trimSessionsLocked keeps at most MaxSessionsPerUser
// sessions per username. The caller must hold s.mu.
func (s *Service) trimSessionsLocked(username string) {
	if username == "" {
		return
	}
	type entry struct {
		hash [32]byte
		when time.Time
	}
	var mine []entry
	for h, sess := range s.sessions {
		if sess.User.Username == username {
			mine = append(mine, entry{hash: h, when: sess.LastSeen})
		}
	}
	if len(mine) <= MaxSessionsPerUser {
		return
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].when.Before(mine[j].when) })
	for _, e := range mine[:len(mine)-MaxSessionsPerUser] {
		delete(s.sessions, e.hash)
	}
}

// BoundSessions returns every session for the current
// user, newest first.
func (s *Service) BoundSessions(token string) ([]Session, error) {
	if s == nil {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.session(token)
	if err != nil {
		return nil, err
	}
	var mine []Session
	for _, sess := range s.sessions {
		if sess.User.Username == current.User.Username {
			mine = append(mine, sess)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].LastSeen.After(mine[j].LastSeen) })
	return mine, nil
}

// ReconnectEvidence is the per-session continuity report.
type ReconnectEvidence struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	LastSeen     time.Time `json:"lastSeen"`
	ExpiresAt    time.Time `json:"expiresAt"`
	IdleSeconds  int64     `json:"idleSeconds"`
	AgeSeconds   int64     `json:"ageSeconds"`
	RemainingSec int64     `json:"remainingSeconds"`
	Refreshable  bool      `json:"refreshable"`
	Renewable    bool      `json:"renewable"`
}

// ReconnectReport returns the per-session evidence used
// by the API and CLI to render "session expiring in N
// minutes" warnings and to decide whether a reconnect
// can succeed silently or requires re-authentication.
func (s *Service) ReconnectReport(token string) (ReconnectEvidence, error) {
	if s == nil {
		return ReconnectEvidence{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return ReconnectEvidence{}, err
	}
	now := time.Now().UTC()
	age := int64(now.Sub(session.CreatedAt).Seconds())
	idle := int64(now.Sub(session.LastSeen).Seconds())
	remaining := int64(session.ExpiresAt.Sub(now).Seconds())
	return ReconnectEvidence{
		ID:           session.ID,
		Username:     session.User.Username,
		Role:         session.User.Role,
		CreatedAt:    session.CreatedAt,
		LastSeen:     session.LastSeen,
		ExpiresAt:    session.ExpiresAt,
		IdleSeconds:  idle,
		AgeSeconds:   age,
		RemainingSec: remaining,
		Refreshable:  remaining > 60,
		Renewable:    remaining > -300, // grace window after expiry
	}, nil
}

// silence unused import warnings when no helper imports sync.
var _ sync.Mutex
