# DuckOS hardware support and evidence

Current beta.2 artifact and qualification status is in [compatibility.md](compatibility.md).
AMD64, ARM64 and ARMv7 are official build targets; only AMD64 received the
complete final live matrix. The physical XU4 was excluded. The broader 1.0
hardware policy below remains a target, not a beta.2 qualification claim.

Status: foundation specification; no hardware support is qualified yet
Last updated: 2026-09-07

DuckOS is one product with one repository, configuration model, component
architecture, update path, and operator interface across `armv7`, `arm64`, and
`amd64`. Hardware changes capacity and available capabilities; it does not
create a separate DuckOS edition.

This document defines how a machine becomes a supported DuckOS target and how
support claims are backed by evidence. It does not claim that an installer,
Capability Engine, CI matrix, lab, or supported platform already exists. The
words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative.

## Product boundary

DuckOS 1.x is a software platform installed on a supported Debian system. It
does not initially provide or own a kernel, bootloader, firmware distribution,
or complete operating-system image. Future ISO, appliance, SD, or eMMC images
may package the same core, but the core MUST remain installable and testable
without depending on such an image.

The Git repository contains the reproducible product source, build inputs,
packaging definitions, documentation, and evidence definitions. No target is
made supported by undocumented manual changes on one router.

An architecture name alone is not a support promise. Support is granted to a
versioned platform profile that identifies at least:

- CPU architecture and ABI/userland assumptions;
- Debian release, kernel range, init/service manager, libc, and required host
  packages;
- filesystem and storage constraints;
- required kernel facilities and permissions;
- known-good device classes, drivers, and firmware where relevant;
- installation/ownership assumptions and incompatible host managers;
- resource, readiness, and recovery budgets;
- scenarios and evidence required to keep the profile qualified.

Exact initial profiles remain open and MUST be frozen before packaging or
network ownership code depends on them.

## Architecture targets

The package and evidence model uses exact normalized identifiers. At minimum:

| Identifier | Intended role | Bootstrap status |
|---|---|---|
| `armv7` | Minimum/resource reference architecture, with ODROID XU4 as the named physical reference target | E0 only; ABI and Debian/kernel baseline are not selected |
| `arm64` | SBC and virtual-machine target class; expected primary Linux integration environment | E0 only; no board, VM, ABI, or OS profile is qualified |
| `amd64` | x86-64 VM, mini-PC, and server target class | E0 only; no microarchitecture, OS, VM, or device profile is qualified |

The build system MUST reject an unknown target rather than silently producing
an artifact for the build host. Update packages MUST declare one exact target
architecture and be rejected before live state changes if it does not match the
installed platform. See [`update-system.md`](update-system.md).

Other architectures can be proposed later, but adding one requires an explicit
platform profile, build path, dependencies, runtime evidence, and maintenance
owner. It must not fork the product model.

## What each kind of evidence proves

Every result MUST identify its execution mode. The following terms are not
interchangeable:

| Evidence | What actually ran | What it can establish | What it cannot establish |
|---|---|---|---|
| Cross-build | A build host produced an artifact for a different target architecture | Source/toolchain portability and artifact production | That the artifact starts, links against the target userland, survives reboot, routes traffic, or works on hardware |
| Emulated runtime | Target userspace and/or CPU instructions ran under an emulator | Many ABI, parser, migration, service, and deterministic integration behaviors when the environment is recorded | Physical drivers, offload, media durability, thermal/resource timing, or real power-loss behavior |
| Native virtual runtime | Target instructions ran natively in a VM on a matching CPU architecture | Native ABI/userland behavior plus VM networking, systemd, reboot, installer, and virtual-disk behavior for the named profile | Physical SBC/NIC/radio/modem/flash behavior or another VM/hypervisor profile |
| Native physical runtime | The artifact ran on a named physical machine with recorded peripherals and media | Behavior of the tested architecture/device/driver/media profile | Untested boards, peripherals, kernels, media, or feature combinations |
| Physical fault/endurance run | A designated physical TEST device was power-cycled, storage-pressured, or run under measured load/endurance | The specific recovery, durability, and resource claim exercised by that run | General safety outside the recorded scenario and target profile |

A cross-build is therefore reported as **built**, never as `ARMv7 TESTED`,
`ARM64 TESTED`, or `AMD64 TESTED`. Emulation MUST be labelled emulated even when
all tests pass. A native VM result MUST not be presented as a physical-device
result. Evidence levels, record fields, and release gates are defined in
[`testing.md`](testing.md).

## ODROID XU4 reference contract

ODROID XU4 is the intentional ARMv7 minimum/reference target for the DuckOS
core. It represents an approximately 2 GB RAM class system with constrained
CPU and commonly constrained SD/eMMC storage. It is not a disposable mascot:
regressions on the reference profile are product regressions unless a reviewed
1.0 scope decision explicitly excludes the affected optional capability.

An ARMv7 cross-build, a QEMU run, or a different ARMv7 board is useful evidence
but does **not** establish XU4 support. A claim such as `ODROID XU4 supported`
requires retained physical evidence from a named XU4 test device, including:

- board revision and stable device identity;
- target ABI/userland, Debian release, kernel, boot firmware, and driver set;
- SD/eMMC type and capacity plus relevant filesystem/mount layout;
- built-in and attached NICs, radios, modems, USB devices, and power topology
  used by the scenario;
- exact DuckOS commit and package digest;
- install/start/readiness and resource measurements;
- the real client/network path for networking claims;
- restart and reboot persistence;
- update Active/Previous and configuration recovery evidence;
- bounded-storage behavior and measured write/growth characteristics;
- physical power-loss or interruption evidence for claims that require it.

Destructive and abrupt-power tests run only on an explicitly designated TEST
XU4 with a documented reimage/recovery procedure and no production role. A
stable/reference unit may supply long-running compatibility evidence, while a
separate destructive unit supplies recovery evidence; records must identify
which one was used.

Passing on one XU4 profile qualifies only that recorded profile and compatible
claims. It does not automatically qualify all ARMv7 boards, storage media, USB
NICs, Wi-Fi adapters, modems, or Debian/kernel combinations.

## Capability Engine contract

DuckOS MUST discover and model capabilities instead of branching on board
names or assuming interface names such as `eth0` and `eth1`. The Capability
Engine is expected to inventory observable facts including:

- normalized architecture, CPU topology/features, and available memory;
- storage capacity, media/filesystem characteristics, and reliable health
  signals where available;
- NICs and stable hardware identity, link modes, queues, and offload features;
- Wi-Fi radios, modems, USB/PCI devices, drivers, and firmware state;
- relevant kernel modules, nftables/network facilities, namespaces, and other
  kernel capabilities;
- service/host prerequisites and measured performance constraints.

It then derives versioned product capabilities and limits from those facts.
Observed facts, inferred recommendations, administrator mappings, and measured
results MUST remain distinguishable. Unknown is not equivalent to absent, and
presence of a device is not proof that its driver/firmware behavior is
qualified.

The Engine and UI consume capabilities to:

- map hardware to stable logical roles such as `WAN1` and `LAN1`;
- hide, disable, or explain unavailable features without corrupting canonical
  configuration;
- set bounded worker, table, metrics, log, and readiness defaults;
- warn or refuse configurations that exceed a qualified resource envelope;
- propose a known safe Debian package/firmware path when a driver is missing,
  subject to administrator approval and the unresolved host-mutation policy.

It MUST NOT silently download and execute arbitrary third-party driver code,
invent a working capability, or use powerful hardware as permission for
unbounded resource growth.

Capability discovery influences availability and safe defaults; it does not
weaken the common configuration schema, privilege boundary, storage reserve,
Safe Apply, update rollback, or audit rules.

## Resource profiles and optional features

Core routing, management, recovery, and bounded observability must remain
viable on the qualified minimum profile. Heavier capabilities such as IDS/IPS,
deep analytics, high-cardinality metrics, large rule/table sets, or expensive
speed tests MAY be enabled on stronger hardware when both prerequisites and
measured budgets are satisfied.

Resource profiles MUST be derived from recorded capability and benchmark data,
not architecture stereotypes. ARM64 can mean a constrained flash SBC or a
large VM; amd64 can mean a tiny thin-provisioned guest or a many-core server.
Every profile retains hard storage and recovery bounds.

At minimum, qualification should measure:

- idle and peak RSS/CPU/process counts for the core;
- startup, readiness, apply, rollback, and reboot recovery time;
- forwarding throughput/latency and connection/rule/table scale relevant to
  the profile;
- DNS, database, log, metrics, and cleanup growth/write rates;
- update staging/migration peak storage and duration;
- behavior under sustained load, thermal pressure, low memory, and low space.

Budgets remain open until measured. Once adopted, they become regression gates
for the applicable profile.

## Interface and peripheral portability

Canonical configuration references DuckOS logical interfaces and roles.
Hardware mapping is separate and uses stable MAC, PCI, USB, platform-path, or
other identifiers carefully; no single identifier is assumed infallible.

Restore or migration to different hardware MUST detect missing, duplicated, or
changed mappings and require explicit remapping before dangerous state is
applied. A configuration that was safe on one device is not assumed safe on a
new topology. Mapping changes use normal validation and Safe Apply.

Wi-Fi, cellular, USB, switch, and acceleration support is capability- and
driver-specific. The UI should explain whether a feature is:

- supported and qualified for the current profile;
- detected but not yet qualified;
- unavailable because a prerequisite is absent;
- disabled by resource/safety policy;
- unsupported on the selected Debian/kernel baseline.

## Installer and host ownership constraints

The installer will eventually verify the platform profile, architecture,
dependencies, storage, filesystem, kernel facilities, conflicting network
managers, service users, and recovery prerequisites before taking ownership.
It must refuse or clearly report an environment whose safety cannot be
established.

Initial 1.0 support MAY require a clean supported Debian installation if that
is necessary for a reliable ownership boundary. Exact Debian/kernel versions,
the managed-network ownership model, and handling of pre-existing systems are
open decisions. DuckOS does not promise rollback of arbitrary administrator
root changes.

The update Active/Previous contract covers DuckOS releases and compatible
canonical state. Rollback of host-level Debian packages, global systemd state,
firmware, kernel, or boot changes remains unresolved; see
[`adr/0004-update-active-previous.md`](adr/0004-update-active-previous.md).

## Support lifecycle

A platform profile progresses by evidence, not intent:

1. **Proposed**: requirements and risks are documented (E0).
2. **Build target**: reproducible target artifacts and static/unit checks pass;
   cross-build is explicitly labelled (at least applicable E1 evidence).
3. **Integration target**: recorded emulated or native Linux integration passes
   for defined claims (applicable E2/E3 evidence).
4. **Hardware-qualified**: required claims pass on named physical profiles with
   reboot, resource, driver, and failure evidence (applicable E4/E5 evidence).
5. **Release-supported**: the frozen 1.0 matrix and release gate include the
   profile and its evidence (E6).

Support can be narrowed or withdrawn only through a documented compatibility
decision, migration/recovery guidance, and product-spec update. A stale lab,
unavailable device, unsupported security baseline, or expired evidence must be
visible; it must not silently remain a green support claim.

## Required evidence before a platform claim

For every release-supported platform profile, retain evidence for:

- reproducible architecture build and authenticated package identity;
- clean install, upgrade from every supported source, rollback, and recovery;
- Agent/Engine/API/UI startup, readiness, and privilege boundaries;
- canonical configuration persistence and migrations across reboot;
- real DHCP/DNS/firewall/routing/WAN behavior from an isolated client where
  those features are in scope;
- IPv4, IPv6, and dual-stack outcomes required by the product matrix;
- storage pressure, bounded logs/metrics/backups, and database recovery;
- process interruption, reboot, and applicable physical power-loss states;
- driver/peripheral behavior and stable logical-interface remapping;
- security/redaction and management recovery behavior;
- performance/endurance budgets for the profile.

No repository-bootstrap document or empty CI matrix satisfies this evidence.

## Open decisions

- Exact Debian, kernel, systemd, nftables, libc, filesystem, and ABI baselines
  for `armv7`, `arm64`, and `amd64`.
- Whether initial 1.0 installation requires clean Debian and exactly which host
  state the installer/network stack owns.
- Exact ODROID XU4 board, boot, storage, NIC, and peripheral qualification
  profiles.
- ARM64 and amd64 physical reference devices and supported VM/hypervisor
  profiles.
- Capability schema, stable hardware identifiers, cache/invalidation behavior,
  and operator overrides.
- Measured minimum resources, readiness/recovery budgets, and optional-feature
  admission thresholds.
- Driver and firmware support/approval policy and hardware compatibility data
  lifecycle.
- Lab automation, device reservation, reimage/recovery, evidence signing, and
  evidence-expiration rules.
- Support/deprecation policy for Debian releases, kernels, boards, peripherals,
  and architectures.
- Host-level Debian mutation and rollback strategy.
