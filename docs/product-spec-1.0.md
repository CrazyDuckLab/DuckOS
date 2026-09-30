# DuckOS 1.0 product specification and scope matrix

Status: **FROZEN V1 REQUIREMENTS — 2026-09-10**
Product implementation status: **OBSERVATION RUNTIME + TRANSACTION/NETWORK CORE IN DEVELOPMENT**
Target external release: **DuckOS 1.0.0**
Requirements source: [`requirements/product.md`](requirements/product.md)

This specification's V1 scope is frozen at the 2026-09-10 handoff baseline.
The authoritative deduplicated requirements and delivery classification are
[the frozen V1 scope](handoff/V1_SCOPE.md). They include stable Easy/Full UX,
first-class CLI, portable/hardware-remapped restore, explicit client isolation,
client artifact lifecycle, native Xray VLESS/Trojan/supported REALITY, product-wide
intent-driven management and strictly bounded logging/diagnostics.

The existing M/S/P IDs and detailed acceptance intent below are retained for
traceability. Historical MUST/SHOULD/POST labels do not override the consolidated
A (V1 Alpha blocker), B (architecture-now/implementation-later), C (post-Alpha)
classification. B/C V1 requirements are deferred within development, not deleted.
Explicit existing post-V1 reservations remain reserved. Internal Alpha does not
authorize a public intermediate release. Current implementation/evidence is
recorded in [PROJECT_STATE](handoff/PROJECT_STATE.md), not inferred from this matrix.

## Product outcome

DuckOS 1.0 is intended to be one hardware-independent, local-first network
operating platform on supported Debian systems for ARMv7, ARM64, and amd64. It
is a software platform, not a custom Linux distribution, kernel, or bootloader.
The normal administrator experience must not require direct Linux configuration
or a shell.

One product architecture spans the supported targets:

```text
Duck Web -> Duck API -> Duck Engine -> semantic IPC -> privileged Duck Agent
                                      -> managed Debian network/data plane
```

The Engine owns product semantics and canonical desired state. Linux files and
runtime state are rendered or observed representations. Dangerous changes and
updates are transactional and recoverable. Local operation, bounded resource
use, IPv6-aware models, honest diagnostics, and real architecture/network-path
evidence are release properties rather than optional polish.

## Current truth

The observation/authentication runtime works; network preview/simulation and
security/recovery components are locally tested. The static managed-routing
Safe Apply adapter and pre-networkd recovery gate exist but installed network
mutation, full WAN/LAN recovery and native qualification remain incomplete.
Vault, SQLite client metadata and reusable QR exports are implemented components,
not yet a live client provisioning product. See the exact code checkpoint and
test status in [PROJECT_STATE](handoff/PROJECT_STATE.md).

## Historical scope vocabulary (traceability only)

| Bucket | Meaning while this draft is under review |
|---|---|
| `MUST FOR 1.0` | Proposed release-blocking capability or invariant. It must meet its complete acceptance gate before 1.0.0. |
| `SHOULD IF READY` | Desired in 1.0, but may move after 1.0 if safe completion would compromise the MUST scope. Any move requires owner approval and recorded rationale. |
| `POST-1.0` | Deliberately reserved by the architecture but proposed outside the first release. No incompatible shortcut may block it. |
| `UNDECIDED` | Material scope or design decision still requiring an ADR or explicit owner decision. Dependent implementation remains blocked. |

`NOT QUALIFIED` means the complete row has not passed its release acceptance;
partial implementation and local evidence may exist in PROJECT_STATE. Future lifecycle states are `SPECIFIED`, `IMPLEMENTED`, `UNIT TESTED`,
`INTEGRATION TESTED`, `ARMv7 TESTED`, `ARM64 TESTED`, `AMD64 TESTED`, `REBOOT
TESTED`, `FAILURE TESTED`, `SECURITY REVIEWED`, `UI COMPLETE`, `DOCUMENTED`, and
`READY`. Critical capabilities may carry several evidence flags at once; only
`READY` after all applicable gates can satisfy a release row.

## Original MUST FOR 1.0 matrix

Rows retain their acceptance intent; current delivery classes are in the frozen V1 scope. None is a completion claim.

| ID | Capability / invariant | Acceptance intent | Status |
|---|---|---|---|
| M-001 | One product across ARMv7, ARM64, and amd64 | One repository, model, API shape, update path, and UX; ODROID XU4 remains the minimum resource reference; capability facts, not board forks, control intensity. | NOT QUALIFIED |
| M-002 | Supported-Debian installer boundary | Reproducible installation from repository/release inputs on an explicitly supported, preferably clean Debian baseline; detect/refuse unsafe pre-existing ownership; no custom kernel/distribution requirement. | NOT QUALIFIED |
| M-003 | API / Engine / Agent architecture | UI and API cannot mutate Linux directly; all writers use Engine semantics; privileged Agent exposes authenticated, versioned, narrow operations and independently validates input. | NOT QUALIFIED |
| M-004 | Versioned canonical configuration | Schema-versioned desired state, stable object identities, migrations, candidate/committed separation, deterministic render inputs, and explicit desired/observed/rendered/history classes. | NOT QUALIFIED |
| M-005 | Coherent configuration transactions and Safe Apply | Validate dependencies/conflicts/capabilities, durably journal, apply an inspectable plan, verify reality, confirm dangerous changes, and commit or roll back the whole transaction. | NOT QUALIFIED |
| M-006 | Reboot, crash, power-loss, and last-known-good recovery | Detect incomplete work, avoid broken-candidate loops, preserve a management path, and verify recovery after interrupted apply/update/rollback. | NOT QUALIFIED |
| M-007 | Capability Engine and Resource Guard | Observe architecture/resources/devices/kernel capabilities with provenance; bound work and warn/refuse unsafe configurations without model-name product forks. | NOT QUALIFIED |
| M-008 | First Flight and secure initial administration | Language, strong admin enrollment, hardware and WAN/LAN identification, baseline DNS/security, review, Safe Apply, connectivity verification, and a documented recovery address/path. | NOT QUALIFIED |
| M-009 | Stable interface roles and Network objects | Logical roles survive rename/reboot/hardware replacement; networks model subnet/VLAN/gateway/services/isolation/management separately from current Linux interface names. | NOT QUALIFIED |
| M-010 | Core WAN | Frozen baseline must include Ethernet DHCP, static addressing, VLAN composition, PPPoE, and selected dual-stack acquisition/PD behavior with secrets, validation, persistence, and diagnostics. | NOT QUALIFIED |
| M-011 | IPv6-first architecture and qualified dual-stack behavior | Address/prefix, firewall, DNS, route, policy, and health models are family-aware; the frozen baseline names and tests supported SLAAC/DHCPv6/PD/static behaviors. | NOT QUALIFIED |
| M-012 | Basic multi-WAN resilience | At least primary/backup failover with multi-signal health, hysteresis, recovery thresholds, explicit session/policy behavior, and no one-ping flapping. | NOT QUALIFIED |
| M-013 | LAN, VLAN, DHCP, DNS, and IPv6 network services | Real client receives and renews service, reaches intended DNS/gateway, respects network management/isolation policy, survives reboot, and reports failures honestly. | NOT QUALIFIED |
| M-014 | Stateful firewall, NAT, and explicit inbound exposure | nftables candidate validation and atomic activation, secure defaults, IPv4/IPv6 policy, explicit port forwards, counters/logging, hairpin behavior where selected, and Safe Apply. | NOT QUALIFIED |
| M-015 | Unified Traffic Policy Engine | Stable match/action model for device/network/destination/service/protocol/schedule to DIRECT, approved WAN/tunnel/route group, or BLOCK; explicit fail-open/fail-closed fallback. | NOT QUALIFIED |
| M-016 | Domain routing and inspectable Service Packs | Versioned/editable service definitions using available non-MITM signals, IPv4/IPv6 sets, QUIC/CDN limitations, diagnostics for incomplete classification, and no promise of perfect app identity. | NOT QUALIFIED |
| M-017 | Route profiles, groups, health, and failover | Transport-independent policy references, multi-signal probes, stable selection/stickiness where required, explicit fallback, and safe recovery when a node disappears. | NOT QUALIFIED |
| M-018 | Authentication, sessions, and unified RBAC | Strong bootstrap auth, server-side permissions, session lifecycle/login history, roles covering all product surfaces, re-auth for high-risk actions, and recovery without undocumented bypass. | NOT QUALIFIED |
| M-019 | Default security baseline | WAN management closed, no public root SSH, default-deny posture where appropriate, management from selected networks/secure tunnels, measurable self-audit, and no fake protection claims. | NOT QUALIFIED |
| M-020 | Secrets protection | Opaque secret references, protected storage, no plaintext normal reads/logs/diffs/process arguments/support bundles, explicit backup handling, rotation/recovery, and a documented trust model. | NOT QUALIFIED |
| M-021 | Audit and integrity/drift visibility | Secret-safe audit of identity and managed mutations; detect managed-state drift and offer inspect/restore/safe import instead of silent overwrite. | NOT QUALIFIED |
| M-022 | Configuration history, backup, and restore | Bounded history and structured backups; encrypted secret option; migration/validation; hardware role remapping; restore through Safe Apply; at least one protected recovery copy. | NOT QUALIFIED |
| M-023 | Manual authenticated software update | One Upgrade Engine receives a local `.duck`, validates bounded input/signature/checksums/architecture/compatibility/space, stages beside Active, migrates a copy, verifies, and commits or recovers. | NOT QUALIFIED |
| M-024 | Active + Previous update invariant | Never overwrite Active in place or delete the only rollback release to make room; stable state retains Active plus one Previous; cleanup happens only after commit and is bounded/retryable. | NOT QUALIFIED |
| M-025 | Storage Policy and disk emergency behavior | Every growing data set has age/size/free-space bounds; priorities protect running system, rollback, config, and recovery; optional jobs/debug/cache degrade first; inode/full-disk faults are tested. | NOT QUALIFIED |
| M-026 | Duck CLI and non-Web recovery surface | CLI uses Engine semantics for managed mutations and provides status/doctor/apply/rollback paths sufficient to recover when Web assets fail; no independent config hacking. | NOT QUALIFIED |
| M-027 | Conflict Detector, policy simulation, and Duck Doctor | Explain pre-apply conflicts and selected path, then diagnose actual client-to-target layers using observed evidence and clear uncertainty. | NOT QUALIFIED |
| M-028 | Honest health, errors, and support evidence | State exact failure stage/current safety/rollback result/action; distinguish observed/inferred/stale/unsupported; produce bounded, sanitized support bundles; no mock production data or fabricated score. | NOT QUALIFIED |
| M-029 | Core operational health | Correct time/NTP health, hardware/resource observations, bounded service restart policies, readiness checks, and local operation without an Internet/cloud dependency. | NOT QUALIFIED |
| M-030 | Premium, usable management UI | Coherent responsive Web UI, progressive Basic/Advanced disclosure, contextual risk/explanations, visible Safe Mode state, touch/keyboard accessibility, graceful client degradation, and localization-ready strings. | NOT QUALIFIED |
| M-031 | Privacy-preserving local-first operation | No mandatory cloud telemetry or HTTPS interception; collect minimal metadata by default; expose retention and uncertainty; keep guest and administrator information separated. | NOT QUALIFIED |
| M-032 | Reproducible build, package, CI, and evidence baseline | Repository-owned format/lint/test/build/package entry points; builds for all three architectures; signed/provenance-friendly artifacts; results label cross/native/emulated evidence accurately. | NOT QUALIFIED |
| M-033 | 1.0 qualification and documentation | Frozen scope and platform matrix, real isolated client path, reboot/fault/security/resource tests, native ARMv7/ARM64/amd64 evidence, operator/recovery docs, and no unresolved critical blocker. | NOT QUALIFIED |

## Original SHOULD matrix (retained requirements)

These requirements remain in the consolidated scope. Their current A/B/C timing
is explicit there; historical labels are not permission to omit them or weaken safety.

| ID | Capability | Acceptance intent if retained in 1.0 | Status |
|---|---|---|---|
| S-001 | Advanced WAN methods | Safely add selected 802.1X, L2TP, legacy-warned PPTP, Wi-Fi-as-WAN, USB tethering, provider options, and transition mechanisms only with complete secret/reboot/rollback tests. | NOT QUALIFIED |
| S-002 | Advanced multi-WAN | Weighted/load-balanced and dedicated-network/device routing with measured health, stickiness, clear failover, and diagnostics. | NOT QUALIFIED |
| S-003 | Subscriptions and provider imports | Strict bounded parsing, normalization/deduplication, preview/diff, atomic last-known-good activation, selectable refresh, health history, and no software-update privileges. | NOT QUALIFIED |
| S-004 | Additional route transports | Add owner-approved WireGuard, Xray, and/or NetBird adapters behind the common route model without transport-specific policy semantics. | NOT QUALIFIED |
| S-005 | Duck Speed Lab | Honest per-WAN/tunnel/node/network/device tests, quality scoring based on named measurements, scheduling warnings, resource limits, and bounded history. | NOT QUALIFIED |
| S-006 | QoS/SQM | Capability-gated CAKE/fq_codel and per-network/device limits/priorities with measurement and no fake application classification. | NOT QUALIFIED |
| S-007 | Device trust and lightweight threat controls | Trusted/Guest/IoT/Quarantined states, egress controls, microsegmentation where topology permits, bounded lists/heuristics, and recoverable temporary blocks. | NOT QUALIFIED |
| S-008 | Captive portal | Per-network/SSID local portal with safe captive detection, basic terms/voucher workflows, isolation, branding/localization, and no HTTPS interception. | NOT QUALIFIED |
| S-009 | Sites, reverse proxy, certificates, and DDNS | Separate HTTP(S) publishing from port forwards; validate bounded uploads/config, protect keys, explain DNS/TLS/backend layers, and avoid becoming a general hosting panel. | NOT QUALIFIED |
| S-010 | Controlled UPnP/NAT-PMP/PCP | Off by default or explicitly selected, per-network, visible, time-bounded, audited, and globally reversible. | NOT QUALIFIED |
| S-011 | Monitoring, timeline, and notifications | Bounded metadata/metrics, deduplicated and hysteretic alerts, per-client/Internet timelines, and resource-aware exports/integrations. | NOT QUALIFIED |
| S-012 | Topology and service discovery | Clearly label observed versus inferred topology and offer reviewed configuration wizards without silently applying dangerous changes. | NOT QUALIFIED |
| S-013 | Emergency/Panic modes | Audited and reversible modes preserve LAN/DHCP/DNS/NAT/secure management while disabling risky policy or exposure. | NOT QUALIFIED |
| S-014 | UPS and hardware diagnostics | Capability-gated NUT integration, safe shutdown, thermal/storage/throttling/NIC health, and actionable alerts. | NOT QUALIFIED |
| S-015 | Command palette and route composer | Search/actions obey RBAC and confirmations; route visualization uses the same explainable policy result as simulation and diagnostics. | NOT QUALIFIED |

## Preserved post-V1 architectural reservations

No 1.0 design may make these needlessly impossible, but no implementation or
release claim is requested by this proposed bucket.

| ID | Reserved capability | Architectural reservation | Status |
|---|---|---|---|
| P-001 | Automatic Internet DuckOS updates and channels | A future signed repository supplies the same `.duck` package to the same Upgrade Engine; it does not create a second updater. | NOT QUALIFIED |
| P-002 | Full IDS/IPS on capable hardware | Modular/capability-gated integration such as Suricata must not burden the XU4 core or receive unrestricted privilege. | NOT QUALIFIED |
| P-003 | Plugin ecosystem | Versioned manifest declares permissions, dependencies, resources, capabilities, and architectures; installation never grants automatic root. | NOT QUALIFIED |
| P-004 | Multi-site controller or optional cloud relay | Local router remains fully usable without cloud; direct secure connectivity is preferred and tenancy/identity boundaries stay explicit. | NOT QUALIFIED |
| P-005 | High availability | Object identities, transactions, and configuration model avoid needless single-node assumptions; actual synchronization/failover semantics are deferred. | NOT QUALIFIED |
| P-006 | Duck Autopilot | Explainable recommendations only; critical changes still require permission and use the ordinary transaction path. | NOT QUALIFIED |
| P-007 | Physical appliance recovery port and images | Software 1.x does not depend on a custom image, kernel, bootloader, or dedicated port, but management/recovery models can add them later. | NOT QUALIFIED |
| P-008 | Full Owner Web Terminal | If later approved, it is a separate optional, re-authenticated, audited security boundary—not an arbitrary-command extension to the Agent API. | NOT QUALIFIED |
| P-009 | Deep commercial/PMS and general application hosting | Keep integration points and portal roles extensible without turning the core router into a cPanel/container platform. | NOT QUALIFIED |

## Decision register (historical IDs; see current decisions)

Resolved contracts must not be reopened from historical wording. Current ADRs
and [open decisions](open-decisions.md) distinguish remaining qualification and
release choices from completed foundation decisions.

| ID | Decision | Required output | Status |
|---|---|---|---|
| U-001 | V1 scope freeze | Consolidated 2026-09-10 requirements and A/B/C timing in handoff/V1_SCOPE.md. | RESOLVED FOR HANDOFF |
| U-002 | Supported Debian/kernel/systemd/nftables/ABI baselines | Versioned matrix for ARMv7, ARM64, and amd64 plus installer ownership assumptions. | NOT QUALIFIED |
| U-003 | API/Engine topology | ADR-0005 resolves the shared unprivileged process; future compatibility evolution remains controlled. | BASELINE RESOLVED |
| U-004 | Agent IPC | ADR-0005/0010 resolve bounded peer-authenticated HTTP/JSON Unix IPC and network coordination. Installed mutation activation remains gated. | BASELINE RESOLVED |
| U-005 | Persistence and transactions | ADR-0006 JSON recovery journal and ADR-0017 SQLite metadata catalog; wider migrations remain work. | BASELINE RESOLVED |
| U-006 | Secrets trust root | Key source, unattended boot, rotation, hardware replacement, recovery, backup/export, and attacker model. | NOT QUALIFIED |
| U-007 | Managed host-network ownership | Supported clean-Debian contract, competing manager behavior, owned paths/units, drift/import, and refusal/recovery rules. | NOT QUALIFIED |
| U-008 | Host-level Debian mutation and rollback | What installer/update may change outside release/config state, how it reverses across interruption, and what evidence proves recovery. | NOT QUALIFIED |
| U-009 | V1 transports | Direct/WAN, WireGuard, native Xray and supported NetBird/subscriptions remain V1 scope with explicit timing in V12. | SCOPE RESOLVED |
| U-010 | Wi-Fi scope and hardware matrix | Decide whether AP/station/mesh and driver/firmware management are MUST, SHOULD, or deferred, with tested chipsets and regulatory model. | NOT QUALIFIED |
| U-011 | Cellular/modem scope and hardware matrix | Decide mandatory LTE/5G protocols/devices, watchdog escalation, SIM/secret model, and native architecture evidence. | NOT QUALIFIED |
| U-012 | Public/open-source license | Dependency/license/business review and explicit license choice before public release or contribution assumptions. | NOT QUALIFIED |
| U-013 | Exact release and evidence authority | CI/lab schedule, artifact signing/trust, performance/recovery budgets, exception policy, evidence store, and release approver. | NOT QUALIFIED |

## Global 1.0 release gate

### Management, edge nodes and client distribution

The following requirements are mandatory additions to the matrix. They refine
M-008, M-009, M-014, M-018–M-021 and S-003–S-004; management is no longer inferred
from WAN/LAN role. Older runtime restrictions remain limitations until migrated
and verified, not the target security policy.

| ID | Requirement | Acceptance gate |
|---|---|---|
| M-034 | Independent Management Access Policy | Trusted sources/interfaces and admin tunnels, IPv4/IPv6 and separate SSH/Web/API exposure; no automatic public management or install-time SSH/network replacement. |
| M-035 | Edge/VPN Node and trunk topology | One physical NIC without physical LAN; trusted management and client tunnels coexist; VLAN trunk and dedicated admin tunnel are supported and tested. |
| M-036 | Full anti-lockout recovery | Snapshot and restore links, addresses, VLANs, bridges, routes, ip rules, nftables, DHCP and canonical state; lost confirmation and reboot cannot commit a candidate; console rescue is documented. |
| M-037 | Client builder and reusable secret exports | Generic nodes/clients/profiles/subscriptions/renderers; supported HAPP output and complete WireGuard client configuration; CRUD, enable/expiry, rotation, preview/rebuild and secret-safe audit. |
| M-038 | QR and download consistency | Subscription QR equals URL; WireGuard QR/copy/.conf bytes match; rotation invalidates exports; standard mobile scan, RBAC, no static secret assets, and no log/audit secret leakage are tested. |
| M-039 | Service port exposure | Contextual Allow on WAN plus central inventory, explicit Safe Apply, ownership/shared dependencies, conflict detection and no implicit exposure on enablement. |
| M-040 | Reachability evidence | Configuration, listener, local firewall and external reachability are separate; provider firewall uncertainty is visible and local permission never implies Internet reachability. |
| M-041 | Intent-driven Network Access Policy | Shared presets, effective summaries/matrix, network/client inheritance, independent services/management, transport-aware enforcement and real isolation/override/rollback tests as specified in [Network Access Policy](network-access-policy.md). |
| M-042 | Beginner-obvious, expert-powerful workflows | Guided common tasks, safe defaults, consequence previews, Engine-computed Effective Access with explanations, exact expert inspection and safe undo meet the [UX completion gate](ux-principles.md). |
| M-043 | Network explainability and what-if | Shared evaluation/generation semantics trace identity, DNS, policy, firewall, routing, transport and NAT; offline candidate comparisons expose uncertainty instead of false reachability claims. |
| M-044 | Live topology | Timestamped canonical/observed graph and useful client/tunnel details; missing or stale telemetry is explicit, with Easy/Full views and no fabricated topology. |
| M-045 | Migration adapters | Bounded supported-format parsing, provenance, recognized/convertible/attention/unsupported preview, no source-code execution and activation only through validators and Safe Apply. |
| M-046 | Semantic network timeline | Revision-linked network events, before/after effective impact, health correlation without invented causality and validated restoration of known-good state. |

M-043–M-046 follow the [network-understanding contract](network-understanding.md)
and extend existing local-first, policy, history and diagnostic requirements.

All rows use the common Engine, Agent, transaction, identity, audit, monitoring,
history and diagnostic architecture. They are requirements, not completion claims.

### Evidence required

An item is not `READY` merely because its UI exists, it cross-compiles, its
process is active, or a mocked test passes. For every applicable MUST row, the
release record must show:

1. accepted specification, canonical owner, privilege and data boundaries;
2. implementation plus deterministic unit/schema/migration tests;
3. real Linux component integration and isolated two-NIC client-path tests;
4. native ARMv7, ARM64, and amd64 evidence for the supported matrix;
5. IPv4, IPv6, and dual-stack evidence where relevant;
6. service restart and full reboot persistence;
7. process kill, power loss, invalid input, dependency loss, and disk/inode
   pressure behavior;
8. coherent rollback or an explicit safe recovery state;
9. security review, RBAC/privilege abuse tests, and secret redaction;
10. bounded storage/resource measurements, especially on ODROID XU4;
11. complete basic/advanced UI, localization/accessibility behavior, CLI/API
    parity where applicable, useful errors, and operator/recovery docs;
12. exact commit, artifact digest, environment, topology, and retained evidence
    as required by [`testing.md`](testing.md).

The first external product release is `1.0.0` only after the complete frozen
MUST matrix passes. Earlier versions/builds remain internal development
artifacts and must not be described as a supported router.

## Development workflow

Source changes are made in the local checkout. Linux test targets receive exact
committed revisions from GitHub, not uncommitted synchronized files. Test results
identify the revision and environment as specified in [development.md](development.md).

## Change control

- Add or change a row when product intent changes; never leave a new requirement
  outside the maintained specification.
- Record consequential architecture decisions in `docs/adr/` and link them to
  affected rows.
- Advance statuses only with evidence. Cross-compilation cannot set an
  architecture-tested flag.
- Record explicitly why a requirement moves between buckets and who approved
  the move.
- Keep requirements, acceptance gates and implementation documentation consistent.
