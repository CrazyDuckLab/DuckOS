# Current execution roadmap

Checkpoint: 2026-09-10. Continue here without restarting architecture planning.
Frozen feature definitions/classes: [V1 scope](V1_SCOPE.md).
Historical twelve-stage roadmap remains the product sequence; current work spans
stages 4, 6 and 7, with tools/observation established and stage-5 native evidence
still outstanding. No stage exit is implied by partial package implementation.

## Exact current milestone and first task

**Finish the real static-network Safe Apply activation path, starting with
boot recovery + management-path verification.** Code checkpoint `6230df2` already
has the adapter, kernel coordinator, networkd reload, file recovery and boot unit.

Start in `internal/network/boot_recovery.go`, `boot_guard.go`,
`managed_routing.go`, `internal/safeapply`, and `cmd/duck-agent/main.go`.

1. Add the no-pending root-enrollment regression described in PROJECT_STATE.
   Validate current private routes/policy ownership before permitting networkd.
2. Complete the two-stage cold-boot model: restore/check files before networkd;
   restore/reconcile the committed or captured kernel/filtering state before
   reporting networking ready. Missing/corrupt journal, failed boot dependency,
   revoked enrollment and unknown drift must remain blocked/recovery-required.
   An unconfirmed candidate must never become committed merely through reboot.
3. Implement the trusted management verifier using actual source/interface/
   route/listener evidence, not an always-success callback. Preserve single-NIC
   and IPv6 management and account for local-output/routed paths and offload.
4. Wire explicit enrollment, stores, boot gate and managed routing adapter into
   Agent/Engine startup and existing network IPC. Expose candidate/status/apply/
   confirm/rollback through authenticated RBAC API and UI/CLI. Do not enable
   unsupported dynamic/link/DNS changes or broaden service permissions blindly.

First-block acceptance: an authorized static routing/firewall change goes through
one durable Engine/Agent transaction, actual Linux operations and verification;
lost HTTP/IPC confirmation, restart and timeout restore before-state or preserve
an explicit safe recovery state. Invalid/unauthorized requests cause no host
mutation. Local fault tests cover every boundary before native execution.

## Remaining dependency order and gates

| Block | Required implementation / outcome | Acceptance and required tests |
|---|---|---|
| R1 Current static activation | First task above; ownership/boot/management checks, canonical stores, API/Agent transaction wiring. | Unit/contract/RBAC/malformed/oversized/redaction; journal interruption, lost ack, stale revision, missing state, failed restore, cold-boot simulation; native namespace/systemd qualification later. |
| R2 Network-wide mutations | Extend the same transaction to physical links, bridges, VLANs, addresses/main routes, WAN DHCP/static/PPPoE, DHCP/DNS and qualified IPv6 RA/DHCPv6/PD. Add MTU Auto/override and explicit supported capability handling. | Pool/network/gateway/reference conflicts; exact rollback of links/leases/routes/rules/nft/services; live clients, renewals, restart/reboot, single-NIC and VLAN trunk, IPv4/IPv6 and lockout cases. |
| R3 Operator workflow | First Flight, stable Full navigation/dense objects, Easy over same model, real network forms, previews/history and first-class Duck CLI. Keep context actions/help/Show CLI near objects. | API/CLI/UI parity, auth/RBAC, safe defaults, keyboard/accessibility, preview/apply/result consistency, confirmation reconnect and no fake success/mascot without connectivity evidence. |
| R4 Security and owned objects | Finish users/session/high-risk auth/recovery, Vault lifecycle and startup integration, protected audit, dependencies/impact, generated-object ownership/detach, visible templates/clones/aliases. | Last-owner/session/permission/re-auth regressions; secrets never in logs/diffs/audit/process args; key loss/revoke/rotation, ownership conflict and dependency deletion tests. Safety work runs alongside R1–R3, not after insecure activation. |
| R5 VPN/client provisioning | Connect SQLite catalog + Vault + export provider to WireGuard, complete live peers/access isolation/route lifecycle and client UI. Integrate HAPP/subscriptions and NetBird; implement Xray native VLESS/Trojan/supported REALITY server/user/profile/subscription objects and guided lifecycle through the common model, never a raw JSON editor. | Real supported transport paths; peer/router/subnet/service isolation where applicable; IPv6/full-tunnel/fallback; exact QR/config/file/URI interoperability; rotation/revoke/expiry and affected-client refresh notices; dependency/backup/logging/Test and GUI/CLI parity; subscription failure preserves last-good. |
| R6 Policy/resilience | Multi-WAN, transport-neutral route groups, health/hysteresis, selective/domain/service-pack policy, overlapping-network translation and declarative publishing. | Shared preview/kernel path, longest-prefix/metric/rule decisions, no fallback leaks, failover/stickiness, DNS/CDN/QUIC uncertainty, NAT translation round-trip, visible source-owned exposure and anti-lockout. |
| R7 Logging and understanding | Bounded logging/audit/temporary trace, Doctor, what-if/Why, topology, packet capture/PCAP/traceroute/Test, monitoring/time/history/Speed Lab. | Flood/queue overflow, slow subscriber, hard RAM/disk/inode limits, external storage disappearance/full, bounded fallback/alerts, trace restoration on cancel/crash; secret sanitation, real measured paths/metrics and uncertainty. |
| R8 Recovery/lifecycle | Portable full backup + readable/selective exports, schema migration, port remapping/identify, safe restore and non-destructive emergency mode; signed .duck Upgrade Engine, Active/Previous and storage reservations. | Wrong/corrupt/malicious archive/signature/arch, secret restore, missing NIC mapping, power/process loss at every transition, migration failure, full disk/inodes, preserve Previous, restore connectivity then confirm. Signing trust bootstrap/host mutation contract must be explicit before implementation claims. |
| R9 Remaining V1 services | Supported Wi-Fi/modems, devices/trust, QoS/SQM, UPS/hardware health, controlled UPnP, sites/TLS/DDNS/portal and migration adapters; advanced UX/search/clone conveniences. | Device-specific capability refusals, driver/regulatory/MTU behavior, resource budgets, client isolation, certificates/secret upload safety, supported import previews and exact unsupported-item reports. Use the same Engine/transaction boundaries. |
| R10 Integration candidate and qualification | Clean exact-commit installable candidate, operator install/recovery commands, isolated native lab plan; then supported architecture, reboot/fault/security/resource/endurance and UX/release evidence. | No production stubs, all V1 acceptance gates met; native amd64/arm64/armv7 (XU4 resource reference), real client traffic, no critical bugs, operator docs, license/signing/publication approval before public 1.0.0. |

Classes B/C may follow a safe internal Alpha, but remain in R1–R10 until their
V1 acceptance is met. Existing post-V1 reservations require compatible contracts,
not premature full implementations. Do not reduce the frozen scope silently.

## Verification workflow

Use targeted tests while implementing, then `make check`, `make build-all`,
`make check-cross`, and diff review for the completed block. Security-sensitive
changes additionally require relevant fault/abuse/isolation regressions; no
exceptions for speed or budget. Record actual evidence and commit/push normal
verified states. Native tests must identify exact commit, environment and
topology; cross-compiled tests are not a native pass.

Do not access `duck-dev-01` now. Continue locally feasible implementation first.
When genuine target verification is needed, prepare a clean installable candidate
and exact isolated test/recovery instructions; ask for the required target action
without touching the host. Physical NIC blink, radios/modems, real boot, power
loss and packet-path qualification necessarily need appropriate hardware/Linux.

## V1 feature-complete and release-ready conditions

Feature-complete means every A/B/C V1 capability in the frozen scope has a real
supported Engine/API plus GUI/CLI workflow, explicit ownership and bounded
resource behavior, and passes applicable local integration/security/regression
tests. No remaining unavailable provider, simulation-only enforcement or decorative
placeholder may be counted as a completed feature. Safety-critical gaps and
unresolved release-path blockers prevent this designation.

Release-ready additionally requires the supported native/platform/device matrix,
actual traffic/isolation/recovery/endurance evidence, install/update/backup
qualification, complete docs and explicit licensing/signing/publication decisions.
An internal Alpha, passing tests or another commit is neither condition by itself.
