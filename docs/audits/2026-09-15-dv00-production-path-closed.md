# DV00 Production Path — CLOSED

- **HEAD**: `fb2507e`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv00_production_test.go`

## Goal

The L-tier DV00 subcases (`TestDV00ProductionMutationOracle`,
`TestDV00NoFakeSuccess`) previously exercised only `ip addr add`
against a kernel fixture. There was no PUBLIC → API → ENGINE →
AGENT → runtime chain for QoS mutations, and no independent
runtime oracle: a successful `/api/v1/qos/apply` could report
"applied" without producing a real `tc` kernel effect, and a
failed apply could leak a partial kernel state without the
controller's guarded rollback path being witnessed.

This closure wires:

  PUBLIC HTTP request body
  → `engine.New(sockPath, env).QoSApply / .QoSRollback` — the
    same constructor `cmd/duck-api` uses
  → real `net.Listen("unix", sockPath)` on the Agent
  → production `agent.Handler` → `ServeQoSApply / ServeQoSRollback`
  → production `agent.QoSHost` → `qos.Controller`
  → production `qos.LinuxRunner` with the production
    `ExecRunner` shape (real `/sbin/tc` via `ip netns exec <ns>`
    so the mutation lands inside the disposable netns and never
    on `eth0`)
  → kernel `qdisc` subsystem on `dummy0` inside `<ns>`
  → independent oracle: separate `os/exec` invocation of
    `/sbin/ip netns exec <ns> /sbin/tc -s qdisc show dev dummy0`
    — the runner does not know the oracle process exists;
    only the kernel couples them.

The fixture uses production constructors end-to-end. The only
thing that differs from `cmd/duck-agent`'s `enableQoSService` is
the `Executor` implementation: the production code uses
`qos.NewExecRunner("/sbin/tc")` (which runs `tc` in the calling
process's netns); the test substitutes a `netnsExecutor` that
prefixes `ip netns exec <ns>` to every argv so the kernel
mutation lands in the disposable netns. Every other layer
(Controller, QoSHost, Handler, Engine, Unix socket transport,
ApplyOutcome envelope, guarded rollback) is the production
constructor with no stubs.

## Production Path Matrix

| Link                  | Production code path                                                                                          | Test                                                |
|-----------------------|---------------------------------------------------------------------------------------------------------------|-----------------------------------------------------|
| **PUBLIC**            | `api.Handler.handleQoSApply` → 32 KiB cap → `qos.Plan.Valid()` → `engine.Engine.QoSApply` (typed-error envelope) | TestDV00ProductionPathRealMutation, TestDV00NegativeInvalidPlan, TestDV00AgentUnavailableDoesNotFakeSuccess, TestDV00NonexistentLinkNoFakeSuccess |
| **ENGINE**            | `engine.Engine.QoSApply` (HTTP POST → `http://agent/agent/v1/qos/apply`; 32 KiB cap; Unix-socket Transport built by `engine.New(sockPath, env)`) | All `TestDV00ProductionPath*` and negative subcases |
| **AGENT IPC**         | `agent.Handler.ServeHTTP` → `/agent/v1/qos/apply` → `ServeQoSApply` (REAL `net.Listen("unix", sockPath)`, not httptest) | All production-path tests                           |
| **AGENT HOST**        | `agent.NewQoSHost` (production host; nil host returns 503 `qos_unavailable`)                                  | TestDV00AgentUnavailableDoesNotFakeSuccess          |
| **RUNTIME**           | `qos.Controller.Apply` → `qos.LinuxRunner.Apply` (real `/sbin/tc` via `ip netns exec <ns> tc qdisc add dev <link> parent root handle 1: pfifo limit 1000`) | TestDV00ProductionPathRealMutation, TestDV00ProductionPathRollback |
| **GUARDED ROLLBACK**  | `qos.LinuxRunner.Rollback` (Step 1: `tc qdisc del dev <link> root` for every root qdisc in current; Step 2: re-apply previous) | TestDV00ProductionPathRollback                      |
| **INDEPENDENT ORACLE**| independent `os/exec /sbin/ip netns exec <ns> /sbin/tc -s qdisc show dev <link>` (separate OS process; not the LinuxRunner's own bookkeeping) | All `TestDV00ProductionPath*` subcases             |
| **NEGATIVE (no fake success)** | Engine refuses `schemaVersion="0"` plan → typed error envelope; Engine surfaces `Agent is unavailable` when listener closed (no fake ApplyOutcome); LinuxRunner refuses `nosuch0` link → Controller returns error | TestDV00NegativeInvalidPlan, TestDV00AgentUnavailableDoesNotFakeSuccess, TestDV00NonexistentLinkNoFakeSuccess |
| **NETNS BOUNDARY**    | `ip netns add duckos-dv00-<suffix>`; dummy0 moved into the netns; agent's `tc` runs via `ip netns exec` so it never touches `eth0`/`172.18.4.149`/default route/SSH :22 | All `TestDV00ProductionPath*` subcases             |

## Subcases (7/7 PASS)

| Test                                              | Production-path evidence                                                                                                                                  |
|---------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| TestDV00ProductionMutationOracle                  | Baseline (pre-existing L-tier): kernel `ip addr add` round-trip against a real `dummy0` in a disposable netns.                                            |
| TestDV00NoFakeSuccess                             | Baseline (pre-existing L-tier): mutation against a non-existent interface MUST error — no fake success.                                                  |
| TestDV00ProductionPathRealMutation                | FULL chain: `engine.New(sockPath, env).QoSApply` → Unix-socket HTTP → `agent.Handler.ServeQoSApply` → `QoSHost.Apply` → `Controller.Apply` → `LinuxRunner.Apply` → `ip netns exec <ns> tc qdisc add dev dummy0 parent root handle 1: pfifo limit 1000` → kernel qdisc. Independent `tc -s qdisc show` oracle observes `pfifo` after, no `pfifo` before. `ApplyOutcome.Applied == 1`. |
| TestDV00ProductionPathRollback                    | Apply installs `pfifo` on `dummy0`; oracle observes it; `engine.Engine.QoSRollback` triggers `Controller.Rollback` → `LinuxRunner.Rollback` → `tc qdisc del dev dummy0 root`; oracle observes no `pfifo` after rollback. |
| TestDV00NegativeInvalidPlan                       | Plan with `schemaVersion="0"` is rejected by `qos.Plan.Valid()` before any `tc` invocation; oracle observes identical kernel state before and after.    |
| TestDV00AgentUnavailableDoesNotFakeSuccess        | Listener closed; `engine.Engine.QoSApply` returns `Agent is unavailable` typed error (NOT a fake `ApplyOutcome.Applied=1`); `ApplyOutcome.Applied == 0`; oracle observes identical kernel state. |
| TestDV00NonexistentLinkNoFakeSuccess              | Plan with `Link="nosuch0"` is forwarded by Engine to Agent, accepted by `Plan.Valid()`, fails inside `LinuxRunner.Apply` (`tc qdisc add dev nosuch0 ...` → exit 1); oracle observes identical kernel state on `dummy0`. |

## Test command

```
sudo -E env "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  "GOCACHE=/home/duckdev/.cache/go-build" \
  go test -count=1 -timeout 90s -v -run TestDV00 ./tests/layer_l/...
```

## Acceptance check

- [x] Production `Engine.New` constructor used.
- [x] Real `net.Listen("unix", sockPath)` for the Agent.
- [x] Production `agent.NewHandler` + `agent.NewQoSHost` + `qos.NewController` + `qos.NewLinuxRunner` constructors.
- [x] Mutation runs real `/sbin/tc` via `ip netns exec <ns>` inside a disposable netns.
- [x] Independent oracle runs `/sbin/tc -s qdisc show dev <link>` from a separate `os/exec`.
- [x] No grep/source-only PASS — every assertion inspects a kernel-state diff.
- [x] Negative case: invalid plan rejected, no mutation.
- [x] Agent unavailable: typed error, no fake `ApplyOutcome`, no kernel mutation.
- [x] Nonexistent link: Runner fails, no kernel mutation on the real `dummy0`.
- [x] Rollback path: kernel state reverts to pre-apply.
- [x] Management boundary `eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22` untouched.
- [x] All layer_l tests pass: `ok github.com/CrazyDuckLab/DuckOS/tests/layer_l 27.544s`.
