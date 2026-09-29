# Frozen DuckOS V1 scope

Baseline: 2026-09-10 handoff consolidation. Product requirements are frozen, not
claimed implemented. This register deduplicates the requirements catalog and
M-001–M-046 / S-001–S-015 matrix plus the new UX/operations requirements.
The detailed contracts linked below remain normative; no requirement disappears
because its implementation is later. Changes require a recorded scope decision.

## Delivery classes

- **A — V1 Alpha blocker:** required for a usable, secure internal Alpha baseline.
- **B — Architecture-now / implementation-later:** model/contracts must support it
  now; complete implementation is still required before V1 feature-complete.
- **C — Post-Alpha:** may follow the internal Alpha; still V1 work unless explicitly
  identified as an existing post-V1 reservation in the final section.

These classes do not authorize reduced security or recovery. A feature that is
exposed must meet its security gates regardless of its class. Alpha is internal:
the first public release remains qualified 1.0.0, not an intermediate public MVP.

## Consolidated requirements

| ID | Class | Frozen capability and acceptance intent | Existing coverage |
|---|---|---|---|
| V01 | A | One local-first DuckOS on supported Debian, one repository/API/config/update/UX across amd64, arm64 and armv7; XU4 resource reference. No custom kernel/bootloader or mandatory account/controller/cloud. Preserve backend abstraction for later Linux distributions. | M001, M003, M031 |
| V02 | A | Reproducible pinned toolchains, locked dependencies, schema/contract checks, build/package/CI provenance, safe fresh installer and accurate capability/architecture support matrix. Never silently adopt host networking or change SSH. | M002, M007, M032 |
| V03 | A | Versioned canonical objects and stable IDs; desired/observed/rendered/history separation; migrations, dependency validation, ownership/drift handling and resource-bounded state. | M004, M021 |
| V04 | A | Full transactional Safe Apply: validate, preview, journal, stage, apply, verify, explicit connectivity confirmation, commit/automatic rollback. Recover links/addresses/bridges/VLANs/routes/rules/firewall/DHCP and canonical state after failure/reboot; retain management and console rescue. | M005, M006, M036 |
| V05 | A | Independent Management Access Policy for trusted sources, interfaces and admin tunnels; SSH/Web/API separation, default-denied public management, single-NIC Edge/VPN Node and trunk scenarios. Client exposure never grants router management. | M009, M019, M034, M035 |
| V06 | A | Ethernet WAN DHCP/static/VLAN and basic LAN/bridge/VLAN, DHCP pools/reservations/leases, DNS upstream/local records and explicit isolation. Logical role mapping survives hardware changes. | M009, M010, M013 |
| V07 | A/B | IPv4/IPv6 parity and dual-stack-first objects/validation/enforcement are A; complete qualified PPPoE, SLAAC/DHCPv6/prefix delegation and provider behavior are B. No implied IPv6 protection from IPv4-only enforcement. | M010, M011 |
| V08 | A | Stateful nftables firewall, input/forward separation, NAT, port forwarding and selected hairpin behavior, counters and bounded logging; no silent WAN exposure. Intent-driven publishing previews exact uplink/protocol/port/target and creates visible attributed objects. | M014, M039, M040 |
| V09 | A/B | Static and selective policy routing with explicit fail-closed/fallback behavior and basic explainable previews are A. Full unified human-readable device/network/destination/service/protocol/schedule policy and transport-neutral route profiles/groups are B. | M015, M017 |
| V10 | B | Multi-WAN primary/backup and qualified weighted/load balancing, multi-signal health, hysteresis, thresholds, stickiness and explicit session/fallback semantics. | M012, S002 |
| V11 | B | Domain routing and editable/versioned Service Packs using available DNS/IP signals; explain CDN, encrypted DNS, QUIC and unsupported classification limits; no MITM or perfect-app claims. | M016 |
| V12 | A/B | Working WireGuard clients/peers and secure standard config/QR lifecycle are A. Native first-class Xray VLESS/Trojan and supported REALITY, NetBird adapters, route-group integration and HAPP/provider subscription workflows are B. Xray has the same convenience-oriented object lifecycle as WireGuard, not a raw JSON editor. Detailed Xray contract below is required for V1. | S003, S004, M037, M038 |
| V13 | A | Explicit tunnel/client isolation: peer communication, router access, Internet-only, selected networks/subnets/hosts/services, full network, custom policy. One normal intent drives router enforcement and client routes; full tunnel is explicit, never default. | M041 |
| V14 | A/B | Authorized secret exports and exact URL/QR or WireGuard config/QR/copy/.conf equality, no public assets, audit/redaction and key/token rotation are A. Client/profile/node CRUD, tags/notes, multiple nodes, expiry/revoke/time-limited access and supported renderer formats are B. Scope changes invalidate exports and require a visible remote re-apply notice. | M020, M037, M038 |
| V15 | A/B | Strong first Owner enrollment, local authentication, sessions, server-side RBAC, high-risk re-auth, secret protection and authenticated management are A. Complete user administration, qualified MFA and recovery UX are B; no undocumented bypass. | M018–M020 |
| V16 | A | First Flight: language, secure administration, capabilities/ports, WAN/LAN or Edge intent, DNS/security, review, Safe Apply, real connectivity tests and recovery path. Optional Easy success mascot appears only after real successful checks. | M008, M042 |
| V17 | A/B | One Easy/Full presentation model and stable Full information architecture, core guided workflows and contextual help are A. Full dense-table/list workflows, localization/accessibility and broad polish are B, not a toy replacement UI. All advanced management remains in GUI/Duck CLI. | M030, M042 |
| V18 | A/B | First-class CLI over the same Engine/API with validation, status, preview/apply/confirm/rollback/recovery is A. Contextual help/examples, autocomplete and GUI Show CLI equivalents across supported operations are B. | M026 |
| V19 | A/B | Dependency/impact preview and generated-object ownership/source visibility are A. Safe intentional detach to manual control, visible-object templates, clone/duplicate, tags/groups/aliases and global command/search navigation are B. | M004, M021, M039, S015 |
| V20 | A/B | Transaction history with diff, actor, time, result, rollback and protected audit is A. Semantic Network Time Machine, effective-impact comparisons and connectivity/health correlation without invented causality are B. | M021, M022, M046 |
| V21 | A/B | Basic Why?/what-if, explicit uncertainty and management/connectivity diagnostics are A. Full shared identity→DNS→access→firewall→policy routing→route→tunnel/WAN→NAT explanation, candidate comparison and Duck Doctor workflows are B. | M027, M028, M043 |
| V22 | B | Real live topology and client/tunnel/object details, provenance/time/staleness, effective access/path/traffic; no decorative fabricated links. Context actions and Test/logs stay next to the relevant object. | S012, M044 |
| V23 | A/B | MTU Auto where supported, validated manual override and family/tunnel-aware MTU models are A. PMTU diagnostics, contextual capture with bounded PCAP export and live traceroute/path/object Test actions are B. No unbounded or default packet-content collection. | M027, M028 |
| V24 | A/B | Protected bounded audit, subsystem/object logging controls and resource-safe failure behavior are A. Rich filters/search/live tail/export and contextual temporary diagnostic trace automation are B. Exact logging contract below applies from first implementation. | M021, M025, M028, S011 |
| V25 | B/C | Bounded monitoring/history, operational alerts, NTP/time/service/resource health are B (core service/resource bounds are A). Advanced notification integrations, Internet flight recorder and measured Duck Speed Lab are C. Scores must name actual measurements. | M029, S005, S011 |
| V26 | A/B | Portable full backup and human-readable config export, secure secret handling, preview/diff/validation, safe restore/rollback are A. Selective restore, supported version migrations and complete different-hardware drag/drop/remapping workflow are B. | M022 |
| V27 | B | Hardware replacement detects missing NICs and maps logical roles with MAC/link/speed/driver evidence; physical Ethernet blink/identify where supported; preview mapping, test connectivity and commit or rollback. | M007, M009, M022 |
| V28 | A | Local authenticated signed .duck packages; bounded verification of signature/checksums/arch/compatibility/space, copy migration, staged Active + Previous, health commit, interrupted-update recovery; never delete sole rollback set or overwrite Active in place. | M023–M025 |
| V29 | A/B | Non-destructive emergency/recovery management and safe degraded operation are A. Additional reversible Panic profiles are B. Preserve canonical configuration, evidence and an explicit secure rescue path. | M006, M026, S013 |
| V30 | B | Extensible bounded migration/import for supported MikroTik/OpenWrt/Linux/Kerio-style formats: recognized/convertible/attention/unsupported analysis, provenance, preview, validation, Safe Apply; never execute source commands or promise full unsupported migration. | M045 |
| V31 | B | Overlapping remote networks: explicit translated address model and guided supported NAT/netmap translation, bidirectional route/enforcement preview and clear limitations; never silently merge conflicting address spaces. | M015, M027 |
| V32 | B/C | Capability-gated Wi-Fi AP/station/mesh, regulatory/driver handling and modem QMI/MBIM/PPP with protected SIM/provider secrets are B; qualification names supported devices. Advanced 802.1X/L2TP/legacy-warned PPTP/Wi-Fi WAN/USB/provider methods are C. | S001, U010, U011 |
| V33 | B/C | Observed devices/clients, trusted/guest/IoT/quarantined policy and topology-aware isolation are B. QoS/SQM/CAKE/fq_codel, controlled UPnP/NAT-PMP/PCP, lightweight threat controls and UPS/NUT/hardware diagnostics are C; offload may not bypass policy. | S006, S007, S010, S014 |
| V34 | B/C | Sites/reverse proxy/TLS/DDNS with separated publishing, bounded uploads and layered diagnostics are B. Captive portal/vouchers/terms/roles/branding/localization is C; no HTTPS interception or general hosting-panel expansion. | S008, S009 |
| V35 | A | Bounded storage/RAM/CPU/jobs/queues and sane degraded behavior; state, routing/firewall, Previous and rescue outrank optional diagnostics. Disk/inode pressure, resource exhaustion, failure, reboot and secret/privilege regressions are mandatory. | M007, M025, M028 |
| V36 | A/B | Honest operator/build/install/recovery/security docs and reproducible test evidence are A. Complete supported native matrix/endurance/accessibility/release qualification and factual discovery/launch assets are B; license, domain, visibility and public launch require explicit decisions. | M032, M033, launch contract |

## Fixed Full-mode information architecture

The permanent main areas are **Interfaces; WAN; LAN / VLAN; Routing; Firewall;
Tunnels; DNS / DHCP; Services; System; Logs / Diagnostics**. Keep their locations
and meaning stable across releases. Ordinary advanced objects are not hidden in
More menus or dynamically moved. Dense tables, large lists, filtering, keyboard
workflows and administrator muscle memory are first-class. Classic WinBox-like
organizational density is a reference, not permission to copy another UI.

Easy/Full never create separate configuration universes. Both are declarative:
intent produces visible, explainable routes/firewall/policy objects. Raw Debian
shell remains an expert recovery/debug layer, not the normal Full workflow.
Keep actions local: Interface → +VLAN; Tunnel → +Peer; Client → access policy;
Service → publish/test/logs. Global search/command palette complements this layout.

## Automation, policy and client lifecycle

The product-wide rule is: **Admin says what should happen; DuckOS handles the
repetitive how, while showing exactly what it did.** This applies to Interfaces,
WAN, LAN/VLAN, DHCP/DNS, Routing/Policy routing, Multi-WAN/failover, Firewall,
NAT/publishing, Tunnels, Services, Clients/devices, System, Updates, Backup/restore
and Logs/diagnostics—not only WireGuard or Xray. Full exposes complexity, not
chores. One intent may safely create/update objects across several subsystems;
preview consequential changes, expose generated objects in normal tables with
ownership/dependencies, and support inspection, appropriate editing, tests and
reversal. GUI and Duck CLI share the model; Linux commands are not the normal UX.

| Workflow | Coherent intent and nearby controls |
|---|---|
| WAN / routing / Multi-WAN | Connection type, addressing, DNS, MTU, health, preferred exit/failover/balancing and policy; derive and show route/rule/firewall dependencies, retaining advanced control. |
| LAN / VLAN / DHCP / DNS | Network, membership/tagging, DHCP scopes/reservations, aliases/records/upstreams/split DNS, isolation and access in related validated workflows. |
| Firewall / NAT / services | Both advanced rules and Publish service / Staff → NAS → SMB / Guest → Internet-only; show listener/address, exposure, firewall/NAT changes, dependencies and Test/status/logs. |
| Clients / devices | Contextual access, aliases/tags, reservations, relevant logs, active sessions and related objects. |
| System / updates / restore | Preview → validate → apply → verify → commit/rollback for consequential changes; restore adds hardware mapping and connectivity verification. |
| Diagnostics | Object-local Test / Why? / Capture / Trace / Logs, not manual discovery of shell commands or journal paths. |

Ownership examples include Managed by: WAN Failover, Published Service, WireGuard
or Xray. A subsystem may deviate only for a concrete documented technical reason;
it may not introduce an independent or contradictory configuration workflow.

Every automated object has an inspectable **Managed by** relationship (for
example WireGuard, template, wizard or service publication), stable owner/source,
dependencies and attributable changes. Templates create ordinary visible
objects. Safe detach transfers responsibility explicitly and does not leave
hidden regeneration or shared dependencies broken. Clone supports VLANs, peers,
policies/firewall objects and DHCP scopes; identity/secret conflicts are validated.
Aliases allow policies such as Staff → NAS → SMB, while expert inspection shows
resolved addresses, rules and tables. All automation is visible, explainable,
attributable and reversible; no silent magic.

Publishing a selected local service must preview concrete consequences, e.g.
“This will open TCP 443 on WAN1 and forward it to 192.168.10.20.” Enablement alone
does not publish. Visibility includes listener/firewall/external-test distinctions.

Client routing and authorization normally remain synchronized. Advanced users
may separate them explicitly with mismatch warnings; client routes never count
as an authorization boundary. Scope/routes/key/token changes produce a new
artifact revision, regenerated exports and an explicit remote re-apply notice
where required. Full-tunnel, router access and peer access are separate decisions.
Pre-apply checks include conflicting networks, invalid DHCP pools, unreachable
gateways, overlapping AllowedIPs, missing references and dependency impact.

## Native Xray management contract

Xray is a first-class DuckOS subsystem using the common Engine/API/object/
transaction model, never a parallel configuration system or raw JSON workflow.
VLESS and Trojan are first-class protocols; REALITY is supported where the
selected protocol/transport supports it. Unsupported combinations are explicitly
rejected, not presented as working. Normal operation needs neither manual Xray
JSON editing nor a Linux shell.

Servers/inbounds, users/clients, profiles and subscriptions are explicit stable
objects. Guided server creation selects protocol, transport/security, listening
interface/address/port and TLS/REALITY options, generates appropriate keys/IDs,
previews dependent firewall/routing/configuration changes, and applies through
the shared transaction and rollback mechanism. REALITY guidance covers keys,
short IDs, SNI/destination, fingerprint and related supported parameters, with
safe generation/defaults and validated advanced manual override in Full mode.

Client/profile lifecycle includes create, disable/revoke, expiry, applicable
credential/key rotation, regeneration, clone, tags/aliases and dependency/impact
preview before destructive changes. Credential/key/server-setting changes identify
affected profiles and subscriptions and tell which remote clients must refresh.

Subscription objects support create, profile membership add/remove, visible
current contents, URL obtain/copy, QR, compatible configuration export/download,
disable/revoke and optional expiry/lifecycle restrictions. URLs and QR payloads
share the common secret export controls. Interoperable representations include
supported `vless://` and `trojan://` URIs, QR and subscription delivery without
binding canonical objects to a particular third-party client.

All Xray objects participate in dependencies, preview, transactional apply,
rollback, portable backup/restore, secret lifecycle, granular per-object logging,
contextual Test, human-readable policy and GUI/CLI parity. Declarative intent
such as User/Profile → Xray service → Internet-only / selected destination
networks / selected exit policy / custom routing may compile to Xray plus system
routing/firewall policy; all generated behavior remains visible and explainable.
Transport capabilities constrain access promises; a proxy is not falsely shown
as an L3 tunnel with equivalent peer/network semantics.

Duck CLI exposes these same objects, conceptually `duck xray server ...`,
`duck xray user ...`, `duck xray profile ...`, `duck xray subscription ...`.
These are target command groups, not claims that they exist at the checkpoint.

## Logging and diagnostic safety contract

Logging supports per-subsystem/object **Off / Errors / Normal / Detailed / Trace**,
filters, live tail, search and export. Security audit is a protected separate
policy, not disabled by switching diagnostic logging Off. Use bounded RAM queues,
hard size/count/age limits, rotation and visible suppressed/dropped counters.
Optional internal diagnostics default to a very small ring (approximately
hundreds of recent records is acceptable); the precise bound is policy, never
permission for growth without a hard cap.

External destinations have quotas, reserved-space thresholds and failure/latency
bounds. Disappearance, full storage or failure triggers a clear alert and only a
small bounded internal fallback, never unlimited spill to the OS filesystem.
Logs, packet capture, diagnostics and telemetry cannot exhaust RAM, fill the
system partition or destabilize routing/firewall/services. Redact secrets before
enqueueing/export, and bound captures and subscriber concurrency.

Automated troubleshooting enables only the necessary detailed/trace sources for
a bounded diagnostic window and restores prior settings on success, error,
cancellation, timeout and restart. Effective levels and temporary ownership remain
visible. Test storage removal/full disk, queue overflow, slow consumers, timeout,
crash recovery and secret/audit boundaries.

## Replacement and restore contract

One portable backup → install on replacement hardware → import/drag-drop → detect
missing old interfaces → map WAN/LAN roles using actual hardware evidence and
port identification where available → preview mappings/config/dependencies →
restore → test connectivity → confirm or automatically roll back. Full backup,
readable export and selective restore share schema migrations and the ordinary
Engine transaction path. Secret inclusion is explicitly protected/encrypted.

## Preserved post-V1 architectural reservations

Existing P-001–P-009 remain **C / post-Alpha architectural reservations**, with
implementation originally reserved beyond V1: online channels using the same
Upgrade Engine; heavy capability-gated IDS/IPS; permissioned plugins; optional
multi-site/cloud relay; HA; advisory Autopilot; appliance images/physical rescue
ports; separately secured Owner Web Terminal; deep commercial/PMS/general-hosting
extensions. V1 must preserve their boundaries, not implement them all to claim
feature-complete. They are retained, not silently removed or promoted to V1 gates.

## Normative detailed references

[Product catalog](../requirements/product.md), [matrix IDs](../product-spec-1.0.md),
[network access](../network-access-policy.md), [network understanding](../network-understanding.md),
[UX](../ux-principles.md), [security](../security-model.md),
[storage](../storage-policy.md), [updates](../update-system.md),
[testing](../testing.md), [launch](../launch/README.md).

V1 feature-complete requires all A/B/C V1 capabilities above implemented through
real supported workflows, no production stubs, no critical unresolved safety
defect, and the roadmap acceptance gates. Public readiness additionally requires
native/hardware/fault qualification and explicit release authority.
