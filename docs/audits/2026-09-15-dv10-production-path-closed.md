# DV10 Production Path — CLOSED

- **HEAD**: `1c91123`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv10_production_test.go`

## Goal

Production-boundary witness for the **replay fail-closed** contract:
PUBLIC issuance → `ztp.Manager.Redeem` → durable redemption journal
→ process restart → corrupt journal handling → consumed token NEVER
becomes usable again. Real cross-process `flock(2)` witness via a
separate `os/exec` child process so the inter-process exclusion is
observable end-to-end.

Independent oracles:

1. **Cross-process flock witness**: `os/exec` re-execs the test binary
   with `-test.run=TestDV10CrossProcessHelper` against the same dir.
   The helper opens the same Manager and reports its outcome via the
   exit code (`2` = `ErrLocked` observed, expected; `0` = lock
   acquired, would race the parent — FAIL).
2. **On-disk journal oracle**: `os.ReadFile` against
   `redemption-journal.log` confirms the redemption evidence is
   observable byte-for-byte from outside the in-process Manager.
3. **`/usr/bin/stat` oracle** for journal file mode (0600) and dir
   mode (no group/world bits).

## Production Path Wired

```
PUBLIC ADMIN ISSUANCE
  -> production ztp.Manager.Issue
     (HMAC-SHA256 over claims, keyed on the production secret)
  -> production ztp.Manager.Redeem
     (MAC + TTL + scope/node + replay dedup + capacity gate)
  -> production ztp.Manager.appendLocked
     (in-memory index + tmp+rename+fsync+parent-fsync)
  -> durable evidence: redemption-journal.log (0o600) on disk
  -> independent cross-process witness (separate os/exec child)
  -> independent file oracle (os.ReadFile + sha256File)
```

Every fixture uses production constructors:

- `ztp.OpenManager(dir, productionKey)` — real per-dir `flock(2)` +
  reload() with fail-closed corruption guard
- `Manager.Issue(ctx, IssueRequest)` — real HMAC-SHA256 over claims
- `Manager.Redeem(ctx, IssuedToken, observedNode)` — real MAC verify +
  TTL freshness + scope/node binding + replay dedup + capacity gate
- `Manager.persistLocked` — real `tmp+rename+fsync+parent-fsync`

No `allow-package-internal-helper` shortcuts. Every assertion runs
through the production chain end-to-end.

## Subcases

| Subcase | Path | Production defect it pins |
| --- | --- | --- |
| `TestDV10ProductionPathIssuanceRedemptionPersists` | `Issue + Redeem + FindByDigest` | Public issuance chain produces durable evidence: journal contains TokenDigest + `"phase":"redeemed"`; `FindByDigest` round-trips |
| `TestDV10ProductionPathRestartPreservesReplayProtection` | Close + re-`OpenManager` + replay | After restart, journal is byte-identical; `Redeem` of original token returns `ErrTokenReplay` (durable dedup survives process boundary) |
| `TestDV10ProductionPathCorruptJournalFailsClosed` | `OpenManager` on corrupt file | Inject garbage line → `OpenManager` returns `journal corrupt line N: invalid character...`; restore original → `Counters().Total == 1` |
| `TestDV10ProductionPathConsumedTokenNeverReusable` | 4-phase replay chain | Phase 2: immediate replay refused; Phase 3: post-restart replay refused; Phase 4: post-corrupt-restore replay STILL refused |
| `TestDV10ProductionPathCrossProcessFlockWitness` | `os/exec` re-exec helper | Separate child process acquires the same dir → `OpenManager` returns `ErrLocked` (cross-process `flock(2)` exclusion) |
| `TestDV10ProductionPathJournalFileModeIsRestrictive` | `os.Stat` + `/usr/bin/stat` oracle | Journal `0o600`, parent dir no group/world bits |
| `TestDV10ProductionPathTruncatedJournalFailsClosed` | `OpenManager` on truncated journal | Mid-JSON truncation (file length halved) → `OpenManager` fails closed |

## Result Matrix

| Subcase | Status | Notes |
| --- | --- | --- |
| `TestDV10ProductionPathIssuanceRedemptionPersists` | PASS | 0.00s; journal contains TokenDigest + redeemed phase |
| `TestDV10ProductionPathRestartPreservesReplayProtection` | PASS | 0.00s; replay refused after Close+re-Open |
| `TestDV10ProductionPathCorruptJournalFailsClosed` | PASS | 0.00s; corrupt line fails closed; restore preserves original |
| `TestDV10ProductionPathConsumedTokenNeverReusable` | PASS | 0.00s; consumed token refused in all 4 phases |
| `TestDV10ProductionPathCrossProcessFlockWitness` | PASS | 0.01s; separate `os/exec` child surfaces `ErrLocked` |
| `TestDV10ProductionPathJournalFileModeIsRestrictive` | PASS | 0.00s; journal 0o600 + dir no group/world bits |
| `TestDV10ProductionPathTruncatedJournalFailsClosed` | PASS | 0.00s; mid-JSON truncation fails closed |

7/7 PASS, FAIL 0, NOT_VERIFIED 0.

## Production Defects Pinned (none fixed in this commit)

The replay fail-closed contract is already implemented correctly:

- `internal/ztp/ztp.go:317` — `reload()` fails on corrupt line;
  in-memory journal is not touched
- `internal/ztp/ztp.go:438` — `Redeem` checks `byDigest` for replay
  protection before appending
- `internal/ztp/ztp.go:445` — capacity gate refuses growth when
  the oldest record is still unexpired
- `internal/ztp/ztp.go:537` — `appendLocked` rewrites the journal
  atomically (tmp+rename+fsync+parent-fsync)
- `internal/ztp/lock_linux.go:14` — `flock(2)` inter-process
  exclusion

DV10's production-path witnesses exercise the existing replay
fail-closed contract end-to-end against the production `OpenManager`
constructor.

## Cross-Process Witness Detail

The cross-process flock witness is the highest-risk production
invariant: a second process racing the durable dedup index could
silently re-open the replay window. The witness:

1. Opens `Manager` in the parent process via the production
   `OpenManager(dir, key)` constructor
2. Confirms the lock file is `0o600` on disk via in-process
   `os.Stat` (independent oracle: parent has the file descriptor
   that holds the flock)
3. Spawns a child process via `exec.Command(childPath,
   "-test.run=TestDV10CrossProcessHelper", "-test.v")` with
   `DV10_DIR` + `DV10_KEY` set in the environment
4. The child runs `TestDV10CrossProcessHelper` which calls
   `ztp.OpenManager(dir, key)` and exits:
   - `2` if `ErrLocked` observed (expected — PASS)
   - `0` if the lock was acquired (would race the parent — FAIL)
   - `1` for any other error
5. The parent asserts exit code `2` and that the child's output
   contains `"ErrLocked"`

When the parent runs as root (`sudo` from CI), the child is
spawned via `sudo -E` so it has the same uid and can open the
parent's `0o600` lock file. This is the production-realistic
witness: two distinct processes cannot both hold the journal's
writer lock.

## Management Boundary Untouched

No changes to:

- `eth0 / 172.18.4.149/16 / default via 172.18.1.254`
- SSH `:22`
- API loopback bound address (no API surface exercised; pure
  ZTP Manager construction)
- UI surface (not started)

## Handoff

DV10 is CLOSED. The full DV00..DV10 backend acceptance chain is
complete; the next milestone is the full backend acceptance
regression run.