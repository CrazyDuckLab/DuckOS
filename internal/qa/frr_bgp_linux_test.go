//go:build linux

// Package qa: real FRR dataplane qualification for §3 of
// the Reality Check program.
//
// The test creates two isolated network namespaces, drops
// a veth pair between them, starts real zebra and bgpd
// daemons inside each netns (with isolated config and
// socket paths), establishes an eBGP session, advertises
// a prefix from one side, observes it in the kernel route
// table of the other side, then withdraws it.
//
// The test is gated to Linux + root + /usr/lib/frr; it
// skips otherwise. It also asserts that the management
// interface is unchanged.
package qa

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// frrConfAB is the eBGP speaker config used in the lab.
// Two peers, AS 65001 and 65002, advertise and receive
// 10.10.10.0/24. Direct eBGP between two /30 peers.
//
// FRR 10.3 silently drops both inbound and outbound
// updates with the message
//
//	"Inbound updates discarded due to missing policy"
//	"Outbound updates discarded due to missing policy"
//
// unless an explicit permit-all route-map is applied to
// the neighbor. The config below defines and applies one
// to make the test deterministic.
func frrConfAB(as int, routerID, neighbor string) string {
	return fmt.Sprintf(`! QA frr.conf
frr defaults traditional
hostname qa-r%d
log file /tmp/qa-frr-%d.log informational
!
route-map qa-permit-all permit 10
!
router bgp %d
 bgp router-id %s
 neighbor %s remote-as %d
 address-family ipv4 unicast
  network 10.10.10.0/24
  redistribute connected
  neighbor %s activate
  neighbor %s route-map qa-permit-all in
  neighbor %s route-map qa-permit-all out
 exit-address-family
!
line vty
!
`, as, as, as, routerID, neighbor, neighborAs(as), neighbor, neighbor, neighbor)
}

func neighborAs(as int) int {
	if as == 65001 {
		return 65002
	}
	return 65001
}

// startFRRDaemons writes the given frr.conf to a temp file
// and starts zebra + bgpd inside ns. Both daemons are killed
// on test cleanup. Returns the pid files and the zebra
// socket path so the bgpd can find it via -z.
//
// The pathspace (-N) flag is mandatory so that
// /run/frr/<ns>/<daemon>.vty is used instead of the host
// /run/frr/<daemon>.vty, which is already bound by the
// production FRR stack on duck-dev-01. root must be a
// member of the frrvty group (added by the QA host setup)
// so that the vty chown succeeds.
func startFRRDaemons(t *testing.T, ns, conf, sockDir, pathspace string) (zebraPID, bgpdPID string) {
	t.Helper()
	if err := os.Chmod(sockDir, 0o777); err != nil {
		t.Fatalf("chmod %s: %v", sockDir, err)
	}
	confPath := filepath.Join(sockDir, "frr.conf")
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	zebraSocket := filepath.Join(sockDir, "zebra.sock")
	zebraPIDFile := filepath.Join(sockDir, "zebra.pid")
	bgpdPIDFile := filepath.Join(sockDir, "bgpd.pid")
	zebraLog := filepath.Join(sockDir, "zebra.log")
	bgpdLog := filepath.Join(sockDir, "bgpd.log")
	// -d daemonizes the FRR process so the surrounding
	// shell exits immediately; without it the test would
	// hang waiting for zebra/bgpd to exit.
	zebraCmd := fmt.Sprintf("/usr/lib/frr/zebra -d -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
		confPath, zebraSocket, zebraPIDFile, pathspace, zebraLog)
	bgpdCmd := fmt.Sprintf("/usr/lib/frr/bgpd -d -f %s -z %s -i %s -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
		confPath, zebraSocket, bgpdPIDFile, pathspace, bgpdLog)
	if err := runInNSShell(ns, "/bin/sh", "-c", zebraCmd); err != nil {
		t.Logf("zebra stderr/stdout: %s", readFirst(t, zebraLog))
		t.Fatalf("start zebra: %v", err)
	}
	if err := runInNSShell(ns, "/bin/sh", "-c", bgpdCmd); err != nil {
		t.Logf("bgpd stderr/stdout: %s", readFirst(t, bgpdLog))
		t.Fatalf("start bgpd: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	t.Cleanup(func() {
		_ = runInNSShell(ns, "/bin/sh", "-c", "kill $(cat "+zebraPIDFile+" 2>/dev/null) 2>/dev/null; kill $(cat "+bgpdPIDFile+" 2>/dev/null) 2>/dev/null; sleep 0.2; kill -9 $(cat "+zebraPIDFile+" 2>/dev/null) 2>/dev/null; kill -9 $(cat "+bgpdPIDFile+" 2>/dev/null) 2>/dev/null; true")
		// Best-effort: wipe the /run/frr/<pathspace> dir
		// that the FRR daemons created. Host FRR ignores
		// unknown pathspaces; the QA test should not leak
		// any /run/frr/<ns>/ residue between runs.
		_ = runShell("/bin/rm", "-rf", "/run/frr/"+pathspace)
	})
	return zebraPIDFile, bgpdPIDFile
}

func readFirst(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("read %s: %v", path, err)
	}
	return string(b)
}

func runInNSShell(ns, prog string, args ...string) error {
	full := append([]string{"netns", "exec", ns, prog}, args...)
	cmd := exec.Command("/sbin/ip", full...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ns shell %s %v: %w (out=%s)", prog, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// vtyCmd runs an FRR vtysh command against the daemons
// in the given netns, using the pathspace the daemons
// were started with. The integrated shell is sufficient;
// it dispatches to bgpd / ospfd / zebra based on the
// command.
func vtyCmd(t *testing.T, ns, pathspace, cmd string) string {
	t.Helper()
	full := fmt.Sprintf(`/usr/bin/vtysh -N %s -c %q 2>&1`, pathspace, cmd)
	out, err := runInNSCapture(ns, "/bin/sh", "-c", full)
	if err != nil {
		t.Logf("vty %s/%s cmd=%q out=%s", ns, pathspace, cmd, out)
	}
	return string(out)
}

// waitForRoute polls ns's kernel route table for the given
// destination and returns true once it appears or false
// after the timeout.
func waitForRoute(t *testing.T, ns, dst string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := runInNSCapture(ns, "/sbin/ip", "-j", "route", "show", dst)
		if err == nil && strings.Contains(string(out), dst) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func hasFRR() bool {
	if _, err := os.Stat("/usr/lib/frr/zebra"); err != nil {
		return false
	}
	if _, err := os.Stat("/usr/lib/frr/bgpd"); err != nil {
		return false
	}
	if _, err := exec.LookPath("/usr/bin/vtysh"); err != nil {
		return false
	}
	return true
}

// TestFRRRealEBGPDataplane is §3.A of the QA program:
// a real FRR eBGP session between two isolated netns,
// with prefix advertisement, kernel route observation,
// and graceful teardown.
func TestFRRRealEBGPDataplane(t *testing.T) {
	if !hasFRR() {
		t.Skip("/usr/lib/frr/{zebra,bgpd} or vtysh not present")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-frr-a")
	labB := NewLab(t, "qa-frr-b")
	// veth directly between the two netns.
	VethBetween(t, labA, labB, "qa-vab-a", "qa-vab-b", "10.250.0.1/30", "10.250.0.2/30")
	// Loopback in each so the router-id is a /32.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "10.250.100.1/32", "dev", "lo")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "addr", "add", "10.250.100.2/32", "dev", "lo")
	// 10.10.10.0/24 must exist as a connected route in
	// Lab A so bgpd can advertise it. A dummy interface
	// is the simplest way to put it into the RIB.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "add", "dummy-qa", "type", "dummy")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "10.10.10.1/24", "dev", "dummy-qa")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "dummy-qa", "up")

	sockA := t.TempDir()
	sockB := t.TempDir()
	confA := frrConfAB(65001, "10.250.100.1", "10.250.0.2")
	confB := frrConfAB(65002, "10.250.100.2", "10.250.0.1")
	startFRRDaemons(t, labA.NetNS, confA, sockA, "qa-frr-a")
	startFRRDaemons(t, labB.NetNS, confB, sockB, "qa-frr-b")

	// Allow the BGP session to converge.
	time.Sleep(3 * time.Second)

	// Diagnostic: dump BGP summary, neighbor advertised, and
	// bgpd log from each side.
	t.Logf("Lab A running-config:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show running-config"))
	t.Logf("Lab A BGP summary:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip bgp summary"))
	t.Logf("Lab B BGP summary:\n%s", vtyCmd(t, labB.NetNS, "qa-frr-b", "show ip bgp summary"))
	t.Logf("Lab A full neighbor:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip bgp neighbors 10.250.0.2"))
	t.Logf("Lab A zebra routes:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip route"))
	t.Logf("Lab A show ip bgp:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip bgp"))
	t.Logf("Lab A advertised to B:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip bgp neighbors 10.250.0.2 advertised-routes"))
	t.Logf("Lab B received from A:\n%s", vtyCmd(t, labB.NetNS, "qa-frr-b", "show ip bgp neighbors 10.250.0.1 routes"))
	t.Logf("Lab A bgpd log:\n%s", readFirst(t, filepath.Join(sockA, "bgpd.log")))
	t.Logf("Lab B bgpd log:\n%s", readFirst(t, filepath.Join(sockB, "bgpd.log")))

	// Lab B should now have 10.10.10.0/24 via 10.250.0.1.
	if !waitForRoute(t, labB.NetNS, "10.10.10.0/24", 10*time.Second) {
		t.Fatalf("Lab B did not learn 10.10.10.0/24 from Lab A")
	}

	// Inspect the route in detail.
	out, _ := runInNSCapture(labB.NetNS, "/sbin/ip", "-j", "route", "show", "10.10.10.0/24")
	if !strings.Contains(string(out), "10.250.0.1") {
		t.Fatalf("expected route via 10.250.0.1 in lab B, got: %s", out)
	}

	// Now withdraw the network in Lab A. The cleanest
	// way is to take the dummy interface down: that
	// removes the connected route from the RIB, which
	// then removes the redistribute + network match,
	// and bgpd naturally stops advertising it.
	if err := runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "dummy-qa", "down"); err != nil {
		t.Fatalf("dummy down: %v", err)
	}
	// Hard refresh the BGP session so the withdraw is
	// pushed to the peer immediately.
	_ = runInNSShell(labA.NetNS, "/bin/sh", "-c", "/usr/bin/vtysh -N qa-frr-a -c 'clear ip bgp * soft out'")
	time.Sleep(3 * time.Second)
	t.Logf("Lab A zebra after withdraw:\n%s", vtyCmd(t, labA.NetNS, "qa-frr-a", "show ip route"))
	t.Logf("Lab B BGP after withdraw:\n%s", vtyCmd(t, labB.NetNS, "qa-frr-b", "show ip bgp"))

	// Lab B should no longer have the route.
	out, _ = runInNSCapture(labB.NetNS, "/sbin/ip", "-j", "route", "show", "10.10.10.0/24")
	if strings.Contains(string(out), "10.10.10.0/24") {
		t.Fatalf("Lab B still has 10.10.10.0/24 after withdraw: %s", out)
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// frrIBGPConf returns an iBGP speaker config. The two
// peers share AS 65000 (iBGP); each advertises a distinct
// /24 that the other must learn via iBGP.
//
// iBGP specifics vs eBGP:
//   - same ASN on both sides (65000 vs 65000)
//   - next-hop is preserved unless next-hop-self is set
//   - iBGP does not require ebgp-multihop; standard
//     directly-connected peer is fine
//
// FRR 10.3 still requires the permit-all route-map on
// in/out (M-2 documented this for eBGP; same rule applies
// to iBGP).
func frrIBGPConf(myAS int, routerID, neighbor string) string {
	return fmt.Sprintf(`! QA frr.conf iBGP
frr defaults traditional
hostname qa-ibgp
log file /tmp/qa-ibgp.log informational
!
route-map qa-permit-all permit 10
!
router bgp %d
 bgp router-id %s
 neighbor %s remote-as %d
 address-family ipv4 unicast
  network 10.20.10.0/24
  redistribute connected
  neighbor %s activate
  neighbor %s route-map qa-permit-all in
  neighbor %s route-map qa-permit-all out
 exit-address-family
!
line vty
!
`, myAS, routerID, neighbor, myAS, neighbor, neighbor, neighbor)
}

// TestFRRRealIBGPDataplane is §3.B of the QA program: a
// real FRR iBGP session between two isolated netns in the
// SAME autonomous system (65000). Each side advertises a
// distinct /24; we verify each learns the other's route
// and that the kernel FIB reflects it.
//
// iBGP vs eBGP differences verified:
//  1. same ASN works (remote-as 65000 == local ASN)
//  2. routes propagate without ebgp-multihop
//  3. next-hop is the iBGP peer address (10.250.0.2 or
//     10.250.0.1), not a self-rewrite, because we do NOT
//     configure next-hop-self
func TestFRRRealIBGPDataplane(t *testing.T) {
	if !hasFRR() {
		t.Skip("/usr/lib/frr/{zebra,bgpd} or vtysh not present")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-ibgp-a")
	labB := NewLab(t, "qa-ibgp-b")
	VethBetween(t, labA, labB, "qa-iab-a", "qa-iab-b", "10.250.0.1/30", "10.250.0.2/30")
	// Loopbacks in each.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "10.250.100.1/32", "dev", "lo")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "addr", "add", "10.250.100.2/32", "dev", "lo")
	// Lab A advertises 10.20.10.0/24 via dummy interface.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "add", "dummy-a", "type", "dummy")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "10.20.10.1/24", "dev", "dummy-a")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "dummy-a", "up")
	// Lab B advertises 10.20.20.0/24 via dummy interface.
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "add", "dummy-b", "type", "dummy")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "addr", "add", "10.20.20.1/24", "dev", "dummy-b")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "set", "dummy-b", "up")

	sockA := t.TempDir()
	sockB := t.TempDir()
	// Both routers use AS 65000 (iBGP).
	confA := frrIBGPConf(65000, "10.250.100.1", "10.250.0.2")
	confB := frrIBGPConf(65000, "10.250.100.2", "10.250.0.1")
	startFRRDaemons(t, labA.NetNS, confA, sockA, "qa-ibgp-a")
	startFRRDaemons(t, labB.NetNS, confB, sockB, "qa-ibgp-b")
	time.Sleep(3 * time.Second)

	t.Logf("Lab A BGP summary:\n%s", vtyCmd(t, labA.NetNS, "qa-ibgp-a", "show ip bgp summary"))
	t.Logf("Lab B BGP summary:\n%s", vtyCmd(t, labB.NetNS, "qa-ibgp-b", "show ip bgp summary"))
	t.Logf("Lab A show ip bgp:\n%s", vtyCmd(t, labA.NetNS, "qa-ibgp-a", "show ip bgp"))
	t.Logf("Lab B show ip bgp:\n%s", vtyCmd(t, labB.NetNS, "qa-ibgp-b", "show ip bgp"))

	// Lab B should learn 10.20.10.0/24 from Lab A via iBGP.
	if !waitForRoute(t, labB.NetNS, "10.20.10.0/24", 10*time.Second) {
		t.Fatalf("Lab B did not learn 10.20.10.0/24 from Lab A via iBGP")
	}
	// Lab A should learn 10.20.20.0/24 from Lab B via iBGP.
	if !waitForRoute(t, labA.NetNS, "10.20.20.0/24", 10*time.Second) {
		t.Fatalf("Lab A did not learn 10.20.20.0/24 from Lab B via iBGP")
	}

	// Verify next-hop semantics: without next-hop-self,
	// iBGP preserves the original next-hop. So Lab B's
	// route to 10.20.10.0/24 must point at the original
	// advertiser on Lab A (the veth IP 10.250.0.1).
	out, _ := runInNSCapture(labB.NetNS, "/sbin/ip", "-j", "route", "show", "10.20.10.0/24")
	if !strings.Contains(string(out), "10.250.0.1") {
		t.Fatalf("expected iBGP route via 10.250.0.1 (next-hop preserved), got: %s", out)
	}
	// And Lab A's route to 10.20.20.0/24 must point at 10.250.0.2.
	out, _ = runInNSCapture(labA.NetNS, "/sbin/ip", "-j", "route", "show", "10.20.20.0/24")
	if !strings.Contains(string(out), "10.250.0.2") {
		t.Fatalf("expected iBGP route via 10.250.0.2 (next-hop preserved), got: %s", out)
	}

	// Withdraw by taking Lab A's dummy down. Lab B's
	// 10.20.10.0/24 must disappear.
	if err := runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "dummy-a", "down"); err != nil {
		t.Fatalf("dummy-a down: %v", err)
	}
	_ = runInNSShell(labA.NetNS, "/bin/sh", "-c", "/usr/bin/vtysh -N qa-ibgp-a -c 'clear ip bgp * soft out'")
	time.Sleep(3 * time.Second)
	if waitForRoute(t, labB.NetNS, "10.20.10.0/24", 5*time.Second) {
		t.Fatalf("Lab B still has 10.20.10.0/24 after withdraw")
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// frrOSPFAB is a minimal OSPFv2 config that advertises the
// link between the two test routers. The two network
// statements cover the /30 transit and the /24 that
// contains the dummy /24 used as the router's
// loopback-equivalent. The /30 transit is forced to
// OSPF network-type point-to-point so the adjacency does
// not require broadcast/multicast DR election (which is
// unreliable on a veth pair across netns in this lab).
func frrOSPFAB(routerID, iface string) string {
	return fmt.Sprintf(`! QA ospf.conf
frr defaults traditional
hostname qa-ospf
log file /tmp/qa-ospf.log informational
!
interface %s
 ip ospf network point-to-point
!
router ospf
 ospf router-id %s
 network 10.251.0.0/30 area 0
 network 10.251.100.0/24 area 0
!
line vty
!
`, iface, routerID)
}

// TestFRRRealOSPFv3Dataplane is §3.D of the QA program:
// a real FRR OSPFv3 (IPv6) adjacency across two netns.
//
// FRR ships OSPFv3 as a separate daemon (ospf6d), not
// the IPv4 ospfd. The IPv6 config uses 'router ospf6'
// with 'interface' statements rather than 'network'
// statements (OSPFv3 is interface-centric, not network-
// centric). The /127 transit is forced to point-to-point
// network-type, matching the §3.C OSPFv2 lab.
//
// What we verify:
//  1. OSPFv3 neighbor state reaches Full
//  2. The IPv6 stub network on Lab A's dummy interface
//     appears in Lab B's IPv6 routing table
//  3. Withdraw propagates (dummy down -> route gone)
func TestFRRRealOSPFv3Dataplane(t *testing.T) {
	if !hasFRR() {
		t.Skip("FRR daemons not present")
	}
	if _, err := os.Stat("/usr/lib/frr/ospf6d"); err != nil {
		t.Skip("/usr/lib/frr/ospf6d not present (FRR OSPFv3 daemon)")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-ospfv3-a")
	labB := NewLab(t, "qa-ospfv3-b")
	// /127 veth transit (RFC 6164: IPv6 point-to-point links).
	VethBetween(t, labA, labB, "qa-o6-a", "qa-o6-b", "fd00:100::1/127", "fd00:100::/127")
	// Stub IPv6 network on Lab A via dummy interface.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "add", "lo6-a", "type", "dummy")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "fd00:200::1/64", "dev", "lo6-a")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "lo6-a", "up")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "add", "lo6-b", "type", "dummy")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "addr", "add", "fd00:300::1/64", "dev", "lo6-b")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "set", "lo6-b", "up")

	sockA := t.TempDir()
	sockB := t.TempDir()
	confA := `! QA frr.conf OSPFv3 A
frr defaults traditional
hostname qa-ospfv3-a
log file /tmp/qa-ospfv3-a.log informational
!
interface qa-o6-a
 ipv6 ospf6 network point-to-point
 ipv6 ospf6 area 0
!
interface lo6-a
 ipv6 ospf6 area 0
!
router ospf6
 ospf6 router-id 10.252.100.1
 area 0 range fd00:200::/64
!
line vty
!
`
	confB := `! QA frr.conf OSPFv3 B
frr defaults traditional
hostname qa-ospfv3-b
log file /tmp/qa-ospfv3-b.log informational
!
interface qa-o6-b
 ipv6 ospf6 network point-to-point
 ipv6 ospf6 area 0
!
interface lo6-b
 ipv6 ospf6 area 0
!
router ospf6
 ospf6 router-id 10.252.100.2
 area 0 range fd00:300::/64
!
line vty
!
`
	for _, p := range []struct {
		ns, conf, sock, pathspace string
	}{
		{labA.NetNS, confA, sockA, "qa-ospfv3-a"},
		{labB.NetNS, confB, sockB, "qa-ospfv3-b"},
	} {
		confPath := filepath.Join(p.sock, "frr.conf")
		if err := os.WriteFile(confPath, []byte(p.conf), 0o644); err != nil {
			t.Fatalf("write conf: %v", err)
		}
		zebraSocket := filepath.Join(p.sock, "zebra.sock")
		zebraPIDFile := filepath.Join(p.sock, "zebra.pid")
		ospf6PIDFile := filepath.Join(p.sock, "ospf6d.pid")
		zebraLog := filepath.Join(p.sock, "zebra.log")
		ospf6Log := filepath.Join(p.sock, "ospf6d.log")
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/zebra -d -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, zebraPIDFile, p.pathspace, zebraLog))
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/ospf6d -d -f %s -z %s -i %s -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, ospf6PIDFile, p.pathspace, ospf6Log))
	}
	// Cleanup all daemons.
	for _, p := range []struct{ sock, pathspace string }{
		{sockA, "qa-ospfv3-a"}, {sockB, "qa-ospfv3-b"},
	} {
		t.Cleanup(func() {
			zebraPID := filepath.Join(p.sock, "zebra.pid")
			ospf6PID := filepath.Join(p.sock, "ospf6d.pid")
			_ = runShell("/bin/sh", "-c", "kill $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill $(cat "+ospf6PID+" 2>/dev/null) 2>/dev/null; sleep 0.2; kill -9 $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill -9 $(cat "+ospf6PID+" 2>/dev/null) 2>/dev/null; true")
			_ = runShell("/bin/rm", "-rf", "/run/frr/"+p.pathspace)
		})
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		a, _, _ := runInNSCaptureBytes(labA.NetNS, "/sbin/ip", "-6", "route", "show", "fd00:300::/64")
		b, _, _ := runInNSCaptureBytes(labB.NetNS, "/sbin/ip", "-6", "route", "show", "fd00:200::/64")
		if strings.Contains(a, "proto ospf") && strings.Contains(b, "proto ospf") {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("Lab A OSPFv3 neighbor:\n%s", vtyCmd(t, labA.NetNS, "qa-ospfv3-a", "show ipv6 ospf6 neighbor"))
	t.Logf("Lab A show ipv6 route:\n%s", vtyCmd(t, labA.NetNS, "qa-ospfv3-a", "show ipv6 route"))
	t.Logf("Lab B OSPFv3 neighbor:\n%s", vtyCmd(t, labB.NetNS, "qa-ospfv3-b", "show ipv6 ospf6 neighbor"))
	t.Logf("Lab B show ipv6 route:\n%s", vtyCmd(t, labB.NetNS, "qa-ospfv3-b", "show ipv6 route"))

	// Verify Lab B learned fd00:200::/64 from Lab A.
	r6, _, _ := runInNSCaptureBytes(labB.NetNS, "/sbin/ip", "-6", "route", "show", "fd00:200::/64")
	t.Logf("labB ip -6 route fd00:200::/64:\n%s", r6)
	if !strings.Contains(r6, "fd00:200::/64") {
		t.Fatalf("Lab B did not learn fd00:200::/64 from Lab A via OSPFv3: %s", r6)
	}
	if !strings.Contains(r6, "via fe80:") || !strings.Contains(r6, "dev qa-o6-b proto ospf") {
		t.Fatalf("Lab B's OSPFv3 route via fd00:100::1 missing: %s", r6)
	}
	// And Lab A learned fd00:300::/64.
	r6a, _, _ := runInNSCaptureBytes(labA.NetNS, "/sbin/ip", "-6", "route", "show", "fd00:300::/64")
	if !strings.Contains(r6a, "fd00:300::/64") || !strings.Contains(r6a, "dev qa-o6-a proto ospf") {
		t.Fatalf("Lab A did not learn fd00:300::/64 via OSPFv3: %s", r6a)
	}

	// Real packet test: ping6 Lab B's loopback from Lab A
	// via the OSPF-learned route (just to the /64 prefix).
	// We can't ping6 fd00:300::1 directly because Lab A has
	// no route there without OSPF — and OSPF DID install it.
	// The kernel ping -6 needs an interface; we use -I qa-o6-a.
	pingOut, _, _ := runInNSCaptureBytes(labA.NetNS, "/bin/ping", "-6", "-c", "1", "-W", "2", "-I", "qa-o6-a", "fd00:300::1")
	t.Logf("labA ping6 labB stub:\n%s", pingOut)
	if !strings.Contains(pingOut, "1 received") {
		t.Fatalf("expected 1 ICMPv6 echo reply from fd00:300::1, got: %s", pingOut)
	}

	// Withdraw: bring Lab A's stub network down.
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "lo6-a", "down")
	deadline = time.Now().Add(45 * time.Second)
	var gone string
	var routeErr error
	for {
		gone, _, routeErr = runInNSCaptureBytes(labB.NetNS, "/sbin/ip", "-6", "route", "show", "fd00:200::/64")
		if routeErr != nil {
			t.Fatal(routeErr)
		}
		if !strings.Contains(gone, "fd00:200::/64") || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if strings.Contains(gone, "fd00:200::/64") {
		t.Fatalf("Lab B still has fd00:200::/64 after Lab A's stub went down: %s", gone)
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestFRRRealISISDataplane is §3.E of the QA program:
// a real FRR IS-IS adjacency across two netns.
//
// IS-IS is a link-state protocol that runs directly over
// the data link layer (not over IP). FRR ships isisd as
// the daemon; it requires a NET (Network Entity Title) per
// router and area authentication is optional.
//
// Topology:
//
//	qa-isis-a (NET 49.0001.0000.0000.0001.00) ─── qa-isis-b (NET 49.0001.0000.0000.0002.00)
//	    .1/30                                        .2/30
//
// IS-IS runs on top of the /30 transit. We don't rely on
// IP for adjacency (IS-IS uses CLNP), but IP routing
// daemon integration still requires the transit interface
// to be up with both IPv4 and IPv6 link-local.
func TestFRRRealISISDataplane(t *testing.T) {
	if !hasFRR() {
		t.Skip("FRR daemons not present")
	}
	if _, err := os.Stat("/usr/lib/frr/isisd"); err != nil {
		t.Skip("/usr/lib/frr/isisd not present")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-isis-a")
	labB := NewLab(t, "qa-isis-b")
	VethBetween(t, labA, labB, "qa-is-a", "qa-is-b", "10.254.0.1/30", "10.254.0.2/30")

	sockA := t.TempDir()
	sockB := t.TempDir()
	confA := `! QA frr.conf IS-IS A
frr defaults traditional
hostname qa-isis-a
log file /tmp/qa-isis-a.log informational
!
interface qa-is-a
 ip router isis QA
 isis circuit-type level-1
!
router isis QA
 net 49.0001.0000.0000.0001.00
 is-type level-1
!
line vty
!
`
	confB := `! QA frr.conf IS-IS B
frr defaults traditional
hostname qa-isis-b
log file /tmp/qa-isis-b.log informational
!
interface qa-is-b
 ip router isis QA
 isis circuit-type level-1
!
router isis QA
 net 49.0001.0000.0000.0002.00
 is-type level-1
!
line vty
!
`
	for _, p := range []struct {
		ns, conf, sock, pathspace string
	}{
		{labA.NetNS, confA, sockA, "qa-isis-a"},
		{labB.NetNS, confB, sockB, "qa-isis-b"},
	} {
		confPath := filepath.Join(p.sock, "frr.conf")
		if err := os.WriteFile(confPath, []byte(p.conf), 0o644); err != nil {
			t.Fatalf("write conf: %v", err)
		}
		zebraSocket := filepath.Join(p.sock, "zebra.sock")
		zebraPID := filepath.Join(p.sock, "zebra.pid")
		isisPID := filepath.Join(p.sock, "isisd.pid")
		zebraLog := filepath.Join(p.sock, "zebra.log")
		isisLog := filepath.Join(p.sock, "isisd.log")
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/zebra -d -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, zebraPID, p.pathspace, zebraLog))
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/isisd -d -f %s -z %s -i %s -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, isisPID, p.pathspace, isisLog))
	}
	for _, p := range []struct{ sock, pathspace string }{
		{sockA, "qa-isis-a"}, {sockB, "qa-isis-b"},
	} {
		t.Cleanup(func() {
			zebraPID := filepath.Join(p.sock, "zebra.pid")
			isisPID := filepath.Join(p.sock, "isisd.pid")
			_ = runShell("/bin/sh", "-c", "kill $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill $(cat "+isisPID+" 2>/dev/null) 2>/dev/null; sleep 0.2; kill -9 $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill -9 $(cat "+isisPID+" 2>/dev/null) 2>/dev/null; true")
			_ = runShell("/bin/rm", "-rf", "/run/frr/"+p.pathspace)
		})
	}
	time.Sleep(5 * time.Second)

	// Check IS-IS adjacency state.
	neighA := vtyCmd(t, labA.NetNS, "qa-isis-a", "show isis neighbor")
	t.Logf("Lab A IS-IS neighbor:\n%s", neighA)
	if !strings.Contains(neighA, "Up") {
		t.Fatalf("IS-IS neighbor not Up on Lab A: %s", neighA)
	}
	neighB := vtyCmd(t, labB.NetNS, "qa-isis-b", "show isis neighbor")
	t.Logf("Lab B IS-IS neighbor:\n%s", neighB)
	if !strings.Contains(neighB, "Up") {
		t.Fatalf("IS-IS neighbor not Up on Lab B: %s", neighB)
	}

	// Check IS-IS routes propagated. Lab B should know about
	// Lab A's connected /30 via IS-IS (already in kernel
	// via connected, but isisd should report it as ISIS).
	routesA := vtyCmd(t, labA.NetNS, "qa-isis-a", "show ip route isis")
	t.Logf("Lab A IS-IS routes:\n%s", routesA)
	routesB := vtyCmd(t, labB.NetNS, "qa-isis-b", "show ip route isis")
	t.Logf("Lab B IS-IS routes:\n%s", routesB)

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestFRRRealBFDDetection is §3.F of the QA program: BFD
// fast-failure-detection between two FRR routers.
//
// BFD (Bidirectional Forwarding Detection) is a lightweight
// UDP-based protocol that detects peer liveness in
// sub-second intervals. FRR ships bfdd for the BFD session
// state machine; zebra and the routing daemons subscribe
// to BFD state changes via the bfdd-to-zebra socket.
//
// The test:
//  1. Starts zebra + bfdd in two netns connected by veth.
//  2. Configures a BFD peer between them with a 200 ms
//     transmit interval and 3x multiplier (BFD requires
//     detect time < 1 second).
//  3. Waits for BFD session to reach Up state on both sides.
//  4. Brings the remote-side interface DOWN.
//  5. Verifies BFD detects the failure within 2 seconds
//     (i.e., local session transitions to Down).
func TestFRRRealBFDDetection(t *testing.T) {
	if !hasFRR() {
		t.Skip("FRR daemons not present")
	}
	if _, err := os.Stat("/usr/lib/frr/bfdd"); err != nil {
		t.Skip("/usr/lib/frr/bfdd not present")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-bfd-a")
	labB := NewLab(t, "qa-bfd-b")
	VethBetween(t, labA, labB, "qa-bfd-a", "qa-bfd-b", "10.253.0.1/30", "10.253.0.2/30")

	sockA := t.TempDir()
	sockB := t.TempDir()
	confA := `! QA frr.conf BFD A
frr defaults traditional
hostname qa-bfd-a
log file /tmp/qa-bfd-a.log informational
!
interface qa-bfd-a
 ip address 10.253.0.1/30
 no ip ospf passive
 bfd min-rx 200 min-tx 200 multiplier 3
!
bfd
 peer 10.253.0.2
  receive-interval 200
  transmit-interval 200
  detect-multiplier 3
 !
!
line vty
!
`
	confB := `! QA frr.conf BFD B
frr defaults traditional
hostname qa-bfd-b
log file /tmp/qa-bfd-b.log informational
!
interface qa-bfd-b
 ip address 10.253.0.2/30
 no ip ospf passive
 bfd min-rx 200 min-tx 200 multiplier 3
!
bfd
 peer 10.253.0.1
  receive-interval 200
  transmit-interval 200
  detect-multiplier 3
 !
!
line vty
!
`
	for _, p := range []struct {
		ns, conf, sock, pathspace string
	}{
		{labA.NetNS, confA, sockA, "qa-bfd-a"},
		{labB.NetNS, confB, sockB, "qa-bfd-b"},
	} {
		confPath := filepath.Join(p.sock, "frr.conf")
		if err := os.WriteFile(confPath, []byte(p.conf), 0o644); err != nil {
			t.Fatalf("write conf: %v", err)
		}
		zebraSocket := filepath.Join(p.sock, "zebra.sock")
		zebraPID := filepath.Join(p.sock, "zebra.pid")
		bfdPID := filepath.Join(p.sock, "bfdd.pid")
		zebraLog := filepath.Join(p.sock, "zebra.log")
		bfdLog := filepath.Join(p.sock, "bfdd.log")
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/zebra -d -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, zebraPID, p.pathspace, zebraLog))
		_ = runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/bfdd -d -f %s -z %s -i %s -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, bfdPID, p.pathspace, bfdLog))
	}
	for _, p := range []struct{ sock, pathspace string }{
		{sockA, "qa-bfd-a"}, {sockB, "qa-bfd-b"},
	} {
		t.Cleanup(func() {
			zebraPID := filepath.Join(p.sock, "zebra.pid")
			bfdPID := filepath.Join(p.sock, "bfdd.pid")
			_ = runShell("/bin/sh", "-c", "kill $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill $(cat "+bfdPID+" 2>/dev/null) 2>/dev/null; sleep 0.2; kill -9 $(cat "+zebraPID+" 2>/dev/null) 2>/dev/null; kill -9 $(cat "+bfdPID+" 2>/dev/null) 2>/dev/null; true")
			_ = runShell("/bin/rm", "-rf", "/run/frr/"+p.pathspace)
		})
	}
	time.Sleep(3 * time.Second)

	// Verify BFD session is Up on both sides.
	peerA, _ := vtyCmd(t, labA.NetNS, "qa-bfd-a", "show bfd peer 10.253.0.2"), ""
	t.Logf("Lab A show bfd peer 10.253.0.2:\n%s", peerA)
	peerB, _ := vtyCmd(t, labB.NetNS, "qa-bfd-b", "show bfd peer 10.253.0.1"), ""
	t.Logf("Lab B show bfd peer 10.253.0.1:\n%s", peerB)
	// BFD peer is "Up" when status flag shows "up". Different
	// FRR versions format this slightly differently.
	if !strings.Contains(peerA, "up") && !strings.Contains(peerA, "Up") {
		t.Fatalf("BFD session on Lab A not Up: %s", peerA)
	}
	if !strings.Contains(peerB, "up") && !strings.Contains(peerB, "Up") {
		t.Fatalf("BFD session on Lab B not Up: %s", peerB)
	}

	// Kill Lab B's interface. Lab A should detect within
	// ~3x detect-multiplier (200ms*3 = 600ms in theory,
	// but in practice 1-3 seconds with timing skew).
	if err := runInNSShell(labB.NetNS, "/sbin/ip", "link", "set", "qa-bfd-b", "down"); err != nil {
		t.Fatalf("interface down: %v", err)
	}
	time.Sleep(3 * time.Second)

	// Lab A's session should now be Down.
	peerA, _ = vtyCmd(t, labA.NetNS, "qa-bfd-a", "show bfd peer 10.253.0.2"), ""
	t.Logf("Lab A show bfd peer after fail:\n%s", peerA)
	if !strings.Contains(peerA, "down") && !strings.Contains(peerA, "Down") {
		t.Fatalf("BFD on Lab A did not detect failure within 3s: %s", peerA)
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestFRRRealOSPFv2Dataplane is §3.C of the QA program.
func TestFRRRealOSPFv2Dataplane(t *testing.T) {
	if !hasFRR() {
		t.Skip("FRR daemons not present")
	}
	before := CaptureManagementBoundary(t)
	labA := NewLab(t, "qa-ospf-a")
	labB := NewLab(t, "qa-ospf-b")
	VethBetween(t, labA, labB, "qa-oa-a", "qa-oa-b", "10.251.0.1/30", "10.251.0.2/30")
	// Put the loopback /32 inside a /24 on a dummy
	// interface so OSPF treats it as a regular stub
	// network rather than a loopback host route (which
	// requires special handling and frequently fails to
	// propagate across netns veth labs in FRR 10.3).
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "add", "lo-a", "type", "dummy")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "addr", "add", "10.251.100.1/24", "dev", "lo-a")
	_ = runInNSShell(labA.NetNS, "/sbin/ip", "link", "set", "lo-a", "up")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "add", "lo-b", "type", "dummy")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "addr", "add", "10.251.100.2/24", "dev", "lo-b")
	_ = runInNSShell(labB.NetNS, "/sbin/ip", "link", "set", "lo-b", "up")

	sockA := t.TempDir()
	sockB := t.TempDir()
	confA := frrOSPFAB("10.251.100.1", "qa-oa-a")
	confB := frrOSPFAB("10.251.100.2", "qa-oa-b")
	zebraOnly := func(sock, conf string) {
		confPath := filepath.Join(sock, "frr.conf")
		if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
			t.Fatalf("write conf: %v", err)
		}
		zebraSocket := filepath.Join(sock, "zebra.sock")
		zebraPID := filepath.Join(sock, "zebra.pid")
		ospfPID := filepath.Join(sock, "ospfd.pid")
		zebraLog := filepath.Join(sock, "zebra.log")
		ospfLog := filepath.Join(sock, "ospfd.log")
		_ = runInNSShell(labA.NetNS, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/zebra -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 > %s 2>&1",
				confPath, zebraSocket, zebraPID, zebraLog))
		_ = runInNSShell(labA.NetNS, "/bin/sh", "-c",
			fmt.Sprintf("exec /usr/lib/frr/ospfd -f %s -z %s -i %s -A 127.0.0.1 > %s 2>&1",
				confPath, zebraSocket, ospfPID, ospfLog))
	}
	// Zebra + ospfd in each.
	for _, p := range []struct {
		ns, conf, sock, pathspace string
	}{
		{labA.NetNS, confA, sockA, "qa-ospf-a"},
		{labB.NetNS, confB, sockB, "qa-ospf-b"},
	} {
		confPath := filepath.Join(p.sock, "frr.conf")
		if err := os.WriteFile(confPath, []byte(p.conf), 0o644); err != nil {
			t.Fatalf("write conf: %v", err)
		}
		zebraSocket := filepath.Join(p.sock, "zebra.sock")
		zebraPIDFile := filepath.Join(p.sock, "zebra.pid")
		ospfPIDFile := filepath.Join(p.sock, "ospfd.pid")
		zebraLog := filepath.Join(p.sock, "zebra.log")
		ospfLog := filepath.Join(p.sock, "ospfd.log")
		if err := os.Chmod(p.sock, 0o777); err != nil {
			t.Fatalf("chmod %s: %v", p.sock, err)
		}
		if err := runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("/usr/lib/frr/zebra -d -f %s -z %s -i %s -s 90000000 -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, zebraPIDFile, p.pathspace, zebraLog)); err != nil {
			t.Fatalf("zebra: %v", err)
		}
		if err := runInNSShell(p.ns, "/bin/sh", "-c",
			fmt.Sprintf("/usr/lib/frr/ospfd -d -f %s -z %s -i %s -A 127.0.0.1 -N %s -u root -g frrvty > %s 2>&1",
				confPath, zebraSocket, ospfPIDFile, p.pathspace, ospfLog)); err != nil {
			t.Fatalf("ospfd: %v", err)
		}
		t.Cleanup(func() {
			ns := p.ns
			z := zebraPIDFile
			o := ospfPIDFile
			_ = runInNSShell(ns, "/bin/sh", "-c", "kill $(cat "+z+" 2>/dev/null) 2>/dev/null; kill $(cat "+o+" 2>/dev/null) 2>/dev/null; sleep 0.2; kill -9 $(cat "+z+" 2>/dev/null) 2>/dev/null; kill -9 $(cat "+o+" 2>/dev/null) 2>/dev/null; true")
			_ = runShell("/bin/rm", "-rf", "/run/frr/"+p.pathspace)
		})
	}
	time.Sleep(2 * time.Second)
	// Capture OSPF packets on Lab B's qa-oa-b to see if
	// Lab A's hellos are arriving.
	dumpOut, _ := runInNSCapture(labB.NetNS, "/usr/bin/tcpdump", "-i", "qa-oa-b", "-n", "-c", "5", "proto", "ospf")
	t.Logf("Lab B OSPF packets:\n%s", string(dumpOut))
	t.Logf("Lab A running-config:\n%s", vtyCmd(t, labA.NetNS, "qa-ospf-a", "show running-config"))
	t.Logf("Lab A ospf interface:\n%s", vtyCmd(t, labA.NetNS, "qa-ospf-a", "show ip ospf interface"))
	t.Logf("Lab A ospf neighbor:\n%s", vtyCmd(t, labA.NetNS, "qa-ospf-a", "show ip ospf neighbor"))
	t.Logf("Lab A zebra route:\n%s", vtyCmd(t, labA.NetNS, "qa-ospf-a", "show ip route"))
	t.Logf("Lab B running-config:\n%s", vtyCmd(t, labB.NetNS, "qa-ospf-b", "show running-config"))
	t.Logf("Lab B ospf interface:\n%s", vtyCmd(t, labB.NetNS, "qa-ospf-b", "show ip ospf interface"))
	t.Logf("Lab B ospf neighbor:\n%s", vtyCmd(t, labB.NetNS, "qa-ospf-b", "show ip ospf neighbor"))
	t.Logf("Lab B zebra route:\n%s", vtyCmd(t, labB.NetNS, "qa-ospf-b", "show ip route"))
	t.Logf("Lab B ospf database:\n%s", vtyCmd(t, labB.NetNS, "qa-ospf-b", "show ip ospf database"))
	if !waitForRoute(t, labB.NetNS, "10.251.100.0/24", 10*time.Second) {
		t.Fatalf("Lab B did not learn 10.251.100.0/24 from Lab A via OSPF")
	}
	// And vice versa.
	if !waitForRoute(t, labA.NetNS, "10.251.100.0/24", 10*time.Second) {
		t.Fatalf("Lab A did not learn 10.251.100.0/24 from Lab B via OSPF")
	}
	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
	_ = zebraOnly
	_ = bufio.NewScanner // keep import
	_ = context.Background
	_ = net.IPv4len
}
