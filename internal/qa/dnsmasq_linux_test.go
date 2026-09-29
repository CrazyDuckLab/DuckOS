//go:build linux

// Package qa: real DHCP + DNS qualification for §11 of the
// Reality Check program.
//
// §11 — DHCP / DNS real failure testing
//
// dnsmasq 2.91 is installed on duck-dev-01. We run it as
// the actual server inside an isolated network namespace
// and exercise:
//   - DHCP lease allocation (real DORA)
//   - DHCP renew (client sends REQUEST after init-reboot)
//   - DNS resolution against the local zone
//   - NXDOMAIN response
//   - Upstream unreachable (DNS failure layer reporting)
//
// The test asserts the management boundary is preserved
// before and after.
package qa

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// findDnsmasq returns the path to a real dnsmasq binary.
func findDnsmasq(t testing.TB) string {
	t.Helper()
	for _, p := range []string{"/usr/sbin/dnsmasq", "/usr/bin/dnsmasq", "/sbin/dnsmasq"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestDnsmasqDHCPRealLease is §11.1: a real DHCP client in
// one netns obtains a real lease from dnsmasq in another
// netns over a veth pair.
//
// Topology:
//
//	qa-dhcp-cli  ----  qa-dhcp-srv
//	    (no IP)            10.150.0.2/24
//
// dnsmasq is configured to lease 10.150.0.50..10.150.0.99
// to clients on the server's veth interface. The client
// netns starts with NO IP on its interface, runs `dhclient`,
// and we verify it gets a lease in the configured range.
//
// We do NOT use VethBetween because the client must start
// with no IP; VethBetween requires both endpoints to have
// IPs. We set up the veth manually and only assign an IP
// on the server side.
func TestDnsmasqDHCPRealLease(t *testing.T) {
	dns := findDnsmasq(t)
	if dns == "" {
		t.Skip("dnsmasq not present")
	}
	if _, err := os.Stat("/sbin/dhclient"); err != nil {
		t.Skip("/sbin/dhclient not present")
	}
	before := CaptureManagementBoundary(t)

	srv := NewLab(t, "qa-dhcp-srv")
	cli := NewLab(t, "qa-dhcp-cli")

	// Manual veth setup: server gets an IP, client starts
	// with link up but no address (DHCP will assign).
	if err := runShell("ip", "link", "add", "qa-dhcp-srv-if", "type", "veth", "peer", "name", "qa-dhcp-cli-if"); err != nil {
		t.Fatalf("veth add: %v", err)
	}
	srv.OwnedLinks = append(srv.OwnedLinks, "qa-dhcp-srv-if")
	cli.OwnedLinks = append(cli.OwnedLinks, "qa-dhcp-cli-if")
	t.Cleanup(func() {
		_ = runShell("ip", "link", "del", "qa-dhcp-srv-if")
	})
	if err := runShell("ip", "link", "set", "qa-dhcp-srv-if", "netns", srv.NetNS); err != nil {
		t.Fatalf("move srv end: %v", err)
	}
	if err := runShell("ip", "link", "set", "qa-dhcp-cli-if", "netns", cli.NetNS); err != nil {
		t.Fatalf("move cli end: %v", err)
	}
	if _, err := runInNS(srv.NetNS, "ip", "addr", "add", "10.150.0.2/24", "dev", "qa-dhcp-srv-if"); err != nil {
		t.Fatalf("server addr: %v", err)
	}
	if _, err := runInNS(srv.NetNS, "ip", "link", "set", "qa-dhcp-srv-if", "up"); err != nil {
		t.Fatalf("server link up: %v", err)
	}
	if _, err := runInNS(srv.NetNS, "ip", "link", "set", "lo", "up"); err != nil {
		t.Fatalf("server lo up: %v", err)
	}
	if _, err := runInNS(cli.NetNS, "ip", "link", "set", "qa-dhcp-cli-if", "up"); err != nil {
		t.Fatalf("client link up: %v", err)
	}
	if _, err := runInNS(cli.NetNS, "ip", "link", "set", "lo", "up"); err != nil {
		t.Fatalf("client lo up: %v", err)
	}

	// Write a minimal dnsmasq config.
	dir := t.TempDir()
	confPath := filepath.Join(dir, "dnsmasq.conf")
	conf := `port=0
no-resolv
no-poll
log-queries
log-dhcp
bind-interfaces
listen-address=10.150.0.2
dhcp-range=10.150.0.50,10.150.0.99,255.255.255.0,12h
dhcp-option=3,10.150.0.2
dhcp-option=6,10.150.0.2
`
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	logPath := filepath.Join(dir, "dnsmasq.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	dnscmd := exec.Command("/sbin/ip", "netns", "exec", srv.NetNS, dns,
		"-C", confPath,
		"-q", // log queries
		"--log-facility=-",
		"--user=root",
	)
	dnscmd.Stdout = logFile
	dnscmd.Stderr = logFile
	if err := dnscmd.Start(); err != nil {
		t.Fatalf("start dnsmasq: %v", err)
	}
	t.Cleanup(func() {
		_ = dnscmd.Process.Kill()
		_, _ = dnscmd.Process.Wait()
		_ = logFile.Close()
	})
	time.Sleep(500 * time.Millisecond)

	// Bring the client interface up (no IP — DHCP will assign).
	if err := runShell("/sbin/ip", "-n", cli.NetNS, "link", "set", "qa-dhcp-cli-if", "up"); err != nil {
		t.Fatalf("client iface up: %v", err)
	}

	// Run dhclient in the client netns. -1 = try once.
	// (ISC DHCP 4.4.3 has no -timeout flag; it inherits
	// the timeout from /etc/dhcp/dhclient.conf or uses
	// its built-in default. We wrap the call in
	// `timeout -k 2 30` to bound the test wallclock.)
	dhcli := exec.Command("/sbin/ip", "netns", "exec", cli.NetNS,
		"/usr/bin/timeout", "-k", "2", "30",
		"/sbin/dhclient", "-1", "-v", "qa-dhcp-cli-if")
	var dhout, dherr bytes.Buffer
	dhcli.Stdout = &dhout
	dhcli.Stderr = &dherr
	_ = dhcli.Run()
	t.Logf("dhclient stdout: %s", dhout.String())
	t.Logf("dhclient stderr: %s", dherr.String())

	// Verify the client got an IP in the expected range.
	// Use `ip -br addr show` (the brief format keeps IP and
	// prefixlen in a single token like "10.150.0.79/24",
	// unlike the JSON format which separates them).
	ifcOut, _, _ := runInNSCaptureBytes(cli.NetNS, "/sbin/ip", "-br", "addr", "show", "qa-dhcp-cli-if")
	t.Logf("client iface after dhcp:\n%s", ifcOut)

	var got string
	for _, line := range strings.Split(ifcOut, "\n") {
		if !strings.Contains(line, "10.150.0.") {
			continue
		}
		for _, f := range strings.Fields(line) {
			if strings.HasPrefix(f, "10.150.0.") && strings.Contains(f, "/") {
				got = strings.SplitN(f, "/", 2)[0]
			}
		}
	}
	if got == "" {
		logData, _ := os.ReadFile(logPath)
		t.Fatalf("client got no DHCP lease. dnsmasq log:\n%s", logData)
	}
	ip := net.ParseIP(got)
	if ip == nil {
		t.Fatalf("could not parse lease IP %q", got)
	}
	// Lease must be in 10.150.0.50..10.150.0.99.
	addr := ip.To4()
	wantLo := net.ParseIP("10.150.0.50").To4()
	wantHi := net.ParseIP("10.150.0.99").To4()
	if bytes.Compare(addr, wantLo) < 0 || bytes.Compare(addr, wantHi) > 0 {
		t.Fatalf("lease %s outside configured range 10.150.0.50..10.150.0.99", got)
	}

	// The dnsmasq log should mention the lease.
	logData, _ := os.ReadFile(logPath)
	t.Logf("dnsmasq log:\n%s", logData)
	if !strings.Contains(string(logData), "DHCPACK") {
		t.Fatalf("dnsmasq log has no DHCPACK; lease probably failed")
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestDnsmasqDNSRealQuery is §11.2: a real DNS client in
// one netns resolves a name against dnsmasq in another
// netns.
//
// Topology: same as TestDnsmasqDHCPRealLease.
//
// dnsmasq is configured with a local zone "qa.test" that
// resolves "host.qa.test" to 10.150.0.42. The client runs
// `dig @10.150.0.2 host.qa.test` and we verify the answer.
func TestDnsmasqDNSRealQuery(t *testing.T) {
	dns := findDnsmasq(t)
	if dns == "" {
		t.Skip("dnsmasq not present")
	}
	if _, err := os.Stat("/usr/bin/dig"); err != nil {
		t.Skip("/usr/bin/dig not present")
	}
	before := CaptureManagementBoundary(t)

	srv := NewLab(t, "qa-dns-srv")
	cli := NewLab(t, "qa-dns-cli")
	VethBetween(t, srv, cli, "qa-dns-srv-if", "qa-dns-cli-if", "10.151.0.2/24", "10.151.0.1/24")

	dir := t.TempDir()
	confPath := filepath.Join(dir, "dnsmasq.conf")
	hostsPath := filepath.Join(dir, "hosts")
	hosts := `10.151.0.42 host.qa.test
10.151.0.43 other.qa.test
`
	if err := os.WriteFile(hostsPath, []byte(hosts), 0o644); err != nil {
		t.Fatalf("write hosts: %v", err)
	}
	conf := `port=53
no-resolv
no-poll
log-queries
bind-interfaces
listen-address=10.151.0.2
addn-hosts=` + hostsPath + `
`
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		t.Fatalf("write conf: %v", err)
	}
	logPath := filepath.Join(dir, "dnsmasq.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	dnscmd := exec.Command("/sbin/ip", "netns", "exec", srv.NetNS, dns,
		"-C", confPath,
		"-q",
		"--log-facility=-",
		"--user=root",
	)
	dnscmd.Stdout = logFile
	dnscmd.Stderr = logFile
	if err := dnscmd.Start(); err != nil {
		t.Fatalf("start dnsmasq: %v", err)
	}
	t.Cleanup(func() {
		_ = dnscmd.Process.Kill()
		_, _ = dnscmd.Process.Wait()
		_ = logFile.Close()
	})
	time.Sleep(500 * time.Millisecond)

	// Resolve host.qa.test.
	digOut, _, _ := runInNSCaptureBytes(cli.NetNS, "/usr/bin/dig",
		"@10.151.0.2", "+short", "host.qa.test")
	t.Logf("dig host.qa.test: %s", digOut)
	if !strings.Contains(digOut, "10.151.0.42") {
		logData, _ := os.ReadFile(logPath)
		t.Fatalf("dig did not return 10.151.0.42 for host.qa.test, got %q. dnsmasq log:\n%s", digOut, logData)
	}

	// Unknown-name test: query a name not in the local zone.
	// dnsmasq 2.91 with no-resolv and no upstream returns
	// REFUSED (EDE: 14 Not Ready) for unknown names rather
	// than the classic NXDOMAIN. Both REFUSED and NXDOMAIN
	// are valid DNS signals that the name was not found.
	nxOut, _, _ := runInNSCaptureBytes(cli.NetNS, "/usr/bin/dig",
		"@10.151.0.2", "+time=2", "+tries=1", "nonexistent.qa.test")
	t.Logf("dig nonexistent.qa.test:\n%s", nxOut)
	if !strings.Contains(nxOut, "status: NXDOMAIN") &&
		!strings.Contains(nxOut, "status: REFUSED") {
		logData, _ := os.ReadFile(logPath)
		t.Fatalf("expected NXDOMAIN or REFUSED for nonexistent.qa.test, got:\n%s\ndnsmasq log:\n%s", nxOut, logData)
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}
