# Network Access Policy

Status: required DuckOS 1.0 product layer; not an implemented policy service.

## Intent and ownership

Common access decisions use native controls rather than requiring nftables or
iproute2 knowledge. The Engine translates intent into explicit canonical rules,
retaining ownership, revision history, audit, Safe Apply and generated details.
This layer does not create a second firewall or tunnel-specific policy island.

Shared concepts are NetworkAccessPolicy, SourceGroup, DestinationGroup,
AccessCapability and PolicyPreset. A source can be a network/server, client
group or individual client. Destination classes remain distinct: Internet,
local networks, other clients, other VPN networks, router services and router
management. LAN access never implicitly authorizes SSH, Web UI or API.

## Controls and inheritance

WireGuard server/network controls cover client isolation, Internet access,
LAN access (none/selected/all allowed LANs), router services, management policy,
LAN-initiated connections toward clients, client-initiated connections toward
LANs, and selected other tunnel networks. Directions refer to new connections;
return traffic semantics must be explicit. Disabling isolation permits only
traffic allowed by the remaining policy, not an unrestricted accept rule.

Network defaults can be overridden per client. The UI identifies each inherited
value and explicit override, showing the effective result. Examples include an
Internet-only guest, an administrator phone with selected LAN access, and a
backup client limited to a NAS network. An override must not silently bypass
management authorization or a non-overridable security boundary.

LAN/VLAN controls use the same language: Internet access, isolation, other local
networks (none/selected/all), incoming access from other networks, and router
services (DNS, DHCP+DNS, selected services or management). Guest/IoT networks
can receive DHCP/DNS without reaching management or other local networks.

## Presets

| Preset | Explicit canonical intent |
|---|---|
| Internet Only | Internet on, LAN/other clients/management off |
| Home Network Access | Internet and selected LANs on; management off; peer access explicit |
| Trusted Admin | Internet and selected authorized LANs; management still subject to Management Access Policy; optional peer access |
| Isolated Guest | Internet on; local networks, other clients and management off |
| Site-to-Site | Explicit routed networks, optional Internet; isolation semantics appropriate for routed sites |
| Custom | Advanced inspectable rules within the same security and transaction boundary |

A preset is a canonical policy expansion, not a UI label. Editing expanded
intent must not leave a misleading preset label. Internet-only classification
must account for local/router destinations, IPv4/IPv6 and special-use ranges,
not merely subtract RFC1918 from a default route.

## Transport and topology capabilities

WireGuard routed IP tunnels differ from Xray proxy inbounds, transparent proxies,
SOCKS/HTTP access and authenticated proxy client identities. Xray exposes only
meaningful controls: Internet, private/selected internal networks, router
addresses, destination groups and direct/tunnel route choices where enforceable.
It must not pretend to provide layer-3 peer isolation for a proxy protocol.
NetBird and future providers share intent but declare their actual enforcement
capabilities and boundaries.

Client isolation means devices cannot directly communicate unless explicitly
allowed. For routed WireGuard peers, enforce identity/address binding and traffic
through the router. Same-subnet Ethernet or Wi-Fi traffic may bypass routed
nftables entirely. Such isolation requires supported bridge/AP/switch enforcement;
without it, the control must be unavailable with an explanation, not marked
effective. External unmanaged switches cannot be made isolated by a routed rule.

## Management and safety

Management remains governed by [Management Access Policy](adr/0013-independent-management-policy.md).
A convenience toggle can request management eligibility but cannot manufacture
a trusted-source grant or bypass authentication/RBAC. Router service permissions
are separate from management and from external [Port Exposure](requirements/product.md).

Conflicts include an override allowing LAN while an enforced boundary denies
private destinations, management requested from an untrusted source, and peer
isolation contradicting an explicit peer allow. Explain the conflicting objects,
effective outcome and safe resolution before apply. No silent precedence guess.

Changes use the common transaction, management-path protection, audit and
configuration history. Lost confirmation or failed verification restores the
last-known-good complete network state; generated rule ownership is retained.

## UI contract

Reusable controls include Access Toggle, Network Selector, Access Preset,
Policy Summary, Policy Matrix, inherited/override indicator and conflict warning.
Network/tunnel pages show a clickable summary of Internet, selected LANs, other
clients, DNS and management. An advanced source → destination matrix distinguishes
Allow, Block, Inherited and Custom. View policy details exposes exact generated
configuration without exposing secrets.

The normal interaction answers “Internet?”, “Home LAN?”, “Other VPN clients?”
and “Router management?” without manual firewall rules. The same language applies
across WireGuard, Xray, NetBird, LAN/VLAN, Wi-Fi and future transports.

## Acceptance

Tests must cover isolated/non-isolated WireGuard peers, Internet-only access,
selected LANs, management denied without explicit authorization and per-client
overrides. LAN/VLAN tests include guest/IoT isolation and selected inter-VLAN
access, with actual layer-2 topology evidence where applicable. Xray tests prove
private-destination restrictions for each supported inbound type. Safe Apply
tests remove the current management path and verify protection or rollback.

Deterministic compilation and UI tests must prove that controls expand into the
same effective policy shown by the summary/matrix. Native client traffic tests
must prove enforcement; displaying a toggle or generating a rule is insufficient.
