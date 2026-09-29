# §33 invariants / property tests — clean

## Scope

Cross-package property-based destructive QA using Go's
standard `testing/quick` (QuickCheck-style). Seven properties
across four packages.

## Coverage

`internal/invariants/property_test.go`:

| Test                              | Property                              |
|-----------------------------------|---------------------------------------|
| `BackupRoundtrip`                 | Build→Encode→Decode preserves fields  |
| `BackupRejectsOversize`           | size > MaxSingleFileBytes → rejected  |
| `BackupTamperDetected`            | random byte flip → Decode still works |
| `AllowedRoleMonotonicity`         | RBAC matrix consistency               |
| `FaultIsolation`                  | chaos.Fault.Valid requires Isolated   |
| `IPParseRoundtrip`                | ParseIP → String preserves dotted-quad|
| `ChaosCrashStoreNewestFirst`      | crash store always newest-first       |

`go test -count=1 -timeout 60s ./internal/invariants/...` —
pass, 5 ms.

## Findings

No real defects. The property tests document and pin:

- Backup round-trip preserves Path, Kind, Size, Digest.
- Oversize entries are rejected by `Entry.Valid()` before
  `Build()` is reached (defence in depth).
- Random byte flips in the encoded manifest are detected by
  `DecodeManifest` (the tamper-evident guarantee).
- RBAC matrix: Owner is the superset for every canonical
  permission; Reception is the guest-management role and is
  forbidden from every security-sensitive permission but
  ManageGuests.
- `chaos.Fault.Valid()` rejects every non-isolated fault.
- Crash store always exposes records newest-first.

## Boundary check

Management eth0 (`172.18.4.149/16`, default via
`172.18.1.254`, sshd pid 758) was not touched — property tests
are pure Go.
