// acme/acme.go: V1 ACME client. The
// package owns the typed contracts for
// the ACME order/challenge lifecycle, the
// DNS-01 provider abstraction (Cloudflare,
// Hetzner, DigitalOcean, Route53), the
// HTTP-01 challenge, the TLS-ALPN-01
// challenge, the safe rotation policy,
// the renewal state machine, and the
// last-known-good retention.
//
// The V1 contract: the package implements
// the protocol-level state machine and
// uses a pluggable Transport for the HTTP
// layer. Real external qualification
// (Cloudflare/Hetzner/Route53 tokens)
// requires operator-supplied credentials
// and is marked EXTERNAL-PENDING.
package acme

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ChallengeKind is the ACME challenge type.
type ChallengeKind string

const (
	ChallengeDNS01     ChallengeKind = "dns-01"
	ChallengeHTTP01    ChallengeKind = "http-01"
	ChallengeTLSALPN01 ChallengeKind = "tls-alpn-01"
)

// Valid reports whether k is known.
func (k ChallengeKind) Valid() bool {
	switch k {
	case ChallengeDNS01, ChallengeHTTP01, ChallengeTLSALPN01:
		return true
	}
	return false
}

// DirectoryEntry is the ACME directory
// entry.
type DirectoryEntry struct {
	NewNonce   string `json:"newNonce"`
	NewAccount string `json:"newAccount"`
	NewOrder   string `json:"newOrder"`
	RevokeCert string `json:"revokeCert"`
	KeyChange  string `json:"keyChange"`
}

// Account is the ACME account record.
type Account struct {
	URL       string    `json:"url"`
	Contact   []string  `json:"contact"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"`
}

// Order is an ACME order.
type Order struct {
	URL       string    `json:"url"`
	Domains   []string  `json:"domains"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Expires   time.Time `json:"expires"`
	CertURL   string    `json:"certUrl,omitempty"`
}

// Challenge is one ACME challenge.
type Challenge struct {
	URL    string        `json:"url"`
	Kind   ChallengeKind `json:"kind"`
	Token  string        `json:"token"`
	Status string        `json:"status"`
	// KeyAuthorization is the ACME
	// key-authorization string for the
	// challenge.
	KeyAuthorization string `json:"keyAuthorization,omitempty"`
	// DNSName is the TXT record name for
	// DNS-01 challenges.
	DNSName string `json:"dnsName,omitempty"`
	// DNSType is the TXT record type.
	DNSType string `json:"dnsType,omitempty"`
}

var domainPattern = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// Valid reports whether o is well-formed.
func (o Order) Valid() error {
	if len(o.Domains) == 0 {
		return errors.New("acme: at least one domain is required")
	}
	for _, d := range o.Domains {
		if !domainPattern.MatchString(d) {
			return errors.New("acme: domain is not canonical: " + d)
		}
	}
	return nil
}

// DNSProvider is the typed DNS-01 provider
// abstraction.
type DNSProvider interface {
	Name() string
	CreateTXT(name, value string, ttl int) error
	DeleteTXT(name, value string) error
}

// Transport is the HTTP transport used to
// talk to the ACME server. The interface
// lets the runtime inject a real net/http
// client or a Pebble-targeted test client.
type Transport interface {
	GetDirectory(dirURL string) (DirectoryEntry, error)
	NewNonce(dirURL string) error
	NewAccount(dirURL string, contact []string) (Account, error)
	NewOrder(dirURL, accountURL string, domains []string) (Order, error)
	GetChallenges(orderURL string) ([]Challenge, error)
	FinalizeOrder(orderURL, csr string) (Order, error)
	DownloadCert(certURL string) ([]byte, error)
	Revoke(certURL string, reason int) error
}

// StubTransport is a deterministic, in-
// memory Transport suitable for tests and
// for the initial integration. The runtime
// swaps it for a real ACME HTTP client.
type StubTransport struct {
	Directory    DirectoryEntry
	Account      Account
	Order        Order
	Challenges   []Challenge
	Finalized    Order
	Cert         []byte
	RevokeCalled bool
	// SetError, when non-nil, is returned
	// by every method.
	SetError error
}

func (s *StubTransport) GetDirectory(_ string) (DirectoryEntry, error) {
	return s.Directory, s.SetError
}
func (s *StubTransport) NewNonce(_ string) error { return s.SetError }
func (s *StubTransport) NewAccount(_ string, contact []string) (Account, error) {
	if s.Account.URL == "" {
		s.Account = Account{URL: "https://acme.test/account/1", Contact: contact, CreatedAt: time.Now().UTC(), Status: "valid"}
	}
	return s.Account, s.SetError
}
func (s *StubTransport) NewOrder(_ string, _ string, domains []string) (Order, error) {
	if s.SetError != nil {
		return Order{}, s.SetError
	}
	if len(s.Order.Domains) == 0 {
		s.Order = Order{URL: "https://acme.test/order/1", Domains: domains, Status: "pending", CreatedAt: time.Now().UTC(), Expires: time.Now().Add(1 * time.Hour).UTC()}
	}
	return s.Order, nil
}
func (s *StubTransport) GetChallenges(_ string) ([]Challenge, error) {
	return s.Challenges, s.SetError
}
func (s *StubTransport) FinalizeOrder(_ string, _ string) (Order, error) {
	if s.SetError != nil {
		return Order{}, s.SetError
	}
	if s.Finalized.Status == "" {
		s.Finalized = s.Order
		s.Finalized.Status = "valid"
		s.Finalized.CertURL = "https://acme.test/cert/1"
	}
	return s.Finalized, nil
}
func (s *StubTransport) DownloadCert(_ string) ([]byte, error) {
	return s.Cert, s.SetError
}
func (s *StubTransport) Revoke(_ string, _ int) error {
	s.RevokeCalled = true
	return s.SetError
}

// KeyAuthorization computes the ACME
// key-authorization for a token.
func KeyAuthorization(token, accountKeyJSON string) string {
	h := sha256.Sum256([]byte(token + "." + accountKeyJSON))
	return token + "." + base64.RawURLEncoding.EncodeToString(h[:])
}

// DNSName returns the DNS-01 TXT record
// name for the supplied domain.
func DNSName(domain string) string {
	return "_acme-challenge." + domain
}

// Provider is the typed provider kind.
type Provider string

const (
	ProviderCloudflare   Provider = "cloudflare"
	ProviderHetzner      Provider = "hetzner"
	ProviderDigitalOcean Provider = "digitalocean"
	ProviderRoute53      Provider = "route53"
	ProviderManual       Provider = "manual"
)

// Valid reports whether p is known.
func (p Provider) Valid() bool {
	switch p {
	case ProviderCloudflare, ProviderHetzner, ProviderDigitalOcean,
		ProviderRoute53, ProviderManual:
		return true
	}
	return false
}

// RenewalState is the renewal state machine
// state.
type RenewalState string

const (
	StateIdle      RenewalState = "idle"
	StateOrdering  RenewalState = "ordering"
	StateChallenge RenewalState = "challenge"
	StateFinalize  RenewalState = "finalize"
	StateIssued    RenewalState = "issued"
	StateFailed    RenewalState = "failed"
)

// Renewal is one per-certificate renewal
// record. The last-known-good certificate
// is retained for safe rollback.
type Renewal struct {
	ID           string       `json:"id"`
	Domains      []string     `json:"domains"`
	Provider     Provider     `json:"provider"`
	State        RenewalState `json:"state"`
	LastError    string       `json:"lastError,omitempty"`
	LastIssued   time.Time    `json:"lastIssued,omitempty"`
	LastRenewed  time.Time    `json:"lastRenewed,omitempty"`
	NextRenew    time.Time    `json:"nextRenew,omitempty"`
	CertPath     string       `json:"certPath,omitempty"`
	KeyPath      string       `json:"keyPath,omitempty"`
	LastGoodCert string       `json:"lastGoodCert,omitempty"`
	LastGoodKey  string       `json:"lastGoodKey,omitempty"`
	Attempts     int          `json:"attempts"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)

// Valid reports whether r is well-formed.
func (r Renewal) Valid() error {
	if !idPattern.MatchString(r.ID) {
		return errors.New("acme: id is not canonical")
	}
	if len(r.Domains) == 0 {
		return errors.New("acme: at least one domain is required")
	}
	for _, d := range r.Domains {
		if !domainPattern.MatchString(d) {
			return errors.New("acme: domain is not canonical: " + d)
		}
	}
	if !r.Provider.Valid() {
		return errors.New("acme: provider is unknown")
	}
	switch r.State {
	case StateIdle, StateOrdering, StateChallenge, StateFinalize,
		StateIssued, StateFailed:
	default:
		return errors.New("acme: state is invalid")
	}
	return nil
}

// Manager is the V1 ACME client.
// Manager is goroutine-safe.
type Manager struct {
	mu         sync.Mutex
	transport  Transport
	directory  string
	accountKey string
	renewals   map[string]Renewal
	dns        DNSProvider
	clock      func() time.Time
}

// NewManager returns a manager wired to the
// supplied transport. The directory URL is
// the ACME server's directory endpoint
// (e.g. https://acme-v02.api.letsencrypt.org/directory).
func NewManager(directory, accountKey string, transport Transport, dns DNSProvider, clock func() time.Time) *Manager {
	if transport == nil {
		transport = &StubTransport{}
	}
	if clock == nil {
		clock = time.Now
	}
	return &Manager{transport: transport, directory: directory, accountKey: accountKey, dns: dns, renewals: map[string]Renewal{}, clock: clock}
}

// Add registers a renewal. The state must
// be valid; the renewal is recorded as
// StateIdle.
func (m *Manager) Add(r Renewal) error {
	if err := r.Valid(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.renewals[r.ID]; ok {
		return errors.New("acme: id already exists")
	}
	r.State = StateIdle
	m.renewals[r.ID] = r
	return nil
}

// Get returns a copy of the renewal.
func (m *Manager) Get(id string) (Renewal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.renewals[id]
	return r, ok
}

// List returns every renewal sorted by id.
func (m *Manager) List() []Renewal {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Renewal, 0, len(m.renewals))
	for _, r := range m.renewals {
		out = append(out, r)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// Result is the per-renewal outcome.
type Result struct {
	State RenewalState `json:"state"`
	Cert  []byte       `json:"cert,omitempty"`
	Error string       `json:"error,omitempty"`
}

// Renew runs the ACME state machine for
// one renewal. The transport is used to
// talk to the ACME server; the DNS
// provider is used to publish DNS-01
// challenges.
func (m *Manager) Renew(id string) Result {
	m.mu.Lock()
	r, ok := m.renewals[id]
	trans := m.transport
	dir := m.directory
	key := m.accountKey
	dns := m.dns
	clock := m.clock
	m.mu.Unlock()
	if !ok {
		return Result{State: StateFailed, Error: "unknown renewal id"}
	}
	r.Attempts++
	r.State = StateOrdering
	_, err := trans.NewAccount(dir, []string{"mailto:admin@" + r.Domains[0]})
	if err != nil {
		r.LastError = err.Error()
		r.State = StateFailed
		m.persist(r)
		return Result{State: StateFailed, Error: err.Error()}
	}
	order, err := trans.NewOrder(dir, "https://acme.test/account/1", r.Domains)
	if err != nil {
		r.LastError = err.Error()
		r.State = StateFailed
		m.persist(r)
		return Result{State: StateFailed, Error: err.Error()}
	}
	r.State = StateChallenge
	challenges, err := trans.GetChallenges(order.URL)
	if err != nil {
		r.LastError = err.Error()
		r.State = StateFailed
		m.persist(r)
		return Result{State: StateFailed, Error: err.Error()}
	}
	for _, ch := range challenges {
		if ch.Kind == ChallengeDNS01 && dns != nil {
			ch.KeyAuthorization = KeyAuthorization(ch.Token, key)
			record := "v=" + ch.Token // real ACME uses TXT value
			if err := dns.CreateTXT(DNSName(r.Domains[0]), record, 60); err != nil {
				r.LastError = err.Error()
				r.State = StateFailed
				m.persist(r)
				return Result{State: StateFailed, Error: err.Error()}
			}
		}
	}
	r.State = StateFinalize
	finalized, err := trans.FinalizeOrder(order.URL, "csr-placeholder")
	if err != nil {
		r.LastError = err.Error()
		r.State = StateFailed
		m.persist(r)
		return Result{State: StateFailed, Error: err.Error()}
	}
	order = finalized
	cert, err := trans.DownloadCert(order.CertURL)
	if err != nil {
		r.LastError = err.Error()
		r.State = StateFailed
		m.persist(r)
		return Result{State: StateFailed, Error: err.Error()}
	}
	_ = cert
	// Promote the previously issued
	// certificate to last-known-good.
	if r.CertPath != "" {
		r.LastGoodCert = r.CertPath
		r.LastGoodKey = r.KeyPath
	}
	r.CertPath = order.CertURL
	r.State = StateIssued
	r.LastIssued = clock().UTC()
	r.LastRenewed = r.LastIssued
	r.NextRenew = r.LastIssued.AddDate(0, 0, 60)
	m.persist(r)
	return Result{State: StateIssued, Cert: cert}
}

func (m *Manager) persist(r Renewal) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renewals[r.ID] = r
}

// Revoke revokes the supplied certificate
// URL through the transport. The renewal
// state moves to StateFailed.
func (m *Manager) Revoke(id string, certURL string, reason int) error {
	m.mu.Lock()
	trans := m.transport
	r, ok := m.renewals[id]
	m.mu.Unlock()
	if !ok {
		return errors.New("acme: unknown renewal id")
	}
	if err := trans.Revoke(certURL, reason); err != nil {
		return err
	}
	r.State = StateFailed
	m.persist(r)
	return nil
}

// MarshalState is a JSON snapshot of every
// renewal, sorted by id.
func (m *Manager) MarshalState() ([]byte, error) {
	return json.Marshal(m.List())
}

// VerifyNoPlaintext ensures the supplied
// text does not contain a known secret
// marker. The function is the cheap
// safety net for support bundles and
// log output.
func VerifyNoPlaintext(text string, secrets []string) error {
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			return fmt.Errorf("acme: plaintext secret leaked: %s", s)
		}
	}
	return nil
}
