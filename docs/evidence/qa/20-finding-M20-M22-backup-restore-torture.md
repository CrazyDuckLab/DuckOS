# §15 backup-restore torture — M-20, M-21, M-22

## Scope

Destructive QA of `internal/backup/` (envelope Build/Encode/Decode/Verify
plus logical Manifest/PlanRestore/Remap) on the `internal/backup`
package boundary. Tests are pure Go; no daemons; runs under `go test
-race -count=1`.

Three real defects were uncovered and fixed.

## M-20: PlanRestore reported `SafeToApply=true` for destructive plans

### Reproduction

```go
plan := PlanRestore("bk", true,
    map[string]bool{"a": true, "b": true},   // backup
    map[string]bool{"b": true, "c": true})  // live
// plan.Adds == ["a"]
// plan.Modifies == ["b"]    // live overwrite
// plan.Removes == ["c"]     // live data loss
// plan.RequiresSecret == true
// plan.SafeToApply == true   // BUG: operator would auto-apply
```

### Root cause

`PlanRestore` initialised `SafeToApply: true` and never downgraded
it. The function filled `Adds`/`Modifies`/`Removes` correctly but
the safety gate did not consult them. An operator that took the
"safe to apply" flag at face value would silently overwrite live
state and drop live-only keys.

### Fix

`internal/backup/restore.go`: `PlanRestore` now downgrades
`SafeToApply` to `false` whenever `Modifies`, `Removes` or
`RequiresSecret` is non-empty, and emits a matching `Impact`
marker so the operator can see *why* the plan was downgraded
(`overwrites-live-objects`, `removes-referenced-objects`,
`requires-secret`).

### Regression

- `TestPlanRestore` (pre-existing) — was the original regression
  that pinned the buggy behaviour; updated to assert
  `SafeToApply == false`.
- `TestTorture_PlanRestoreConflicts` (new) — Modifies path.
- `TestTorture_PlanRestoreSecretGate` (new) — secret path.
- `TestTorture_PlanRestoreEmptyEdgeCases` (new) — empty
  backup/live edge cases.
- `TestTorture_PlanRestoreCleanIsSafe` (new) — fresh live +
  disjoint keys.

## M-21: `pathPattern` accepted `..` traversal segments

### Reproduction

```go
e := Entry{Path: "etc/../../passwd", Kind: KindConfig, ...}
e.Valid() // returns nil — BUG
```

### Root cause

`pathPattern` (`^[a-zA-Z0-9][a-zA-Z0-9._/\-]{0,254}$`) permitted
the dot character anywhere except as the first byte, so
`etc/../../passwd` matched. The package relied on the regex to
canonicalise paths but never rejected `..`, empty segments,
leading `/` or doubled `/`.

### Fix

`internal/backup/backup.go`: added `hasPathTraversal(p string)
bool` that splits on `/` and rejects empty, `.` or `..`
segments, plus a leading `/`. `Entry.Valid` calls it after the
regex check.

### Regression

- `TestTorture_PathTraversal` (new) — 8 traversal patterns
  rejected (incl. `..`, `./`, leading `/`, doubled `/`, empty).
  Also spot-checks 3 legitimate paths still pass.

## M-22: `Remap` silently dropped invalid `RemapRequest`s

### Reproduction

```go
lm := LogicalManifest{
    BackupID: "bk", SchemaVersion: 2,
    Interfaces: []InterfaceBinding{{Logical: "wan", Physical: "eth0", Kind: "ethernet"}},
}
reqs := []RemapRequest{
    {Logical: "wan", Physical: "eth1", MAC: "00:11:22:33:44:55"},
    {Logical: "", Physical: "eth2"},      // invalid: missing logical
    {Logical: "lan", Physical: ""},       // invalid: missing physical
}
out, missing := Remap(lm, reqs)
// out.Diagnostics == nil   // BUG: invalid reqs vanish
// missing == ["lan"]       // only "lan" surfaces as missing, "eth2" silently dropped
```

### Root cause

`Remap` indexed requests by `Logical` in a map. A request with
`Logical == ""` indexed under `""`, which never matches a real
interface. A request failing `Valid()` (missing physical) was
indexed anyway and never surfaced. The operator had no way to
detect that two of their requests were malformed.

### Fix

`internal/backup/restore.go`: `Remap` now calls
`RemapRequest.Valid()` on every request, drops the failing
ones, and appends `"malformed-remap:<err>"` to
`LogicalManifest.Diagnostics`. `LogicalManifest` gained a
`Diagnostics []string` field.

### Regression

- `TestTorture_RemapRejectsInvalid` (new) — feeds three
  requests (one valid, two malformed) and asserts
  `out.Diagnostics` has 2 entries and the valid remap is
  applied.

## Coverage

`internal/backup/torture_test.go` (10 subtests, ~314 lines):

| Test                              | Concern                                |
|-----------------------------------|----------------------------------------|
| `ConcurrentBuilds`                | race-safe Build/Encode/Decode          |
| `TamperedDigest`                  | VerifyDigest rejects payload/digest    |
| `SchemaMismatch`                  | Encode+Decode reject bad schemaVersion |
| `PathTraversal`                   | `..`, `./`, `/`, `//`, empty rejected  |
| `OversizeEntry`                   | per-entry & total size caps            |
| `TooManyEntries`                  | `MaxEntries` cap                       |
| `UnknownKind`                     | kind whitelist enforced                |
| `PlanRestoreConflicts`            | Modifies → unsafe                      |
| `PlanRestoreCleanIsSafe`          | disjoint fresh adds → safe             |
| `PlanRestoreSecretGate`           | requires-secret → unsafe               |
| `PlanRestoreEmptyEdgeCases`       | empty backup/live boundaries           |
| `RemapRejectsInvalid`             | diagnostics for malformed requests     |
| `RandomEntriesRoundtrip`          | 64 random entries + tamper detection   |

`go test -race -count=1 ./internal/backup/...` — PASS, 1.14s.

## Boundary check

Management eth0 (`172.18.4.149/16`, default via `172.18.1.254`,
sshd pid 758) was not touched — these tests are pure Go, no
network.
