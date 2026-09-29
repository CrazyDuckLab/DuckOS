# Final targeted backend verification — 5ec79fa

## 1. Verified HEAD

`5ec79faddc25009633c340663e683732d0e871d3`, branch `V1 UI qualification branch`.
Scope: `4038b6d397ddadb49a35a4a2bdba751fb0934897..5ec79fa`, necessary
surrounding call sites and previous security regressions only. No repo-wide audit,
UI work, remote Linux access, commit or push.

At entry, `cmd/duck/main.go` and `internal/engine/config.go` had uncommitted
formatting-only changes. Those changes were preserved. The local fixes described
below are additional uncommitted changes, not part of the verified HEAD.

## 2. Status of the ten C-class blockers

PARTIAL means some relevant implementation exists, not permission to freeze.
No blocker is CLOSED as a complete production workflow.

| # | Blocker | Status | Production evidence / remaining gap |
|---|---|---|---|
| 1 | Real domain listers | PARTIAL | API startup now registers three source-backed listers: interfaces, routes, QoS classes. Sixteen other registered domains remain unsupported placeholders; QoS source remains unavailable. Route IDs use destinations (including `/`), incompatible with the RBAC ID grammar and not unique across tables. |
| 2 | Generic config coordinator | PARTIAL | Startup constructs Config/Networking, but EnableConfig still does not assign `Engine.config`; the public handler therefore receives nil. Explicit CLI request IDs were compared to freshly allocated transaction IDs only after scheduling runtime work. Validation/diff remain unavailable; status advertises no actions. |
| 3 | Guarded QoS execution | OPEN | Controller now attempts rollback and host preserves topology on apply error. Agent startup still deliberately disables the service. No durable, serialized management-safe tc transaction is supplied by the diff. |
| 4 | WG/Xray CRUD + export | OPEN | No production API/Engine/Agent provider registration. Xray added PUT operations persist catalog objects without invoking runtime. WireGuard gained peer adapter methods but not a production pipeline or valid persistent secret configuration. |
| 5 | Per-object RBAC | PARTIAL | Conditional per-row filtering exists and deny precedence is preserved. No production Policy assignment/loader is wired. Mutation/export authorization is not per-object. Filtering still exposes denied IDs in Issues; route IDs cannot be authorized by the current grammar. |
| 6 | CLI/API parity | PARTIAL | Network commands moved to public config routes, but that coordinator is unavailable and caller request IDs are not implemented. Upgrade/backup/provenance/diagnostic commands still use private Agent IPC as normal commands; authenticated API/Engine parity is not established. |
| 7 | Durable update/recovery | PARTIAL | Activate calls VerifyCandidate; health checks now call the cancellation wrapper. No production orchestrator is attached, no durable state/anchor/migration recovery workflow is supplied. Agent still installs NewUpgradeHost(nil). |
| 8 | Durable backup/restore | OPEN | JournalStore can write/reopen a file, but ExecuteRestore still requires *RestoreJournal, not JournalStore; no execution integration uses the new storage. Agent still installs NewBackupHost(nil). |
| 9 | Anycast execution | OPEN | Added an evidence-read IPC/Engine accessor, not an advertisement/withdrawal executor or live observer. Agent startup does not enable a source; the fallback reports Startup=ready with empty evidence. No public Anycast route is mounted. |
| 10 | ZTP production wiring | OPEN | Added a separate manager and IPC wrapper, but startup does not load a key or register the manager. Redemption does not authenticate an issued token; TTL is not checked. Replay retention is evicted. Fallback status reports Ready=true. |

Composition evidence: [API startup](../../cmd/duck-api/main.go),
[Agent startup](../../cmd/duck-agent/main.go),
[registered listers](../../internal/objectlisters/listers.go),
[Config](../../internal/engine/config.go),
[API object filter](../../internal/api/objects.go),
[CLI](../../cmd/duck/main.go).

## 3. New Critical/High

No remotely reachable Critical exploit was established. Most dangerous new
helpers are still unmounted or disabled; that limits present reachability but
does not make them safe to enable or freeze.

### Locally fixed High defects, with regressions

- **Fabricated configuration receipts:** outcomeFromState produced committed or
  rolled-back receipts from a pending transaction or an outcome-less snapshot,
  even using a fresh local timestamp. Fixed: require reconciled state, no pending
  transaction, matching recorded outcome/current revision and recorded completion
  time; otherwise return unknown/recovery, never success. Negative tests reproduced
  the previous behavior. Existing positive coordinator tests still pass.
- **Caller receipt bypass:** Config.Reconcile accepted the supplied receipt into
  the journal through reconcileFromOutcome without obtaining Agent evidence.
  Restored fail-closed behavior; only the existing authenticated Networking
  status/watch path may reconcile. Regression checks rejection.
- **Apply-before-ID-refusal:** Prepare scheduled asynchronous runtime apply, then
  rejected a different caller request ID and attempted rollback concurrently with
  submission. Until durable caller IDs are supported, explicit request IDs now
  fail before admission or IPC. This is containment; CLI apply remains blocked.
- **QoS rollback cancellation/false restoration:** rollback inherited canceled
  request context and always appended “apply reverted” even when rollback failed.
  Fixed independent bounded rollback context and truthful warning conditional.
  The regression failed before the fix and passes afterwards.

### High-risk software blockers documented, not redesigned

- **ZTP authentication and replay:** Redeem accepts any nonempty unseen token;
  computing a keyed digest of caller input is not verification of an issued token.
  IssuedAt is set at redemption, so the claimed token-age check does not exist.
  After MaxRedemptionJournal entries, an old digest is deleted and an old token
  can be redeemed again. Reload silently skips damaged lines, also losing replay
  evidence. No parent-directory fsync or multi-process exclusion is present.
  This requires a real issuance/authentication/durable redemption design, not
  merely supplying a deployment key. [Manager](../../internal/ztp/ztp.go).
- **WireGuard runtime/persistence:** new peer commands route wg-style `set` through
  the available ip runner, pass preshared-key bytes as an argument rather than a
  protected file, and can expose arguments on the runner error path. The adapter
  now persists ExportServerConfig, whose PrivateKey is a redacted placeholder;
  that is not a bootable wg-quick configuration. Peer persistence reads the catalog
  before the API updates it, and peer deletion passes only ID to an adapter that
  requires ServerID/PublicKey. Correct transactional/provider wiring is required;
  no real host changes were made. [Adapter](../../internal/wireguard/adapter.go),
  [runner](../../internal/wireguard/adapter_linux.go), [API](../../internal/wireguard/api.go).
- **False readiness:** nil Anycast/ZTP sources produce ready responses. An empty
  evidence model is not an operational provider. These are reachable IPC read
  paths, though public management routes are not mounted. Readiness needs a real
  provider/capability contract, not unconditional success.
- **Restore crash safety:** JournalStore is disconnected from ExecuteRestore.
  Its in-memory cache advances before persistence succeeds; reload skips corrupt
  records and rename lacks parent-directory fsync. Successful reopen tests do not
  establish crash recovery. No verified live-revision CAS or connectivity commit
  loop is supplied by this remediation. [Store](../../internal/backup/journal_store.go).

## 4. Remaining SOFTWARE blockers

All ten workflows still have C-class work, as detailed above. In particular,
production startup wiring, public methods, durable transaction ownership,
per-object policy loading/enforcement, executable restore/update recovery and
real WG/Xray/Anycast/ZTP providers cannot be classified as deployment-only tests.
No missing architecture was filled in during this verification pass.

## 5. Acceptable typed-unavailable B

A truly unknown/unsupported optional kind, absent radio capability, or missing
deployment credential can legitimately be unavailable when the response explains
why and exposes no executable action. That is not the situation for hard-coded
placeholders covering core firewall rules/sets, addresses/leases, DNS/DHCP,
WireGuard and Xray objects. Their `unsupported_kind` responses still represent
core V1 software blockers. The remaining BGP/OSPF/ISIS/route-map placeholders also
cannot be declared complete merely from an unsupported response. No reduced V1
scope was assumed. Anycast/ZTP ready fallbacks are not honest B capabilities.

## 6. Hardware/credential/network A

Real NIC/kernel/daemon integration, BGP-session advertisement observation,
WireGuard/Xray client interoperability, deployment credentials and actual
reboot/power-loss tests remain A only after the software path exists. Lack of a
real BGP session does not explain a missing Anycast executor. Lack of a production
key does not explain ZTP's missing token authentication or durable replay rules.

## 7–13. Verdicts

| Contract | Verdict |
|---|---|
| Safe Apply | Existing deadline regression preserved; new config receipt/admission defects fixed locally; end-to-end product contract FAIL |
| API | FAIL — nil public Config, incomplete domain/provider paths |
| CLI | FAIL — network route migration is real but does not make apply executable; other normal-admin IPC paths remain |
| RBAC | FAIL — deny tests pass; production policy and mutation/export enforcement incomplete |
| Update/recovery | FAIL — helper integration improved, production durable recovery absent |
| Backup/restore | FAIL — standalone persistence is not wired into restore execution |
| UI readiness | FAIL for a frozen full V1 backend |

## Targeted tests and evidence

- Target HEAD plus preserved formatting: targeted Engine, API, Agent, listers,
  identity, QoS, WG, Xray, upgrade, backup, ZTP, Safe Apply, clientexport, CLI and
  Agent-command tests passed. No full repository test repeat.
- New no-invented-receipt and QoS cancellation/truth tests failed before fixes;
  those and caller-ID/receipt rejection regressions pass after local fixes.
- Race checks passed: Engine, API, QoS, upgrade, backup, ZTP, WG, Xray, CLI;
  selected Policy/Verification/Audit/ObjectKey identity security tests.
- Targeted go vet and Linux amd64 command build passed. These are compile/unit
  results, not evidence that unwired providers execute on Linux.
- Existing config authorization, session, Safe Apply deadline, secret-export,
  incomplete-package/corrupted-release and unsafe-HTTP regressions passed.
- git diff --check passed; original formatting changes preserved. No UI or
  duck-dev-01 access. Local fixes/report remain uncommitted.

BACKEND VERIFICATION: FAIL

SAFE TO FREEZE BACKEND: NO

SAFE TO START UI IMPLEMENTATION: NO
