package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/identity"
)

func TestPolicyStartupRejectsCorruptOverlay(t *testing.T) {
	dir := t.TempDir()
	store, err := openPolicyStoreForStartup(dir)
	if err != nil {
		t.Fatal(err)
	}
	grant := identity.Grant{
		Role:       identity.Helpdesk,
		Object:     identity.ObjectKey{Kind: "wireguard.peer", ID: "peer-1"},
		Permission: identity.ConfigureNetwork,
		Decision:   identity.DecisionDeny,
		Reason:     "security regression",
	}
	if err := store.Set(grant); err != nil {
		t.Fatal(err)
	}
	reopened, err := openPolicyStoreForStartup(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Policy().AuthorizeObject(identity.Helpdesk, grant.Object, identity.ConfigureNetwork); err == nil {
		t.Fatal("persisted deny was not enforced before corruption")
	}
	if err := os.WriteFile(filepath.Join(dir, "policy-overlay.log"), []byte("{not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openPolicyStoreForStartup(dir); err == nil {
		t.Fatal("production startup helper accepted corrupt policy overlay")
	}
}
