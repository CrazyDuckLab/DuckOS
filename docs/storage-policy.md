# DuckOS storage policy

Status: foundation specification; not implemented or verified
Last updated: 2026-09-07

Storage safety is a platform invariant, not a housekeeping feature. Optional
observability or convenience data must never silently consume the space needed
to route traffic, recover configuration, or roll back a release.

This document describes required behavior. It does not claim that quota,
cleanup, pressure monitoring, database maintenance, or alerting exists yet.
The words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative.

## Goals

- Keep the currently running system viable.
- Preserve a known-working release and compatible recovery state.
- Prevent logs, metrics, backups, uploads, caches, and temporary files from
  growing without bound.
- Detect pressure early, reclaim predictably, and degrade optional work before
  core networking.
- Protect low-end flash devices from excessive writes without imposing a
  separate DuckOS product model.
- Make every automatic deletion explainable, auditable, and safe after power
  loss.

## Protected set and priority order

The storage controller MUST classify owned data and account for unowned data on
the same filesystems. Higher-priority data cannot be automatically sacrificed
to preserve lower-priority data.

| Priority | Class | Examples | Default pressure behavior |
|---:|---|---|---|
| 1 | Running system | Base OS space needed to boot and run, committed Active DuckOS release, live canonical state | Protected; never pressure-cleaned |
| 2 | Release rollback | Previous known-working release and its compatible state | Protected; never deleted to stage/update |
| 3 | Configuration recovery | Current config, transaction journal, at least one verified recovery backup/snapshot | Protected; retention beyond the protected minimum is bounded |
| 4 | Security-critical data | Required audit/security records and trust metadata | Strongly protected, but bounded and exportable; exact legal/product retention is open |
| 5 | Normal operational logs | System, network, service, and update logs | Age, size, and free-space bounded |
| 6 | Historical metrics/events | Metrics, flight recorder, network timeline, speed history | Sampled/compacted and bounded |
| 7 | Debug data | Verbose logs, packet-free diagnostic traces, temporary support material | Short-lived; first persistent class to expire |
| 8 | Cache and temporary data | Downloads, update input after commit, staging remnants, regenerated cache, scratch files | Lowest priority; reclaimed first when safe |

“Protected” does not mean the data may grow without limit. It means its minimal
viable set has precedence and requires a deliberate recovery workflow rather
than generic cleanup. Security records and extra backups need explicit quotas
so they do not eventually threaten priorities 1–3.

The protected set MUST include enough independently validated metadata to pair
each retained release with compatible canonical state. A directory named
`previous` without verified content is not a rollback set.

## Core invariants

1. Every DuckOS-owned growing dataset MUST have an owner, accounting method,
   and bounded retention policy.
2. A bound MUST cover both normal retention and emergency free-space behavior.
3. Stable release retention is normally Active plus at most one Previous.
4. Previous MUST NOT be deleted merely to make a new update fit.
5. The only copy of canonical state, the transaction journal, and the minimum
   recovery backup MUST NOT be generic cleanup targets.
6. “Unlimited” may disable age-based expiry for an eligible dataset; it MUST NOT
   disable the emergency reserve or permission to protect the router.
7. Cleanup MUST start with the lowest-priority safe data and MUST respect data
   dependencies and in-flight transactions.
8. An update MUST reserve peak required space before staging. Final installed
   size is not an adequate estimate.
9. Automatic snapshots, backups, logs, metrics, events, support bundles, and
   failed-update artifacts MUST NOT accumulate forever.
10. Cleanup and compaction MUST themselves be crash-safe and must not require
    more temporary space than is available.
11. A filesystem reaching 100% usage is a preventable platform failure, not a
    normal retention trigger.
12. Storage pressure actions MUST be visible and rate-limited; alert storms
    must not create additional pressure.
13. DuckOS MUST account per relevant filesystem/mount. Free bytes on one mount
    cannot satisfy a reserve on another.
14. Unrecognized or administrator-created data MUST NOT be silently deleted by
    DuckOS.

## Capacity model

For each writable filesystem, the policy engine needs to track:

- total, available, and filesystem-reserved capacity;
- DuckOS-owned bytes by data class and owner;
- unowned/unknown consumption;
- growth rate and cleanup debt;
- write health where the platform exposes reliable signals;
- currently reserved transaction space;
- a hard emergency floor and a larger operational safety reserve.

The exact formula and defaults remain open. The calculation MUST use bytes as
well as percentages: a percentage alone is misleading on both tiny flash and
large disks. Reserved space must cover realistic worst-case writes needed to
keep core services and recovery operational.

An allocation request is admitted only when:

`available - existing reservations - request peak - safety reserve >= 0`

The eventual implementation must account for filesystem allocation behavior,
copy-on-write, sparse files, database journals/WAL, archive expansion, and
metadata overhead rather than treating logical file size as exact disk use.

Reservations MUST be durable for operations such as updates, migrations,
backup restore, and support-bundle creation. They need an owner, expiration or
transaction binding, and crash-recovery semantics so abandoned reservations do
not persist forever.

## Retention model

Each growing dataset SHOULD declare the applicable dimensions below. The
strictest reached bound triggers compaction or deletion of eligible data:

- maximum age;
- maximum bytes;
- maximum item/row/file count;
- minimum free-space floor;
- sampling or cardinality limit;
- minimum protected history;
- deletion/compaction batch size;
- retention priority and dependency constraints.

Typical UI presets may include 1, 7, 30, and 90 days plus Custom, but defaults
must be derived and validated later. Categories may differ: audit, security,
normal service, debug, traffic metadata, metrics, and backups do not require the
same history.

Selecting **Unlimited** for an eligible log means “no age deletion during
normal conditions.” The UI MUST present a serious confirmation warning. Size,
write-rate, and emergency protection remain active even after confirmation.
Some high-cardinality or safety-critical datasets MAY refuse Unlimited when no
safe bounded representation exists.

Automatic debug mode SHOULD expire by default, for example after a selected
diagnostic window. The exact choices are an open UX decision.

## Storage pressure state machine

Exact byte/percentage thresholds are intentionally not frozen. The proposed
70/80/90/95 percent values are initial concepts, not qualified defaults. The
implementation MUST combine available bytes, percentage, growth, filesystem,
platform profile, and pending reservations, and MUST use hysteresis when
leaving a state.

| State | Meaning | Required policy response |
|---|---|---|
| `NORMAL` | Operational reserve is healthy | Enforce routine retention incrementally |
| `NOTICE` | Sustained growth or reserve trend needs attention | Surface forecast; run normal eligible cleanup without disrupting jobs |
| `WARNING` | Operational reserve is threatened | Reclaim expired cache/debug/metrics, warn once with rate limiting, restrict avoidable large work |
| `CRITICAL` | Core/recovery reserve may soon be threatened | Stop verbose logging, pause heavy scheduled jobs, block update staging and other large allocations, aggressively reclaim eligible low-priority data |
| `EMERGENCY` | Hard free-space floor is threatened or crossed | Protect network core; retain only bounded essential writes; clean safe low-priority data; reject nonessential mutations and produce an actionable alert |
| `RECOVERING` | Space has returned but integrity/cleanup must be checked | Verify databases, journals, rollback set, and services; leave pressure state only after hysteresis and checks pass |

Pressure transitions and actions MUST be persisted compactly enough to survive
reboot without creating a log loop. If automatic cleanup cannot recover safe
space, DuckOS reports which filesystem and classes consume it and requests an
operator decision. It does not guess that unknown data is disposable.

## Reclamation order and safety

Within dependency constraints, pressure cleanup proceeds from priority 8
toward priority 4:

1. orphaned or expired scratch files and regenerable cache;
2. committed update uploads and safe-to-delete staging remnants;
3. expired debug logs and support-bundle workspaces;
4. expired/compacted historical metrics and event timelines;
5. expired normal operational logs;
6. backup/snapshot history beyond the protected recovery minimum;
7. security/audit history only according to its explicit bounded policy.

The engine MUST remeasure between bounded batches and stop when the target
reserve is restored. It MUST NOT follow unsafe symlinks, cross ownership
boundaries, delete files merely because they are old, or remove artifacts used
by an active transaction. Cleanup operations need idempotent journal/retry
behavior where partial deletion or compaction could leave inconsistent state.

Database maintenance deserves special treatment:

- retention deletes rows in bounded batches;
- indexes and cardinality are controlled from schema design;
- WAL/journal growth is monitored;
- compaction/vacuum is scheduled only with adequate temporary space and an
  interruption-safe plan;
- corruption detection and recovery are tested;
- deleting a database file is never the generic answer to pressure.

## Updates, releases, and migrations

The update space reservation covers peak simultaneous use of:

- Active and its live compatible state;
- the protected rollback set;
- compressed package input;
- expanded candidate release;
- candidate config/database copy;
- migration scratch and journal/WAL growth;
- verification logs;
- safety reserve.

After successful commit, Active/Previous roles rotate and only then may the
older obsolete release, uploaded package, and staging workspace be reclaimed.
If cleanup fails, the update can remain committed, but the debt is visible,
bounded, retried safely, and included in the next preflight. Failed update data
is retained only under diagnostic bounds after recovery is verified.

See [`update-system.md`](update-system.md) and
[ADR-0004](adr/0004-update-active-previous.md).

## Backups and configuration history

Backups are structured DuckOS data, not unbounded `/etc` archives. Automatic
snapshots need count, age, and byte limits; an eventual policy may combine last
N, daily N, and weekly N. Exact defaults are open.

Retention MUST preserve:

- current canonical state and schema metadata;
- the config/state associated with the rollback release;
- the configured minimum recovery backup;
- transaction history required to recover an incomplete change.

An encrypted backup that includes secrets must stay encrypted during storage,
retention, export, and deletion workflows. Backup verification, not file
presence alone, determines whether a recovery copy is usable. Restoring older
state remains a Safe Apply operation and may require hardware remapping.

## Logs, metrics, and diagnostics

- Logs are bounded by age and size under normal presets.
- Security and audit data may have longer retention but remain bounded.
- Metrics aggregation reduces old resolution and cardinality where useful.
- Flight Recorder and Network Timeline store metadata rather than packet
  payload by default.
- Heavy speed-test history, captures, or support bundles require explicit
  quotas; scheduled throughput tests are paused under pressure.
- Repeated failures use deduplication/backoff so they cannot fill storage.
- Sensitive fields are redacted before persistence, not only at export time.
- Exported support bundles have input-window and final-size limits.

Retention deletion itself SHOULD leave a compact audit summary (class, range,
reason, policy, reclaimed bytes), not one record per deleted row/file.

## Architecture and media profiles

One policy model applies to all hardware, but its defaults are selected from
measured storage and capabilities rather than a hardcoded product fork.

| Target class | Expected constraints to design for | Evidence required before defaults are claimed safe |
|---|---|---|
| ARMv7 / ODROID XU4 reference | Approximately 2 GB RAM class, commonly constrained/older eMMC or SD, slower compression and database maintenance, high sensitivity to write amplification | Long-duration growth, abrupt-power, low-space, cleanup, update-peak, and wear-conscious workload tests on real hardware/media |
| ARM64 SBC/VM | Can range from small flash devices to fast virtual disks; architecture alone predicts little | Real capacity/media discovery plus physical SBC and VM pressure tests |
| amd64 VM/server | Often larger/faster but may use a tiny virtual root disk or thin provisioning | Filesystem-aware VM and physical tests; no “large machine means unlimited” assumption |

Capability inputs may include filesystem type, mount topology, available bytes,
device/virtual media identity, discard support, reliable health signals, RAM,
CPU, and measured cleanup cost. DuckOS MUST clearly distinguish observed
capabilities from inferred recommendations.

Flash-oriented profiles SHOULD favor lower write frequency, batching, sampled
metrics, bounded WAL, conservative debug defaults, and fewer retained high-
resolution data points. Such tuning must not weaken the protected rollback or
audit invariants.

## Ownership and operator experience

Each managed data path needs a documented owner and deletion API. DuckOS only
automatically deletes artifacts it owns and can classify. The UI/API should
show, per filesystem:

- capacity and safety reserve;
- owned usage by class;
- unknown/unmanaged usage;
- current pressure state and trend;
- active reservations and cleanup debt;
- retained Active/Previous/recovery artifacts;
- what was reclaimed and why;
- a safe, actionable next step when automatic recovery is insufficient.

No failure should be reduced to “disk full.” The administrator needs to know
which allocation was refused, whether routing/recovery remains safe, and which
data can be exported or removed.

## Verification requirements

Storage behavior is not proven by a unit test alone. Qualification needs
evidence, per supported architecture/media profile, for:

- each retention dimension and priority ordering;
- Unlimited warning and continued emergency protection;
- concurrent writers during cleanup;
- crash/reboot during file deletion, database retention, compaction, and
  reservation changes;
- a filled disposable filesystem, including inode exhaustion where relevant;
- unknown/unmanaged data consuming the filesystem;
- failed update cleanup and abandoned reservations;
- refusal to delete Previous for an update;
- core networking continuity under critical pressure;
- bounded alert/log behavior during repeated failures;
- database corruption detection/recovery;
- long-running growth and write-amplification measurements on the ARMv7
  reference target.

Evidence levels are defined in [`testing.md`](testing.md). No such qualification
is claimed at repository bootstrap.

## Open decisions

- Per-filesystem operational and hard-reserve formula, default thresholds, and
  hysteresis.
- Exact default retention by dataset and hardware/media profile.
- Which security/audit minimums are product policy versus operator policy.
- Storage accounting mechanism and ownership manifest.
- Database journaling, compaction, corruption recovery, and backup design.
- Durable reservation and cleanup-journal formats.
- Handling of read-only filesystems, overlay/copy-on-write filesystems, thin
  provisioning, network storage, and separate data mounts.
- What device-health/wear signals are trustworthy across supported hardware.
- User-authorized emergency recovery when the protected set itself no longer
  fits; this must be separate from routine cleanup/update admission.
- Encryption and secure deletion expectations for secrets-bearing data.
- Performance and write-budget thresholds for ARMv7, ARM64, and amd64
  qualification.
