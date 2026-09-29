// acme/acme_test.go pins the V1 ACME
// state machine, key-authorization, and
// DNS-01 helper.
package acme

import (
	"testing"
	"time"
)

type stubDNS struct {
	created []string
	deleted []string
	err     error
}

func (s *stubDNS) Name() string { return "stub" }
func (s *stubDNS) CreateTXT(name, value string, ttl int) error {
	if s.err != nil {
		return s.err
	}
	s.created = append(s.created, name)
	return nil
}
func (s *stubDNS) DeleteTXT(name, value string) error {
	s.deleted = append(s.deleted, name)
	return nil
}

func TestKeyAuthorization(t *testing.T) {
	ka := KeyAuthorization("tok", "key-json")
	if ka == "" || ka[:3] != "tok" {
		t.Fatalf("bad ka: %q", ka)
	}
}

func TestDNSName(t *testing.T) {
	if got := DNSName("example.com"); got != "_acme-challenge.example.com" {
		t.Fatalf("dns name: %s", got)
	}
}

func TestProviderValid(t *testing.T) {
	for _, p := range []Provider{ProviderCloudflare, ProviderHetzner, ProviderDigitalOcean, ProviderRoute53, ProviderManual} {
		if !p.Valid() {
			t.Fatalf("%s must be valid", p)
		}
	}
	if Provider("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestRenewalValid(t *testing.T) {
	good := Renewal{ID: "r1", Domains: []string{"example.com", "*.example.com"}, Provider: ProviderCloudflare, State: StateIdle}
	if err := good.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Renewal{ID: "r1", Domains: []string{"!!bad"}, Provider: ProviderCloudflare, State: StateIdle}
	if err := bad.Valid(); err == nil {
		t.Fatal("bad domain must fail")
	}
	noDomains := Renewal{ID: "r1", Provider: ProviderCloudflare, State: StateIdle}
	if err := noDomains.Valid(); err == nil {
		t.Fatal("missing domains must fail")
	}
	badState := Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: "garbage"}
	if err := badState.Valid(); err == nil {
		t.Fatal("unknown state must fail")
	}
}

func TestManagerAddGetList(t *testing.T) {
	m := NewManager("https://acme.test/dir", "key", &StubTransport{}, &stubDNS{}, time.Now)
	if err := m.Add(Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: StateIdle}); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: StateIdle}); err == nil {
		t.Fatal("duplicate must fail")
	}
	if _, ok := m.Get("r1"); !ok {
		t.Fatal("get failed")
	}
	if len(m.List()) != 1 {
		t.Fatal("list size")
	}
}

func TestManagerRenew(t *testing.T) {
	trans := &StubTransport{
		Directory:  DirectoryEntry{NewAccount: "https://acme.test/acc", NewOrder: "https://acme.test/order"},
		Challenges: []Challenge{{URL: "https://acme.test/chal/1", Kind: ChallengeDNS01, Token: "tok-1"}},
		Cert:       []byte("FAKE-CERT"),
	}
	m := NewManager("https://acme.test/dir", "key", trans, &stubDNS{}, time.Now)
	if err := m.Add(Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: StateIdle}); err != nil {
		t.Fatal(err)
	}
	res := m.Renew("r1")
	if res.State != StateIssued {
		t.Fatalf("state: %s err: %s", res.State, res.Error)
	}
	r, _ := m.Get("r1")
	if r.LastIssued.IsZero() {
		t.Fatal("last issued must be set")
	}
	if r.NextRenew.IsZero() {
		t.Fatal("next renew must be set")
	}
	// Now run a second renewal: it must
	// promote the previously issued cert
	// to LastGoodCert. The StubTransport
	// returns a different CertURL each
	// call so the test can distinguish.
	trans.Finalized.CertURL = "https://acme.test/cert/2"
	res = m.Renew("r1")
	if res.State != StateIssued {
		t.Fatalf("state: %s err: %s", res.State, res.Error)
	}
	r, _ = m.Get("r1")
	if r.LastGoodCert != "https://acme.test/cert/1" {
		t.Fatalf("last-good not promoted: %q", r.LastGoodCert)
	}
	if r.CertPath != "https://acme.test/cert/2" {
		t.Fatalf("new cert path: %q", r.CertPath)
	}
}

func TestManagerRenewTransportError(t *testing.T) {
	trans := &StubTransport{SetError: errStub("transport down")}
	m := NewManager("https://acme.test/dir", "key", trans, nil, time.Now)
	m.Add(Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: StateIdle})
	res := m.Renew("r1")
	if res.State != StateFailed {
		t.Fatalf("expected failed, got %s", res.State)
	}
}

func TestManagerRevoke(t *testing.T) {
	trans := &StubTransport{}
	m := NewManager("https://acme.test/dir", "key", trans, nil, time.Now)
	m.Add(Renewal{ID: "r1", Domains: []string{"example.com"}, Provider: ProviderCloudflare, State: StateIdle})
	if err := m.Revoke("r1", "https://acme.test/cert/1", 0); err != nil {
		t.Fatal(err)
	}
	if !trans.RevokeCalled {
		t.Fatal("revoke not called")
	}
}

func TestVerifyNoPlaintext(t *testing.T) {
	if err := VerifyNoPlaintext("hello world", []string{"secret"}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyNoPlaintext("hello secret world", []string{"secret"}); err == nil {
		t.Fatal("leak must be detected")
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
