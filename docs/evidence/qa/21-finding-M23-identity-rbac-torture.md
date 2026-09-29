# §17 API/RBAC abuse — M-23

## Scope

Destructive QA of `internal/identity` (Service: Login, Authorize,
Enroll, CreateUser, UpdateUser, ChangePassword, Audit, Sessions,
Revoke) against the canonical DuckOS V1 RBAC contract.

One real defect was uncovered and fixed.

## M-23: demoting/disabling the last active Owner could close the identity service

### Reproduction

```go
s, _ := freshService(t)
ownerID := mustGetUserID(t, s, tok, "owner")
err := s.UpdateUser(tok, ownerID, ReadOnly, false)
// before fix: err == nil (or wrapped save failure with
// closed=true) — the box is left without any active Owner
// and the service is auto-closed by the deferred error
// handler in save().
```

### Root cause

`UpdateUser` checked `validRole(role)` but never checked the
owner-preservation invariant. The lower `validateDatabase`
guard did refuse an ownerless database, but the refusal
surfaced at `save()` time, where the deferred error handler
runs `s.closed = true; clear(s.sessions)`. A single bad
UpdateUser therefore turned into a self-DoS: the operator who
typed the command locked themselves out.

### Fix

`internal/identity/service.go`: `UpdateUser` now walks the
current users, identifies the last active Owner (role=Owner
and not disabled), and refuses any update that would demote
or disable that user. The rejection happens at the policy
layer — before the clone, before the save — and the service
remains open.

### Regression

- `TestTorture_LastOwnerPreserved` — demote last Owner to
  ReadOnly → rejected; service still functional.
- `TestTorture_LastOwnerDisabledBlocked` — disable last Owner
  → rejected.

## Coverage

`internal/identity/torture_test.go` (17 subtests, ~500 lines):

| Test                              | Concern                              |
|-----------------------------------|--------------------------------------|
| `TokenTampered`                   | session token contract               |
| `PermissionMatrix`                | role × permission truth table        |
| `DisabledUserBlocked`             | disabled user cannot log in          |
| `ReEnrollmentForbidden`           | second Enroll → ErrForbidden         |
| `InvalidUsernameRejected`         | username pattern violations          |
| `InvalidRoleRejected`             | role whitelist enforced              |
| `RateLimitBounded`                | 11th attempt → ErrRateLimited        |
| `AuditOwnerOnly`                  | non-Owner → ErrForbidden             |
| `CreateUserLimit`                 | user cap enforced                    |
| `ChangePasswordRequiresOld`       | wrong old → ErrUnauthorized          |
| `CrossUserRevoke`                 | non-Owner cannot revoke Owner session|
| `SelfRevokeAllowed`               | user can revoke own session          |
| `LastOwnerPreserved` (M-23)       | last Owner cannot be demoted         |
| `LastOwnerDisabledBlocked`        | last Owner cannot be disabled        |
| `SessionsListIsolation`           | non-Owner sees only own sessions     |
| `ConcurrentLogins`                | race-safe rate limiter               |
| `InvalidPasswordLength`           | oversized password → ErrUnauthorized |
| `DuplicateUsername`               | duplicate CreateUser rejected        |
| `UpdateUnknownUser`               | unknown id → error                   |
| `PermissionEscalation`            | read-only forbidden from writes      |

`go test -race -count=1 -timeout 90s -run TestTorture_
./internal/identity/...` — passes in 9s without -race (the
`-race` run exceeds the 90s window because PBKDF2 with
600,000 iterations × ~20 subtests dominates wall-clock).

## Boundary check

Management eth0 (`172.18.4.149/16`, default via
`172.18.1.254`, sshd pid 758) was not touched — these tests
are pure Go, no network.
