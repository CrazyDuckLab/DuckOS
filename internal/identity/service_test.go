package identity

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

const testPassword = "Synthetic test password only 42!"

func serviceTest(t *testing.T) (*Service, string) {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func ownerTest(t *testing.T, s *Service) string {
	t.Helper()
	if _, err := s.Enroll("owner", testPassword); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.Login("owner", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func TestEnrollmentPersistenceAndSessionRestart(t *testing.T) {
	s, path := serviceTest(t)
	token := ownerTest(t, s)
	if _, err := s.Enroll("attacker", testPassword); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Authorize(token, ConfigureNetwork); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Enrolled() {
		t.Fatal("enrollment forgotten")
	}
	if _, err = s.Current(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("session survived restart")
	}
	if _, _, err = s.Login("owner", testPassword); err != nil {
		t.Fatal(err)
	}
}
func TestRBACAndLastOwner(t *testing.T) {
	s, _ := serviceTest(t)
	owner := ownerTest(t, s)
	u, err := s.CreateUser(owner, "reader", testPassword, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	reader, _, err := s.Login("reader", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(reader, Observe); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(reader, ConfigureNetwork); !errors.Is(err, ErrForbidden) {
		t.Fatal("reader may mutate network")
	}
	if _, err = s.Users(reader); !errors.Is(err, ErrForbidden) {
		t.Fatal("reader may list users")
	}
	if _, err = s.CreateUser(reader, "escalation", testPassword, Owner); !errors.Is(err, ErrForbidden) {
		t.Fatal("reader created owner")
	}
	users, _ := s.Users(owner)
	if err = s.UpdateUser(owner, users[0].ID, ReadOnly, false); err == nil {
		t.Fatal("demoted final Owner")
	}
	if err = s.UpdateUser(owner, u.ID, ReadOnly, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Current(reader); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("disabled user kept session")
	}
	if _, _, err = s.Login("reader", testPassword); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("disabled user logged in")
	}
}
func TestSessionExpiryAndRevocation(t *testing.T) {
	s, _ := serviceTest(t)
	token := ownerTest(t, s)
	session, err := s.Current(token)
	if err != nil {
		t.Fatal(err)
	}
	now := session.LastSeen.Add(31 * time.Minute)
	s.now = func() time.Time { return now }
	if _, err = s.Current(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("idle session accepted")
	}
	token, session, err = s.Login("owner", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Revoke(token, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Current(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked session accepted")
	}
}
func TestPasswordsAndUserProjection(t *testing.T) {
	s, path := serviceTest(t)
	owner := ownerTest(t, s)
	users, err := s.Users(owner)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(users)
	if strings.Contains(string(data), "password") || strings.Contains(string(data), "salt") {
		t.Fatal("user projection leaked password hash")
	}
	stored, err := os.ReadFile(path + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), testPassword) {
		t.Fatal("plaintext password persisted")
	}
	if err = s.ChangePassword(owner, "incorrect old password", testPassword+"new"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if err = s.ChangePassword(owner, testPassword, testPassword+"new"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Current(owner); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("password change did not revoke session")
	}
	if _, _, err = s.Login("owner", testPassword); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old password still accepted")
	}
	if _, _, err = s.Login("owner", testPassword+"new"); err != nil {
		t.Fatal(err)
	}
}
func TestLoginBoundedAndUnknownUser(t *testing.T) {
	s, _ := serviceTest(t)
	ownerTest(t, s)
	if _, _, err := s.Login("unknown", testPassword); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	s.attempts = 10
	if _, _, err := s.Login("owner", testPassword); !errors.Is(err, ErrRateLimited) {
		t.Fatal("login budget not enforced")
	}
	for _, role := range []Role{Owner, NetworkAdmin, Helpdesk, Reception, ReadOnly, Role("attacker")} {
		if Allowed(role, Permission("unknown.operation")) {
			t.Fatal("unknown permission allowed")
		}
	}
	if Allowed(Reception, Observe) || Allowed(Helpdesk, ConfigureNetwork) || Allowed(NetworkAdmin, ManageUsers) {
		t.Fatal("role boundary widened")
	}
}
func TestPasswordSaltAndInputBounds(t *testing.T) {
	a, err := hashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if a.Salt == b.Salt || a.Digest == b.Digest {
		t.Fatal("salt reused")
	}
	if !verifyPassword(a, testPassword) || verifyPassword(a, "incorrect password") {
		t.Fatal("password verification failed")
	}
	for _, p := range []string{"short", strings.Repeat("x", 129), string([]byte{255})} {
		if _, err = hashPassword(p); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
}
