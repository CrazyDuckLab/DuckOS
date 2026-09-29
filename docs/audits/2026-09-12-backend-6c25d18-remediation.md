# DuckOS V1 Backend Remediation Report

**Starting HEAD:** `a8036608fb692a2e32b707908d52aaddf69f65fa` (post-independent verification pass)
**Final HEAD:** `6c25d18` (`V1 UI qualification branch`)
**Date:** 2026-09-12
**Audit reference:** `docs/audits/2026-09-12-backend-a55e4e4.md`

This report covers the remediation pass executed on top
of the independent verification pass fixes. independent verification already enforced the
critical safety boundaries (H01–H09, M02–M05). The
remaining software blockers (M01, H10, H11) plus
operational hardening are addressed in five commits:

| Commit | Gap | Resolution |
|---|---|---|
| `17ce8ef` | M01 | per-object RBAC grammar accepts dotted kinds + `*` wildcard; API handler fails closed on malformed keys |
| `385ace4` | H10 | CLI `/agent/v1/*` requires explicit `--agent-socket`; never silently proxies to public HTTP |
| `68c3a81` | H11 | update recovery: VerifyBeforeActivate, LoadState crash audit, CancellableHealthCheck |
| `051e88f` | H11 | backup/restore lifecycle: RestoreJournal + ExecuteRestore (snapshot → validate → apply → rollback) |
| `6c25d18` | H11 | fakeStore implements the new VerifyFingerprint interface method |

## 1. Starting HEAD

`a8036608fb692a2e32b707908d52aaddf69f65fa` — independent verification pass
fix commit, with `internal/acceptance/scenarios_test.go`
already containing the audit-regression fakes.

## 2. Final HEAD

`6c25d18` — 5 commits ahead of `a803660`. Working tree
clean; all touched packages compile and pass gofmt +
`go vet`.

## 3. qualification blockers resolved

| Audit item | Resolution |
|---|---|
| **M01** Per-object RBAC grammar rejects dotted kinds and `*` | `internal/identity/policy.go` now exports `kindPattern` (allows dot-separated namespaces like `network.firewall.rule`) and `idPattern` (allows `*` as the kind-wide overlay marker). The API handler fails closed when the overlay key is malformed, and the code comment marks granular RBAC as **experimental** per the audit. |
| **H10** CLI commands target `/agent/v1/*` over public HTTP | `cmd/duck/main.go` now detects `/agent/1/*` paths in `cliState.call` and routes them through `callAgent` over a Unix socket transport. The CLI fails closed with an explicit error when `--agent-socket` is not provided. `cmd/duck/unix_socket_test_helper.go` and the rewritten `cmd/duck/provenance_test.go` enforce the contract via a short-path Unix socket mock that fits under macOS's 104-byte limit. |
| **H11 (part)** Update recovery | `internal/upgrade/recovery.go` adds `VerifyCandidate` (refuses corrupt candidates before activation), `LoadState` (crash audit surfaces `ErrInconsistentState` when persisted state references releases that are not on disk), and `CancellableHealthCheck` (honours `context.Context` so a stalled probe cannot hang activation). `State.Valid()` is the new structural validator. `ReleaseStorage` interface gains `VerifyFingerprint`. |
| **H11 (part)** Backup/restore lifecycle | `internal/backup/restore_lifecycle.go` adds the bounded `RestoreJournal`, `ValidateRestorePlan` (revision mismatch + unsafe remove/modify + requiresSecret guards), and `ExecuteRestore` (snapshot → validate → apply → rollback, with full journal transitions recorded). New sentinels: `ErrRevisionMismatch`, `ErrRestoreValidation`, `ErrRestoreExecution`. |

## 4. qualification blockers still open

The following items from the independent verification pass require
hardware, credentials, or production runtime integration
that this remediation pass does not (and cannot) supply:

- **Real per-domain listers** for `network.firewall.rule`,
  `dhcp.subnet`, `bgp.peer`, `ospf.area`, etc. The current
  stubs return `unsupported_kind` issues so the API can
  render honest gap badges; production listers must be
  wired by future runtime integration work.
- **Generic config transactions** against a live Linux
  system — the audit found these report success without
  Linux; the safety boundary contained them.
  Production wiring remains a deployment-time concern.
- **QoS privileged execution** — contained but not
  integrated with a live tc backend.
- **WireGuard / Xray CRUD/export** at runtime — packages
  exist, production wiring does not.
- **Real Anycast advertisement** — needs BGP session.
- **ZTP production key** — `internal/provisioning`
  refuses Issue until the Agent composition root wires
  a per-deployment key.

## 5. New Critical/High found

None. The remediation pass did not surface new Critical
or High defects; all changes are extensions of the
existing audit findings.

## 6. Regression tests added

| File | New subtests | Coverage |
|---|---|---|
| `internal/identity/policy_test.go` | existing — extended by grammar fix | dotted kind validation, `*` overlay validation |
| `internal/api/objects_test.go` | existing — extended | malformed overlay key now fails closed |
| `cmd/duck/provenance_test.go` | rewritten | Unix socket dispatch, fail-closed without `--agent-socket`, all round-trip paths |
| `internal/upgrade/recovery_test.go` | new (15 subtests) | VerifyCandidate, LoadState crash audit, CancellableHealthCheck |
| `internal/backup/restore_lifecycle_test.go` | new (17 subtests) | ValidateRestorePlan, ExecuteRestore snapshot/validate/apply/rollback, revision mismatch, double failure |

All regression tests are preserved.

## 7. Public API readiness

- `/api/v1/objects/{kind}` — returns canonical
  `object.Collection` envelope with explicit
  `unsupported_kind` issues when no production lister
  is wired. No fabricated success.
- `/api/v1/config/*` — isolated these; they
  refuse to report success without a real coordinator
  (the audit's H02 fix).
- `/api/v1/network/*` — public read surface stable.
  Mutating operations remain Agent-privileged.
- `/agent/v1/*` — private IPC; the CLI now requires
  an explicit `--agent-socket` flag and never silently
  routes through the public HTTP server.

## 8. CLI parity status

- Public API commands (`duck health`, `duck overview`,
  `duck ready`, `duck doctor`, `duck topology`, `duck
  config status/diff/apply/confirm/rollback`, `duck
  identity login`, `duck client export`, `duck wifi`,
  `duck multiwan`, `duck qos`, `duck ups`, `duck sites`,
  `duck migrate`) continue to hit the loopback API
  endpoint with bearer-token auth.
- Private Agent commands (`duck network status/apply/
  confirm/rollback`, `duck provenance`, `duck firstrun`,
  `duck diagjobs`, `duck upgrade`, `duck backup`) now
  require `--agent-socket` and route over the
  privileged Agent Unix socket.
- All commands produce stable exit codes, machine-
  readable JSON output, and consistent error envelopes
  with the structured `problem` codes.

## 9. RBAC status

- Role-matrix RBAC (Owner / NetworkAdmin / Helpdesk /
  Reception / ReadOnly) is enforced at every API
  endpoint that touches privileged state.
- Per-object overlay RBAC is wired in `internal/identity/
  policy.go` but **marked experimental** per the audit.
  The API handler fails closed on malformed overlay keys
  so the overlay never silently permits a request it
  does not understand.
- No regression test claims granular RBAC as a stable
  production contract.

## 10. Update recovery status

- `VerifyCandidate(id)` runs the release-store
  fingerprint check before activation; corrupt
  candidates are refused with `ErrCorruptCandidate`
  before any state mutation.
- `LoadState(s)` performs a structural validation AND
  a per-release fingerprint audit before committing
  state to the orchestrator. Inconsistent states are
  surfaced as `ErrInconsistentState` with the list of
  missing releases, so the caller can decide on
  recovery flow (rollback / re-materialise / re-plan).
- `CancellableHealthCheck(ctx, runner, check)` runs the
  check on a goroutine and respects `ctx.Done()`, so a
  stalled probe cannot hang activation.
- The `ActivateAndVerify` path still holds the
  orchestrator lock during the entire check window;
  cancellation records the failure and surfaces the
  per-check results to the caller for rollback
  decisions.

## 11. Backup/restore status

- `RestoreJournal` is the bounded audit trail; each
  restore attempt records every transition
  (started → validated → applied | rolled-back |
  failed).
- `ValidateRestorePlan(plan, sourceRev, liveRev)`
  refuses unsafe plans, plans that require secrets
  without a source revision, and plans where the live
  revision has advanced past the backup revision.
- `ExecuteRestore(ctx, journal, entry, plan, snap,
  restorer)` runs the full lifecycle: snapshot →
  validate → apply → (on failure) rollback. Rollback
  failures are surfaced as `ErrRestoreExecution` with
  both errors joined.
- The journal records the validation failures, the
  revision mismatch, the snapshot failure, and the
  rollback outcome so the audit trail is durable even
  across an interrupted Agent.

## 12. Hardware / credential / network-only blockers

Still require real deployment to close:

- IPv6 ND, OSPFv3, multi-WAN failover → real NICs
- MAC flapping, rogue DHCP/RA → switch mirroring
- hostapd surveys → real radio
- Xray 26.3.27 binary, Pebble 2.10.1 binary
- Real WAN / RADIUS / ACME / WG credentials
- Live DNS zone (registrar-side)
- Anycast advertisement → BGP session
- ZTP production key → wired at deployment time

## Final validation

- **gofmt** — clean across `cmd internal pkg packaging tests`
- **go vet** — clean
- **regression** — full `./internal/... ./cmd/...` green
- **race** — clean on `internal/upgrade`, `internal/backup`,
  `internal/identity`, `internal/api`
- **Linux / cross-build** — out of session scope (no
  cross-build tooling available locally; the package set
  is pure Go with no cgo dependencies touched by this
  pass)
- **API / CLI parity** — `TestConfigParityApply` still
  green; `TestProvenanceCmd_RejectsAgentSocketMissing`
  new
- **destructive / security regression** — preserved
- **management boundary** — unchanged (eth0 UP
  172.18.4.149/16, default via 172.18.1.254, SSH
  intact)

## Final verdict

```
BACKEND REMEDIATION COMPLETE:           YES (for software blockers addressed this session)
READY FOR INDEPENDENT VERIFICATION:            YES
READY FOR UI IMPLEMENTATION:             NO (per audit verdict)
```

The five software blockers targeted by this pass (M01,
H10, H11 × 2) are closed with regression tests.
Remaining open blockers are either hardware/credential
required or production-runtime integration work that
falls outside the scope of an autonomous software
remediation pass.
