# ADR-0004: Use side-by-side Active and Previous releases

- Status: Accepted
- Date: 2026-09-07
- Scope: DuckOS software releases and compatible canonical state

## Context

DuckOS updates real router and management state. An in-place overwrite can
leave a machine with a mixture of old and new binaries, an incompatible
configuration, or no reliable recovery path after a process failure, reboot,
or power loss. Keeping every historic release indefinitely would preserve some
fallback data but violates the bounded-storage contract, especially on the
ODROID XU4 ARMv7 reference target.

The project needs one update model for local `.duck` uploads and any future
official repository, across `armv7`, `arm64`, and `amd64`. It must also be
honest about the boundary between DuckOS-owned releases/configuration and
shared Debian host state.

## Decision

DuckOS software updates use immutable, side-by-side release directories and a
durable transaction. They never overwrite the serving release in place.

In stable state the normal retained set is:

- **Active**: the committed release currently selected for normal service;
- **Previous**: at most one last known-working committed release, paired with
  canonical configuration/state compatible with it.

A fresh installation may have no Previous. During an update, a separately
staged **Candidate** and its migrated copy of canonical state may coexist with
Active and Previous. Candidate is not Active until the update passes required
health verification and commits.

The transaction follows this semantic order:

1. receive the package as untrusted, bounded input;
2. authenticate and validate package, exact architecture, compatibility,
   capabilities, rollback set, and peak free-space requirement;
3. stage the Candidate in a new, non-selectable location;
4. migrate and validate a copy of canonical state, never the only live copy;
5. durably record activation intent and provisionally select Candidate plus its
   compatible state using an atomic selection mechanism;
6. start and verify configuration-aware Engine, Agent, API, UI, management,
   and applicable network health;
7. on success, commit Candidate as Active and the former Active as Previous;
8. only after commit, retire the still-older Previous and bounded temporary
   artifacts;
9. on failure after activation, coherently restore the former release and its
   compatible state, then verify recovery.

The durable journal and actual authenticated artifacts determine recovery
after reboot. A half-finished candidate is never inferred to be committed from
a directory name, timestamp, process status, or a single symlink. Recovery and
cleanup operations must be idempotent, and a failed candidate must not create
an automatic boot loop.

Previous MUST NOT be deleted to create room for a new update. If Active,
Previous, Candidate, configuration copies, migration workspace, diagnostic
allowance, and safety reserve cannot coexist at peak usage, the update is
rejected before live selection changes.

The same Upgrade Engine and state machine accept both the initial local upload
source and a future repository source. Automatic Internet DuckOS updates are
not part of the initial implementation.

## Health and commit boundary

Process existence or systemd `active` status is insufficient to commit. Health
is evaluated against expected configuration and distinguishes a critical
internal failure from an optional module or an intentionally offline WAN.
Architecture/resource-aware readiness deadlines begin from actual readiness,
not merely process spawn.

Rollback is complete only when the former release and compatible state are
restored and recovery health is established. If neither candidate nor rollback
can be verified, the system enters an explicit operator-required recovery state
and preserves the safest available local management path and bounded evidence.

Exact probes, confirmation rules, timing budgets, journal layout, atomic
pointer mechanism, and fsync boundaries are implementation decisions constrained
by this ADR and [`../update-system.md`](../update-system.md).

## Host-level Debian boundary

This decision guarantees only the model for DuckOS release payloads and their
compatible DuckOS-owned canonical state. Selecting an older release does not
by itself undo changes to Debian packages, maintainer scripts, global systemd
state, users/groups, firmware, kernel, bootloader, initramfs, sysctls, udev, or
other shared host state.

**Host-level Debian mutation and rollback remains an open decision.** Until a
follow-up ADR chooses and validates a strategy, an update that requires such an
unresolved mutation cannot claim the rollback guarantee defined here and must
be rejected from the rollback-guaranteed path. Candidate strategies, none yet
selected, include:

- self-contained release payloads with host prerequisites owned by the
  installer/platform contract;
- a tightly bounded declarative and reversible host-operation vocabulary;
- a verified platform snapshot mechanism where available;
- a coordinated Debian transaction with explicit forward, reverse, crash, and
  recovery semantics.

This open decision must be resolved before 1.0 qualification if any supported
upgrade needs host mutation.

## Consequences

### Positive

- Active is left intact through receive, validation, staging, and migration.
- Release and configuration rollback form one coherent recovery unit.
- One state machine serves all architectures and present/future package
  sources.
- Retaining only Active and Previous bounds steady-state release storage.
- Wrong-architecture and insufficient-space failures occur before live change.
- Durable transaction state makes reboot and fault-injection behavior
  specifiable and testable.

### Costs and constraints

- Peak update space exceeds final installed size because rollback, Candidate,
  migrated state, and reserve coexist.
- Configuration migrations must preserve a compatible old copy and support all
  declared upgrade paths.
- Activation, journaling, filesystem durability, health probes, and cleanup
  need architecture- and filesystem-specific failure testing.
- A first installation has no Previous and therefore needs a separately
  specified installer/recovery contract.
- Failed candidate diagnostics require explicit count, size, and age bounds.
- This ADR does not solve base-OS rollback.

## Alternatives rejected

### Overwrite Active in place

Rejected because interruption can create a mixed, unverifiable release and
destroy the only known-working recovery target.

### Keep every installed release

Rejected because unbounded growth conflicts with the storage policy and the
minimum ARMv7 target. More history is not a substitute for verified rollback.

### Delete Previous when staging needs space

Rejected because an update must not purchase capacity by destroying its
recovery path. The safe result is refusal with an actionable space report.

### Separate manual and repository updaters

Rejected because two activation/migration paths would split safety behavior and
evidence. Sources may differ; the authenticated package and Upgrade Engine do
not.

### Treat external Internet reachability as universal health

Rejected because a correctly configured offline router would falsely roll
back. Health must be configuration-aware and layered.

## Required verification

Implementation of this ADR cannot advance beyond specification without tests
for successful rotation, wrong architecture, corrupt/invalid packages,
unsupported migration, low space, process death, reboot/power interruption at
every durable state boundary, candidate health failure, verified rollback,
cleanup failure/retry, bounded evidence, and intentionally offline operation.

Cross-building architecture artifacts is not runtime evidence. Emulated runs
must remain labelled emulated, native VM runs must remain distinct from
physical hardware, and XU4 support requires named physical XU4 evidence. See
[`../testing.md`](../testing.md) and
[`../hardware-support.md`](../hardware-support.md).

## Open follow-up decisions

- Host-level Debian mutation and rollback strategy.
- Exact release identifier, directory/pointer scheme, journal format, and
  crash-durability protocol.
- `.duck` manifest/archive/signature and trust-rotation design.
- Supported source/target/schema migration graph and downgrade policy.
- Resume-versus-rollback behavior after reboot during verification.
- Configuration-aware probe set, adaptive deadlines, and administrator
  confirmation policy.
- Cleanup and failed-candidate diagnostic retention bounds.
- First-install recovery model when Previous does not yet exist.
