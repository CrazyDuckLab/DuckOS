# High remediation progress

Branch: `V1 UI qualification branch`.

## Completed

- H01/H02/H04/H05/H06/H07: previously committed fixes, not reassessed here.
- H10 (`87399b7`): native FRR validation, durable intent and startup reconciliation. Production public-API test independently observed advertise/withdraw through `vtysh show ip bgp` in an isolated namespace/pathspace.
- H03 (`2268e96`): durable tc ownership journal, before-state observation, cancellation-safe rollback, explicit recovery-required handling and serialized controller mutations. The production public-API regression retained external `pfifo 77: limit 2345`, rolled back partial execution, verified restart/explicit rollback, and killed a separate process after real tc mutation before reopening the journal. Uncertain runtime ownership is not automatically deleted.

## H08 — partial, NOT FIXED

The mutation mutex is separate from the snapshot mutex, so a health reader can call `State` without deadlock while competing mutations remain serialized. Failed first activation clears the durable Active selection and retains the candidate for diagnosis. Selection-only history no longer proves runtime health; the unwired production runtime provider fails closed.

Still required: actual signed candidate execution through the production activation mechanism; independent running build/version/health evidence; durable phase journaling; startup reconciliation against actual runtime; previous-release runtime restoration; invalid-executable, startup-health-failure and interrupted-activation production regressions. This is a software blocker, not a hardware-only limitation.

### Durable workflow continuation

Production composition now opens a single-writer durable workflow instead of the previous no-op state restoration. Candidate, policy and history are fsync/rename persisted. The workflow persists an interruption marker before selecting a release. Corruption, an unjournaled Active selection and stale/closed writers fail closed. Reopening a prior Active or interrupted activation explicitly requires runtime reconciliation; metadata is not a runtime witness. A failed first activation leaves Candidate available for diagnosis and no Active selection after reopen.

Additional verified regressions:

- Real public API → Engine → Unix Agent → production upgrade composition → ReleaseStore: signed stage, service reopen, refused activation, second reopen. PASS. The signed payload is deliberately non-executable; this test does **not** claim executable activation coverage.
- Separate process killed after the durable activation marker and actual Active symlink replacement: reopen returns recovery-required and refuses further mutations. PASS. This is interruption detection, **not** automatic runtime rollback.
- Corrupt workflow retained without resetting evidence; second writer denied; unjournaled selection rejected; closed writer denied. PASS.
- Targeted upgrade/Agent race and vet for upgrade/Agent/composition. PASS.

Exact remaining H08 subcases: production signed-payload execution; independent running build/version/health oracle; failed startup cleanup and Previous runtime restoration with fresh contexts; durable runtime phase outcomes; startup reconciliation against actual running services; automatic crash/restart recovery across execution phases; production regression of those paths. H08 remains NOT FIXED. H09 remains untouched in this continuation.

## H09 — NOT FIXED

Still required: descriptor-relative no-follow restore confinement; authoritative revision validation under serialization; durable before-images before effects; startup recovery of interrupted restore; fresh bounded rollback contexts; production symlink/revision/selective-restore/partial-failure and kill/restart regressions.

## Verification in this continuation

- PASS: `go test -race ./internal/qos ./internal/agent -run 'Owned|Controller|QoS|LinuxRunner'`.
- PASS: real Linux `TestH03ProductionOwnershipAndCrashRecovery`, including its isolated child and separate killed process.
- PASS: `go vet ./internal/qos ./internal/agent ./cmd/duck-agent`.
- PASS: `go test -race ./internal/upgrade`.
- PASS: `go test -race ./internal/agent -run 'H08|Upgrade|Orchestrator'`.
- PASS: `go vet ./internal/upgrade ./internal/agent`.
- One invocation used an invalid timeout flag; it did not run the privileged regression and was rerun with `-timeout=120s`.

Before/after the H03 privileged group, eth0 remained `172.18.4.149/16`, with default route `172.18.1.254 dev eth0`; the same SSH transport completed the checks. No host management-network mutations were performed.

Exact remaining H-list: **H08, H09**. Backend freeze readiness is not claimed.
