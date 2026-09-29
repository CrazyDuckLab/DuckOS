# Project state — 2026-09-10

Code checkpoint: `6230df277bb6d9049117bd519904f548b1634ec7`.
The subsequent handoff commit changes specification/documentation only.
Repository: `CrazyDuckLab/DuckOS`, branch `main`, SSH origin
`git@github.com:CrazyDuckLab/DuckOS.git`.
Local checkout: `<repo root>`.

## Completion truth

DuckOS V1 is **not feature-complete, not an Alpha-qualified router and not a
production release**. The last communicated estimate was approximately 13% of
the full requested product, not a measured completion metric. Do not derive a
new percentage from file counts, commits or passing tests. Frozen scope now
includes all consolidated UX/CLI/logging/recovery requirements.

## Implemented and working end-to-end

- Real System Overview: React → API → Engine → peer-authenticated Agent → Linux
  hostname, uptime, CPU/load, RAM/storage, interfaces, addresses, routes, DNS,
  basic service and nftables observations. Missing/stale/unsupported state is
  explicit; production collectors do not substitute test data.
- First Owner enrollment and login UI/API, local password authentication,
  built-in RBAC, session expiry/revocation and identity audit. Optional private
  TLS certificate loading exists; full certificate/MFA/recovery lifecycle does not.
- Authenticated candidate preview and offline traffic simulation through the
  API/Engine. Simulation shares policy/forwarding semantics with generation and
  labels candidate assumptions; it is not a live reachability test.
- Overview topology UI uses observed interfaces/routes and timestamps. It is
  not yet the complete WAN/bridge/VLAN/tunnel/device relationship graph.
- Pinned build tooling, Go/React production assets, Linux cross-builds and a
  fresh Debian 13 amd64 TEST installer with checksum/ELF/path/ownership guards.

## Implemented components, not installed product features

| Component | Evidence / remaining integration |
|---|---|
| Canonical network schema/renderers | WAN/LAN, VLAN, static dual-stack, DHCP/DNS, firewall/NAT, independent management, private routes/rules; validation and bounded deterministic artifacts. Complete runtime editing is absent. |
| Safe Apply | Durable JSON journal, independent watchdog, outcome reconciliation and reboot rollback core. Engine/Agent network IPC exists but startup and public mutation API/UI remain gated. |
| Real Linux adapters | Scoped nftables, private routes, policy rules, artifact writes, DNS syntax/lifecycle, networkd reload and boot checks. Fixed executables/arguments and captured-state restore; no arbitrary shell. Mac tests inject executors, not real kernel mutations. |
| Managed routing adapter | Actual Safe Apply controller + persistent files + daemon/kernel sequencing tested with fault injection. Only unchanged static interfaces, firewall/NAT and private routes/policy rules are admitted. |
| Boot recovery | `--recover-network-artifacts`, `OpenExisting`, pre-networkd oneshot, persistent dependency/loaded-state verifier; file recovery preserves pending journal. Packaged but not enabled/connected to a complete runtime boot pipeline. |
| DNS/DHCP service | Dedicated non-login identity, private leases, explicit config and readiness marker gates. Installer neither configures nor starts/enables it. Root lifecycle and recovery primitives exist. |
| Secret exports | Reusable exact URL/config/file/QR payload pipeline, WireGuard config generation, authorized/audited exports, redaction, large QR modal and independent QR decoding tests. No live client provider or completed client page. |
| Vault | Explicit private local master key, per-write derived authenticated encryption, context-bound references and bounded encrypted store. Not provisioned/wired into installed client lifecycle; key rotation/backup recovery incomplete. |
| SQLite catalog | Client metadata, optimistic revisions, bounded history/audit and private exclusive store using modernc SQLite. Not yet the canonical network store or active export provider. |

## Missing product workflows

No complete GUI/CLI-controlled WAN/LAN mutation, bridge/VLAN lifecycle,
DHCP/RA/PD/PPPoE lifecycle or network-wide Safe Apply. No complete Multi-WAN,
DNS management, live WireGuard/client provisioning, Xray/NetBird/HAPP subscription
integration, Duck CLI, Doctor/Speed Lab, bounded product logging/monitoring,
portable backup/restore, signed .duck Upgrade Engine/Active-Previous, Wi-Fi/modem
management, clients/trust controls, sites/portal, migration or full first-run
network flow. The fixed dense Full-mode IA and new convenience requirements are
specification, not completed UI.

There are no intended production mock-data paths. `synthetic*` executors and
fixtures are confined to tests. Optional Engine services that lack providers
fail unavailable; their packages/tests do not mean the feature is available.
The ordinary installed Agent is still networking read-only. Do not remove its
activation gates to obtain a superficially working demo.

## Known gaps and security review follow-ups

1. The immediate blocker to runtime activation is complete management-path
   verification and cold-boot recovery ordering, followed by explicit enrollment
   and Engine/Agent/API integration. Boot artifact recovery alone does not restore
   packet filtering/routes after kernel state is lost at reboot.
2. `RecoverRoutingArtifacts` validates bindings/artifact state on its no-pending
   branch, but does not yet validate all current private-route/policy intent
   against revoked table/rule enrollment there. Add that regression/gate during
   the next boot-integration block before enabling it.
3. Forwarding barriers do not cover local-output, L2 bridging or XDP/tc/offload
   bypasses. Competing manager ownership and offload admission still need complete
   activation checks. Native route/networkd protocol/scope behavior is unqualified.
4. Dynamic leases, link retirement, main-table and address restoration are outside
   the static adapter. DNS process-active is not DNS/DHCP readiness. Preserve those
   explicit refusals until real capture/restore/readiness exists.
5. Ordinary Agent unit still has read-only filesystem/network restrictions;
   mutation/recovery units and privileges need deliberate integrated qualification,
   not blanket relaxation. Installer leaves networkd/DNS activation disabled and
   root network journal uninitialized, requiring explicit provisioning.
6. General resource-bounded product logging, protected audit retention/export,
   temporary trace restoration and external-storage fallback are not implemented
   by merely having systemd log rate limits.
7. SQLite migration/backup, Vault/master-key recovery, high-risk re-auth/MFA and
   complete user administration remain unfinished. Audit/history are bounded
   foundations, not a complete tamper-resistant operational audit system.
8. Existing long-form foundation documents contain historical future-tense
   implementation notes. Current ADRs and this state snapshot supersede those
   notes; do not reopen resolved topology/IPC/storage choices from stale prose.

## Test status

Final code verification on 2026-09-10, Darwin/arm64, against the source saved as
`6230df2`:

- `make check`: PASS — formatting, schema contract, module integrity, Go vet,
  complete Go race suite, TypeScript, frontend formatting, **32 UI tests**,
  **23 installer tests**, **7 documentation/launch tests**, shell syntax and diff.
- `make build-all`: PASS — production frontend and Linux amd64/arm64/ARMv7 binaries.
- `make check-cross`: PASS — network/Vault/catalog test binaries for all three
  Linux architectures (9 binaries); compile only, not execution.
- New security regressions cover exact service/unit/dependency ownership,
  missing/corrupt state behavior, no silent journal initialization, pending boot
  phases, no boot confirmation, stale/foreign profiles, failed/lost service
  acknowledgements, guarded route/rule/firewall changes and Safe Apply recovery.
- Existing auth/RBAC, secret/QR redaction and export equality, malicious input,
  artifact/path/permission, state/fsync/fault and rollback suites were retained.
- No Linux target execution, real systemd boot, real forwarding, phone scan,
  power-loss, hardware or native supported-architecture qualification was performed
  during this checkpoint. Native opt-in namespace tests exist in CI configuration;
  GitHub Actions success was not independently verified.
- The pre-commit build labels contained `8eeed64...dirty`. These are build evidence,
  not an installable release claim. Rebuild the clean handoff revision when preparing
  exact-commit artifacts; do not distribute stale dirty payloads.

## Local work commands

```sh
cd <repo root>
source scripts/env.sh
make check
make build-all
make check-cross
git diff --check
```

Toolchains are repository-local (`.tools`), with caches in `.cache`. Tests using
local sockets may require sandbox permission. Existing SSH authentication is
configured; do not generate keys or expose credentials again. Preserve retained
`.previous-*` local builds unless an explicit bounded cleanup is approved.
`duck-dev-01` has not been touched and must not be accessed implicitly.
