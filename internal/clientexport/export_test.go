package clientexport

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

const testID = "0123456789abcdef0123456789abcdef"
const testRevision = "1123456789abcdef0123456789abcdef"

type providerFunc func(context.Context, string) (*Document, error)

func (f providerFunc) Export(ctx context.Context, id string) (*Document, error) { return f(ctx, id) }

func wgFixture(t *testing.T) (WireGuardClient, []byte) {
	t.Helper()
	key, _, err := GenerateWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	_, public, err := GenerateWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	return WireGuardClient{Addresses: []string{"10.50.0.2/32", "fd00:50::2/128"}, DNS: []string{"10.50.0.1"}, ServerPublicKey: public, Endpoint: "vpn.example.test:51820", AllowedIPs: []string{"0.0.0.0/0", "::/0"}, PersistentKeepalive: 25}, key
}

func TestSubscriptionPayloadIsExactAndSecret(t *testing.T) {
	url := "https://vpn.example.test/sub?token=synthetic%2Fsecret&format=happ"
	doc, err := SubscriptionURL(testID, testRevision, url)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Reveal() != url || string(doc.Download()) != url {
		t.Fatal("URL normalized or changed between exports")
	}
	checkRedaction(t, doc, "synthetic%2Fsecret")
	for _, bad := range []string{"http://example.test/sub", "https://example.test/sub#credential", "https://example.test/\nsecret"} {
		if _, err := SubscriptionURL(testID, testRevision, bad); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
}

func checkRedaction(t *testing.T, doc *Document, secret string) {
	t.Helper()
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("export", "document", doc)
	if strings.Contains(log.String(), secret) || strings.Contains(fmt.Sprintf("%v %+v %#v", doc, *doc, doc), secret) {
		t.Fatal("secret appeared in logging")
	}
	if data, err := json.Marshal(doc); err == nil || bytes.Contains(data, []byte(secret)) {
		t.Fatal("implicit serialization released secret")
	}
}

func TestWireGuardExportsAndRotation(t *testing.T) {
	c, key := wgFixture(t)
	doc, err := WireGuard(testID, testRevision, c, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "[Interface]\nPrivateKey = " + base64.StdEncoding.EncodeToString(key) + "\nAddress = 10.50.0.2/32, fd00:50::2/128\nDNS = 10.50.0.1\n\n[Peer]\nPublicKey = " + c.ServerPublicKey + "\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = vpn.example.test:51820\nPersistentKeepalive = 25\n"
	if doc.Reveal() != want || string(doc.Download()) != want || !strings.HasSuffix(doc.Filename(), ".conf") {
		t.Fatal("WireGuard export mismatch")
	}
	checkRedaction(t, doc, base64.StdEncoding.EncodeToString(key))
	key2, _, err := GenerateWireGuardKey()
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := WireGuard(testID, "2123456789abcdef0123456789abcdef", c, key2, nil)
	if err != nil || rotated.Reveal() == doc.Reveal() || rotated.Revision() == doc.Revision() || strings.Contains(rotated.Reveal(), base64.StdEncoding.EncodeToString(key)) {
		t.Fatal("rotation reused old export", err)
	}
	if key2[0]&7 != 0 || key2[31]&128 != 0 || key2[31]&64 == 0 {
		t.Fatal("nonstandard private key clamp")
	}
}

func TestWireGuardRejectsInjectionAndUnsupportedConfiguration(t *testing.T) {
	for name, mutate := range map[string]func(*WireGuardClient){
		"endpoint injection": func(c *WireGuardClient) { c.Endpoint = "vpn.example.test:51820\nPostUp=id" },
		"address injection":  func(c *WireGuardClient) { c.Addresses = []string{"10.0.0.2/32\nPostUp=id"} },
		"DNS injection":      func(c *WireGuardClient) { c.DNS = []string{"10.0.0.1\nSaveConfig=true"} },
		"wrong key":          func(c *WireGuardClient) { c.ServerPublicKey = "invalid" },
		"low order key": func(c *WireGuardClient) {
			c.ServerPublicKey = base64.StdEncoding.EncodeToString(append([]byte{1}, make([]byte, 31)...))
		},
		"noncanonical route": func(c *WireGuardClient) { c.AllowedIPs = []string{"10.0.0.2/24"} },
		"invalid keepalive":  func(c *WireGuardClient) { c.PersistentKeepalive = 65536 },
	} {
		t.Run(name, func(t *testing.T) {
			c, key := wgFixture(t)
			mutate(&c)
			if _, err := WireGuard(testID, testRevision, c, key, nil); err == nil {
				t.Fatal("invalid client configuration accepted")
			}
		})
	}
	if _, err := NewDocument(testID, testRevision, "wireguard", "client.conf", strings.Repeat("x", MaxPayloadBytes+1)); err == nil {
		t.Fatal("oversized QR payload silently accepted")
	}
}

func TestExportServiceAuthenticatesBeforeResolutionAndAuditsWithoutPayload(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	const password = "Synthetic export test password 42!"
	if _, err := auth.Enroll("owner", password); err != nil {
		t.Fatal(err)
	}
	owner, _, err := auth.Login("owner", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(owner, "reader", password, identity.ReadOnly); err != nil {
		t.Fatal(err)
	}
	reader, _, err := auth.Login("reader", password)
	if err != nil {
		t.Fatal(err)
	}
	url, revision, calls := "https://vpn.example.test/sub?token=synthetic-secret-one", testRevision, 0
	p := providerFunc(func(context.Context, string) (*Document, error) {
		calls++
		return SubscriptionURL(testID, revision, url)
	})
	service, err := NewService(auth, map[string]Provider{"subscription": p})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"invalid", reader} {
		if _, err := service.Export(context.Background(), token, "subscription", testID, "qr"); err == nil {
			t.Fatal("unauthorized secret retrieval")
		}
	}
	if calls != 0 {
		t.Fatal("provider read secrets before authorization")
	}
	first, err := service.Export(context.Background(), owner, "subscription", testID, "qr")
	if err != nil {
		t.Fatal(err)
	}
	url, revision = "https://vpn.example.test/sub?token=synthetic-secret-two", "2123456789abcdef0123456789abcdef"
	second, err := service.Export(context.Background(), owner, "subscription", testID, "download")
	if err != nil || second.Reveal() == first.Reveal() || second.Revision() == first.Revision() {
		t.Fatal("service cached a rotated secret", err)
	}
	audit, err := auth.Audit(owner)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(audit)
	if bytes.Contains(raw, []byte("synthetic-secret")) || bytes.Contains(raw, []byte("https://")) {
		t.Fatal("audit contained secret URL")
	}
	if !bytes.Contains(raw, []byte("client-export.subscription.qr")) || !bytes.Contains(raw, []byte("client-export.subscription.download")) {
		t.Fatal("secret views were not audited")
	}
	if err := auth.RecordClientExport(owner, "subscription", testID, "secret-url-payload"); err == nil {
		t.Fatal("arbitrary audit payload accepted")
	}
}

type denyAudit struct{}

func (denyAudit) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{}, nil
}
func (denyAudit) RecordClientExport(string, string, string, string) error {
	return identity.ErrUnauthorized
}

func TestFailedFinalAuthorizationWithholdsDocument(t *testing.T) {
	p := providerFunc(func(context.Context, string) (*Document, error) {
		return SubscriptionURL(testID, testRevision, "https://example.test/sub?token=synthetic")
	})
	s, _ := NewService(denyAudit{}, map[string]Provider{"subscription": p})
	doc, err := s.Export(context.Background(), "session", "subscription", testID, "qr")
	if doc != nil || !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("document returned after final authorization failed")
	}
}
