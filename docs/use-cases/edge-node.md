# Single-NIC edge nodes and VPS VPN gateways

Design guide; a validated plan is not yet a deployable VPN gateway.

A single-NIC gateway can use one physical WAN/uplink for both Internet transport
and trusted administration. Client traffic arrives over a WireGuard, Xray or
NetBird transport; an optional separate admin tunnel isolates management from
client VPN access. A physical LAN is not required. A VLAN trunk can instead
provide logical WAN/LAN networks over one physical link.

Plan public address acquisition, upstream/provider firewall rules, trusted
administrator CIDRs, client address pools, listener ports and forwarding/NAT.
Management Access Policy is independent of WAN role. Enabling a tunnel must not
silently open a port; service exposure requires reviewed Allow on WAN and Safe Apply.
Local firewall permission does not prove external reachability.

For a WireGuard-to-VPS exit path, qualification must prove handshake, client DNS,
forwarding to the uplink, return traffic, IPv6 policy and selected-route failure
behavior. Change the management path in an isolated test and deliberately lose
confirmation; recovery must restore access. Keep a provider console available.
Installation alone must preserve current SSH, interfaces and firewall.

See [management decision](../adr/0013-independent-management-policy.md),
[client provisioning](client-provisioning.md), [security](../security-model.md)
and [testing](../testing.md).
