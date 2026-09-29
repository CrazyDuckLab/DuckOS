# Targeted backend verification — 2ceaefc

## 1. Verified HEAD and scope

`V1 UI qualification branch`, `2ceaefcf87317277a2b4bffc2eeb611c8c9f4265`.
Reviewed only `a8036608fb692a2e32b707908d52aaddf69f65fa..2ceaefc`
and surrounding code needed to verify those changes and preservation of the
[previous audit](2026-09-12-backend-a55e4e4.md) fixes. No repository-wide re-audit,
UI work, remote host access, new features, commit or push. Local fixes below are
uncommitted changes on the verified HEAD, not part of that commit.

## 2. Previous findings: CLOSED / OPEN / REGRESSED

| Finding | Targeted result |
|---|---|
| H01 config auth/secret DTO | CLOSED for the earlier defect; regression passes |
| H02 journal-only configuration success | Containment preserved; real public transaction integration OPEN |
| H03 Safe Apply late confirmation | CLOSED for the deadline defect; regression passes |
| H04 privileged QoS | Safe unavailable gate preserved; guarded execution OPEN |
| H05 unsafe CLI HTTP/redirects | Earlier fix preserved; private paths no longer use public HTTP; parity OPEN |
| H06 corrupted/incomplete release activation | CLOSED for the earlier storage defect; regressions pass |
| H07 package panic / signed architecture / missing health | Earlier fixes preserved; new recovery helper defect below |
| H08 scoped provisioning / H09 session continuity | Earlier regression tests pass; no changes to those implementations |
| M01, explicitly included in remediation scope | Grammar corrected; wildcard semantics REGRESSED, fixed locally; full per-object enforcement OPEN |
| H10 API/CLI/runtime contract | OPEN |
| H11 update recovery / portable restore | OPEN; helper additions do not close production recovery |

## 3. New Critical/High and local fixes

No Critical exploit was demonstrated. New High-risk remediation defects are
qualified by reachability: RBAC overlay and restore/recovery helpers are not
connected as full production public workflows.

- **Wildcard authorization bypass.** A grant for `kind:*` was accepted but
  `AuthorizeObject` looked up only the exact ID. A concrete object could bypass
  a kind-wide deny. Reproduced by a failing test. Fixed by reading exact and
  wildcard grants under one lock, with any applicable deny taking precedence.
  Tests cover both wildcard deny and specific deny overriding wildcard allow.
  [Policy](../../internal/identity/policy.go).
- **Unsafe restore admission and cancellation recovery.** `SafeToApply=false`
  still admitted additions; lexicographic comparison admitted different opaque
  revisions; missing revisions were accepted. Rollback reused a canceled request
  context and could not restore partially changed state. All reproduced before
  fixes. Fixed refusal of unsafe plans, nonempty exact revision matching,
  entry/plan backup-ID binding, pre-canceled request refusal and independent
  bounded rollback context. Recheck request cancellation before/after apply.
  This is containment, not an atomic live-revision CAS or real restore verifier.
  [Restore lifecycle](../../internal/backup/restore_lifecycle.go).
- **Recovery health helper failure handling.** A nil runner could panic in its
  goroutine; already canceled work still started and cancellation could race with
  successful completion. Added nil/cancellation gates and regressions. The runner
  interface still cannot actually cancel running work, and the helper is not
  used by ActivateAndVerify. [Recovery helper](../../internal/upgrade/recovery.go).

## 4. Remaining SOFTWARE blockers

- **RBAC:** accepting dotted kinds and `*` is not per-row authorization. The
  objects handler checks `kind:*`, not each returned object; a specific deny is
  not a demonstrated collection filter. Policy startup/provider/export wiring
  remains incomplete. [Handler](../../internal/api/objects.go).
- **CLI/Engine boundary:** `callAgent` bypasses API/Engine and sends the bearer
  token to IPC, but Agent authenticates OS peer UID, not that session's role.
  Ordinary login users are rejected by the real socket; running as the trusted
  API service UID does not recreate user RBAC or Engine mediation. No remote
  privilege escalation is claimed. Missing public methods cannot be repaired
  by direct socket transport. The socket also defaults automatically; it is not
  actually an explicitly required flag as the remediation report claims.
  [CLI](../../cmd/duck/main.go), [peer gate](../../internal/agent/socket_linux.go).
- **Updates:** VerifyCandidate, LoadState and CancellableHealthCheck have no
  non-test integration call sites in the verified tree. LoadState verifies
  referenced payloads, not reconciliation with actual Active/Previous anchors or
  a durable transaction. VerifyFingerprint is a digest check, not a new signature
  check. ActivateAndVerify still uses the old Executor. Source/schema compatibility,
  durable restart recovery and cancellation of running probes remain unresolved.
  Corrupt-release refusal is supplied by the earlier storage fix, not proof that
  the new recovery path is wired. [Orchestrator](../../internal/upgrade/orchestrator.go).
- **Restore:** RestoreJournal is an in-memory map, not durable restart evidence;
  updates replace entries rather than retaining every transition. ExecuteRestore
  has no production caller, no serialized live-state CAS, no verified archive/
  secret capability or connectivity confirmation phase. It marks Applied on a nil
  callback error. A callback-only test is not proof of real restore. Secret access
  cannot be inferred from the presence of a revision string. Parallel restore
  operations and post-crash rollback remain unqualified.
- **Public object/runtime wiring:** missing real domain listers, generic config
  coordinator, guarded QoS and WireGuard/Xray providers remain software work.

## 5–6. Blocker classification and acceptable typed-unavailable capabilities

A = hardware/credential/network-only; B = truthful typed-unavailable capability
acceptable for an otherwise implemented contract; C = software freeze blocker.

| Claimed blocker | Classification |
|---|---|
| Real per-domain listers | **C**. Returning unsupported for all required domains is missing software, not missing hardware |
| Generic config against Linux | **C** for missing public coordinator; **A** only for final real-kernel verification |
| QoS against tc | **C** for missing guarded executor; its safe unavailable gate is valid containment, not implementation; **A** for final live tc checks |
| WireGuard/Xray CRUD/export runtime | **C** for missing providers/CRUD/export wiring; **A** for credentials and live interoperability checks after wiring |
| Anycast BGP advertisement | **C** for absent execution/observation integration; the inspected evidence store alone does not advertise routes. A real BGP session test is **A**, but is not the only missing part |
| ZTP production key | Missing supplied deployment secret can be **A**, and `ErrNoProductionKey` is a valid **B** gate. Key lifecycle/bootstrap wiring and durable redemption/replay protection remain **C**; supplying random bytes is not the complete solution |
| Hardware/credential/network tests | **A** when testing implemented operations: kernel/NIC isolation, restart, real daemon/BGP behavior and external credentials |

**B is acceptable** for unavailable radio hardware, absent deployment credentials,
or an explicitly unsupported optional operation on a node, provided the public
contract reports the reason and suppresses unsupported actions. It does not
reclassify unfinished core workflows as ready. The current generic unsupported
collections and absent mutation/provider paths do not establish a V1 UI freeze.

## 7–12. Verdicts

| Contract | Verdict |
|---|---|
| Safe Apply | Earlier deadline fix preserved; public end-to-end contract **FAIL** |
| API | **FAIL** — real collections, mutations and per-object enforcement incomplete |
| CLI | Private HTTP exclusion passes; API/Engine/RBAC parity **FAIL** |
| Update/recovery | Corrupted candidate gate passes; durable integrated recovery **FAIL** |
| Backup/restore | Local validation/rollback defects fixed; verified durable lifecycle **FAIL** |
| UI readiness | **FAIL** for implementation against a frozen full V1 backend |

## Targeted verification evidence

- Before local fixes: targeted tests for Duck CLI/Agent, API, identity, upgrade,
  backup, Safe Apply, Engine, provisioning, client exports and acceptance passed.
  New wildcard/restore security regressions then failed and reproduced the defects.
- After local fixes: race tests passed for API, upgrade, backup and Duck CLI;
  identity race checks covered Policy/ObjectKey/Audit/Verification tests.
- Prior config authorization/secret projection, session handling, Safe Apply,
  short-package, corrupted-release and QoS fail-closed regressions passed.
- New CLI regression verifies private paths never reach public HTTP and Agent
  redirects remain refused. Unix mock tests do not prove Linux peer-UID parity.
- Targeted `go vet` passed; Linux amd64 command cross-build passed. Backup was
  rerun after its final cancellation/ID checks. No full repository suite repeated.
- `git diff --check` passed. No Linux runtime/hardware test was executed.

BACKEND VERIFICATION: FAIL

SAFE TO FREEZE BACKEND: NO

SAFE TO START UI IMPLEMENTATION: NO
