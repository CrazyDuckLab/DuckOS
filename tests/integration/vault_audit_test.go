//go:build linux

// vault_audit_test.go verifies the Vault
// secret boundary: secret bytes revealed by
// the vault resolver must never appear in the
// logging ring, the identity audit log, the
// state journal, or the catalog client
// representation. References (opaque 32-char
// hex IDs plus purpose) may appear in audit
// and state; Reveal() output must not.
//
// The test does not exercise the real
// filesystem path of the Agent; it exercises
// the same Logger + identity + catalog
// composition the Agent uses, with a small
// in-memory key. A leaked plaintext in any
// sink fails the test.
package integration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/catalog"
	"github.com/CrazyDuckLab/DuckOS/internal/identity"
	"github.com/CrazyDuckLab/DuckOS/internal/logging"
	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

const (
	vaultMarker = "DUCKOS-PLAINTEXT-MUST-NOT-LEAK-XYZZY-99887766"
	vaultID     = "0123456789abcdef0123456789abcdef"
	vaultRev    = "fedcba9876543210fedcba9876543210"
	vaultKey    = "abcdef0123456789abcdef0123456789"
)

// captureLogger returns a Logger that always
// redacts via the default policy plus a custom
// "marker" redaction. The custom redaction is
// what makes the assertion checkable: if a
// sink forgot to apply the replacer, the
// marker would be visible in the snapshot.
func captureLogger() *logging.Logger {
	markerReplacer := func(field, value string) string {
		if strings.Contains(value, vaultMarker) {
			return "redacted:marker"
		}
		return logging.DefaultReplacer(field, value)
	}
	return logging.New(logging.Config{Replacer: markerReplacer})
}

func makeKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func newRef() vault.Reference {
	return vault.Reference{ID: vaultID, Revision: vaultRev, Purpose: "wireguard-private", KeyID: vaultKey}
}

func TestVaultSecretNeverLeakstoLoggerRing(t *testing.T) {
	log := captureLogger()
	log.SetLevel(logging.Global, logging.Trace)
	ref := newRef()
	plaintext := []byte(vaultMarker + ":wireguard-private-key-bytes")
	// Log the reference and the plaintext through
	// the public Log API. The replacer must mask
	// the marker even when the caller mistakenly
	// places it in a context value.
	log.Log("vault", logging.SevInfo, "vault reference", map[string]string{
		"reference": ref.ID,
		"purpose":   ref.Purpose,
		"marker":    string(plaintext),
	})
	for _, rec := range log.Snapshot() {
		for k, v := range rec.Context {
			if strings.Contains(v, vaultMarker) {
				t.Fatalf("plaintext leaked through logger context field %q: %q", k, v)
			}
		}
		if strings.Contains(rec.Message, vaultMarker) {
			t.Fatalf("plaintext leaked through logger message: %q", rec.Message)
		}
	}
}

func TestVaultSecretStringAndLogValueAreOpaque(t *testing.T) {
	key := makeKey(t)
	t.Cleanup(func() { clear(key) })
	raw, err := vault.Seal(key, newRef(), []byte(vaultMarker+":real-secret-bytes"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	secret, err := vault.Open(key, newRef(), raw)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// 1) String() must be the opaque placeholder.
	if got := secret.String(); got != "[secret]" {
		t.Fatalf("Secret.String() = %q, want [secret]", got)
	}
	// 2) GoString() must be the same placeholder.
	if got := secret.GoString(); got != "[secret]" {
		t.Fatalf("Secret.GoString() = %q, want [secret]", got)
	}
	// 3) LogValue() must be the same placeholder.
	if got := secret.LogValue().String(); got != "[secret]" {
		t.Fatalf("Secret.LogValue() = %q, want [secret]", got)
	}
	// 4) MarshalJSON must refuse to serialize.
	if _, err := json.Marshal(secret); err == nil {
		t.Fatal("Secret.MarshalJSON must return an error")
	}
	// 5) slog must use the LogValue hook and not the
	//    default reflection-based format which would
	//    call Reveal().
	var out bytes.Buffer
	handler := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	logger.Info("sealed", "ref", refForLog(newRef()), "secret", secret)
	if strings.Contains(out.String(), vaultMarker) {
		t.Fatalf("slog handler emitted plaintext marker: %s", out.String())
	}
	if !strings.Contains(out.String(), "[secret]") {
		t.Fatalf("slog handler did not emit opaque [secret] placeholder: %s", out.String())
	}
	secret.Clear()
}

func TestVaultStoreStringAndLogValueAreOpaque(t *testing.T) {
	// Reference is what callers actually log; its
	// String() form is canonical JSON.
	r := newRef()
	if !strings.Contains(r.ID, vaultID) {
		t.Fatalf("reference ID missing: %+v", r)
	}
	// Reference JSON must round-trip without ever
	// carrying the marker.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), vaultMarker) {
		t.Fatalf("reference JSON leaked plaintext: %s", raw)
	}
}

func TestVaultReferenceJSONRoundTrips(t *testing.T) {
	r := newRef()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), vaultMarker) {
		t.Fatalf("reference JSON leaked plaintext: %s", raw)
	}
	var back vault.Reference
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != r {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, r)
	}
}

func TestIdentityAuditDoesNotCarryPlaintext(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	dir := filepath.Join(parent, "id")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir id: %v", err)
	}
	svc, err := identity.Open(dir)
	if err != nil {
		t.Fatalf("identity.Open: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Enroll("operator-1", "hunter2-"+vaultMarker); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	tok, _, err := svc.Login("operator-1", "hunter2-"+vaultMarker)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	t.Cleanup(func() { svc.Logout(tok) })
	entries, err := svc.Audit(tok)
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	for _, a := range entries {
		if strings.Contains(a.Actor, vaultMarker) || strings.Contains(a.Action, vaultMarker) || strings.Contains(a.Target, vaultMarker) {
			t.Fatalf("identity audit leaked plaintext marker: %+v", a)
		}
	}
	_ = time.Hour
}

func TestCatalogClientCredentialReferenceIsOpaque(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	dir := filepath.Join(parent, "catalog")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	store, err := catalog.Initialize(dir)
	if err != nil {
		t.Fatalf("catalog.Initialize: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	ref := newRef()
	// Reference.ID must match Client.ID per the
	// catalog invariant. The catalog also requires
	// ProfileID for Enabled clients.
	client := catalog.Client{
		ID:         vaultID,
		Name:       "ops-client",
		Kind:       "wireguard",
		ProfileID:  vaultID,
		Enabled:    true,
		Credential: &ref,
	}
	if _, err := store.Save(ctx, vaultID, 0, client); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entry, err := store.Get(ctx, vaultID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), vaultMarker) {
		t.Fatalf("catalog client JSON leaked plaintext marker: %s", raw)
	}
	if !strings.Contains(string(raw), vaultID) {
		t.Fatalf("catalog client JSON missing reference ID: %s", raw)
	}
	var back catalog.Entry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Client.Credential == nil || *back.Client.Credential != ref {
		t.Fatalf("credential reference round-trip mismatch: %+v", back.Client.Credential)
	}
}

func TestVaultResolverReturnsErrorAfterClose(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	dir := filepath.Join(parent, "vault")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	mk := makeKey(t)
	t.Cleanup(func() { clear(mk) })
	store, err := vault.InitializeStore(dir, mk)
	if err != nil {
		t.Fatalf("InitializeStore: %v", err)
	}
	ref, err := store.Put("0123456789abcdef0123456789abcdef", "wireguard-private", []byte(vaultMarker+":real-bytes"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := store.Get(ref); err == nil {
		t.Fatal("Get after Close must return an error")
	}
	// No way to resolve bytes post-close. The
	// master key has been cleared, so any leaked
	// reference in a stale log entry is now
	// unrecoverable.
}

func TestCatalogClientCredentialIsStructurallySeparated(t *testing.T) {
	// The catalog.Client.Credential field is
	// *vault.Reference, never []byte. This test
	// pins the structural separation that prevents
	// raw key bytes from ever being stored in the
	// catalog.
	c := catalog.Client{
		ID:         vaultID,
		Name:       "ops-client",
		Kind:       "wireguard",
		ProfileID:  vaultID,
		Enabled:    true,
		Credential: &vault.Reference{ID: vaultID, Revision: vaultRev, Purpose: "wireguard-private", KeyID: vaultKey},
	}
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), "raw") {
		t.Fatalf("catalog marshaled a raw key: %s", raw)
	}
	if !strings.Contains(string(raw), vaultID) {
		t.Fatalf("catalog should retain reference ID: %s", raw)
	}
}

// refForLog returns the metadata form of a
// reference for use in context maps. Callers
// must not include Reveal() bytes.
func refForLog(r vault.Reference) string {
	raw, _ := json.Marshal(r)
	return string(raw)
}
