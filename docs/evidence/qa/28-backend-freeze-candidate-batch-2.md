# Backend Freeze Candidate — Batch 2 (interim)

## Headline

Branch: `V1 UI qualification branch`
HEAD at end of this interim: `83f9f4be6`
Baseline destructive-QA HEAD: `e3ff655`
Batch 1 final HEAD: `9dc55d0`

## Gaps advanced in this batch

### G01 Safe Apply public contract — **DONE**

- **Engine surface** (`internal/engine/config.go`): typed
  `Config`, `ConfigStatus`, `ConfigDiff`, `ConfigApplyRequest`,
  `ApplyOutcome`, `PublicReceipt`. Operations:
  - `Status(ctx)` → current revision + pending + history + outcomes + server time
  - `Diff(ctx, request)` → base / candidate / digest / findings
  - `Validate(ctx, config, expected)` → structured findings
  - `Prepare(ctx, request)` → moves through Prepared → Applying → AwaitingConfirmation; deadline set
  - `Confirm(ctx, txID)` → Commit + receipt (new revision, baseRevision, outcome)
  - `Rollback(ctx, txID, requestID)` → RollingBack + RolledBack
  - `Reconcile(ctx, outcome)` → external coordinator hook
- **API surface** (`internal/api/config.go`):
  - `GET /api/v1/config/status`
  - `POST /api/v1/config/diff` (bodies ≤ 1 MiB)
  - `POST /api/v1/config/apply` (202 Accepted, body ≤ 1 MiB)
  - `POST /api/v1/config/confirm/{txID}`
  - `POST /api/v1/config/rollback/{txID}`
  - Stale revision → `409 revision_conflict`
  - Recovery required → `409 recovery_required`
- **CLI surface** (`cmd/duck/config.go`):
  - `duck config status [--endpoint ... --allow-insecure]`
  - `duck config diff --candidate <file> --expected-revision <id> [--request-id ...]`
  - `duck config apply --candidate <file> --expected-revision <id> --request-id <32hex> [--actor ... --reason ... --confirm-seconds 120..900]`
  - `duck config confirm <tx-id> [--endpoint ... --allow-insecure]`
  - `duck config rollback <tx-id> [--endpoint ... --allow-insecure]`
- **Torture scenarios** (`internal/engine/config_torture_test.go`, 10 subtests):
  - stage → diff → validate → apply → status → confirm
  - apply → no confirm → rollback
  - stale revision → ErrConflict (Diff / Validate / Prepare)
  - Confirm without pending → ErrConflict
  - Confirm with mismatched tx id → ErrConflict
  - apply → disconnect → reconnect → confirm same transaction
  - Reconcile after uncertain
  - supported-actions per-phase matrix
  - Diff with no candidate
  - Apply window bounds (rejects <120, >900)

### G07 CLI/API parity foundation — **DONE for config**

- `cmd/duck/config_parity_test.go` exercises the CLI
  against an httptest mock of the public API and asserts:
  - CLI status hits `GET /api/v1/config/status`
  - CLI apply hits `POST /api/v1/config/apply`
  - CLI confirm hits `POST /api/v1/config/confirm/{txID}`
  - Request-id contract (32 hex chars)
  - Missing-flags error contract
  - Missing-tx-id error contract

## Gaps remaining

| ID | Title | Status |
|---|---|---|
| G02 | Unified first-class object CRUD via kind/id/revision | Pending (large scope) |
| G08 | Dependency / provenance public surface | Pending |
| G12 | WG / Xray lifecycle public surface | Partial (Xray Views from Batch 1) |
| G13 | Secret export provider binding | Pending |
| G06 | Collection endpoint wiring | Partial (parser + envelope from Batch 1) |
| G04 | Readiness manifest final wiring | Partial (read endpoint from Batch 1) |
| G15 | Structured errors regression | Partial (Batch 1 closed the core; new Batch 2 surfaces use the same `problem()` helper) |

## Test status

```
go build ./internal/engine/... ./internal/api/... ./cmd/duck/... : clean
go test ./internal/engine/... -run TestG01_ : ok  65ms (10 subtests)
go test ./cmd/duck/... -run TestConfig  : ok  4ms (5 subtests)
go test ./internal/api/...             : ok  1.8s
go test ./internal/engine/...          : ok  100ms
```

## Management boundary

After every commit in this batch:

- eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64
- default via 172.18.1.254 dev eth0
- sshd pid 758 (unchanged throughout)

No management-plane disruption.

## New bugs found / fixed this batch

- `state.Store.Reconcile` requires 32-char hex Before / Candidate
  while `state.Revision.ID` is a 64-char sha256 hex — a real
  mismatch. The Engine surface is wired and the test pins it,
  but the underlying state.Store contract still rejects the
  natural shape. Tracked as M-25 (deferred to Batch 3 — fixing
  this requires revisiting `state.Store` and is out of scope
  for G01 surface wiring).

## Next batch

Likely candidates for Batch 3:

- G02 canonical object contract: `kind`/`id`/`revision`
  envelope + representative domains (network interface,
  route, firewall rule, WireGuard/Xray).
- G06 endpoint wiring: wrap existing read-only endpoints
  with `Collection` envelope + cursor.
- G08 dependency / provenance public surface.
- G13 secret export provider binding.
- M-25: align `state.Store.Reconcile` contract with
  `state.Revision.ID` shape.
