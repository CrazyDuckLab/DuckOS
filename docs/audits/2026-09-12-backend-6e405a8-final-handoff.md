# DuckOS V1 backend acceptance — final handoff

- **Commit**: `6e405a8` `fix(b06): public /api/v1/upgrade + /api/v1/backup boundary`
- **Seal**: `903a4cd` `fix(audit): seal final verification fixes` (independent verification baseline)
- **Checkpoint 1**: `cc27a3d` `docs(audit): backend-119be5c remediation report`
- **Checkpoint 2**: `74ad9c9` `docs(audit): backend-65a71ae acceptance handoff`
- **Author**: Viacheslav Fomenko
- **Date**: 2026-09-12 (Darwin 25.6.0, arm64; non-Linux runner)
- **Scope**: B-class software blockers (B01–B10) + DV00–DV10 acceptance per the contract in
  `the developer handoff prompt`. No UI. No real hardware or credentials.
  No touch to `duck-dev-01`, `eth0`, `172.18.4.149`, or the management boundary.

---

## 1. Verdict

**BACKEND VERIFICATION: PARTIAL** — every required S-tier subcase passes with
real evidence; every L-tier subcase that needs a disposable Linux fixture is
honestly reported `NOT_VERIFIED` because no such fixture is available in this
host environment. The contract permits `NOT_VERIFIED` for environment reasons
and explicitly forbids silent skip; the harness returns non-zero on every
`NOT_VERIFIED` so the operator cannot mistake a partial run for a green light.

**SAFE TO FREEZE BACKEND: NO** — 26 of 56 subcases are still `NOT_VERIFIED`.
A freeze would hide the real L-tier gap.

**SAFE TO START UI: NO** — same reasons as above; per the contract UI does
not start until the S-tier subcases all PASS (they now do) and the L-tier
subcases have been executed on a disposable Linux fixture (they have not).

**What changed since `74ad9c9`:**
- B06 is **CLOSED**. The 2 S-tier failures in DV06 are gone.
- Total: 30 PASS / 0 FAIL / 26 NOT_VERIFIED (was 28 / 2 / 26).
- L-tier status is unchanged: this host has no Linux kernel access (no root,
  no Docker, no Lima, no Multipass, no QEMU, no OrbStack, no `unshare`).
  Per the contract these remain `NOT_VERIFIED`.

---

## 2. How the contract was executed

```sh
# 1. catalogue
scripts/test-backend-acceptance.sh --list

# 2. one DV
scripts/test-backend-acceptance.sh --id DV10 --evidence /tmp/duckos-evidence

# 3. every required DV with real evidence
scripts/test-backend-acceptance.sh --all --evidence /tmp/duckos-evidence
```

`/tmp/duckos-evidence/` after the live run contains:

```
DV00/                 DV04-WG/               DV07/                 summary.json
DV01/                 DV04-XRAY/             DV08/                 acceptance.log
DV02/                 DV05/                  DV09/                 results.jsonl
DV03/                 DV06/                  DV10/
```

Each DV subdirectory holds `results.jsonl`, `summary.json`, `runner.log`, and
`test-<package>.log` for every `go test` invocation the runner issued.

Live tally (`results.jsonl`, 56 subcases):

| Status      | Count | Detail                                                                     |
|-------------|------:|----------------------------------------------------------------------------|
| PASS        |    30 | every required S-tier subcase whose evidence is reproducible on this host. |
| FAIL        |     0 | DV06 B06 closed.                                                           |
| NOT_VERIFIED|    26 | every L-tier subcase (no disposable Linux fixture available on this host). |
| silent skip |     0 | the harness refuses to silently skip a required subcase.                   |

`harness overall_exit=1` because the contract demands a non-zero exit when any
required subcase is `NOT_VERIFIED` — the operator never sees a green light on an
unverified gap.

---

## 3. Per-DV outcome

| DV    | Family                              | S-pass | S-fail | L-not-verified |
|-------|-------------------------------------|------:|-------:|---------------:|
| DV00  | production composition              |     1 |      0 |              1 |
| DV01  | core collections + identities       |     2 |      0 |              2 |
| DV02  | config transaction + Safe Apply     |     4 |      0 |              2 |
| DV03  | QoS apply / partial / rollback      |     1 |      0 |              4 |
| DV04-WG  | WireGuard runtime lifecycle      |     1 |      0 |              3 |
| DV04-XRAY| Xray runtime lifecycle           |     1 |      0 |              3 |
| DV05  | production RBAC                     |     5 |      0 |              0 |
| DV06  | normal CLI → public API parity      |     5 |      0 |              0 |
| DV07  | executable update + recovery        |     2 |      0 |              3 |
| DV08  | portable/selective restore          |     2 |      0 |              4 |
| DV09  | Anycast software advertise/withdraw |     1 |      0 |              3 |
| DV10  | authentic ZTP issuance / redemption |     5 |      0 |              1 |

### DV00 — production composition / no fake success

- **S production_composition — PASS**: the 6 production-root wiring anchors are
  present in the source:
    - `cmd/duck-agent/main.go:enableUpgradeService`
    - `cmd/duck-agent/main.go:enableZTPService`
    - `cmd/duck-agent/main.go:enableQoSService`
    - `cmd/duck-api/main.go:EnableIdentity`
    - `cmd/duck-api/main.go:EnablePolicy`
    - `internal/objectlisters/listers.go:RegisterAll`
- **L runtime_oracle_mutation — NOT_VERIFIED**: disposable Linux veth fixture
  unavailable on this host.

### DV01 — core collections, identities and pagination

- **S list_all_pages — PASS**: `go test ./internal/objectlisters/...` runs
  `TestDV01CoverageEveryKindWired` (every required kind registered),
  `TestDV01PendingKindHonestUnavailable` (no fake Complete=true on missing
  source), `TestDV01RealKindsReflectSource` (4 live listers project real data),
  `TestDV01WireguardXrayRegistered` (6 WG/Xray CRUD kinds acknowledged).
- **S detail_denied — PASS**: `go test ./internal/api/...` covers per-row RBAC
  enforcement on the list endpoint.
- **L restart_preserves_identity, disabled_daemon — NOT_VERIFIED**: no Linux
  Engine+Agent restart path available.

### DV02 — real config transaction + Safe Apply

- **S idempotent_request_id, duplicate_request_id_different_payload,
  stale_revision_refused, confirm_crossing_deadline — PASS**:
  `go test ./internal/engine/...` covers every admission gate.
- **L preview_apply_confirm, kill_after_admission — NOT_VERIFIED**: no Linux
  veth + nft/route fixture available.

### DV03 — QoS apply / partial / rollback

- **S cancel_does_not_abort_recovery — PASS**: `go test ./internal/qos/...`
  exercises the `WithoutCancel` rollback path + truth-on-failure warnings.
- **L tc_oracle_apply_rollback, partial_apply_does_not_commit,
  failed_rollback_recovery_required, deny_zero_runtime_calls — NOT_VERIFIED**:
  no real `tc qdisc` / `tc class` / `tc filter` oracle available.

### DV04-WG — WireGuard runtime lifecycle

- **S denied_export_zero_effect — PASS**: `go test ./internal/wireguard/...`
  asserts denied export produces zero key-resolver calls.
- **L crud_kernel_runtime, restart_no_resurrection,
  export_redacted_matches_qr — NOT_VERIFIED**: no kernel wg fixture.

### DV04-XRAY — Xray runtime lifecycle

- **S incompatible_protocol_rejected — PASS**: `go test ./internal/xray/...`
  asserts protocol/security validation rejects unsupported combinations
  before runtime.
- **L crud_xray_validation, subscription_qr_matches_url,
  restart_no_resurrection — NOT_VERIFIED**: no real xray daemon.

### DV05 — production RBAC

- **S wildcard_deny_wins, exact_deny_beats_wildcard_allow,
  policy_load_durability, denied_export_no_resolver — PASS**:
  `go test ./internal/identity/...` enforces every precedence rule and the
  durable PolicyStore semantics.
- **S production_startup_loads_policy — PASS**: `cmd/duck-api/main.go` calls
  `identity.OpenPolicyStore` + `handler.EnablePolicy` in `main()`.
- **L — none required for DV05**.

### DV06 — normal CLI → public API parity

- **S network_cli_uses_public_api — PASS**: `cmd/duck/config.go` reaches
  `/api/v1/config/*`.
- **S upgrade_cli_uses_public_api — PASS**: `cmd/duck/upgrade.go` now reaches
  `/api/v1/upgrade`; the new `Engine.Upgrade` method proxies the read to the
  Agent `/agent/v1/upgrade` IPC and the api server exposes it as a read-only
  `/api/v1/upgrade` endpoint with G06-bounded query keys and no body.
- **S backup_cli_uses_public_api — PASS**: `cmd/duck/backup.go` now reaches
  `/api/v1/backup`; `Engine.Backup` proxies through the Agent `/agent/v1/backup`
  IPC; the api server exposes it as a read-only `/api/v1/backup` endpoint that
  accepts the bounded `id` query parameter.
- **S actor_principal_not_spoofed — PASS**: no `--actor` flag in the normal
  CLI surface; principal always comes from the session.
- **S non_tty_does_not_prompt — PASS**: no interactive `Stdin` read in
  `cmd/duck/*.go`.

### DV07 — executable update + restart/crash recovery

- **S corrupt_package_refused, rollback_failure_not_healthy — PASS**:
  `go test ./internal/upgrade/...` asserts corrupt/short/unsigned/wrong-arch
  packages are refused before activation and that a rollback failure
  surfaces recovery-required (never `Healthy=true`).
- **L signed_package_activate, health_failure_recovery,
  kill_at_phase_boundary — NOT_VERIFIED**: no real signed fixture + Linux
  service build-marker.

### DV08 — portable/selective restore + restart recovery

- **S corrupt_archive_refused, disk_write_failure_not_committed — PASS**:
  `go test ./internal/backup/...` covers the journal-store
  `ErrCapacityExhausted` semantics and path-traversal refusal.
- **L portable_selective_restore, selected_unselected_comparison,
  cancelled_before_during_rollback, kill_at_phase_boundary — NOT_VERIFIED**:
  no cross-host fixture + cross-process kill on Linux.

### DV09 — Anycast software advertise/withdraw

- **S missing_source_never_ready — PASS**: `go test ./internal/anycast/...`
  asserts a nil source / missing FRR is reported as
  `not-advertised`, never `Ready=true`.
- **L frr_advertise_withdraw, health_failure_unadvertise,
  restart_reconciles_actual — NOT_VERIFIED**: no disposable FRR fixture.

### DV10 — authentic ZTP issuance / redemption / replay durability

- **S production_key_loader — PASS**: `cmd/duck-agent/main.go` exposes
  `--ztp-key-file` / `--ztp-journal-dir` flags, calls `enableZTPService()`
  which opens the production HMAC key (0600 mode, ≥32 bytes — short/wrong-mode
  key fails closed), constructs `*ztp.Manager` via `ztp.OpenManager`, and
  exposes it through `handler.EnableZTP(agent.NewZTPHost(mgr))`.
- **S authentic_issue_redeem — PASS**: `go test ./internal/ztp/...`
  verifies `Issue` produces an HMAC-signed `IssuedToken` and `Redeem` checks
  MAC + TTL + node binding.
- **S replay_refused — PASS**: second `Redeem` of the same `IssuedToken`
  returns `ErrTokenReplay` (replay-safe retention, no eviction of unexpired
  records).
- **S capacity_256_fail_closed — PASS**: `MaxRedemptionJournal=256`; full +
  unexpired -> `ErrCapacityExhausted`, no eviction of unexpired records.
- **S corrupt_journal_lines_skipped — PASS**: malformed journal lines
  skipped on reload; good records preserved.
- **L second_writer_no_collision — NOT_VERIFIED**: two concurrent
  `OpenManager` writers require Linux `flock(2)`; cannot execute on darwin
  without weakening the acceptance criterion.

---

## 4. B-class blocker status

| ID  | Title                                         | Verdict | Evidence                                                                                |
|-----|-----------------------------------------------|---------|-----------------------------------------------------------------------------------------|
| B01 | engine wiring                                 | CLOSED  | `internal/engine/config.go:Engine.SetConfig`, `Engine.EnableConfig`; DV00/DV02 PASS.    |
| B02 | engine wiring (alias of B01)                  | CLOSED  | (same)                                                                                  |
| B03 | network control                               | PARTIAL | unit coverage; production composition verified at DV00; L-tier NOT_VERIFIED              |
| B04 | anycast / wireguard / xray runtime            | PARTIAL | S-tier PASS for denied_export / incompatible_protocol; L-tier NOT_VERIFIED              |
| B05 | identity / policy persistence                 | CLOSED  | `internal/identity/policy_store.go` + DV05 `production_startup_loads_policy` PASS      |
| B06 | upgrade / backup CLI parity                   | CLOSED  | DV06 `upgrade_cli_uses_public_api` + `backup_cli_uses_public_api` now PASS.             |
| B07 | upgrade wiring                                | CLOSED  | `internal/agent/upgrade_host_adapter.go` `OrchestratorAdapter`; DV00 / DV07 S-tier PASS  |
| B08 | backup executor + parent-dir fsync            | CLOSED  | `internal/backup/agent_executor.go` `ExecuteRestore` + `JournalStore`; DV08 S-tier PASS |
| B09 | DV execution (this report)                    | PARTIAL | DV00–DV10 executed; 30 PASS / 0 FAIL / 26 NOT_VERIFIED; no silent skip                  |
| B10 | ZTP redesign + production key loader          | CLOSED  | `internal/ztp/ztp.go` + `enableZTPService` in `cmd/duck-agent/main.go`; DV10 S-tier PASS |

The contract asked for `B01 PARTIAL / B03 PARTIAL / B04 PARTIAL / B06 PARTIAL / B09 OPEN`
at the previous checkpoint. The current state:

- B01 — CLOSED (was PARTIAL).
- B03 — PARTIAL (S-tier PASS at DV02; L-tier NOT_VERIFIED — disposable veth not
  present on this host).
- B04 — PARTIAL (S-tier PASS at DV04-WG/XRAY; L-tier NOT_VERIFIED — kernel wg /
  xray daemon not present on this host).
- B06 — CLOSED (was OPEN; DV06 FAILs gone after the public /api/v1/upgrade +
  /api/v1/backup boundary).
- B09 — PARTIAL (was OPEN; every required DV executed; the remaining L-tier
  NOT_VERIFIED is a documented environment constraint, not a hidden defect).

---

## 5. Honest gaps the contract must surface

### 5.1 L-tier subcases — disposable Linux fixture prerequisite

Every L-tier subcase requires real Linux kernel access. This host (macOS
26.6.2, arm64) has no Docker, no Lima, no Multipass, no QEMU, no OrbStack,
no `unshare`, no root. The contract permits `NOT_VERIFIED` for environment
reasons and forbids silent skip. The harness returns non-zero so the
operator cannot mistake a partial run for a green light.

To close these subcases the verifier must run on a Linux + root host
(same as `duck-dev-01`'s target OS) — a disposable VM, container, or
netns is fine. The per-DV runners already exist; re-running on such a
host closes every L-tier subcase to PASS or FAIL.

| DV    | L subcases (Linux fixture required)                                                  |
|-------|--------------------------------------------------------------------------------------|
| DV00  | runtime_oracle_mutation — disposable veth + netns mutation                           |
| DV01  | restart_preserves_identity, disabled_daemon — Engine+Agent cross-process restart     |
| DV02  | preview_apply_confirm, kill_after_admission — Linux veth + nft/route + cross-process  |
| DV03  | tc_oracle_apply_rollback, partial_apply_does_not_commit, failed_rollback_recovery_required, deny_zero_runtime_calls — real `tc qdisc`/`tc class`/`tc filter` oracle |
| DV04-WG  | crud_kernel_runtime, restart_no_resurrection, export_redacted_matches_qr — kernel `wg` |
| DV04-XRAY| crud_xray_validation, subscription_qr_matches_url, restart_no_resurrection — xray daemon in a Linux netns |
| DV07  | signed_package_activate, health_failure_recovery, kill_at_phase_boundary — Linux service build-marker |
| DV08  | portable_selective_restore, selected_unselected_comparison, cancelled_before_during_rollback, kill_at_phase_boundary — cross-host + cross-process kill |
| DV09  | frr_advertise_withdraw, health_failure_unadvertise, restart_reconciles_actual — disposable FRR in Linux netns |
| DV10  | second_writer_no_collision — two concurrent `OpenManager` writers with Linux `flock(2)` |

### 5.2 What is now closed but was open

- B06 (`upgrade_cli_uses_public_api` + `backup_cli_uses_public_api`) — closed
  by `6e405a8` `fix(b06): public /api/v1/upgrade + /api/v1/backup boundary`.

### 5.3 What still cannot be verified here

- `DV02.L` / `DV03.L` — disposable Linux veth/netns (no root).
- `DV04-WG.L` / `DV04-XRAY.L` — kernel wg / xray runtime (no Linux kernel).
- `DV07.L` — Linux service build-marker (no Linux service manager).
- `DV08.L` — cross-host backup fixture + cross-process kill (no Linux).
- `DV09.L` — disposable FRR (no Linux).
- `DV10.L` — Linux `flock(2)` inter-process lock (no Linux kernel; macOS
  BSD `flock` is not equivalent and the contract forbids weakening).

---

## 6. Code anchors

| Anchor                                                       | What it proves                                                       |
|--------------------------------------------------------------|----------------------------------------------------------------------|
| `cmd/duck-agent/main.go:enableZTPService`                    | B10 production key loader (0600 + ≥32B + OpenManager).               |
| `cmd/duck-agent/main.go:enableUpgradeService`                | B07 production orchestrator (State() reflected on every IPC call).   |
| `cmd/duck-api/main.go:EnableIdentity`, `EnablePolicy`        | B05 production RBAC + durable PolicyStore.                           |
| `internal/engine/engine.go:Engine.Upgrade`, `Engine.Backup`  | B06 public boundary proxies to /agent/v1/* IPC.                      |
| `internal/engine/objects.go:ObjectList`                      | honest `engine_unavailable` envelope (Complete=false, not Complete=true). |
| `internal/objectlisters/listers.go:unsupportedLister`        | honest degraded surface (Complete=false + `engine_unavailable`).    |
| `internal/ztp/ztp.go:OpenManager`, `Issue`, `Redeem`         | HMAC + replay-safe retention + ErrCapacityExhausted.                 |
| `internal/identity/policy_store.go:OpenPolicyStore`          | 0600 mode + parent-dir fsync + corrupt-line skip.                   |
| `internal/backup/agent_executor.go:ExecuteRestore`           | durable journal-backed restore (B08).                                |
| `internal/api/server.go:/api/v1/upgrade`, `/api/v1/backup`   | B06 public surface + read-only + bounded query keys.                 |
| `scripts/test-backend-acceptance.sh`                         | G00 harness with `--list`, `--id DVxx`, `--all --evidence`.          |
| `scripts/dv-runners/{lib.sh,DV00..DV10,DV04-WG,DV04-XRAY}.sh` | per-DV runners with honest `PASS` / `FAIL` / `NOT_VERIFIED`.         |

---

## 7. What UI must NOT do

The contract explicitly forbids UI work in this remediation. The handoff is
backend-only; the dashboard / CLI-UI surfaces must not start until:

1. ~~B06 OPEN is closed.~~ — done at `6e405a8`.
2. Every L-tier subcase is exercised on a disposable Linux fixture and
   re-classified to `PASS` or `FAIL`.

Until then the UI surface must continue to render the honest
`engine_unavailable` issue for every kind whose S-tier source is missing
or whose L-tier runtime has not been closed.

---

## 8. What changes the verdict

- Run `scripts/test-backend-acceptance.sh --all --evidence /tmp/duckos-evidence`
  on a Linux + root fixture: every L-tier subcase moves from `NOT_VERIFIED`
  to `PASS` (or `FAIL`).

When that condition holds and no FAIL remains, `overall_exit=0` and the
same handoff can be re-written as `BACKEND VERIFICATION: PASS /
SAFE TO FREEZE BACKEND: YES / SAFE TO START UI: YES`. Until then this
report must not be quoted as endorsing a freeze or UI start.

---

## 9. Commit walk

```
6e405a8 fix(b06): public /api/v1/upgrade + /api/v1/backup boundary
74ad9c9 docs(audit): backend-65a71ae acceptance handoff
65a71ae test(acceptance): per-DV runners + harness aggregation + real evidence
d924074 fix(ztp,objects): production key loader + honest engine_unavailable + DV01 coverage
cc27a3d docs(audit): backend-119be5c remediation report   ← checkpoint 1
1560956 docs(acceptance): document the G00 harness and B-class status
119be5c test(acceptance): build G00 production-boundary harness
418cfdc fix(upgrade): wire UpgradeHost to real orchestrator via OrchestratorAdapter
9b5a2fd feat(identity): durable per-object policy overlay
b3b2b44 fix(backup): durable executor + parent-dir fsync + transition entries
f3e0b67 fix(ztp,engine): authentic issuance + replay-safe retention + Engine.config wiring
903a4cd fix(audit): seal final verification fixes         ← verification seal
```

No UI. No `duck-dev-01`. No real network / hardware. No new contracts
beyond what the remediation prompt fixed.
