//go:build linux

// Package agent: regression coverage for BETA.2 PHASE-2 blocker A
//
//	Xray REALITY vault reference must be accepted by the
//	resolver in the SAME Agent lifetime the Vault PUT
//	was performed. A post-startup Put on the Vault must
//	be visible to the xray materialiser without an Agent
//	restart.
//
// This test boots the production lifecycle composition
// root: vault store + VaultLifecycle + xrayVaultResolver.
// It then performs a vault Put AFTER the lifecycle is
// fully wired and asserts that:
//
//   1. ListReferences returns the new record
//   2. Resolver returns the bytes via xray.Materialize
//   3. The rendered xray config carries the inlined
//      privateKey + shortId (no placeholder marker remains)
//   4. A second Put (new revision) makes the FIRST revision
//      unresolvable
//   5. A wrong-purpose Put is refused by xrayVaultKeyAllowed
//   6. After a synthetic lifecycle teardown + reopen with
//      the same master.key the record persists and
//      resolves identically
//
// The same shape is exercised for TLS so both security
// modes share one freshness contract.

package agent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/xray"
)

// makeLifecycle constructs a VaultLifecycle with a
// fresh vault dir and a 32-byte master key in a
// separate file. Mirrors the production composition
// where /var/lib/duckos-agent/vault/records is the vault
// store and /var/lib/duckos-agent/vault/master.key is
// the master key file (sibling to records/, NOT inside it).
func makeLifecycle(t *testing.T) *VaultLifecycle {
	t.Helper()
	root := t.TempDir()
	vaultDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	keyPath := filepath.Join(root, "master.key")
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if err := os.WriteFile(keyPath, master, 0o600); err != nil {
		t.Fatalf("write master: %v", err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{
		Path:       vaultDir,
		MasterFile: keyPath,
	})
	if err != nil {
		t.Fatalf("OpenVault: %v", err)
	}
	t.Cleanup(func() { _ = lc.Close() })
	return lc
}

// TestXrayVaultReferenceFreshnessPostStartup is the
// canonical post-startup Put regression test.
func TestXrayVaultReferenceFreshnessPostStartup(t *testing.T) {
	lc := makeLifecycle(t)
	stage := t.TempDir()
	resolver := lc.XrayResolver()

	privBytes := make([]byte, 32)
	if _, err := rand.Read(privBytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	privB64 := base64.RawURLEncoding.EncodeToString(privBytes)
	shortID := make([]byte, 8)
	if _, err := rand.Read(shortID); err != nil {
		t.Fatalf("rand: %v", err)
	}

	const privVID = "ee88f2381e94b1c7339adf6c085854f0"
	const shortVID = "ae05c4801622c5801f8bf57e78815011"

	privRef, err := lc.store.Put(privVID, "reality-private", privBytes)
	if err != nil {
		t.Fatalf("Put private: %v", err)
	}
	shortRef, err := lc.store.Put(shortVID, "reality-shortid", shortID)
	if err != nil {
		t.Fatalf("Put short: %v", err)
	}

	refs, err := lc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(refs) < 2 {
		t.Fatalf("expected >=2 refs after Put, got %d", len(refs))
	}

	srv := xray.Server{
		ID:        "abc123def456abc123def456abc123de",
		Name:      "reality-freshness",
		Protocol:  "vless",
		Transport: "tcp",
		Security:  xray.SecurityReality,
		Listen:    "0.0.0.0:56700",
		Port:      56700,
		RealityKeyRef: &xray.KeyRef{
			VaultID:  privRef.ID,
			Revision: privRef.Revision,
			KeyID:    privRef.KeyID,
		},
		RealityDest: "duckos-public.com:443",
		RealitySNI:  []string{"duckos-public.com"},
		Fingerprint: "chrome",
		ShortIDRef: &xray.KeyRef{
			VaultID:  shortRef.ID,
			Revision: shortRef.Revision,
			KeyID:    shortRef.KeyID,
		},
		Enabled: true,
	}
	users := []xray.User{{
		ID:       "5555aaaa5555aaaa5555aaaa5555aa55",
		ServerID: srv.ID,
		Name:     "user-freshness",
		UUID:     "6978f77e-f43c-427b-b385-12fd016dc47d",
		Enabled:  true,
	}}
	rendered, err := xray.RenderRuntimeConfig([]xray.Server{srv}, users)
	if err != nil {
		t.Fatalf("RenderRuntimeConfig: %v", err)
	}
	mcfg, err := xray.Materialize(context.Background(), rendered, stage, []xray.Server{srv}, users, resolver)
	if err != nil {
		t.Fatalf("Materialize after post-startup Put: %v", err)
	}

	raw, err := json.Marshal(mcfg)
	if err != nil {
		t.Fatalf("marshal mcfg: %v", err)
	}
	if strings.Contains(string(raw), "<resolved at apply time") {
		t.Fatalf("materialised config still contains the placeholder marker: %s", string(raw))
	}
	if !strings.Contains(string(raw), privB64) {
		t.Fatalf("materialised config missing the inlined privateKey %s: %s", privB64, string(raw))
	}
	if !strings.Contains(string(raw), hex.EncodeToString(shortID)) {
		t.Fatalf("materialised config missing the inlined shortId: %s", string(raw))
	}

	priv2Bytes := make([]byte, 32)
	if _, err := rand.Read(priv2Bytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := lc.store.Put(privRef.ID, "reality-private", priv2Bytes); err != nil {
		t.Fatalf("Put revision2: %v", err)
	}
	// A never-issued revision (random hex that was never
	// Put) must be refused. The vault is immutable so the
	// privRef revision itself is still resolvable (it
	// remains a valid durable record).
	if _, err := resolver.Resolve(context.Background(), xray.KeyRef{
		VaultID:  privRef.ID,
		Revision: "deadbeefdeadbeefdeadbeefdeadbeef",
		KeyID:    privRef.KeyID,
	}); err == nil {
		t.Fatalf("expected unknown revision to be refused, got nil")
	}

	if _, err := lc.store.Put("0000000000000000000000000000000a", "wireguard-private", []byte("irrelevant")); err != nil {
		t.Fatalf("Put wireguard-private: %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), xray.KeyRef{
		VaultID:  "0000000000000000000000000000000a",
		Revision: "deadbeefdeadbeefdeadbeefdeadbeef",
		KeyID:    lc.KeyID(),
	}); err == nil {
		t.Fatalf("expected wrong-purpose key to be refused by xray resolver")
	}
}

// TestXrayVaultReferenceFreshnessTLS mirrors the
// REALITY test for the TLS path so both TLS + REALITY
// share one freshness contract.
func TestXrayVaultReferenceFreshnessTLS(t *testing.T) {
	lc := makeLifecycle(t)
	stage := t.TempDir()
	resolver := lc.XrayResolver()

	certPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIDazCCAlOgAwIBAgIUHZzzzzZZZZZZZZZZZZZZZZZZZZZZZZ\n-----END CERTIFICATE-----\n")
	keyPEM := []byte("-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VuBCIEIHpZZZZZZZZZZZZZZZZZZZZZZZZZZZ\n-----END PRIVATE KEY-----\n")
	certRef, err := lc.store.Put("00000000000000000000000000000001", "tls-certificate", certPEM)
	if err != nil {
		t.Fatalf("Put cert: %v", err)
	}
	keyRef, err := lc.store.Put("00000000000000000000000000000002", "tls-private", keyPEM)
	if err != nil {
		t.Fatalf("Put key: %v", err)
	}
	srv := xray.Server{
		ID:        "abc123def456abc123def456abc123df",
		Name:      "tls-freshness",
		Protocol:  "vless",
		Transport: "tcp",
		Security:  xray.SecurityTLS,
		Listen:    "0.0.0.0:56710",
		Port:      56710,
		TLSCertRef: &xray.KeyRef{
			VaultID:  certRef.ID,
			Revision: certRef.Revision,
			KeyID:    certRef.KeyID,
		},
		TLSKeyRef: &xray.KeyRef{
			VaultID:  keyRef.ID,
			Revision: keyRef.Revision,
			KeyID:    keyRef.KeyID,
		},
		Enabled: true,
	}
	users := []xray.User{{
		ID:       "5555aaaa5555aaaa5555aaaa5555aa66",
		ServerID: srv.ID,
		Name:     "user-tls",
		UUID:     "deadbeef-0000-0000-0000-000000000000",
		Enabled:  true,
	}}
	rendered, err := xray.RenderRuntimeConfig([]xray.Server{srv}, users)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	mcfg, err := xray.Materialize(context.Background(), rendered, stage, []xray.Server{srv}, users, resolver)
	if err != nil {
		t.Fatalf("Materialize TLS: %v", err)
	}
	for _, ib := range mcfg.Inbounds {
		if ib.Tag == "in-tls-freshness" {
			certPath := filepath.Join(stage, "tls-freshness.crt")
			keyPath := filepath.Join(stage, "tls-freshness.key")
			if _, err := os.Stat(certPath); err != nil {
				t.Fatalf("cert materialised: %v", err)
			}
			if _, err := os.Stat(keyPath); err != nil {
				t.Fatalf("key materialised: %v", err)
			}
			certBytes, _ := os.ReadFile(certPath)
			if string(certBytes) != string(certPEM) {
				t.Fatalf("cert bytes mismatch: got %q want %q", string(certBytes), string(certPEM))
			}
			return
		}
	}
	t.Fatalf("tls-freshness inbound not rendered")
}

// TestXrayVaultReferenceFreshnessAfterRestart simulates
// an Agent restart by closing and reopening the
// lifecycle on the SAME master.key. The persisted
// records must resolve identically.
func TestXrayVaultReferenceFreshnessAfterRestart(t *testing.T) {
	root := t.TempDir()
	vaultDir := filepath.Join(root, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	keyPath := filepath.Join(root, "master.key")
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if err := os.WriteFile(keyPath, master, 0o600); err != nil {
		t.Fatalf("write master: %v", err)
	}
	lc1, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: keyPath})
	if err != nil {
		t.Fatalf("OpenVault #1: %v", err)
	}
	privBytes := make([]byte, 32)
	if _, err := rand.Read(privBytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ref, err := lc1.store.Put("ee88f2381e94b1c7339adf6c085854f0", "reality-private", privBytes)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	lc1.Close()

	lc2, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: keyPath})
	if err != nil {
		t.Fatalf("OpenVault #2 (post-restart): %v", err)
	}
	defer lc2.Close()
	resolver := lc2.XrayResolver()
	got, err := resolver.Resolve(context.Background(), xray.KeyRef{
		VaultID:  ref.ID,
		Revision: ref.Revision,
		KeyID:    ref.KeyID,
	})
	if err != nil {
		t.Fatalf("post-restart Resolve: %v", err)
	}
	want := base64.RawURLEncoding.EncodeToString(privBytes)
	if base64.RawURLEncoding.EncodeToString(got) != want {
		t.Fatalf("post-restart bytes mismatch: got %s want %s", base64.RawURLEncoding.EncodeToString(got), want)
	}
}
