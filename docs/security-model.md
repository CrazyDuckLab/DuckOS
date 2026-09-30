# DuckOS security model

Status: living threat model; partial identity/observation implementation
Related decision: [ADR-0002](adr/0002-agent-privilege-boundary.md)
Requirements source: [`requirements/product.md`](requirements/product.md)

DuckOS is a privileged network control product that will run on the boundary of
real home, office, hotel, and branch networks. Its default posture is to reduce
exposure before an attack occurs and to fail without stranding the
administrator or silently weakening policy.

This document records required trust boundaries, threats, and controls. It does
not claim a completed security review, hardened image, Vault, authentication
stack, or tested firewall. [ADR-0008](adr/0008-local-identity.md) describes
the implemented local identity subset and its explicit remaining gates.
Requirements below are not automatically evidence that every control exists.

## Security objectives

DuckOS must protect:

- the availability and correctness of forwarding, firewall, DNS/DHCP, routing,
  and management;
- canonical configuration, last-known-good recovery state, releases, and
  transaction history;
- administrator identities, sessions, roles, and audit evidence;
- passwords, provider/subscription tokens, PPP credentials, VPN private keys,
  API tokens, and certificate keys;
- guest, device, and traffic metadata from unnecessary exposure;
- the integrity of managed host state and the release/update supply chain.

The main security outcomes are least privilege, explicit exposure, safe
defaults, recoverable mutation, secret containment, and honest evidence.
Availability under power loss, disk pressure, and resource exhaustion is a
security property for a router.

## Non-goals and explicit limitations

- DuckOS cannot secure a compromised Debian kernel, boot chain, firmware, or
  physical console in 1.x; those are platform trust assumptions to be
  documented for each support tier.
- DuckOS cannot guarantee rollback of arbitrary manual root-shell changes.
- A router cannot isolate peers whose same-L2 traffic does not traverse it;
  VLAN, SSID, port, switch, or client isolation is required for that guarantee.
- Domain/service identification without interception is incomplete by nature
  because of encrypted DNS, ECH evolution, QUIC, CDNs, and shared addresses.
- DuckOS will not perform HTTPS man-in-the-middle interception for analytics,
  portals, QoS, or service classification.
- GeoIP/ASN lists, heuristic anomaly detection, and a security score are
  supplemental signals, not proof of safety.

## Trust zones and boundaries

| Zone or boundary | Trust posture | Required controls |
|---|---|---|
| WAN / upstream networks | Hostile and failure-prone | No management by default; explicit inbound exposure only; rate/size limits and safe parsers |
| LANs and managed devices | Authenticated network location is not identity | Per-network management permission, segmentation, device state, least access |
| Browser / UI | Fully attacker-controlled input | Server-side authentication, authorization, CSRF/origin strategy, validation, output encoding, no secret reflection |
| External API clients | Potentially hostile or compromised | Versioned API, scoped credentials, RBAC, quotas/rate limits, audit, revocation |
| API and Engine | Unprivileged but high-value control plane | Minimal OS rights, input limits, protected state access, no arbitrary root execution |
| Engine-to-Agent IPC | Critical privilege boundary | Local peer authentication, least-access endpoint, version negotiation, bounded typed messages, independent validation |
| Privileged Agent | High-impact, assumed target | Small semantic surface, least OS capabilities, sandbox/hardening where compatible, structured results, no caller shell |
| Managed daemons/tools | Separate parsers and attack surfaces | Pinned/reviewed dependencies, generated candidates, native validation, constrained invocation, health checks |
| Persistent storage | May be stolen, corrupted, or exhausted | Permissions, atomicity, integrity/recovery, encryption for secrets, capacity reserves and bounded retention |
| Update/installer input | Untrusted until verified | Architecture/compatibility checks, checksums, signatures, space check, side-by-side staging, rollback |
| Plugins/modules | Not trusted merely because installed | Declared permissions/dependencies/resources/architectures; no automatic unrestricted root |

## Threat actors and failure sources

- an unauthenticated remote attacker on WAN or an exposed service;
- a malicious or compromised LAN/guest/IoT client;
- a low-privilege or stolen administrator session exceeding its role;
- a compromised browser, API process, dependency, managed daemon, or plugin;
- malicious configuration, update, backup, subscription, portal, or site input;
- accidental administrator lockout or destructive configuration;
- manual root changes and competing Debian network managers causing drift;
- storage exhaustion, memory/CPU exhaustion, log floods, restart storms, clock
  failure, power loss, or interrupted writes;
- stolen media, backups, support bundles, logs, or crash reports;
- a supply-chain attacker modifying source, dependencies, build output, or
  release metadata.

## Default exposure posture

The required baseline is:

- WAN to management is denied;
- WAN to LAN is denied unless an explicit rule/forward permits it;
- management is allowed only from selected networks;
- SSH is not publicly exposed, root SSH login is disabled, and key-based SSH is
  preferred;
- remote administration prefers WireGuard, NetBird, or another trusted secure
  tunnel;
- stateful established/related traffic is handled deliberately and default-deny
  policies are used where appropriate;
- UPnP/NAT-PMP/PCP, if supported, is globally controllable, per-network,
  time-bounded, visible, audited, and accompanied by warnings;
- an explicitly public management panel is an exceptional administrator choice
  requiring strong warning, hardening, and audit—not a wizard convenience.

Exact initial firewall rules are not defined here and no networking
implementation should be inferred from this posture.

## Identity, authentication, and authorization

DuckOS requires one coherent role system for UI, API, CLI-mediated remote use,
terminal, updates, backups, security, and secrets. Candidate roles from the
requirements include Owner, Network Admin, Helpdesk, Reception, Read Only, and
custom roles; their exact permission lattice remains to be designed.

Required properties:

- strong initial administrator enrollment with no shipped default credential;
- server-side permission checks on every operation and sensitive read;
- session enumeration, expiry, revocation, login history, and relevant audit;
- re-authentication for high-risk operations such as a root terminal or secret
  export;
- CSRF/origin/session fixation defenses appropriate to the selected web auth
  architecture;
- secure password storage and rate-limited/brute-force-resistant login;
- 2FA and WebAuthn/passkeys where practical, without making recovery an
  undocumented bypass.

The authentication/session technology, recovery credential process, WebAuthn
relying-party rules, and headless/API credential format are open foundation
decisions. “Strong auth” is a requirement, not evidence that those details are
resolved.

## Privilege boundary

Normal product flow is:

```text
untrusted client -> unprivileged API/Engine -> authenticated semantic IPC
                 -> privileged Agent -> explicitly owned Linux operation
```

The Agent accepts operations such as applying a validated firewall plan,
configuring an interface, restarting a managed service, or reading bounded
hardware state. It never accepts `RunShell(command)`, a caller-selected binary,
raw shell fragments, arbitrary file paths, or caller-controlled environment.

The Agent revalidates message shape, authorization context required by the
contract, object/operation bounds, target ownership, and preconditions. It
returns typed, size-bounded, secret-sanitized results. Denials and mutations are
audited without logging protected input.

Exact IPC transport, encoding, peer credential mechanism, service user/group,
Linux capabilities, systemd sandbox settings, and compatibility window remain
open decision F-002. See [ADR-0002](adr/0002-agent-privilege-boundary.md).

### Managed command execution

The ban on arbitrary shell APIs does not imply that the Agent can never invoke
a reviewed Debian tool. Where a native API is impractical, an operation may use
a fixed executable and constructed argument vector under strict validation,
with a minimal environment, bounded output/time, no secret command-line
arguments, and no shell interpolation. Each adapter owns its threat review and
rollback behavior.

### Web terminal exception

The proposed advanced Web Terminal is deliberately different from the normal
management API. If implemented, full shell access must be an optional,
explicitly enabled, Owner-authorized boundary with re-authentication,
short-lived sessions, no WAN exposure by default, session termination, and
audit. It must not be implemented by adding arbitrary execution to the Agent
contract, and the UI must state that manual root changes are outside guaranteed
DuckOS rollback. Its isolation and privilege design require a separate ADR and
security review.

## Safe configuration changes

Validation and rollback protect against both malicious and accidental changes.
For LAN address, VLAN, firewall, WAN, DNS, routing, and management access,
DuckOS must:

1. authorize the requested semantic change;
2. create a complete candidate and secret-safe diff;
3. detect conflicts and lockout/fail-open risk;
4. preserve a last-known-good state and required storage;
5. apply a bounded plan through the Agent;
6. verify actual services, policy, and expected management paths;
7. require confirmation when risk warrants it;
8. commit or roll back the entire coherent transaction;
9. surface the exact stage, outcome, and recovery action.

Closing a browser tab is not proof that the management path failed. Conversely,
an API heartbeat alone is not proof that client traffic, DNS, DHCP, or routing
works. Verification is expectation-aware, including intentionally offline
systems. Restore from history or backup uses the same Safe Apply path.

## Secrets and sensitive data

Secrets include at least subscription URLs/tokens, provider and PPP
credentials, VPN keys, API tokens, and certificate private keys. Requirements:

- protected storage and strict file/process permissions;
- encryption at rest where practical, with the threat and key source stated;
- opaque references from canonical configuration;
- masked UI and no plaintext in normal API responses;
- no secret in command-line arguments, environment inherited unnecessarily,
  logs, audit diffs, metrics, crash output, or support bundles;
- explicit encrypted handling for backup/export and secure rotation/deletion.

The Vault trust root, unattended boot, recovery after hardware replacement, and
operator recovery process are unresolved F-004. Until selected, claims such as
“encrypted at rest” must name the key source and attacker model.

## Updates and supply chain

Initial DuckOS software update input is a local package, not an automatic
Internet updater. Local does not mean trusted: before modification the Upgrade
Engine verifies architecture, compatible release/schema path, space and safety
reserve, checksums, and signature. A new version is staged beside the active
one; it never overwrites active in place. The stable invariant is Active + one
Previous known-working release, with commit only after health verification.

Release security must grow to include signed artifacts/metadata,
architecture-specific CI, dependency and static security checks, and
provenance/reproducibility where feasible. Installer and lifecycle instructions
must not rely on blind `curl | sudo sh`. Arbitrary hooks are minimized or
constrained and audited.

Subscription updates are a separate, lower-authority data mechanism. They are
fetched, parsed, validated, diffed, and atomically activated while preserving
last-known-good data. Subscription content cannot acquire software-update or
Agent privileges.

## Availability and resource exhaustion

On a router, an attacker or faulty component must not be able to fill the system
disk merely by generating logs, metrics, events, captures, jobs, or failures.
All such data is bounded by age and size, rate-limited/sampled where necessary,
and subject to emergency free-space protection.

Recovery priority is running system, previous working release, canonical
configuration and recovery backup, then security-critical evidence. Debug logs,
cache, and temporary data yield first. “Unlimited” retention can mean no normal
age expiry only; it cannot disable the emergency reserve.

Services require sane dependency ordering, readiness checks, bounded restart
loops, and backoff. Heavy IDS, metrics cardinality, workers, speed tests, and
retention adapt to measured capabilities, especially on ARMv7. Disk full,
invalid time, reboot, and abrupt power loss are required fault cases.

## Privacy and observability

DuckOS is local-first and does not enable cloud telemetry by default. Future
telemetry must be explicit, minimal, documented, configurable, and anonymized
where possible. Flight Recorder and topology features store metadata rather
than packet payload by default and distinguish observations from inference.

Logs and diagnostics must be useful without collecting secrets or unnecessary
payload. Support bundles are size-bounded and redact passwords, tokens, private
keys, subscription secrets, and sensitive traffic/customer data. Redaction
needs adversarial tests; a best-effort regex alone is not a security boundary.

## Security UI and honest claims

A Security Score, health score, threat alert, app classification, or topology
edge is shown only when derived from named measurable evidence. UI distinguishes
confirmed, observed, inferred, stale, unavailable, and unsupported states.
DuckOS does not invent a reassuring score, claim perfect service recognition,
or hide failure behind “Something went wrong.”

Errors are human-readable with expandable sanitized technical detail. They state
what failed, current/rollback state, whether the network is safe, and the next
action. Guest portal users never receive administrator technical alerts.

## Audit requirements

Audit covers authentication events, session and role changes, configuration and
firewall changes, secret changes without secret values, updates, rollback,
backup restore, terminal sessions, public exposure, and emergency actions.
Records include actor, time, action, target, result, and correlation/transaction
identity where applicable.

Audit storage is access-controlled, tamper-evident where the chosen design can
support it, bounded, and retained longer than ordinary debug data by policy.
The exact integrity mechanism and export model remain open.

## Security verification gates

Before calling a boundary production-ready, record evidence for:

- abuse-case and privilege review;
- authentication/authorization and cross-role tests;
- malformed, oversized, replayed/stale, and unauthorized request tests;
- secret redaction across API, UI, process list, logs, audit, crash paths, and
  support bundles;
- transaction interruption, management lockout, rollback failure, reboot, power
  loss, and disk-full tests;
- real firewall and traffic-path tests for both address families where
  applicable;
- dependency/license/security review and supported architecture runtime tests;
- upgrade signature, wrong-architecture, corruption, migration, and rollback
  tests.

Cross-compilation and unit tests alone are not a security acceptance result.

## Open security decisions

The following block implementation or production claims:

- F-002: Agent protocol, peer authentication, compatibility, and process
  hardening;
- F-003: canonical integrity, journaling, and crash-safe persistence;
- F-004: Vault trust root, unattended boot, rotation, recovery, and backup;
- F-005/F-006: supported Debian baseline and exclusive network ownership;
- F-007: rollback scope for Debian packages and shared system state;
- concrete web authentication, recovery, RBAC, terminal, plugin, audit
  integrity, and release signing designs.

Each decision must state its threat model, failure behavior, ARMv7/resource
cost, migration path, and verification evidence.
