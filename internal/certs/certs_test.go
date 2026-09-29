// certs/certs_test.go pins the V1 certificate
// manager contract. The test exercises:
//   - domain validation (wildcards, length, hyphen)
//   - Source validation for each Kind
//   - parseImported happy path (self-signed)
//   - parseImported refusal: wrong key, expired,
//     not-yet-valid, mismatched SAN, oversized
//     PEM, wrong PEM type
//   - VaultRef is the only accepted path for
//     DNS-01 credentials (cleartext is refused)
//   - Fingerprint is stable and 32 chars
package certs

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestValidDomain(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"a.example.com", true},
		{"*.example.com", true},
		{"*.sub.example.com", true},
		{"", false},
		{"example", false},
		{"example..com", false},
		{"-example.com", false},
		{"example-.com", false},
		{"example.com.", false},
		{strings.Repeat("a", 254), false},
	} {
		if got := ValidDomain(tc.in); got != tc.want {
			t.Errorf("ValidDomain(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSourceValidateRejectsUnknownKind(t *testing.T) {
	s := Source{Kind: "garbage", Domains: []string{"example.com"}}
	if err := s.Validate(); err == nil {
		t.Fatal("Validate must reject unknown kind")
	}
}

func TestSourceValidateRejectsNoDomains(t *testing.T) {
	s := Source{Kind: KindImported}
	if err := s.Validate(); err == nil {
		t.Fatal("Validate must reject empty domains")
	}
}

func TestSourceValidateRequiresKeyOrVaultRefForImported(t *testing.T) {
	s := Source{Kind: KindImported, Domains: []string{"example.com"}, Certificate: pemLeaf(t, "example.com", time.Hour)}
	if err := s.Validate(); err == nil {
		t.Fatal("imported source without key/vault reference must be rejected")
	}
}

func TestSourceValidateRejectsCleartextDNSCredential(t *testing.T) {
	s := Source{Kind: KindACMEDNS01, Domains: []string{"example.com"}, DNSProvider: "cloudflare", DNSCredential: "api-token-plaintext"}
	if err := s.Validate(); err == nil {
		t.Fatal("cleartext DNS credential must be rejected")
	}
}

func TestSourceValidateRequiresProviderForDNS01(t *testing.T) {
	s := Source{Kind: KindACMEDNS01, Domains: []string{"example.com"}}
	if err := s.Validate(); err == nil {
		t.Fatal("DNS-01 without provider must be rejected")
	}
}

func TestParseImportedHappyPath(t *testing.T) {
	domains := []string{"example.com", "www.example.com"}
	leaf, key, err := MakeSelfSigned(domains, time.Hour)
	if err != nil {
		t.Fatalf("MakeSelfSigned: %v", err)
	}
	certPEM := encodeCert(t, leaf)
	keyPEM := encodeKey(t, key)
	s := Source{Kind: KindImported, Domains: domains, Certificate: certPEM, PrivateKey: keyPEM}
	ic, err := ParseImported(s)
	if err != nil {
		t.Fatalf("ParseImported: %v", err)
	}
	if !ic.Leaf.Equal(leaf) {
		t.Fatal("parsed leaf must equal the input")
	}
}

func TestParseImportedRefusesWrongKey(t *testing.T) {
	domains := []string{"example.com"}
	leaf, _, _ := MakeSelfSigned(domains, time.Hour)
	wrongKey, _ := GenerateKey()
	certPEM := encodeCert(t, leaf)
	keyPEM := encodeKey(t, wrongKey)
	s := Source{Kind: KindImported, Domains: domains, Certificate: certPEM, PrivateKey: keyPEM}
	if _, err := ParseImported(s); err == nil {
		t.Fatal("ParseImported must refuse a key that does not match the cert")
	}
}

func TestParseImportedRefusesExpired(t *testing.T) {
	domains := []string{"example.com"}
	leaf, key, _ := MakeSelfSigned(domains, -time.Hour)
	s := Source{Kind: KindImported, Domains: domains, Certificate: encodeCert(t, leaf), PrivateKey: encodeKey(t, key)}
	if _, err := ParseImported(s); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestParseImportedRefusesNotYetValid(t *testing.T) {
	// Generate a cert that is not yet valid
	// (NotBefore in the future) by overriding
	// the test helper to accept a future offset.
	domains := []string{"example.com"}
	leaf, key, err := makeSelfSignedAt(domains, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	s := Source{Kind: KindImported, Domains: domains, Certificate: encodeCert(t, leaf), PrivateKey: encodeKey(t, key)}
	if _, err := ParseImported(s); err == nil {
		t.Fatal("ParseImported must refuse a not-yet-valid cert")
	}
}

func TestParseImportedRefusesMismatchedSAN(t *testing.T) {
	leaf, key, _ := MakeSelfSigned([]string{"example.com"}, time.Hour)
	s := Source{Kind: KindImported, Domains: []string{"other.com"}, Certificate: encodeCert(t, leaf), PrivateKey: encodeKey(t, key)}
	if _, err := ParseImported(s); err == nil {
		t.Fatal("ParseImported must refuse a SAN mismatch")
	}
}

func TestParseImportedRefusesOversizedPEM(t *testing.T) {
	s := Source{Kind: KindImported, Domains: []string{"example.com"}, Certificate: strings.Repeat("A", MaxCertBytes+1)}
	if _, err := ParseImported(s); err == nil {
		t.Fatal("oversized PEM must be rejected")
	}
}

func TestParseImportedRefusesWrongPEMType(t *testing.T) {
	domains := []string{"example.com"}
	leaf, key, _ := MakeSelfSigned(domains, time.Hour)
	// Wrap the cert PEM but with a wrong type.
	wrong := strings.Replace(encodeCert(t, leaf), "CERTIFICATE", "PUBKEY", 1)
	s := Source{Kind: KindImported, Domains: domains, Certificate: wrong, PrivateKey: encodeKey(t, key)}
	if _, err := ParseImported(s); err == nil {
		t.Fatal("ParseImported must refuse a wrong PEM type")
	}
}

func TestFingerprintIsStableAnd32Chars(t *testing.T) {
	leaf, _, _ := MakeSelfSigned([]string{"example.com"}, time.Hour)
	fp1 := Fingerprint(leaf)
	fp2 := Fingerprint(leaf)
	if fp1 != fp2 {
		t.Fatalf("fingerprint must be stable: %q vs %q", fp1, fp2)
	}
	if matched := regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(fp1); !matched {
		t.Fatalf("fingerprint must be 32 hex chars, got %q", fp1)
	}
}

func TestParseImportedWithPKCS8Key(t *testing.T) {
	domains := []string{"example.com"}
	leaf, key, _ := MakeSelfSigned(domains, time.Hour)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	s := Source{Kind: KindImported, Domains: domains, Certificate: encodeCert(t, leaf), PrivateKey: keyPEM}
	if _, err := ParseImported(s); err != nil {
		t.Fatalf("ParseImported with PKCS#8: %v", err)
	}
}

func pemLeaf(t *testing.T, domain string, ttl time.Duration) string {
	t.Helper()
	leaf, _, err := MakeSelfSigned([]string{domain}, ttl)
	if err != nil {
		t.Fatalf("MakeSelfSigned: %v", err)
	}
	return encodeCert(t, leaf)
}

func makeSelfSignedAt(domains []string, notBefore, notAfter time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: domains[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     domains,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func encodeCert(t *testing.T, leaf *x509.Certificate) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}))
}

func encodeKey(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}
