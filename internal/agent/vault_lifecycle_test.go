package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/vault"
)

func writeMasterKey(t *testing.T, dir string) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "master.key")
	if err := os.WriteFile(path, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenVaultRejectsBadInputs(t *testing.T) {
	if _, err := OpenVault(context.Background(), VaultDeps{}); err == nil {
		t.Fatal("empty deps were accepted")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(context.Background(), VaultDeps{Path: dir, MasterFile: dir + "/missing"}); err == nil {
		t.Fatal("missing master key was accepted")
	}
}

func TestOpenVaultRefusesWorldAccessibleMaster(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	if err := os.Chmod(master, 0o644); err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master}); err == nil {
		t.Fatal("world-readable master was accepted")
	}
}

func TestOpenVaultRejectsShortMaster(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := filepath.Join(dir, "master.key")
	if err := os.WriteFile(master, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master}); err == nil {
		t.Fatal("short master was accepted")
	}
}

func TestOpenVaultInitialisesAndLists(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lc.Close() })
	if lc.KeyID() == "" {
		t.Fatal("vault key id is empty")
	}
	refs, err := lc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatal("new vault must start empty")
	}
}

func TestVaultLifecyclePutAndResolve(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lc.Close() })
	ref, err := lc.Put("0123456789abcdef0123456789abcdef", "wireguard-private", []byte("this-is-a-test-secret-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := lc.Resolver()
	got, err := resolver.Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "this-is-a-test-secret-bytes" {
		t.Fatal("secret did not round-trip")
	}
}

func TestVaultLifecycleRevokeStopsResolving(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lc.Close() })
	ref, err := lc.Put("0123456789abcdef0123456789abcdef", "wireguard-private", []byte("this-is-a-test-secret-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.Revoke(ref.ID, ref.Revision, "operator", "rotation"); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.Resolver().Resolve(context.Background(), ref); err == nil {
		t.Fatal("revoked reference resolved")
	}
}

func TestVaultLifecycleCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lc.Close(); err != nil {
		t.Fatal("second close returned error", err)
	}
	if _, err := lc.List(); err == nil {
		t.Fatal("closed lifecycle still answered List")
	}
}

func TestVaultLifecycleReopenIsStable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc1, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	ref, err := lc1.Put("0123456789abcdef0123456789abcdef", "wireguard-private", []byte("stable-secret-bytes-32-aaaa"))
	if err != nil {
		t.Fatal(err)
	}
	if err := lc1.Close(); err != nil {
		t.Fatal(err)
	}
	lc2, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lc2.Close() })
	got, err := lc2.Resolver().Resolve(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stable-secret-bytes-32-aaaa" {
		t.Fatal("secret did not survive reopen")
	}
	if lc1.KeyID() != lc2.KeyID() {
		t.Fatal("key id changed across opens")
	}
}

func TestVaultResolverRejectsClosedLifecycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	master := writeMasterKey(t, dir)
	vaultDir := filepath.Join(dir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lc, err := OpenVault(context.Background(), VaultDeps{Path: vaultDir, MasterFile: master})
	if err != nil {
		t.Fatal(err)
	}
	if err := lc.Close(); err != nil {
		t.Fatal(err)
	}
	ref := vault.Reference{ID: "0123456789abcdef0123456789abcdef", Revision: "ffffffffffffffffffffffffffffffff", KeyID: lc.KeyID(), Purpose: "wireguard-private"}
	if _, err := lc.Resolver().Resolve(context.Background(), ref); err == nil {
		t.Fatal("closed lifecycle resolved a reference")
	}
}

var _ = errors.New
