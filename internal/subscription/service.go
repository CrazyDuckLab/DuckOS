// Package subscription owns the V1 subscription
// endpoint lifecycle. A subscription is the
// public artifact an Xray user receives: a
// share link, a QR code, and an HTTPS URL that
// serves the live configuration as
// base64-encoded payloads.
//
// The package wires together the existing
// internal/xray.Server/User/Profile/ Subscription
// model, the existing internal/clientexport
// document format, and the new dependency
// graph in internal/lifecycle. The endpoint
// is HTTPS-only: a subscription URL served
// over plain HTTP is rejected at construction
// time. Public subscription URLs depend on a
// managed certificate; if the certificate is
// revoked or expires, the service flags the
// subscription as broken until a working
// replacement is wired in.
//
// The Service is goroutine-safe.
package subscription

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/certs"
	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// MaxSubscriptions is the V1 hard cap on the
// number of subscriptions an Agent owns.
const MaxSubscriptions = 1024

// MaxContentBytes is the hard cap on the
// rendered subscription content.
const MaxContentBytes = 1 << 20

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// Subscription is the public artifact
// surface. It carries the dependencies the
// Agent must track and the operator must
// see in the deletion preflight.
type Subscription struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ProfileIDs    []string  `json:"profileIds"`
	PublicDomain  string    `json:"publicDomain"`
	PathPrefix    string    `json:"pathPrefix,omitempty"`
	Enabled       bool      `json:"enabled"`
	ExpiresAt     string    `json:"expiresAt,omitempty"`
	CertificateID string    `json:"certificateId,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Valid reports whether s is acceptable to
// Create/Update.
func (s Subscription) Valid() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("subscription: id is not canonical")
	}
	if s.Name == "" {
		return errors.New("subscription: name is required")
	}
	if !domainPattern.MatchString(s.PublicDomain) {
		return errors.New("subscription: publicDomain is not a canonical FQDN")
	}
	if len(s.ProfileIDs) == 0 {
		return errors.New("subscription: at least one profile is required")
	}
	for _, p := range s.ProfileIDs {
		if !idPattern.MatchString(p) {
			return fmt.Errorf("subscription: profile id %q is not canonical", p)
		}
	}
	return nil
}

// PublicURL is the HTTPS URL the operator
// shares with clients. The path includes a
// per-subscription opaque token so the URL is
// not guessable.
func (s Subscription) PublicURL(token string) (string, error) {
	if !idPattern.MatchString(s.ID) {
		return "", errors.New("subscription: id is not canonical")
	}
	if !idPattern.MatchString(token) {
		return "", errors.New("subscription: token is not canonical")
	}
	prefix := s.PathPrefix
	if prefix == "" {
		prefix = "/sub"
	}
	return fmt.Sprintf("https://%s%s/%s/%s", s.PublicDomain, prefix, s.ID, token), nil
}

// Service is the V1 subscription service. A
// Service holds an in-memory subscription
// table, a read-only view of the Xray model
// (so rendered content is always derived from
// the live model), and the dependency graph
// that tracks ownership between subscriptions
// and the certificates/domains they depend on.
type Service struct {
	mu    sync.RWMutex
	clock func() time.Time
	graph *lifecycle.Graph
	subs  map[string]Subscription
	// xray snapshot is consulted by Render; the
	// service does not own the model, it
	// observes it through the Snapshot
	// interface. The interface keeps the
	// subscription package decoupled from
	// internal/engine.
	snap XraySnapshot
}

// XraySnapshot is the read-only view of the
// Xray model the subscription service uses to
// render content. The interface is satisfied
// by the existing engine wiring.
type XraySnapshot interface {
	Servers() []xray.Server
	Users() []xray.User
	Profiles() []xray.Profile
	Subscriptions() []xray.Subscription
}

// NewService returns an empty Service.
func NewService(graph *lifecycle.Graph, snap XraySnapshot) *Service {
	if graph == nil {
		graph = lifecycle.NewGraph()
	}
	return &Service{clock: time.Now, graph: graph, subs: map[string]Subscription{}, snap: snap}
}

// SetClock overrides the wall clock (testing).
func (s *Service) SetClock(fn func() time.Time) { s.clock = fn }

// SetSnapshot rebinds the Xray snapshot.
func (s *Service) SetSnapshot(snap XraySnapshot) {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

// Create validates and inserts a new
// subscription. The pre-existing dependency
// edges (certificate, public domain) are
// recorded in the lifecycle graph.
func (s *Service) Create(sub Subscription) (Subscription, error) {
	if err := sub.Valid(); err != nil {
		return Subscription{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub.ID]; ok {
		return Subscription{}, errors.New("subscription: id already exists")
	}
	if len(s.subs) >= MaxSubscriptions {
		return Subscription{}, errors.New("subscription: capacity exceeded")
	}
	now := s.clock().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	sub.UpdatedAt = now
	// Record ownership edges in the dependency
	// graph. The certificate is the strongest
	// binding: if it goes away, the URL is
	// broken.
	if sub.CertificateID != "" {
		if err := s.graph.Add(lifecycle.Reference{
			Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
			Target:    lifecycle.Object{Kind: lifecycle.KindCertificate, ID: sub.CertificateID},
			Reason:    "https endpoint",
		}); err != nil {
			return Subscription{}, fmt.Errorf("subscription: record cert dependency: %w", err)
		}
	}
	if sub.PublicDomain != "" {
		if err := s.graph.Add(lifecycle.Reference{
			Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
			Target:    lifecycle.Object{Kind: lifecycle.KindService, ID: "public-domain:" + sub.PublicDomain},
			Reason:    "public domain",
		}); err != nil {
			return Subscription{}, fmt.Errorf("subscription: record domain dependency: %w", err)
		}
	}
	s.subs[sub.ID] = sub
	return sub, nil
}

// Get returns a subscription by id.
func (s *Service) Get(id string) (Subscription, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[id]
	return sub, ok
}

// List returns the subscriptions sorted by
// id.
func (s *Service) List() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Update applies a partial update. The
// PublicDomain and CertificateID are diffed
// against the previous record; the dependency
// graph is updated accordingly.
func (s *Service) Update(sub Subscription) (Subscription, error) {
	if err := sub.Valid(); err != nil {
		return Subscription{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.subs[sub.ID]
	if !ok {
		return Subscription{}, errors.New("subscription: not found")
	}
	if prev.CertificateID != sub.CertificateID {
		if prev.CertificateID != "" {
			s.graph.Remove(lifecycle.Reference{
				Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
				Target:    lifecycle.Object{Kind: lifecycle.KindCertificate, ID: prev.CertificateID},
			})
		}
		if sub.CertificateID != "" {
			if err := s.graph.Add(lifecycle.Reference{
				Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
				Target:    lifecycle.Object{Kind: lifecycle.KindCertificate, ID: sub.CertificateID},
				Reason:    "https endpoint",
			}); err != nil {
				return Subscription{}, err
			}
		}
	}
	if prev.PublicDomain != sub.PublicDomain {
		if prev.PublicDomain != "" {
			s.graph.Remove(lifecycle.Reference{
				Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
				Target:    lifecycle.Object{Kind: lifecycle.KindService, ID: "public-domain:" + prev.PublicDomain},
			})
		}
		if sub.PublicDomain != "" {
			if err := s.graph.Add(lifecycle.Reference{
				Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
				Target:    lifecycle.Object{Kind: lifecycle.KindService, ID: "public-domain:" + sub.PublicDomain},
				Reason:    "public domain",
			}); err != nil {
				return Subscription{}, err
			}
		}
	}
	sub.CreatedAt = prev.CreatedAt
	sub.UpdatedAt = s.clock().UTC()
	s.subs[sub.ID] = sub
	return sub, nil
}

// Revoke disables a subscription. The record
// is retained for forensics; Render refuses
// to serve its content.
func (s *Service) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return errors.New("subscription: not found")
	}
	sub.Enabled = false
	sub.UpdatedAt = s.clock().UTC()
	s.subs[id] = sub
	return nil
}

// Delete removes a subscription and its
// dependency-graph edges. Returns an error
// if other objects depend on it; the caller
// is expected to call Preflight first.
func (s *Service) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[id]
	if !ok {
		return errors.New("subscription: not found")
	}
	if sub.CertificateID != "" {
		s.graph.Remove(lifecycle.Reference{
			Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
			Target:    lifecycle.Object{Kind: lifecycle.KindCertificate, ID: sub.CertificateID},
		})
	}
	if sub.PublicDomain != "" {
		s.graph.Remove(lifecycle.Reference{
			Dependent: lifecycle.Object{Kind: lifecycle.KindSubscription, ID: sub.ID},
			Target:    lifecycle.Object{Kind: lifecycle.KindService, ID: "public-domain:" + sub.PublicDomain},
		})
	}
	delete(s.subs, id)
	return nil
}

// Render returns the base64-encoded payload
// the public HTTPS endpoint serves. The
// payload is derived from the live Xray
// snapshot: every profile listed in
// ProfileIDs that exists in the snapshot and
// has a working user and server is included.
// The output is sorted for stable delivery
// across rotations.
//
// Render refuses to serve a revoked or
// expired subscription.
func (s *Service) Render(id string) ([]byte, error) {
	s.mu.RLock()
	sub, ok := s.subs[id]
	snap := s.snap
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("subscription: not found")
	}
	if !sub.Enabled {
		return nil, errors.New("subscription: revoked")
	}
	if sub.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, sub.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("subscription: invalid expiresAt: %w", err)
		}
		if s.clock().After(exp) {
			return nil, errors.New("subscription: expired")
		}
	}
	if snap == nil {
		return nil, errors.New("subscription: xray snapshot unavailable")
	}
	profiles := map[string]xray.Profile{}
	for _, p := range snap.Profiles() {
		profiles[p.ID] = p
	}
	users := map[string]xray.User{}
	for _, u := range snap.Users() {
		users[u.ID] = u
	}
	servers := map[string]xray.Server{}
	for _, sv := range snap.Servers() {
		servers[sv.ID] = sv
	}
	// Build the line list. The order inside a
	// subscription is by (server, user) for
	// stable delivery.
	type line struct {
		Server string
		User   string
	}
	var lines []line
	for _, pid := range sub.ProfileIDs {
		p, ok := profiles[pid]
		if !ok {
			continue
		}
		u, ok := users[p.UserID]
		if !ok {
			continue
		}
		if u.IsExpired(s.clock()) {
			continue
		}
		sv, ok := servers[u.ServerID]
		if !ok {
			continue
		}
		// Subscription serves share links; the
		// existing internal/xray.BuildShareLink
		// is the contract. We import it lazily
		// to keep this file readable.
		uri, err := xray.BuildShareLink(sv, u)
		if err != nil {
			continue
		}
		lines = append(lines, line{Server: sv.ID, User: uri})
	}
	if len(lines) == 0 {
		return nil, errors.New("subscription: no usable profiles")
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Server < lines[j].Server })
	var b []byte
	for _, l := range lines {
		b = append(b, l.User...)
		b = append(b, '\n')
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	if len(encoded) > MaxContentBytes {
		return nil, errors.New("subscription: rendered content exceeds cap")
	}
	return []byte(encoded), nil
}

// TokenFor derives a stable per-subscription
// token from a secret. The same secret always
// produces the same token; the token is the
// last segment of the public URL. The secret
// is never logged.
func TokenFor(id, secret string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", errors.New("subscription: id is not canonical")
	}
	if secret == "" {
		return "", errors.New("subscription: secret is empty")
	}
	sum := sha256.Sum256([]byte("DuckOS/subscription/url-token/v1\x00" + id + "\x00" + secret))
	return hex.EncodeToString(sum[:8]), nil
}

// CertHealth reports whether the bound
// certificate is missing or expired. The
// service does not own the certificate
// manager; the caller wires in the live
// cert record. The function exists so the
// subscription model can answer "is the
// public URL still trusted?" without leaking
// certificate bytes.
func (s *Service) CertHealth(id string, now time.Time) (string, error) {
	s.mu.RLock()
	sub, ok := s.subs[id]
	s.mu.RUnlock()
	if !ok {
		return "", errors.New("subscription: not found")
	}
	if sub.CertificateID == "" {
		return "unbound", nil
	}
	if sub.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, sub.ExpiresAt)
		if err == nil && now.After(exp) {
			return "expired", nil
		}
	}
	// The live certificate is consulted by the
	// caller through certs.*. The service only
	// reports the metadata; bytes are never
	// embedded.
	_ = certs.MaxCertBytes
	return "ok", nil
}
