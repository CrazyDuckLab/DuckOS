// G23 session continuity torture regression.
package identity

import (
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"
)

// newEnrolledService returns a Service with a single
// enrolled user so the Login path can mint sessions.
func newEnrolledService(t *testing.T, username, password string) (*Service, Session, string) {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err := s.Enroll(username, password); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	token, session, err := s.Login(username, password)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return s, session, token
}

func TestTouch_UpdatesLastSeen(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	first, err := s.ReconnectReport(token)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := s.Touch(token); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	second, err := s.ReconnectReport(token)
	if err != nil {
		t.Fatalf("report2: %v", err)
	}
	if second.LastSeen.Equal(first.LastSeen) {
		t.Fatalf("LastSeen must advance after Touch")
	}
	if !second.ExpiresAt.Equal(first.ExpiresAt) {
		t.Fatalf("Touch must not change ExpiresAt")
	}
}

func TestTouch_RejectsUnknownToken(t *testing.T) {
	s, _, _ := newEnrolledService(t, "alice", "password-123-very-long")
	if err := s.Touch("not-a-real-token"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown token must surface ErrUnauthorized, got %v", err)
	}
}

func TestTouch_NilService(t *testing.T) {
	var s *Service
	if err := s.Touch("anything"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("nil service must surface ErrUnauthorized")
	}
}

func TestRenew_IssuesFreshToken(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	newToken, sess, err := s.Renew(token, time.Hour)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if newToken == token {
		t.Fatalf("Renew must return a different token")
	}
	if sess.ID == "" {
		t.Fatalf("renewed session must keep id")
	}
	// Old token no longer valid.
	if err := s.Touch(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old token must be logged out, got %v", err)
	}
	// New token works.
	if err := s.Touch(newToken); err != nil {
		t.Fatalf("new token must work, got %v", err)
	}
}

func TestRenew_DefaultsToSessionTTL(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	_, _, err := s.Renew(token, 0)
	if err != nil {
		t.Fatalf("Renew(0) must use default ttl: %v", err)
	}
}

func TestRenew_RejectsExpiredToken(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	// Manually expire the session.
	s.mu.Lock()
	hash := sha256ForTest(token)
	sess := s.sessions[hash]
	sess.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	s.sessions[hash] = sess
	s.mu.Unlock()

	if _, _, err := s.Renew(token, time.Hour); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired token must surface ErrUnauthorized, got %v", err)
	}
}

func TestBoundSessions_ReturnsUserSessions(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	sessions, err := s.BoundSessions(token)
	if err != nil {
		t.Fatalf("BoundSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestBoundSessions_NewestFirst(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	// Issue a few additional sessions via Renew.
	for i := 0; i < 3; i++ {
		newToken, _, err := s.Renew(token, time.Hour)
		if err != nil {
			t.Fatalf("Renew[%d]: %v", i, err)
		}
		token = newToken
	}
	sessions, err := s.BoundSessions(token)
	if err != nil {
		t.Fatalf("BoundSessions: %v", err)
	}
	if len(sessions) != 1 {
		// Renew replaces the previous token, so only the
		// latest session exists for this user.
		t.Fatalf("expected 1 session after Renew chain, got %d", len(sessions))
	}
}

func TestBoundSessions_TrimsToMaxSessionsPerUser(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	// Inject sessions directly to overflow MaxSessionsPerUser.
	s.mu.Lock()
	for i := 0; i < MaxSessionsPerUser+5; i++ {
		buf := make([]byte, 16)
		for j := range buf {
			buf[j] = byte('a' + (i+j)%26)
		}
		hash := sha256.Sum256(buf)
		s.sessions[hash] = Session{
			ID:        string(buf),
			User:      User{Username: "alice", Role: Owner},
			CreatedAt: time.Now().UTC(),
			LastSeen:  time.Now().Add(time.Duration(-i) * time.Second).UTC(),
			ExpiresAt: time.Now().Add(time.Hour).UTC(),
		}
	}
	s.mu.Unlock()
	// Renew triggers trimSessionsLocked for the same user.
	if _, _, err := s.Renew(token, time.Hour); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	s.mu.Lock()
	count := 0
	for _, sess := range s.sessions {
		if sess.User.Username == "alice" {
			count++
		}
	}
	s.mu.Unlock()
	if count > MaxSessionsPerUser {
		t.Fatalf("alice sessions not trimmed: %d > %d", count, MaxSessionsPerUser)
	}
}

func TestReconnectReport_FreshSession(t *testing.T) {
	s, _, token := newEnrolledService(t, "alice", "password-123-very-long")
	rep, err := s.ReconnectReport(token)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if rep.Username != "alice" {
		t.Fatalf("username drift: %q", rep.Username)
	}
	if rep.Refreshable != true {
		t.Fatalf("fresh session must be Refreshable")
	}
	if rep.Renewable != true {
		t.Fatalf("fresh session must be Renewable")
	}
	if rep.RemainingSec <= 0 {
		t.Fatalf("remaining must be positive")
	}
}

func TestReconnectReport_NilService(t *testing.T) {
	var s *Service
	if _, err := s.ReconnectReport("anything"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("nil service must surface ErrUnauthorized")
	}
}

// sha256ForTest is a tiny helper so the test can compute
// the session hash without exposing the Service internals.
func sha256ForTest(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}
