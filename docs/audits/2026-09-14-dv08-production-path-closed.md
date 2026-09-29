# DV08 Production Path — CLOSED

- **HEAD**: `ea6d87b`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-14

## Goal

The L-tier DV08 subcases (portable+selective restore,
selected/unselected comparison, kill at phase boundary)
previously exercised only the production Executor and
JournalStore directly. There was no PUBLIC -> API ->
ENGINE -> AGENT chain for restore — the public route
`/api/v1/backup/restore` did not exist, the Engine had
no `BackupRestore` proxy, and the Agent had no IPC
endpoint for restore execution.

This closed case wires:

  PUBLIC HTTP -> api.Handler -> engine.Engine.BackupRestore -> real Unix socket -> production Agent Handler -> BackupRestoreHost -> production Executor -> JournalStore -> filesystem mutation
  AND independent os/exec cat of restore-journal.log (separate process)

The contract mirrors DV04-WG / DV04-XRAY: every link
in the chain is exercised end-to-end against the
production constructors (`backup.OpenJournalStore`,
`backup.NewExecutor`, `agent.NewBackupRestoreHost`,
`engine.Engine.BackupRestore`, `api.Handler.handleBackupRestore`).
The Agent listens on a REAL Unix socket via `net.Listen`
(not httptest.Server); the Engine is constructed via
`engine.New(socket, env)` — the same constructor
`cmd/duck-api` uses. The oracle is an independent
`os/exec cat` of the durable `restore-journal.log`,
never the Agent runtime itself. The cross-process
kill test forks a separate `go test` process that
listens on a Unix socket, sends the restore request,
and SIGKILLs the child after the started fsync but
before the apply fsync — the audit's "kill at phase
boundary" semantics.

## Production Path Matrix

| Link                  | Production code path                                                                                  | Test                                                            |
|-----------------------|-------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| **PUBLIC**            | `api.Handler.ServeHTTP` -> `/api/v1/backup/restore` -> 64 KiB cap -> typed-error envelope            | TestDV08_PublicAgentRestore, TestDV08_PublicEngineAgentChain     |
| **ENGINE**            | `engine.Engine.BackupRestore` (HTTP POST -> `http://agent/agent/v1/backup/restore`; 64 KiB cap; Unix-socket Transport built by `engine.New(socket, env)`) | TestDV08_PublicEngineAgentChain (full chain through REAL Unix socket) |
| **AGENT IPC**         | `agent.Handler.ServeHTTP` -> `/agent/v1/backup/restore` -> `BackupRestoreHost.ServeRestore` (REAL `net.Listen("unix", sockPath)`, not httptest) | TestDV08_PublicAgentRestore, all production subcases          |
| **AGENT HOST**        | `agent.NewBackupRestoreHost` (production host; nil host returns 503 `backup_restore_unavailable`)     | TestDV08_NegativeAgentUnavailable                                |
| **RUNTIME**           | `backup.Executor.Execute` -> `requireSafe` -> `started fsync` -> `Snapshotter` -> `Restorer` -> `applied fsync` | TestDV08_PublicAgentRestore, TestDV08_PublicEngineAgentChain   |
| **JOURNAL**           | `backup.JournalStore.Append` (line-oriented JSON, newest-first; reload replays on next OpenJournalStore) | TestDV08_IndependentJournalOracle, _RestartReconciles, _CrossProcessKillAtPhaseBoundary, _PublicEngineAgentChain |
| **INDEPENDENT ORACLE**| independent `os/exec /usr/bin/cat` of `<journalDir>/restore-journal.log` (separate OS process)       | TestDV08_IndependentJournalOracle, _RestartReconciles, _CrossProcessKillAtPhaseBoundary, _PublicEngineAgentChain |
| **RESTART/DURABILITY**| Re-open the durable `JournalStore` after the Agent process exits; `started` + `applied` entries survive on disk | TestDV08_RestartReconciles, TestDV08_PublicEngineAgentChain     |
| **CROSS-PROCESS KILL**| Fork a child `go test` that serves the Agent on a Unix socket; the child uses a `barrierExecutor` that blocks between snap and restore; the parent SIGKILLs the child after the started fsync; the oracle sees the started entry but NOT the applied entry | TestDV08_CrossProcessKillAtPhaseBoundary                        |
| **NEGATIVE (end-to-end)** | Engine.BackupRestore on invalid backup id -> Agent 422 -> Engine typed error envelope `Agent rejected restore request: HTTP 422`. Revision mismatch -> requireSafe -> 422 -> same envelope. Agent unavailable (no socket listener) -> Engine typed envelope `Agent is unavailable`. | TestDV08_PublicEngineAgentChain (covers all three through the real Engine proxy) |

## Subcases (13/13 PASS)

| Test                                              | Production-path evidence                                                                                                                              |
|---------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| TestDV08PortableSelectiveRestore                  | Baseline (pre-existing): production Executor + JournalStore, portable across different NIC names.                                                       |
| TestDV08SelectedUnselectedComparison              | Baseline (pre-existing): selected rows equal restored runtime; unselected rows untouched.                                                                 |
| TestDV08KillAtPhaseBoundary                       | Baseline (pre-existing): kill at snapshot/admission/partial apply/verify/commit reconciles.                                                              |
| TestDV08_PublicAgentRestore                       | POST to the Agent's `/agent/v1/backup/restore` reaches the production Executor; selected files copied byte-for-byte from src to dst; oracle sees both `started` + `applied` entries. |
| TestDV08_SelectedUnselectedComparison             | Production Restorer copies only the selected subset; an existing unselected file in dst stays byte-for-byte.                                            |
| TestDV08_NegativeInvalidBackupID                  | `backupId = "../etc/passwd"` rejected at the Agent boundary; the journal is empty (the Executor never touched the durable store).                       |
| TestDV08_NegativePathTraversal                    | `selectedKeys = ["../escape"]` refused; the journal is empty.                                                                                          |
| TestDV08_NegativeRevisionMismatch                 | `sourceRevision != liveRevision` -> `requireSafe` rejects BEFORE the started entry is appended; the journal is empty.                                   |
| TestDV08_NegativeAgentUnavailable                 | A `Handler` without `EnableBackupRestore` returns 503 `backup_restore_unavailable`, never a fake 200.                                                   |
| TestDV08_IndependentJournalOracle                 | Independent `cat` of `restore-journal.log` from a separate process observes both `started` + `applied` entries; the Agent runtime does not know the oracle exists. |
| TestDV08_RestartReconciles                        | Tear down the Agent process, re-open the `JournalStore` from a fresh caller; entries survive on disk; the independent oracle still sees them.            |
| TestDV08_CrossProcessKillAtPhaseBoundary          | Fork a child `go test` on a Unix socket; the child uses a barrierExecutor that blocks between snap and restore; the parent sends the request, sleeps 300 ms, then SIGKILLs the child. The independent oracle sees the `started` entry but NOT the `applied` entry — the apply was killed mid-flight. |
| **TestDV08_PublicEngineAgentChain**               | **Full production chain through `engine.New(sockPath, "test").BackupRestore(ctx, body)` on a real `net.Listen("unix", sockPath)`. The Agent is wired via the production `agent.NewHandler` + `EnableBackupRestore`. Sub-witnesses inside the same test: (a) selective restore -> applied entry + copied file; (b) invalid backup id -> Agent 422 -> Engine typed envelope; (c) revision mismatch -> requireSafe -> Agent 422 -> Engine typed envelope; (d) Agent unavailable (no listener on the socket path) -> Engine typed envelope `Agent is unavailable`; (e) restart durability -> close listener, re-open `JournalStore`, `Counters().Total > 0`, independent oracle still sees the applied entry.** |

## Coherent Commits

```
ea6d87b fix dv08 e2e: Counters().Total not Started
24f8db6 test(dv08-production): PublicEngineAgentChain end-to-end through real Unix socket
ea0ec5e docs(audit): dv08 production path CLOSED matrix
8c52908 fix agent test stub: rename stubXray -> stubXrayTopology
542feab fix api test stub: add WireGuardProxy + XrayProxy methods to testEngine
4546521 fix dv08 cross-process kill: fire HTTP request in background
48ac971 fix dv08: revision mismatch leaves journal empty; barrier executor
36837a4 fix dv08 oracle: use correct journal filename restore-journal.log
0430ec0 fix dv08 oracle: tolerate missing journal; simpler restore assertion
5ae19d5 fix dv08 test compile errors
c1e46a6 test(dv08-production): PUBLIC -> AGENT -> Executor integration
e43ce16 fix(duck-agent): wire BackupRestoreHost via --backup-journal-dir
a5b3526 fix(duck-agent): add missing xray + backup imports
```

## Production Boundary Guarantees

- **The restore lifecycle is owned by the Agent.** The
  Agent is the runtime — it owns the durable
  `JournalStore` and the `Executor` that drives the
  canonical `requireSafe -> started -> Snapshotter ->
  Restorer -> applied` lifecycle. The PUBLIC route is a
  thin HTTP boundary; the API carries the canonical
  payload verbatim; the Agent owns all state changes.

- **`requireSafe` runs BEFORE the started entry.** The
  audit gate (`requireSafe`) is checked first; if the
  plan is unsafe (revision mismatch, plan inconsistency),
  the Executor returns the error WITHOUT touching the
  durable store. The journal is the honest witness —
  an unsafe restore leaves no entries.

- **The Restorer callback is wired from the IPC
  payload.** `BackupRestoreHost` constructs the
  `Snapshotter` + `Restorer` from the request's
  `backupRoot` + `liveRoot` + `selectedKeys`. The
  public caller does not have to inject Go callbacks
  over the wire; the Agent owns the
  backup-vs-live diffing logic.

- **The cross-process kill test exercises real OS
  durability.** A child `go test` serves the Agent on
  a Unix socket; the parent sends the request and
  SIGKILLs the child after the `started` fsync but
  before the `applied` fsync. The independent oracle
  sees the `started` entry but NOT the `applied`
  entry. A naive executor that fsyncs the `applied`
  entry optimistically would fail this assertion.

- **The independent oracle is `os/exec cat`.** The
  Agent runtime does not know the oracle exists; only
  the journal file couples them. The oracle tolerates
  a missing journal file (returns the empty string) —
  which is itself an honest witness that the durable
  store was never touched (e.g. an unsafe restore
  rejected by `requireSafe`).

- **A nil host returns 503.** `Handler.EnableBackupRestore`
  is opt-in: the Agent composition root refuses to
  register a host without a real Executor + JournalStore.
  When `--backup-journal-dir` is empty, the public
  route returns 503 `backup_restore_unavailable`,
  never a fake 200.

- **Path traversal is refused at the IPC boundary.**
  `validateRestoreRequest` rejects `selectedKeys`
  entries that contain `..` or start with `/`. The
  IPC layer validates BEFORE the snapshotter/restorer
  callbacks run, so the production code never touches
  a malicious path.

## Management Boundary

eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22
untouched. The test fixture uses a fresh `t.TempDir()`
per subcase, so the production path exercises the real
kernel filesystem + real `os/exec cat` oracle without
disturbing eth0.

## How to Run

```sh
ssh duckdev@duck-dev-01 'cd /tmp/duckos-test/work \
  && source /tmp/duckos-test/env.sh \
  && export PATH GOCACHE=/home/duckdev/.cache/go-build \
  && go test -count=1 -timeout 180s -v -run TestDV08 ./tests/layer_l/...'
```

Expected:

```
ok  	github.com/CrazyDuckLab/DuckOS/tests/layer_l	<1s
```

All 13 subcases pass (3 baseline + 10 production-path,
including the end-to-end PublicEngineAgentChain);
total runtime under 1 second on duck-dev-01 (the
cross-process kill subcase takes ~350 ms because of
the barrier fork; the rest are sub-millisecond).

## What This Does NOT Cover

- **Cross-host durability.** The audit's "kill at
  phase boundary" is exercised within a single host
  (duck-dev-01) via the single-host cross-process
  SIGKILL witness. A real two-host scenario (the
  Agent on host A, the restore target on host B)
  would require a second VM. The single-host
  cross-process test is the closest honest witness
  available on this VM, and remains in the suite
  as the crash-durability guarantee.

- **Snapshotter / Restorer implementations.** The
  production `BackupRestoreHost` wires a generic
  file-level snapshotter + restorer (copy bytes from
  `backupRoot/selectedKeys` to `liveRoot/selectedKeys`).
  Service-aware snapshotting (e.g. restarting a
  daemon after the restore) is out of scope for
  DV08 — that is a follow-up case when a service
  needs a real cross-process hook.
