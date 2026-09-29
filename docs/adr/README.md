# Architecture decision records

Architecture decision records (ADRs) capture consequential DuckOS decisions
that implementation must preserve. They complement the living product
specification and architecture documents; they do not replace either one.

## Statuses

- **Proposed** — under review and not an implementation constraint.
- **Accepted** — approved direction for new work.
- **Superseded** — replaced by a later ADR, which must be linked.
- **Deprecated** — retained for history but no longer recommended.

An accepted ADR may deliberately leave implementation details open. Those
details remain in [`../open-decisions.md`](../open-decisions.md) until a later
ADR resolves them. Material changes are recorded by superseding an ADR rather
than rewriting its historical decision.

## Index

| ADR | Status | Decision |
|---|---|---|
| [0001](0001-core-language.md) | Accepted | React with TypeScript for the web UI; Go as the default language for backend/core executables |
| [0002](0002-agent-privilege-boundary.md) | Accepted | Separate narrow privileged Agent behind versioned semantic IPC |
| [0003](0003-config-source-of-truth.md) | Accepted | Versioned canonical desired state, separate from rendered and observed Linux state |
| [0004](0004-update-active-previous.md) | Accepted | Side-by-side update with verified activation, rollback, and bounded Active + Previous retention |
| [0005](0005-executable-observation-foundation.md) | Accepted, read-only scope | Executable API/Engine/Agent observation path, loopback authentication, toolchain and fresh-install contract |
| [0006](0006-durable-state.md) | Accepted | Bounded durable configuration snapshots, exclusive writer, explicit recovery journal |
| [0007](0007-managed-network-rendering.md) | Accepted, offline implementation | Validated networkd/dnsmasq/nftables plans; activation requires ownership and recovery gates |
| [0008](0008-local-identity.md) | Accepted | Local identity, first Owner enrollment, bounded sessions and explicit role enforcement |
| [0009](0009-firewall-kernel-recovery.md) | Accepted, activation gated | Scoped nftables mutations and captured kernel recovery |
| [0010](0010-network-coordinator-ipc.md) | Accepted, activation gated | Shared Engine/Agent transaction identity and durable outcome reconciliation |
| [0011](0011-agent-resource-enrollment.md) | Accepted, prerequisite only | Root-controlled physical/VLAN/firewall resource allowlist |
| [0012](0012-fail-closed-ip-policy-routing.md) | Accepted, compilation only | Dual-stack private-table policy plans with explicit unreachable fallback guards |
| [0013](0013-independent-management-policy.md) | Accepted, compilation only | Explicit trusted management grants independent of WAN/LAN role |
| [0014](0014-private-route-kernel-recovery.md) | Accepted, activation gated | Scoped private-route inventory, mutation and exact-state recovery |
| [0015](0015-secret-client-exports.md) | Accepted, lifecycle integration pending | Shared authenticated secret exports with identical QR, clipboard and file payloads |
| [0016](0016-secret-record-encryption.md) | Accepted, Vault activation gated | Context-bound encrypted secret records with per-write derived keys |
| [0017](0017-sqlite-client-catalog.md) | Accepted, activation separate | SQLite canonical client metadata without replacing Safe Apply journals or Vault |
| [0018](0018-shared-network-evaluation.md) | Accepted, candidate-only | Shared nftables/policy instructions for generation and honest offline flow explanations |
| [0019](0019-owned-policy-rule-recovery.md) | Accepted, activation gated | Scoped RPDB capture, bounded real rule mutation and exact-state recovery |
| [0020](0020-network-transaction-forwarding-barrier.md) | Accepted, activation gated | Verified nftables forwarding barrier retained across composite transaction stages |

## Record format

Each record states its context, decision, consequences, and explicitly
unresolved work. A decision is not implementation or test evidence. The
repository must not claim a boundary production-ready until the corresponding
verification gates are satisfied.
