package identity

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

func TestAuditContinuityCannotReviveIdleSession(t *testing.T) {
	for _, op := range []string{"touch", "renew"} {
		t.Run(op, func(t *testing.T) {
			s, _, token := newEnrolledService(t, "owner", "Synthetic session audit password!")
			hash := sha256.Sum256([]byte(token))
			s.mu.Lock()
			sess := s.sessions[hash]
			sess.LastSeen = time.Now().Add(-31 * time.Minute)
			s.sessions[hash] = sess
			s.mu.Unlock()
			var err error
			if op == "touch" {
				err = s.Touch(token)
			} else {
				_, _, err = s.Renew(token, time.Hour)
			}
			if !errors.Is(err, ErrUnauthorized) {
				t.Fatal("idle session revived:", err)
			}
		})
	}
}
func TestAuditRenewLifetimeIsBounded(t *testing.T) {
	s, _, token := newEnrolledService(t, "owner", "Synthetic session audit password!")
	if _, _, err := s.Renew(token, 365*24*time.Hour); !errors.Is(err, ErrForbidden) {
		t.Fatal("unbounded renewal accepted")
	}
	if _, err := s.Current(token); err != nil {
		t.Fatal("refused renewal invalidated current session")
	}
}
