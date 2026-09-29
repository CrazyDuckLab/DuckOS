// Package qa hosts the Reality Check / Red Team QA harness.
//
// It is intentionally lightweight and only depends on the
// rest of DuckOS where it must. It exposes:
//
//   - secret canary generators and grep-based leak checkers
//     used to verify that no plaintext secret ever reaches
//     a support bundle, log, JSON/YAML render, or CLI output.
//   - resource counters for goroutine / fd / temp-file leak
//     detection over repeated operations.
//   - a small Linux lab helper that creates disposable
//     network namespaces and veth pairs without touching the
//     management interface.
//
// The harness is meant to be invoked from go test on
// duck-dev-01 and from Darwin in CI. It must never log or
// print canary values verbatim to anywhere except the
// controlled test report.
package qa

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Canary is a single fake secret planted into a test
// configuration so that the QA harness can later grep the
// generated artifacts and confirm the secret is not present
// in plaintext.
//
// Each canary carries:
//   - Value   : the unique random secret. The value is
//     always a long pure-hex string with no
//     recognizable prefix, so the existing
//     redactor patterns (long hex token, ed25519
//     seed) match it as a real secret would.
//   - Marker  : a short human-readable tag identifying the
//     subsystem it was planted in (vault, xray,
//     wireguard, acme, radius, api-token, import).
//     The marker is NOT included in Value, because
//     real secret issuers attach recognizable
//     prefixes ("sk_live_", "priv_", "Bearer ")
//     and a faithful redaction policy should not
//     rely on prefix matching.
//   - Where   : a description of where the canary was
//     written; used in failure messages only.
type Canary struct {
	Marker string
	Value  string
	Where  string
}

// NewCanary returns a fresh canary carrying a random 256-bit
// hex secret and a stable marker. The value is pure hex so
// the support redactor treats it as a real secret.
func NewCanary(marker string) Canary {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("qa: crypto/rand failed: " + err.Error())
	}
	return Canary{
		Marker: "DUCKQA-" + strings.ToUpper(marker),
		Value:  hex.EncodeToString(buf),
		Where:  "",
	}
}

// MustSeedFile writes a canary value into a temporary file
// under the test scratch dir and returns the file path. The
// file is created with 0600 perms. The test must arrange
// cleanup.
func (c Canary) MustSeedFile(t testing.TB, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("qa: seed mkdir: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(c.Value), 0o600); err != nil {
		t.Fatalf("qa: seed write: %v", err)
	}
	return path
}

// ScanForLeak walks the given root and returns one entry per
// canary it finds verbatim in any regular file. Symlinks are
// not followed. Tests should call AssertNoLeak (which calls
// this) or invoke it directly to assert the exact set of
// leaks for regression purposes.
func ScanForLeak(root string, canaries ...Canary) []string {
	var found []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > 8*1024*1024 {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		as := string(data)
		for _, c := range canaries {
			if c.Value != "" && strings.Contains(as, c.Value) {
				found = append(found, fmt.Sprintf("value %s in %s (marker=%s)", c.Value, path, c.Marker))
			} else if c.Marker != "" && strings.Contains(as, c.Marker) {
				found = append(found, fmt.Sprintf("marker %s in %s", c.Marker, path))
			}
		}
		return nil
	})
	return found
}

// AssertNoLeak walks the given root and fails the test if
// any canary value or marker is found verbatim in any
// regular file. Symlinks are not followed.
func AssertNoLeak(t testing.TB, root string, canaries ...Canary) {
	t.Helper()
	if len(canaries) == 0 {
		return
	}
	found := ScanForLeak(root, canaries...)
	if len(found) > 0 {
		for _, f := range found {
			t.Errorf("SECRET LEAK: %s", f)
		}
		t.FailNow()
	}
}

// AssertNoLeakInString returns true if any canary value or
// marker is present in s. Tests should call t.Fatal if this
// returns true.
func ContainsLeak(s string, canaries ...Canary) bool {
	for _, c := range canaries {
		if c.Value != "" && strings.Contains(s, c.Value) {
			return true
		}
		if c.Marker != "" && strings.Contains(s, c.Marker) {
			return true
		}
	}
	return false
}

// AssertNoLeakInString fails if any canary value is present
// in the provided string. Useful for tests that exercise
// String(), JSON marshaling, error messages, log output, or
// panic values directly.
func AssertNoLeakInString(t testing.TB, label, s string, canaries ...Canary) {
	t.Helper()
	for _, c := range canaries {
		if c.Value != "" && strings.Contains(s, c.Value) {
			t.Fatalf("SECRET LEAK in %s: canary %s present", label, c.Marker)
		}
		if c.Marker != "" && strings.Contains(s, c.Marker) {
			t.Fatalf("SECRET LEAK in %s: marker %s present", label, c.Marker)
		}
	}
}

// TempDir returns a temporary directory under the system
// temp dir. The directory is removed on test cleanup.
func TempDir(t testing.TB) string {
	t.Helper()
	d, err := os.MkdirTemp("", "duckqa-")
	if err != nil {
		t.Fatalf("qa: tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// GoroutineCount returns the number of goroutines right
// now. Used as a coarse leak detector: tests that create
// resources in a loop should observe a stable count after
// each iteration, not a monotonically growing one.
func GoroutineCount() int { return runtime.NumGoroutine() }
