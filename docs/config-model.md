# Canonical configuration model

Status: foundation constraints, storage/schema design pending
Decision: [ADR-0003](adr/0003-config-source-of-truth.md)
Requirements source: [`requirements/product.md`](requirements/product.md)

This document defines what DuckOS configuration means and the safety properties
of its lifecycle. It intentionally does not choose a serialization format,
database schema, or storage engine. Those choices are open decision F-003 and
must be recorded before persistent Engine implementation.

## Two scoped sources of truth

- **Git and signed release inputs** are authoritative for product source,
  schemas, migrations, packaging definitions, documentation, and provenance.
- **The canonical configuration store on one DuckOS installation** is
  authoritative for that installation's desired managed state.

These meanings are complementary. Rendered `/etc` files, daemon-specific
configuration, nftables state, routes, links, and other kernel state are derived
or observed representations—not the DuckOS business model.

## State classes

DuckOS must keep the following classes conceptually distinct even if a future
storage design co-locates some of them:

| Class | Meaning | Authority and lifecycle |
|---|---|---|
| Product definition | Schemas, migrations, renderer behavior, defaults | Versioned in Git and release artifacts |
| Committed desired state | Last administrator-approved, durable DuckOS intent | Canonical runtime source for reconciliation and reboot |
| Candidate desired state | A copy proposed by a transaction or migration | Isolated until validation, verification, and commit; disposable on rollback |
| Secret material | Credentials, private keys, tokens, protected URLs | Stored through a separate protected mechanism and referred to by opaque ID |
| Rendered artifacts | Daemon configs and other host material generated from intent | Replaceable outputs; must carry ownership/provenance where practical |
| Observed state | Hardware, link, address, route, daemon, and health facts | Time-bounded evidence from the host; never silently promoted to intent |
| Transaction journal | Durable stage, before/candidate revision, plan, outcome | Used for recovery, audit, and coherent rollback |
| History/audit | Who changed what, when, why, diff, and result | Bounded according to storage and security policy |
| Metrics/events/logs | Operational evidence | Bounded, sampled where needed, never required to reconstruct canonical intent |

## Model invariants

1. A committed revision is complete and schema-valid; partial canonical writes
   are never exposed as the current configuration.
2. Every object has a stable opaque identity separate from its mutable display
   name and from any current Linux identifier.
3. References use stable identities and are checked for existence, type, and
   lifecycle before commit.
4. IP address and prefix types are address-family aware. Core objects do not
   encode an IPv4-only assumption.
5. Logical port roles such as `WAN1` and `LAN1` are separate from hardware
   binding evidence such as MAC, PCI/USB path, or current interface name.
6. Secrets are not embedded in ordinary desired-state documents, diffs, logs,
   process arguments, or support bundles.
7. All managed writers—including UI, external API, CLI, restore, migration, and
   automation—use the same Engine mutation and transaction path.
8. Linux artifacts can be recreated deterministically from a committed revision,
   protected secrets, platform support, and explicit runtime inputs.
9. A candidate never destroys the only working revision or only recovery copy.
10. Retention and cleanup cannot remove the state needed to boot, roll back, or
    explain an unresolved transaction.

## Object graph

The requirements identify stable product concepts including:

- hardware and attachment: `HardwareDevice`, `Interface`, `PortRole`,
  `WiFiRadio`, `SSID`, `Modem`;
- connectivity: `WAN`, `Network`, `DHCPService`, `DNSPolicy`;
- enforcement and routing: `FirewallRule`, `NATRule`, `RouteProfile`,
  `RouteGroup`, `TrafficPolicy`, `ServicePack`;
- identity and endpoints: `Device`, `User`, `Role`, `Site`, `Portal`,
  `Certificate`;
- providers and operations: `Subscription`, `SubscriptionNode`, `HealthProbe`,
  `SpeedTest`, `NotificationPolicy`;
- lifecycle and safety: `ConfigTransaction`, `Release`, `Backup`,
  `StoragePolicy`.

This list is a vocabulary and design input, not an approved schema. Object
boundaries, cardinalities, required fields, and normalization must be designed
with the first vertical use cases and versioned schema tests. Introducing a new
object requires a clear owner and lifecycle; not every UI panel merits a
persistent object.

### Identity and references

IDs must remain stable across rename, reorder, render, and normal reboot. Names
are human-facing and may require uniqueness only within a defined scope.
External or observed identifiers are attributes/bindings, not primary product
identity.

Deleting an object with inbound references must be rejected or handled by an
explicit, validated replacement/fallback plan. For example, removal of a route
node must not silently turn a fail-closed policy into direct access.

### Logical interfaces and hardware mapping

Configuration expresses intent against logical roles and capabilities.
Installation and restore maintain a distinct mapping to discovered hardware.
When evidence is insufficient or hardware changes, DuckOS asks for mapping
instead of guessing destructively. Current interface names are observations and
may change across reboot or hardware replacement.

### Desired defaults

Defaults are part of the versioned schema/product definition, not values
silently injected by a renderer. A default that affects security or
connectivity must be visible in the resulting candidate and review. The initial
first-flight LAN intent is `192.168.88.1/24`, but activation still depends on
hardware mapping, conflict checks, and Safe Apply.

## Schema envelope requirements

The eventual representation must carry enough information to identify:

- configuration schema version;
- immutable revision identity or sequence;
- the installation/configuration identity where needed;
- object graph and references;
- creation/change provenance appropriate for audit;
- compatibility or feature requirements needed before activation.

This is not a commitment to JSON, YAML, Protocol Buffers, SQL tables, or a
single-file layout. An illustrative field name in documentation must not become
a compatibility contract until a schema ADR and checked-in schema approve it.

## Validation pipeline

Validation is layered so an early failure cannot partially modify the host:

1. **Decode and schema validation** — representation is well formed, version is
   understood, values and required fields are structurally valid.
2. **Semantic validation** — object-local invariants, address-family rules,
   ranges, and allowed combinations hold.
3. **Reference/dependency validation** — targets exist; deletion, ordering, and
   ownership constraints hold.
4. **Conflict analysis** — overlapping subnets, duplicate services/ports,
   policy shadowing, impossible routes, invalid VLANs, and certificate/domain
   conflicts are detected.
5. **Capability validation** — the mapped hardware, kernel, installed support,
   storage, and resources can execute the intent.
6. **Safety analysis** — management lockout, fail-open changes, rollback space,
   recovery path, service disruption, and confirmation requirements are made
   explicit.
7. **Renderer/tool validation** — candidate artifacts pass native validators
   where available before a live service is disturbed.

Passing structural validation does not mean the configuration is safe to apply.
Warnings must be distinguishable from hard errors and cannot silently waive a
non-negotiable invariant.

## Transaction lifecycle

The requirements describe this conceptual progression:

```text
BEFORE -> CANDIDATE -> APPLYING -> VERIFYING -> READY_FOR_CONFIRMATION
                                                        |          |
                                                        v          v
                                                   COMMITTED   ROLLED_BACK
```

Some changes can commit after verification without interactive confirmation;
dangerous networking and management changes require an armed rollback and an
appropriate confirmation window. The confirmation timer begins only after the
required services are ready—not while a slow device is still applying work.

Before applying, DuckOS must durably record enough information to recognize and
recover an interrupted transaction. The detailed journal, state-transition
atomicity, lock model, retry/idempotency rules, and timeout policy are not yet
selected.

### Concurrency

The implementation must prevent lost updates and overlapping unsafe applies.
Revision preconditions, serialization, or another mechanism may be used, but
the choice belongs in the persistence/transaction design. A stale UI must not
overwrite a newer committed revision without an explicit conflict.

### Apply plans

An apply plan is produced by the Engine and contains semantic ordered steps,
preconditions, expected effects, verification, and rollback information. It is
not a shell script and cannot contain caller-controlled executable fragments.
Plans should be inspectable enough for diagnostics and audit while keeping
secret values out of their representation.

### Verification and commit

Verification compares expected effects with fresh observations at the relevant
layers. A service being `active` is not enough when the intended outcome is
DHCP, DNS, routing, firewall, or management reachability. Expected offline
operation must not fail solely due to lack of public Internet.

Commit makes the complete candidate the sole current desired revision and
records the transaction result atomically. Cleanup follows commit and is safe
to retry; cleanup failure must not retroactively erase the working or rollback
state.

### Rollback

Rollback restores a coherent previous managed revision and its compatible
rendered/runtime state. It does not independently roll back arbitrary files.
Rollback is verified and its failure is surfaced as a high-severity state with
an actionable recovery path. Exact handling of Debian packages and shared
systemd state is open decision F-007.

## Reboot and power-loss recovery

On startup, DuckOS inspects durable committed and transaction state before
reconciling the host. It must be able to distinguish at least:

- a complete committed revision;
- a candidate never applied;
- an apply or verification interrupted midway;
- a candidate awaiting confirmation when management disappeared;
- a rollback interrupted or not verified.

Recovery must be idempotent, avoid a broken-candidate boot loop, preserve
diagnostic evidence within bounds, and prefer the last-known-good managed state
when safe. Atomic file replacement uses temporary write, appropriate flush, and
rename semantics where the chosen persistence layer requires them.

## Migrations

Schemas are explicitly versioned. A migration:

```text
committed old revision
  -> isolated copy
  -> ordered migration to a supported schema
  -> schema + semantic + capability validation
  -> candidate activation and health verification
  -> commit or discard/rollback
```

The only working copy is never irreversibly migrated in place. The previous
release retains a compatible last-known-good configuration until the new
release has committed. Migrations require forward, failure, interruption, and
rollback-compatibility tests on every supported architecture relevant to their
code path.

## Rendered state, ownership, and drift

Every renderer must define:

- which files, units, objects, or kernel namespaces DuckOS owns;
- how an artifact is generated and validated;
- permissions and secret exposure;
- atomic replacement/reload behavior;
- how reality is verified;
- how rollback and reboot restoration work.

The installer must establish a supported ownership model rather than race
NetworkManager, systemd-networkd, or an existing administrator setup. Until
F-006 is resolved, no document should imply that DuckOS may take over arbitrary
existing networking safely.

When a managed artifact differs from its expected derivation, DuckOS reports
drift and offers inspection, restoration, or a safe supported import where one
exists. It neither silently accepts an external edit as canonical intent nor
blindly overwrites an unknown administrator change without explanation.

## Secrets

Canonical objects refer to secrets by opaque identifiers and can expose only
safe metadata such as presence, type, and rotation state. Secret material is
excluded from ordinary reads and diffs, masked in UI, redacted from logs and
support bundles, and never passed on a process command line.

Encrypted-at-rest mechanism, key source, recovery, unattended boot, rotation,
and encrypted backup behavior are open decision F-004. Until it is resolved,
no implementation may claim a complete Vault design.

## History, backup, and retention

Each committed or failed transaction records who/what initiated it, time,
before/after revision identities, a secret-safe diff, reason/context where
available, stage, verification, rollback, and final outcome. History uses
bounded retention, while unresolved recovery evidence is protected from normal
cleanup.

A backup contains canonical configuration, schema version, relevant state and
metadata, and optionally secrets only under explicit encryption. Restore is a
candidate transaction and may require interface remapping; prior validity on
different hardware is not proof of present safety.

## Open design work

Before persistent configuration implementation, resolve and record:

- serialization, on-disk layout, and SQLite's exact role (F-003);
- transaction journal, revision, locking, and crash-recovery protocol (F-003);
- Vault trust root and unattended boot (F-004);
- ownership of host networking and import/drift boundaries (F-006);
- rollback scope for host packages and shared system state (F-007);
- retention defaults and capacity reservations in the storage policy;
- compatibility contract between configuration schema, Engine, Agent, and
  release versions.
