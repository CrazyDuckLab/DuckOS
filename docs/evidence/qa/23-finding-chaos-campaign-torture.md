# §32 chaos campaign torture — clean

## Scope

Destructive QA of `internal/chaos` fault store, telemetry
pref contract, crash store retention, readiness state
machine, SSH-key store, clone protection, console catalog.

10 subtests, all pass on the existing code.

## Coverage

`internal/chaos/torture_test.go`:

| Test                              | Concern                              |
|-----------------------------------|--------------------------------------|
| `FaultIsolationContract`          | ID/kind/target/Isolated enforcement  |
| `FaultConcurrentAdd`              | race-safe Add under 64 goroutines    |
| `FaultUnknownKind`                | canonical kind list documented       |
| `TelemetryContract`               | space-rejected UA; opt-in allowed    |
| `CrashStoreRetention`             | newest-first ordering, bound enforced|
| `ReadinessStateMachine`           | every canonical state settable       |
| `SSHKeyStore`                     | round-trip + overwrite semantics     |
| `CloneProtectionContract`         | empty-class silently ignored         |
| `ConsoleCatalogContract`          | empty name rejected; register works  |

`go test -count=1 -timeout 60s -run TestTorture_
./internal/chaos/...` — pass, 2 ms.

## Findings

No real defects. The fault isolation gate (`Isolated: true`)
is the canonical DuckOS V1 invariant and is enforced in
`Fault.Valid()`. The crash store trims to its bound and keeps
the newest entries first.

## Boundary check

Management eth0 (`172.18.4.149/16`, default via
`172.18.1.254`, sshd pid 758) was not touched — these tests
are pure Go, no network or daemon involvement.
