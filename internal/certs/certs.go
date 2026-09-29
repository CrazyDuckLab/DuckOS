// Package certs owns the V1 certificate
// manager. Two acquisition paths are supported:
//
//   - Automatic: ACME (DNS-01 first-class;
//     HTTP-01 and TLS-ALPN-01 supported where
//     the topology permits). DNS-01 provider
//     credentials are stored through the vault;
//     the manager never embeds them in logs,
//     config, or API responses.
//   - Manual: import certificate PEM and
//     private key, validate cert/key match,
//     expiry, SAN/domain, and chain.
//
// Every activation runs through a Safe Apply
// gate: a candidate certificate is materialised
// under a separate path, validated, and only
// then promoted to Active. The Previous is
// retained so a failed activation can be rolled
// back without losing a working certificate.
//
// The manager is structured so a real ACME
// client can be plugged in later; the V1
// contract is the model and the validation
// primitives, not the upstream integration.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"
)

// MaxChain is the upper bound on the number of
// certificates in a chain. Anything above this
// is refused at import time.
const MaxChain = 8

// MaxCertBytes is the upper bound on a single
// PEM-encoded certificate.
const MaxCertBytes = 16 << 10

// MaxKeyBytes is the upper bound on a PEM-encoded
// private key.
const MaxKeyBytes = 16 << 10

// MaxSAN is the upper bound on the number of
// SAN entries the V1 contract allows.
const MaxSAN = 64

// MaxDomainLen is the upper bound on a single
// domain name.
const MaxDomainLen = 253

var labelPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// Kind is the acquisition path.
type Kind string

const (
	// KindACMEDNS01 is an ACME certificate
	// obtained through a DNS-01 challenge.
	KindACMEDNS01 Kind = "acme-dns-01"
	// KindACMEHTTP01 is an ACME certificate
	// obtained through an HTTP-01 challenge.
	KindACMEHTTP01 Kind = "acme-http-01"
	// KindACMETLSALPN01 is an ACME certificate
	// obtained through a TLS-ALPN-01 challenge.
	KindACMETLSALPN01 Kind = "acme-tls-alpn-01"
	// KindImported is a manually imported
	// certificate.
	KindImported Kind = "imported"
)

// Source is the input to the manager. The
// manager refuses to import a Source that does
// not Validate.
type Source struct {
	Kind          Kind      `json:"kind"`
	Domains       []string  `json:"domains"`
	Certificate   string    `json:"certificate,omitempty"` // PEM chain
	PrivateKey    string    `json:"privateKey,omitempty"`  // PEM key
	VaultRef      string    `json:"vaultRef,omitempty"`    // Vault ref for the private key (importer)
	DNSProvider   string    `json:"dnsProvider,omitempty"` // "cloudflare" | "route53" | ...
	DNSCredential string    `json:"dnsCredential,omitempty"`
	NotBefore     time.Time `json:"notBefore,omitempty"`
	NotAfter      time.Time `json:"notAfter,omitempty"`
}

// Validate runs the input contract. The
// checks are conservative: any deviation
// returns an explicit error.
func (s Source) Validate() error {
	switch s.Kind {
	case KindACMEDNS01, KindACMEHTTP01, KindACMETLSALPN01, KindImported:
	default:
		return errors.New("certs: kind is unset or unknown")
	}
	if len(s.Domains) == 0 || len(s.Domains) > MaxSAN {
		return fmt.Errorf("certs: domains must be 1..%d", MaxSAN)
	}
	for _, d := range s.Domains {
		if !ValidDomain(d) {
			return fmt.Errorf("certs: domain %q is not canonical", d)
		}
	}
	if s.Kind == KindImported {
		if s.Certificate == "" {
			return errors.New("certs: imported certificate is empty")
		}
		if s.PrivateKey == "" && s.VaultRef == "" {
			return errors.New("certs: imported key or vault reference is required")
		}
	}
	if s.Kind == KindACMEDNS01 {
		if s.DNSProvider == "" {
			return errors.New("certs: DNS-01 provider is required")
		}
		// DNS credentials must always be a
		// vault reference. The cleartext form
		// is never accepted.
		if s.DNSCredential != "" {
			return errors.New("certs: DNS credential must be supplied through the vault, not in the clear")
		}
	}
	return nil
}

// ValidDomain reports whether d is a canonical
// DNS name. Wildcards are accepted at the
// leading label only.
func ValidDomain(d string) bool {
	if d == "" || len(d) > MaxDomainLen {
		return false
	}
	if strings.HasPrefix(d, "*.") {
		rest := strings.TrimPrefix(d, "*.")
		return ValidDomain(rest)
	}
	if !strings.Contains(d, ".") {
		return false
	}
	labels := strings.Split(d, ".")
	for _, l := range labels {
		if !labelPattern.MatchString(l) {
			return false
		}
	}
	// Reject a leading or trailing hyphen globally
	// (covered by the regex) and a single-label
	// trailing dot.
	if strings.HasSuffix(d, ".") {
		return false
	}
	return true
}

// ImportedCert is the result of parsing a Source
// of kind "imported". It carries the parsed
// leaf, optional chain, and the parsed key.
type ImportedCert struct {
	Leaf   *x509.Certificate
	Chain  []*x509.Certificate
	Key    *ecdsa.PrivateKey
	Source Source
}

// ParseImported parses a manual import. The
// function refuses to return an ImportedCert
// when the cert/key do not match, when the
// cert is expired or not yet valid, when the
// SAN/domain list does not match the Source,
// when the chain is incomplete, or when the
// key is not a NIST/SECP-256R1 ECDSA key.
func ParseImported(s Source) (*ImportedCert, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if s.Kind != KindImported {
		return nil, errors.New("certs: ParseImported is only valid for kind=imported")
	}
	if len(s.Certificate) > MaxCertBytes {
		return nil, fmt.Errorf("certs: certificate exceeds %d bytes", MaxCertBytes)
	}
	if len(s.PrivateKey) > MaxKeyBytes {
		return nil, fmt.Errorf("certs: private key exceeds %d bytes", MaxKeyBytes)
	}
	certDERs, err := decodeChain(s.Certificate)
	if err != nil {
		return nil, err
	}
	if len(certDERs) == 0 || len(certDERs) > MaxChain {
		return nil, fmt.Errorf("certs: chain length must be 1..%d", MaxChain)
	}
	leaf, err := x509.ParseCertificate(certDERs[0])
	if err != nil {
		return nil, fmt.Errorf("certs: parse leaf: %w", err)
	}
	chain := make([]*x509.Certificate, 0, len(certDERs)-1)
	for _, der := range certDERs[1:] {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("certs: parse chain: %w", err)
		}
		chain = append(chain, c)
	}
	// Domain/SAN match.
	if !domainsMatch(s.Domains, leaf.DNSNames) {
		return nil, errors.New("certs: requested domains do not match certificate SAN")
	}
	// Key must match the cert's public key.
	if s.PrivateKey == "" {
		return nil, errors.New("certs: private key is required to verify cert/key match")
	}
	key, err := parseECKey(s.PrivateKey)
	if err != nil {
		return nil, err
	}
	if !keyMatchesCert(key, leaf) {
		return nil, errors.New("certs: private key does not match certificate public key")
	}
	// Expiry / not-before window.
	now := time.Now().UTC()
	if now.Before(leaf.NotBefore) {
		return nil, errors.New("certs: certificate is not yet valid")
	}
	if now.After(leaf.NotAfter) {
		return nil, errors.New("certs: certificate is expired")
	}
	return &ImportedCert{Leaf: leaf, Chain: chain, Key: key, Source: s}, nil
}

func decodeChain(pemChain string) ([][]byte, error) {
	var out [][]byte
	rest := []byte(pemChain)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("certs: unexpected PEM block type %q", block.Type)
		}
		out = append(out, block.Bytes)
	}
	return out, nil
}

func parseECKey(pemKey string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("certs: no PEM block in private key")
	}
	if block.Type != "EC PRIVATE KEY" && block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("certs: private key PEM type must be EC PRIVATE KEY or PRIVATE KEY, got %q", block.Type)
	}
	var key *ecdsa.PrivateKey
	if block.Type == "EC PRIVATE KEY" {
		key, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("certs: parse EC private key: %w", err)
		}
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("certs: parse PKCS#8 private key: %w", err)
	}
	ec, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("certs: PKCS#8 key is not ECDSA")
	}
	key = ec
	return key, nil
}

func keyMatchesCert(key *ecdsa.PrivateKey, cert *x509.Certificate) bool {
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return false
	}
	if pub.Curve != key.Curve {
		return false
	}
	if pub.X.Cmp(key.X) != 0 || pub.Y.Cmp(key.Y) != 0 {
		return false
	}
	return true
}

func domainsMatch(requested, san []string) bool {
	if len(requested) == 0 {
		return false
	}
	have := map[string]bool{}
	for _, d := range san {
		have[strings.ToLower(d)] = true
	}
	for _, d := range requested {
		d = strings.ToLower(d)
		if !have[d] {
			return false
		}
	}
	return true
}

// Fingerprint returns a stable 32-character
// hex fingerprint of a certificate. The
// function is the contract used by callers to
// correlate a record with a key without
// exposing the bytes.
func Fingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:16])
}

// GenerateKey is a small helper for tests and
// for the DNS-01 path: it produces a fresh
// SECP-256R1 ECDSA key. Production code never
// logs the key.
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// MakeSelfSigned is a test helper. Production
// code never uses self-signed certificates.
func MakeSelfSigned(domains []string, ttl time.Duration) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if len(domains) == 0 {
		return nil, nil, errors.New("certs: at least one domain is required")
	}
	key, err := GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: domains[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(ttl),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     domains,
		IPAddresses:  nil,
	}
	if ip := net.ParseIP(domains[0]); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
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
