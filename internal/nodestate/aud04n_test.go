// Package nodestate / aud04n_test.go: regression suite for AUD-04N
// (fresh-install restart availability blocker). Scenario A–I per
// the remediation mission; scenario A mirrors the auditor's
// TestAUDIT_FreshInstallSecondBootLosesCenter.
package nodestate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CrazyDuckLab/DuckOS/internal/persist"
	"github.com/CrazyDuckLab/DuckOS/internal/xnode"
)

// A: fresh install → no mutations → restart → center available.
// (The auditor reproduction.)
func TestAUD04N_A_FreshInstallSecondBootStaysAvailable(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("boot 1 failed: %v", err)
	}
	// The fresh install must leave a durable (empty) snapshot.
	if !stateExists(dir) {
		t.Fatal("AUD-04N DEFECT: fresh install left no state snapshot")
	}
	// Boot 2 with zero node mutations: the center must come up.
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("AUD-04N DEFECT: second boot of untouched fresh install fails closed: %v", err)
	}
}

// B: fresh install → subscription-only (no nodes) → restart → the
// center (and thus subscription delivery validation) stays usable.
func TestAUD04N_B_SubscriptionOnlyRestart(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
	// Subscription tokens live in a sibling store; "subscription
	// only" means zero node mutations. Restart:
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("subscription-only restart blocked: %v", err)
	}
}

// C: fresh install → create node → restart → identity preserved.
func TestAUD04N_C_NodeIdentitySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	sf, err := EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A node enrolls after init; the snapshot persists it.
	st := sf.Get()
	st.Nodes = append(st.Nodes, xnode.Node{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name: "edge", Kind: xnode.KindRemote, CredentialHash: "h", Status: xnode.StatusActive})
	if out, err := sf.Update(func(cur *xnode.DurableState) { *cur = st }); err != nil || out != persist.CommitOK {
		t.Fatalf("node commit: %v %v", out, err)
	}
	sf2, err := EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf2.Get().Nodes) != 1 || sf2.Get().Nodes[0].Name != "edge" {
		t.Fatal("node identity lost across restart")
	}
}

// D: interrupted initialization before the snapshot → safe recovery
// (next boot retries init cleanly).
func TestAUD04N_D_InterruptedInitRecovers(t *testing.T) {
	dir := t.TempDir()
	// Simulate the interrupted state: NOTHING was committed yet.
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
}

// E: snapshot committed, marker missing → safe reconciliation.
func TestAUD04N_E_SnapshotWithoutMarker(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, NodeMigratedMarker)); err != nil {
		t.Fatal(err)
	}
	sf, err := EnsureMigrated(dir)
	if err != nil {
		t.Fatalf("snapshot-guarded boot failed: %v", err)
	}
	// The boundary is re-established.
	if _, err := os.Stat(filepath.Join(dir, NodeMigratedMarker)); err != nil {
		t.Fatal("marker not re-established")
	}
	_ = sf
}

// F: marker present, committed snapshot genuinely lost → fail
// closed (legacy quarantine preserved).
func TestAUD04N_F_MarkerWithoutSnapshotFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeLegacyNodes(t, dir, []xnode.Node{{ID: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Name: "edge", Kind: xnode.KindRemote, CredentialHash: "h", Status: xnode.StatusActive}})
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, NodeStateFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureMigrated(dir); err == nil {
		t.Fatal("AUD-04N REGRESSION: legacy re-activated after snapshot loss")
	}
}

// G: legacy migration completed → restart → no resurrection.
func TestAUD04N_G_LegacyMigrationNoResurrection(t *testing.T) {
	dir := t.TempDir()
	writeLegacyNodes(t, dir, []xnode.Node{{ID: "ffffffffffffffffffffffffffffffff",
		Name: "legacy", Kind: xnode.KindRemote, CredentialHash: "old", Status: xnode.StatusActive}})
	sf, err := EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.Get().Nodes) != 1 || sf.Get().Nodes[0].CredentialHash != "old" {
		t.Fatal("migration lost the legacy node")
	}
	sf2, err := EnsureMigrated(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf2.Get().Nodes) != 1 {
		t.Fatal("restart duplicated or dropped migrated nodes")
	}
}

// H: storage error during initialization → no false success, no
// marker, next boot retries.
func TestAUD04N_H_InitStorageErrorNoFalseSuccess(t *testing.T) {
	dir := t.TempDir()
	restore := persist.SyncDirForTest(func(string) error { return errors.New("injected ENOSPC") })
	_, err := EnsureMigrated(dir)
	restore()
	if err == nil {
		t.Fatal("initialization acknowledged despite failing storage")
	}
	// The marker must NOT exist (no boundary established on a
	// failed init).
	if _, err := os.Stat(filepath.Join(dir, NodeMigratedMarker)); err == nil {
		t.Fatal("boundary marker committed on a failed init")
	}
	// Storage recovers → init retries cleanly.
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("retry after storage recovery: %v", err)
	}
}

// I: post-rename directory fsync failure during init — commit
// uncertainty: the operation fails, the marker is not written, and
// the NEXT boot treats the on-disk snapshot (which the rename
// placed) as live.
func TestAUD04N_I_InitUncertainCommitRecoversOnNextBoot(t *testing.T) {
	dir := t.TempDir()
	restore := persist.SyncDirForTest(func(string) error { return errors.New("injected EIO on directory fsync") })
	_, err := EnsureMigrated(dir)
	restore()
	if err == nil || !contains(err.Error(), "uncertain") {
		t.Fatalf("init uncertainty not reported: %v", err)
	}
	// Next boot: the snapshot exists (rename landed) → live.
	if !stateExists(dir) {
		t.Fatal("uncertain init commit lost the snapshot that the rename placed")
	}
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("boot after uncertain init: %v", err)
	}
}

func stateExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, NodeStateFile))
	return err == nil
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func writeLegacyNodes(t *testing.T, dir string, nodes []xnode.Node) {
	t.Helper()
	f, err := persist.Open[[]xnode.Node](dir, "nodes.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Update(func(cur *[]xnode.Node) { *cur = nodes }); err != nil {
		t.Fatal(err)
	}
}
