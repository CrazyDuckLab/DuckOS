package xray

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
)

func TestReacceptCatalogPreRenameFaults(t *testing.T) {
	for _, stage := range []string{"write", "file-sync", "close", "rename"} {
		for name, mutate := range reacceptMutators() {
			t.Run(stage+"/"+name, func(t *testing.T) {
				c, err := OpenFileCatalog(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				if err := c.Restore(t.Context(), reacceptState()); err != nil {
					t.Fatal(err)
				}
				before, _ := c.Snapshot(t.Context())
				raw, _ := os.ReadFile(c.path)
				c.fault = func(at string) error {
					if at == stage {
						return errors.New("injected storage fault")
					}
					return nil
				}
				if err := mutate(c); err == nil || errors.Is(err, persist.ErrUncertain) {
					t.Fatalf("wrong outcome: %v", err)
				}
				after, err := c.Snapshot(t.Context())
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatal("pre-rename failure changed mirror")
				}
				disk, _ := os.ReadFile(c.path)
				if !bytes.Equal(raw, disk) {
					t.Fatal("pre-rename failure changed disk")
				}
				c.fault = nil
				if err := mutate(c); err != nil {
					t.Fatal("known failed outcome prevented retry", err)
				}
			})
		}
	}
}
func TestReacceptCatalogUncertainDeletionFailsClosedWithoutResurrection(t *testing.T) {
	c, err := OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(t.Context(), reacceptState()); err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(c.path)
	c.fault = func(stage string) error {
		if stage == "directory-sync" {
			return errors.New("injected directory sync fault")
		}
		return nil
	}
	stopCalls, applyCalls := 0, 0
	api := API{Catalog: c, Apply: func(context.Context, []Server, []User) error { applyCalls++; return nil }, FailClosed: func() error { stopCalls++; return nil }}
	err = api.mutateCatalog(t.Context(), func() error { return c.RemoveServer(t.Context(), "server-a") })
	if !errors.Is(err, persist.ErrUncertain) || stopCalls != 1 || applyCalls != 0 {
		t.Fatalf("uncertain catalog did not stop serving: %v stops=%d applies=%d", err, stopCalls, applyCalls)
	}
	restrictive, _ := os.ReadFile(c.path)
	if bytes.Equal(old, restrictive) {
		t.Fatal("API restored stale pre-revocation catalog")
	}
	if _, err := c.Snapshot(t.Context()); !errors.Is(err, persist.ErrUncertain) {
		t.Fatal("uncertain snapshot was exposed")
	}
	if err := c.Restore(t.Context(), reacceptState()); !errors.Is(err, persist.ErrUncertain) {
		t.Fatal("uncertain store allowed old snapshot restoration")
	}
	c.fault = nil
	if err := os.WriteFile(c.path, old, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.ReconcileStorage(); err == nil || c.Degraded() == "" {
		t.Fatal("reconcile accepted stale credential snapshot")
	}
	if err := os.WriteFile(c.path, restrictive, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.ReconcileStorage(); err != nil {
		t.Fatal(err)
	}
	st, err := c.Snapshot(t.Context())
	if err != nil || c.Degraded() != "" {
		t.Fatal(err)
	}
	if len(st.Servers) != 1 || st.Servers[0].ID != "server-b" || len(st.Users) != 1 || st.Users[0].ID != "user-b" {
		t.Fatal("revoked identities resurrected")
	}
	reopened, err := OpenFileCatalog(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	disk, _ := reopened.Snapshot(t.Context())
	if !reflect.DeepEqual(st, disk) {
		t.Fatal("restart lost authoritative deletion")
	}
	info, _ := os.Stat(c.path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("catalog permissions are not private")
	}
}
func TestReacceptCatalogOwnsNestedState(t *testing.T) {
	c, err := OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := reacceptState()
	input.Servers[0].Tags = []string{"original"}
	input.Servers[0].RealityKeyRef = &KeyRef{KeyID: "key-original"}
	if err := c.Restore(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	input.Servers[0].Tags[0] = "external-write"
	input.Servers[0].RealityKeyRef.KeyID = "external-key"
	input.Subscriptions[0].ProfileIDs[0] = "external-profile"
	got, _ := c.Snapshot(t.Context())
	if got.Servers[0].Tags[0] != "original" || got.Servers[0].RealityKeyRef.KeyID != "key-original" || got.Subscriptions[0].ProfileIDs[0] != "profile-a" {
		t.Fatal("caller modified state without commit")
	}
	got.Servers[0].Tags[0] = "snapshot-write"
	got.Subscriptions[0].ProfileIDs[0] = "snapshot-write"
	servers, _ := c.ListServers(t.Context())
	servers[0].RealityKeyRef.KeyID = "readback-write"
	sub, _ := c.GetSubscription(t.Context(), "shared")
	sub.ProfileIDs[0] = "getter-write"
	final, _ := c.Snapshot(t.Context())
	if final.Servers[0].Tags[0] != "original" || final.Servers[0].RealityKeyRef.KeyID != "key-original" || final.Subscriptions[0].ProfileIDs[0] != "profile-a" {
		t.Fatal("readback aliased authoritative state")
	}
}
func TestReacceptCatalogRejectsInvalidExistingDocument(t *testing.T) {
	for _, raw := range []string{"", "null", "[]", "{", "{} {}"} {
		t.Run(raw, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := OpenFileCatalog(dir); err == nil {
				t.Fatal("invalid document silently reset catalog")
			}
		})
	}
}
func TestReacceptUserDeletionPrunesOnlyItsSubscriptionReferences(t *testing.T) {
	c, err := OpenFileCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Restore(t.Context(), reacceptState()); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveUser(t.Context(), "user-a"); err != nil {
		t.Fatal(err)
	}
	st, _ := c.Snapshot(t.Context())
	if len(st.Profiles) != 1 || st.Profiles[0].ID != "profile-b" || !reflect.DeepEqual(st.Subscriptions[0].ProfileIDs, []string{"profile-b"}) {
		t.Fatal("dangling or unrelated subscription reference")
	}
}
