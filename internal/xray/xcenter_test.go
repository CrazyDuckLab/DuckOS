// Package xray Xray Center tests: REALITY key generation, resolved
// share links and strict connection-URI import round-trips.
package xray

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateRealityKeypair(t *testing.T) {
	a, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if a.PrivateKey == b.PrivateKey || a.PublicKey == b.PublicKey {
		t.Fatal("keypair generation is not random")
	}
	for _, k := range []string{a.PublicKey, a.PrivateKey, b.PublicKey, b.PrivateKey} {
		if len(k) != 43 {
			t.Fatalf("x25519 key %q is not 43-char raw URL base64", k)
		}
	}
	if !isRealityPublicKey(a.PublicKey) {
		t.Fatal("generated public key fails the canonical validator")
	}
}

func TestGenerateShortIDAndUUID(t *testing.T) {
	sid, err := GenerateShortID()
	if err != nil {
		t.Fatal(err)
	}
	if len(sid) != MaxShortIDHex {
		t.Fatalf("short id %q is not %d hex chars", sid, MaxShortIDHex)
	}
	u1, err := GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	if !isValidUUID(u1) {
		t.Fatalf("generated uuid %q is not canonical", u1)
	}
	if u1[14] != '4' {
		t.Fatalf("generated uuid %q is not version 4", u1)
	}
}

func xcenterRealityServer() Server {
	kp, err := GenerateRealityKeypair()
	if err != nil {
		panic(err)
	}
	return Server{
		ID: "33333333333333333333333333333333", Name: "reality-1",
		Protocol: ProtocolVLESS, Listen: "0.0.0.0:443", Port: 443,
		Transport: TransportTCP, Security: SecurityReality,
		RealityDest: "example.com:443", RealitySNI: []string{"example.com"},
		Fingerprint: "chrome", RealityPublicKey: kp.PublicKey,
		RealityKeyRef: &KeyRef{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "cccccccccccccccccccccccccccccccc"},
		ShortIDRef:    &KeyRef{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "dddddddddddddddddddddddddddddddd"},
		Enabled:       true,
	}
}

func TestBuildResolvedShareLink_VLESSReality(t *testing.T) {
	s := xcenterRealityServer()
	u := User{ID: "44444444444444444444444444444444", ServerID: s.ID, Name: "alice",
		UUID: "11111111-1111-1111-1111-111111111111", Flow: FlowVision, Enabled: true}
	link, err := BuildResolvedShareLink(s, u, LinkParams{
		Credential:       u.UUID,
		RealityPublicKey: s.RealityPublicKey,
		ShortID:          "0123456789abcdef",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"vless://11111111-1111-1111-1111-111111111111@0.0.0.0:443",
		"type=tcp", "security=reality", "flow=xtls-rprx-vision",
		"sid=0123456789abcdef", "sni=example.com", "fp=chrome", "#alice",
	} {
		if !strings.Contains(link, want) {
			t.Errorf("link missing %q: %s", want, link)
		}
	}
	// Round-trip through the strict importer.
	parsed, err := ParseConnectionURI(link)
	if err != nil {
		t.Fatalf("generated link does not parse back: %v", err)
	}
	if parsed.Protocol != ProtocolVLESS || parsed.Transport != TransportTCP ||
		parsed.Security != SecurityReality || parsed.PublicKey != s.RealityPublicKey ||
		parsed.ShortID != "0123456789abcdef" || parsed.Flow != FlowVision {
		t.Fatalf("round-trip mismatch: %+v", parsed.ParsedConnection)
	}
}

func TestBuildResolvedShareLink_VMessAndSS(t *testing.T) {
	vm := Server{
		ID: "55555555555555555555555555555555", Name: "vm-1",
		Protocol: ProtocolVMess, Listen: "203.0.113.10:8443", Port: 8443,
		Transport: TransportWeb, Security: SecurityNone, Enabled: true,
	}
	u := User{ID: "66666666666666666666666666666666", ServerID: vm.ID, Name: "bob",
		UUID: "22222222-2222-2222-2222-222222222222", Enabled: true}
	link, err := BuildResolvedShareLink(vm, u, LinkParams{Credential: u.UUID, SNI: "vpn.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "vmess://") {
		t.Fatalf("vmess link prefix: %s", link)
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["add"] != "203.0.113.10" || doc["net"] != "ws" || doc["tls"] != "" {
		t.Fatalf("vmess doc mismatch: %s", payload)
	}
	if p, ok := doc["port"].(float64); !ok || p != 8443 {
		t.Fatalf("vmess port mismatch: %s", payload)
	}
	parsed, err := ParseConnectionURI(link)
	if err != nil {
		t.Fatalf("vmess round-trip: %v", err)
	}
	if parsed.Protocol != ProtocolVMess || parsed.Host != "203.0.113.10" || parsed.Port != 8443 {
		t.Fatalf("vmess parse mismatch: %+v", parsed.ParsedConnection)
	}

	ss := Server{
		ID: "77777777777777777777777777777777", Name: "ss-1",
		Protocol: ProtocolShadowsocks, SSMethod: "2022-blake3-aes-256-gcm",
		Listen: "203.0.113.11:8388", Port: 8388,
		Transport: TransportTCP, Security: SecurityNone, Enabled: true,
	}
	link, err = BuildResolvedShareLink(ss, u, LinkParams{Credential: "PSKVALUE"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "ss://") {
		t.Fatalf("ss link prefix: %s", link)
	}
	parsed, err = ParseConnectionURI(link)
	if err != nil {
		t.Fatalf("ss round-trip: %v", err)
	}
	if parsed.Method != "2022-blake3-aes-256-gcm" || parsed.Credential != "PSKVALUE" ||
		parsed.Host != "203.0.113.11" || parsed.Port != 8388 {
		t.Fatalf("ss parse mismatch: %+v", parsed.ParsedConnection)
	}
}

func TestParseConnectionURI_RejectsUnsupported(t *testing.T) {
	cases := []string{
		"http://example.com/not-a-link",
		"vless://user@host:notaport?type=tcp",
		"vless://user@host:443?type=quic",
		"vless://user@host:443?type=tcp&security=reality&flow=bogus",
		"ss://rot13-method:pw@1.2.3.4:8388",
		"trojan://pw@host:443?type=tcp&security=tls&alpn=h2", // alpn lands in UnknownQuery, must not error
	}
	for i, raw := range cases {
		parsed, err := ParseConnectionURI(raw)
		if i == 5 {
			continue
		}
		if err == nil {
			t.Errorf("case %d accepted: %s -> %+v", i, raw, parsed.ParsedConnection)
		}
	}
	parsed, err := ParseConnectionURI("trojan://pw@host:443?type=tcp&security=tls&alpn=h2#lbl")
	if err != nil {
		t.Fatalf("unknown query params must be preserved, not rejected: %v", err)
	}
	if parsed.UnknownQuery["alpn"] != "h2" {
		t.Fatalf("alpn dropped: %+v", parsed.UnknownQuery)
	}
	if parsed.Name != "lbl" {
		t.Fatalf("label lost: %+v", parsed.ParsedConnection)
	}
}

func TestValidateCompatibility_Matrix(t *testing.T) {
	// REALITY over WebSocket is rejected.
	s := xcenterRealityServer()
	s.Transport = TransportWeb
	if err := s.Validate(); err == nil {
		t.Fatal("REALITY over ws accepted")
	}
	// REALITY over XHTTP is accepted (recommended modern pairing).
	s.Transport = TransportXHTTP
	if err := s.Validate(); err != nil {
		t.Fatalf("REALITY over xhttp rejected: %v", err)
	}
	// Flow on non-VLESS or non-REALITY pairing is rejected.
	s2 := tortureValidServerFixture()
	if err := validateUserCompatibility(s2.Protocol, s2.Security, s2.Transport,
		User{Flow: FlowVision}); err == nil {
		t.Fatal("flow without REALITY accepted")
	}
	// SS without a cipher is rejected.
	ss := tortureValidServerFixture()
	ss.Protocol = ProtocolShadowsocks
	if err := ss.Validate(); err == nil {
		t.Fatal("shadowsocks without method accepted")
	}
	// Only one transport block may be set.
	s3 := tortureValidServerFixture()
	s3.Transport = TransportWeb
	s3.TransportSettings = &TransportSettings{
		WS: &WSSettings{Path: "/w"}, GRPC: &GRPCSettings{ServiceName: "svc"},
	}
	if err := s3.Validate(); err == nil {
		t.Fatal("multiple transport blocks accepted")
	}
	// WS path must start with /.
	s4 := tortureValidServerFixture()
	s4.Transport = TransportWeb
	s4.TransportSettings = &TransportSettings{WS: &WSSettings{Path: "nope"}}
	if err := s4.Validate(); err == nil {
		t.Fatal("ws path without leading slash accepted")
	}
}

func TestRenderConfig_NewProtocolsAndTransports(t *testing.T) {
	vm := Server{
		ID: "55555555555555555555555555555555", Name: "vm-1",
		Protocol: ProtocolVMess, Listen: "127.0.0.1:8443", Port: 8443,
		Transport: TransportXHTTP, Security: SecurityNone, Enabled: true,
		TransportSettings: &TransportSettings{XHTTP: &XHTTPSettings{Path: "/xvm", Mode: "stream-one"}},
	}
	u := User{ID: "66666666666666666666666666666666", ServerID: vm.ID, Name: "bob",
		UUID: "22222222-2222-2222-2222-222222222222", Enabled: true}
	cfg, err := RenderConfig([]Server{vm}, []User{u}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 {
		t.Fatalf("inbound count %d", len(cfg.Inbounds))
	}
	var stream struct {
		Network string `json:"network"`
		XHTTP   struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		} `json:"xhttpSettings"`
	}
	if err := json.Unmarshal(cfg.Inbounds[0].StreamSettings, &stream); err != nil {
		t.Fatal(err)
	}
	if stream.Network != "xhttp" || stream.XHTTP.Path != "/xvm" || stream.XHTTP.Mode != "stream-one" {
		t.Fatalf("xhttp stream settings: %s", cfg.Inbounds[0].StreamSettings)
	}
	var settings struct {
		Clients []struct {
			ID      string `json:"id"`
			AlterID int    `json:"alterId"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(cfg.Inbounds[0].Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 1 || settings.Clients[0].ID != u.UUID || settings.Clients[0].AlterID != 0 {
		t.Fatalf("vmess clients: %s", cfg.Inbounds[0].Settings)
	}
}
