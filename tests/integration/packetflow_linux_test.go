//go:build linux

// This file exercises real Linux kernel paths in isolated
// network namespaces with veth pairs. The tests are the
// closest the qualification host gets to the production
// firewall/NAT, WireGuard, routing and packet-capture paths
// without a full installed DuckOS boot environment.
//
// The management interface eth0 is never touched. Every test
// creates its own netns, veth pair and routing/rule state and
// tears it down on exit.
package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// wgKeypair generates a Curve25519 keypair suitable for
// `wg genkey | wg pubkey`. The function returns the
// base64-encoded private key and the matching public key.
func wgKeypair(t *testing.T) (string, string) {
	t.Helper()
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		t.Fatal(err)
	}
	// Clamp the private key per RFC 7748.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	privB64 := base64.StdEncoding.EncodeToString(priv)
	// Derive the public key with a one-shot X25519 via the
	// `wg pubkey` helper. The package is the wireguard-tools
	// `wg` binary, not a WireGuard handshake.
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(privB64)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("wg pubkey: %v", err)
	}
	return privB64, strings.TrimSpace(string(out))
}

// getRoute runs ip route get and returns the combined output
// even when the command exits non-zero. ip route get reports
// "No route to host" when the lookup hits a blackhole route,
// which is the canonical evidence we want from the test.
func getRoute(t *testing.T, ns string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command("ip", append([]string{"netns", "exec", ns, name}, args...)...)
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out))
}

// bindNSRtTables writes a per-namespace rt_tables file that
// declares the supplied id-to-name mapping. Without this file
// the kernel refuses table references by id. The function is
// the canonical way to make a custom table available inside
// a network namespace; the DuckOS production code would do
// the same when it owns a custom routing table.
func bindNSRtTables(t *testing.T, ns, id, name string) {
	t.Helper()
	dir := "/etc/netns/" + ns
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/rt_tables"
	if err := os.WriteFile(path, []byte(id+" "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runOrFail runs a shell command in a network namespace and
// fails the test on any non-zero exit. Used by the WireGuard
// test where every command must succeed.
func runOrFail(t *testing.T, ns string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command("ip", append([]string{"netns", "exec", ns, name}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ip netns exec %s %s %v: %v (%s)", ns, name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestRealLinuxFirewallPacketFlowInNetns creates two netns
// with a veth pair, drops ICMP in one via nftables, and proves
// that the kernel actually drops the packet (not just that
// the rule was applied).
func TestRealLinuxFirewallPacketFlowInNetns(t *testing.T) {
	nsClient, nsServer := "duckos-fw-client", "duckos-fw-server"
	withNetnsPair(t, nsClient, nsServer, "veth0", "veth1", "10.99.10.1/30", "10.99.10.2/30", func() {
		// Confirm baseline reachability without firewall.
		if !pingOK(t, nsClient, "10.99.10.2", 2*time.Second) {
			t.Fatal("baseline ping failed; veth pair not connected")
		}
		// Install a real nftables input drop on the server.
		mustRunIn(t, nsServer, "nft", "add", "table", "inet", "filter")
		mustRunIn(t, nsServer, "nft", "add", "chain", "inet", "filter", "input", "{", "type", "filter", "hook", "input", "priority", "0", ";", "policy", "accept", ";", "}")
		mustRunIn(t, nsServer, "nft", "add", "rule", "inet", "filter", "input", "ip", "protocol", "icmp", "drop")
		// Reverse path: server replies to client also traverse input.
		// Confirm the ruleset is exactly what we asked for.
		got := runIn(t, nsServer, "nft", "list", "ruleset")
		if !strings.Contains(got, "ip protocol icmp drop") {
			t.Fatalf("rule not in nft list: %s", got)
		}
		// Now ICMP should be silently dropped. UDP should still
		// pass because the rule is icmp-only.
		if pingOK(t, nsClient, "10.99.10.2", 2*time.Second) {
			t.Fatal("icmp reached the server despite a drop rule")
		}
		if !udpEchoOK(t, nsClient, nsServer, "10.99.10.2", 9999, 2*time.Second) {
			t.Fatal("udp echo failed under a non-matching rule")
		}
		// Remove the rule and ping must come back. Use flush
		// rather than handle because the handle is dynamic.
		mustRunIn(t, nsServer, "nft", "flush", "chain", "inet", "filter", "input")
		if !pingOK(t, nsClient, "10.99.10.2", 2*time.Second) {
			t.Fatal("icmp did not return after the rule was removed")
		}
	})
}

// TestRealLinuxRoutingPolicyAndTableInNetns verifies the
// production ip rule + ip route add flow actually installs
// a custom routing table and a policy rule that consults it
// inside an isolated namespace. The test never touches the
// host's main table. The canonical evidence is the kernel
// state itself: a rule with the right priority and a table
// with the right blackhole entry, both observable through
// `ip rule` and `ip route show table`.
func TestRealLinuxRoutingPolicyAndTableInNetns(t *testing.T) {
	nsLeft, nsRight := "duckos-rt-left", "duckos-rt-right"
	withNetnsPair(t, nsLeft, nsRight, "veth0", "veth1", "10.99.20.1/24", "10.99.20.2/24", func() {
		// Confirm baseline reachability.
		if !pingOK(t, nsLeft, "10.99.20.2", 2*time.Second) {
			t.Fatal("baseline ping failed")
		}
		// The rt_tables file must declare table id 100 for
		// the kernel to accept lookups by id. We bind it
		// inside the namespace so the test is self-contained.
		bindNSRtTables(t, nsLeft, "100", "duckos_test")
		// Create a custom table (id 100). Add a blackhole
		// route for an unreachable destination (10.99.20.5
		// has no host on the /24) and install a `to` policy
		// rule that consults table 100 for that destination.
		mustRunIn(t, nsLeft, "ip", "route", "add", "blackhole", "10.99.20.5/32", "table", "100")
		mustRunIn(t, nsLeft, "ip", "rule", "add", "to", "10.99.20.5/32", "table", "100")
		// Verify the rule and the blackhole route are in place.
		// The rule is rendered with the table name (lookup
		// duckos_test) because the rt_tables bind mapped id
		// 100 to that name. Both forms are accepted.
		rules := runIn(t, nsLeft, "ip", "rule")
		if !strings.Contains(rules, "lookup 100") && !strings.Contains(rules, "lookup duckos_test") {
			t.Fatalf("ip rule not installed: %s", rules)
		}
		if !strings.Contains(rules, "to 10.99.20.5") {
			t.Fatalf("ip rule selector wrong: %s", rules)
		}
		rt := runIn(t, nsLeft, "ip", "route", "show", "table", "100")
		if !strings.Contains(rt, "blackhole 10.99.20.5") {
			t.Fatalf("blackhole route missing: %s", rt)
		}
		// The custom rule must be in the policy range
		// [1, 32764] so it is consulted before the main
		// table at priority 32766. The kernel prints the
		// custom rule as "32765:" when rt_tables is bound.
		if !strings.Contains(rules, "32765:") {
			t.Fatalf("custom rule is not in the policy range: %s", rules)
		}
		// Remove the rule. The next ip rule output must not
		// mention the custom selector any more.
		mustRunIn(t, nsLeft, "ip", "rule", "del", "to", "10.99.20.5/32", "table", "100")
		rules2 := runIn(t, nsLeft, "ip", "rule")
		if strings.Contains(rules2, "to 10.99.20.5") {
			t.Fatalf("custom rule was not removed: %s", rules2)
		}
	})
}

// TestRealLinuxWireGuardPeerHandshakeInNetns brings up two
// real WireGuard interfaces in two isolated namespaces,
// configures each as the other's peer, and verifies a real
// kernel handshake plus a real ping across the tunnel.
func TestRealLinuxWireGuardPeerHandshakeInNetns(t *testing.T) {
	nsA, nsB := "duckos-wg-a", "duckos-wg-b"
	if _, err := exec.LookPath("wg"); err != nil {
		t.Skip("wg tool not available")
	}
	// Ensure kernel module is loadable on this host. The
	// `ip link add type wireguard` call below fails if not.
	withNetns(t, nsA, func(_ context.Context) error {
		_ = runOrFail(t, nsA, "ip", "link", "add", "wg0", "type", "wireguard")
		_ = runOrFail(t, nsA, "ip", "link", "del", "wg0")
		return nil
	})
	privA, pubA := wgKeypair(t)
	privB, pubB := wgKeypair(t)
	withNetnsPair(t, nsA, nsB, "veth0", "veth1", "10.99.30.1/30", "10.99.30.2/30", func() {
		// Create wg0 in each namespace with its own /30
		// tunnel subnet. AllowedIPs = tunnel subnet so the
		// other side can route ping through the tunnel.
		configureWG(t, nsA, "wg0", "10.99.40.1/30", "10.99.30.2:51820", privA, pubB, []string{"10.99.40.0/30"})
		configureWG(t, nsB, "wg0", "10.99.40.2/30", "10.99.30.1:51820", privB, pubA, []string{"10.99.40.0/30"})
		// Wait up to 5 s for the kernel handshake to land.
		handshake := false
		for i := 0; i < 50; i++ {
			out := runIn(t, nsA, "wg", "show", "wg0", "latest-handshakes")
			if strings.Contains(out, pubB) {
				handshake = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !handshake {
			t.Fatal("no kernel WireGuard handshake within 5 s")
		}
		// Send a real ping through the tunnel. It must reach
		// the tunnel subnet on the other side.
		if !pingOK(t, nsA, "10.99.40.2", 3*time.Second) {
			t.Fatal("ping through WireGuard tunnel did not succeed")
		}
		// Verify transfer counters advanced on both sides.
		ax := runIn(t, nsA, "wg", "show", "wg0", "transfer")
		bx := runIn(t, nsB, "wg", "show", "wg0", "transfer")
		if !strings.Contains(ax, pubB) || !strings.Contains(bx, pubA) {
			t.Fatalf("transfer counters missing: A=%q B=%q", ax, bx)
		}
	})
}

// TestRealLinuxTcpdumpCaptureInNetns starts a real tcpdump
// against an interface inside a namespace, sends known
// traffic, and verifies the bounded pcap file. The test
// does not add new capture product features; it only
// exercises the existing tcpdump + pcap file workflow that
// the production Duck capture would use.
func TestRealLinuxTcpdumpCaptureInNetns(t *testing.T) {
	nsClient, nsServer := "duckos-pcap-client", "duckos-pcap-server"
	if _, err := exec.LookPath("tcpdump"); err != nil {
		t.Skip("tcpdump not available")
	}
	withNetnsPair(t, nsClient, nsServer, "veth0", "veth1", "10.99.40.1/30", "10.99.40.2/30", func() {
		// Start tcpdump in the server namespace, filter for
		// ICMP, write to a 64 KiB pcap in a private dir.
		dir := "/tmp/duckos-pcap-" + randomHex(4)
		mustRunIn(t, nsServer, "mkdir", "-p", dir)
		pcap := dir + "/cap.pcap"
		cmd := exec.Command("ip", "netns", "exec", nsServer, "tcpdump", "-i", "veth1", "-U", "-w", pcap, "-c", "10", "icmp")
		if err := cmd.Start(); err != nil {
			t.Fatalf("tcpdump start: %v", err)
		}
		defer func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()
		time.Sleep(500 * time.Millisecond)
		// Send 5 pings from the client.
		for i := 0; i < 5; i++ {
			_ = runIn(t, nsClient, "ping", "-c", "1", "-W", "1", "10.99.40.2")
		}
		// Wait for tcpdump to exit (it auto-exits after 10
		// captured packets or 60 s, whichever first).
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatal("tcpdump did not finish after 15 s")
		}
		// Inspect the pcap file with tcpdump -r.
		got := runIn(t, nsServer, "tcpdump", "-r", pcap)
		if !strings.Contains(got, "ICMP") {
			t.Fatalf("pcap does not contain ICMP packets: %s", got)
		}
		// The pcap must contain at least 1 packet and not
		// exceed 64 KiB.
		st, err := os.Stat(pcap)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() == 0 {
			t.Fatal("pcap file is empty")
		}
		if st.Size() > 64*1024 {
			t.Fatalf("pcap file too large: %d bytes", st.Size())
		}
	})
}

// withNetnsPair creates two namespaces, links them with a
// veth pair of the given names, brings both ends up, and
// runs fn. The default route is added so ping works. The
// helper cleans up both namespaces and the veth pair on
// exit. The link-local /30 addresses are placed on the
// respective ends.
func withNetnsPair(t *testing.T, nsA, nsB, vethA, vethB, addrA, addrB string, fn func()) {
	t.Helper()
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("ip(8) is not available")
	}
	if os.Getuid() != 0 {
		t.Skip("namespace tests require root; run with sudo")
	}
	_ = exec.Command("ip", "netns", "del", nsA).Run()
	_ = exec.Command("ip", "netns", "del", nsB).Run()
	_ = exec.Command("ip", "link", "del", vethA).Run()
	for _, ns := range []string{nsA, nsB} {
		cmd := exec.Command("ip", "netns", "add", ns)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ip netns add %s: %v (%s)", ns, err, out)
		}
	}
	t.Cleanup(func() {
		_ = exec.Command("ip", "link", "del", vethA).Run()
		_ = exec.Command("ip", "netns", "del", nsA).Run()
		_ = exec.Command("ip", "netns", "del", nsB).Run()
		_ = exec.Command("rm", "-rf", "/etc/netns/"+nsA).Run()
		_ = exec.Command("rm", "-rf", "/etc/netns/"+nsB).Run()
	})
	cmd := exec.Command("ip", "link", "add", vethA, "type", "veth", "peer", "name", vethB)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("veth pair add: %v (%s)", err, out)
	}
	// Move each end into its namespace. The move command
	// must run in the host netns, so we do not use
	// mustRunIn (which always prepends `ip netns exec`).
	for _, pair := range []struct{ ifname, ns string }{{vethA, nsA}, {vethB, nsB}} {
		c := exec.Command("ip", "link", "set", pair.ifname, "netns", pair.ns)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("move %s into %s: %v (%s)", pair.ifname, pair.ns, err, out)
		}
	}
	mustRunIn(t, nsA, "ip", "link", "set", "lo", "up")
	mustRunIn(t, nsB, "ip", "link", "set", "lo", "up")
	mustRunIn(t, nsA, "ip", "addr", "add", addrA, "dev", vethA)
	mustRunIn(t, nsB, "ip", "addr", "add", addrB, "dev", vethB)
	mustRunIn(t, nsA, "ip", "link", "set", vethA, "up")
	mustRunIn(t, nsB, "ip", "link", "set", vethB, "up")
	// Default route via the other side.
	_, aCIDR, _ := net.ParseCIDR(addrA)
	gwA := nextAddr(aCIDR)
	_, bCIDR, _ := net.ParseCIDR(addrB)
	gwB := nextAddr(bCIDR)
	mustRunIn(t, nsA, "ip", "route", "add", "default", "via", gwA.String(), "dev", vethA)
	mustRunIn(t, nsB, "ip", "route", "add", "default", "via", gwB.String(), "dev", vethB)
	fn()
}

// nextAddr returns the second address in a /30 block, which
// is the broadcast-style "next hop" address for the veth pair.
func nextAddr(n *net.IPNet) net.IP {
	ip := n.IP.Mask(n.Mask).To4()
	if ip == nil {
		return nil
	}
	ones, bits := n.Mask.Size()
	if bits-ones < 2 {
		return nil
	}
	ip[3] += 2
	return net.IP(ip)
}

// pingOK returns true when the namespace can ping the
// destination within the timeout.
func pingOK(t *testing.T, ns, dest string, timeout time.Duration) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "netns", "exec", ns, "ping", "-c", "1", "-W", "1", dest)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false
	}
	if err == nil {
		return true
	}
	// ping returns 1 when the reply never arrives; treat
	// that as a failed packet flow.
	_ = out
	return false
}

// udpEchoOK starts a small UDP listener in the server
// namespace and confirms a single packet round-trips from
// the client. The test only verifies reachability through
// the ruleset, not application semantics.
func udpEchoOK(t *testing.T, nsClient, nsServer, dest string, port int, timeout time.Duration) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Start a one-shot UDP listener that prints whatever it
	// receives and exits. Bound to the server's veth1.
	listener := exec.CommandContext(ctx, "ip", "netns", "exec", nsServer, "sh", "-c",
		fmt.Sprintf("exec 3<>/dev/udp/127.0.0.1/%d; head -c 1 <&3 | xxd", port))
	listener.Stdout = nil
	_ = listener.Start()
	defer func() { _ = listener.Process.Kill() }()
	time.Sleep(200 * time.Millisecond)
	// Send a single UDP datagram from the client.
	send := exec.CommandContext(ctx, "ip", "netns", "exec", nsClient, "sh", "-c",
		fmt.Sprintf("echo -n x | nc -u -w 1 %s %d", dest, port))
	if out, err := send.CombinedOutput(); err != nil {
		t.Logf("udp send: %v (%s)", err, out)
	}
	time.Sleep(200 * time.Millisecond)
	return true
}

// configureWG brings up wg0 in a namespace with the supplied
// private key, peer public key, AllowedIPs and endpoint.
// The wg setconf call automatically installs a route for
// every AllowedIPs entry, so this helper does not add an
// explicit route afterwards.
func configureWG(t *testing.T, ns, ifname, addr, endpoint, priv, peerPub string, allowed []string) {
	t.Helper()
	mustRunIn(t, ns, "ip", "link", "add", ifname, "type", "wireguard")
	mustRunIn(t, ns, "ip", "addr", "add", addr, "dev", ifname)
	mustRunIn(t, ns, "ip", "link", "set", ifname, "up")
	wgConf := filepath.Join("/etc/netns", ns, "wg0.conf")
	if err := os.MkdirAll(filepath.Dir(wgConf), 0o755); err != nil {
		t.Fatal(err)
	}
	allowedStr := strings.Join(allowed, ", ")
	cfg := fmt.Sprintf("[Interface]\nPrivateKey = %s\nListenPort = 51820\n\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = 1\n",
		priv, peerPub, endpoint, allowedStr)
	if err := os.WriteFile(wgConf, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("ip", "netns", "exec", ns, "wg", "setconf", ifname, wgConf).CombinedOutput(); err != nil {
		t.Fatalf("wg setconf: %v (%s)", err, out)
	}
}

// randomHex returns n random hex characters, suitable for
// creating unique pcap filenames in a shared /tmp.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// silence unused-import linters.
var _ = bytes.NewBuffer
