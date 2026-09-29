//go:build linux

// chaos/readiness_registry_linux.go: G04 daemon probe
// registry. The agent composition root calls
// RegisterDefaultProbes to wire every classifier the
// DuckOS freeze candidate recognises. Each probe is
// non-mutating and bounded; classifications never
// claim "supported" without real evidence.
package chaos

// RegisterDefaultProbes wires the canonical probe set into
// r. Callers may add additional probes afterwards; the
// recorder replaces earlier registrations for the same
// feature key.
func RegisterDefaultProbes(r *ReadinessRecorder) {
	if r == nil {
		return
	}
	register := func(p ReadinessProbe) { r.Register(PrerequisiteProbe{p}) }
	// Routing: FRR provides BGP/OSPF/ISIS/RIP/BFD.
	register(DaemonProbe{
		FeatureKey:  "routing.frr",
		Binary:      "vtysh",
		StateFile:   "/run/frr/frr.pid",
		VersionArgs: []string{"-c", "show version"},
	})
	// Firewall: nftables is the primary backend; absence is
	// hardware-pending until an alternate is wired.
	register(ExecutableProbe{
		FeatureKey: "network.firewall",
		Binary:     "nft",
		Reason:     "nftables binary not installed",
	})
	// DHCP: prefer ISC DHCP, fall back to dnsmasq.
	register(DaemonProbe{
		FeatureKey:  "network.dhcp.isc",
		Binary:      "dhcpd",
		VersionArgs: []string{"--version"},
	})
	register(DaemonProbe{
		FeatureKey:  "network.dhcp.dnsmasq",
		Binary:      "dnsmasq",
		VersionArgs: []string{"--version"},
	})
	// DNS: prefer bind9; dnsmasq doubles as recursive DNS.
	register(DaemonProbe{
		FeatureKey:  "network.dns.bind",
		Binary:      "named",
		VersionArgs: []string{"-v"},
	})
	// RADIUS: FreeRADIUS for AAA.
	register(DaemonProbe{
		FeatureKey:  "aaa.radius",
		Binary:      "radiusd",
		VersionArgs: []string{"-v"},
	})
	// Wi-Fi AP: hostapd.
	register(DaemonProbe{
		FeatureKey:  "wifi.ap",
		Binary:      "hostapd",
		StateFile:   "/run/hostapd/global",
		VersionArgs: []string{"-v"},
	})
	// VPN: WireGuard kernel module + xray binary.
	register(KernelModuleProbe{FeatureKey: "vpn.wireguard", ModuleName: "wireguard"})
	register(ExecutableProbe{
		FeatureKey: "vpn.xray",
		Binary:     "xray",
		Reason:     "xray binary not installed",
	})
	// Upgrade: requires distro package manager + persistent
	// journal directory. apt/dnf/pacman are all acceptable.
	register(PathProbe{
		FeatureKey: "upgrade.journal",
		Path:       "/var/lib/duckos/upgrade",
		WantDir:    true,
		Label:      "upgrade journal directory",
	})
	// Backup: same persistent directory.
	register(PathProbe{
		FeatureKey: "backup.journal",
		Path:       "/var/lib/duckos/backup",
		WantDir:    true,
		Label:      "backup journal directory",
	})
	// Multi-WAN: requires at least two physical NICs.
	// The probe stays at experimental until the topology
	// host reports the second link; the agent composition
	// root registers an override probe that consults the
	// live topology if needed.
	register(ExecutableProbe{
		FeatureKey: "multiwan",
		Binary:     "ip",
		Reason:     "iproute2 missing; multi-wan unsupported",
	})
}
