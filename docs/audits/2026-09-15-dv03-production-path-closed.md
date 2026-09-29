# DV03 Production Path — CLOSED

- **HEAD**: `fcb7929`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv03_production_test.go`

## Goal

Production-boundary witness for the QoS apply/rollback chain
with the *strict* backend contract: PUBLIC → Engine → Agent →
qos.Controller → real `/sbin/tc` binary → kernel qdisc
subsystem, observed by an independent oracle in a separate
process. PATH/tool resolution uses the production
`/sbin/tc` (or `/usr/sbin/tc` fallback). No direct-tc-only
tests as the sole PASS witness.

## Production Path Wired

```
PUBLIC HTTP POST /api/v1/qos/{apply,rollback,status}
  -> production api.Handler.handleQoSApply / handleQoSRollback
  -> engine.Engine.QoSApply / QoSRollback
     (real Engine, real Unix-socket Transport built by
      engine.New(sockPath, env))
  -> real net.Listen("unix", sockPath)
  -> production agent.Handler.serveQoSApply
  -> production qos.QoSHost
  -> production qos.NewController
  -> production qos.LinuxRunner
     (real os/exec of /sbin/tc via
      `ip netns exec <ns> tc qdisc add/replace/del`
      inside a disposable netns, never on eth0)
  -> kernel qdisc subsystem on dummy0 inside <ns>
  -> independent runtime oracle: separate os/exec
     `ip netns exec <ns> tc -s qdisc show dev <link>` from a
     process the Runner does not know about
```

## Production Path Matrix

| Link                     | Production code path                                                                       | Test                                                       |
|--------------------------|--------------------------------------------------------------------------------------------|------------------------------------------------------------|
| **PUBLIC**               | `api.Handler.handleQoSApply` -> authenticated Bearer (Owner enroll + login)                  | All `TestDV03Production*` subcases                          |
| **ENGINE**               | `engine.Engine.QoSApply / QoSRollback` via real `engine.New(sockPath, env)`               | All `TestDV03Production*` subcases                          |
| **AGENT IPC**            | real `net.Listen("unix", sockPath)` -> `agent.Handler.serveQoSApply`                       | All `TestDV03Production*` subcases                          |
| **AGENT HOST**           | `agent.NewQoSHost(factory)` -> `qos.NewController` -> `qos.LinuxRunner`                   | All `TestDV03Production*` subcases                          |
| **RUNTIME**              | `qos.LinuxRunner.Apply` -> real os/exec of `/sbin/tc` via `ip netns exec`                  | `TestDV03ProductionPathApplyMutatesKernel`                  |
| **GUARDED ROLLBACK**     | `qos.LinuxRunner.Rollback` -> real os/exec of `tc qdisc del`                              | `TestDV03ProductionPathRollbackRestoresKernel`              |
| **INDEPENDENT ORACLE**   | separate os/exec `tc -s qdisc show dev <link>`                                             | All kernel-mutation subcases                                |
| **INVALID PLAN**         | typed error envelope at API edge; kernel oracle unchanged                                  | `TestDV03ProductionPathInvalidPlanNoFakeSuccess`            |
| **AGENT UNAVAILABLE**    | typed error from Engine; kernel oracle unchanged                                          | `TestDV03ProductionPathAgentUnavailableNoFakeSuccess`       |
| **RUNNER FAILURE**       | failing Runner (every Apply/Rollback returns error) -> typed 5xx envelope, never fake 200 | `TestDV03ProductionPathRollbackFailureRecoveryRequired`     |

## Subcases (5/5 PASS)

| Test                                                            | Result | Notes                                                                                                                                            |
|-----------------------------------------------------------------|--------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| TestDV03ProductionPathApplyMutatesKernel                        | PASS   | Full chain: Engine.QoSApply -> Agent /qos/apply -> qos.Controller.Apply -> qos.LinuxRunner.Apply -> real `tc qdisc add dev dummy0 parent root handle 1: pfifo`; independent kernel oracle observes pfifo. A 200 with no oracle delta is a fake-success violation. |
| TestDV03ProductionPathRollbackRestoresKernel                   | PASS   | Apply then explicit rollback through /qos/rollback; LinuxRunner's per-link teardown removes the kernel qdisc; oracle observes the qdisc gone.        |
| TestDV03ProductionPathInvalidPlanNoFakeSuccess                 | PASS   | Malformed Plan body (wrong schemaVersion, wrong steps type) rejected at the API edge with a typed error envelope; kernel oracle unchanged.      |
| TestDV03ProductionPathAgentUnavailableNoFakeSuccess            | PASS   | Agent listener closed; /qos/apply surfaces a typed error; kernel oracle unchanged.                                                                  |
| TestDV03ProductionPathRollbackFailureRecoveryRequired          | PASS   | Failing Runner (every Apply/Rollback returns error) -> /qos/apply surfaces typed 5xx envelope, never fake 200; kernel oracle unchanged because Apply returned before any tc ran. |

## Test Command

```
sudo -E env "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  "GOCACHE=/home/duckdev/.cache/go-build" \
  go test -count=1 -timeout 60s -v -run TestDV03Production ./tests/layer_l/...
```

## Acceptance Check

- [x] Production `Engine.New` constructor + real Unix-socket transport.
- [x] Production `api.Handler.handleQoSApply` reaches `qos.QoSHost` through
      the production `agent.NewQoSHost` factory.
- [x] Production `qos.NewController` wired with production `qos.LinuxRunner`.
- [x] Real `/sbin/tc` binary (or `/usr/sbin/tc` fallback) via `os/exec`.
- [x] Every tc argv runs inside a disposable netns via `ip netns exec`
      so the kernel mutation lands on dummy0 and never on eth0.
- [x] Independent oracle runs `tc -s qdisc show dev <link>` from a
      separate `os/exec` process.
- [x] Rollback path exercises the real teardown (not just the API
      returning 200).
- [x] Invalid-plan rejection (typed error, kernel unchanged).
- [x] Agent-unavailable path (typed error, kernel unchanged).
- [x] Runner-failure path (typed 5xx envelope, kernel unchanged).
- [x] Management boundary `eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22` untouched.
