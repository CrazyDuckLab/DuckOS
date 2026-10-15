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
}

func NewTokenStore() *TokenStore {
	return &TokenStore{byHash: map[string]*TokenRecord{}, clock: time.Now}
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
// a replacement. The old URLs stop resolving immediately.
func (s *TokenStore) Rotate(subID string, ttl time.Duration) (string, TokenRecord, error) {
	s.mu.Lock()
	for _, rec := range s.byHash {
		if rec.SubID == subID && !rec.Revoked {
			rec.Revoked = true
		}
	}
	s.mu.Unlock()
	return s.Issue(subID, ttl)
}

// RevokeSubscription revokes every token of a subscription
// (suspend / delete path).
func (s *TokenStore) RevokeSubscription(subID string) int {
	s.mu.Lock()
	n := 0
	for _, rec := range s.byHash {
		if rec.SubID == subID && !rec.Revoked {
			rec.Revoked = true
			n++
		}
	}
	s.mu.Unlock()
	return n
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
	// Render produces the payload for a subscription id. It must
	// internally refuse disabled/expired subscriptions; any error
	// becomes the uniform 404 so state never leaks.
	Render func(subID string) ([]byte, error)
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
	rec, ok := h.Store.Lookup(token)
	if !ok {
		uniform404(w)
		return
	}
	if !h.limiter.allow("tok:" + rec.TokenHash[:8]) {
		uniform404(w)
		return
	}
	if h.Render == nil {
		uniform404(w)
		return
	}
	payload, err := h.Render(rec.SubID)
	if err != nil || len(payload) == 0 {
		uniform404(w)
		return
	}
	hd := w.Header()
	hd.Set("Content-Type", "text/plain; charset=utf-8")
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
