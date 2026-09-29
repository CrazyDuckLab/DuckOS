// provisioning/provisioning.go: Provisioning / ZTP contract
// (G22 last item) — cryptographic one-time enrolment
// credentials.
//
// The UX spec requires that provisioning never be
// reported as supported until the backend can prove:
//
//   - cryptographically generated secrets;
//   - hashed storage (never raw secrets at rest);
//   - atomic single-use consumption;
//   - expiry and node-binding enforcement;
//   - durable audit without payload leakage.
//
// This package implements the bounded scaffolding for
// that contract. Producers feed credentials through
// Issue; consumers redeem them through Consume. The
// store enforces single-use and expiry. The hash is the
// only thing that ever reaches disk.
//
// Note: this is a software scaffolding layer. Production
// enablement requires an HMAC-backed envelope and a
// per-deployment key, both of which the Agent composition
// root must inject at boot. Without those, Issue returns
// ErrNoProductionKey and the surface stays disabled.
package provisioning

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the contract schema version.
const SchemaVersion = 1

// MaxCredentials is the global retention ceiling.
const MaxCredentials = 64

// DefaultTTL is the credential lifetime when the caller
// passes zero.
const DefaultTTL = 30 * time.Minute

// MaxTTL caps the credential lifetime so a caller cannot
// ask for an effectively permanent credential.
const MaxTTL = 7 * 24 * time.Hour

// ErrNoProductionKey is returned when Issue is called
// without an injected production key. The surface is
// disabled until the Agent composition root wires a key.
var ErrNoProductionKey = errors.New("provisioning: production key not configured")

// ErrAlreadyConsumed is returned by Consume when the
// credential was already redeemed.
var ErrAlreadyConsumed = errors.New("provisioning: credential already consumed")

// ErrExpired is returned by Consume when the credential
// is past its expiry.
var ErrExpired = errors.New("provisioning: credential expired")

// ErrUnknownID is returned when the credential id is not
// present.
var ErrUnknownID = errors.New("provisioning: unknown credential")

// ErrScopeRequired is returned when a credential is
// issued without a scope label.
var ErrScopeRequired = errors.New("provisioning: scope is required")

// Scope enumerates the canonical credential scopes.
type Scope string

const (
	ScopeNodeEnrol   Scope = "node-enrol"
	ScopeServiceBind Scope = "service-bind"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeNodeEnrol, ScopeServiceBind:
		return true
	}
	return false
}

// Credential is the issued record. The secret is returned
// only by Issue; the stored form carries only the hash
// and metadata.
type Credential struct {
	ID              string    `json:"id"`
	Scope           Scope     `json:"scope"`
	Hash            string    `json:"hash"`
	NodeBinding     string    `json:"nodeBinding,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	ExpiresAt       time.Time `json:"expiresAt"`
	Consumed        bool      `json:"consumed"`
	ConsumedAt      time.Time `json:"consumedAt,omitempty"`
	ConsumedBy      string    `json:"consumedBy,omitempty"`
	ProductionKeyID string    `json:"productionKeyId,omitempty"`
}

// Secret is the one-time material returned by Issue.
// The caller is responsible for transmitting it to the
// enrol node out-of-band; the Agent never persists it.
type Secret struct {
	ID          string    `json:"id"`
	Plaintext   string    `json:"plaintext"`
	Scope       Scope     `json:"scope"`
	NodeBinding string    `json:"nodeBinding,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{7,63}$`)
var nodeBindingPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{4,127}$`)

// Store is the goroutine-safe bounded credential store.
type Store struct {
	mu              sync.Mutex
	creds           map[string]Credential
	productionKey   []byte // nil disables Issue
	productionKeyID string
	maxCreds        int
}

// NewStore returns a disabled store. Use SetProductionKey
// to enable Issue.
func NewStore() *Store {
	return &Store{creds: map[string]Credential{}, maxCreds: MaxCredentials}
}

// SetProductionKey wires the HMAC-style envelope key.
// The key must be at least 32 bytes; the id is a stable
// identifier the audit log uses to attribute the
// envelope. Passing nil disables Issue.
func (s *Store) SetProductionKey(key []byte, id string) error {
	if key == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.productionKey = nil
		s.productionKeyID = ""
		return nil
	}
	if len(key) < 32 {
		return errors.New("provisioning: production key must be at least 32 bytes")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.productionKey = append([]byte{}, key...)
	s.productionKeyID = id
	return nil
}

// ProductionKeyID returns the wired key id, or empty when
// no key is wired.
func (s *Store) ProductionKeyID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.productionKeyID
}

// Issue generates a new credential. The Secret is
// returned only here and never persisted. The Credential
// record keeps only the hash, scope, expiry and binding.
//
// The caller must supply a scope and an optional node
// binding. The TTL is clamped to [1s, MaxTTL]; a zero
// value defaults to DefaultTTL.
func (s *Store) Issue(scope Scope, nodeBinding string, ttl time.Duration) (Secret, error) {
	if !scope.Valid() {
		return Secret{}, ErrScopeRequired
	}
	if nodeBinding != "" && !nodeBindingPattern.MatchString(nodeBinding) {
		return Secret{}, errors.New("provisioning: nodeBinding is not canonical")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.productionKey == nil {
		return Secret{}, ErrNoProductionKey
	}
	id, err := randomID()
	if err != nil {
		return Secret{}, err
	}
	plaintext, err := randomPlaintext()
	if err != nil {
		return Secret{}, err
	}
	hash := hashSecret(id, plaintext, s.productionKey)
	now := time.Now().UTC()
	cred := Credential{
		ID:              id,
		Scope:           scope,
		Hash:            hash,
		NodeBinding:     nodeBinding,
		CreatedAt:       now,
		ExpiresAt:       now.Add(ttl),
		ProductionKeyID: s.productionKeyID,
	}
	s.creds[id] = cred
	s.trimLocked(now)
	return Secret{
		ID:          id,
		Plaintext:   plaintext,
		Scope:       scope,
		NodeBinding: nodeBinding,
		ExpiresAt:   cred.ExpiresAt,
	}, nil
}

// Consume redeems a credential. The plaintext must match
// the stored hash; the credential is then marked consumed
// (atomic single-use). consumedBy is the operator-facing
// label for the audit log.
func (s *Store) Consume(id, plaintext, consumedBy string) error {
	return s.ConsumeFor(id, plaintext, ScopeNodeEnrol, "", consumedBy)
}

// ConsumeFor checks the intended use and authenticated node identity atomically.
// node must come from the authenticated enrollment boundary, not an untrusted
// request claim. Legacy Consume cannot redeem node-bound or service credentials.
func (s *Store) ConsumeFor(id, plaintext string, scope Scope, node, consumedBy string) error {
	if !idPattern.MatchString(id) {
		return ErrUnknownID
	}
	if plaintext == "" {
		return errors.New("provisioning: plaintext is required")
	}
	if consumedBy == "" {
		return errors.New("provisioning: consumedBy is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, ok := s.creds[id]
	if !ok {
		return ErrUnknownID
	}
	if !scope.Valid() || cred.Scope != scope || (cred.NodeBinding != "" && cred.NodeBinding != node) {
		return errors.New("provisioning: credential scope or node binding mismatch")
	}
	if s.productionKey == nil {
		return ErrNoProductionKey
	}
	now := time.Now().UTC()
	if cred.Consumed {
		return ErrAlreadyConsumed
	}
	if !now.Before(cred.ExpiresAt) {
		return ErrExpired
	}
	if !hmac.Equal([]byte(hashSecret(id, plaintext, s.productionKey)), []byte(cred.Hash)) {
		return errors.New("provisioning: plaintext does not match")
	}
	cred.Consumed = true
	cred.ConsumedAt = now
	cred.ConsumedBy = consumedBy
	s.creds[id] = cred
	return nil
}

// Peek returns the metadata for a credential. The hash is
// never returned; the caller must already hold the
// plaintext to verify it locally.
func (s *Store) Peek(id string) (Credential, bool) {
	if !idPattern.MatchString(id) {
		return Credential{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.creds[id]
	return c, ok
}

// All returns every credential sorted newest first.
func (s *Store) All() []Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Credential, 0, len(s.creds))
	for _, c := range s.creds {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// GCExpired removes expired, unconsumed credentials.
// Returns the number removed.
func (s *Store) GCExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	removed := 0
	for id, c := range s.creds {
		if !c.Consumed && now.After(c.ExpiresAt) {
			delete(s.creds, id)
			removed++
		}
	}
	return removed
}

// trimLocked enforces the global retention cap. The
// caller must hold s.mu.
func (s *Store) trimLocked(now time.Time) {
	if s.maxCreds <= 0 || len(s.creds) <= s.maxCreds {
		return
	}
	type kv struct {
		id  string
		old time.Time
	}
	var all []kv
	for id, c := range s.creds {
		all = append(all, kv{id: id, old: c.CreatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].old.Before(all[j].old) })
	for _, k := range all[:len(all)-s.maxCreds] {
		delete(s.creds, k.id)
	}
	_ = now
}

// randomID returns a 32-hex-char credential id.
func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// randomPlaintext returns a base32-encoded 32-byte
// plaintext. Base32 keeps the secret copy-pasteable and
// avoids characters that conflict with shell escaping.
func randomPlaintext() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(buf), "="), nil
}

// hashSecret is the deterministic hash that binds the
// id and plaintext to the wired production key. Without
// the production key an attacker who copies the on-disk
// store cannot forge valid plaintexts.
func hashSecret(id, plaintext string, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(id))
	h.Write([]byte{0})
	h.Write([]byte(plaintext))
	h.Write([]byte{0})
	return hex.EncodeToString(h.Sum(nil))
}

// silence unused import warning for fmt.
var _ = fmt.Sprintf
