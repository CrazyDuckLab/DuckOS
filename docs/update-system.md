# DuckOS update system

Status: target specification with implemented update foundations; not a qualified beta.2 upgrade path

**Beta.2 is fresh-install-only. No beta.1 → beta.2 in-place upgrade is qualified.**
The public tarballs are installer payloads, not signed `.duck` updates.
Last updated: 2026-09-07

This document defines the safety contract for DuckOS software updates. It does
not claim that an updater, package format, signing service, migration runner, or
release repository exists yet. Until tests produce evidence under
[`testing.md`](testing.md), every behavior below is a requirement rather than an
implementation claim.

The words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative.

## Scope

The first update source is a trusted `.duck` package uploaded locally by an
administrator. Automatic Internet updates are out of the initial scope. A
future official repository MUST feed the same package into the same Upgrade
Engine; it must not introduce a second activation or migration path.

Subscription and service-pack refreshes are data updates owned by their
respective subsystems. They are not DuckOS software updates and do not inherit
permission to alter the installed DuckOS release.

DuckOS 1.x is a software platform on a supported Debian installation. It does
not own the kernel, bootloader, or base distribution. The consequences of that
boundary are explicit in [Host-level Debian changes](#host-level-debian-changes).

## Terms and durable objects

- **Release payload**: immutable, architecture-specific DuckOS executables and
  assets stored in a release directory.
- **Active**: the release recorded by the last committed update transaction as
  the normal service release.
- **Candidate**: a separately staged release being validated, migrated, or
  provisionally activated. A candidate is not yet Active.
- **Previous**: the last known-working committed release retained as the normal
  rollback target. A fresh installation may not have one.
- **Rollback set**: Previous (or the pre-update Active while a transaction is in
  progress), its compatible canonical configuration/state, and the metadata
  needed to reactivate and verify it.
- **Update transaction journal**: crash-durable record of the transaction ID,
  package identity, source and target releases, architecture, current phase,
  selected config copies, checksums, health results, and cleanup obligations.
- **Commit**: the point after candidate activation and health verification at
  which the candidate becomes Active and the former Active becomes Previous.

Release directories are conceptually placed below
`/opt/duckos/releases/<release-id>/`, with an atomic selection mechanism such as
`/opt/duckos/current`. The exact release identifier and pointer implementation
remain open, but activation MUST be atomic from the perspective of service
startup and crash recovery.

## Non-negotiable invariants

1. An update MUST NOT overwrite the active release in place.
2. A candidate MUST be staged beside the active release and completely
   validated before activation.
3. Stable state normally retains Active plus one Previous known-working
   release. A first installation is the allowed zero-Previous case.
4. Previous MUST NOT be deleted merely to create room for a candidate.
5. The only working canonical configuration MUST NOT be migrated in place.
6. Activation MUST NOT commit until configuration-aware health checks pass.
7. Any failure after the live selection changes MUST enter coherent rollback;
   individual files must not be restored independently.
8. Rollback is not complete until the old release and compatible state have
   started and recovery health checks pass.
9. An interrupted transaction MUST be discoverable and recoverable after
   reboot. It MUST NOT produce an activation loop.
10. Wrong-architecture, corrupt, unsigned/untrusted, incompatible, or
    insufficient-space packages MUST be rejected before live state changes.
11. Update input, staging, migration workspaces, and obsolete releases MUST be
    bounded. Cleanup MUST happen only after the data is no longer required for
    rollback or diagnosis.
12. The update path MUST preserve a viable management/recovery path.
13. Update logs and support artifacts MUST redact secrets and obey the storage
    policy.
14. Every architecture receives the same state machine and safety contract;
    architecture-specific payloads do not create product forks.

These invariants implement [ADR-0004](adr/0004-update-active-previous.md).

## Package envelope

The eventual `.duck` envelope is expected to carry, at minimum:

- an authenticated manifest;
- an architecture-specific payload;
- payload and metadata checksums;
- signature metadata and a verifiable trust chain;
- source-version and target-version compatibility constraints;
- input and output configuration schema versions;
- required space, including temporary and rollback reserve estimates;
- declared dependencies and required capabilities;
- migrations and tightly constrained lifecycle actions, if any.

Manifest parsing MUST be bounded and resistant to malformed input. Archive
extraction MUST reject path traversal, special-file abuse, unsafe permissions,
duplicate/ambiguous entries, decompression bombs, and content not covered by
the authenticated manifest. Verification MUST occur before extracted content
can affect a live service.

Arbitrary root shell hooks are not part of the foundation contract. If a later
design permits lifecycle actions, their vocabulary, privileges, inputs,
outputs, timeouts, audit record, architecture support, idempotency, and rollback
semantics require a separate security decision.

The exact serialization, compression, signing algorithm, key rotation model,
and offline trust bootstrap are open decisions.

## Durable state machine

The implementation MAY expose friendlier UI labels, but it MUST durably
distinguish at least the following semantic states. Transitions that can alter
recovery decisions are journaled before the associated side effect.

| State | Meaning | Live selection changed? | Safe failure action |
|---|---|---:|---|
| `IDLE` | No update transaction; committed Active is serving | No | None |
| `RECEIVING` | Bounded local upload or future repository fetch | No | Discard incomplete input |
| `VALIDATING` | Envelope, signature, checksums, compatibility, architecture, resources, and space are checked | No | Reject and retain Active |
| `STAGING` | Candidate payload is materialized in a new immutable location | No | Remove incomplete candidate when safe |
| `MIGRATING` | A copy of canonical state is migrated and validated for the candidate | No | Discard candidate copy; retain original |
| `READY_TO_ACTIVATE` | Payload and candidate state are complete and activation prerequisites pass | No | Retain Active; candidate can be retried or removed |
| `ACTIVATING` | Atomic release/state selection and managed service transition are in progress | Possibly | Resolve journal, then roll back unless successful activation is provable |
| `VERIFYING` | Candidate is provisionally serving while required probes run | Yes, provisionally | Roll back coherently |
| `COMMITTING` | Verified candidate is being made the committed Active and retention metadata is advanced | Yes | Reconcile journal idempotently; never guess from a single symlink |
| `ROLLING_BACK` | Pre-update release and compatible state are being restored | Yes | Continue recovery; do not retry candidate automatically |
| `VERIFYING_RECOVERY` | Restored release is serving while recovery probes run | Restored | Finish rollback or enter operator-required recovery |
| `FAILED_SAFE` | Active was not changed, or verified rollback restored service; failure is recorded | No/current old release | Explain and allow bounded diagnostics/retry |
| `RECOVERY_REQUIRED` | Neither candidate health nor verified rollback can be established automatically | Unknown/degraded | Preserve evidence and the safest reachable management path; require operator action |

`COMMITTED`, `ROLLED_BACK`, and `REJECTED` may be recorded as transaction
outcomes while the system returns to `IDLE`. A transaction ID MUST make retries
and recovery actions idempotent. The implementation MUST reject concurrent
software update transactions.

## Transaction sequence

### 1. Receive without trust

The upload is untrusted until verification completes. Reception MUST enforce
configured size limits and must not consume the emergency reserve. Merely
uploading a file cannot invoke a hook or select a release.

### 2. Preflight and authenticate

Before changing live state, the engine MUST verify:

- package structure and every authenticated byte;
- package signature and trust policy;
- exact target architecture and compatible DuckOS/schema range;
- required capabilities and supported Debian/kernel baseline once defined;
- downgrade policy and replay protections once defined;
- current system/update journal consistency;
- availability and readability of the rollback set;
- safe storage requirement from the storage policy;
- absence of unresolved host-level mutations.

Failure here leaves Active unchanged.

### 3. Stage immutably

The engine materializes a new release directory using atomic file practices.
Partial staging is never a selectable release. Files, ownership, permissions,
and authenticated digests are checked after materialization. Executables must
not be taken from an upload directory.

### 4. Migrate a copy

The engine snapshots or copies the canonical configuration and only migrates
the candidate copy:

`old state -> candidate copy -> migration -> schema validation -> semantic validation`

The old release retains a compatible last-known-good state. Migrations MUST be
deterministic or otherwise auditable, bounded, and tested across every
supported upgrade path. A failed migration leaves the live state untouched.
Secrets require protected handling and MUST NOT leak into migration logs.

### 5. Activate provisionally

After writing the durable intent, the engine atomically selects the candidate
release and candidate state, transitions managed services, waits for actual
readiness, and starts expected-state health checks. Process existence or a
systemd `active` state alone is insufficient.

### 6. Verify expected behavior

Health evaluation is based on the router's intended configuration. Required
layers include, when configured:

- release identity and transaction/state integrity;
- privileged Agent contract and readiness;
- Engine, API, and Web UI readiness;
- canonical configuration load and schema consistency;
- management reachability;
- LAN gateway behavior;
- DHCP, DNS, firewall, routing, and WAN service health;
- external connectivity only where the desired state says it is expected.

Probes MUST distinguish a critical internal failure from an optional module or
an intentionally offline WAN. Probe deadlines and readiness budgets are
capability-aware: ARMv7 cannot be judged by a timer chosen only for fast amd64
hardware. The user confirmation timer, where required, starts after the system
is ready, not while it is still booting or migrating.

### 7. Commit, then clean

Only after required verification succeeds may the transaction:

1. make the candidate the committed Active;
2. designate the former Active and its compatible state as Previous;
3. retire the still-older Previous, if one exists;
4. enqueue safe cleanup of obsolete staging, uploads, and migration work.

Cleanup failure does not retroactively make a healthy update fail, but it MUST
be visible, bounded, safely retryable, and considered by future space checks.

## Power loss and reboot recovery

Every nonterminal state needs an explicit reboot rule. At startup the recovery
controller reads the durable journal and validates actual release/state
artifacts before starting normal service.

- Before live selection changes, incomplete input or staging can be quarantined
  or removed; the committed Active remains authoritative.
- During ambiguous activation, the controller must use the journal and verified
  artifacts, not timestamps or directory names. If successful candidate
  activation cannot be proved, it rolls back.
- During verification, reboot does not silently promote the candidate. The
  system either resumes bounded verification under an explicit policy or rolls
  back; this policy remains to be finalized.
- During rollback, restart continues rollback idempotently.
- A failed candidate MUST be marked so boot does not repeatedly activate it.
- If recovery cannot establish a coherent release/config pair, the system
  enters `RECOVERY_REQUIRED` and preserves the best available local management
  path and diagnostics.

Crash consistency details—journal format, `fsync` boundaries, directory sync,
pointer reconciliation, and behavior across filesystems—must be designed and
fault-injection tested before this guarantee is considered implemented.

## Storage preflight and retention

The update engine delegates capacity policy to
[`storage-policy.md`](storage-policy.md). Preflight accounts for, on each
relevant filesystem:

- committed Active;
- the protected rollback set;
- compressed input and expanded candidate payload;
- candidate configuration/state copy;
- migration scratch space and database copy-on-write overhead;
- journal, logs, and verification artifacts;
- filesystem and emergency safety reserve;
- incomplete cleanup obligations from earlier transactions.

The estimate must include peak simultaneous usage, not just final installed
size. If safe capacity is unavailable, the update does not start. It MUST NOT
delete Previous to make the estimate pass.

At stable state, obsolete releases do not accumulate. The intended retention
is Active plus at most one Previous. Failed candidate data may be retained
briefly for diagnosis only under an explicit size/count/age bound and never at
the expense of the rollback set or emergency reserve.

## Host-level Debian changes

Release-pointer rollback and candidate-config rollback do **not** by themselves
reverse mutations to shared host state. Examples include:

- Debian package installation, removal, or maintainer scripts;
- kernel, bootloader, initramfs, firmware, or libc changes;
- users/groups, capabilities, sysctls, udev rules, and files outside managed
  release/config locations;
- global systemd unit definitions or presets;
- irreversible database or third-party service state changes.

ADR-0004 therefore accepts only the Active/Previous release and compatible
configuration model. It makes no false atomicity claim for the items above.

Until a follow-up ADR selects and validates a host-mutation strategy, the
rollback-guaranteed update path MUST reject a package that requires unresolved
host-level changes. Preinstalled dependencies may be checked during preflight;
checking them is not permission to mutate them. Candidate strategies include:

1. keep release payloads self-contained and make host prerequisites an
   installer/platform responsibility;
2. permit only a small declarative, reversible, idempotent host-operation set;
3. use a platform snapshot mechanism where it is genuinely available and
   verified;
4. define a coordinated Debian package transaction with explicit forward and
   backward plans.

No option is selected here. Before 1.0 qualification, the project must define
which host state DuckOS owns, what can change in an update, how power loss is
handled, and what recovery evidence proves reversal.

## Architecture handling

The manifest uses an exact normalized architecture identifier. An `armv7`
package must never be accepted as `arm64`, and neither is interchangeable with
`amd64`. A universal product model does not imply universal binaries.

| Concern | ARMv7 reference target | ARM64 targets | amd64 targets |
|---|---|---|---|
| Package payload | Native 32-bit ARM build; exact ABI baseline is open | Native 64-bit ARM build; ABI baseline is open | Native x86-64 build; microarchitecture baseline is open |
| Typical update constraint | Tight RAM, slow CPU, and eMMC/SD write budget; streaming/bounded verification is essential | Wide range from constrained flash SBC to VM/server; capabilities must be measured | Often faster/larger, but storage and atomicity cannot be assumed |
| Time budgets | Capability-aware and expected to be the conservative reference | Derived from measured target capability | Derived from measured target capability, not a license for unbounded work |
| Native dependencies | Every dependency and migration tool needs real ARMv7 evidence | Needs native ARM64 evidence | Needs native amd64 evidence |

Exact minimum Debian versions, kernel capabilities, filesystems, free-space
reserves, and performance budgets are not yet qualified. See
[`hardware-support.md`](hardware-support.md).

## Security and audit

Update authorization is a privileged RBAC action and SHOULD require recent
authentication. The unprivileged API cannot gain an arbitrary root command
channel through update metadata or hooks. The privileged boundary revalidates
semantic operations and authenticated package identity.

The audit trail records actor, time, source, package/release identity, manifest
digest, transaction phases, migrations, health decisions, rollback, cleanup,
and final outcome. It never records private keys, tokens, passwords, or secret
configuration values. Logs and support bundles are bounded and redacted.

Signing keys, offline root trust, key rotation/revocation, downgrade
authorization, recovery-key behavior, and release provenance need dedicated
design and security review.

## Required evidence before claiming support

At minimum, update qualification must cover every supported architecture and
upgrade path with authenticated artifacts and retained evidence for:

- successful update and correct Active/Previous rotation;
- corrupt archive, malformed manifest, invalid signature, and wrong
  architecture rejection;
- unsupported source/schema migration rejection;
- insufficient-space rejection without deleting rollback data;
- migration failure before activation;
- Engine/Agent/API/UI failure during verification;
- power loss or process kill at every durable state boundary;
- reboot during provisional activation and rollback;
- candidate health failure followed by verified recovery;
- intentionally offline configuration that must not cause a false rollback;
- failed and retried cleanup under storage pressure;
- no secret disclosure and no unbounded diagnostic growth.

Build success or a systemd `active` result is not update evidence. Evidence
levels and artifact requirements are defined in
[`testing.md`](testing.md).

## Open decisions

- Exact `.duck` manifest schema, compression, signature algorithms, and trust
  bootstrap/rotation.
- Exact release ID and atomic pointer/journal implementation.
- Initial supported source/target version and schema migration graph.
- Resume-versus-rollback rule after reboot during candidate verification.
- Critical, conditional, and optional health-probe policy per configuration.
- Adaptive deadlines and administrator confirmation policy.
- Host-level Debian mutation and rollback model.
- Ownership boundaries for systemd units, service users, firmware, and other
  shared host state.
- Downgrade, replay, and emergency recovery authorization.
- Filesystem assumptions, reserve formula, and cleanup bounds.
- Whether the initial platform requires a clean Debian installation and the
  exact Debian/kernel/ABI baselines per architecture.
