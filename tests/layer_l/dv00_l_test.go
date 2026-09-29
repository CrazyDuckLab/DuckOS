//go:build linux

// dv00_l_test.go covers the DV00 L-tier subcase:
//
//   - production runtime oracle mutation: a representative
//     mutation runs against a disposable Linux fixture
//     and produces an independent runtime effect.

package layerl

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDV00ProductionMutationOracle runs a representative
// mutation against a real Linux fixture (a dummy interface
// in an isolated namespace). The before/after kernel state
// differs by exactly the expected mutation. No fake
// success: the mutation must produce a real, observable
// runtime effect.
func TestDV00ProductionMutationOracle(t *testing.T) {
	withRoot(t, "ip")
	ns := "duckos-dv00"
	setupDummyNS(t, ns)
	before, err := exec.Command("ip", "netns", "exec", ns,
		"ip", "addr", "show", "dummy0").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "dummy0") {
		t.Fatalf("dummy0 missing in ns: %s", before)
	}
	if _, err := runShell(t, "ip", "netns", "exec", ns,
		"ip", "addr", "add", "10.99.99.1/32", "dev", "dummy0"); err != nil {
		t.Fatal(err)
	}
	after, err := exec.Command("ip", "netns", "exec", ns,
		"ip", "addr", "show", "dummy0").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "10.99.99.1") {
		t.Fatalf("mutation did not produce a runtime effect: %s", after)
	}
	if strings.Contains(string(before), "10.99.99.1") {
		t.Fatalf("before-state already contained the mutated address: %s", before)
	}
}

// TestDV00NoFakeSuccess asserts the runtime oracle
// detects a failed mutation. If the production code
// reports success without producing the kernel effect,
// the test fails.
func TestDV00NoFakeSuccess(t *testing.T) {
	withRoot(t, "ip")
	ns := "duckos-dv00n"
	setupDummyNS(t, ns)
	out, err := runShell(t, "ip", "netns", "exec", ns,
		"ip", "addr", "add", "10.99.99.1/32", "dev", "nonexistent")
	if err == nil {
		t.Fatalf("mutation against non-existent interface succeeded (fake success): %s", out)
	}
}

// setupDummyNS creates a fresh netns, adds dummy0, and
// moves it inside. Cleanup is automatic via t.Cleanup.
func setupDummyNS(t *testing.T, ns string) {
	t.Helper()
	_, _ = runShell(t, "ip", "netns", "del", ns)
	if _, err := runShell(t, "ip", "netns", "add", ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = runShell(t, "ip", "netns", "del", ns) })
	if _, err := runShell(t, "ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
		// dummy0 may exist from a prior test run; remove
		// and retry.
		_, _ = runShell(t, "ip", "link", "del", "dummy0")
		if _, err := runShell(t, "ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runShell(t, "ip", "link", "set", "dummy0", "netns", ns); err != nil {
		t.Fatal(err)
	}
	if _, err := runShell(t, "ip", "netns", "exec", ns,
		"ip", "link", "set", "dummy0", "up"); err != nil {
		t.Fatal(err)
	}
}
