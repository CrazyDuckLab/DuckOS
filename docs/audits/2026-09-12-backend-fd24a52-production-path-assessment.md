# DuckOS V1 Backend — Production-Path Assessment (DV05/DV10 fail-closed + QoS wire)

- **Date (UTC)**: 2026-09-12
- **Prior HEAD**: `ab1ea60`
- **Current HEAD**: `fd24a52`
- **Branch**: `V1 UI qualification branch`
- **Working tree**: clean

## 1. Scope of this session

The user-requested scope was: rewrite the entire L-tier acceptance so every subcase exercises
`public API / normal CLI → Engine → Agent → real runtime → independent oracle → restart/durability → negative case`,
and the FAIL rule means "DuckOS code defect must be fixed, not environment-blocked".

This assessment is the honest report of what was actually delivered in this session
versus what remains as a known DuckOS code defect. Test-harness evidence (`scripts/test-backend-acceptance.sh --all`)
is included for completeness but is NOT a substitute for production-path evidence when the contract demands it.

## 2. Commits in this session

```
fd24a52 fix(duck-agent): audit regression now asserts guarded QoS host is wired
11b61ae fix(duck-agent): wire production QoS host via qos.LinuxRunner + Controller
d58af81 fix(ztp): TestManagerMalformedLinesFailClosed replaces skip test
e1f0f1a fix(dv05/dv10): policy + ztp reload fail-closed on corruption
```

Test harness still reports **56 PASS / 0 FAIL / 0 NOT_VERIFIED** for `--all`.

## 3. What was actually fixed

### 3.1 HIGH SECURITY — DV05 policy fail-closed ✅
- `internal/identity/policy_store.go`: `reload()` now returns an error on any corrupt line
  (bad JSON, invalid grant, schema mismatch). Silently dropping a deny rule on disk corruption
  is a permissive-fallback vulnerability; the production composition root must refuse to load
  the overlay and block authorization until the operator restores a known-good file.
- `TestPolicyStoreMalformedLinesFailClosed` asserts the new contract; restores the overlay
  and confirms the good grant is preserved after restore.

### 3.2 HIGH SECURITY — DV10 ZTP replay fail-closed ✅
- `internal/ztp/ztp.go`: `Manager.reload()` now returns an error on any corrupt journal line.
  Silently dropping a redeemed-token record re-opens the replay window; the production
  composition root must refuse to start until the operator restores a known-good journal.
- `TestDV10CorruptJournalFailsClosed` + `TestManagerMalformedLinesFailClosed` assert the new
  contract. The flock(2) L-tier subcases continue to pass.

### 3.3 Production wiring — QoS host enabled ✅
- `cmd/duck-agent/main.go`: `enableQoSService()` previously returned an error to keep raw
  `tc(8)` plans out of the privileged composition root. The guarded durable coordinator
  (`qos.LinuxRunner` + `qos.NewController` + `agent.NewQoSHost`) is now wired. The Controller
  enforces preview / before-state capture / partial-apply commit gates / restart-safe
  rollback; the Agent IPC exposes the host at `/agent/v1/qos/apply`.
- `TestAuditUnsafeQoSRuntimeIsGuarded` updated to assert the new contract (host IS wired,
  Controller IS the authoritative guard).

## 4. Production-path matrix per DV

Each row answers the contract: did this DV really exercise `public entrypoint → engine →
agent → real runtime → independent oracle → restart/durability → negative case`?

Legend:
- ✅ = link exists and is exercised by an integration test
- ⚠️ = link exists in production code but is not exercised through the production boundary
- ❌ = link missing (DuckOS code defect)

| DV        | Subcase                            | Public entrypoint | Engine path | Agent path | Runtime mutation | Independent oracle | Restart/durability | Negative case | Verdict |
|-----------|------------------------------------|-------------------|-------------|------------|------------------|---------------------|---------------------|----------------|---------|
| DV00      | production_composition             | ⚠️ (grep)         | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV00      | runtime_oracle_mutation            | ❌                | ❌          | ❌         | ✅ (kernel)      | ✅ (kernel)         | ❌                  | ❌             | ⚠️ PARTIAL |
| DV01      | list_all_pages                     | ❌                | ⚠️ (lister) | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV01      | detail_denied                      | ❌                | ⚠️         | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV01      | restart_preserves_identity         | ❌                | ❌          | ❌         | ❌               | ❌                  | ✅ (catalog)         | ❌             | ⚠️ PARTIAL |
| DV01      | disabled_daemon                    | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ✅ (typed err) | ⚠️ PARTIAL |
| DV02      | idempotent_request_id              | ❌                | ✅ (Engine) | ❌         | ❌               | ❌                  | ❌                  | ❌             | ⚠️ PARTIAL |
| DV02      | duplicate_request_id_diff_payload  | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ⚠️ PARTIAL |
| DV02      | stale_revision_refused             | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ⚠️ PARTIAL |
| DV02      | confirm_crossing_deadline          | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ⚠️ PARTIAL |
| DV02      | preview_apply_confirm              | ❌                | ✅ (in-netns)| ❌        | ✅ (dummy/route) | ✅ (kernel)         | ❌                  | ❌             | ⚠️ PARTIAL |
| DV02      | kill_after_admission               | ❌                | ✅ (catalog)| ❌         | ❌               | ❌                  | ❌                  | ❌             | ⚠️ PARTIAL |
| DV03      | cancel_does_not_abort_recovery     | ❌                | ✅ (qos)    | ⚠️ (wired now)| ✅ (tc)      | ✅ (tc oracle)      | ❌                  | ❌             | ⚠️ PARTIAL |
| DV03      | tc_oracle_apply_rollback           | ❌                | ✅          | ⚠️         | ✅ (tc)          | ✅ (tc)             | ❌                  | ❌             | ⚠️ PARTIAL |
| DV03      | partial_apply_does_not_commit      | ❌                | ✅          | ⚠️         | ✅ (tc)          | ✅ (tc)             | ❌                  | ❌             | ⚠️ PARTIAL |
| DV03      | failed_rollback_recovery_required  | ❌                | ✅          | ⚠️         | ✅ (tc)          | ✅ (tc)             | ❌                  | ❌             | ⚠️ PARTIAL |
| DV03      | deny_zero_runtime_calls            | ❌                | ✅          | ⚠️         | ✅ (tc)          | ✅ (tc)             | ❌                  | ❌             | ⚠️ PARTIAL |
| DV04-WG   | crud_kernel_runtime                | ❌                | ❌          | ❌         | ✅ (host ns wg)  | ✅ (wg show)        | ✅ (catalog reopen) | ❌             | ❌ FAIL |
| DV04-WG   | restart_no_resurrection            | ❌                | ❌          | ❌         | ✅               | ✅                  | ✅                  | ❌             | ❌ FAIL |
| DV04-WG   | export_redacted_matches_qr         | ❌                | ❌          | ❌         | ❌ (no Agent)    | ❌                  | ❌                  | ✅ (no PrivateKey) | ❌ FAIL |
| DV04-WG   | denied_export_zero_effect          | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ✅ (s-level)   | ❌ FAIL |
| DV04-XRAY | crud_xray_validation               | ❌                | ❌          | ❌         | ✅ (xray run -test) | ✅ (Configuration OK) | ❌             | ✅ (bad JSON rejected) | ❌ FAIL |
| DV04-XRAY | subscription_qr_matches_url        | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV04-XRAY | restart_no_resurrection            | ❌                | ❌          | ❌         | ✅ (catalog)    | ✅                  | ✅                  | ❌             | ❌ FAIL |
| DV04-XRAY | incompatible_protocol_rejected     | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ✅ (s-level)   | ❌ FAIL |
| DV05      | wildcard_deny_wins                 | ⚠️ (s-level)     | ✅ (policy) | ✅        | ❌               | ❌                  | ✅ (fail-closed)    | ✅ (corrupt)   | ⚠️ PARTIAL |
| DV05      | exact_deny_beats_wildcard_allow    | ⚠️ (s-level)     | ✅          | ✅        | ❌               | ❌                  | ✅                  | ✅             | ⚠️ PARTIAL |
| DV05      | policy_load_durability             | ⚠️ (s-level)     | ✅          | ✅        | ❌               | ❌                  | ✅ (fail-closed)    | ✅             | ⚠️ PARTIAL |
| DV05      | denied_export_no_resolver          | ⚠️ (s-level)     | ✅          | ✅        | ❌               | ❌                  | ❌                  | ✅             | ⚠️ PARTIAL |
| DV05      | production_startup_loads_policy    | ✅ (grep)         | ✅          | ✅        | ❌               | ❌                  | ✅                  | ❌             | ⚠️ PARTIAL |
| DV06      | network_cli_uses_public_api        | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV06      | upgrade_cli_uses_public_api        | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV06      | backup_cli_uses_public_api         | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV06      | actor_principal_not_spoofed        | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV06      | non_tty_does_not_prompt            | ❌                | ❌          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV07      | corrupt_package_refused            | ⚠️ (s-level)     | ✅ (orchestrator) | ⚠️ (nil storage) | ❌       | ❌                  | ❌                  | ✅             | ❌ FAIL |
| DV07      | rollback_failure_not_healthy       | ⚠️ (s-level)     | ✅          | ⚠️         | ❌               | ❌                  | ❌                  | ✅             | ❌ FAIL |
| DV07      | signed_package_activate            | ❌                | ⚠️         | ⚠️         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV07      | health_failure_recovery            | ❌                | ⚠️         | ⚠️         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV07      | kill_at_phase_boundary             | ❌                | ⚠️         | ⚠️         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV08      | corrupt_archive_refused            | ⚠️ (s-level)     | ✅ (backup) | ❌ (read-only host) | ❌ | ❌                 | ❌                  | ✅             | ❌ FAIL |
| DV08      | disk_write_failure_not_committed   | ⚠️ (s-level)     | ✅          | ❌         | ❌               | ❌                  | ❌                  | ✅             | ❌ FAIL |
| DV08      | portable_selective_restore          | ❌                | ✅ (executor) | ❌       | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV08      | selected_unselected_comparison     | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV08      | cancelled_before_during_rollback   | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV08      | kill_at_phase_boundary             | ❌                | ✅          | ❌         | ❌               | ❌                  | ❌                  | ❌             | ❌ FAIL |
| DV09      | missing_source_never_ready         | ⚠️ (s-level)     | ✅ (anycast) | ⚠️ (read-only host) | ❌ | ❌                | ❌                  | ✅             | ❌ FAIL |
| DV09      | frr_advertise_withdraw             | ❌                | ❌          | ❌         | ✅ (kernel ip route) | ✅ (kernel)     | ❌                  | ❌             | ❌ FAIL |
| DV09      | health_failure_unadvertise         | ❌                | ❌          | ❌         | ✅ (kernel)      | ✅                  | ❌                  | ❌             | ❌ FAIL |
| DV09      | restart_reconciles_actual          | ❌                | ❌          | ❌         | ✅ (kernel)      | ✅                  | ❌                  | ❌             | ❌ FAIL |
| DV10      | production_key_loader              | ✅ (grep)         | ✅ (ztp)    | ✅         | ❌               | ❌                  | ✅                  | ❌             | ⚠️ PARTIAL |
| DV10      | authentic_issue_redeem             | ❌                | ✅ (ztp)    | ✅ (ZTPHost) | ❌              | ❌                  | ✅ (journal)        | ✅             | ⚠️ PARTIAL |
| DV10      | replay_refused                     | ❌                | ✅          | ✅         | ❌               | ❌                  | ✅                  | ✅             | ⚠️ PARTIAL |
| DV10      | capacity_256_fail_closed           | ❌                | ✅          | ✅         | ❌               | ❌                  | ✅                  | ✅             | ⚠️ PARTIAL |
| DV10      | corrupt_journal_lines_skipped      | ❌                | ✅          | ✅         | ❌               | ❌                  | ✅ (fail-closed)    | ✅             | ⚠️ PARTIAL |
| DV10      | second_writer_no_collision         | ❌                | ✅ (flock)  | ✅         | ❌               | ❌                  | ✅ (real flock)     | ✅             | ⚠️ PARTIAL |

## 5. Aggregate honest tally (production-path contract)

- **PASS** (every link ✅): **0**
- **PARTIAL** (most links ⚠️): **16**
- **FAIL** (production-path missing or test bypasses production boundary): **40**

The test harness reports 56 PASS because it does not enforce the production-path contract; it
records whether each Go test command exited 0. The Go tests themselves exercise packages
directly (not through the public API → Engine → Agent path), so the test harness is
**not equivalent to the production-path matrix** above.

## 6. Remaining DuckOS code defects (FAIL, not NOT_VERIFIED)

These are concrete production-wiring gaps that block the production-path contract. Each
must be fixed to achieve the user's "FAIL=0, NOT_VERIFIED=0" goal:

1. **DV00 production_composition** — no integration test that boots `cmd/duck-api` + `cmd/duck-agent`
   binaries, opens the public socket, and exercises the canonical wiring anchor. Grep is not evidence.
2. **DV01 list_all_pages / detail_denied** — no integration test that drives
   `GET /api/v1/objects/{kind}` through the live HTTP handler with a real Engine + lister.
3. **DV01 restart_preserves_identity** — no Agent cross-process restart test that observes
   `lister.SnapshotID()` preserved across the agent's `kill -9` + reopen.
4. **DV02 idempotent_request_id / duplicate / stale_revision / confirm_deadline** — no
   integration test that drives `POST /api/v1/config/prepare` with a real Engine + state.Open
   journal.
5. **DV02 kill_after_admission** — no Engine cross-process kill test that observes the
   durable record without double execution.
6. **DV03** — QoS host is now wired but no integration test that drives
   `POST /agent/v1/qos/apply` with a real Engine → Agent IPC → LinuxRunner → tc(8) oracle.
7. **DV04-WG** — no `internal/agent/wg_host.go` exists. The WireGuard CRUD has no Agent IPC
   endpoint; the tests call the catalog directly.
8. **DV04-XRAY** — no `internal/agent/xray_host.go` exists. The Xray CRUD has no Agent IPC
   endpoint; the tests call the catalog directly.
9. **DV06** — no harness that invokes the real `cmd/duck` binary and asserts transport /
   endpoint / effective principal parity. The current runner greps source.
10. **DV07** — `enableUpgradeService()` constructs the orchestrator with `nil` ReleaseStorage.
    No real activation path; the tests exercise the orchestrator's state machine in-memory.
11. **DV08** — `agent.NewBackupHost(nil)` is a read-only stub. No `internal/agent/restore_host.go`
    exists. Real restore execution (Executor + JournalStore) is not wired into the Agent IPC.
12. **DV09** — `agent.EnableAnycast(...)` is a read-only source. No `internal/agent/frr_host.go`
    exists. Real FRR execution (zebra + bgpd) is not wired; the current tests use the kernel
    `ip route` oracle which is NOT a FRR oracle (the user explicitly rejected that).
13. **DV06/DV07/DV08/DV09 negative case** — many negative cases (denied export, denied apply,
    refused restore) are exercised at the package level but not through the public surface.

## 7. Required NOT_VERIFIED

- **0**

No subcase is blocked by an external A-class resource. Every FAIL above is a DuckOS code
defect (missing production host / unwired executor / unwired journal / etc.) that can be fixed
in subsequent sessions.

## 8. Management boundary

Asserted before and after every destructive test group in this session:

```
eth0              UP             172.18.4.149/16
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149
LISTEN on 0.0.0.0:22 and [::]:22
systemctl is-active ssh = active
```

No drift.

## 9. Verdict (per the user's contract)

```
BACKEND VERIFICATION:                FAIL   (40 subcases do not satisfy the production-path contract)
SAFE TO FREEZE BACKEND:              NO    (production hosts for DV03/DV04-WG/DV04-XRAY/DV07/DV08/DV09 are missing or unwired)
SAFE TO START UI IMPLEMENTATION:     NO    (UI gate is closed by contract)
```

## 10. Remediation roadmap (not delivered in this session)

The remaining 40 FAILs are concrete code defects. Each requires:
- A new Agent IPC host (e.g. `internal/agent/wg_host.go`) wrapping the existing package
- Wiring it into `cmd/duck-agent/main.go` alongside `enableZTPService` / `enableQoSService`
- An integration test that boots `cmd/duck-api` + `cmd/duck-agent` in-process and exercises the
  public surface through `httptest`
- An independent runtime oracle (e.g. real `wg show` / `vtysh` / `xray api statsquery`) that
  does NOT share code with the production path

Estimated scope: ~1500 LOC of new production wiring + ~1000 LOC of integration tests.
This is a multi-day effort, not a single session.

End of assessment.
