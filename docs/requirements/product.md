# Product requirements

This catalog defines product intent, not implementation status. Detailed
acceptance gates live in the [scope matrix](../product-spec-1.0.md), with
architecture, security, storage, update and testing contracts in the adjacent
documents. Changes to scope require an explicit recorded decision.

## Consolidated V1 baseline

The frozen V1 scope is the deduplicated continuation of this catalog.
It retains all requirements below and specifies
Alpha blockers, architecture-now/later work and post-Alpha timing without
silently removing capabilities. It is normative for the consolidated additions:
stable dense Full navigation, one Easy/Full model, product-wide declarative
workflows, GUI/CLI parity/help, managed-object lifecycle/dependencies, portable
hardware-remapped restore, synchronized tunnel routes/isolation and client
refresh, MTU/PMTU, native Xray VLESS/Trojan/supported REALITY objects, bounded
per-object logging/trace/capture and contextual diagnostics. Its detailed
contracts define acceptance; these are not independent parallel feature lists.

## Platform and configuration

DuckOS runs on supported Debian without a custom kernel or bootloader. One API,
configuration model, UI, backup and update lifecycle span amd64, arm64 and armv7.
ODROID XU4 constrains core resource use; stronger hardware can enable heavier
analysis. Capability discovery covers CPU, memory, storage, NICs, offloads,
radios, modems, USB, drivers and kernel features rather than board-name guesses.

Canonical schema-versioned objects have stable IDs and separate desired,
observed, rendered and historical state. Logical interfaces map explicitly to
hardware identifiers; replacement hardware triggers mapping review. The Engine
is the sole semantic writer. The Agent has authenticated bounded IPC, independent
validation, explicit ownership and no general shell operation. API, UI and CLI
share validation, jobs, errors, audit and transaction semantics. Reconciliation
detects drift instead of silently overwriting another network manager.

## Networking and management safety

WAN includes DHCP, static addressing, VLAN composition, PPPoE and qualified
dual-stack acquisition/prefix delegation. Additional provider methods include
802.1X, L2TP, legacy-warned PPTP, Wi-Fi uplink and USB tethering subject to
capability and security qualification. Models include IPv6 from the outset.
LAN and VLAN networks include gateways, isolation, DHCP pools/reservations,
leases, local DNS, upstream selection and IPv6 services. Bridges and trunk
router-on-a-stick topologies require explicit membership and ownership.

Management Access Policy is independent of WAN/LAN role. It supports trusted
source IP/CIDR, selected interfaces, separate SSH and Web/API exposure, IPv4/IPv6,
and dedicated administrative WireGuard tunnels. Public management is denied by
default, but a WAN interface may legitimately carry trusted management traffic.
Client-service exposure never implicitly authorizes management exposure.

Installation preserves existing interface assignments, network/firewall state,
SSH port and SSH availability. Existing management paths are detected and
protected where possible. A separate physical LAN is not required: Edge/VPN Node
mode supports one physical uplink, client tunnels and an optional distinct admin
tunnel. VPN clients can forward/NAT through the uplink while management remains
governed independently. This includes VPS, cloud VM and single-NIC appliances.

Safe Apply covers management addresses, interface roles/disablement, VLAN/bridge
membership, default and management routes, ip rules and input firewall changes.
A durable full rollback snapshot precedes staging and apply. Actual health is
verified before confirmation; timeout, lost confirmation or verification failure
rolls back. Recovery restores relevant links, addresses, VLANs, bridges, routes,
rules, nftables, DHCP configuration/state and canonical configuration together.
Temporary narrowly scoped management protection may preserve an active session.
Reboot never commits an unconfirmed candidate. Last-known-good configuration,
management address transition/recovery aliases and a local-console rescue path
remain available. Confirmation windows account for reconnection latency.

## Routing, firewall and service exposure

[Network Access Policy](../network-access-policy.md) adds shared intent-driven
presets, network defaults and per-client overrides across routed/proxy transports
and LAN/VLAN. Internet, local networks, peer access, router services and management
are separate controls, compiled by the Engine with topology-aware capabilities.

Stateful nftables filtering has explicit input/forward policies, NAT/masquerade,
port forwarding, selected hairpin behavior, IPv4/IPv6 and bounded counters/logs.
Multi-WAN supports primary/backup and qualified weighted/load-balanced routing,
multi-signal health, hysteresis, recovery thresholds and explicit session stickiness.
Unified traffic policy matches devices, networks, destinations, services,
protocols and schedules to DIRECT, WAN, tunnel, route group or BLOCK with explicit
fail-open/fail-closed behavior. Route profiles/groups are transport independent.
Domain routing and editable/versioned service packs use available DNS/IP signals,
with honest CDN, encrypted DNS and QUIC limitations and no HTTPS interception.

Each enabled listener declares its required port/protocol/interface/family.
Enabling a service does not open the firewall. Contextual **Allow on WAN** shows
an inspectable diff, source restrictions, selected uplink, TCP/UDP/both and IPv4/IPv6,
then uses Safe Apply and verifies actual state. Rule ownership and dependencies
prevent duplicates and prevent deleting a rule still needed by another service.
Incompatible listeners, overlaps and policy conflicts are reported before apply.

Network → Port Exposure lists service/configuration, port, protocol, uplink,
source restriction, listener state, firewall state, external reachability and
actions. The same warnings appear on subscription, WireGuard, Xray and site cards.
Configured, listening, locally permitted and externally reachable are distinct
states. An open nftables rule does not prove Internet reachability; upstream
provider firewalls/security groups may block traffic. External tests require
actual evidence; otherwise reachability remains not verified.

## VPN, subscriptions and client exports

WireGuard, Xray and NetBird integrate through supported transport adapters, not
independent policy engines. Provider subscriptions use bounded parsing,
normalization, deduplication, preview/diff, atomic last-known-good activation,
refresh scheduling and health history. Subscription refresh is separate from
software update and has no software-install privileges.

The client builder has Node, Client, Client Profile, Subscription and Renderer
entities. Clients/profiles support CRUD, one or multiple nodes, protocol/inbound
selection, per-client credentials, enable/disable, expiration, tags, notes,
rotation, preview, rebuild and audit. Initial output includes supported
HAPP-compatible subscriptions; the model permits raw Xray, sing-box, Clash and
future formats without coupling core objects to a particular client.

One reusable exporter produces URL, raw text/configuration, downloadable file
and QR-compatible payload. Every subscription exposes Show QR, Copy URL and
Export; QR contains the exact generated URL. Every WireGuard client exposes
Show QR, Download .conf, Copy config and Rotate keys. Its complete standard
client configuration is byte-identical in QR, copy and downloaded file and is
scannable by standard mobile WireGuard clients. Changes to URL, token or keys
invalidate/regenerate corresponding exports.

Secret exports require authenticated, authorized access. Private keys and
credential-bearing subscription URLs never appear in logs, ordinary audit
payloads or public/static assets. Viewing/exporting is audited using non-secret
metadata. The large phone-scannable modal warns that anyone scanning the QR can
use that connection. Tests cover exact payload/file equality, rotation,
unauthorized access and secret absence from logs/audit. These exports share
the main Engine/API, RBAC, history and secret storage boundaries.

## Hardware, clients and network services

Wi-Fi AP/station/mesh features derive from actual iw/driver/regulatory capabilities,
including supported bands and roaming options; unsupported combinations are
explained. Cellular management qualifies QMI/MBIM/PPP and supported devices,
protects SIM/provider credentials, and uses bounded watchdog escalation.
QoS/SQM uses capability-qualified tc, CAKE/fq_codel and per-network/device limits.
Hardware offload must not silently bypass policy or invalidate counters.

Device inventory, trust states, guest/IoT isolation, quarantine and microsegmentation
use observed topology and explicit policy. IDS/IPS and heavier analytics are
optional capability-gated modules; core safety does not depend on them. DNS
security, bounded threat lists and temporary blocks are explainable/recoverable.
UPnP/NAT-PMP/PCP are explicitly enabled, scoped, time-bounded, visible and reversible.

Sites/reverse proxy separate HTTP(S) publishing from port forwarding, validate
uploads/configuration, protect certificate keys and diagnose DNS/TLS/backend
layers. Certificate issuance/renewal and DDNS have explicit failure behavior.
Captive portal supports network/SSID isolation, safe captive detection,
terms/vouchers, roles, branding and localization without HTTPS interception.
Commercial/PMS extensions remain separate from core routing.

## Security, diagnostics and experience

First Flight covers language, secure Owner enrollment, capabilities, topology,
WAN/LAN or Edge selection, DNS/security, review, Safe Apply and real connectivity.
Authentication includes strong passwords, session expiry/enumeration/revocation,
roles, high-risk reauthentication, recovery and a qualified MFA path. Secrets
use opaque references with explicit trust-root, rotation, backup and recovery
semantics. Public root SSH is not a default management mechanism.

Audit covers identity, configuration, exposure, exports, updates and recovery
without secrets. Integrity/drift detection offers inspect/restore/safe import.
Support bundles are bounded and sanitized. Panic modes preserve basic local
services and secure recovery while disabling selected risky features.

Duck Doctor explains actual client-to-target layers and uncertainty. Conflict
Detector and Policy Simulator inspect the same configuration/route semantics.
Duck Speed Lab measures selected WAN/tunnel/node/network/device paths with named
metrics, resource limits, scheduling warnings and bounded history. Scores must
derive from evidence, never invented health. Monitoring includes timelines,
Internet flight recorder, observed/inferred topology, notifications with
deduplication/hysteresis, external integrations, time/NTP, UPS and hardware health.
Diagnostics do not collect packet contents or unlimited metadata by default.

The responsive dark visual system is coherent, restrained and accessible,
with contextual Basic/Advanced controls, keyboard/touch support, localization,
search and route composition. Safe Mode is unmistakable. Errors explain failure
stage, current state, rollback outcome and next action with sanitized technical
detail. One compact text-based duck mark is shared by UI, CLI and installer;
cartoon/emoji mascots are not the primary identity. Decorative effects degrade
gracefully on constrained clients. Correctness takes precedence over animation.

The global [UX acceptance contract](../ux-principles.md) requires guided intent,
recommended controls, consequence preview, computed Effective Access and an
expert inspection path. Toggle state alone is not evidence of effective access.

## Durability, backup and updates

Bounded revision history supports inspect/diff/rollback. Structured backups
include optional encrypted secrets, migration/validation, hardware remapping and
transactional restore. A protected recovery copy survives routine retention.
Storage policy combines bytes, percentage, inodes, growth, filesystem and durable
reservations. Running system, Previous release and configuration/recovery outrank
logs, metrics, debug and caches; optional work degrades first under pressure.

Local signed .duck packages enter one Upgrade Engine. Bounded input, signatures,
checksums, architecture, compatibility and free space are verified before staging
beside Active. Migration operates on a copy. Health verification precedes commit;
interruption or failure restores a known-good state. Stable retention is Active
plus one Previous; the only rollback copy is never deleted to make room. Cleanup
is bounded/retryable. Host Debian mutations have a separate recovery contract.
Future online channels reuse this engine and package format.

Service supervision, boot recovery, job scheduling and events are bounded and
recoverable. Optional plugins declare permissions, dependencies, resources,
capabilities and architectures; installation does not grant unrestricted root.
Future multi-site, optional cloud, HA, appliance images and recovery ports must
not weaken local operation or require a separate product architecture. Advisory
autopilot does not bypass confirmation. Any full administrative terminal is a
separate optional, reauthenticated and audited security boundary.

## Qualification

Public discovery documentation describes real capability and distinguishes plans
from qualified availability. README, deployment/migration guides, issue templates,
hardware reports, screenshots, metadata and future site routing remain factual.
Canonical domain, license, release version and publication/indexing require
explicit launch decisions. Private preparation must not change repository
visibility, publish releases, announce the project or enable staging indexing.

Reproducible builds and signed provenance identify exact revisions, dependencies
and targets. Debian installation validates environment, ownership, space and
conflicts, creates least-privilege services and preserves remote access. Native
installer, update, backup and rescue instructions are part of acceptance.

Tests include schema/migrations, unit/API/UI, security/RBAC/redaction, isolated
Linux integration and real client traffic, IPv4/IPv6, reboot, process/power loss,
disk/inode pressure, failed rollback and resource limits on reference hardware.
Single-NIC tests must prove trusted management survives WAN changes, client VPN
traffic exits the uplink, and lost confirmation restores connectivity. Two-NIC
and VLAN trunk paths remain separately required. Cross-builds are not native
test evidence. Exact platform/device/dependency versions and artifact hashes
accompany results; missing or unsupported observations are never reported as healthy.
