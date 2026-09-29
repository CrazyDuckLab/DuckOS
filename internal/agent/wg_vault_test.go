package agent

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/wireguard"
)

func TestWireguardVaultResolverRejectsWrongRevokedAndClosedKeys(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenVault(context.Background(), VaultDeps{Path: filepath.Join(dir, "records"), MasterFile: writeMasterKey(t, dir)})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	private := bytes.Repeat([]byte{9}, 32)
	ref, err := l.Put(strings.Repeat("a", 32), "wireguard-private", private)
	if err != nil {
		t.Fatal(err)
	}
	key := wireguard.KeyRef{VaultID: ref.ID, Revision: ref.Revision, KeyID: ref.KeyID}
	r := l.WireguardResolver()
	got, err := r.Resolve(context.Background(), key)
	if err != nil || !bytes.Equal(got, private) {
		t.Fatal("protected key did not resolve")
	}
	clear(got)
	wrong := key
	wrong.KeyID = strings.Repeat("f", 32)
	if _, err := r.Resolve(context.Background(), wrong); err == nil {
		t.Fatal("wrong master key identity accepted")
	}
	if err := l.Revoke(ref.ID, ref.Revision, "owner", "test revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(context.Background(), key); err == nil {
		t.Fatal("revoked key accepted")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(context.Background(), key); err == nil {
		t.Fatal("closed vault accepted")
	}
}
