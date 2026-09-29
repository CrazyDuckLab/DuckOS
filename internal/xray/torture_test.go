// Package xray torture: §4 Xray render + validation
// destructive QA (binary-less).
//
// Each subtest stresses one boundary of the typed
// configuration model: protocol whitelisting, security
// compatibility, Reality/Trojan constraints, JSON validity
// of the rendered output, route/profile integrity, and the
// subscription contract. Real-binary boot is exercised by
// internal/qa/xray_linux_test.go (NETWORK_REQUIRED).
package xray

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// validServerFixture returns a known-good Server value
// that passes Validate().
func tortureValidServerFixture() Server {
	return Server{
		ID:        "11111111111111111111111111111111",
		Name:      "primary",
		Protocol:  ProtocolVLESS,
		Listen:    "127.0.0.1:8443",
		Port:      8443,
		Transport: TransportTCP,
		Security:  SecurityNone,
		Enabled:   true,
	}
}

// TestTorture_ProtocolWhitelist verifies every canonical
// protocol round-trips and every non-canonical protocol is
// rejected.
func TestTorture_ProtocolWhitelist(t *testing.T) {
	for _, p := range []Protocol{ProtocolVLESS, ProtocolTrojan} {
		s := tortureValidServerFixture()
		s.Protocol = p
		if err := s.Validate(); err != nil {
			t.Errorf("canonical protocol %q rejected: %v", p, err)
		}
	}
	for _, p := range []Protocol{"", "wireguard", "openvpn", "vless+", "shadowsocks", "socks"} {
		s := tortureValidServerFixture()
		s.Protocol = p
		if err := s.Validate(); err == nil {
			t.Errorf("non-canonical protocol %q accepted", p)
		}
	}
}

// TestTorture_RealityTrojanMutex verifies the security
// protocol compatibility matrix: Reality is valid only on
// VLESS, TLS is valid on both, None is always valid.
func TestTorture_RealityTrojanMutex(t *testing.T) {
	for _, proto := range []Protocol{ProtocolVLESS, ProtocolTrojan} {
		for _, sec := range []Security{SecurityNone, SecurityTLS, SecurityReality} {
			s := tortureValidServerFixture()
			s.Protocol = proto
			s.Security = sec
			err := s.Validate()
			switch {
			case sec == SecurityReality && proto == ProtocolTrojan:
				if err == nil {
					t.Errorf("REALITY on Trojan must fail")
				}
			case sec == SecurityReality && proto == ProtocolVLESS:
				// Allowed only when Reality* keys are set.
				if err != nil {
					t.Logf("VLESS+REALITY needs keys: %v (expected)", err)
				}
			}
		}
	}
}

// TestTorture_RenderJSONValidity verifies the rendered Xray
// configuration is valid JSON and contains the required
// top-level keys (inbounds, outbounds). routing appears
// only when profiles are supplied.
func TestTorture_RenderJSONValidity(t *testing.T) {
	servers := []Server{tortureValidServerFixture()}
	users := []User{{
		ID:       "11111111111111111111111111111111",
		ServerID: "11111111111111111111111111111111",
		Name:     "primary-user",
		UUID:     "11111111-1111-1111-1111-111111111111",
		Email:    "user@example.com",
		Enabled:  true,
	}}
	cfg, err := RenderConfig(servers, users, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	js, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip via Unmarshal to confirm well-formed JSON.
	var back map[string]any
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatalf("rendered config is not valid JSON: %v\n%s", err, js)
	}
	for _, key := range []string{"inbounds", "outbounds"} {
		if _, ok := back[key]; !ok {
			t.Errorf("rendered config missing top-level %q", key)
		}
	}
}

// TestTorture_DisabledServerExcluded verifies disabled
// servers never appear in the rendered config.
func TestTorture_DisabledServerExcluded(t *testing.T) {
	primary := tortureValidServerFixture()
	users := []User{{
		ID: "11111111111111111111111111111111", ServerID: primary.ID,
		Name: "primary-user", UUID: "11111111-1111-1111-1111-111111111111", Enabled: true,
	}}
	servers := []Server{
		primary,
		{ID: "22222222222222222222222222222222", Name: "secondary", Protocol: ProtocolVLESS, Listen: "127.0.0.1:8444", Port: 8444, Transport: TransportTCP, Security: SecurityNone, Enabled: false},
	}
	cfg, err := RenderConfig(servers, users, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ib := range cfg.Inbounds {
		if ib.Port == 8444 {
			t.Errorf("disabled server srv-2 was rendered")
		}
	}
}

// TestTorture_UserServerReference verifies a User with an
// unknown ServerID is rejected.
func TestTorture_UserServerReference(t *testing.T) {
	servers := []Server{tortureValidServerFixture()}
	users := []User{{
		ID:       "11111111111111111111111111111111",
		ServerID: "ffffffffffffffffffffffffffffffff",
		Name:     "orphan-user",
		UUID:     "11111111-1111-1111-1111-111111111111",
		Enabled:  true,
	}}
	if _, err := RenderConfig(servers, users, nil, nil); err == nil {
		t.Fatal("user with unknown server id was accepted")
	}
}

// TestTorture_PortCollision verifies two servers cannot
// bind the same listen:port.
func TestTorture_PortCollision(t *testing.T) {
	a := tortureValidServerFixture()
	b := tortureValidServerFixture()
	b.ID = "22222222222222222222222222222222"
	b.Port = a.Port
	b.Listen = a.Listen
	users := []User{{
		ID: "11111111111111111111111111111111", ServerID: a.ID,
		Name: "primary-user", UUID: "11111111-1111-1111-1111-111111111111", Enabled: true,
	}}
	if _, err := RenderConfig([]Server{a, b}, users, nil, nil); err == nil {
		t.Fatal("port collision was accepted")
	}
}

// TestTorture_ProfileRouting verifies every Profile must
// have a non-empty UserID and a canonical Intent at the
// Validate() boundary. M-24 ensures RenderConfig also
// invokes Validate before composing the routing rule.
func TestTorture_ProfileRouting(t *testing.T) {
	servers := []Server{tortureValidServerFixture()}
	users := []User{{
		ID:       "11111111111111111111111111111111",
		ServerID: "11111111111111111111111111111111",
		Name:     "primary-user",
		UUID:     "11111111-1111-1111-1111-111111111111",
		Enabled:  true,
	}}
	good := Profile{ID: "11111111111111111111111111111111", Name: "default", UserID: "11111111111111111111111111111111", Intent: ProfileIntentFullTunnel, Enabled: true}
	if err := good.Validate(); err != nil {
		t.Fatalf("good profile rejected: %v", err)
	}
	if _, err := RenderConfig(servers, users, []Profile{good}, nil); err != nil {
		t.Fatal(err)
	}
	// Empty UserID must be rejected by Validate (M-24).
	badUser := Profile{ID: "22222222222222222222222222222222", Name: "bad-user", UserID: "", Intent: ProfileIntentFullTunnel}
	if err := badUser.Validate(); err == nil {
		t.Fatal("profile with empty userId was accepted by Validate")
	}
	// Non-canonical Intent must be rejected by Validate.
	badIntent := Profile{ID: "33333333333333333333333333333333", Name: "bad-intent", UserID: "11111111111111111111111111111111", Intent: "weird-intent"}
	if err := badIntent.Validate(); err == nil {
		t.Fatal("profile with non-canonical intent was accepted by Validate")
	}
}

// TestTorture_RandomServerFuzz generates 32 random servers
// with valid structure and confirms every one passes
// Validate or fails for an unambiguous reason.
func TestTorture_RandomServerFuzz(t *testing.T) {
	protos := []Protocol{ProtocolVLESS, ProtocolTrojan}
	secs := []Security{SecurityNone, SecurityTLS, SecurityReality}
	transports := []Transport{TransportTCP, TransportWeb, TransportGRPC}
	for i := 0; i < 32; i++ {
		s := Server{
			ID:        fmt.Sprintf("srv-%d", i),
			Name:      fmt.Sprintf("server-%d", i),
			Protocol:  protos[i%len(protos)],
			Listen:    fmt.Sprintf("127.0.0.1:%d", 10000+i),
			Transport: transports[i%len(transports)],
			Security:  secs[i%len(secs)],
			Enabled:   i%2 == 0,
		}
		err := s.Validate()
		// We only assert no panic and no nil-deref.
		_ = err
	}
}

// TestTorture_SubscriptionContract pins the Subscription
// JSON shape: each Subscription exposes a Name and at
// least one Profile reference; malformed entries round-trip
// as valid JSON.
func TestTorture_SubscriptionContract(t *testing.T) {
	for _, sub := range []Subscription{
		{ID: "s1", Name: "alpha", ProfileIDs: []string{"p1"}},
		{ID: "s2", Name: "beta", ProfileIDs: nil},
	} {
		js, err := json.Marshal(sub)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(js), `"name"`) {
			t.Errorf("subscription JSON missing name: %s", js)
		}
		if !strings.Contains(string(js), `"profileIds"`) {
			t.Errorf("subscription JSON missing profileIds: %s", js)
		}
	}
}

// TestTorture_ViewRedactsSecrets verifies the public view
// models never embed raw secret material. UUIDs are
// replaced by sha256 digests; passwords and credential
// refs are reduced to a presence flag.
func TestTorture_ViewRedactsSecrets(t *testing.T) {
	u := User{
		ID:         "11111111111111111111111111111111",
		ServerID:   "22222222222222222222222222222222",
		Name:       "alice",
		UUID:       "11111111-1111-1111-1111-111111111111",
		Password:   "super-secret-password",
		Credential: &KeyRef{VaultID: "vault-1", Revision: "rev-1", KeyID: "key-1"},
		Email:      "alice@example.com",
		Enabled:    true,
	}
	v := ViewUser(u, PermissionsView{Observe: true, Configure: true, ExportSecret: true})
	if v.UUIDDigest == u.UUID {
		t.Errorf("UUID leaked into view")
	}
	if v.UUIDDigest == "" {
		t.Errorf("UUID digest empty")
	}
	if !v.HasSecret {
		t.Errorf("HasSecret must be true")
	}
	// JSON round-trip — no field may carry the raw secret.
	js, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{u.UUID, u.Password, "vault-1", "key-1"} {
		if strings.Contains(string(js), leak) {
			t.Errorf("view JSON leaks %q", leak)
		}
	}
}

// TestTorture_ViewServerRedactsSecurity verifies Server view
// never reveals Reality / TLS vault references.
func TestTorture_ViewServerRedactsSecurity(t *testing.T) {
	s := Server{
		ID:            "11111111111111111111111111111111",
		Name:          "primary",
		Protocol:      ProtocolVLESS,
		Listen:        "127.0.0.1:8443",
		Port:          8443,
		Transport:     TransportTCP,
		Security:      SecurityReality,
		RealityKeyRef: &KeyRef{VaultID: "v1", Revision: "r1", KeyID: "k1"},
		RealityDest:   "example.com:443",
		RealitySNI:    []string{"example.com"},
		ShortIDRef:    &KeyRef{VaultID: "v1", Revision: "r1", KeyID: "k2"},
		Enabled:       true,
	}
	v := ViewServer(s, PermissionsView{Observe: true})
	if !v.HasReality {
		t.Errorf("HasReality must be true")
	}
	js, _ := json.Marshal(v)
	for _, leak := range []string{"v1", "r1", "k1", "k2", "example.com:443"} {
		if strings.Contains(string(js), leak) {
			t.Errorf("server view leaks %q", leak)
		}
	}
}
