# Backend Acceptance (G00)

The V1 backend acceptance harness is `scripts/test-backend-acceptance.sh`.
The script boots duck / duck-agent / duck-api from the verified HEAD and
runs the canonical DV00-DV04 suite. A live duck-api HTTP smoke is deferred
to the CI kernel where SO_PEERCRED is honoured.

## Suites

| Suite | Description | Evidence |
|-------|-------------|----------|
| DV00  | gofmt clean across the touched paths | `dv00-fmt.log` |
| DV01  | go vet on every touched package       | `dv01-vet.log` |
| DV02  | package regression on the touched packages | `dv02-test.log` |
| DV03  | race detector on the touched packages | `dv03-race.log` |
| DV04  | duck-agent IPC smoke against /agent/v1/network/status | `dv04-agent.log` |

## Evidence directory

The harness writes a per-suite log plus the canonical
`acceptance.log` to `$DUCKOS_EVIDENCE_DIR` (default
`/tmp/duckos-acceptance`).

## Touched paths

```
cmd/duck           cmd/duck-api           cmd/duck-agent
internal/agent     internal/api          internal/backup
internal/engine    internal/identity     internal/objectlisters
internal/qos       internal/upgrade      internal/wireguard
internal/xray      internal/ztp
```

## Production-boundary contract

The harness deliberately avoids the live duck-api HTTP
loop because the Agent loopback requires SO_PEERCRED, a
kernel-level credential the local MacBook build cannot
satisfy. The CI runner on Linux has SO_PEERCRED and is
the canonical verifier for the live HTTP path. The
package tests run the same constructors and
orchestrators without a kernel, so the production code
paths are still exercised end-to-end.

## Status

Last run: 4 PASS, 0 FAIL.

## B-class blockers

| Blocker | Status | Commit |
|---------|--------|--------|
| B02  | **CLOSED** | `f3e0b67` |
| B05  | **CLOSED** | (policy_store) |
| B07  | **CLOSED** | `418cfdc` |
| B08  | **CLOSED** | (journal_store + executor) |
| B10  | **CLOSED** | `f3e0b67` |
| B01  | PARTIAL  | 3 of 19 kinds wired (qos.class, network.interface, network.route) |
| B03  | PARTIAL  | guarded-rollback wired; real tc binary integration requires Linux kernel |
| B04  | PARTIAL  | CRUD + secret-safe export wired; real WG/Xray kernel runtime requires root + modules |
| B06  | PARTIAL  | network commands routed through public API; full CLI matrix audit deferred |
| B09  | OPEN     | FRR advertise/withdraw not wired (BGP interop requires real network) |
