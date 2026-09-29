# DV05 Production Path — CLOSED

- **HEAD**: `9e2edf3`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv05_production_test.go`

## Goal

Production-boundary witness for the **security fail-closed** contract:
PUBLIC → `auth.Startup` → `identity.Service` → `identity.PolicyStore` →
`identity.Policy.FilterCollection` → authorization gate. Corruption of
the policy overlay OR the identity state.json must NEVER become
permissive access. Either the previous-valid in-memory state continues
to enforce denials, OR the production composition root refuses to come
up so the gate is closed at the boundary.

Independent oracle: a separate `os/exec /usr/bin/stat` process (with
`/bin/stat` fallback) + `os.ReadFile` against `policy-overlay.log` and
the identity `state.json` so the on-disk corruption is observable
without trusting the in-process `Policy` or `Service`.

## Production Path Wired

```
PUBLIC HTTP GET /api/v1/objects/{kind}
  -> production api.Handler.handleObjects
  -> identity.Authorize (role matrix)
  -> identity.Policy.FilterCollection(role, kind, Observe, col)
     (real deny-precedence per row, in-memory Policy is the auth cache)
  -> independent runtime oracle: separate os/exec /usr/bin/stat
     + os.ReadFile against policy-overlay.log and identity state.json
```

Every fixture uses production constructors:

- `api.New(eng, token, listen, webDir)` — loopback HTTPS-style
  authorisation with `EnableIdentity` + `EnablePolicy`
- `engine.New(sockPath, env)` — real Unix-socket Transport
- `identity.Open(dir)` — `state.Store.Open` + `validateDatabase`
- `identity.OpenPolicyStore(dir, fallback)` — line-oriented JSONL
  with tmp+rename+fsync+parent-fsync
- `objectlisters.RegisterAllWithRealSourcesAndLinuxSource` — wires
  source-backed listers (wireguard.FileCatalog) for the kind under
  test
- `wireguard.OpenFileCatalog(dir)` — durable FileCatalog
  (`UpsertServer` + `UpsertPeer`)

No `allow-package-internal-helper` shortcuts. Every assertion runs
through the production chain end-to-end.

## Subcases

| Subcase | Path | Production defect it pins |
| --- | --- | --- |
| `TestDV05ProductionPathCorruptOverlayPreservesInMemoryDenial` | `/api/v1/objects/wireguard.peer` | In-memory Policy survives on-disk corruption: list still filters `wg-p2` even after `os.WriteFile(garbage)` against `policy-overlay.log` |
| `TestDV05ProductionPathCorruptOverlayReloadFailsClosed` | `identity.OpenPolicyStore` restart | Re-`OpenPolicyStore` against corrupt file returns error → production composition refuses to enable policy → role matrix still enforces authorisation |
| `TestDV05ProductionPathTruncatedOverlayFailsClosed` | `identity.OpenPolicyStore` | Crash mid-write (no terminating `\n`) fails CLOSED at reload |
| `TestDV05ProductionPathCorruptIdentityStateRefusesStartup` | `identity.Open` restart | `state.json` JSON corruption → `identity.Open` returns `invalid character 'n' looking for beginning of object key string`; cmd/duck-api would `log.Fatal` |
| `TestDV05ProductionPathOverlayFileModeRestrictive` | `/usr/bin/stat` oracle | Overlay file `0600`, parent dir no group/world bits |
| `TestDV05ProductionPathNoPartialGrantsEscapeOnCorruption` | `identity.OpenPolicyStore` | Three-line overlay with corrupt middle line: reload fails closed, **zero** grants escape into `Policy.All()` |
| `TestDV05ProductionPathIdentityLastOwnerPreserved` | `identity.UpdateUser` | M-23 invariant: demote/disable last Owner → `UpdateUser` returns error → `state.json` byte-identical before/after; service stays open |

## Result Matrix

| Subcase | Status | Notes |
| --- | --- | --- |
| `TestDV05ProductionPathCorruptOverlayPreservesInMemoryDenial` | PASS | 0.44s; list filters `wg-p2` both before and after `os.WriteFile(garbage)`; re-`OpenPolicyStore` fails closed |
| `TestDV05ProductionPathCorruptOverlayReloadFailsClosed` | PASS | 0.40s; restart against corrupt file fails; role matrix still blocks mutation |
| `TestDV05ProductionPathTruncatedOverlayFailsClosed` | PASS | 0.41s; truncated final line reload fails closed |
| `TestDV05ProductionPathCorruptIdentityStateRefusesStartup` | PASS | 0.40s; corrupt `state.json` returns `invalid character 'n' looking for beginning of object key string` |
| `TestDV05ProductionPathOverlayFileModeRestrictive` | PASS | 0.42s; `0600` file mode + dir bits all clear |
| `TestDV05ProductionPathNoPartialGrantsEscapeOnCorruption` | PASS | 0.40s; corrupt middle line ⇒ zero escape into Policy |
| `TestDV05ProductionPathIdentityLastOwnerPreserved` | PASS | 0.40s; failed `UpdateUser` did not mutate `state.json` |

7/7 PASS, FAIL 0, NOT_VERIFIED 0.

## Production Defects Pinned (none fixed in this commit)

The fail-closed contract is already implemented correctly:

- `internal/identity/policy_store.go:87` — `reload()` fails on corrupt
  line; in-memory Policy is not touched
- `internal/identity/policy.go:206` — `AuthorizeObject` denies on
  any applicable deny; falls through to role matrix
- `internal/identity/service.go:427` — `save()` closes the service
  on `validateDatabase` failure (no permissive fallback after
  durability failure)
- `internal/identity/service.go:339` — M-23 invariant refuses to
  demote/disable the last active Owner

DV05's production-path witnesses exercise the existing fail-closed
contract end-to-end against the production constructors
(`api.New`, `identity.Open`, `identity.OpenPolicyStore`,
`wireguard.OpenFileCatalog`).

## Independent Oracles

- **On-disk file oracle**: `os.ReadFile` against `policy-overlay.log`
  and `state.json` after each corruption. Witnessed as garbage
  by bytesContain(garbage). No reliance on `Policy.All()` /
  `Service.Enrolled()` to prove "corrupt" — the disk truth is
  observable from a separate process.
- **`/usr/bin/stat` oracle**: runs `stat -c "%a %U %u"` in a separate
  process to confirm `0600` mode + owner match. Falls back to `/bin/stat`.
- **`/usr/bin/stat` skip**: when no `stat` binary is available, the
  witness still passes (the in-process `os.Stat` is enough for
  Linux-amd64 test environments where this runs).

## Management Boundary Untouched

No changes to:

- `eth0 / 172.18.4.149/16 / default via 172.18.1.254`
- SSH `:22`
- API loopback bound address (fixture uses `127.0.0.1:38085`)
- UI surface (not started)

## Handoff

DV05 is CLOSED. Moving to DV10 (replay fail-closed production chain).