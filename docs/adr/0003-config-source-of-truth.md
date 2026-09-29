# ADR-0003: Canonical configuration as source of truth

- Status: Accepted
- Date: 2026-09-07
- Scope: product source and per-installation managed state

## Context

The phrase “source of truth” applies to two different scopes in DuckOS. Product
source and reproducible build inputs belong in Git, while a running router
needs durable, structured desired state that survives reboot, can be migrated,
and can drive verification and rollback. Treating `/etc` files, command output,
or current kernel objects as the business model would couple DuckOS to tool
syntax, lose intent, and make coherent transactions and restores unreliable.

## Decision

1. The Git repository and signed release inputs are authoritative for product
   source, schemas, migrations, build and packaging definitions,
   documentation, and release provenance. Important product changes do not
   exist only as manual edits on a router.
2. Each DuckOS installation has a schema-versioned canonical configuration
   store that is authoritative for its committed desired managed state.
3. Candidate desired state is isolated from the committed revision. A complete
   candidate passes layered validation, is applied as a coherent transaction,
   is verified, and is either committed atomically or rolled back. Partial
   canonical writes never become current state.
4. Rendered `/etc` files, daemon configuration, systemd state, nftables rules,
   links, addresses, routes, and other kernel/runtime objects are derived or
   observed state. They are not silently promoted to canonical intent.
5. Duck Engine is the sole semantic writer of canonical desired state. UI,
   external API, CLI, restore, migration, and automation use the same Engine
   mutation and transaction path.
6. Canonical objects use stable opaque identities and address-family-aware
   types. Logical interface roles are distinct from mutable Linux interface
   names and hardware-binding evidence.
7. Secrets are stored through a separate protected mechanism and are referenced
   from ordinary configuration by opaque identity. Plain secret material is
   excluded from normal configuration reads, diffs, logs, process arguments,
   and support bundles.
8. Schema versions and ordered migrations are part of the product definition.
   Migration operates on an isolated copy, preserves a compatible
   last-known-good revision, and commits only after validation and health
   verification.
9. Observed state retains source, time, and uncertainty as appropriate. Drift
   is reported and resolved explicitly; external edits are neither silently
   adopted nor blindly overwritten without an ownership policy.

## Consequences

- A supported clean Debian system plus official DuckOS inputs can reproduce
  managed state from explicit intent rather than undocumented host edits.
- Renderers and Agent adapters are replaceable implementation details provided
  that they preserve canonical semantics, ownership, verification, and
  rollback.
- Backup is structured, versioned DuckOS data rather than an indiscriminate
  archive of `/etc`; restore uses migration, mapping, validation, and Safe
  Apply.
- Reconciliation after reboot begins from the last committed revision and
  durable transaction evidence, not from whichever partial host artifacts are
  present.
- History, logs, metrics, snapshots, and rendered artifacts remain bounded and
  cannot be the only copy required for recovery.
- Manual root changes outside declared DuckOS ownership cannot receive a false
  guarantee of import or rollback.

## Deliberately unresolved

This ADR fixes the meaning and ownership of canonical state but does not close
the persistence work in F-003. Before persistent Engine implementation, decide:

- serialization, schema representation, storage engine, and on-disk layout;
- SQLite's exact role, if any;
- revision identity, concurrency control, transaction journal, durability,
  locking, crash recovery, and integrity checks;
- compatibility rules across configuration, Engine, Agent, and release
  versions;
- secret trust root and unattended-boot/recovery design (F-004);
- supported host-network ownership and safe import/drift boundaries (F-006);
- rollback scope for Debian packages and shared systemd state (F-007).

The detailed lifecycle invariants are maintained in
[`../config-model.md`](../config-model.md); storage technology must be selected
against those invariants rather than choosing a database first.
