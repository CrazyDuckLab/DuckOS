# DuckOS testing and evidence policy

Status: evidence policy with executable foundation checks
Last updated: 2026-09-07

DuckOS controls real network and recovery state. An HTTP 200 response, a green
build, or a process reported as `active` is never sufficient evidence that a
feature works safely. This document defines how claims become supportable.

It is a policy for test infrastructure and release qualification. Current
commands are `make check` and `make build-all`. Virtual labs and physical
qualification are not implied. The words **MUST**, **MUST NOT**, **SHOULD**,
and **MAY** are normative.

## Principles

1. Test the observable outcome and actual traffic path, not only API plumbing.
2. Test failure, rollback, reboot, low storage, and power interruption as normal
   scenarios for critical features.
3. Record architecture, hardware, OS/kernel, topology, artifact digest, config,
   and result for every compatibility claim.
4. Cross-compilation is build evidence, not runtime or hardware evidence.
5. Emulation is useful but does not replace native physical execution where
   timing, drivers, storage, offload, or power loss matter.
6. ARMv7, ARM64, and amd64 share one behavior contract; each nevertheless needs
   architecture-specific evidence.
7. IPv6 is part of the test model from the start. An IPv4-only test cannot prove
   a dual-stack claim.
8. Mocks and fixtures may isolate logic, but a mocked Linux/network result must
   never be reported as implemented system behavior.
9. Tests must not use real credentials or the owner's production
   infrastructure. Repository fixtures contain synthetic secrets and reserved
   example data only.
10. A flaky, skipped, manually assumed, or irreproducible result is not a pass.
11. Test environments are DEV/TEST assets. Destructive tests MUST be isolated
    from production and from unrelated projects/devices.
12. Evidence is attached to a precise build/configuration and becomes stale
    after relevant changes.

## Evidence levels

Evidence is assigned **per claim, scenario, architecture, and release
artifact**, not to “DuckOS” globally. Levels are cumulative for that precise
claim; a higher label requires the relevant lower-level evidence unless a test
record explicitly explains why a level is inapplicable.

| Level | Name | Minimum evidence | What it does not prove |
|---:|---|---|---|
| E0 | Specified | Requirement, owner, invariants, acceptance criteria, and open dependencies are documented | Any implementation or runtime behavior |
| E1 | Build/static/unit | Reproducible target build plus applicable formatting, lint/static/security checks and deterministic unit/property tests | Linux integration, service readiness, packets, reboot, or hardware compatibility |
| E2 | Linux component integration | Native or explicitly identified emulated Linux execution against real component boundaries, persisted state, and constrained Agent/Engine semantics | End-to-end router behavior or physical drivers |
| E3 | Virtual router path | Disposable two-NIC router VM/namespace lab with a separate client exercising real DHCP/DNS/firewall/routing/WAN traffic and relevant reboot scenarios | Physical NIC/radio/modem/storage behavior |
| E4 | Native target/hardware | The scenario passes on a named native architecture and hardware/media profile, including required reboot and realistic load checks | Destructive recovery/security qualification unless executed |
| E5 | Failure and security qualified | Fault injection, storage pressure, interruption, rollback/recovery, privilege/security abuse cases, and bounded-observability checks pass for the claim | Release readiness across other claims/architectures |
| E6 | Release qualified | Frozen-scope acceptance matrix passes on all mandatory architectures/topologies with reviewed evidence, no unresolved release blocker, and documentation/support path complete | Future hardware or untested configurations |

E-levels complement, rather than replace, the product status vocabulary:
`SPECIFIED`, `IMPLEMENTED`, `UNIT TESTED`, `INTEGRATION TESTED`, `ARMv7 TESTED`,
`ARM64 TESTED`, `AMD64 TESTED`, `REBOOT TESTED`, `FAILURE TESTED`, `SECURITY
REVIEWED`, `UI COMPLETE`, `DOCUMENTED`, and `READY`.

At repository bootstrap, the documents can establish E0 only. A checkbox or
manually edited status cannot advance evidence without a linked test record.

## Evidence record

Every retained result used for a product or hardware claim MUST identify:

- claim/requirement and test case ID;
- Git commit and exact release/package digest;
- configuration/schema version and sanitized fixture identity;
- build provenance sufficient to reproduce the artifact;
- target architecture, ABI/userland, and execution mode (native, virtualized,
  or emulated);
- hardware, NIC/radio/modem/storage identifiers and relevant firmware/drivers;
- Debian release, kernel, filesystem, and dependency versions;
- lab topology, IP-family mode, and upstream assumptions;
- test commands/automation version and timestamps;
- expected result, observed result, duration, and resource measurements where
  relevant;
- sanitized logs, packet metadata/counters, health output, and rollback state
  needed to explain the result;
- pass/fail/blocked/skipped outcome and known limitations;
- reviewer or automated attestation where release policy requires it.

Secrets, private keys, subscription URLs/tokens, production addresses, and
packet payloads are excluded or redacted. Evidence storage is bounded under
[`storage-policy.md`](storage-policy.md).

## Target matrix

The initial required architecture targets are:

| Target | Intended role in validation | Current evidence |
|---|---|---|
| ARMv7, ODROID XU4 reference device | Minimum/resource reference; native driver, storage, reboot, and destructive testing | E0 target only; no build/runtime evidence claimed |
| ARM64 Debian VM with two NICs (`duck-dev-01`) | GitHub-fed Linux build/test router target; never an uncommitted Remote SSH development workspace | E0 planned lab only |
| Separate client VM (`DUCK-CLIENT`) | Real client behind the isolated LAN | E0 planned lab only |
| ARM64 physical SBC class | Native ARM64 hardware, storage, NIC, and capability validation | E0 target class only |
| amd64 Debian VM/device | Native x86-64 build/runtime and performance/driver coverage | E0 target class only |

The exact Debian releases, kernels, hypervisor, NIC models, and physical ARM64
and amd64 devices remain open. The matrix must be versioned once selected.

One expected lab path is:

`DUCK-CLIENT -> isolated LAN NIC -> duck-dev-01 -> WAN NIC -> controlled upstream`

Development and repository editing occur in the local macOS checkout. When a
later roadmap stage authorizes this target, `duck-dev-01` obtains an exact
committed revision or traceable artifact from GitHub. Stage 1 does not install,
provision, configure, or otherwise modify that host.

The lab MUST prevent accidental DHCP, RA, routing, or destructive traffic from
escaping into the developer's normal LAN. External Internet access is used only
when a scenario requires it; deterministic local upstream fixtures are
preferred for failure reproduction.

## Test layers

### Specification and contract tests

Before implementation, a feature has canonical state, ownership, privilege
boundary, validation, state machine, rollback/reboot behavior, storage bounds,
IPv4/IPv6 behavior, security considerations, and human-visible errors defined.
Schema and API/IPC compatibility tests follow those contracts.

### Unit, property, and fuzz tests

Pure decisions—validation, conflict detection, migrations, parsers, retention,
policy matching, manifest handling, and state transitions—SHOULD have
deterministic unit/property tests. Untrusted parsers and privileged-boundary
inputs require fuzzing or equivalent adversarial coverage with resource limits.

Unit tests avoid wall-clock and random nondeterminism. Time, entropy, hardware
inventory, and probes are injected through explicit test boundaries.

### Component integration

Run real storage and process boundaries where meaningful: SQLite/journaling,
Unix IPC, permission checks, service config validators, filesystem atomicity,
and systemd behavior on Linux. Tests MUST distinguish a simulated dependency
from a real one in their names and evidence.

### Virtual network integration

The router and client use isolated interfaces and packet paths. A networking
feature is not integrated until the test observes relevant client behavior,
state/counters, failure reporting, and cleanup. Network namespaces may provide
fast coverage; VM tests add reboot, systemd, kernel, and installer realism.

### Physical target tests

Physical runs cover native CPU/ABI, NIC/radio/modem drivers, offload behavior,
storage media, thermals, resource limits, reboot, and power-loss recovery. Board
model alone is insufficient: evidence names the actual device and peripherals.

### UI and accessibility tests

UI tests verify real backend state, authorization, localization keys, error
disclosure/redaction, safe confirmation flows, keyboard/touch behavior, and
graceful degradation. Fixtures may support visual development, but screenshots
of fixture data do not prove feature implementation.

### Security tests

Security coverage includes authentication/session/RBAC, WAN management denial,
Agent input revalidation, unauthorized IPC access, update/package parsing,
secret handling, support-bundle redaction, path/file permissions, dependency
review, and abuse/rate limits. A security review records assumptions and
unresolved risks; a scanner alone is not a review.

### Performance and endurance tests

Measure idle CPU/RAM, throughput, latency, rule/table scaling, DNS performance,
database growth, log write rate, cleanup cost, and long-running stability. ARMv7
is the conservative reference. Budgets must be selected from measured data and
then enforced as regressions; no budgets are fixed by this document.

## Feature acceptance template

A significant feature's acceptance record SHOULD answer:

1. What canonical state and schema represent it?
2. Which component and privilege boundary own each operation?
3. What validates it and what conflicts are rejected?
4. Is apply atomic or transactionally recoverable?
5. What happens after reboot, process death, power loss, and disk-full?
6. How does rollback preserve a coherent state?
7. What are its IPv4, IPv6, and dual-stack outcomes?
8. What secrets exist, how are they supplied, and what is redacted?
9. What grows on disk and what bounds it?
10. Which ARMv7, ARM64, and amd64 evidence records support the claim?
11. What does the UI/API/CLI report on success, failure, and recovery?
12. Can Duck Doctor or another diagnostic path explain the failure?
13. Which security and management-lockout cases were tested?
14. Did a real client or observable consumer experience the intended result?

The frozen 1.0 scope matrix determines which fields are mandatory per
capability. Critical networking, security, update, backup/restore, and recovery
features require the complete set unless an explicit reviewed exception exists.

## Required foundation scenarios

### Network path

For applicable features, verify from a separate client:

- link and interface-role discovery;
- DHCP lease acquisition/renewal and IPv6 RA/DHCPv6 behavior;
- DNS resolution and failure reporting;
- firewall default deny, established/related behavior, and counters;
- NAT/routing to the controlled WAN/upstream;
- management reachability only from authorized networks;
- actual policy-selected route, tunnel/WAN failure semantics, and recovery;
- persistence across service restart and router reboot.

Same-L2 isolation is claimed only in a topology where the router/AP/switch can
enforce or observe it.

### Safe Apply and configuration

Exercise valid, invalid, conflicting, and lockout-risk candidates; change LAN
address/VLAN/firewall/WAN/DNS/routing; close the browser without losing the
actual management path; lose the actual path; let confirmation expire; reboot
in every durable state; and verify whole-transaction rollback plus recovery
alias behavior where supported.

### Software update

The matrix includes success, corrupt archive, malformed manifest, wrong
architecture, invalid signature, unsupported migration, migration corruption,
insufficient space, failed cleanup, service failure, management loss, process
kill, and reboot/power interruption at every journal boundary. It verifies
Active/Previous rotation and the explicit limit of host-level rollback. See
[`update-system.md`](update-system.md).

### Storage

Use disposable bounded filesystems to exercise byte and inode exhaustion,
unknown/unowned data, concurrent writers, retention ordering, abandoned
reservations, WAL/database growth, compaction interruption, Unlimited mode,
alert deduplication, and network-core continuity. Never fill the developer's or
tester’s real root filesystem. See [`storage-policy.md`](storage-policy.md).

### Backup, restore, and hardware change

Validate encrypted backup integrity, secret handling, schema migration,
corruption rejection, bounded retention, restore through Safe Apply, and
explicit logical-interface remapping when hardware differs. A prior valid
configuration is not assumed safe on a new topology.

## Destructive and fault-injection catalog

Before DuckOS 1.0 release qualification, controlled tests MUST cover at least:

- kill Engine/Agent or a managed service during apply/update/rollback;
- abrupt reboot and power loss at persistent state boundaries;
- fill storage and exhaust inodes;
- corrupt or truncate canonical/config/update/database artifacts;
- remove WAN, DNS, tunnel, route node, NIC, or storage at relevant moments;
- invalidate a subscription while policies still reference nodes;
- apply bad firewall, VLAN, LAN address, and management-access changes;
- make time invalid for TLS, signatures, schedules, and audit;
- induce slow hardware/readiness and bounded restart-loop behavior;
- fail notification/export paths without blocking core recovery.

Hardware power cycling is performed only on designated sacrificial TEST devices
with a documented recovery method and backups. Fault injection in VMs runs
first where it can reduce risk, but cannot replace physical media evidence.

## CI and scheduled validation

The eventual CI baseline MUST build and test `armv7`, `arm64`, and `amd64` from
one repository. It should include formatting, lint/static checks, unit/schema/
migration tests, dependency/security checks, package validation, and selected
integration tests.

CI results MUST label native, cross-compiled, emulated, and virtualized runs.
Architecture build jobs protect portability but do not grant an `ARCH TESTED`
status. Physical lab and destructive/endurance tests may run outside untrusted
pull-request contexts and publish signed/sanitized results back to the evidence
system.

Untrusted changes MUST NOT receive production secrets, signing keys, privileged
network access, or control of physical devices. Test artifacts and logs have
retention limits.

## Failure reporting and triage

A failed test records:

- exact failing layer and last coherent product state;
- whether rollback ran and whether recovery was verified;
- impact on management and network service;
- sanitized evidence needed to reproduce it;
- whether the issue is deterministic, intermittent, environment-specific, or
  not yet classified.

Retries do not erase the original failure. A test may be quarantined only with
an owner, issue, reason, and deadline; a quarantined mandatory release test
remains a release blocker unless explicitly reviewed.

## Release gate

The first externally distributed product is DuckOS 1.0.0, not a partially
qualified public MVP. Internal development builds, integration builds, and RCs
are allowed but remain internal.

E6 requires a frozen 1.0 MUST matrix, exact supported-platform matrix, signed
candidate artifacts, migration paths, recovery evidence, security review,
bounded storage evidence, complete operator documentation, and no unresolved
critical defect. “Works on my VM” cannot satisfy the gate.

## Open decisions

- Test framework/toolchain and location/schema for machine-readable evidence.
- Exact Debian/kernel/hypervisor and hardware matrix.
- Which scenarios run on every change, nightly, before RC, and before 1.0.
- Performance, endurance, storage-write, readiness, and recovery-time budgets.
- Physical lab orchestration and secure device reservation/reimaging.
- Packet observation tooling and privacy/redaction policy.
- Reproducible build/provenance and test-result attestation format.
- Fuzzing duration/corpus retention and architecture coverage.
- Browser/accessibility/localization matrix.
- Release authority, exception policy, and evidence-expiration rules.
