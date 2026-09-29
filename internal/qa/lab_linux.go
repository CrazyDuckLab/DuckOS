//go:build linux

// Lab helpers used by the Reality Check
// program. The lab creates disposable network namespaces,
// veth pairs, bridges, and ip links, and asserts that none
// of those touches the management interface. Every helper
// is paired with a Cleanup that uses t.Cleanup so the host
// is left in a known state even if a test fails.
package qa

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// CaptureManagementBoundary records the current state of
// the management interface, route, and sshd. Tests call it
// before and after destructive batches and AssertBoundary
// after the test to confirm the boundary is intact.
func CaptureManagementBoundary(t testing.TB) ManagementBoundary {
	t.Helper()
	if runtime.GOOS != "linux" {
		return ManagementBoundary{}
	}
	if os.Geteuid() != 0 {
		t.Skip("lab requires root")
	}
	b := ManagementBoundary{Interface: "eth0"}
	out, err := exec.Command("/sbin/ip", "-br", "-j", "addr", "show", "eth0").Output()
	if err == nil {
		b.Address = trimAll(string(out))
	}
	out, err = exec.Command("/sbin/ip", "-j", "route", "show", "default").Output()
	if err == nil {
		b.Gateway = trimAll(string(out))
	}
	err = exec.Command("pgrep", "-f", "sshd").Run()
	b.SSHAlive = err == nil
	return b
}

// AssertBoundaryEqual fails the test if the post-test
// boundary differs from the snapshot. A difference is
// treated as a HARD test failure because it means the lab
// damaged the production management plane.
func AssertBoundaryEqual(t testing.TB, before, after ManagementBoundary) {
	t.Helper()
	if before.Address != "" && before.Address != after.Address {
		t.Fatalf("MANAGEMENT BOUNDARY VIOLATION: eth0 address changed from %q to %q", before.Address, after.Address)
	}
	if before.Gateway != "" && before.Gateway != after.Gateway {
		t.Fatalf("MANAGEMENT BOUNDARY VIOLATION: default route changed from %q to %q", before.Gateway, after.Gateway)
	}
	if before.SSHAlive && !after.SSHAlive {
		t.Fatalf("MANAGEMENT BOUNDARY VIOLATION: sshd is no longer running")
	}
}

// NewLab creates a fresh network namespace with a
// loopback, no routes, and a /24 inside-link address.
// The lab is automatically torn down on test cleanup.
// A leak assertion is registered FIRST so it runs AFTER
// every other cleanup (t.Cleanup is LIFO).
func NewLab(t testing.TB, name string) *Lab {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("linux lab requires Linux")
	}
	if os.Geteuid() != 0 {
		t.Skip("lab requires root (sudo -n)")
	}
	if !hasBinary("/sbin/ip") {
		t.Skip("/sbin/ip not found")
	}
	if name == "" {
		name = fmt.Sprintf("lab-%d", time.Now().UnixNano())
	}
	_ = runShell("ip", "netns", "del", name)
	if err := runShell("ip", "netns", "add", name); err != nil {
		t.Fatalf("lab: netns add %s: %v", name, err)
	}
	lab := &Lab{Name: name, NetNS: name, t: t}
	// LIFO: register CleanupLeaked FIRST so it runs AFTER
	// the netns / link deletion cleanups registered below.
	t.Cleanup(func() { lab.CleanupLeaked() })
	t.Cleanup(func() {
		_ = runShell("ip", "netns", "del", name)
	})
	return lab
}

// VethPair creates a veth pair connecting the lab netns to
// a host-side peer link. The host-side link gets the host
// end of the pair; the lab end is moved into the netns.
// Both ends are auto-deleted. The single lab-wide leak
// check is registered in NewLab, so it runs LAST.
func (l *Lab) VethPair(hostName, labName, hostAddr, labAddr string) {
	l.t.Helper()
	if err := runShell("ip", "link", "add", hostName, "type", "veth", "peer", "name", labName); err != nil {
		l.t.Fatalf("lab: veth add %s/%s: %v", hostName, labName, err)
	}
	l.OwnedLinks = append(l.OwnedLinks, hostName)
	l.t.Cleanup(func() {
		_ = runShell("ip", "link", "del", hostName)
	})
	if err := runShell("ip", "link", "set", labName, "netns", l.NetNS); err != nil {
		l.t.Fatalf("lab: move %s into %s: %v", labName, l.NetNS, err)
	}
	if err := runShell("ip", "addr", "add", hostAddr, "dev", hostName); err != nil {
		l.t.Fatalf("lab: addr on %s: %v", hostName, err)
	}
	if err := runShell("ip", "link", "set", hostName, "up"); err != nil {
		l.t.Fatalf("lab: up %s: %v", hostName, err)
	}
	if _, err := runInNS(l.NetNS, "ip", "addr", "add", labAddr, "dev", labName); err != nil {
		l.t.Fatalf("lab: addr in ns on %s: %v", labName, err)
	}
	if _, err := runInNS(l.NetNS, "ip", "link", "set", labName, "up"); err != nil {
		l.t.Fatalf("lab: up %s in ns: %v", labName, err)
	}
	if _, err := runInNS(l.NetNS, "ip", "link", "set", "lo", "up"); err != nil {
		l.t.Fatalf("lab: up lo in ns: %v", err)
	}
}

// VethBetween creates a veth pair connecting two lab
// netns directly. Each end is moved into its respective
// netns. The host sees the pair only briefly during
// creation; the veth is auto-deleted when the first lab
// is torn down.
func VethBetween(t testing.TB, a, b *Lab, aName, bName, aAddr, bAddr string) {
	t.Helper()
	if err := runShell("ip", "link", "add", aName, "type", "veth", "peer", "name", bName); err != nil {
		t.Fatalf("lab: veth add %s/%s: %v", aName, bName, err)
	}
	// Track the link in BOTH labs so cleanup from either
	// side removes it.
	a.OwnedLinks = append(a.OwnedLinks, aName)
	b.OwnedLinks = append(b.OwnedLinks, bName)
	t.Cleanup(func() {
		_ = runShell("ip", "link", "del", aName)
	})
	if err := runShell("ip", "link", "set", aName, "netns", a.NetNS); err != nil {
		t.Fatalf("lab: move %s into %s: %v", aName, a.NetNS, err)
	}
	if err := runShell("ip", "link", "set", bName, "netns", b.NetNS); err != nil {
		t.Fatalf("lab: move %s into %s: %v", bName, b.NetNS, err)
	}
	if _, err := runInNS(a.NetNS, "ip", "addr", "add", aAddr, "dev", aName); err != nil {
		t.Fatalf("lab: addr in %s on %s: %v", a.NetNS, aName, err)
	}
	if _, err := runInNS(b.NetNS, "ip", "addr", "add", bAddr, "dev", bName); err != nil {
		t.Fatalf("lab: addr in %s on %s: %v", b.NetNS, bName, err)
	}
	if _, err := runInNS(a.NetNS, "ip", "link", "set", aName, "up"); err != nil {
		t.Fatalf("lab: up %s in %s: %v", aName, a.NetNS, err)
	}
	if _, err := runInNS(b.NetNS, "ip", "link", "set", bName, "up"); err != nil {
		t.Fatalf("lab: up %s in %s: %v", bName, b.NetNS, err)
	}
	if _, err := runInNS(a.NetNS, "ip", "link", "set", "lo", "up"); err != nil {
		t.Fatalf("lab: up lo in %s: %v", a.NetNS, err)
	}
	if _, err := runInNS(b.NetNS, "ip", "link", "set", "lo", "up"); err != nil {
		t.Fatalf("lab: up lo in %s: %v", b.NetNS, err)
	}
}

// Run executes a command inside the lab netns and returns
// its combined output.
func (l *Lab) Run(prog string, args ...string) ([]byte, error) {
	return runInNS(l.NetNS, prog, args...)
}

// RunShell runs a command in the lab netns, failing the
// test on non-zero exit.
func (l *Lab) RunShell(prog string, args ...string) {
	l.t.Helper()
	if _, err := runInNS(l.NetNS, prog, args...); err != nil {
		l.t.Fatalf("lab shell: %s %v: %v", prog, args, err)
	}
}

// CleanupLeaked asserts that no netns, veth, or bridge
// from the test remains. It is called by the test harness
// after t.Cleanup; if any residue is found the test fails.
func (l *Lab) CleanupLeaked() {
	l.t.Helper()
	out, err := exec.Command("/sbin/ip", "-br", "netns", "list").Output()
	if err == nil && strings.Contains(string(out), l.Name) {
		l.t.Errorf("LEAK: netns %s still present", l.Name)
		_ = runShell("ip", "netns", "del", l.Name)
	}
	for _, link := range l.OwnedLinks {
		out, err := exec.Command("/sbin/ip", "-br", "link", "show", link).Output()
		if err == nil && len(strings.TrimSpace(string(out))) > 0 {
			l.t.Errorf("LEAK: link %s still present", link)
			_ = runShell("ip", "link", "del", link)
		}
	}
}

func runShell(prog string, args ...string) error {
	cmd := exec.Command(prog, args...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w (out=%s)", prog, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runInNS(ns, prog string, args ...string) ([]byte, error) {
	full := append([]string{"netns", "exec", ns, prog}, args...)
	cmd := exec.Command("/sbin/ip", full...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	return cmd.CombinedOutput()
}

func hasBinary(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func trimAll(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// LinkExists returns true if the named link is present on
// the host (not inside any netns).
func LinkExists(name string) bool {
	out, err := exec.Command("/sbin/ip", "-br", "link", "show", name).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// ParseCIDR is a thin wrapper around net.ParseCIDR that
// fails the test on bad input.
func ParseCIDR(t testing.TB, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("qa: bad CIDR %q: %v", s, err)
	}
	return n
}

// TempTree creates a fresh directory under t.TempDir and
// returns it.
func TempTree(t testing.TB) string {
	t.Helper()
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "etcd"), 0o700); err != nil {
		t.Fatalf("qa: temp tree: %v", err)
	}
	return d
}

// ErrNoLab skips the current test because the lab
// environment (root + /sbin/ip) is not available.
var ErrNoLab = errors.New("qa: lab environment not available")
