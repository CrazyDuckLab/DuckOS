# Home, homelab and small-business gateways

Design guide; full router activation is not available in the installed runtime.

A home router or small-business gateway needs more than an nftables web interface:
address acquisition, LAN DHCP/DNS, forwarding/NAT, policy, recovery and an
administrator path must work together. Separate guest/IoT networks may use VLANs;
their firewall policy must not grant management merely because they are LANs.

Before an isolated trial, record uplink method, VLAN tags, IPv4/IPv6 delegation,
LAN subnets, DHCP reservations, DNS behavior and trusted administrator sources.
Check for overlapping subnets and competing network managers. Keep the existing
gateway and console recovery available; do not connect trial DHCP to a live LAN.

Acceptance requires a real client to acquire/renew an address, resolve local and
external DNS, traverse the intended firewall/NAT path, respect isolation, and
recover after reboot and lost Safe Apply confirmation. API success is insufficient.
See [requirements](../product-spec-1.0.md), [testing](../testing.md),
[routing](routing.md) and [installation](../install.md).
