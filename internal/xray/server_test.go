package xray

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

type testAccess struct{}

func (testAccess) Authorize(string, identity.Permission) (identity.User, error) {
	return identity.User{Username: "owner", Role: identity.Owner}, nil
}

func validServerFixture() Server {
	return Server{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      "primary",
		Protocol:  ProtocolVLESS,
		Transport: TransportTCP,
		Security:  SecurityNone,
		Listen:    "127.0.0.1:8443",
		Port:      8443,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func validUserFixture(serverID string) User {
	return User{
		ID:        "fedcba9876543210fedcba9876543210",
		ServerID:  serverID,
		Name:      "alice",
		UUID:      "01234567-89ab-cdef-0123-456789abcdef",
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestServerValidateAcceptsCanonicalForm(t *testing.T) {
	server := validServerFixture()
	if err := server.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestServerValidateRejectsBadProtocol(t *testing.T) {
	server := validServerFixture()
	server.Protocol = "shadowsocks"
	if err := server.Validate(); err == nil {
		t.Fatal("invalid protocol was accepted")
	}
}

func TestServerValidateRejectsRealityOnTrojan(t *testing.T) {
	server := validServerFixture()
	server.Protocol = ProtocolTrojan
	server.Security = SecurityReality
	if err := server.Validate(); err == nil {
		t.Fatal("REALITY on Trojan was accepted")
	}
}

func TestServerValidateRequiresRealityKeyAndShortID(t *testing.T) {
	server := validServerFixture()
	server.Security = SecurityReality
	server.RealityDest = "example.com:443"
	server.RealitySNI = []string{"example.com"}
	if err := server.Validate(); err == nil {
		t.Fatal("REALITY without keys was accepted")
	}
	server.RealityKeyRef = &KeyRef{VaultID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32)}
	server.ShortIDRef = &KeyRef{VaultID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32)}
	if err := server.Validate(); err != nil {
		t.Fatal("REALITY with valid keys was rejected", err)
	}
}

func TestServerValidateRequiresTLSReferences(t *testing.T) {
	server := validServerFixture()
	server.Security = SecurityTLS
	if err := server.Validate(); err == nil {
		t.Fatal("TLS without cert/key was accepted")
	}
}

func TestUserValidateAcceptsUUID(t *testing.T) {
	user := validUserFixture("0123456789abcdef0123456789abcdef")
	if err := user.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUserValidateRejectsBadUUID(t *testing.T) {
	user := validUserFixture("0123456789abcdef0123456789abcdef")
	user.UUID = "not-a-uuid"
	if err := user.Validate(); err == nil {
		t.Fatal("non-canonical UUID was accepted")
	}
}

func TestUserValidateRejectsMissingCredentials(t *testing.T) {
	user := validUserFixture("0123456789abcdef0123456789abcdef")
	user.UUID = ""
	if err := user.Validate(); err == nil {
		t.Fatal("user without any credential was accepted")
	}
}

func TestProfileValidateEnforcesIntent(t *testing.T) {
	profile := Profile{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      "primary",
		UserID:    "fedcba9876543210fedcba9876543210",
		Intent:    "wild-west",
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := profile.Validate(); err == nil {
		t.Fatal("unknown intent was accepted")
	}
	profile.Intent = ProfileIntentSelection
	if err := profile.Validate(); err == nil {
		t.Fatal("selection intent with no CIDRs was accepted")
	}
	profile.Selection = []string{"10.0.0.0/8"}
	if err := profile.Validate(); err != nil {
		t.Fatal("valid selection profile was rejected", err)
	}
}

func TestSubscriptionValidateEnforcesUniqueProfiles(t *testing.T) {
	sub := Subscription{
		ID:         "0123456789abcdef0123456789abcdef",
		Name:       "guests",
		ProfileIDs: []string{"0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef"},
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if err := sub.Validate(); err == nil {
		t.Fatal("duplicate profile ids were accepted")
	}
	sub.ProfileIDs = []string{"0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210"}
	if err := sub.Validate(); err != nil {
		t.Fatal("unique profiles were rejected", err)
	}
}

func TestIsValidUUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"01234567-89ab-cdef-0123-456789abcdef", true},
		{"01234567-89AB-cdef-0123-456789abcdef", false}, // uppercase
		{"not-a-uuid", false},
		{"", false},
		{"012345678-9ab-cdef-0123-456789abcdef", false}, // wrong length
	}
	for _, c := range cases {
		if got := isValidUUID(c.in); got != c.want {
			t.Fatalf("isValidUUID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRenderConfigEmitsInbound(t *testing.T) {
	server := validServerFixture()
	user := validUserFixture(server.ID)
	cfg, err := RenderConfig([]Server{server}, []User{user}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatal("expected one inbound, got", len(cfg.Inbounds))
	}
	if cfg.Inbounds[0].Protocol != "vless" {
		t.Fatal("protocol was not vless")
	}
}

func TestAPIServerWithoutUsersCommitsCatalogWithoutRuntimeApply(t *testing.T) {
	cat, err := OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	api := &API{
		Access:  testAccess{},
		Catalog: cat,
		Prefix:  "/xray/v1/",
		Apply: func(context.Context, []Server, []User) error {
			called = true
			return nil
		},
	}
	body := `{"id":"0123456789abcdef0123456789abcdef","name":"primary","protocol":"vless","transport":"tcp","security":"none","listen":"127.0.0.1:8443","port":8443,"enabled":true}`
	r := httptest.NewRequest(http.MethodPost, "http://agent/xray/v1/servers", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer owner")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("server-only mutation failed: %d %s", w.Code, w.Body.String())
	}
	if called {
		t.Fatal("runtime apply should wait until the server has at least one enabled user")
	}
	servers, err := cat.ListServers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Fatalf("catalog did not commit pending server: %#v", servers)
	}
}

func TestAPIRuntimeFailureDoesNotCommitCompletedConfigMutation(t *testing.T) {
	cat, err := OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := validServerFixture()
	if err := cat.UpsertServer(context.Background(), server); err != nil {
		t.Fatal(err)
	}
	api := &API{
		Access:  testAccess{},
		Catalog: cat,
		Prefix:  "/xray/v1/",
		Apply: func(context.Context, []Server, []User) error {
			return errors.New("forced runtime failure")
		},
	}
	user := validUserFixture(server.ID)
	raw, _ := json.Marshal(user)
	r := httptest.NewRequest(http.MethodPost, "http://agent/xray/v1/users", strings.NewReader(string(raw)))
	r.Header.Set("Authorization", "Bearer owner")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code == http.StatusCreated || w.Code == http.StatusOK {
		t.Fatalf("runtime failure reported success: %d %s", w.Code, w.Body.String())
	}
	users, err := cat.ListUsers(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("catalog committed user despite runtime failure: %#v", users)
	}
}

func TestRenderConfigSkipsDisabledServers(t *testing.T) {
	server := validServerFixture()
	server.Enabled = false
	user := validUserFixture(server.ID)
	cfg, err := RenderConfig([]Server{server}, []User{user}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 0 {
		t.Fatal("disabled server produced an inbound")
	}
}

func TestRenderConfigRejectsUnsupportedCombination(t *testing.T) {
	server := validServerFixture()
	server.Protocol = ProtocolTrojan
	server.Security = SecurityReality
	user := validUserFixture(server.ID)
	user.UUID = ""
	user.Password = "secret-password"
	if _, err := RenderConfig([]Server{server}, []User{user}, nil, nil); err == nil {
		t.Fatal("REALITY on Trojan was rendered")
	}
}

func TestBuildShareLinkUsesCredentialReference(t *testing.T) {
	server := validServerFixture()
	user := validUserFixture(server.ID)
	user.Credential = &KeyRef{VaultID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", KeyID: "cccccccccccccccccccccccccccccccc"}
	link, err := BuildShareLink(server, user)
	if err != nil {
		t.Fatal(err)
	}
	if link == "" || (link[:6] != "vless:" && link[:8] != "trojan:") {
		t.Fatal("share link has the wrong scheme", link)
	}
	if contains(link, "secret-password") || contains(link, user.UUID) {
		t.Fatal("share link leaked secret bytes")
	}
}

func TestBuildShareLinkRejectsExcessLength(t *testing.T) {
	server := validServerFixture()
	user := validUserFixture(server.ID)
	// Long credential reference forces the link to exceed the limit.
	user.Credential = &KeyRef{VaultID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("c", 32)}
	// Force excess by creating an absurdly long name.
	server.Name = strings.Repeat("a", 80)
	if _, err := BuildShareLink(server, user); err == nil {
		t.Fatal("excess link was accepted")
	}
}

func TestRenderConfigRoutesFromProfiles(t *testing.T) {
	server := validServerFixture()
	user := validUserFixture(server.ID)
	profile := Profile{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      "primary",
		UserID:    user.ID,
		Intent:    ProfileIntentSelection,
		Selection: []string{"10.0.0.0/8", "192.168.0.0/16"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	cfg, err := RenderConfig([]Server{server}, []User{user}, []Profile{profile}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Routing == nil || len(cfg.Routing.Rules) != 1 {
		t.Fatal("expected exactly one routing rule")
	}
	rule := cfg.Routing.Rules[0]
	if rule.OutboundTag != "direct" {
		t.Fatal("unexpected outbound tag", rule.OutboundTag)
	}
	if len(rule.IPs) != 2 {
		t.Fatal("expected two selection CIDRs, got", rule.IPs)
	}
}

func TestRenderConfigJSONIsValid(t *testing.T) {
	// The internal Version field is intentionally
	// hidden from the JSON wire format
	// (json:"-") — see commit be03086 — because
	// xray-core's loader rejects an unknown top-
	// level "version" key. The runtime lives in
	// Config.Version in memory; the wire format
	// carries only inbounds / outbounds / routing
	// / dns.
	//
	// This regression test therefore asserts the
	// CURRENT schema: the wire JSON must contain
	// inbounds + outbounds, and must NOT contain
	// the internal version field.
	server := validServerFixture()
	user := validUserFixture(server.ID)
	cfg, err := RenderConfig([]Server{server}, []User{user}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Fatalf("Config.Version in memory must be 1; got %d", cfg.Version)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	// Version is internal — it MUST NOT leak to the
	// xray-core wire format.
	if _, present := roundTrip["version"]; present {
		t.Fatalf("Config.Version leaked to the wire format: %s", data)
	}
	if _, ok := roundTrip["inbounds"]; !ok {
		t.Fatalf("wire format must contain inbounds; got %s", data)
	}
	if _, ok := roundTrip["outbounds"]; !ok {
		t.Fatalf("wire format must contain outbounds; got %s", data)
	}
}

func contains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
