# H09 restore remediation checkpoint

Historical checkpoint, superseded by [H09 restore handoff](2026-09-20-h09-restore-handoff.md).
The separate restore service was subsequently authorized and implemented.

H09 remains open. This checkpoint is not acceptance or backend freeze.

## Completed

The restore Executor checks admission cancellation, uses a bounded apply context,
and creates a separate fresh 30-second recovery context after failed or canceled
apply. A late successful callback cannot commit after its context was canceled.
Regression covers callbacks returning cancellation and callbacks returning a
late nil result. Both must undo without inheriting cancellation or recording an
applied outcome.

Linux `go test -race ./internal/backup -count=1` and `go vet ./internal/backup`
passed. No live configuration was restored by these tests.

## Production privilege boundary requiring a decision

The production restore route currently delegates through Engine to
`BackupRestoreHost` in Agent. That host writes its configured live root directly;
the default is `/var/lib/duckos/config`. The installed Agent retains
`ProtectSystem=strict` with only `/run/duckos-agent` writable. Persistent live
files and the restore journal therefore cannot be written by that service.

The separately authorized updater accepts only update operations. Reusing its
privileges for restore would expand that security boundary. No such expansion,
Agent sandbox relaxation, or writable-root exception has been made.

A production restore writer needs explicit placement and authority before the
remaining implementation is wired. One option is a separate narrow local restore
service with fixed managed roots and authenticated IPC, coordinated with the
Engine's authoritative revision and writer serialization. It must not accept
arbitrary destination paths or bypass the Engine's current-state ownership.

## Exact remaining subcases

1. Descriptor-relative, no-follow confinement for source, destination and undo;
   reject child symlinks and swaps without touching files outside managed roots.
2. Expected revision checked against authoritative current state while holding
   the same writer serialization as ordinary configuration changes.
3. Private durable before-images and phase journal before the first live write;
   commit-journal failure cannot leave an unreported applied restore.
4. Startup recovery of interrupted transactions using durable undo, including
   partial multi-file failure and cross-process kill after the first mutation.
5. Selective successful restore and production public/Engine/IPC regressions.
6. H09 race/vet and final combined H08/H09, closed-H fallout and full backend /
   Layer-L regression, followed by management-boundary verification.

H08 was committed separately as
`9a416e60474c82f30d2f911f658b69d70b2e0cfe`. Closed findings are not reopened.
