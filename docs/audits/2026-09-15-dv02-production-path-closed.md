# DV02 Production Path — CLOSED

- **HEAD**: `f06913b`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv02_production_test.go`

## Goal

Production-boundary witness for the Config/Safe Apply chain
with the *strict* backend contract: durable caller-supplied
RequestID idempotency (not "any RequestID → 503"),
isolated Linux runtime, real Engine/Agent/process/Unix-socket,
independent runtime oracle, kernel mutation visible to a
separate process, process-kill/restart durability, and
honest negative/no-fake-success.

Two production defects were fixed during the closure:
1. `Config.Prepare` previously rejected every caller-supplied
   RequestID with 503 `config_runtime_unavailable`. Per the
   original backend contract the RequestID must be durably
   admitted. The fix: Prepare passes the RequestID through
   to `Networking.Apply`, which calls `store.PrepareRequest`
   with the caller's ID.
2. `state.Store.PrepareRequest` previously rejected any
   repeated RequestID with `ErrConflict`. Per the contract
   the same RequestID + same candidate must be idempotent
   (no duplicate kernel mutation, prior outcome returned).
   The fix: match against in-flight Pending first, then
   match against durable Outcomes — same body returns a
   synthetic post-Apply transaction; conflicting body
   returns ErrConflict.
3. `Engine.Networking.control` previously returned as soon
   as the Agent's control handler accepted the request. The
   Agent runs the actual rollback/confirm in a goroutine;
   the canonical journal must observe a durable outcome
   before the public API returns, otherwise the
   `outcomeFromState` envelope carries no Receipt and
   surfaces `recovery_required`. The fix: poll the Agent's
   status until the canonical Pending record is reconciled.

## Production Path Wired

```
PUBLIC HTTP POST /api/v1/config/{apply,rollback,status}
  -> production api.Handler.handleConfig
  -> engine.Config.{Prepare,Rollback,Status}
  -> engine.Networking.{Apply,Rollback,Status}
     (Prepare passes RequestID; Apply calls PrepareRequest
      with caller identity; control polls reconciliation)
  -> real net.Listen("unix", sockPath)
  -> production agent.Handler.serveNetwork
  -> production agent.NetworkService (with StartupVerify)
  -> production safeapply.Open (real Controller, real Worker)
  -> production safeapply.Adapter (netnsApplyAdapter:
     real `ip netns exec <ns> ip addr add/del` inside a
     disposable netns, never on eth0)
  -> durable state.Store (real disk-backed 0o700 directory;
     two separate journals — canonical for Engine, exec
     for Agent — both use the same TransactionID so the
     idempotency contract is honoured on both sides)
  -> independent runtime oracle: separate os/exec
     `ip -j addr show dev dummy0` from a process the
     Adapter and the Controller do not know about
```

## Production Path Matrix

| Link                     | Production code path                                                                      | Test                                                  |
|--------------------------|-------------------------------------------------------------------------------------------|-------------------------------------------------------|
| **PUBLIC**               | `api.Handler.handleConfig` -> authenticated Bearer (Owner enroll + login)                 | All `TestDV02Production*` subcases                    |
| **ENGINE**               | `engine.Config.{Prepare,Rollback,Status}` via real `engine.New(sockPath, env)`            | All `TestDV02Production*` subcases                    |
| **REQUEST ID ADMISSION** | `engine.Config.Prepare` -> `engine.Networking.Apply` -> `state.Store.PrepareRequest(X, ...)` with caller-supplied `X`; in-flight Pending match + Outcomes match for idempotent dedup | `TestDV02ProductionPathIdempotencyByRequestID`       |
| **AGENT IPC**            | real `net.Listen("unix", sockPath)` -> `agent.Handler.serveNetwork`                       | All `TestDV02Production*` subcases                    |
| **AGENT HOST**           | `agent.NewNetworkService` -> `agent.StartupVerify` (real BootKernel + Enrollment)         | All `TestDV02Production*` subcases                    |
| **RUNTIME**              | `safeapply.Controller.ApplyRequest` -> `safeapply.Adapter.Apply` -> real `ip addr add`   | `TestDV02ProductionPathApplyMutatesKernel`            |
| **GUARDED ROLLBACK**     | `safeapply.Controller.Rollback` -> `safeapply.Adapter.Restore` -> real `ip addr del`       | `TestDV02ProductionPathTimeoutRollback`               |
| **CANONICAL RECONCILE**  | `Engine.Networking.control` polls Agent status until Pending is reconciled (commit or rollback outcome) | `TestDV02ProductionPathApplyMutatesKernel`, `TestDV02ProductionPathTimeoutRollback`, `TestDV02ProductionPathKillRestartRequestIDIdempotent` |
| **INDEPENDENT ORACLE**   | separate os/exec `ip -j addr show dev dummy0`                                             | All kernel-mutation subcases                          |
| **JOURNAL DURABILITY**   | `state.Store.persist` writes through fsync; reopens from `state.json`                     | `TestDV02ProductionPathKillRestartPreservesJournal`  |
| **RESTART-SAFE IDEMPOTENCY** | Caller-supplied RequestID dedup survives process restart via durable canonical Outcomes | `TestDV02ProductionPathKillRestartRequestIDIdempotent` |
| **INVALID PLAN**         | typed error envelope at API edge; kernel oracle unchanged                                 | `TestDV02ProductionPathInvalidPlanNoFakeSuccess`      |
| **AGENT UNAVAILABLE**    | typed error from Engine; kernel oracle unchanged                                         | `TestDV02ProductionPathAgentUnavailableNoFakeSuccess` |

## Subcases (7/7 PASS)

| Test                                                       | Result | Notes                                                                                                                            |
|------------------------------------------------------------|--------|----------------------------------------------------------------------------------------------------------------------------------|
| TestDV02ProductionPathApplyMutatesKernel                   | PASS   | Full chain: Engine.Prepare -> Networking.Apply -> Agent /apply -> safeapply.Worker.SubmitRequest -> Controller.ApplyRequest -> netns adapter -> kernel address visible to oracle; canonical Pending recorded; rollback path restores the kernel via adapter.Restore |
| TestDV02ProductionPathTimeoutRollback                      | PASS   | Apply then explicit rollback through public API; adapter.Restore removes the kernel address; oracle observes the address gone; canonical reconciled |
| TestDV02ProductionPathIdempotencyByRequestID               | PASS   | (1) First apply with RequestID=X, body=B1 — canonical records Pending with X. (2) Idempotent retry with same X, same B1 — canonical returns prior outcome without re-running the adapter. (3) Conflicting retry with same X, different B2 — rejected with 409 conflict (no silent re-use) |
| TestDV02ProductionPathKillRestartRequestIDIdempotent        | PASS   | Apply + confirm + process kill (api server + handler + listener + controller + stores closed). Re-open canonical + exec + identity + listener + engine from the durable journal. Re-login. Re-issue the same RequestID with the same body. Canonical Outcomes return the prior outcome idempotently — survives restart |
| TestDV02ProductionPathKillRestartPreservesJournal          | PASS   | Close controller + exec store. Re-open from the durable exec journal. safeapply.Open's recovery rolls back the pending transaction; a rolled-back outcome for the pending ID is present in the exec journal |
| TestDV02ProductionPathInvalidPlanNoFakeSuccess             | PASS   | Malformed config body (bad role/name) rejected at the API edge with a typed error envelope; kernel oracle unchanged                  |
| TestDV02ProductionPathAgentUnavailableNoFakeSuccess        | PASS   | Agent listener closed; /config/apply surfaces a typed error; kernel oracle unchanged                                                  |

## Test Command

```
sudo -E env "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  "GOCACHE=/home/duckdev/.cache/go-build" \
  go test -count=1 -timeout 180s -v -run TestDV02Production ./tests/layer_l/...
```

## Acceptance Check

- [x] Production `Engine.New` constructor + real Unix-socket transport.
- [x] Production `objectlisters` boundary used for Engine wiring.
- [x] Real `agent.NewNetworkService` + `StartupVerify` with real BootKernel + Enrollment.
- [x] Real `safeapply.Open` with real Controller + real Worker.
- [x] Real `safeapply.Adapter` mutating kernel via `ip netns exec` inside disposable netns.
- [x] **Durable caller-supplied RequestID idempotency** (strict backend contract):
      - same RequestID + same body → idempotent return
      - same RequestID + different body → 409 conflict
      - survives process restart via durable canonical Outcomes
- [x] Independent oracle runs `ip -j addr show dev <link>` from a separate `os/exec`.
- [x] Engine.Networking.control synchronously reconciles canonical Pending before
      returning to the public API (the Agent's control handler is async).
- [x] Process kill/restart preserves durable canonical journal + exec journal.
- [x] Invalid-plan rejection (typed 400, kernel unchanged).
- [x] Agent-unavailable path (typed error, kernel unchanged).
- [x] Management boundary `eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22` untouched.

## Production Defects Fixed During Closure

| Defect                                                                  | Fix                                                                                          |
|-------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| `Config.Prepare` returned 503 for every caller-supplied RequestID       | `Config.Prepare` now passes RequestID through to `Networking.Apply`                           |
| `state.Store.PrepareRequest` rejected every repeated RequestID         | Idempotency loop: Pending match (same body → idempotent, different → 409); Outcome match (committed → idempotent synthetic, different → 409) |
| `Engine.Networking.control` returned before the Agent's control goroutine wrote the durable outcome, causing outcomeFromState to surface recovery_required | Polling loop reconciles canonical Pending before returning (re-reads snap.Pending each iteration; only exits when Pending cleared) |
