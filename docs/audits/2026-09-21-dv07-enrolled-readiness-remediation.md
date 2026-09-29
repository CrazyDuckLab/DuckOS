# DV07: enrolled-runtime readiness authentication

## Scope

Single-High remediation of updater readiness authentication after identity
enrollment. Backend baseline: `182eb6462964538bf7895668deb73248ab2d428b`.
Commit parent: `ab4c85d02aecb11d7594698baa8c15748da067cb`; the three intervening
UI-only commits are preserved without changes. No full backend regression or
DV00–DV10 re-audit is included in this pass.

## Production change

The updater no longer uses the installation bootstrap token for health checks.
It signs a short-lived Ed25519 assertion with
`/etc/duckos/updater-health.key` (root-owned, mode 0600). The API receives only
`/etc/duckos/updater-health.pub` (root-owned, mode 0644). Assertions authorize
only local `GET /api/v1/ready`, with no query or body, for 15 seconds. They do
not authorize management operations or issue user sessions. Enrollment still
disables bootstrap-token access; ordinary session authorization is unchanged.
Wrong, tampered, expired, future, duplicate-header and non-loopback assertions
fail closed. Readiness still evaluates the Engine, and the updater independently
checks running executable digests and runtime build identity.

The installer provisions the key pair once with
`/usr/libexec/duckos/duck-updater --init-health-key`. Existing files are never
overwritten. Existing development installations require this root-only
provisioning step and the updated API service argument before using the new
updater. Both keys must be provisioned before service activation; interrupted
provisioning requires explicit operator recovery, not automatic key replacement.
Neither the Agent sandbox nor updater service permissions are expanded.

## Regression witness

`TestH08PublicSystemdSupervisor` runs signed native candidates through real
systemd units in an isolated network namespace. Candidate services use the
production API Handler, persisted enrolled identity, Engine and Agent observer;
the old bootstrap-only readiness stub is not used for this witness.

The witness covers failed first activation without false Active, healthy
activation, unhealthy candidate rollback to Previous, explicit Previous rollback,
independent runtime verification and reopening the supervisor against the
enrolled running release. Failure injection returns unhealthy status without
changing production authorization. Existing interrupted-workflow and crash
recovery regressions remain in the targeted upgrade package.

The units and temporary files are fixture-owned and cleaned up. No installed
DuckOS service is replaced: the witness refuses to run if those units exist.

## Verification results

Linux checkout: `/tmp/duckos-health-remediation.EKi27W`, backend baseline plus
the exact remediation sources (UI-only changes are outside these checks).

- Production `TestH08PublicSystemdSupervisor`: PASS, including enrolled healthy
  activation, failed first activation, unhealthy rollback and startup reopen.
  Final run: 15.207 seconds.
- `go test -race ./internal/healthauth ./internal/api ./internal/identity
  ./internal/upgrade -count=1 -timeout=300s`: all four packages PASS. Existing
  helper/environment skips are not counted as production evidence.
- Targeted `go vet` and `go build` for API/updater/Agent and affected packages:
  PASS.
- `node --test tests/installer/install.test.mjs`: 23 PASS, 0 FAIL, 0 SKIP.
- `git diff --check`: PASS.

Additional systemd witness attempts with the entire test harness instrumented
by `-race` FAILED twice at the first public package-stage request with HTTP 503
`agent_unavailable`; no race report was emitted. Both stopped before candidate
activation. Stripping fixture debug symbols, as production builds do, did not
resolve this. The ordinary production witness passed again afterward. Instrumented
large-payload processing exceeding the existing transport deadline is suspected,
not independently proven. This additional race witness is NOT reported as PASS;
no production timeout, authorization check or acceptance assertion was relaxed.
The failed instrumented witness remains a verification limitation for recheck.

Management boundary after all runs: `eth0 172.18.4.149/16`, default route
`172.18.1.254 dev eth0`, SSH listening on IPv4/IPv6 port 22. Both fixture service
units report `not-found` after cleanup. Agent/updater sandbox settings unchanged.
