//go:build linux

package qa

import (
	"strings"
	"testing"
)

// TestLabNetnsCreateAndTeardown is the §49 / §50 smoke
// test: a fresh network namespace is created, brought up,
// and torn down without leaving residue and without
// touching the management interface.
//
// On non-Linux or non-root hosts the test is skipped.
func TestLabNetnsCreateAndTeardown(t *testing.T) {
	before := CaptureManagementBoundary(t)
	lab := NewLab(t, "qa-smoke")
	if _, err := runInNS(lab.NetNS, "ip", "link", "show"); err != nil {
		t.Fatalf("ip link show in %s: %v", lab.NetNS, err)
	}
	out, err := lab.Run("ip", "-br", "link", "show")
	if err != nil {
		t.Fatalf("lab.Run: %v", err)
	}
	if !strings.Contains(string(out), "lo") {
		t.Fatalf("expected lo in lab output, got: %s", out)
	}
	// Drop a veth that the test owns and the teardown
	// will clean up.
	lab.VethPair("qa-vh0", "qa-vl0", "10.99.0.1/30", "10.99.0.2/30")
	// Touch the veth from inside the netns.
	out, err = lab.Run("ip", "-br", "addr", "show", "qa-vl0")
	if err != nil {
		t.Fatalf("lab addr: %v", err)
	}
	if !strings.Contains(string(out), "10.99.0.2") {
		t.Fatalf("expected 10.99.0.2 on qa-vl0, got: %s", out)
	}
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}
