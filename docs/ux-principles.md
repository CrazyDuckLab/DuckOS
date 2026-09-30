# Beginner-obvious, expert-powerful UX

DuckOS common workflows must be usable correctly by a junior administrator
without prior documentation, while an experienced engineer can inspect and
control the exact underlying behavior. This is a product-wide acceptance rule,
not a substitute for network correctness or security.

## Workflow structure

Every major feature follows intent → simple controls → preview → apply → result.
The default view exposes recommended controls for common cases; advanced detail
reveals networks, addresses, routes, priorities, protocol settings and generated
policy. Progressive disclosure need not use separate pages and must not remove
expert capability. All paths use the same Engine semantics and safety boundaries.

Use outcome-oriented labels: give a client Internet access, allow Main LAN,
allow clients to communicate, use the backup WAN on failure, route a selected
service through a tunnel, or allow a service from the Internet. Avoid requiring
chain names, interface pairs or route metrics for these common tasks.

## Defaults and explanations

A new Guest network defaults to Internet and DHCP on, local networks, peer
communication and management off, subject to real isolation capabilities.
A WireGuard remote-access server defaults to Internet on, selectable LAN access,
peer communication off and management off. Administrative VPN management still
requires explicit Management Access Policy authorization.

Important controls have one short consequence explanation, with longer technical
details expandable. Safe defaults must remain useful, but a capability that cannot
actually be enforced is unavailable with an explanation rather than silently enabled.

## Consequence preview and Effective Access

Before important changes, show who is affected, newly allowed destinations,
destinations remaining blocked, management consequences and recovery behavior.
Advanced users can inspect the exact generated policy before apply.

Effective Access evaluates inheritance, client overrides, firewall and routing
together. It is computed by the Engine, not copied from checkbox state. Distinguish
Internet, local networks/hosts, other clients, router DNS/services, Web UI and SSH.
Selecting a destination explains the rule or boundary responsible for the result.
Unknown enforcement or reachability is reported as unknown, not inferred as success.
See [Network Access Policy](network-access-policy.md) for transport/topology constraints.

## Guided first-time tasks

Guided workflows cover Internet configuration, LAN, Guest network, WireGuard
remote access, adding a WireGuard client and showing QR, multi-WAN failover,
Xray inbound, client subscription, Allow on WAN and routing selected traffic
through a tunnel. Each starts with intent, provides a safe preview and ends with
actual result, failure or rollback status. Unsupported workflows must not appear
completed merely because a wizard can be opened.

## Expert inspection

Inspect generated routes, nftables policy, policy-routing decisions, interfaces,
addresses, DNS decisions, tunnel routing, service exposure and effective
configuration. Advanced edits still obey validation, RBAC, ownership, transaction
and management recovery constraints. Secret export remains a separate explicitly
authorized operation; advanced inspection must not reveal keys by accident.

## UX completion gate

Every major feature requires evidence that a beginner can discover it, understand
its consequences, identify the safe default, complete the common case without
documentation and see the result. An expert must be able to inspect exact behavior,
and a dangerous mistake must be safely reversible. A missing element means the
feature is not UX-complete even if backend tests pass.

## Consolidated V1 management contract

The [frozen V1 scope](handoff/V1_SCOPE.md) extends this UX contract across every
major subsystem: stable fixed Full areas, dense lists, one Easy/Full model,
intent-driven cross-subsystem automation, visible owned objects/dependencies,
contextual actions and GUI/CLI parity. Full exposes complexity, not chores;
raw Linux configuration is a debug/recovery layer, not ordinary administration.
