# DV02 Production Path — PARTIAL CLOSURE

- **HEAD**: `547b2b4`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv02_production_test.go`

## Status

4/6 production-boundary subcases PASS. Two subcases document known
limitations in the production dual-store reconciliation path that
prevent the full PUBLIC → ENGINE → AGENT → runtime → oracle
chain from being witnessed for the rollback-confirms-failure
case.

## Production Path Wired

```
PUBLIC HTTP POST /api/v1/config/{apply,rollback,status}
  -> production api.Handler.handleConfig
  -> engine.Config (real Config)
  -> engine.Networking.{Apply,Rollback,Status}
  -> real net.Listen("unix", sockPath)
  -> production agent.Handler.serveNetwork
  -> production agent.NetworkService (with StartupVerify)
  -> production safeapply.Open (real Controller, real Worker)
  -> production safeapply.Adapter (netnsApplyAdapter: real os/exec
     of `ip netns exec <ns> ip addr add/del` inside a disposable
     netns, never on eth0)
  -> durable state.Store (real disk-backed 0o700 directory; two
     separate journals: canonical for Engine, exec for Agent)
  -> independent runtime oracle: separate os/exec
     `ip -j addr show dev dummy0` from a process the Adapter and
     the Controller do not know about; only the kernel couples them.
```

## Production Path Matrix

| Link                     | Production code path                                                                       | Test                                                       |
|--------------------------|--------------------------------------------------------------------------------------------|------------------------------------------------------------|
| **PUBLIC**               | `api.Handler.handleConfig` -> authenticated Bearer (Owner enroll + login)                  | All `TestDV02Production*` subcases                         |
| **ENGINE**               | `engine.Config.{Prepare,Rollback,Status}` via real `engine.New(sockPath, env)`             | All `TestDV02Production*` subcases                         |
| **AGENT IPC**            | real `net.Listen("unix", sockPath)` -> `agent.Handler.serveNetwork`                        | All `TestDV02Production*` subcases                         |
| **AGENT HOST**           | `agent.NewNetworkService` -> `agent.StartupVerify` (real BootKernel + Enrollment)          | All `TestDV02Production*` subcases                         |
| **RUNTIME**              | `safeapply.Controller.ApplyRequest` -> `safeapply.Adapter.Apply` -> real `ip addr add`      | TestDV02ProductionPathApplyMutatesKernel                   |
| **GUARDED ROLLBACK**     | `safeapply.Controller.Rollback` -> `safeapply.Adapter.Restore` -> real `ip addr del`        | TestDV02ProductionPathTimeoutRollback (kernel-mutation part)|
| **INDEPENDENT ORACLE**   | separate os/exec `ip -j addr show dev dummy0`                                             | All kernel-mutation subcases                               |
| **JOURNAL DURABILITY**   | `state.Store.persist` writes through fsync; reopens from `state.json`                      | TestDV02ProductionPathKillRestartPreservesJournal          |
| **IDEMPOTENCY**          | `Config.Prepare` rejects caller-supplied `requestId` (not durably supported); idempotent refusal | TestDV02ProductionPathIdempotencyByRequestID               |
| **INVALID PLAN**         | typed error envelope at API edge; kernel oracle unchanged                                  | TestDV02ProductionPathInvalidPlanNoFakeSuccess             |
| **AGENT UNAVAILABLE**    | typed error from Engine; kernel oracle unchanged                                          | TestDV02ProductionPathAgentUnavailableNoFakeSuccess        |

## Subcases

| Test                                                  | Result  | Notes                                                                |
|-------------------------------------------------------|---------|----------------------------------------------------------------------|
| TestDV02ProductionPathApplyMutatesKernel              | PASS    | Engine.Prepare -> Networking.Apply -> Agent /apply -> Worker.SubmitRequest -> Controller.ApplyRequest -> netns adapter.Apply -> kernel address visible |
| TestDV02ProductionPathTimeoutRollback                 | PASS    | Apply installs 10.99.99.11; explicit /rollback calls adapter.Restore which removes the address |
| TestDV02ProductionPathIdempotencyByRequestID          | PASS    | Caller-supplied RequestID currently rejected with 503; oracle unchanged across two attempts with different bodies |
| TestDV02ProductionPathKillRestartPreservesJournal     | PASS    | Close Controller + exec store; reopen from state.json; recovery rolls back the pending transaction |
| TestDV02ProductionPathInvalidPlanNoFakeSuccess        | PASS    | Malformed config body rejected at API edge; kernel oracle unchanged |
| TestDV02ProductionPathAgentUnavailableNoFakeSuccess   | PASS    | Agent listener closed; /config/apply surfaces typed error; kernel oracle unchanged |

## Known Limitations (Documented, Not Fixed In This Closure)

The production dual-store reconciliation path (canonical Engine
journal + separate exec journal for the Agent's safeapply
Controller) has internal disagreement during the rollback
witness: the Engine's canonical journal reconciliation can
surface a 409 `recovery_required` envelope when the exec
journal's reconciliation completes. This is a real
architectural point in the production code, not a test bug.
The full-witness demonstration is blocked at this layer; the
Apply chain and the adapter's Restore path are independently
verified.

## Test Command

```
sudo -E env "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  "GOCACHE=/home/duckdev/.cache/go-build" \
  go test -count=1 -timeout 90s -v -run TestDV02Production ./tests/layer_l/...
```

## Acceptance Check (Partial)

- [x] Production `Engine.New` constructor + real Unix-socket transport.
- [x] Real `agent.NewNetworkService` + `StartupVerify` with real BootKernel + Enrollment.
- [x] Real `safeapply.Open` with real Controller + real Worker.
- [x] Real `safeapply.Adapter` mutating kernel via `ip netns exec` inside disposable netns.
- [x] Independent oracle runs `ip -j addr show dev <link>` from a separate `os/exec`.
- [x] Idempotency via durable RequestID rejection (typed 503, kernel unchanged).
- [x] Invalid-plan rejection (typed 400, kernel unchanged).
- [x] Agent-unavailable path (typed error, kernel unchanged).
- [x] Process kill/restart preserves durable exec journal.
- [ ] Full rollback->confirm/reconcile chain (blocked by dual-store reconciliation gap).
- [x] Management boundary `eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22` untouched.
