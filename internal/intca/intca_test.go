package intca

import (
	"testing"
	"time"
)

func TestCAValid(t *testing.T) {
	now := time.Now()
	good := CA{Name: "core", Type: "root", CertPath: "/etc/duckos/ca.crt", KeyVaultRef: "vault/ca/core", ValidFrom: now, ValidUntil: now.AddDate(1, 0, 0)}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	badType := CA{Name: "core", Type: "garbage", CertPath: "x", KeyVaultRef: "v", ValidFrom: now, ValidUntil: now.AddDate(1, 0, 0)}
	if err := badType.Valid(); err == nil {
		t.Fatal("bad type must fail")
	}
	noVault := CA{Name: "core", Type: "root", CertPath: "x", ValidFrom: now, ValidUntil: now.AddDate(1, 0, 0)}
	if err := noVault.Valid(); err == nil {
		t.Fatal("missing vault must fail")
	}
	badRange := CA{Name: "core", Type: "root", CertPath: "x", KeyVaultRef: "v", ValidFrom: now, ValidUntil: now.Add(-time.Hour)}
	if err := badRange.Valid(); err == nil {
		t.Fatal("validUntil < validFrom must fail")
	}
}

func TestCARevoke(t *testing.T) {
	now := time.Now()
	s := NewCAStore()
	s.AddCA(CA{Name: "core", Type: "root", CertPath: "x", KeyVaultRef: "v", ValidFrom: now, ValidUntil: now.AddDate(1, 0, 0)})
	s.Issue(IssuedCert{Serial: "1", Subject: "device1", CAName: "core", NotBefore: now, NotAfter: now.AddDate(0, 1, 0)})
	if err := s.Revoke("1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke("missing"); err == nil {
		t.Fatal("missing must fail")
	}
}

func TestAPIToken(t *testing.T) {
	now := time.Now()
	s := NewAPITokenStore()
	s.Add(APIToken{ID: "t1", Name: "ci", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	s.Touch("t1")
	s.Revoke("t1")
	if err := s.Add(APIToken{ID: "t1", Name: "x", CreatedAt: now, ExpiresAt: now.Add(-time.Hour)}); err == nil {
		t.Fatal("invalid expiry must fail")
	}
}

func TestProvenance(t *testing.T) {
	s := NewProvenanceStore()
	s.Set(Provenance{Key: "firewall:r1", Owner: "manual"})
	if err := s.TakeOverControl("firewall:r1", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseControl("firewall:r1", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("firewall:r1"); !ok {
		t.Fatal("get")
	}
}

func TestTemplate(t *testing.T) {
	s := NewTemplateStore()
	s.Add(Template{Name: "edge", Body: map[string]interface{}{"mtu": 1500}})
	if _, ok := s.Get("edge"); !ok {
		t.Fatal("get")
	}
	if err := s.Add(Template{Name: "!bad", Body: map[string]interface{}{}}); err == nil {
		t.Fatal("bad name must fail")
	}
}

func TestZTP(t *testing.T) {
	s := NewZTPStore()
	s.Issue(ZTPCredential{ID: "z1", Token: "abc", ExpiresAt: time.Now().Add(time.Hour)})
	if err := s.Consume("z1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Consume("z1"); err == nil {
		t.Fatal("double-consume must fail")
	}
	if err := s.Consume("missing"); err == nil {
		t.Fatal("missing consume must fail")
	}
}
