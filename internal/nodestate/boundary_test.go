// Package main / nodestate_test.go: AUD-04 regression suite — the
// durable migration boundary of the node-security catalog.
package nodestate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/nodestate"
	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

func writeLegacy(t *testing.T, dir string, nodes []xnode.Node) {
	t.Helper()
	f, err := persist.Open[[]xnode.Node](dir, "nodes.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Update(func(cur *[]xnode.Node) { *cur = nodes }); err != nil {
		t.Fatal(err)
	}
}

// First installation: no legacy, no marker → empty snapshot, marker
// established (a stray legacy file appearing later can never
// activate).
func TestAUD04_FirstInstall(t *testing.T) {
	dir := t.TempDir()
	sf, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Get().Nodes) != 0 {
		t.Fatal("fresh install produced nodes")
	}
	if _, err := os.Stat(filepath.Join(dir, nodestate.NodeMigratedMarker)); err != nil {
		t.Fatal("migration boundary marker not established")
	}
}

// First migration: legacy active record migrates; restart keeps it.
func TestAUD04_FirstMigrationAndRestart(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "edge",
		Kind: xnode.KindRemote, CredentialHash: "hash1", Status: xnode.StatusActive}})
	sf, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Get().Nodes) != 1 {
		t.Fatal("migration lost the node")
	}
	// Restart.
	sf2, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf2.Get().Nodes) != 1 || sf2.Get().Nodes[0].CredentialHash != "hash1" {
		t.Fatal("restart altered migrated state")
	}
}

// The audit scenario: post-migration snapshot loss must NOT
// re-migrate from legacy (revoked node stays revoked in the sense
// that the legacy active record is never reactivated; startup fails
// closed).
func TestAUD04_LostSnapshotFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "deadbeefdeadbeefdeadbeefdeadbeef", Name: "edge",
		Kind: xnode.KindRemote, CredentialHash: "aabb", Status: xnode.StatusActive}})
	if _, err := nodestate.EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
	// Revoke AFTER migration: the unified snapshot moves ahead of
	// legacy.
	sf, _ := persist.Open[xnode.DurableState](dir, nodestate.NodeStateFile)
	st := sf.Get()
	st.Nodes[0].Status = xnode.StatusRevoked
	if out, err := sf.Update(func(cur *xnode.DurableState) { *cur = st }); err != nil || out != persist.CommitOK {
		t.Fatalf("revoke commit: %v %v", out, err)
	}
	// Storage anomaly: the unified snapshot is lost; legacy files
	// survive by design.
	if err := os.Remove(filepath.Join(dir, nodestate.NodeStateFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := nodestate.EnsureMigrated(dir); err == nil {
		t.Fatal("AUD-04 DEFECT: startup re-migrated from legacy after snapshot loss")
	}
}

// Corrupt unified snapshot: fail closed, never legacy fallback.
func TestAUD04_CorruptSnapshotFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Name: "edge",
		Kind: xnode.KindRemote, CredentialHash: "h", Status: xnode.StatusActive}})
	if _, err := nodestate.EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nodestate.NodeStateFile), []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := nodestate.EnsureMigrated(dir); err == nil {
		t.Fatal("corrupt snapshot silently recovered from legacy")
	}
}

// Interrupted migration (snapshot committed, marker lost): the
// snapshot itself guards; the marker is re-established.
func TestAUD04_InterruptedMigration(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "cccccccccccccccccccccccccccccccc", Name: "edge",
		Kind: xnode.KindRemote, CredentialHash: "h", Status: xnode.StatusActive}})
	sf, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, nodestate.NodeMigratedMarker))
	sf2, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatalf("snapshot-guarded restart failed: %v", err)
	}
	if len(sf2.Get().Nodes) != 1 || sf.Get().Nodes[0].ID != "cccccccccccccccccccccccccccccccc" {
		t.Fatal("interrupted migration changed state")
	}
}

// Inconsistent legacy data: migration refuses (fail closed).
func TestAUD04_InconsistentLegacy(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "", Kind: xnode.KindRemote, Status: xnode.StatusActive}})
	if _, err := nodestate.EnsureMigrated(dir); err == nil {
		t.Fatal("inconsistent legacy accepted")
	}
}

// Revoked legacy record: migrates as revoked — never activated.
func TestAUD04_RevokedLegacyStaysRevoked(t *testing.T) {
	dir := t.TempDir()
	writeLegacy(t, dir, []xnode.Node{{ID: "dddddddddddddddddddddddddddddddd", Name: "edge",
		Kind: xnode.KindRemote, CredentialHash: "h", Status: xnode.StatusRevoked}})
	sf, err := nodestate.EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sf.Get().Nodes[0].Status != xnode.StatusRevoked {
		t.Fatal("revoked legacy record activated by migration")
	}
	_ = time.Now
}
