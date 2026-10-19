// Package subscription / delivery.go: the public subscription
// delivery surface. The handler serves GET <prefix>/<opaque-token>
// and nothing else: it never exposes administrative assets, API
// capabilities or metadata beyond the rendered payload, so the
// public surface stays isolated from the future Stealth Admin gate.
//
// Security posture:
//
//   - Tokens are 32 cryptographically random bytes, base64url
//     encoded. Only SHA-256 token digests are stored; a database
//     disclosure does not reveal usable URLs.
//   - Every failure mode (unknown, revoked, expired, disabled)
//     returns the identical 404 with the identical body, so the
//     endpoint discloses nothing about token existence.
//   - A bounded fixed-window rate limiter per token digest and per
//     client IP throttles brute forcing; limiter state is pruned so
//     memory stays bounded on low-power hardware.
//   - Access logs never receive the token; only the first 8 hex
//     characters of its digest are recorded for correlation.
//   - Constant-time comparison is used on the final digest match.
package subscription

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

// MaxTokenBytes is the entropy budget of a delivery token.
const MaxTokenBytes = 32

// tokenPrefixVersion is mixed into the stored digest so a token can
// never be replayed into another context (e.g. an admin session).
const tokenDigestDomain = "DuckOS/subscription/delivery-token/v1"

// TokenRecord is the at-rest form of an issued delivery token. Only
// the digest is persisted.
type TokenRecord struct {
	SubID      string    `json:"subId"`
	TokenHash  string    `json:"tokenHash"`
	IssuedAt   time.Time `json:"issuedAt"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
	Revoked    bool      `json:"revoked"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	UseCount   uint64    `json:"useCount"`
}

// TokenStore keeps the digest-indexed token table.
type TokenStore struct {
	mu     sync.RWMutex
	byHash map[string]*TokenRecord
	clock  func() time.Time
	// persist commits the full record set durably and reports the
	// commit phase. NOT_COMMITTED → the mutation rolls back; an
	// UNCERTAIN outcome (post-rename fsync failure) KEEPS the new
	// restrictive state, marks the store degraded and blocks all
	// further mutations — writing over an uncertain snapshot could
	// resurrect revoked URLs after a restart.
	persist func([]TokenRecord) (persist.CommitOutcome, error)
	// degraded carries the active commit-uncertainty diagnosis;
	// while set, every mutation is refused until reconciliation.
	degraded string
}

func NewTokenStore() *TokenStore {
	return &TokenStore{byHash: map[string]*TokenRecord{}, clock: time.Now}
}

// SetPersist installs the durability hook with commit-phase
// reporting.
func (s *TokenStore) SetPersist(fn func([]TokenRecord) (persist.CommitOutcome, error)) {
	s.persist = fn
}

// Degraded reports the active commit-uncertainty diagnosis.
func (s *TokenStore) Degraded() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.degraded
}

// Reconcile resolves uncertainty: the disk snapshot becomes
// authoritative and the store returns to the healthy state. If the
// disk is older than the uncertain in-memory state, the restrictive
// in-memory state was NOT durably committed — the operator must
// re-revoke; this is reported, never silenced.
func (s *TokenStore) Reconcile(records []TokenRecord) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded == "" {
		return "healthy"
	}
	s.byHash = map[string]*TokenRecord{}
	for i := range records {
		rec := records[i]
		s.byHash[rec.TokenHash] = &rec
	}
	s.degraded = ""
	return "recovered: disk state adopted (verify recent revocations)"
}

func (s *TokenStore) persistLocked() (persist.CommitOutcome, error) {
	if s.persist == nil {
		return persist.CommitOK, nil
	}
	out := make([]TokenRecord, 0, len(s.byHash))
	for _, rec := range s.byHash {
		out = append(out, *rec)
	}
	return s.persist(out)
}

func (s *TokenStore) requireHealthyLocked() error {
	if s.degraded != "" {
		return fmt.Errorf("subscription: storage degraded (%s); reconciliation required before mutations", s.degraded)
	}
	return nil
}

// LoadRecords seeds the store from persisted state (startup).
func (s *TokenStore) LoadRecords(records []TokenRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range records {
		rec := records[i]
		s.byHash[rec.TokenHash] = &rec
	}
}

func (s *TokenStore) snapshot() []TokenRecord {
	out := make([]TokenRecord, 0, len(s.byHash))
	for _, rec := range s.byHash {
		out = append(out, *rec)
	}
	return out
}

// hashToken derives the at-rest digest of a presentation token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(tokenDigestDomain + "\x00" + token))
	return hex.EncodeToString(sum[:])
}

// NewToken returns a fresh opaque presentation token.
func NewToken() (string, error) {
	buf := make([]byte, MaxTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("subscription: generate delivery token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// Issue mints a new token for the subscription and records its
// digest. The presentation token is returned exactly once.
func (s *TokenStore) Issue(subID string, ttl time.Duration) (string, TokenRecord, error) {
	if err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.requireHealthyLocked()
	}(); err != nil {
		return "", TokenRecord{}, err
	}
	if subID == "" {
		return "", TokenRecord{}, errors.New("subscription: issue requires a subscription id")
	}
	token, err := NewToken()
	if err != nil {
		return "", TokenRecord{}, err
	}
	now := s.clock().UTC()
	rec := TokenRecord{SubID: subID, TokenHash: hashToken(token), IssuedAt: now}
	if ttl != 0 {
		rec.ExpiresAt = now.Add(ttl)
	}
	s.mu.Lock()
	// Bound the store: drop expired/revoked records beyond capacity.
	if len(s.byHash) >= 8192 {
		for h, r := range s.byHash {
			if r.Revoked || (!r.ExpiresAt.IsZero() && now.After(r.ExpiresAt)) {
				delete(s.byHash, h)
			}
		}
	}
	s.byHash[rec.TokenHash] = &rec
	outcome, perr := s.persistLocked()
	if perr != nil {
		// AUD-02: the issuance outcome is never discarded.
		// NOT_COMMITTED → roll back, the token never existed.
		// COMMIT_UNCERTAIN → keep the record (it may be on disk),
		// mark the store DEGRADED and fail the operation; the
		// token must NOT be handed out as valid while the
		// authoritative state is uncertain.
		if outcome != persist.CommitUncertain {
			delete(s.byHash, rec.TokenHash)
		} else {
			s.degraded = fmt.Sprintf("commit_uncertain on token issue: %v", perr)
		}
		s.mu.Unlock()
		if outcome == persist.CommitUncertain {
			return "", TokenRecord{}, fmt.Errorf("subscription: token commit outcome UNCERTAIN; token not issued: %w", perr)
		}
		return "", TokenRecord{}, fmt.Errorf("subscription: token not committed durably: %w", perr)
	}
	s.mu.Unlock()
	return token, rec, nil
}

// Lookup resolves a presentation token to its record. The final
// comparison is constant-time.
func (s *TokenStore) Lookup(token string) (TokenRecord, bool) {
	if token == "" || len(token) > 128 {
		return TokenRecord{}, false
	}
	digest := hashToken(token)
	s.mu.RLock()
	rec, ok := s.byHash[digest]
	if ok {
		rc := *rec
		if !rc.Revoked &&
			(rc.ExpiresAt.IsZero() || s.clock().Before(rc.ExpiresAt)) &&
			hmac.Equal([]byte(rc.TokenHash), []byte(digest)) {
			rc.UseCount++
			rec = &rc
		} else {
			ok, rec = false, nil
		}
	} else {
		rec = nil
	}
	s.mu.RUnlock()
	if !ok {
		return TokenRecord{}, false
	}
	// Last-used metadata is advisory; a failed write is harmless.
	s.mu.Lock()
	if live, ok := s.byHash[digest]; ok {
		live.LastUsedAt = s.clock().UTC()
		live.UseCount++
	}
	s.mu.Unlock()
	return *rec, true
}

// Rotate revokes every active token of the subscription and issues
// a replacement. The revocation part is transactional: if the
// durable commit fails, the previous tokens stay active and the
// error propagates — the caller never sees a "rotated" success
// whose old URLs would resurrect after a restart.
func (s *TokenStore) Rotate(subID string, ttl time.Duration) (string, TokenRecord, error) {
	s.mu.Lock()
	if err := s.requireHealthyLocked(); err != nil {
		s.mu.Unlock()
		return "", TokenRecord{}, err
	}
	var prevRevoked []string
	for _, rec := range s.byHash {
		if rec.SubID == subID && !rec.Revoked {
			rec.Revoked = true
			prevRevoked = append(prevRevoked, rec.TokenHash)
		}
	}
	outcome, perr := s.persistLocked()
	if perr != nil {
		if outcome != persist.CommitUncertain {
			for _, h := range prevRevoked {
				if rec, ok := s.byHash[h]; ok {
					rec.Revoked = false
				}
			}
		} else {
			s.degraded = fmt.Sprintf("commit_uncertain on rotation: %v", perr)
		}
		s.mu.Unlock()
		if outcome == persist.CommitUncertain {
			return "", TokenRecord{}, fmt.Errorf("subscription: rotation commit outcome UNCERTAIN; restrictive state retained: %w", perr)
		}
		return "", TokenRecord{}, fmt.Errorf("subscription: rotation not committed durably: %w", perr)
	}
	s.mu.Unlock()
	token, rec, err := s.Issue(subID, ttl)
	if err != nil {
		return "", TokenRecord{}, err
	}
	return token, rec, nil
}

// RevokeSubscription revokes every token of a subscription
// (suspend / delete path).
func (s *TokenStore) RevokeSubscription(subID string) (int, error) {
	s.mu.Lock()
	n := 0
	var prevRevoked []string
	for _, rec := range s.byHash {
		if rec.SubID == subID && !rec.Revoked {
			rec.Revoked = true
			prevRevoked = append(prevRevoked, rec.TokenHash)
			n++
		}
	}
	outcome, perr := s.persistLocked()
	if perr != nil {
		if outcome != persist.CommitUncertain {
			for _, h := range prevRevoked {
				if rec, ok := s.byHash[h]; ok {
					rec.Revoked = false
				}
			}
		} else {
			s.degraded = fmt.Sprintf("commit_uncertain on revocation: %v", perr)
		}
		s.mu.Unlock()
		if outcome == persist.CommitUncertain {
			return 0, fmt.Errorf("subscription: revocation commit outcome UNCERTAIN; restrictive state retained: %w", perr)
		}
		return 0, fmt.Errorf("subscription: revocation not committed durably: %w", perr)
	}
	s.mu.Unlock()
	return n, nil
}

// TokenCount reports the active token count for a subscription.
func (s *TokenStore) TokenCount(subID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	now := s.clock()
	for _, rec := range s.byHash {
		if rec.SubID == subID && !rec.Revoked &&
			(rec.ExpiresAt.IsZero() || now.Before(rec.ExpiresAt)) {
			n++
		}
	}
	return n
}

// rateLimiter is a bounded fixed-window counter keyed by string
// (token digest or client IP). Windows are pruned lazily; total
// tracked keys are capped so low-power hardware is protected.
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string]windowHit
	clock  func() time.Time
}

type windowHit struct {
	count int
	start time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{window: window, limit: limit, hits: map[string]windowHit{}, clock: time.Now}
}

// allow reports whether key may proceed within the current window.
func (l *rateLimiter) allow(key string) bool {
	now := l.clock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) >= 4096 {
		for k, h := range l.hits {
			if now.Sub(h.start) >= l.window {
				delete(l.hits, k)
			}
		}
		if _, ok := l.hits[key]; !ok && len(l.hits) >= 4096 {
			// Saturated with live windows: refuse new keys only.
			return false
		}
	}
	h, ok := l.hits[key]
	if !ok || now.Sub(h.start) >= l.window {
		l.hits[key] = windowHit{count: 1, start: now}
		return true
	}
	h.count++
	l.hits[key] = h
	return h.count <= l.limit
}

// DeliveryHandler serves the public subscription endpoint.
type DeliveryHandler struct {
	Store *TokenStore
	// Render produces the default (base64) payload for a
	// subscription id. It must internally refuse disabled/expired
	// subscriptions; any error becomes the uniform 404 so state
	// never leaks.
	Render func(subID string) ([]byte, error)
	// RenderFormat, when wired, produces the payload in the
	// requested format. The ?format= query value is validated
	// against the Formats allow-list before this is called.
	RenderFormat func(subID string, format Format) ([]byte, error)
	// Validate, when wired, authoritatively validates a presentation
	// token for THIS request (production wiring: the agent-side
	// store over the restricted IPC). Revocation and rotation become
	// effective immediately — no daemon restart, no cache. When nil,
	// the local Store is consulted (single-process deployments).
	Validate func(token string) (subID string, ok bool)
	// Prefix is the mounted path, default "/sub".
	Prefix string
	// RequestsPerMinute is the per-token and per-IP limit.
	RequestsPerMinute int
	// Userinfo optionally renders the standard
	// Subscription-Userinfo header (upload/download/total/expire).
	Userinfo func(subID string) string
	limiter  *rateLimiter
}

func NewDeliveryHandler(store *TokenStore, render func(subID string) ([]byte, error)) *DeliveryHandler {
	return &DeliveryHandler{
		Store:             store,
		Render:            render,
		Prefix:            "/sub",
		RequestsPerMinute: 30,
		limiter:           newRateLimiter(30, time.Minute),
	}
}

// uniform404 is the single failure response. Body and headers are
// constant across all failure modes.
func uniform404(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("not found\n"))
}

func (h *DeliveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET")
		uniform404(w)
		return
	}
	format := Format(r.URL.Query().Get("format"))
	if r.URL.Query().Get("format") != "" && !Formats[format] {
		uniform404(w)
		return
	}
	if format == "" {
		format = FormatBase64
	}
	path := strings.TrimPrefix(r.URL.Path, h.Prefix)
	token := strings.Trim(path, "/")
	// The URL shape is exactly /<token>: any extra segments,
	// administrative-looking paths or query abuse is a miss.
	if token == "" || strings.Contains(token, "/") {
		uniform404(w)
		return
	}
	if !h.limiter.allow("ip:" + clientIP(r)) {
		uniform404(w)
		return
	}
	var rec TokenRecord
	var ok bool
	if h.Validate != nil {
		var subID string
		subID, ok = h.Validate(token)
		if ok {
			rec = TokenRecord{SubID: subID}
		}
	} else {
		rec, ok = h.Store.Lookup(token)
	}
	if !ok {
		uniform404(w)
		return
	}
	if !h.limiter.allow("tok:" + TokenDigestPrefix(token)) {
		uniform404(w)
		return
	}
	var payload []byte
	var err error
	if format == FormatBase64 || h.RenderFormat == nil {
		if h.Render == nil {
			uniform404(w)
			return
		}
		payload, err = h.Render(rec.SubID)
	} else {
		payload, err = h.RenderFormat(rec.SubID, format)
	}
	if err != nil || len(payload) == 0 {
		uniform404(w)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", format.ContentType())
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Content-Length", fmt.Sprint(len(payload)))
	hd.Set("Profile-Update-Interval", "24")
	if h.Userinfo != nil {
		if ui := h.Userinfo(rec.SubID); ui != "" {
			hd.Set("Subscription-Userinfo", ui)
		}
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(payload)
	}
}

// clientIP extracts the immediate peer address. Proxy headers are
// deliberately ignored: the endpoint is public-facing and
// header-based IP trust enables spoof-driven limiter bypass.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// TokenDigestPrefix is the safe log correlation fragment for a
// token: the first 8 hex chars of its digest. Full digests and raw
// tokens must never reach logs.
func TokenDigestPrefix(token string) string {
	d := hashToken(token)
	if len(d) > 8 {
		d = d[:8]
	}
	return d
}
