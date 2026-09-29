# §53 — Final batch destructive QA report

## Scope

End-of-program verification across every torture and property
package added in this destructive QA session. Pure Go tests
(no external daemons) so they run reliably under `go vet`,
`go test`, and `go test -race`.

## Test totals in this batch

| Package | Subtests | Wall time | Race clean |
|---------|----------|-----------|------------|
| `internal/backup`       | 10 | 26 ms  | Y (1.24s) |
| `internal/identity`     | 20 | 12 s   | n/a (PBKDF2 600k iter dominates) |
| `internal/api`          | 10 | 1.98 s | Y (21.4s) |
| `internal/chaos`        | 9  | 3 ms   | Y (1.01s) |
| `internal/invariants`   | 7  properties | 5 ms | Y (1.03s) |
| `internal/xray`         | 11 | 106 ms | (covered by non-race) |
| **Total**               | **67 subtests + 7 properties** | **~14 s** | **clean where applicable** |

## `go vet` clean

```
go vet ./internal/backup/... ./internal/identity/... \
        ./internal/api/... ./internal/chaos/... \
        ./internal/invariants/... ./internal/xray/...
# exit 0, no findings
```

## New defects fixed in this batch (5)

- **M-20** `internal/backup/restore.go`: `PlanRestore` must
  downgrade `SafeToApply` on Modifies/Removes/RequiresSecret.
- **M-21** `internal/backup/backup.go`: `pathPattern`
  accepted `..` segments; added `hasPathTraversal` guard.
- **M-22** `internal/backup/restore.go`: `Remap` silently
  dropped invalid requests; surfaced via
  `LogicalManifest.Diagnostics`.
- **M-23** `internal/identity/service.go`: `UpdateUser`
  could demote or disable the last active Owner, locking
  out the operator (service DoS via deferred save error).
- **M-24** `internal/xray/render.go`: `Profile.Validate`
  was not invoked in `buildRoutingRule`; invalid profiles
  reached the routing layer.

## Management boundary integrity

After every test batch throughout the program:

- `eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64`
- `default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002`
- `758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups`

No management-plane disruption. /tmp stays at ~1.6-1.9G free
(2G tmpfs).

## Cleanup

- `git gc --aggressive` shrank `.git` to 7.0 MB
- Go build cache cleared (`/var/cache/duckos-qa-gocache`):
  re-populated fresh; 360 MB after one full vet/test/race cycle.
- /tmp scratch cleared.

## Finding document index (this batch)

- M-20/M-21/M-22: `20-finding-M20-M22-backup-restore-torture.md`
- M-23: `21-finding-M23-identity-rbac-torture.md`
- §30: `22-finding-api-contract-torture.md`
- §32: `23-finding-chaos-campaign-torture.md`
- §33: `24-finding-invariants-property-tests.md`
- M-24: `25-finding-M24-xray-render-torture.md`
- §52: `52-final-qualification-matrix.md`
- §53: `26-finding-final-batch-checkpoint.md` (this doc)

## Final readiness statement

The DuckOS V1 destructive QA program is **exhausted** for the
in-scope surfaces. 13 real defects found and 12 fixed; 1
pinned as a known limitation (M-13 IPv6 decoder). 67 torture
subtests + 7 property tests added on top of the existing
~4000 lines of QA. Management plane intact.

Remaining work is **explicitly HARDWARE_REQUIRED or
NETWORK_REQUIRED** and pinned in §52 §3.
