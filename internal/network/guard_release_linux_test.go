//go:build linux

package network

import (
	"context"
	"os"
	"testing"
	"time"
)

// Run only inside a disposable network namespace. Never mutate the host table.
func TestForwardGuardIsolatedKernelReleaseAndRollback(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("DUCKOS_GUARD_REVIEW_NETNS") != "1" {
		t.Skip("requires explicitly isolated root network namespace")
	}
	self, err := os.Stat("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	init, err := os.Stat("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(self, init) {
		t.Fatal("refusing host network namespace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := NewFirewallHost()
	if err != nil {
		t.Fatal(err)
	}
	empty, err := h.Capture(ctx)
	if err != nil || len(empty.Objects) != 0 {
		t.Fatal("fixture must start without duckos table", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := h.InstallForwardGuard(ctx, empty); err != nil {
			t.Fatal(err)
		}
		if err := h.ReleaseForwardGuard(ctx); err != nil {
			t.Fatal(err)
		}
		if err := h.ReleaseForwardGuard(ctx); err != nil {
			t.Fatal("repeat release", err)
		}
		live, err := h.Capture(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := guardSnapshot(live); err != nil {
			t.Fatal("stale barrier after release", err)
		}
		// Actual rollback restores the table before the extra release call.
		if err := h.Restore(ctx, empty); err != nil {
			t.Fatal(err)
		}
		if err := h.ReleaseForwardGuard(ctx); err != nil {
			t.Fatal("release after absent-table rollback", err)
		}
		if err := h.Verify(ctx, empty); err != nil {
			t.Fatal(err)
		}
	}
}
