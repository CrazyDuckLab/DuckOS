//go:build linux

package network

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/CrazyDuckLab/DuckOS/internal/safeapply"
)

// Explicit opt-in and a separate network namespace are both mandatory. This
// test never runs on the host namespace, including ordinary root go test runs.
func requireIsolatedNetworkNamespace(t *testing.T) {
	t.Helper()
	if os.Getenv("DUCKOS_ISOLATED_NETNS_TEST") != "1" {
		t.Skip("requires explicit isolated Linux network namespace")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	init, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == init {
		t.Fatal("refusing firewall test in PID 1 network namespace")
	}
}

func TestNFTNativeAtomicApplyAndRestore(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	h, err := NewFirewallHost()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	empty, err := h.Capture(ctx)
	if err != nil || len(empty.Objects) != 0 {
		t.Fatal("isolated namespace must start without DuckOS table", err)
	}
	first, err := h.Apply(ctx, fixture(), empty)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := h.Restore(ctx, empty); err != nil {
			t.Error("cleanup failed", err)
		}
	}()
	if err = h.Verify(ctx, first); err != nil {
		t.Fatal(err)
	}
	candidate := fixture()
	candidate.Firewall.Forwards[0].Port = 9443
	if _, err = h.Apply(ctx, candidate, first); err != nil {
		t.Fatal(err)
	}
	if err = h.Verify(ctx, first); err == nil {
		t.Fatal("changed live rules passed old fingerprint")
	}
	if err = h.Restore(ctx, first); err != nil {
		t.Fatal(err)
	}
	guarded, err := h.InstallForwardGuard(ctx, first)
	if err != nil {
		t.Fatal("native forwarding barrier installation", err)
	}
	guarded, err = h.ApplyGuarded(ctx, candidate, guarded)
	if err != nil || verifyForwardGuard(guarded) != nil {
		t.Fatal("native guarded candidate", err)
	}
	if err = h.Restore(ctx, first); err != nil {
		t.Fatal("native guarded recovery", err)
	}
	// Real adapter/controller integration, still only in this isolated netns.
	s := firewallTestStore(t, t.TempDir())
	a, err := NewFirewallAdapter()
	if err != nil {
		t.Fatal(err)
	}
	c, err := safeapply.Open(s, a)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	snapshot, _ := s.Snapshot()
	raw, _ := json.Marshal(candidate)
	tx, err := c.Apply(snapshot.Current.ID, raw, "native-test", "isolated nft rollback", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Rollback(tx.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(ctx, first); err != nil {
		t.Fatal("controller failed native before-state recovery", err)
	}
	if _, err = h.Apply(ctx, Config{Version: 1}, first); err != nil {
		t.Fatal(err)
	}
	if err = h.Verify(ctx, empty); err != nil {
		t.Fatal("empty configuration retained active chains", err)
	}
	guarded, err = h.InstallForwardGuard(ctx, empty)
	if err != nil {
		t.Fatal("native empty-state barrier", err)
	}
	guarded, err = h.ApplyGuarded(ctx, Config{Version: 1}, guarded)
	if err != nil || verifyForwardGuard(guarded) != nil {
		t.Fatal("native guarded removal", err)
	}
	if err = h.Restore(ctx, empty); err != nil {
		t.Fatal(err)
	}
}

func TestNFTNativeManagementPolicySyntax(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	h, err := NewFirewallHost()
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Check(context.Background(), edgeFixture()); err != nil {
		t.Fatal(err)
	}
}

func TestDNSMasqNativeGeneratedSyntax(t *testing.T) {
	requireIsolatedNetworkNamespace(t)
	if err := CheckDNSMasq(context.Background(), fixture()); err != nil {
		t.Fatal(err)
	}
	c := fixture()
	c.Interfaces[1].DHCPServer.Reservations = []Reservation{{MAC: "02:00:00:00:00:10", Address: "198.51.100.50", Hostname: "client"}}
	if err := CheckDNSMasq(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := runDNSMasqCheck(context.Background(), []byte("duckos-invalid-option=yes\n")); err == nil {
		t.Fatal("native dnsmasq accepted invalid syntax")
	}
}
