# ADR-0013: Management authorization independent of topology role

- Status: Accepted for validation/rendering; host activation remains gated
- Date: 2026-09-09
- Refines: ADR-0007 management grants and ADR-0012 policy anchoring

## Context

A single-NIC edge node can receive administrative traffic on its WAN interface.
WAN/LAN is a topology role, not sufficient authorization for management access.
Installation must not alter existing SSH or network state. Dangerous changes
need complete management-path recovery, not just a successful nft batch.

## Decision

The optional canonical `managementPolicy` contains bounded rules with stable ID,
interface ID, SSH/Web/API service, explicit TCP port and canonical trusted source
CIDRs. IPv4 and IPv6 sources render family-specific nft rules. A host source is
represented as /32 or /128. An explicit policy has no implicit LAN grant and
cannot mix with legacy interface `management` flags. Default routes as trusted
sources (/0), absent sources, unknown interfaces, duplicate transport grants,
invalid ports and unsupported services are rejected. Listener configuration is
separate: a firewall permission does not start or expose an API listener by itself.

Existing version-1 configurations without this field retain their exact legacy
LAN-only behavior until explicitly migrated. Reading old state must not silently
change firewall behavior. Migration requires a candidate, diff, management-path
capture, Safe Apply and recovery verification. Older strict readers cannot read
the new field, which must be accounted for by update compatibility checks.

Explicit grants allow a one-physical-interface WAN configuration with static or
dynamic address acquisition and no physical LAN. Without LAN DNS intent, this
configuration emits no DNS service configuration, not an implicit loopback
listener. DNS intent without a supported LAN listener is rejected. VPN interface
kinds, admin tunnels, bridge support and tunnel NAT are separate remaining work;
accepting a one-NIC plan does not claim a functioning VPN gateway.

RPDB sections anchor to the lexicographically first explicitly selected management
interface, independent of role. Legacy plans retain their former anchor. Dynamic
link lifecycle cannot be considered an atomic transaction: the eventual adapter
must protect and verify rules through link loss, replacement and reboot.

Management policy changes remain rejected by the firewall-only adapter. They
require the composite transaction to protect the current management source,
route, address and interface, verify reconnection and roll back on lost confirmation.
No installed mutation endpoint or systemd permission is enabled by this decision.

## Verification

Portable tests cover source-scoped dual-stack WAN grants, explicit ports,
role-independent authorization, no implicit DNS/management grants, invalid and
ambiguous policies, canonical round trips, unchanged legacy rendering and
firewall-only adapter refusal. Native nft parsing, actual management reconnect,
lost-confirmation, reboot and full network rollback require isolated Linux tests.
