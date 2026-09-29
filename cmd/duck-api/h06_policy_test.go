package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

func TestH06UnknownPersistedDenyBlocksProductionStartup(t *testing.T) {
	for _, permission := range []string{"network.writX", "", "*"} {
		t.Run(permission, func(t *testing.T) {
			dir := t.TempDir()
			store, err := openPolicyStoreForStartup(dir)
			if err != nil {
				t.Fatal(err)
			}
			g := identity.Grant{Role: identity.NetworkAdmin, Object: identity.ObjectKey{Kind: "wireguard.peer", ID: "peer-1"}, Permission: identity.ConfigureNetwork, Decision: identity.DecisionDeny, Reason: "deny"}
			if err := store.Set(g); err != nil {
				t.Fatal(err)
			}
			good, err := openPolicyStoreForStartup(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := good.Policy().AuthorizeObject(g.Role, g.Object, g.Permission); err == nil {
				t.Fatal("persisted deny not enforced")
			}
			path := filepath.Join(dir, "policy-overlay.log")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := bytes.ReplaceAll(raw, []byte(`"network.write"`), []byte(`"`+permission+`"`))
			if bytes.Equal(raw, corrupt) {
				t.Fatal("fixture did not corrupt persisted permission")
			}
			if err := os.WriteFile(path, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			if reopened, err := openPolicyStoreForStartup(dir); err == nil || reopened != nil {
				t.Fatal("invalid deny accepted at production startup")
			}
			if err := good.Policy().AuthorizeObject(g.Role, g.Object, g.Permission); err == nil {
				t.Fatal("existing valid deny disappeared")
			}
		})
	}
}
