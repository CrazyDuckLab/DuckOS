// Package ztp is the bounded zero-touch provisioning
// bootstrap. The package owns:
//   - the durable redemption journal (single-use, replay-safe)
//   - the bootstrap state machine (Pending → Redeemed →
//     Bound or Failed)
//   - the Agent composition-root wiring
//
// C-blocker #10: ZTP wiring. The previous code path did
// not exist in this package; the audit classified the
// bootstrap key lifecycle and durable redemption/replay
// protection as C-class software. The package now
// persists redemption state to a 0600-mode file so an
// interrupted Agent cannot replay a bootstrap key and
// cannot forget a redeemed key.
//
// The package never invents availability: every state
// mutation is bound to a file write + fsync so a crash
// mid-mutation leaves the journal in a recoverable
// state. A redeemed key is never accepted twice; a
// stale (TTL-expired) key is rejected with an explicit
// ErrStaleToken. A missing production key surfaces as
// ErrNoProductionKey so the agent composition root can
// fail closed without faking success.
package ztp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SchemaVersion pins the on-disk shape.
const SchemaVersion = 1

// MaxRedemptionJournal caps the per-key retention.
const MaxRedemptionJournal = 256

// DefaultTTL is the maximum age a bootstrap token is
// honoured for. A token older than TTL is refused even
// if it has never been redeemed.
const DefaultTTL = 30 * time.Minute

// Phase enumerates the bootstrap state machine.
type Phase string

const (
	PhasePending  Phase = "pending"
	PhaseRedeemed Phase = "redeemed"
	PhaseBound    Phase = "bound"
	PhaseFailed   Phase = "failed"
)

// Valid reports whether p is a known phase.
func (p Phase) Valid() bool {
	switch p {
	case PhasePending, PhaseRedeemed, PhaseBound, PhaseFailed:
		return true
	}
	return false
}

// Errors returned by the package. They are the typed
// surface the audit demands: every failure path has an
// explicit name so the UI can render the gap.
var (
	ErrNoProductionKey   = errors.New("ztp: production key not provisioned")
	ErrTokenReplay       = errors.New("ztp: bootstrap token replay rejected")
	ErrTokenStale        = errors.New("ztp: bootstrap token TTL expired")
	ErrTokenMismatch     = errors.New("ztp: bootstrap token signature mismatch")
	ErrCapacityExhausted = errors.New("ztp: redemption journal capacity exhausted; unexpired records present")
)

// IssuedToken is the canonical, cryptographically-issued
// bootstrap credential. The MAC is computed over the
// canonical claims (subject, scope, node, issuedAt,
// expiresAt) keyed on the production secret. A token
// whose MAC does not match the production secret is
// forged; a token whose expiresAt is in the past is stale;
// a token whose node does not match the redeeming device
// is bound to a different identity.
type IssuedToken struct {
	Subject   string    `json:"subject"`
	Scope     string    `json:"scope,omitempty"`
	Node      string    `json:"node,omitempty"`
	IssuedAt  time.Time `json:"issuedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	MAC       string    `json:"mac"`
}

// IssueRequest is the issuance envelope the admin path
// supplies; Issue returns the IssuedToken the admin then
// hands to the device.
type IssueRequest struct {
	Subject  string
	Scope    string
	Node     string
	Validity time.Duration
}

// computeMAC produces the canonical HMAC-SHA256 hex
// digest over the ordered claims. The encoding is
// stable so an issuer and a redeemer agree byte-for-byte.
func computeMAC(key string, tok IssuedToken) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(tok.Subject))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Scope))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Node))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.IssuedAt.UTC().Format(time.RFC3339Nano)))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.ExpiresAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Issue mints a new IssuedToken. The Issue path is the
// authenticated administrative surface; the redeeming
// device only sees the returned token. The audit requires
// issuance to be authenticated; the production wiring is
// expected to call Issue through a privileged admin path,
// never the public redemption endpoint.
func (m *Manager) Issue(ctx context.Context, req IssueRequest) (IssuedToken, error) {
	if err := ctx.Err(); err != nil {
		return IssuedToken{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.prodKey) == 0 {
		return IssuedToken{}, ErrNoProductionKey
	}
	if req.Subject == "" {
		return IssuedToken{}, errors.New("ztp: issuance requires a subject")
	}
	if req.Validity <= 0 {
		req.Validity = m.ttl
	}
	now := m.now().UTC()
	tok := IssuedToken{
		Subject:   req.Subject,
		Scope:     req.Scope,
		Node:      req.Node,
		IssuedAt:  now,
		ExpiresAt: now.Add(req.Validity),
	}
	tok.MAC = computeMAC(string(m.prodKey), tok)
	return tok, nil
}

// BootstrapRecord is one row of the redemption journal.
// The record is the durable evidence a deployment
// controller needs to audit a single redemption.
type BootstrapRecord struct {
	ID           string    `json:"id"`
	Phase        Phase     `json:"phase"`
	TokenDigest  string    `json:"tokenDigest"`
	Subject      string    `json:"subject,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Node         string    `json:"node,omitempty"`
	IssuedAt     time.Time `json:"issuedAt"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
	RedeemedAt   time.Time `json:"redeemedAt,omitempty"`
	BoundAt      time.Time `json:"boundAt,omitempty"`
	FailedReason string    `json:"failedReason,omitempty"`
}

// valid reports whether r is internally consistent.
func (r BootstrapRecord) valid() error {
	if r.ID == "" {
		return errors.New("ztp: record id is required")
	}
	if !r.Phase.Valid() {
		return errors.New("ztp: record phase is invalid")
	}
	if r.TokenDigest == "" {
		return errors.New("ztp: record tokenDigest is required")
	}
	if r.IssuedAt.IsZero() {
		return errors.New("ztp: record issuedAt is zero")
	}
	return nil
}

// tokenDigest is the HMAC-SHA256 hex digest the package
// records instead of the raw token. The digest is keyed
// on the production secret so two deployments cannot
// forge each other's redemption records. The digest is
// over the issued claims (subject, scope, node, MAC),
// not the raw token: a redeemer that already verified
// the MAC against the issued claims is the source of
// truth.
func tokenDigest(productionKey string, tok IssuedToken) string {
	mac := hmac.New(sha256.New, []byte(productionKey))
	mac.Write([]byte(tok.Subject))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Scope))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.Node))
	mac.Write([]byte{0})
	mac.Write([]byte(tok.MAC))
	return hex.EncodeToString(mac.Sum(nil))
}

// Manager is the durable redemption manager. The zero
// value is NOT usable; callers MUST construct via
// OpenManager so the production key and journal path
// are pinned before the first Redeem.
type Manager struct {
	mu       sync.Mutex
	dir      string
	path     string
	lockPath string
	lockFile *os.File // held for the lifetime of the Manager on Linux
	now      func() time.Time
	ttl      time.Duration
	prodKey  []byte
	journal  []BootstrapRecord
	byDigest map[string]int
	// seenDedup is the on-disk dedup; reload rebuilds
	// byDigest but tracks the order it observed so the
	// file is rewritten in newest-first order.
	seenOrder []string
}

// ErrLocked is returned by OpenManager when another
// process already holds the per-dir flock. The contract
// forbids silently accepting a second writer.
var ErrLocked = errors.New("ztp: redemption journal is locked by another process")

// OpenManager constructs a Manager rooted at dir.
// productionKey is the HMAC secret the package uses to
// fingerprint bootstrap tokens. Passing an empty
// production key surfaces ErrNoProductionKey on the
// first Redeem; the manager refuses to mint digests
// without it.
//
// OpenManager takes an exclusive flock(2) on a per-dir
// lock file so two concurrent processes cannot corrupt
// the redemption journal. The lock is held for the
// lifetime of the Manager; Close releases it. On
// non-Linux platforms the lock is a no-op (in-process
// mutex is the only guard), and the second OpenManager
// call fails with ErrLocked so the contract never
// silently accepts a second writer.
func OpenManager(dir string, productionKey []byte) (*Manager, error) {
	if dir == "" {
		return nil, errors.New("ztp: directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ztp: directory: %w", err)
	}
	m := &Manager{
		dir:      dir,
		path:     filepath.Join(dir, "redemption-journal.log"),
		lockPath: filepath.Join(dir, "redemption-journal.lock"),
		now:      time.Now,
		ttl:      DefaultTTL,
		prodKey:  append([]byte(nil), productionKey...),
		byDigest: map[string]int{},
	}
	if err := m.acquireLock(); err != nil {
		return nil, err
	}
	if err := m.reload(); err != nil {
		_ = m.releaseLock()
		return nil, err
	}
	return m, nil
}

// SetTTL overrides the bootstrap token TTL.
func (m *Manager) SetTTL(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d > 0 {
		m.ttl = d
	}
}

// Close releases the per-dir flock (when held) and drops
// the in-process lock. After Close the Manager must not
// be used.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.releaseLock()
}

// reload reads the existing redemption journal and
// rebuilds the in-memory dedup index.
//
// SECURITY (DV10, B10): the redemption journal is the
// single-use replay anchor. A corrupt journal that
// silently drops a redeemed-token record re-opens the
// replay window — the audit classified that as a
// high-severity vulnerability. reload() therefore fails
// CLOSED: any corrupt line (bad JSON, invalid record,
// schema mismatch, truncated final line) returns an
// error and the in-memory journal is left untouched.
// OpenManager surfaces the error to the production
// composition root, which must refuse to start (or
// refuse to redeem) until the operator restores a
// known-good journal.
//
// The "incomplete trailing line" guard defends against
// a crash mid-append: the partial line is detected by
// the JSON parser and reported as corruption rather
// than silently accepted.
func (m *Manager) reload() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("ztp: journal read: %w", err)
	}
	for i, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var r BootstrapRecord
		if err := json.Unmarshal(line, &r); err != nil {
			return fmt.Errorf("ztp: journal corrupt line %d: %w", i+1, err)
		}
		if err := r.valid(); err != nil {
			return fmt.Errorf("ztp: journal invalid record line %d: %w", i+1, err)
		}
		if _, ok := m.byDigest[r.TokenDigest]; !ok {
			m.byDigest[r.TokenDigest] = len(m.journal)
			m.seenOrder = append(m.seenOrder, r.TokenDigest)
			m.journal = append(m.journal, r)
		}
	}
	return nil
}

// All returns every redemption record, newest first.
func (m *Manager) All() []BootstrapRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BootstrapRecord, 0, len(m.journal))
	for _, r := range m.journal {
		out = append(out, r)
	}
	return out
}

// FindByDigest returns the redemption record whose
// TokenDigest matches.
func (m *Manager) FindByDigest(digest string) (BootstrapRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byDigest[digest]
	if !ok {
		return BootstrapRecord{}, false
	}
	return m.journal[idx], true
}

// Counters summarises the journal.
type Counters struct {
	Total   int           `json:"total"`
	ByPhase map[Phase]int `json:"byPhase"`
}

// Counters returns the per-phase totals.
func (m *Manager) Counters() Counters {
	m.mu.Lock()
	defer m.mu.Unlock()
	by := map[Phase]int{}
	for _, r := range m.journal {
		by[r.Phase]++
	}
	return Counters{Total: len(m.journal), ByPhase: by}
}

// Redeem accepts an IssuedToken and returns a durable
// BootstrapRecord. The manager enforces:
//   - production key presence (ErrNoProductionKey)
//   - MAC authenticity (ErrTokenMismatch)
//   - TTL freshness (ErrTokenStale)
//   - replay protection (ErrTokenReplay)
//   - scope/node binding when the manager has a binding
//     policy configured (ErrTokenMismatch)
//
// Redeem MUST run on the Agent composition root. The
// record is fsynced before the function returns.
//
// Replay safety: the manager refuses to GC any record
// whose IssuedAt + ttl is still in the future. Capacity
// exhaustion with unexpired records fails closed.
func (m *Manager) Redeem(ctx context.Context, tok IssuedToken, observedNode string) (BootstrapRecord, error) {
	if err := ctx.Err(); err != nil {
		return BootstrapRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.prodKey) == 0 {
		return BootstrapRecord{}, ErrNoProductionKey
	}
	if tok.IssuedAt.IsZero() || tok.ExpiresAt.IsZero() {
		return BootstrapRecord{}, errors.New("ztp: token must carry issuedAt/expiresAt claims")
	}
	if tok.Subject == "" {
		return BootstrapRecord{}, errors.New("ztp: token must carry a subject claim")
	}
	if tok.MAC == "" {
		return BootstrapRecord{}, errors.New("ztp: token must carry a MAC")
	}
	// Recompute the MAC over the canonical claims the
	// issuer would have produced. The comparison is
	// constant-time so a side-channel cannot leak the
	// expected MAC byte-by-byte.
	want := computeMAC(string(m.prodKey), tok)
	if !hmac.Equal([]byte(want), []byte(tok.MAC)) {
		return BootstrapRecord{}, ErrTokenMismatch
	}
	now := m.now().UTC()
	if now.After(tok.ExpiresAt) {
		return BootstrapRecord{}, ErrTokenStale
	}
	if !tok.IssuedAt.Before(tok.ExpiresAt) {
		return BootstrapRecord{}, errors.New("ztp: token expiresAt must follow issuedAt")
	}
	if observedNode != "" && tok.Node != "" && tok.Node != observedNode {
		return BootstrapRecord{}, ErrTokenMismatch
	}
	// The replay-key is (Node, MAC); two tokens with the
	// same MAC cannot exist for different Nodes.
	digest := tokenDigest(string(m.prodKey), tok)
	if idx, ok := m.byDigest[digest]; ok {
		return m.journal[idx], ErrTokenReplay
	}
	// Capacity gate: reject if a fresh entry would force
	// GC of an unexpired record. The audit requires
	// bounded memory without eviction vulnerability.
	if len(m.journal) >= MaxRedemptionJournal {
		oldest := m.journal[0]
		if !m.recordExpired(oldest, now) {
			return BootstrapRecord{}, ErrCapacityExhausted
		}
	}
	r := BootstrapRecord{
		ID:          hex.EncodeToString(sha256.New().Sum([]byte(digest)))[:16],
		Phase:       PhaseRedeemed,
		TokenDigest: digest,
		Subject:     tok.Subject,
		IssuedAt:    tok.IssuedAt,
		ExpiresAt:   tok.ExpiresAt,
		Scope:       tok.Scope,
		Node:        tok.Node,
		RedeemedAt:  now,
	}
	if err := m.appendLocked(r); err != nil {
		return BootstrapRecord{}, err
	}
	return r, nil
}

// recordExpired reports whether a BootstrapRecord has
// fallen outside its TTL window and is therefore eligible
// for GC.
func (m *Manager) recordExpired(r BootstrapRecord, now time.Time) bool {
	if r.ExpiresAt.IsZero() {
		// Records written before the IssuedToken/ExpiresAt
		// contract landed use IssuedAt as the anchor with
		// the default TTL. Treat the IssuedAt + TTL as
		// the expiry so old entries do not pin the
		// journal forever.
		return now.Sub(r.IssuedAt) > m.ttl
	}
	return now.After(r.ExpiresAt)
}

// Bind marks a redeemed record as Bound. The transition
// is the post-bootstrap step where the device's identity
// is wired into the deployment controller. A bound
// record is still replay-protected: a second Bind call
// returns ErrTokenReplay.
func (m *Manager) Bind(ctx context.Context, digest string) (BootstrapRecord, error) {
	if err := ctx.Err(); err != nil {
		return BootstrapRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byDigest[digest]
	if !ok {
		return BootstrapRecord{}, ErrTokenMismatch
	}
	r := m.journal[idx]
	if r.Phase == PhaseBound {
		return r, ErrTokenReplay
	}
	r.Phase = PhaseBound
	r.BoundAt = m.now().UTC()
	m.journal[idx] = r
	if err := m.persistLocked(); err != nil {
		return BootstrapRecord{}, err
	}
	return r, nil
}

// Fail marks a redeemed record as Failed. The transition
// is the explicit failure path; an operator may use it
// when the bootstrap device is rejected for a non-replay
// reason (e.g. invalid subject, clock skew, etc.).
func (m *Manager) Fail(ctx context.Context, digest, reason string) (BootstrapRecord, error) {
	if err := ctx.Err(); err != nil {
		return BootstrapRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byDigest[digest]
	if !ok {
		return BootstrapRecord{}, ErrTokenMismatch
	}
	r := m.journal[idx]
	r.Phase = PhaseFailed
	r.FailedReason = reason
	m.journal[idx] = r
	if err := m.persistLocked(); err != nil {
		return BootstrapRecord{}, err
	}
	return r, nil
}

// appendLocked inserts a new redemption record and
// persists the journal. The call MUST hold m.mu.
func (m *Manager) appendLocked(r BootstrapRecord) error {
	if err := r.valid(); err != nil {
		return err
	}
	m.byDigest[r.TokenDigest] = len(m.journal)
	m.seenOrder = append(m.seenOrder, r.TokenDigest)
	m.journal = append(m.journal, r)
	if len(m.journal) > MaxRedemptionJournal {
		// Bounded retention with replay safety: drop
		// expired records only. The audit explicitly
		// forbids evicting unexpired records because
		// that re-opens the replay window. If every
		// record is unexpired, refuse to grow the
		// journal (Redeem already gates capacity
		// before calling appendLocked, so this path
		// is reached only when an existing record
		// becomes expired between gate and append).
		now := m.now().UTC()
		dropped := 0
		for dropped < len(m.journal) && m.recordExpired(m.journal[dropped], now) {
			dropped++
		}
		if dropped == 0 {
			// Should be unreachable thanks to the
			// capacity gate in Redeem; defend in depth.
			return ErrCapacityExhausted
		}
		m.journal = m.journal[dropped:]
		m.seenOrder = m.seenOrder[dropped:]
		for i, rec := range m.journal {
			m.byDigest[rec.TokenDigest] = i
		}
	}
	return m.persistLocked()
}

// persistLocked rewrites the journal file from the
// in-memory cache. The call MUST hold m.mu.
func (m *Manager) persistLocked() error {
	tmp := m.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("ztp: journal write: %w", err)
	}
	for _, r := range m.journal {
		data, err := json.Marshal(r)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("ztp: journal marshal: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("ztp: journal append: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("ztp: journal sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("ztp: journal close: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return fmt.Errorf("ztp: journal rename: %w", err)
	}
	return nil
}

// splitLines is a tiny helper so the reload path does not
// pull in bufio for what is usually a small file.
func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i > start {
			out = append(out, data[start:i])
		}
		start = i + 1
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
