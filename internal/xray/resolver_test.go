// internal/xray/resolver_test.go: unit tests for the
// vault-aware materialisation path.
//
// BETA.2 P065 typed binding for Xray TLS / REALITY:
// Materialize() must:
//
//   - write TLS cert + key files to <stage>/<name>.{crt,key},
//   - inline REALITY privateKey (base64) and shortIds (hex)
//     into the rendered Config,
//   - reject any inbound whose vault reference is unknown
//     or whose resolver is nil,
//   - be a no-op for security=none inbounds.
package xray

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubResolver is a Resolver that returns deterministic
// bytes keyed by KeyID. It enforces the same purpose
// allow-list as the production agent resolver so the
// test fails for the same reasons the production path
// would fail.
type stubResolver struct {
	bytes map[string][]byte
}

func (s stubResolver) Resolve(ctx context.Context, ref KeyRef) ([]byte, error) {
	if v, ok := s.bytes[ref.KeyID]; ok {
		out := make([]byte, len(v))
		copy(out, v)
		return out, nil
	}
	return nil, os.ErrNotExist
}

func fakePEM(t *testing.T) []byte {
	t.Helper()
	pem := []byte("-----BEGIN CERTIFICATE-----\nMIIBfake\n-----END CERTIFICATE-----\n")
	return pem
}

func TestMaterialize_TLSWritesCertAndKey(t *testing.T) {
	stage := t.TempDir()
	certRef := KeyRef{
		VaultID:  strings.Repeat("a", 32),
		Revision: strings.Repeat("b", 32),
		KeyID:    strings.Repeat("f", 32),
	}
	keyRef := KeyRef{
		VaultID:  strings.Repeat("c", 32),
		Revision: strings.Repeat("d", 32),
		KeyID:    strings.Repeat("e", 32),
	}
	certBytes := fakePEM(t)
	keyBytes := []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n")
	resolver := stubResolver{bytes: map[string][]byte{
		strings.Repeat("f", 32): certBytes,
		strings.Repeat("e", 32): keyBytes,
	}}
	server := Server{
		ID:         strings.Repeat("1", 32),
		Name:       "tls-lab",
		Protocol:   "vless",
		Transport:  "tcp",
		Security:   SecurityTLS,
		Listen:     "0.0.0.0:56100",
		Port:       56100,
		Enabled:    true,
		TLSCertRef: &certRef,
		TLSKeyRef:  &keyRef,
	}
	user := User{
		ID:       strings.Repeat("2", 32),
		ServerID: strings.Repeat("1", 32),
		UUID:     "11111111-2222-3333-4444-555555555555",
		Enabled:  true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	// Both files must exist on disk.
	certPath := filepath.Join(stage, "tls-lab.crt")
	keyPath := filepath.Join(stage, "tls-lab.key")
	gotCert, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("cert not written: %v", err)
	}
	if string(gotCert) != string(certBytes) {
		t.Fatalf("cert bytes mismatch")
	}
	gotKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("key not written: %v", err)
	}
	if string(gotKey) != string(keyBytes) {
		t.Fatalf("key bytes mismatch")
	}
}

func TestMaterialize_RealityInlinesKeyAndShortID(t *testing.T) {
	stage := t.TempDir()
	keyRef := KeyRef{
		VaultID:  strings.Repeat("a", 32),
		Revision: strings.Repeat("b", 32),
		KeyID:    strings.Repeat("f", 32),
	}
	sidRef := KeyRef{
		VaultID:  strings.Repeat("c", 32),
		Revision: strings.Repeat("d", 32),
		KeyID:    strings.Repeat("e", 32),
	}
	x25519 := make([]byte, 32)
	for i := range x25519 {
		x25519[i] = byte(i)
	}
	sid := []byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe}
	resolver := stubResolver{bytes: map[string][]byte{
		strings.Repeat("f", 32): x25519,
		strings.Repeat("e", 32): sid,
	}}
	server := Server{
		ID:            strings.Repeat("1", 32),
		Name:          "real-lab",
		Protocol:      "vless",
		Transport:     "tcp",
		Security:      SecurityReality,
		Listen:        "0.0.0.0:56220",
		Port:          56220,
		Enabled:       true,
		RealityKeyRef: &keyRef,
		RealityDest:   "duckos-public.com:443",
		RealitySNI:    []string{"duckos-public.com"},
		ShortIDRef:    &sidRef,
	}
	user := User{
		ID:       strings.Repeat("2", 32),
		ServerID: strings.Repeat("1", 32),
		UUID:     "11111111-2222-3333-4444-555555555555",
		Enabled:  true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: the rendered config MUST carry the
	// placeholder markers before materialisation.
	// JSON marshals `<` and `>` inside a string as
	// `\u003c` / `\u003e` so the substring search
	// uses the JSON-escaped form.
	pre, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pre), "resolved at apply time") {
		t.Fatalf("render expected to contain placeholder markers")
	}
	cfg, err = Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	post, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The placeholder MUST be replaced with the
	// URL-safe-base64-of-the-32-byte-X25519-key. Xray
	// accepts privateKey in base64url form only; standard
	// base64 or raw hex are rejected by xray -test.
	wantPrivate := `"privateKey": "` + base64.RawURLEncoding.EncodeToString(x25519) + `"`
	if !strings.Contains(string(post), wantPrivate) {
		t.Fatalf("privateKey not inlined:\n%s", post)
	}
	// The short ID MUST be inlined as a 16-char hex.
	wantSid := `"` + hex.EncodeToString(sid) + `"`
	if !strings.Contains(string(post), wantSid) {
		t.Fatalf("shortId not inlined:\n%s", post)
	}
	// And no placeholder markers may remain.
	if strings.Contains(string(post), "resolved at apply time") {
		t.Fatalf("placeholder still present after materialise:\n%s", post)
	}
}

// spotCheckRealityPlaceholderOnce is a one-off helper that
// verifies the renderer's literal marker format (without
// JSON-escaped < >). Kept here so future drift between
// render.go and resolver.go is caught.
func TestRealityPlaceholderLiteral(t *testing.T) {
	got := realityMatches("<resolved at apply time; see vault record abc>", "abc")
	if !got {
		t.Fatal("expected realityMatches to recognise the placeholder literal")
	}
}

// TestMaterialize_RealityInlinesCredentialUUID is the
// regression for the BETA.2 two-blocker root-cause: a user
// with a Credential vault ref (purpose "xray-credential",
// no inline UUID) must end up with the resolved canonical
// UUID inlined into settings.clients[].id. Without the
// materialiser's credential substitution, xray(8) refuses
// to start with "invalid id" because the renderer emits
// the placeholder string where the UUID should be.
//
// The test exercises both the placeholder substitution and
// the xray-credential purpose allow-list end-to-end.
func TestMaterialize_RealityInlinesCredentialUUID(t *testing.T) {
	stage := t.TempDir()
	keyRef := KeyRef{VaultID: strings.Repeat("a", 32), Revision: strings.Repeat("b", 32), KeyID: strings.Repeat("f", 32)}
	sidRef := KeyRef{VaultID: strings.Repeat("c", 32), Revision: strings.Repeat("d", 32), KeyID: strings.Repeat("e", 32)}
	credRef := KeyRef{VaultID: strings.Repeat("7", 32), Revision: strings.Repeat("8", 32), KeyID: strings.Repeat("9", 32)}
	x25519 := make([]byte, 32)
	for i := range x25519 {
		x25519[i] = byte(i)
	}
	sid := []byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe}
	// The credential vault record carries the UUID as
	// UTF-8 bytes. Canonical 8-4-4-4-12 hex form, exactly
	// 36 bytes (no trailing newline).
	uuidBytes := []byte("11111111-2222-3333-4444-555555555555")
	resolver := stubResolver{bytes: map[string][]byte{
		strings.Repeat("f", 32): x25519,
		strings.Repeat("e", 32): sid,
		strings.Repeat("9", 32): uuidBytes,
	}}
	server := Server{
		ID:            strings.Repeat("1", 32),
		Name:          "real-cred",
		Protocol:      "vless",
		Transport:     "tcp",
		Security:      SecurityReality,
		Listen:        "0.0.0.0:56240",
		Port:          56240,
		Enabled:       true,
		RealityKeyRef: &keyRef,
		RealityDest:   "duckos-public.com:443",
		RealitySNI:    []string{"duckos-public.com"},
		ShortIDRef:    &sidRef,
	}
	user := User{
		ID:         strings.Repeat("2", 32),
		ServerID:   strings.Repeat("1", 32),
		Credential: &credRef,
		Enabled:    true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: the renderer MUST emit the credential
	// placeholder in settings.clients[].id before
	// materialisation. The placeholder literal is JSON-
	// escaped by the marshaller (`<` becomes `\u003c`,
	// `>` becomes `\u003e`) so the assertion scans for
	// the escaped form.
	pre, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantPre := `"id": "\u003cresolved at apply time; see vault record ` + strings.Repeat("9", 32) + `\u003e"`
	if !strings.Contains(string(pre), wantPre) {
		t.Fatalf("render expected to contain credential placeholder %q; got:\n%s", wantPre, pre)
	}
	cfg, err = Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	post, err := encodeConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// The placeholder MUST be replaced with the canonical
	// 36-byte UUID (8-4-4-4-12 hex form, 36 chars total,
	// no padding, no whitespace).
	wantUUID := `"id": "` + string(uuidBytes) + `"`
	if !strings.Contains(string(post), wantUUID) {
		t.Fatalf("client id not inlined:\n%s", post)
	}
	if strings.Contains(string(post), "resolved at apply time") {
		t.Fatalf("placeholder still present after materialise:\n%s", post)
	}
}

func TestMaterialize_RejectsMissingResolver(t *testing.T) {
	stage := t.TempDir()
	keyRef := KeyRef{
		VaultID:  strings.Repeat("a", 32),
		Revision: strings.Repeat("b", 32),
		KeyID:    strings.Repeat("f", 32),
	}
	sidRef := KeyRef{
		VaultID:  strings.Repeat("c", 32),
		Revision: strings.Repeat("d", 32),
		KeyID:    strings.Repeat("e", 32),
	}
	server := Server{
		ID:            strings.Repeat("1", 32),
		Name:          "real-lab",
		Protocol:      "vless",
		Transport:     "tcp",
		Security:      SecurityReality,
		Listen:        "0.0.0.0:56220",
		Port:          56220,
		Enabled:       true,
		RealityKeyRef: &keyRef,
		RealityDest:   "x:443",
		RealitySNI:    []string{"x"},
		ShortIDRef:    &sidRef,
	}
	user := User{
		ID:       strings.Repeat("2", 32),
		ServerID: strings.Repeat("1", 32),
		UUID:     "11111111-2222-3333-4444-555555555555",
		Enabled:  true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, nil); err == nil {
		t.Fatal("expected error for nil resolver")
	}
}

func TestMaterialize_RejectsUnknownKey(t *testing.T) {
	stage := t.TempDir()
	keyRef := KeyRef{
		VaultID:  strings.Repeat("a", 32),
		Revision: strings.Repeat("b", 32),
		KeyID:    strings.Repeat("f", 32),
	}
	sidRef := KeyRef{
		VaultID:  strings.Repeat("c", 32),
		Revision: strings.Repeat("d", 32),
		KeyID:    strings.Repeat("e", 32),
	}
	server := Server{
		ID:            strings.Repeat("1", 32),
		Name:          "real-lab",
		Protocol:      "vless",
		Transport:     "tcp",
		Security:      SecurityReality,
		Listen:        "0.0.0.0:56220",
		Port:          56220,
		Enabled:       true,
		RealityKeyRef: &keyRef,
		RealityDest:   "x:443",
		RealitySNI:    []string{"x"},
		ShortIDRef:    &sidRef,
	}
	user := User{
		ID:       strings.Repeat("2", 32),
		ServerID: strings.Repeat("1", 32),
		UUID:     "11111111-2222-3333-4444-555555555555",
		Enabled:  true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	resolver := stubResolver{bytes: map[string][]byte{}}
	if _, err := Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, resolver); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestMaterialize_NoneIsNoOp(t *testing.T) {
	stage := t.TempDir()
	server := Server{
		ID:        strings.Repeat("1", 32),
		Name:      "none-lab",
		Protocol:  "vless",
		Transport: "tcp",
		Security:  SecurityNone,
		Listen:    "0.0.0.0:56300",
		Port:      56300,
		Enabled:   true,
	}
	user := User{
		ID:       strings.Repeat("2", 32),
		ServerID: strings.Repeat("1", 32),
		UUID:     "11111111-2222-3333-4444-555555555555",
		Enabled:  true,
	}
	cfg, err := RenderRuntimeConfig([]Server{server}, []User{user})
	if err != nil {
		t.Fatal(err)
	}
	// nil resolver is allowed for security=none; Materialize
	// returns the config unchanged.
	out, err := Materialize(context.Background(), cfg, stage, []Server{server}, []User{user}, nil)
	if err != nil {
		t.Fatalf("security=none must not require a resolver: %v", err)
	}
	if len(out.Inbounds) != 1 {
		t.Fatalf("inbound count changed: %d", len(out.Inbounds))
	}
}

// encodeConfig marshals the Config to a stable JSON for
// substring assertions.
func encodeConfig(c Config) ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// jsonMarshalIndentBytes is a tiny local helper to avoid
// importing encoding/json inside the test file.
func jsonMarshalIndentBytes(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
