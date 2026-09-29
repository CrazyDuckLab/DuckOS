# ADR-0012: Fail-closed IP policy routing execution plans

- Status: Accepted for compilation; native mutation activation remains gated
- Date: 2026-09-09
- Management policy anchoring refined by [ADR-0013](0013-independent-management-policy.md).

The bounded execution configuration supports IPv4/IPv6 source/destination CIDR
policy selectors and private route tables 10000–10999. This is a low-level
execution model for future named Traffic Policies and Route Profiles/Groups,
not a replacement for them or a claim of domain/SNI classification. Tables and
RPDB priority ownership require separate local root enrollment; API intent
cannot grant itself that ownership. Kernel inventory collision checks remain
mandatory before enabling an adapter that changes these resources.

Each selector uses an even lookup priority in 20000–21998 and the following
priority for an identical unreachable selector. A failed/empty selected table
must not implicitly fall through to main and leak traffic through a different
WAN. Priorities are explicit, bounded and unique. Table/family references must
resolve to configured routes; duplicate unicast route keys are rejected rather
than treated as implicit multipath configuration.

Connected subnets take precedence at priorities 10000–12047. Each uses a main
lookup suppressing routes less specific than that subnet, followed immediately
by unreachable. Thus loss of a connected route does not authorize a default
WAN fallback for that destination. Zero-width connected subnets are rejected
when policy routing is present because this protection would be ambiguous.
Local kernel rules retain priority zero. Unmatched external traffic otherwise
keeps normal main-table behavior.

Networkd policy sections are anchored once to the lexicographically first
management LAN ID, not a transient WAN. Rule order is deterministic and does
not depend on interface input order. Private-table gateways are marked on-link
only after validating that they belong to a configured link (or scoped IPv6
link-local gateway). Generated artifact size limits apply to the expanded plan.

Networkd reload is not atomic. Production activation must guard matching traffic
while replacing rules/routes, verify the complete RPDB and tables, retain
captured recovery state and restore guards safely on failure/reboot. These
requirements are NOT satisfied merely by emitting paired rules. The installed
Agent remains read-only; the current firewall-only adapter refuses policy
changes. Multi-WAN health selection, named route groups, tunnel/node resolution,
domain policies and runtime traffic verification remain separate unfinished work.

The optional schema fields are rejected by older strict readers once used;
update compatibility must not roll back onto binaries that cannot read them.

Reference: [systemd 257 network configuration](https://www.freedesktop.org/software/systemd/man/257/systemd.network.html),
RoutingPolicyRule Family/Priority/Type/SuppressPrefixLength and Route GatewayOnLink.
