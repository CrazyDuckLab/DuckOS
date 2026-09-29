# Foundation open decisions

This register contains decisions that are genuinely unresolved and material.
It is not a general backlog. Each item must become an ADR or an explicit product
specification decision before dependent implementation starts.

| ID | Decision | Blocks | Current constraint |
|---|---|---|---|
| F-001 | API and Engine deployment topology | Resolved | ADR-0005: separate packages in one unprivileged Go process. |
| F-002 | Agent IPC | Baseline resolved | ADR-0005/0010: peer-authenticated bounded Unix HTTP/JSON and network coordinator receipts; runtime activation/qualification remains work. |
| F-003 | Persistence | Baseline resolved; broader migration open | ADR-0006 durable JSON revisions and ADR-0017 implemented SQLite client catalog; do not replace working stores without migration/recovery evidence. |
| F-004 | Secrets lifecycle | Key rotation/backup recovery | ADR-0016 implements explicit local master key and authenticated encrypted records. Hardware/root compromise limits are documented; complete lifecycle remains open. |
| F-005 | Supported Debian/kernel/systemd/nftables baseline and ARMv7 ABI | Native platform qualification | ADR-0005 pins tools and Debian 13 amd64 first install; ARMv7/ARM64 cross-build without native support claims. |
| F-006 | Host-network activation | Integrated native qualification | ADR-0011/0023 define root-enrolled resources and explicit manager-wide reload; boot/management/bypass/competing-manager gates remain incomplete. |
| F-007 | Host-level Debian change and rollback contract | Upgrade Engine | ADR-0005 bounds first installation to new DuckOS-owned state; refuses existing installs and OS package mutations. Upgrade rollback remains open. |
| F-008 | V1 scope freeze | Resolved for handoff | 2026-09-10 frozen V1_SCOPE preserves requirements with A/B/C delivery classes; changes need recorded scope decisions. |
| F-009 | Open-source license | Public release/public repository | No `LICENSE` is added until dependencies and business intent are reviewed. |

## Clarification adopted at bootstrap

The term *source of truth* has two scoped meanings:

- Git is the source of truth for product source, build inputs, documentation,
  packaging definitions, and release provenance.
- The versioned canonical configuration store is the source of truth for the
  desired state of one DuckOS installation.

Rendered Linux configuration and transient kernel state are never the product's
canonical configuration model.
