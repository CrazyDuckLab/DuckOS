package serviceunit

import (
	"strings"
	"testing"
)

func TestDNSServiceBoundary(t *testing.T) {
	unit := DNSMasq()
	for _, line := range []string{
		"User=duck-dnsmasq", "Group=duck-dnsmasq", "UMask=0077",
		"ConditionPathExists=/etc/duckos-dnsmasq/dnsmasq.conf",
		"ConditionPathExists=/run/duckos-agent/network-ready",
		"ProtectSystem=strict", "ProtectHome=yes", "NoNewPrivileges=yes",
		"StateDirectory=duckos-dnsmasq", "StateDirectoryMode=0700",
		"RuntimeDirectoryMode=0700", "LimitCORE=0",
	} {
		if !strings.Contains("\n"+unit, "\n"+line+"\n") {
			t.Errorf("missing boundary %s", line)
		}
	}
	for _, forbidden := range []string{"--no-daemon", "--log-queries", "SupplementaryGroups=duckos", "CAP_SYS_ADMIN", "--conf-dir="} {
		if strings.Contains(unit, forbidden) {
			t.Errorf("unsafe directive %s", forbidden)
		}
	}
	if !strings.Contains(unit, "--keep-in-foreground --conf-file=/etc/duckos-dnsmasq/dnsmasq.conf") {
		t.Fatal("missing explicit production configuration")
	}
}

func TestNetworkRecoveryRunsBeforeNetworkdWithoutNetworkAuthority(t *testing.T) {
	unit := NetworkRecovery()
	// The qualified production unit intentionally does NOT use
	// PrivateNetwork=yes: recovery must inspect host routes and
	// rules via rtnetlink, which requires the kernel network
	// stack to be reachable. The service is sandboxed through
	// IPAddressDeny=any, RestrictAddressFamilies and the empty
	// CapabilityBoundingSet instead. We assert the current
	// intentional contract (PrivateNetwork=no + the actual
	// safety net below).
	for _, line := range []string{
		"DefaultDependencies=no",
		// The recovery step runs strictly before the host
		// network stack comes up. network-pre.target is the
		// networkd pre-state we hand artifacts off at.
		"Before=network-pre.target systemd-networkd.service duck-agent.service",
		"RequiredBy=systemd-networkd.service",
		"Type=oneshot",
		"RemainAfterExit=yes",
		"CapabilityBoundingSet=",
		"AmbientCapabilities=",
		// We DO NOT isolate from the network — recovery
		// inspects rtnetlink state of the host.
		"PrivateNetwork=no",
		// The remaining network-safety net is enforced by
		// outbound-traffic denial + address-family restriction.
		"IPAddressDeny=any",
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		"ProtectSystem=strict",
		"UMask=0077",
		"TimeoutStartSec=30",
	} {
		if !strings.Contains("\n"+unit, "\n"+line+"\n") {
			t.Errorf("missing boundary %s", line)
		}
	}
	// Safety net must NOT be silently bypassed by a missing-journal
	// guard. ConditionPathExists on the journal would let a broken
	// boot skip recovery entirely.
	if strings.Contains(unit, "ConditionPathExists=") {
		t.Fatal("missing journal must fail, not skip a required boot gate")
	}
}
