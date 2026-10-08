# DuckOS architecture

Status: target architecture; current implementation snapshot is recorded in the source tree (`internal/`) and ADRs in `docs/adr/`

The handoff snapshot and subsequent ADRs supersede historical future-tense status
notes here. Product-wide intent-driven workflows, visible owned generated objects,
stable Full navigation and GUI/CLI parity follow the frozen V1 contract documented in [`docs/product-spec-1.0.md`](product-spec-1.0.md).
Requirements source: [`product-spec-1.0.md`](product-spec-1.0.md)

This document defines the target component boundaries and the invariants that
implementation must preserve. [ADR-0005](adr/0005-executable-observation-foundation.md)
defines the historical observation foundation. Beta.2 adds qualified WireGuard
and Xray runtime paths; other flows below remain target
architecture; see [`docs/testing.md`](testing.md) for the qualification-evidence framework.

## Beta.2 implementation snapshot

Official release artifacts cover AMD64, ARM64 and ARMv7 (`GOARCH=arm GOARM=7`).
Only AMD64 received the complete final two-node live matrix. The Agent has zero
effective Linux capabilities and private authenticated IPC. WireGuard mutations
use `duck-wg-launcher` with CAP_NET_ADMIN only; FRR uses `duck-frr-launcher`.
Xray 26.3.27 uses Vault-backed secret materialization. Port Mapping includes
backend / Trusted LAN / nftables lifecycle / TTL / revoke / cleanup, without
UPnP IGD, SSDP/M-SEARCH, NAT-PMP or PCP protocol frontends.
See [release notes](release-notes.md), [compatibility](compatibility.md) and
[qualification evidence](../beta2-final-report.md). The broader requirements
below remain target architecture where not explicitly qualified.

## Product boundary

DuckOS 1.x is one local-first network operating platform delivered on top of a
supported Debian installation. It is not a separate Linux distribution and does
not own a custom kernel or bootloader in 1.x. The same product, configuration
model, API shape, release mechanism, and user experience must span ARMv7, ARM64,
and amd64.

ODROID XU4/ARMv7 with roughly 2 GiB of RAM is the minimum reference target. More
capable hardware may unlock or scale expensive features, but does not create a
product fork. Capability and resource observations, rather than board model
names, drive availability, concurrency, retention, and warnings.

The architecture is deliberately a small modular control plane plus a narrow
privileged Agent. Linux remains the packet-processing data plane.

## Architectural invariants

1. The browser, UI, and ordinary API never edit Linux configuration or invoke
   arbitrary privileged commands.
2. Product mutations pass through Engine semantics and the same validation and
   transaction rules, whether initiated by UI, API, or CLI.
3. The privileged Agent exposes only versioned, semantic operations and
   validates every request again at the privilege boundary.
4. Versioned canonical desired state is distinct from observed runtime state and
   rendered Linux artifacts. `/etc` is not the DuckOS business model.
5. Dangerous changes are planned, validated, applied as one coherent
   transaction, verified, and either committed or rolled back.
6. Recovery after process failure, reboot, power loss, management-path loss, and
   disk pressure is part of normal design, not a later enhancement.
7. IPv4 and IPv6 are first-class model inputs even when individual feature
   delivery is staged.
8. Interfaces are addressed through stable logical identities and roles; no
   feature may assume names such as `eth0` or `eth1`.
9. Logs, metrics, events, snapshots, backups, and temporary artifacts are
   bounded and subordinate to the running system and recovery data.
10. DuckOS functions locally without a required cloud service. It does not use
    HTTPS interception as a convenience mechanism.

## System context

```text
 Administrator                         Automation / integrations
      |                                         |
      v                                         v
 Browser + Duck Web                    Versioned Duck API
      |                                         |
      +-------------------+---------------------+
                          v
                    Duck API boundary
                          |
                          v
                      Duck Engine <------------- Duck CLI
                  (product semantics)             (same semantics)
                          |
                 versioned semantic IPC
                          |
                          v
               privileged Duck Agent
                          |
          +---------------+----------------+
          v               v                v
     Linux kernel    managed daemons    systemd / host
       data plane      and files          facilities
```

The arrows show command dependency, not one process per box. API and Engine
ship in one unprivileged process for the first slice (ADR-0005, F-001). Agent
is a separate privilege boundary. HTTP/JSON v1 over a Unix socket with Linux
peer UID authentication serves observation; mutation IPC is still future work.
The semantic, non-shell contract is accepted in
[ADR-0002](adr/0002-agent-privilege-boundary.md).

## Control plane and data plane

DuckOS is primarily the control and management plane. It models intent,
validates it, produces an apply plan, asks the Agent to perform bounded host
operations, and verifies the result. Packet forwarding, filtering, queuing, and
encapsulation run in the Linux kernel or explicitly managed networking daemons.

This separation has two consequences:

- loss of the Web UI must not itself stop established forwarding;
- an API success response is never sufficient evidence that a networking
  feature works—the real traffic path and reboot restoration must be tested.

## Component responsibilities

### Duck Web

- Presents real API and system state; fixtures are permitted only in explicitly
  marked design/test contexts.
- Performs usability checks and explains risk, but is not a security or
  validation boundary.
- Never reads or writes host configuration directly.
- Uses capability information and progressive disclosure instead of assuming
  hardware or exposing every advanced control by default.

React and TypeScript compile to static assets using Vite. CSS tokens define
Midnight Glass surfaces, typography, accents, spacing and reduced motion.

### Duck API

- Provides the versioned external management contract and enforces
  authentication, authorization, request limits, and safe serialization.
- Translates external requests into Engine use cases rather than Linux
  primitives.
- Exposes no arbitrary command execution endpoint.
- Reports current state, failure stage, rollback outcome, and actionable errors
  without leaking secrets.

API and Engine topology, authentication technology, and public versioning
details remain open; the dependency direction does not.

### Duck Engine

- Owns product semantics, canonical desired-state mutations, validation,
  dependency/conflict analysis, planning, transactions, and orchestration.
- Resolves stable product objects and logical interface roles into an apply
  plan using observed hardware and capabilities.
- Coordinates verification, confirmation, commit, rollback, history, and
  incomplete-transaction recovery.
- Supplies the same mutation semantics to API and CLI.
- Never delegates policy decisions to the Agent merely because the Agent is
  privileged.

The preferred shape is a modular monolith. Additional processes or external
infrastructure require a demonstrated isolation, lifecycle, or scaling need and
an ADR.

### Configuration subsystem

- Maintains schema-versioned canonical desired state and revisions.
- Keeps secrets out of ordinary configuration representations and logs.
- Supports candidate copies, migrations, diffs, history, backup/restore, and
  last-known-good recovery.
- Separates desired, observed, rendered, and historical data as specified in
  [`config-model.md`](config-model.md).

The storage engine and serialization format are intentionally not selected yet
(F-003). SQLite is a candidate for appropriate state/history uses, not an
implementation claim or a blanket choice for all data.

### Capability Engine and Resource Guard

- Observes architecture, CPU, memory, storage, NICs, buses, radios, modems,
  drivers, kernel features, and supported offload.
- Produces facts and measured resource limits, including provenance and
  uncertainty where applicable.
- Lets modules adapt worker counts, feature intensity, retention, and warnings.
- Does not infer support solely from a board name or silently convert one DuckOS
  into a different product edition.

### Duck Agent

- Is the only ordinary DuckOS component allowed to perform the narrow set of
  privileged host mutations.
- Authenticates its local caller, checks protocol compatibility, revalidates
  inputs, applies explicit semantic operations, and returns structured,
  sanitized results.
- Uses native APIs such as netlink where justified and tightly controlled tools
  where native integration is not practical.
- Does not accept shell fragments, executable paths, or arbitrary environment
  from an API caller.

See [ADR-0002](adr/0002-agent-privilege-boundary.md) and ADR-0005. Implemented
identities, socket permissions, sandbox and read protocol are defined there;
native Linux security verification remains a separate evidence gate.

### Managed service adapters

Adapters render and validate configuration for explicitly selected components
such as a DNS/DHCP daemon, Xray, a reverse proxy, or systemd units. An adapter:

- maps canonical objects to a candidate artifact;
- invokes the component's validation mode where available;
- describes dependencies, restart/reload effects, verification, and rollback;
- must not turn command output parsing into the product's canonical model.

Specific component selections are made per subsystem after maintenance,
license, Debian availability, architecture, security, and resource review.

### Background jobs and events

A coherent lightweight job model is required for health probes, cleanup,
backups, subscription refresh, certificate renewal, and similar work. Jobs must
be bounded, observable, cancellable where appropriate, persistent when loss
would be unsafe, and retried with backoff and stable jitter.

A lightweight event mechanism may decouple modules. Kafka, RabbitMQ, or another
external broker is not justified for the router core. Whether an event bus is
needed, and its persistence semantics, remain open until concrete consumers are
designed.

## Principal flows

### Read and observation

```text
Agent/native observers -> observed facts -> Engine interpretation -> API -> UI/CLI
                                      |
                                      +-> drift and health evaluation
```

Observed facts include a timestamp, source, and uncertainty where relevant.
They do not silently replace desired state.

### Managed mutation

```text
request
  -> authorize
  -> create candidate from committed revision
  -> structural + semantic + dependency + safety validation
  -> capability/conflict analysis
  -> deterministic apply plan and rollback information
  -> persist transaction intent
  -> Agent applies bounded operations
  -> verify expected state and management path
  -> optional confirmation window
  -> commit candidate OR restore the coherent previous state
  -> audit and bounded cleanup
```

The final transaction state machine and durability protocol require their own
design. The safety properties above are requirements, not evidence of an
existing transaction implementation.

### Startup and recovery

At startup, the Engine must identify the last committed revision and any
incomplete transaction before issuing new mutations. It must not repeatedly
reapply a known-bad candidate. Recovery chooses a safe action from durable
evidence, restores the last-known-good managed state where possible, verifies
the result, and exposes unresolved ambiguity to the administrator.

### Backup and restore

Backup is structured DuckOS data, not a blind archive of `/etc`. Restore loads
a candidate, migrates and validates it, requests mapping of logical roles to
new hardware where needed, and uses Safe Apply. Protected secrets may be
included only in an explicitly encrypted backup.

## Trust and ownership boundaries

| Boundary | Trust assumption | Ownership |
|---|---|---|
| Browser to API | Browser input and client-side checks are untrusted | API authenticates, authorizes, limits, and validates |
| API/Engine to Agent | Unprivileged caller may be compromised | Agent authenticates caller and independently validates semantic requests |
| Canonical to rendered state | Rendering or host application may fail or drift | Engine owns intent; adapters/Agent apply; verifier compares reality |
| DuckOS to Debian host | Existing managers or manual changes may conflict | Installer establishes an explicit supported ownership model before networking changes |
| Local system to external services | WAN, provider, clock, DNS, and cloud can fail or lie | Local core remains manageable; probes report evidence without assuming Internet is required |

NetworkManager/systemd-networkd ownership and supported clean-Debian baseline
must be decided before networking implementation (F-005 and F-006).

## Failure and resource model

Every critical component design must answer:

- What durable state exists before the operation?
- What happens if the process or machine stops at each transition?
- How is the last-known-good state selected after reboot?
- Can a management path be retained or recovered?
- How much disk, memory, CPU, and write amplification can this consume on XU4?
- What is discarded first under storage pressure?
- What data or credentials could appear in errors, logs, support bundles, or
  process arguments?

Critical services use bounded restart policies and readiness checks. Optional
observability and jobs yield to forwarding, canonical state, recovery, and the
previous working release. A consciously offline router is not unhealthy merely
because a public Internet probe fails.

## Repository architecture

The monorepo is expected to contain web/API entry points, core Engine/Agent,
capability and configuration packages, feature modules, CLI, packaging,
platform integration, migrations, tests, documentation, and CI. Directory
boundaries should follow ownership and dependency direction rather than mirror
every UI page or create one service per module.

No directory layout in the requirements is treated as immutable. A material
change must identify the problem it solves and update this document or an ADR
before the implementation silently diverges.

## Architecture evidence tiers

Claims must state their evidence:

1. static/schema/unit verification;
2. host integration test;
3. two-NIC Debian VM traffic path;
4. reboot and injected-failure test;
5. native architecture/hardware test, including XU4 where relevant.

Cross-compilation is build evidence only. It is not ARMv7 runtime, packet-path,
reboot, or failure evidence.

## Decisions still required before networking code

The blocking register is maintained in [`open-decisions.md`](open-decisions.md).
In particular, networking implementation waits for:

- API/Engine deployment topology (F-001);
- exact Agent IPC and compatibility rules (F-002);
- canonical persistence and serialization (F-003);
- secrets trust root and unattended-boot model (F-004);
- Debian/kernel/toolchain baseline (F-005);
- managed host-network ownership (F-006);
- host-level change and rollback contract (F-007).

The following are intentionally not implied by this document: a chosen
DNS/DHCP daemon, a public API encoding, a database layout, a Linux network
manager, a process supervisor beyond Debian/systemd integration, a container
runtime, or a cloud control plane.
