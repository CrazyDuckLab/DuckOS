# Understanding the network

This contract extends the existing access-policy, preview, monitoring and revision
architecture. It does not introduce a second policy engine or a cloud dependency.
Features below are readiness requirements, not claims of implemented behavior.

## Explainability and what-if evaluation

The Engine evaluates a typed traffic question against a named configuration
revision and a timestamped observation snapshot. Inputs identify a client, IP or
network, destination IP/hostname/network, protocol (TCP, UDP or ICMP) and optional
port. Ports apply only to protocols that have ports. Network ranges represent a
set of possible paths and must not be reduced to an arbitrary representative IP.

An explanation links identity, applicable DNS/category decisions, access policy,
firewall, policy routing, selected route, tunnel/WAN and NAT. Each step carries
stable canonical rule/object IDs, reason codes and available generated-configuration
locations. Presentation order must not imply incorrect Linux hook ordering: DNAT
can change a destination before route/forward evaluation, while source NAT usually
follows it. Local input, forwarding, proxy and bridge paths are distinct.

The evaluator and configuration generators consume shared normalized policy and
ordering semantics. A separate approximate UI rule matcher is not authoritative.
Results distinguish allowed, blocked, conditional, unknown and unsupported; an
unsupported stage must never silently become allowed. Unknown ingress identity,
stale state, competing host rules, conntrack state, ECMP, dynamic leases and external
firewalls can limit certainty. Configured intent, observed local behavior and
end-to-end reachability are separate evidence levels.

What-if simulation runs without generating packets or making DNS requests when
canonical and observed information is sufficient. Hostnames use a stated existing
resolution snapshot; otherwise resolution is unknown or an explicit separate
diagnostic operation is offered. Multiple A/AAAA answers can yield different
results. Expected WAN/tunnel, DNS treatment and fallback behavior are shown when
determinable. Candidate previews compare the same question before and after a
change. Simulation neither activates a configuration nor confirms Safe Apply.

Easy Mode answers what works, what blocks it and where traffic is expected to go.
Full Mode additionally exposes rule precedence, route/table/rule priorities,
transport assumptions and generated configuration. Explanations never include
private keys, credential URLs or raw packet contents.

## Live topology

Topology is a graph of canonical identities correlated with observed interfaces,
bridges, VLANs, LANs, Wi-Fi, WANs, WireGuard/Xray/NetBird and discovered devices.
Internet reachability is an observed or explicitly unknown relationship, not a
permanently green decorative node. Each edge records its source, timestamp and
kind: physical link, membership, routed dependency or proxy/service relationship.
Desired-but-unobserved and unsupported resources remain visibly distinct.

Selecting a client shows available addresses, interface/network association, DHCP
identity, traffic measurements and effective access/path. Selecting a tunnel shows
observed health, related clients, counters and route/policy dependencies. Missing
telemetry remains unavailable, never zero or simulated activity. Easy Mode groups
technical details; Full Mode can expand them without changing the underlying graph.

## Extensible migration preview

Source adapters identify supported OpenWrt, MikroTik, Kerio-style and generic Linux
configuration formats and versions. They parse data only: never execute imported
shell, scripts, hooks, binary plugins or command substitutions. Parsing is bounded
in bytes, objects, nesting and time. Source configurations are potentially secret
and never enter routine logs or support bundles.

Each understood source object receives provenance and one classification:
recognized, convertible, requires attention or unsupported. Conversion produces
canonical candidates and a loss/ambiguity report, including interface remapping,
unresolved references, management-access implications and secret-import requirements.
Unsupported source objects are preserved in the report, not silently dropped while
claiming equivalence. An adapter may support a narrow subset; no full-migration
promise follows from recognizing a vendor format.

Conversion has no host side effects. Applying imports uses the existing validators,
ownership checks, consequence preview and Safe Apply transaction. Hardware and
capability mismatches require explicit resolution before activation.

## Network time machine

The existing revision history becomes a semantic timeline: WAN/path, route,
firewall policy, client access, tunnel and DNS changes, plus observed connectivity
and health events. Events link revision, transaction, actor, affected objects and
timestamps without duplicating secrets. Clock uncertainty and observation gaps
remain visible. Temporal correlation does not establish causation.

Before/after comparisons show canonical changes and effective impact using the
same evaluator. Restoring a known-good revision creates a new candidate; it must
pass current capability/ownership checks and Safe Apply. A historical success does
not prove compatibility with today's hardware, credentials or upstream network.
Retention must preserve unresolved recovery evidence and last-known-good references.

## Local-first boundary

These capabilities use local Engine/Agent state and local storage. Core management,
routing, firewall, supported VPN, monitoring, configuration and signed local-package
updates do not require a Crazy Duck Lab account or external controller. A remote
VPN endpoint or upstream resolver can still be required by a chosen connection;
that is not a mandatory product cloud. Future cloud features are optional and must
not gate local operation or recovery.

## Acceptance evidence

- Shared evaluator/generator parity and rule-order tests; unknown information cannot
  produce a false definitive allow or reachability claim.
- IPv4/IPv6, input/forward, NAT, private-table fallback and tunnel dependency tests;
  no packet/DNS generation during offline simulation.
- Topology derived from actual snapshots, with stable identities, missing/stale
  data handling and no decorative fabricated links or counters.
- Import fixtures for each explicitly supported format/version, hostile inputs,
  unsupported-object reporting and no apply without the normal transaction gates.
- Timeline/revision correlation, before/after impact and restore/rollback tests.
- Local management and local-package operations tested without product-cloud access.
