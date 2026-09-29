# Architecture at the handoff checkpoint

## Components and implemented boundaries

```text
React / TypeScript Web
        ↓ authenticated HTTP, bounded requests
Go API + Engine (one unprivileged process)
        ↓ HTTP/JSON v1 over Unix socket; Linux SO_PEERCRED UID check
Go privileged Agent
        ↓ fixed semantic operations / bounded fixed executables
Linux kernel + systemd-networkd + dnsmasq + nftables
```

The CLI is a required additional Engine/API client, not yet a complete product.
The current Agent runtime serves observations; mutation services and adapters
exist but are not activated by its normal startup path. `cmd/duck-agent` also
contains the separate pre-network artifact recovery command at this checkpoint.

| Code | Responsibility and current boundary |
|---|---|
| `internal/api`, `internal/engine` | Authenticated management, observations, candidate preview/simulation; optional coordinator/export use cases are not equivalent to installed activation. |
| `internal/agent` | Peer-authenticated Unix IPC, observation handlers, explicitly gated network service. |
| `internal/network` | Canonical network validation, renderers, simulation, enrollment, scoped real Linux adapters, guarded routing, managed-routing Safe Apply adapter and boot recovery. |
| `internal/state`, `internal/safeapply` | Private bounded JSON revision journal, exclusive writer, fsync/atomic replacement, outcomes and independent confirmation watchdog. |
| `internal/identity` | Local Owner enrollment, passwords, built-in RBAC, sessions and identity audit. |
| `internal/catalog` | Pure-Go SQLite client metadata/history/audit; not yet the universal configuration store or live client provider. |
| `internal/vault`, `internal/clientexport` | Protected key and context-bound encrypted records; redacted export objects and authorized exact QR/file payloads. |
| `web`, `shared` | UI, reusable secret modal, contracts/schema; no privileged application logic. |
| `packaging`, `scripts`, `tests` | Units, pinned tooling, fresh TEST installer, checks, cross-builds, native opt-in fixtures. |

## Do not casually redesign

Keep the modular Go control plane, separate privileged Agent, canonical
desired/observed/rendered/history separation, stable logical identities,
explicit root enrollment, independent management policy, bounded journals,
one-shot apply admission and deterministic render/evaluation semantics.
Do not replace the working JSON recovery/identity stores with SQLite as a
cosmetic consistency change. SQLite metadata already exists; any migration must
preserve durability, rollback, compatibility and secret boundaries.

The accepted storage trust model uses an explicitly provisioned private local
master key and per-record authenticated encryption. Missing keys or journals
must not be silently regenerated. Root/full-disk compromise is outside the
protection provided by a key stored beside the encrypted installation state.

## Configuration/application model

Engine and Agent have separate durable coordination responsibilities. Candidate
IDs/outcome receipts reconcile uncertain IPC outcomes; a matching config hash
alone is not proof that a transaction committed. The Agent independently
validates configuration, ownership, generated artifacts and kernel scope.

Safe Apply journals before mutation, has bounded apply/verify/recovery, and uses
an independent confirmation watchdog. Browser disconnect does not cancel it.
Restart rolls back pending changes; it never implicitly confirms them.

The static managed-routing adapter admits firewall/NAT, private routes and
policy rules on unchanged static interfaces. It deliberately refuses dynamic
DHCP/RA, changed links/main routes, DNS and management intent. It stages kernel
changes under `duckos_apply_guard`, replaces owned networkd files, reloads and
checks actual state, and releases the forwarding barrier only after management
and ownership verification. Recovery reverses files, daemon and captured kernel
state; unknown drift remains an error rather than a flush/overwrite operation.

The boot phase restores pending before-state files before networkd starts but
does not clear the journal or claim runtime recovery. The packaged required
oneshot, persistent dependency, loaded ordering, successful state and exact
unit are checked by `VerifyRoutingBootGuard`. Installer installation does not
enable that dependency. Runtime startup and cold-boot packet safety still need
integration; this code must not be represented as a finished boot guarantee.

## Security invariants

- API/GUI/CLI input is untrusted; RBAC is server-side and Agent validation is independent.
- Root authority names physical ID/name/MAC bindings, VLAN names, firewall table,
  private tables and policy ranges. Manager-wide networkd reload is a separate
  explicit grant; ownership of one NIC does not imply global reload authority.
- Dedicated DNS identity cannot read the API token via group membership. Its
  config and lease directories are separate. Installation does not start DNS/DHCP.
- Service state, firewall permission and external reachability are distinct facts.
- A forward-hook barrier does not control local output, L2 bridging, XDP/tc or
  offloaded bypass paths; verification must account for them before activation.
- Management is independent of WAN/LAN roles, including single-NIC Edge nodes.
- No cloud dependency, mandatory telemetry or HTTPS interception in core operation.
- Logs, diagnostics and history yield to routing, state and recovery resources.
- Active is never overwritten in place; signed update/migration/Previous recovery
  remain required architecture, not implemented updater behavior.

## Target UX/model contracts

Intent-driven, visible and reversible management is a product-wide invariant,
not a VPN-specific convenience layer. Every major subsystem shares the
[cross-subsystem workflow contract](V1_SCOPE.md#automation-policy-and-client-lifecycle):
Engine-derived mechanical work, human-readable impact preview, normal-table
generated objects with ownership, contextual tests/actions, and GUI/CLI parity.

Easy and Full share one model. Full retains fixed dense navigation and declarative
intent, visible generated objects/owners and exact technical inspection. It is
not a Linux-console mode. Hardware/backend adapters contain Debian specifics;
high-level object semantics must remain portable to later Linux distributions.

Read ADRs 0005–0011 for baseline/runtime contracts, 0012–0014 and 0018–0023 for
networking, 0015–0017 for exports/Vault/catalog, and the checkpoint's boot-recovery
documentation before changing those areas. Earlier target prose that still says
these choices are entirely undecided is superseded by the ADRs and this snapshot.
