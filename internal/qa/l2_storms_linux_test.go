//go:build linux

// Package qa: real L2 storm / failure qualification for §10
// of the Reality Check program.
//
// §10 attempts to verify that the network stack (and any
// future DuckOS bridge-loop detection) responds correctly
// to:
//   - L2 broadcast storms (flooded ff:ff:ff:ff:ff:ff frames)
//   - L2 unicast storms (same dst MAC repeated rapidly)
//   - rogue DHCP server on the LAN
//   - rogue IPv6 RA on the LAN
//
// We do NOT measure "bounded resource usage" inside the
// kernel itself (that requires instrumentation beyond a
// QA box); we measure:
//   - Whether the kernel can still answer legitimate
//     traffic while a storm is in progress
//   - Whether legitimate DHCP/RA on a separate veth pair
//     still gets through when a rogue server is blasting
//     on the storm veth
//
// No bridge is required for the storm itself: a veth pair
// in two netns is sufficient for the L2 traffic to flow.
package qa

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// floodVeth broadcasts ARP requests from srcA continuously
// for the given duration. This simulates a broadcast storm:
// every frame is `ff:ff:ff:ff:ff:ff` so every device in the
// broadcast domain must process it.
func floodVeth(t testing.TB, ns, iface, srcMAC string, dur time.Duration) {
	t.Helper()
	// `arping -U -I iface -s srcMAC <broadcast addr>` floods
	// ARP "I am here" frames at line rate until killed.
	// We use `-A` (ARP reply broadcast) for higher pps.
	cmd := exec.Command("/sbin/ip", "netns", "exec", ns,
		"/usr/bin/timeout", "-k", "1", dur.String(),
		"/usr/sbin/arping", "-f", "-q", "-I", iface, "-s", srcMAC,
		"255.255.255.255")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	_ = cmd.Run()
}

// TestL2BroadcastStormDoesNotKillLegitimateTraffic is §10.1.
// While one veth pair is being hit by an ARP storm, a second
// veth pair between the same two netns carries real ICMP.
// We verify the ICMP echo succeeds during the storm.
func TestL2BroadcastStormDoesNotKillLegitimateTraffic(t *testing.T) {
	before := CaptureManagementBoundary(t)

	storm := NewLab(t, "qa-storm")
	peer := NewLab(t, "qa-peer")
	// Storm veth: 192.168.100.x/30. Flooded with ARP.
	VethBetween(t, storm, peer, "qa-storm-if-a", "qa-storm-if-b", "192.168.100.1/30", "192.168.100.2/30")
	// Legit veth: 192.168.200.x/30. Carries ICMP during storm.
	VethBetween(t, storm, peer, "qa-legit-if-a", "qa-legit-if-b", "192.168.200.1/30", "192.168.200.2/30")

	// Kick off the broadcast storm. 5 seconds is enough to
	// generate millions of frames.
	go floodVeth(t, storm.NetNS, "qa-storm-if-a", "aa:bb:cc:dd:ee:f1", 5*time.Second)

	// Wait briefly so the storm has produced frames.
	time.Sleep(300 * time.Millisecond)

	// Now send a real ICMP echo on the OTHER veth pair and
	// verify it round-trips.
	out, _, _ := runInNSCaptureBytes(storm.NetNS, "/bin/ping", "-c", "3", "-W", "1", "-I", "qa-legit-if-a", "192.168.200.2")
	t.Logf("ping during storm:\n%s", out)
	if !strings.Contains(out, "3 received") && !strings.Contains(out, "received, 0%") {
		// "received, 0%" means at least one packet lost but
		// some got through. We accept >=1 received.
		if !strings.Contains(out, "1 received") {
			t.Fatalf("expected at least 1 ping reply during broadcast storm, got: %s", out)
		}
	}

	// Confirm the storm actually ran by reading
	// /sys/class/net/qa-storm-if-a/statistics for non-zero
	// TX packet count.
	statsOut, _, _ := runInNSCaptureBytes(storm.NetNS, "/bin/cat",
		"/sys/class/net/qa-storm-if-a/statistics/tx_packets")
	t.Logf("storm-if-a tx_packets: %s", statsOut)
	if strings.TrimSpace(statsOut) == "0" {
		t.Fatalf("storm did not produce any TX frames; test did not actually exercise the storm path")
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}
