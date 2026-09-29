# ADR 0018: Shared network generation and evaluation

Status: Accepted; candidate-only routed-flow evaluation is implemented.

## Decision

Use the same ordered intermediate instructions for nftables forwarding/NAT
generation and offline flow evaluation. Policy routing likewise compiles one
ordered list of connected-network protection and private-table lookup/guard
pairs, consumed by networkd generation and the evaluator. Preserve existing
artifact behavior during this refactoring; no new host mutation is authorized.

Keep the executed packet path explicit: destination NAT precedes route lookup
and forward filtering; source NAT follows an accepted forward decision. Input,
output, bridge, existing conntrack and application proxy paths are different
models, not approximations of forwarding. Rules retain canonical object IDs for
explanation; Linux names are rendered bindings, not persistent identities.

Candidate-only results always include an exact configuration digest, runtime
assumptions and a not-tested reachability label. Deterministic managed allow/drop
decisions remain conditional on those assumptions. Missing dynamic state,
ambiguous route metrics and unspecified relevant ports produce unknown results.
An offline result is never proof of host compatibility or Safe Apply success.

The protected API is Engine-owned and performs no Agent IPC, shell execution,
DNS query, traffic generation or persistence. Source/destination and configuration
data stay out of URLs and errors. The endpoint currently requires network.write;
broader helpdesk explanation access will need separately scoped observed identity
and policy data rather than access to arbitrary configuration previews.

## Evidence and remaining qualification

Tests cover IPv4/IPv6, longest-prefix/metric selection, DNAT ordering, first-match
firewall drops, NAT family restrictions, private-table misses, connected-network
protection, optional ports, unsupported connection paths, dynamic uncertainty,
determinism, cancellation, and real API/Engine local-identity authorization.
Linux namespace comparisons against generated nftables and RPDB behavior remain
required native qualification, not implied by Mac tests or cross-compilation.
Higher-level access policy, DNS/category resolution, observed-state explainability
and the full configuration UI extend this representation without separate
contradictory decision logic.

## References

- [Netfilter hook ordering](https://wiki.nftables.org/wiki-nftables/index.php/Netfilter_hooks)
- [Simulation API contract](../../shared/api/network-simulation.md)
- [Product network-understanding requirements](../network-understanding.md)
