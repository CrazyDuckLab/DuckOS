// Package certs / inventory_test.go: inventory, expiration health,
// renewal policy and binding semantics.
package certs

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"
)

// selfPEM builds a self-signed cert and returns leaf + PEM strings.
func selfPEM(t *testing.T, domains []string, ttl time.Duration) (string, string) {
	t.Helper()
	cert, key, err := MakeSelfSigned(domains, ttl)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certPEM), string(keyPEM)
}

func TestImportValidatesRealPEM(t *testing.T) {
	inv := NewInventory()
	certPEM, keyPEM := selfPEM(t, []string{"vpn.example.com"}, 90*24*time.Hour)
	rec, err := inv.Import("", Source{
		Kind: KindImported, Domains: []string{"vpn.example.com"},
		Certificate: certPEM, PrivateKey: keyPEM,
	}, "vault-ref-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Fingerprint == "" || rec.KeyVaultRef != "vault-ref-1" || rec.NotAfter.IsZero() {
		t.Fatalf("record: %+v", rec)
	}
	// Mismatched key must be refused.
	_, key2 := selfPEM(t, []string{"other.example.com"}, time.Hour)
	if _, err := inv.Import("", Source{
		Kind: KindImported, Domains: []string{"vpn.example.com"},
		Certificate: certPEM, PrivateKey: key2,
	}, ""); err == nil {
		t.Fatal("cert/key mismatch accepted")
	}
}

func TestHealthAndRenewalDue(t *testing.T) {
	inv := NewInventory()
	base := time.Now().UTC()
	inv.SetClock(func() time.Time { return base })
	// ACME record expiring in 20 days with 30-day renewal window:
	// health expiring, renewal due.
	rec, err := inv.RegisterACME("acme-1", KindACMEDNS01, []string{"a.example.com"}, "cloudflare")
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{NotBefore: base.Add(-time.Hour), NotAfter: base.Add(20 * 24 * time.Hour), DNSNames: []string{"a.example.com"}}
	if err := inv.IssueACME(rec.ID, leaf); err != nil {
		t.Fatal(err)
	}
	list := inv.List(0)
	if len(list) != 1 || list[0].Health != HealthExpiring {
		t.Fatalf("health: %+v", list)
	}
	due := inv.RenewDue(base)
	if len(due) != 1 || due[0].ID != rec.ID {
		t.Fatalf("renew due: %+v", due)
	}
	// Failure diagnostics are bounded and recorded.
	inv.MarkRenewalFailed(rec.ID, errors.New("dns-01: provider rejected token very long message"))
	got := inv.List(0)
	if got[0].Renewal.Status != "failed" || got[0].Renewal.LastError == "" || len(got[0].Renewal.LastError) > 256 {
		t.Fatalf("renewal failure: %+v", got[0].Renewal)
	}
	// Renewal success clears the error.
	if err := inv.IssueACME(rec.ID, &x509.Certificate{NotAfter: base.Add(90 * 24 * time.Hour), DNSNames: []string{"a.example.com"}}); err != nil {
		t.Fatal(err)
	}
	got = inv.List(0)
	if got[0].Renewal.Status != "issued" || got[0].Renewal.LastError != "" {
		t.Fatalf("renewal recovery: %+v", got[0].Renewal)
	}
	if len(inv.RenewDue(base)) != 0 {
		t.Fatal("renewal due after fresh issue")
	}
}

func TestBindingGuardsRemoval(t *testing.T) {
	inv := NewInventory()
	if _, err := inv.RegisterACME("acme-1", KindACMEHTTP01, []string{"b.example.com"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := inv.Bind("acme-1", "srv-1"); err != nil {
		t.Fatal(err)
	}
	if err := inv.Remove("acme-1"); err == nil {
		t.Fatal("bound record removed")
	}
	if err := inv.Bind("acme-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := inv.Remove("acme-1"); err != nil {
		t.Fatalf("unbind-then-remove: %v", err)
	}
}

func TestExpiredHealth(t *testing.T) {
	rec := Record{NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(-time.Hour)}
	if rec.HealthAt(time.Now(), 0) != HealthExpired {
		t.Fatal("expired record not classified")
	}
}
