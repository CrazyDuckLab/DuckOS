//go:build linux

// Package qa: real IPv6 firewall qualification for §8.4 / §9.
// We extend the §8 nftables infra with IPv6 (`table ip6`)
// and verify:
//   - IPv6 ICMPv6 echo works through the firewall
//   - IPv6 input/output chains accept established/related
//   - Neighbor-discovery (ICMPv6 NS/NA) still works
package qa

import (
	"strings"
	"testing"
	"time"
)

// TestIPv6FirewallAllowsEstablishedAndICMPv6 is §8.4 / §9:
// real IPv6 nftables ruleset + real ICMPv6 echo.
//
// Topology mirrors §8.1: LAN <-> router <-> WAN, all /127
// IPv6 transit. We ping6 WAN from LAN; reply must arrive.
func TestIPv6FirewallAllowsEstablishedAndICMPv6(t *testing.T) {
	nft := findNFT(t)
	if nft == "" {
		t.Skip("nft not present")
	}
	before := CaptureManagementBoundary(t)
	lan := NewLab(t, "qa-ip6-lan")
	rtr := NewLab(t, "qa-ip6-rtr")
	wan := NewLab(t, "qa-ip6-wan")
	VethBetween(t, lan, rtr, "qa-ip6-lan-if", "qa-ip6-rtr-lan", "fd01:1::2/127", "fd01:1::3/127")
	VethBetween(t, rtr, wan, "qa-ip6-rtr-wan", "qa-ip6-wan-if", "fd01:2::2/127", "fd01:2::3/127")

	_, _, err := runInNSCaptureBytes(rtr.NetNS, "/sbin/sysctl", "-w", "net.ipv6.conf.all.forwarding=1")
	if err != nil {
		t.Fatalf("sysctl ip6 forward: %v", err)
	}
	rs := `table ip6 filter {
  chain input {
    type filter hook input priority 0; policy accept;
  }
  chain forward {
    type filter hook forward priority 0; policy accept;
    iifname "qa-ip6-rtr-lan" oifname "qa-ip6-rtr-wan" accept
    iifname "qa-ip6-rtr-wan" oifname "qa-ip6-rtr-lan" ct state established,related accept
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

	_, _, err = runInNSCaptureBytes(lan.NetNS, "/sbin/ip", "-6", "route", "add", "fd01:2::2/127", "via", "fd01:1::3", "dev", "qa-ip6-lan-if")
	if err != nil {
		t.Fatalf("lan onlink route to fd01:2::2/127: %v", err)
	}
	_, _, err = runInNSCaptureBytes(wan.NetNS, "/sbin/ip", "-6", "route", "add", "fd01:1::2/127", "via", "fd01:2::2", "dev", "qa-ip6-wan-if")
	if err != nil {
		t.Fatalf("wan onlink route to fd01:1::2/127: %v", err)
	}

	// Wait for IPv6 duplicate-address detection before the packet oracle.
	deadline := time.Now().Add(5 * time.Second)
	for {
		ready := true
		for _, lab := range []*Lab{lan, rtr, wan} {
			out, _, e := runInNSCaptureBytes(lab.NetNS, "/sbin/ip", "-6", "addr", "show")
			if e != nil || strings.Contains(out, "tentative") {
				ready = false
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("IPv6 fixture addresses did not become ready")
		}
		time.Sleep(100 * time.Millisecond)
	}
	pingOut, _, _ := runInNSCaptureBytes(lan.NetNS, "/bin/ping", "-6", "-c", "2", "-W", "2", "fd01:2::3")
	t.Logf("IPv6 ping:\n%s", pingOut)
	if !strings.Contains(pingOut, "2 received") && !strings.Contains(pingOut, "1 received") {
		t.Fatalf("expected IPv6 ping reply, got: %s", pingOut)
	}

	// Verify the IPv6 ruleset survives via `nft list ruleset`.
	snap, _, _ := runInNSCaptureBytes(rtr.NetNS, nft, "list", "ruleset")
	t.Logf("nft ip6 ruleset snapshot (truncated):\n%s", firstLines(snap, 30))
	if !strings.Contains(snap, "ip6") {
		t.Fatalf("nft ruleset snapshot missing ip6 family")
	}
	if !strings.Contains(snap, "qa-ip6-rtr-wan") {
		t.Fatalf("nft ruleset snapshot missing qa-ip6-rtr-wan interface reference")
	}

	after := CaptureManagementBoundary(t)
	AssertBoundaryEqual(t, before, after)
}

func firstLines(s string, n int) string {
	out := ""
	for i, line := range strings.Split(s, "\n") {
		if i >= n {
			break
		}
		out += line + "\n"
	}
	return out
}
