package identity

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/state"
)

var ErrUnauthorized = errors.New("authentication required or credentials invalid")
var ErrForbidden = errors.New("permission denied")
var ErrRateLimited = errors.New("login capacity exceeded; retry later")
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,47}$`)

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     Role   `json:"role"`
	Disabled bool   `json:"disabled"`
}
type storedUser struct {
	User     User         `json:"user"`
	Password passwordHash `json:"password"`
}
type Audit struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
}
type database struct {
	Version       int          `json:"version"`
	Users         []storedUser `json:"users"`
	Audit         []Audit      `json:"audit"`
	OwnerRecovery bool         `json:"ownerRecovery,omitempty"`
}
type Session struct {
	ID        string    `json:"id"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	LastSeen  time.Time `json:"lastSeen"`
}
type Service struct {
	mu       sync.Mutex
	store    *state.Store
	db       database
	sessions map[[32]byte]Session
	dummy    passwordHash
	attempts int
	window   time.Time
	now      func() time.Time
	closed   bool
}

func Open(path string) (*Service, error) {
	initial, _ := json.Marshal(database{Version: 1, Users: []storedUser{}, Audit: []Audit{}})
	store, err := state.Open(path, initial, validateDatabase)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			store.Close()
		}
	}()
	snap, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	// Identity has no external side effect before the atomic canonical commit.
	if snap.Pending != nil {
		if snap.Pending.Phase != state.RollingBack {
			if err = store.Transition(snap.Pending.ID, state.RollingBack, time.Time{}); err != nil {
				return nil, err
			}
		}
		if err = store.RolledBack(snap.Pending.ID); err != nil {
			return nil, err
		}
	}
	var db database
	if err = json.Unmarshal(snap.Current.Config, &db); err != nil {
		return nil, err
	}
	dummy, err := hashPassword(randomID() + randomID())
	if err != nil {
		return nil, err
	}
	s := &Service{store: store, db: db, sessions: map[[32]byte]Session{}, dummy: dummy, now: time.Now}
	ok = true
	return s, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.sessions)
	return s.store.Close()
}
func (s *Service) Enrolled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.db.Users) > 0 && !s.db.OwnerRecovery
}

// Enroll may only be reached after the API verifies the installation bootstrap
// credential and the secure origin boundary. Only offline root recovery may
// explicitly reopen enrollment while preserving non-Owner users.
func (s *Service) Enroll(username, password string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (len(s.db.Users) > 0 && !s.db.OwnerRecovery) {
		return User{}, ErrForbidden
	}
	if !usernamePattern.MatchString(username) {
		return User{}, errors.New("invalid username")
	}
	for _, existing := range s.db.Users {
		if existing.User.Username == username {
			return User{}, errors.New("username already exists")
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	u := User{ID: randomID(), Username: username, Role: Owner}
	if u.ID == "" {
		return User{}, errors.New("entropy unavailable")
	}
	next := cloneDB(s.db)
	next.OwnerRecovery = false
	next.Users = append(next.Users, storedUser{User: u, Password: hash})
	if err = s.save(next, u.ID, "owner.enroll", u.ID); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Service) Login(username, password string) (string, Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", Session{}, ErrUnauthorized
	}
	now := s.now()
	if s.window.IsZero() || now.Sub(s.window) >= time.Minute {
		s.window = now
		s.attempts = 0
	}
	if s.attempts >= 10 {
		return "", Session{}, ErrRateLimited
	}
	s.attempts++
	if len(username) > 48 || len(password) > 512 {
		return "", Session{}, ErrUnauthorized
	}
	hash := s.dummy
	var user User
	for _, u := range s.db.Users {
		if u.User.Username == username {
			hash = u.Password
			user = u.User
			break
		}
	}
	valid := verifyPassword(hash, password)
	if !valid || user.ID == "" || user.Disabled {
		return "", Session{}, ErrUnauthorized
	}
	s.prune(now)
	if len(s.sessions) >= 64 {
		return "", Session{}, ErrRateLimited
	}
	token := randomID() + randomID()
	id := randomID()
	if len(token) != 64 || id == "" {
		return "", Session{}, errors.New("entropy unavailable")
	}
	session := Session{ID: id, User: user, CreatedAt: now, ExpiresAt: now.Add(8 * time.Hour), LastSeen: now}
	s.sessions[sha256.Sum256([]byte(token))] = session
	return token, session, nil
}

func (s *Service) Authorize(token string, permission Permission) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return User{}, err
	}
	if !Allowed(session.User.Role, permission) {
		return User{}, ErrForbidden
	}
	return session.User, nil
}

// Role returns the role for the supplied bearer token.
// The boolean is false when the token is unknown or the
// service is not enrolled. The method is intentionally
// read-only so the per-object authorization overlay
// (G21) can consult the role without mutating state.
func (s *Service) Role(token string) (Role, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return "", false
	}
	return session.User.Role, true
}
func (s *Service) Current(token string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session(token)
}
func (s *Service) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sha256.Sum256([]byte(token)))
}
func (s *Service) Sessions(token string) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.session(token)
	if err != nil {
		return nil, err
	}
	list := []Session{}
	s.prune(s.now())
	for _, session := range s.sessions {
		if session.User.ID == current.User.ID || current.User.Role == Owner {
			list = append(list, session)
		}
	}
	return list, nil
}
func (s *Service) Revoke(token, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.session(token)
	if err != nil {
		return err
	}
	for key, v := range s.sessions {
		if v.ID == id {
			if v.User.ID != current.User.ID && current.User.Role != Owner {
				return ErrForbidden
			}
			delete(s.sessions, key)
			return nil
		}
	}
	return errors.New("session not found")
}
func (s *Service) Users(token string) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return nil, err
	}
	if !Allowed(session.User.Role, ManageUsers) {
		return nil, ErrForbidden
	}
	users := []User{}
	for _, u := range s.db.Users {
		users = append(users, u.User)
	}
	return users, nil
}
func (s *Service) CreateUser(token, username, password string, role Role) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return User{}, err
	}
	if !Allowed(session.User.Role, ManageUsers) {
		return User{}, ErrForbidden
	}
	if !usernamePattern.MatchString(username) || !validRole(role) || len(s.db.Users) >= 64 {
		return User{}, errors.New("invalid user or user limit")
	}
	for _, u := range s.db.Users {
		if u.User.Username == username {
			return User{}, errors.New("username already exists")
		}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	u := User{ID: randomID(), Username: username, Role: role}
	if u.ID == "" {
		return User{}, errors.New("entropy unavailable")
	}
	next := cloneDB(s.db)
	next.Users = append(next.Users, storedUser{User: u, Password: hash})
	if err = s.save(next, session.User.ID, "user.create", u.ID); err != nil {
		return User{}, err
	}
	return u, nil
}

func (s *Service) UpdateUser(token, id string, role Role, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return err
	}
	if !Allowed(session.User.Role, ManageUsers) {
		return ErrForbidden
	}
	if !validRole(role) {
		return errors.New("invalid role")
	}
	// M-23: refuse to demote or disable the last active Owner
	// at the policy layer, before any save attempt. The lower
	// validateDatabase guard would otherwise fail the save and
	// the defer in save() would close the entire identity
	// service, leaving the box unmanageable.
	var currentRole Role
	var currentDisabled bool
	var found bool
	for _, u := range s.db.Users {
		if u.User.ID == id {
			currentRole = u.User.Role
			currentDisabled = u.User.Disabled
			found = true
			break
		}
	}
	if !found {
		return errors.New("user not found")
	}
	wasLastActiveOwner := currentRole == Owner && !currentDisabled
	becomesNonOwner := role != Owner || disabled
	if wasLastActiveOwner && becomesNonOwner {
		return errors.New("identity: last active Owner cannot be demoted or disabled")
	}
	next := cloneDB(s.db)
	for i := range next.Users {
		if next.Users[i].User.ID == id {
			next.Users[i].User.Role = role
			next.Users[i].User.Disabled = disabled
			break
		}
	}
	if err = s.save(next, session.User.ID, "user.update", id); err != nil {
		return err
	}
	for key, v := range s.sessions {
		if v.User.ID == id {
			delete(s.sessions, key)
		}
	}
	return nil
}
func (s *Service) ChangePassword(token, oldPassword, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return err
	}
	for i, u := range s.db.Users {
		if u.User.ID == session.User.ID {
			if !verifyPassword(u.Password, oldPassword) {
				return ErrUnauthorized
			}
			hash, e := hashPassword(newPassword)
			if e != nil {
				return e
			}
			next := cloneDB(s.db)
			next.Users[i].Password = hash
			if e = s.save(next, u.User.ID, "password.change", u.User.ID); e != nil {
				return e
			}
			for key, v := range s.sessions {
				if v.User.ID == u.User.ID {
					delete(s.sessions, key)
				}
			}
			return nil
		}
	}
	return ErrUnauthorized
}
func (s *Service) Audit(token string) ([]Audit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.session(token)
	if err != nil {
		return nil, err
	}
	if session.User.Role != Owner {
		return nil, ErrForbidden
	}
	return append([]Audit(nil), s.db.Audit...), nil
}
func (s *Service) session(token string) (Session, error) {
	if s.closed || len(token) != 64 {
		return Session{}, ErrUnauthorized
	}
	key := sha256.Sum256([]byte(token))
	session, ok := s.sessions[key]
	now := s.now()
	if !ok || !now.Before(session.ExpiresAt) || now.Sub(session.LastSeen) >= 30*time.Minute || now.Before(session.CreatedAt) {
		delete(s.sessions, key)
		return Session{}, ErrUnauthorized
	}
	session.LastSeen = now
	s.sessions[key] = session
	return session, nil
}
func (s *Service) prune(now time.Time) {
	for key, v := range s.sessions {
		if !now.Before(v.ExpiresAt) || now.Sub(v.LastSeen) >= 30*time.Minute || now.Before(v.CreatedAt) {
			delete(s.sessions, key)
		}
	}
}
func (s *Service) save(next database, actor, action, target string) (err error) {
	next.Audit = append(next.Audit, Audit{At: s.now().UTC(), Actor: actor, Action: action, Target: target})
	if len(next.Audit) > 256 {
		next.Audit = next.Audit[len(next.Audit)-256:]
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err = validateDatabase(data); err != nil {
		return err
	}
	// A durability failure may have committed on disk. Never keep authorizing
	// against the old in-memory identity policy in that ambiguous state.
	defer func() {
		if err != nil {
			s.closed = true
			clear(s.sessions)
		}
	}()
	snap, err := s.store.Snapshot()
	if err != nil {
		return err
	}
	tx, err := s.store.Prepare(snap.Current.ID, data, actor, action)
	if err != nil {
		return err
	}
	if err = s.store.Transition(tx.ID, state.Applying, time.Time{}); err != nil {
		return err
	}
	if err = s.store.Transition(tx.ID, state.AwaitingConfirmation, time.Now().Add(2*time.Minute)); err != nil {
		return err
	}
	if err = s.store.Commit(tx.ID); err != nil {
		return err
	}
	s.db = next
	return nil
}
func validateDatabase(raw json.RawMessage) error {
	var db database
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&db); err != nil {
		return errors.New("invalid identity database")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing identity database")
	}
	if db.Version != 1 || len(db.Users) > 64 || len(db.Audit) > 256 {
		return errors.New("invalid identity version or limits")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	owners := 0
	for _, u := range db.Users {
		id, e := hex.DecodeString(u.User.ID)
		if e != nil || len(id) != 16 || ids[u.User.ID] || names[u.User.Username] || !usernamePattern.MatchString(u.User.Username) || !validRole(u.User.Role) || !validHash(u.Password) {
			return errors.New("invalid identity entry")
		}
		ids[u.User.ID] = true
		names[u.User.Username] = true
		if u.User.Role == Owner && !u.User.Disabled {
			owners++
		}
	}
	if (len(db.Users) > 0 && owners == 0 && !db.OwnerRecovery) || (db.OwnerRecovery && owners != 0) {
		return errors.New("last active Owner must be preserved")
	}
	for _, a := range db.Audit {
		if a.At.IsZero() || len(a.Actor) != 32 || len(a.Target) != 32 || len(a.Action) > 48 {
			return errors.New("invalid audit entry")
		}
	}
	return nil
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
func cloneDB(d database) database {
	b, _ := json.Marshal(d)
	var next database
	_ = json.Unmarshal(b, &next)
	return next
}
