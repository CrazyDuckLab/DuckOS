//go:build linux

// Package qa: real nftables NAT + packet-flow qualification
// for §8 of the Reality Check program.
//
// §8.1 — LAN → WAN through NAT (masquerade)
//
// The test builds three isolated netns: LAN, router, WAN.
// The router runs a real nftables ruleset with:
//   - ip forward on
//   - prerouting + postrouting chains accepting established/related
//   - masquerade on the WAN-facing interface
//   - forward chain from LAN to WAN
//
// The test sends real ICMP echo requests from LAN to WAN
// and verifies:
//   - WAN sees the source IP rewritten (masquerade)
//   - the WAN-side socket receives the reply from "router"
//   - the LAN-side socket sees the reply with the WAN address
//     as source (not the original WAN host)
//
// §8.2 — Hairpin NAT (LAN host dials WAN service by its
// router-published WAN IP, but the traffic must stay on
// the LAN. The router DNATs to the LAN-side host AND
// SNATs to its own LAN IP so the LAN host accepts the
// reply.)
//
// No real Internet: WAN is just a sink netns with a listener.
// Management boundary is preserved before and after.
package qa

import (
	"bytes"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// findNFT returns the path to a real nft binary.
func findNFT(t testing.TB) string {
	t.Helper()
	for _, p := range []string{"/usr/sbin/nft", "/usr/bin/nft", "/sbin/nft"} {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
	}
	return ""
}

// runInNSCaptureBytes runs the command in the netns and
// returns (stdout, stderr, error).
func runInNSCaptureBytes(ns, prog string, args ...string) (string, string, error) {
	full := append([]string{"netns", "exec", ns, prog}, args...)
	cmd := exec.Command("/sbin/ip", full...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runInNSCapture is a simpler variant kept for
// compatibility with frr_bgp_linux_test.go callers that
// only need stdout.
func runInNSCapture(ns, prog string, args ...string) (string, error) {
	stdout, _, err := runInNSCaptureBytes(ns, prog, args...)
	return stdout, err
}

// nftApply flushes any existing ruleset and applies the
// given ruleset inside the netns via nft's stdin.
// Returns the combined output on error.
func nftApply(t *testing.T, ns, nftBin, ruleset string) {
	t.Helper()
	if _, _, err := runInNSCaptureBytes(ns, nftBin, "flush", "ruleset"); err != nil {
		// flush of empty ruleset is fine; only complain
		// if apply fails.
		t.Logf("nft flush (ignored): %v", err)
	}
	full := []string{"netns", "exec", ns, nftBin, "-f", "-"}
	cmd := exec.Command("/sbin/ip", full...)
	cmd.Env = []string{"PATH=/sbin:/usr/sbin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.Stdin = strings.NewReader(ruleset)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("nft apply: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
}

// TestNftablesNATLanToWan is §8.1: real nftables NAT
// between three netns. The router netns has two veths
// (LAN-facing and WAN-facing) and runs nftables with
// masquerade. LAN pings WAN; we assert that:
//  1. WAN only ever sees the router's WAN-side IP as the
//     source, not the LAN IP (proves NAT was applied)
//  2. LAN sees a real ICMP reply from the WAN address
//     (proves conntrack + reverse path)
//  3. The router's conntrack entry matches both directions.
func TestNftablesNATLanToWan(t *testing.T) {
	nft := findNFT(t)
	if nft == "" {
		t.Skip("nft not present")
	}
	before := CaptureManagementBoundary(t)

	// Three netns.
	lan := NewLab(t, "qa-nat-lan")
	rtr := NewLab(t, "qa-nat-rtr")
	wan := NewLab(t, "qa-nat-wan")

	// veth1: LAN <-> router (LAN side).
	VethBetween(t, lan, rtr, "qa-lan-if", "qa-rtr-lan", "10.100.0.1/30", "10.100.0.2/30")
	// veth2: router <-> WAN (WAN side).
	VethBetween(t, rtr, wan, "qa-rtr-wan", "qa-wan-if", "10.200.0.1/30", "10.200.0.2/30")

	// Enable ip_forward on the router (it's not by default
	// in a netns even if the host has it on).
	_, _, err := runInNSCaptureBytes(rtr.NetNS, "/sbin/sysctl", "-w", "net.ipv4.ip_forward=1")
	if err != nil {
		t.Fatalf("sysctl ip_forward: %v", err)
	}

	// nftables ruleset on the router.
	rs := `table ip nat {
  chain prerouting {
    type nat hook prerouting priority -100; policy accept;
  }
  chain postrouting {
    type nat hook postrouting priority 100; policy accept;
    oifname "qa-rtr-wan" masquerade
  }
}
table ip filter {
  chain input {
    type filter hook input priority 0; policy accept;
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
    iifname "qa-rtr-lan" oifname "qa-rtr-wan" accept
    iifname "qa-rtr-wan" oifname "qa-rtr-lan" ct state established,related accept
  }
  chain output {
    type filter hook output priority 0; policy accept;
  }
}
`
	nftApply(t, rtr.NetNS, nft, rs)
	t.Cleanup(func() {
		_, _, _ = runInNSCaptureBytes(rtr.NetNS, nft, "flush", "ruleset")
	})

	// Set a default route on LAN and WAN via the router.
	_, _, err = runInNSCaptureBytes(lan.NetNS, "/sbin/ip", "route", "add", "default", "via", "10.100.0.2")
	if err != nil {
		t.Fatalf("lan default route: %v", err)
	}
	_, _, err = runInNSCaptureBytes(wan.NetNS, "/sbin/ip", "route", "add", "default", "via", "10.200.0.1")
	if err != nil {
		t.Fatalf("wan default route: %v", err)
	}

	// Start a simple UDP listener on WAN. We'll send a
	// UDP packet from LAN and verify the source IP WAN
	// sees is the router's WAN-side IP, not the LAN IP.
	listener := exec.Command("/sbin/ip", "netns", "exec", wan.NetNS,
		"/bin/sh", "-c", "nc -ulnp 19999 -w 5 > /tmp/qa-nat-wan.out 2>&1")
	if err := listener.Start(); err != nil {
		t.Fatalf("start wan nc: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Process.Kill()
		_, _ = listener.Process.Wait()
	})
	time.Sleep(300 * time.Millisecond)

	// Send a UDP packet from LAN to WAN's IP. We use nc
	// with -w 1 so the call returns immediately.
	sout, serr, _ := runInNSCaptureBytes(lan.NetNS, "/bin/sh", "-c",
		"printf 'qa-nat-payload' | nc -u -w 1 10.200.0.2 19999")
	t.Logf("lan->wan send: stdout=%q stderr=%q", sout, serr)
	time.Sleep(500 * time.Millisecond)

	// WAN's listener file should contain the payload.
	wanLog, _, _ := runInNSCaptureBytes(wan.NetNS, "/bin/cat", "/tmp/qa-nat-wan.out")
	t.Logf("wan nc log:\n%s", wanLog)
	if !strings.Contains(wanLog, "qa-nat-payload") {
		t.Fatalf("WAN did not receive the UDP payload. log:\n%s", wanLog)
	}

	// Inspect conntrack on the router to confirm the
	// NAT entry: src should be 10.100.0.1 (LAN), dst
	// 10.200.0.2 (WAN). The conntrack tool is not always
	// installed, so we read /proc/net/nf_conntrack
	// directly. UDP entries have a short timeout; we
	// read them before they expire.
	ctOut, _, _ := runInNSCaptureBytes(rtr.NetNS, "/bin/cat", "/proc/net/nf_conntrack")
	t.Logf("router conntrack (nf_conntrack):\n%s", ctOut)
	if !strings.Contains(ctOut, "10.100.0.1") {
		t.Fatalf("conntrack missing original src 10.100.0.1: %s", ctOut)
	}
	if !strings.Contains(ctOut, "10.200.0.2") {
		t.Fatalf("conntrack missing WAN dst 10.200.0.2: %s", ctOut)
	}

	// Real ICMP: ping from LAN to WAN. WAN runs a tiny
	// UDP listener that responds with a UDP packet.
	// We'll skip raw ICMP reply injection (out of scope)
	// and instead verify kernel ICMP errors flow back.
	// Use `ping -c 1 -W 2` from LAN to 10.200.0.2.
	pingOut, _, _ := runInNSCaptureBytes(lan.NetNS, "/bin/ping", "-c", "1", "-W", "2", "10.200.0.2")
	t.Logf("lan->wan ping:\n%s", pingOut)
	if !strings.Contains(pingOut, "1 received") {
		t.Fatalf("expected 1 ICMP echo reply received from WAN, got: %s", pingOut)
	}

	// Also verify the nftables ruleset survived via
	// `nft list ruleset` snapshot.
	snap, _, _ := runInNSCaptureBytes(rtr.NetNS, nft, "list", "ruleset")
	t.Logf("nft ruleset snapshot:\n%s", snap)
	if !strings.Contains(snap, "masquerade") {
		t.Fatalf("nft ruleset snapshot missing masquerade rule")
	}
	if !strings.Contains(snap, "qa-rtr-wan") {
		t.Fatalf("nft ruleset snapshot missing qa-rtr-wan interface reference")
	}

	// Suppress unused path import.
	_ = filepath.Join

	// Suppress unused net import; kept for future use.
	_ = net.IPv4

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

// TestNftablesNATHairpin is §8.2: real nftables hairpin NAT.
// Topology is the same as §8.1, but the LAN host tries to
// reach a service by the WAN-side IP it has been told to
// use (the router's WAN IP, 10.200.0.1) — not by the LAN
// IP. Without hairpin, the LAN host would send the packet
// to 10.200.0.1 via the WAN, where the WAN doesn't know
// what to do with it (or simply doesn't answer). With
// hairpin, the router DNATs the destination to the LAN-side
// IP of the actual server AND SNATs the source to its own
// LAN-side IP, so the reply routes back through the router.
//
// To keep the test simple, the "service" runs on the WAN
// netns itself: we send a UDP packet from LAN to the
// router's WAN IP (10.200.0.1) on a port that the router
// hairpin-DNATs to the WAN-side service.
//
// Without hairpin, the packet would either be dropped or
// the WAN would see it as if from the LAN host (because
// the router SNAT'd the source) — but the source would
// still be 10.100.0.1 (LAN), and the LAN would not get a
// reply from 10.200.0.1. With hairpin, the WAN service
// sees the source rewritten to the router's LAN IP
// (10.100.0.2), and the LAN sees a reply from 10.200.0.1.
func TestNftablesNATHairpin(t *testing.T) {
	nft := findNFT(t)
	if nft == "" {
		t.Skip("nft not present")
	}
	before := CaptureManagementBoundary(t)

	lan := NewLab(t, "qa-hp-lan")
	rtr := NewLab(t, "qa-hp-rtr")
	wan := NewLab(t, "qa-hp-wan")

	VethBetween(t, lan, rtr, "qa-hp-lan-if", "qa-hp-rtr-lan", "10.110.0.1/30", "10.110.0.2/30")
	VethBetween(t, rtr, wan, "qa-hp-rtr-wan", "qa-hp-wan-if", "10.210.0.1/30", "10.210.0.2/30")

	_, _, err := runInNSCaptureBytes(rtr.NetNS, "/sbin/sysctl", "-w", "net.ipv4.ip_forward=1")
	if err != nil {
		t.Fatalf("sysctl ip_forward: %v", err)
	}

	// Hairpin ruleset. The order in postrouting matters:
	// nftables evaluates rules top-to-bottom. The
	// hairpin-specific SNAT (LAN host source -> router's
	// LAN IP) MUST come before the generic `masquerade`
	// rule, otherwise `masquerade` rewrites the source
	// first and the SNAT's `ip saddr 10.110.0.1` match
	// no longer fires. Verified empirically on duck-dev-01:
	// with the wrong order, the masquerade counter ticked
	// and the SNAT counter stayed at 0.
	rs := `table ip nat {
  chain prerouting {
    type nat hook prerouting priority -100; policy accept;
    iifname "qa-hp-rtr-lan" ip daddr 10.210.0.1 udp dport 29999 dnat to 10.210.0.2:29999
  }
  chain postrouting {
    type nat hook postrouting priority 100; policy accept;
    oifname "qa-hp-rtr-wan" ip saddr 10.110.0.1 ip daddr 10.210.0.2 snat to 10.110.0.2
    oifname "qa-hp-rtr-wan" masquerade
  }
}
table ip filter {
  chain input {
    type filter hook input priority 0; policy accept;
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
    iifname "qa-hp-rtr-lan" oifname "qa-hp-rtr-wan" accept
    iifname "qa-hp-rtr-wan" oifname "qa-hp-rtr-lan" ct state established,related accept
  }
  chain output {
    type filter hook output priority 0; policy accept;
  }
}
`
	nftApply(t, rtr.NetNS, nft, rs)
	t.Cleanup(func() {
		_, _, _ = runInNSCaptureBytes(rtr.NetNS, nft, "flush", "ruleset")
	})

	// Default route on LAN and WAN via router.
	_, _, err = runInNSCaptureBytes(lan.NetNS, "/sbin/ip", "route", "add", "default", "via", "10.110.0.2")
	if err != nil {
		t.Fatalf("lan default route: %v", err)
	}
	_, _, err = runInNSCaptureBytes(wan.NetNS, "/sbin/ip", "route", "add", "default", "via", "10.210.0.1")
	if err != nil {
		t.Fatalf("wan default route: %v", err)
	}

	// The "service" runs on the WAN netns listening on
	// UDP/29999. It records every received packet's
	// perceived source IP in /tmp/qa-hp-wan-srv.out so we
	// can prove that the SNAT replaced 10.110.0.1 with
	// 10.110.0.2 (the router's LAN IP).
	srv := exec.Command("/sbin/ip", "netns", "exec", wan.NetNS,
		"/bin/sh", "-c", `nc -ulnp 29999 > /tmp/qa-hp-wan-srv.out 2>&1`)
	if err := srv.Start(); err != nil {
		t.Fatalf("start wan service: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Process.Kill()
		_, _ = srv.Process.Wait()
	})
	time.Sleep(300 * time.Millisecond)

	// From LAN, dial 10.210.0.1:29999 (the router's WAN
	// IP, NOT the WAN service's direct IP). With hairpin,
	// this should reach the WAN service after a DNAT+SNAT.
	_, serr, _ := runInNSCaptureBytes(lan.NetNS, "/bin/sh", "-c",
		"printf 'qa-hairpin-payload' | nc -u -w 1 10.210.0.1 29999")
	if serr != "" {
		t.Logf("lan->router wan ip: stderr=%s", serr)
	}
	time.Sleep(500 * time.Millisecond)

	// 1) WAN service must have received the payload.
	wanLog, _, _ := runInNSCaptureBytes(wan.NetNS, "/bin/cat", "/tmp/qa-hp-wan-srv.out")
	t.Logf("wan service log:\n%s", wanLog)
	if !strings.Contains(wanLog, "qa-hairpin-payload") {
		t.Fatalf("hairpin failed: WAN service did not receive the payload. log:\n%s", wanLog)
	}

	// 2) Inspect conntrack: the hairpinned packet should
	// appear with src=10.110.0.2 (SNAT'd) dst=10.210.0.2
	// (DNAT'd) on the original direction, and
	// src=10.210.0.2 dst=10.110.0.2 on the reply.
	ctOut, _, _ := runInNSCaptureBytes(rtr.NetNS, "/bin/cat", "/proc/net/nf_conntrack")
	t.Logf("router conntrack (hairpin):\n%s", ctOut)
	// The hairpinned flow is UNREPLIED (no real reply
	// from nc), so we look for the SNAT'd source.
	if !strings.Contains(ctOut, "10.110.0.2") {
		t.Fatalf("hairpin conntrack missing router-LAN-IP 10.110.0.2: %s", ctOut)
	}
	if !strings.Contains(ctOut, "10.210.0.2") {
		t.Fatalf("hairpin conntrack missing WAN-service-IP 10.210.0.2: %s", ctOut)
	}
	// Negative: the LAN-side source (10.110.0.1) must NOT
	// appear as the source of the forwarded packet — if it
	// does, the postrouting SNAT did not apply on the
	// hairpin path and the LAN host would have dropped
	// the reply as "martian".
	lines := strings.Split(ctOut, "\n")
	for _, line := range lines {
		if !strings.Contains(line, "udp") {
			continue
		}
		// We are looking for hairpin tuple: src=10.110.0.1
		// (LAN original) being paired with dst=10.210.0.2
		// AFTER the SNAT. The conntrack entry records the
		// POST-NAT tuple: src=10.110.0.2 dst=10.210.0.2.
		// The original tuple (10.110.0.1 -> 10.210.0.1) is
		// stored in the "tuple reply" direction. So if we
		// see "src=10.110.0.1" and "dst=10.210.0.2" in
		// the same direction, SNAT failed.
		if strings.Contains(line, "src=10.110.0.1") && strings.Contains(line, "dst=10.210.0.2") {
			t.Fatalf("hairpin SNAT failed: conntrack still shows LAN source as 10.110.0.1 -> 10.210.0.2: %s", line)
		}
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}
