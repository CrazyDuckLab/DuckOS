// Package nodestate / reaudit_equiv_test.go: the independent
// auditor's TestAUDIT_FreshInstallSecondBootLosesCenter re-expressed
// in the development branch (harness adapted to the dev suite's
// helpers; the CENTRAL ASSERTION is preserved verbatim: boot 2 of an
// untouched fresh install must succeed).
package nodestate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReaudit_FreshInstallSecondBootLosesCenter(t *testing.T) {
	dir := t.TempDir()
	// Boot 1: fresh install — marker established, center enabled.
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("boot 1 failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, NodeStateFile)); err == nil {
		t.Log("note: fresh install DID leave a state snapshot — finding may be fixed")
	} else {
		t.Log("confirmed: fresh install leaves no state snapshot")
	}
	// Boot 2 with zero node mutations: the center must still come up.
	if _, err := EnsureMigrated(dir); err != nil {
		t.Fatalf("CONFIRMED (AUD-04N): second boot of an untouched fresh install fails closed: %v", err)
	}
	// Extended per the remediation mission: a THIRD boot (and any
	// number after) remains available.
	for i := 3; i <= 4; i++ {
		if _, err := EnsureMigrated(dir); err != nil {
			t.Fatalf("boot %d failed: %v", i, err)
		}
	}
}
