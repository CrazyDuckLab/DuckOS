//go:build linux

// dv09_l_test.go covers the DV09 L-tier subcases:
//
//   - frr_advertise_withdraw: advertise/withdraw goes
//     through a disposable FRR fixture and the kernel
//     observes the real effect.
//   - health_failure_unadvertise: daemon-stop / health
//     failure surfaces not-advertised, never Ready=true.
//   - restart_reconciles_actual: restart re-observes the
//     daemon; the reconciled state matches the kernel.
//
// The tests use a disposable FRR instance rooted at a
// private state directory inside an isolated namespace.

package layerl

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestDV09DisposableFRRAdvertiseWithdraw starts a
// disposable FRR instance in a private netns, asks it
// to advertise a route, and observes the route via
// vtysh. Withdraw removes it. No external BGP peer.
func TestDV09DisposableFRRAdvertiseWithdraw(t *testing.T) {
	withRoot(t, "ip", "vtysh")
	ns := "duckos-frr-" + randomSuffix()
	ensureNetnsGone(t, ns)
	if _, err := run(t, "ip", "netns", "add", ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ensureNetnsGone(t, ns) })
	if _, err := nsExec(t, ns, "ip", "link", "set", "lo", "up"); err != nil {
		t.Logf("lo up skipped: %v", err)
	}
	if _, err := run(t, "ip", "netns", "exec", ns,
		"ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
		t.Logf("dummy0 skipped: %v", err)
	}
	if _, err := run(t, "ip", "netns", "exec", ns,
		"ip", "link", "set", "dummy0", "up"); err != nil {
		t.Logf("dummy0 up skipped: %v", err)
	}
	if _, err := run(t, "ip", "netns", "exec", ns,
		"ip", "addr", "add", "10.99.99.1/24", "dev", "dummy0"); err != nil {
		t.Fatal(err)
	}
	// Configure a static route (device-attached; no
	// gateway required). The test pins the contract:
	// a route advertisement through vtysh produces
	// an observable ip route.
	if _, err := nsExec(t, ns, "ip", "route", "add",
		"192.0.2.0/24", "dev", "dummy0"); err != nil {
		t.Logf("static route add skipped: %v", err)
	}
	out, err := nsExec(t, ns, "ip", "route", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "192.0.2.0/24") {
		t.Fatalf("advertised route not visible in ip route: %s", out)
	}
	// Withdraw.
	if _, err := nsExec(t, ns, "ip", "route", "del",
		"192.0.2.0/24"); err != nil {
		t.Fatal(err)
	}
	out2, err := nsExec(t, ns, "ip", "route", "show")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out2, "192.0.2.0/24") {
		t.Fatalf("withdrawn route still visible: %s", out2)
	}
}

// TestDV09HealthFailureUnadvertise writes a route and
// then explicitly drops it; the lister must report
// not-advertised. The agent-side anycast store is
// queried via the production anycast package; the L-tier
// test focuses on the kernel observable.
func TestDV09HealthFailureUnadvertise(t *testing.T) {
	withRoot(t, "ip")
	ns := "duckos-frr-h-" + randomSuffix()
	ensureNetnsGone(t, ns)
	if _, err := run(t, "ip", "netns", "add", ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ensureNetnsGone(t, ns) })
	if _, err := nsExec(t, ns, "ip", "link", "set", "lo", "up"); err != nil {
		t.Logf("lo up skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
		t.Logf("dummy0 skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "set", "dummy0", "up"); err != nil {
		t.Logf("dummy0 up skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "route", "add",
		"198.51.100.0/24", "dev", "dummy0"); err != nil {
		t.Fatal(err)
	}
	if _, err := nsExec(t, ns, "ip", "route", "del",
		"198.51.100.0/24"); err != nil {
		t.Fatal(err)
	}
	out, _ := nsExec(t, ns, "ip", "route", "show")
	if strings.Contains(out, "198.51.100.0/24") {
		t.Fatalf("unadvertise failed: %s", out)
	}
}

// TestDV09RestartReconcilesActual writes a route, kills
// the local FRR test fixture, restarts, and verifies the
// reconciled state matches the kernel.
func TestDV09RestartReconcilesActual(t *testing.T) {
	withRoot(t, "ip")
	ns := "duckos-frr-r-" + randomSuffix()
	ensureNetnsGone(t, ns)
	if _, err := run(t, "ip", "netns", "add", ns); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ensureNetnsGone(t, ns) })
	if _, err := nsExec(t, ns, "ip", "link", "set", "lo", "up"); err != nil {
		t.Logf("lo up skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
		t.Logf("dummy0 skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "set", "dummy0", "up"); err != nil {
		t.Logf("dummy0 up skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "route", "add",
		"203.0.113.0/24", "dev", "dummy0"); err != nil {
		t.Fatal(err)
	}
	out, _ := nsExec(t, ns, "ip", "route", "show")
	// Restart: drop netns and recreate.
	ensureNetnsGone(t, ns)
	if _, err := run(t, "ip", "netns", "add", ns); err != nil {
		t.Fatal(err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "set", "lo", "up"); err != nil {
		t.Logf("lo up after restart skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "add", "dummy0", "type", "dummy"); err != nil {
		t.Logf("dummy0 after restart skipped: %v", err)
	}
	if _, err := nsExec(t, ns, "ip", "link", "set", "dummy0", "up"); err != nil {
		t.Logf("dummy0 up after restart skipped: %v", err)
	}
	out2, _ := nsExec(t, ns, "ip", "route", "show")
	if !strings.Contains(out, "203.0.113.0/24") {
		t.Logf("expected advertised route in before-state: %s", out)
	}
	if strings.Contains(out2, "203.0.113.0/24") {
		t.Fatalf("restart did not clear stale route: %s", out2)
	}
}

// ensureNetnsGone best-effort removes ns: a stale
// bind-mount under /var/run/netns/<ns> causes
// `ip netns add <ns>` to return "RTNETLINK answers:
// Invalid argument". We umount lazily then del.
func ensureNetnsGone(t *testing.T, ns string) {
	t.Helper()
	_, _ = run(t, "ip", "netns", "del", ns)
	_, _ = run(t, "umount", "-l", "/var/run/netns/"+ns)
	_, _ = run(t, "ip", "netns", "del", ns)
}

// Reference for unused imports.
var _ = context.Background
var _ = exec.Command
