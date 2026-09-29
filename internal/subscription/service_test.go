// subscription/service_test.go pins the V1
// subscription service contract: HTTPS-only
// URLs, dependency tracking for cert/domain
// changes, revoke/expire refusal of Render,
// stable per-subscription tokens, and content
// derived from the live xray snapshot.
package subscription

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/certs"
	"github.com/CrazyDuckLab/DuckOS/internal/lifecycle"
	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

type fakeSnap struct {
	servers  []xray.Server
	users    []xray.User
	profiles []xray.Profile
	subs     []xray.Subscription
}

func (f *fakeSnap) Servers() []xray.Server             { return f.servers }
func (f *fakeSnap) Users() []xray.User                 { return f.users }
func (f *fakeSnap) Profiles() []xray.Profile           { return f.profiles }
func (f *fakeSnap) Subscriptions() []xray.Subscription { return f.subs }

func TestSubscriptionValidRejectsBadDomain(t *testing.T) {
	s := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "not a domain", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	if err := s.Valid(); err == nil {
		t.Fatal("Valid must reject non-FQDN")
	}
}

func TestSubscriptionValidRequiresProfiles(t *testing.T) {
	s := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com"}
	if err := s.Valid(); err == nil {
		t.Fatal("Valid must require at least one profile")
	}
}

func TestPublicURLRequiresHTTPS(t *testing.T) {
	s := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	url, err := s.PublicURL("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("PublicURL: %v", err)
	}
	if !strings.HasPrefix(url, "https://") {
		t.Fatalf("PublicURL must be https, got %s", url)
	}
}

func TestTokenForIsStableAndDistinct(t *testing.T) {
	a, _ := TokenFor("sub-1", "secret")
	b, _ := TokenFor("sub-1", "secret")
	if a != b {
		t.Fatalf("TokenFor must be stable")
	}
	c, _ := TokenFor("sub-2", "secret")
	if a == c {
		t.Fatalf("TokenFor must differ across ids")
	}
}

func TestCreateRecordsCertificateDependency(t *testing.T) {
	g := lifecycle.NewGraph()
	svc := NewService(g, &fakeSnap{})
	sub := Subscription{
		ID: "sub-1", Name: "blog",
		PublicDomain:  "blog.example.com",
		ProfileIDs:    []string{"abcdef0123456789abcdef0123456789"},
		CertificateID: "cert-1",
	}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	plan, err := g.Preflight(lifecycle.Object{Kind: lifecycle.KindCertificate, ID: "cert-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasDependents {
		t.Fatal("expected subscription to depend on the certificate")
	}
}

func TestUpdateCertChangeRebindsDependency(t *testing.T) {
	g := lifecycle.NewGraph()
	svc := NewService(g, &fakeSnap{})
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true, CertificateID: "cert-1"}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	updated := sub
	updated.CertificateID = "cert-2"
	if _, err := svc.Update(updated); err != nil {
		t.Fatal(err)
	}
	// The old cert must have no dependents.
	plan, _ := g.Preflight(lifecycle.Object{Kind: lifecycle.KindCertificate, ID: "cert-1"})
	if plan.HasDependents {
		t.Fatal("old cert must be released")
	}
	// The new cert must own the subscription.
	plan, _ = g.Preflight(lifecycle.Object{Kind: lifecycle.KindCertificate, ID: "cert-2"})
	if !plan.HasDependents {
		t.Fatal("new cert must own the subscription")
	}
}

func TestRevokeRefusesRender(t *testing.T) {
	snap := &fakeSnap{
		profiles: []xray.Profile{{ID: "abcdef0123456789abcdef0123456789", UserID: "fedcba9876543210fedcba9876543210"}},
		users:    []xray.User{{ID: "fedcba9876543210fedcba9876543210", ServerID: "0123456789abcdef0123456789abcdef", UUID: "abc", Name: "u-blog", Credential: &xray.KeyRef{VaultID: "0123456789abcdef0123456789abcdef", Revision: "fedcba9876543210fedcba9876543210", KeyID: "abcdef0123456789abcdef0123456789"}}},
		servers:  []xray.Server{{ID: "0123456789abcdef0123456789abcdef", Protocol: xray.ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443, Transport: xray.TransportTCP, Security: xray.SecurityNone, Name: "blog"}},
	}
	svc := NewService(lifecycle.NewGraph(), snap)
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	if err := svc.Revoke("sub-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Render("sub-1"); err == nil {
		t.Fatal("Render must refuse a revoked subscription")
	}
}

func TestExpiredSubscriptionRefusesRender(t *testing.T) {
	snap := &fakeSnap{
		profiles: []xray.Profile{{ID: "abcdef0123456789abcdef0123456789", UserID: "fedcba9876543210fedcba9876543210"}},
		users:    []xray.User{{ID: "fedcba9876543210fedcba9876543210", ServerID: "0123456789abcdef0123456789abcdef", UUID: "abc", Name: "u-blog", Credential: &xray.KeyRef{VaultID: "0123456789abcdef0123456789abcdef", Revision: "fedcba9876543210fedcba9876543210", KeyID: "abcdef0123456789abcdef0123456789"}}},
		servers:  []xray.Server{{ID: "0123456789abcdef0123456789abcdef", Protocol: xray.ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443, Transport: xray.TransportTCP, Security: xray.SecurityNone, Name: "blog"}},
	}
	svc := NewService(lifecycle.NewGraph(), snap)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return clock })
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true, ExpiresAt: "2026-01-01T00:00:00Z"}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Minute)
	if _, err := svc.Render("sub-1"); err == nil {
		t.Fatal("Render must refuse an expired subscription")
	}
}

func TestDeleteRemovesDependencies(t *testing.T) {
	g := lifecycle.NewGraph()
	svc := NewService(g, &fakeSnap{})
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true, CertificateID: "cert-1"}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete("sub-1"); err != nil {
		t.Fatal(err)
	}
	plan, _ := g.Preflight(lifecycle.Object{Kind: lifecycle.KindCertificate, ID: "cert-1"})
	if plan.HasDependents {
		t.Fatal("Delete must release the cert dependency")
	}
}

func TestRenderUsesLiveSnapshot(t *testing.T) {
	snap := &fakeSnap{
		profiles: []xray.Profile{{ID: "abcdef0123456789abcdef0123456789", UserID: "fedcba9876543210fedcba9876543210"}},
		users:    []xray.User{{ID: "fedcba9876543210fedcba9876543210", ServerID: "0123456789abcdef0123456789abcdef", UUID: "abc", Name: "u-blog", Credential: &xray.KeyRef{VaultID: "0123456789abcdef0123456789abcdef", Revision: "fedcba9876543210fedcba9876543210", KeyID: "abcdef0123456789abcdef0123456789"}}},
		servers:  []xray.Server{{ID: "0123456789abcdef0123456789abcdef", Protocol: xray.ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443, Transport: xray.TransportTCP, Security: xray.SecurityNone, Name: "blog"}},
	}
	svc := NewService(lifecycle.NewGraph(), snap)
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	rendered, err := svc.Render("sub-1")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(rendered), "vless://") && !strings.Contains(string(rendered), "dmxlc3M6Ly8") {
		t.Fatalf("rendered content must contain a vless share link, got %q", rendered)
	}
}

func TestRenderExcludesExpiredUsers(t *testing.T) {
	snap := &fakeSnap{
		profiles: []xray.Profile{{ID: "abcdef0123456789abcdef0123456789", UserID: "fedcba9876543210fedcba9876543210"}},
		users:    []xray.User{{ID: "fedcba9876543210fedcba9876543210", ServerID: "0123456789abcdef0123456789abcdef", UUID: "abc", ExpiresAt: "2020-01-01T00:00:00Z"}},
		servers:  []xray.Server{{ID: "0123456789abcdef0123456789abcdef", Protocol: xray.ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443, Transport: xray.TransportTCP, Security: xray.SecurityNone, Name: "blog"}},
	}
	svc := NewService(lifecycle.NewGraph(), snap)
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Render("sub-1"); err == nil {
		t.Fatal("Render must refuse when every user is expired")
	}
}

func TestCertHealthStates(t *testing.T) {
	svc := NewService(lifecycle.NewGraph(), &fakeSnap{})
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Unbound
	sub := Subscription{ID: "sub-1", Name: "blog", PublicDomain: "blog.example.com", ProfileIDs: []string{"abcdef0123456789abcdef0123456789"}, Enabled: true}
	if _, err := svc.Create(sub); err != nil {
		t.Fatal(err)
	}
	state, _ := svc.CertHealth("sub-1", now)
	if state != "unbound" {
		t.Fatalf("unbound cert state: %q", state)
	}
	// Bound
	sub.CertificateID = "cert-1"
	if _, err := svc.Update(sub); err != nil {
		t.Fatal(err)
	}
	state, _ = svc.CertHealth("sub-1", now)
	if state != "ok" {
		t.Fatalf("bound cert state: %q", state)
	}
	// Expired (subscription expiry drives the
	// state).
	sub.ExpiresAt = "2026-01-01T00:00:00Z"
	if _, err := svc.Update(sub); err != nil {
		t.Fatal(err)
	}
	state, _ = svc.CertHealth("sub-1", now)
	if state != "expired" {
		t.Fatalf("expired state: %q", state)
	}
}

// silence "imported and not used" if certs is
// pulled by another test in the future.
var _ = pem.EncodeToMemory
var _ = x509.Certificate{}
var _ = pkix.Name{}
var _ = certs.MaxCertBytes
