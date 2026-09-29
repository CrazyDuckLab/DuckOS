# H09 confined restore remediation

Date: 2026-09-20. Branch: `V1 UI qualification branch`.
Parent: `5faa24ccf7e15c0069a8ee9b6cd696cf33cd69df`.
Scope: H09 only. Result: **H09 FIXED within the managed-file restore contract**.
This is not a full backend/freeze verdict. Closed findings were not re-audited.

## Production boundary

`PUBLIC HTTP → Engine authoritative admission → Agent Unix proxy → duck-restore Unix IPC → managed files`.

The root-owned standalone executable is installed at `/usr/libexec/duckos/duck-restore`,
outside the replaceable release. It accepts no executable paths, commands, hooks,
runtime root arguments or public absolute roots. Its root-only socket is
`/run/duckos-restore/restore.sock` (0600, private directory); SO_PEERCRED rejects
foreign UIDs independently of filesystem permissions.

Trusted production roots are `/var/lib/duckos/restore-source` (read-only backups),
`/var/lib/duckos/restore-live` (managed files), and `/var/lib/duckos-restore`
(private durable journal). The canonical Engine journal is not a restore target.
Requests contain only request ID, expected revision, backup ID and selected relative keys.
Only an enrolled identity with ManageBackups permission can use the public route.

The Agent and updater systemd units are unchanged. The restore unit preserves
ProtectSystem=strict, RestrictSUIDSGID, private networking and an exact write
allowlist; CAP_CHOWN only preserves the original group on managed files.
Engine's existing config directory is explicitly writable in its API unit so
restore admission is durable. No general root filesystem write access is added.

## Correctness and recovery

- Engine compares expected revision to its own current revision under the config
  writer lock. A durable request-bound reservation excludes concurrent config
  preparation and survives restart; the caller cannot provide the authoritative side.
- Descriptor-relative no-follow resolution rejects symlinks, traversal and mount
  crossings. Where systemd blocks openat2, pinned openat descriptors plus statx
  mount IDs preserve confinement, including same-filesystem bind mounts.
- Complete bounded before-images and applying intent are fsynced before any live
  write. File replacement and journal transitions use atomic rename and directory fsync.
- Failed/cancelled apply uses a fresh bounded rollback context. Rollback verifies
  restored bytes, mode and group. Uncertain persistence/undo returns recovery-required,
  not success; unsafe paths are never followed to force an apparent rollback.
- Service startup resolves interrupted writes before accepting IPC. Engine startup
  reconciles its reservation against durable receipts. Unknown admitted requests
  receive a durable rollback tombstone, preventing delayed execution after reconciliation.
- Request-ID replay is bound to the canonical request digest. Terminal receipts
  are retained; capacity exhaustion refuses new work rather than forgetting replay IDs.

## Evidence executed on Linux

All following H09 gates passed:

1. Real built `duck-restore` executable launched by systemd using the production
   sandbox and fixed managed paths, privately bind-mounted to disposable roots.
   Selective write is checked by an independent `/bin/cat` oracle. SIGKILL/restart
   preserves the receipt; changing the source afterwards cannot replay a new write.
2. Public authenticated HTTP through real Engine admission and two Unix IPC hops:
   invented revision is 409 before mutation, unauthenticated access is denied,
   caller-supplied root is rejected, selected content changes and unselected content stays intact.
3. Foreign UID 65534 cannot obtain a response even when the test socket permissions
   deliberately permit connection; an authorized root peer succeeds.
4. Child/source symlink, symlink swap, traversal, absolute/foreign root, mount escape,
   missing parent, partial multi-file failure and cancelled-request rollback regressions.
5. Cross-process SIGKILL after the first live write, automatic durable undo on reopen,
   terminal rolled-back receipt, journal-write failure and corrupt journal refusal.
6. Engine admission restart reconciliation, ordinary-writer exclusion and delayed-request tombstones.

Commands (root where namespace/systemd/foreign-UID witnesses require it):

```sh
go test -race ./internal/restore ./internal/state ./internal/engine ./cmd/duck-restore ./cmd/duck-agent ./tests/layer_l -run 'TestH09|TestDV08_PublicEngineAgentChain' -count=1 -v -timeout=180s
go test -race ./internal/state ./internal/backup -count=1 -timeout=180s
go vet ./internal/restore ./internal/state ./internal/engine ./internal/agent ./internal/api ./cmd/duck-restore ./cmd/duck-agent
go build ./cmd/duck-restore ./cmd/duck-agent ./cmd/duck-api
node --test tests/installer/install.test.mjs
```

Targeted race: PASS. Production vet/build: PASS. Installer tests: 23 PASS, 0 FAIL.
Two directly skipped helper entries are executed by their parent subprocess tests;
no required H09 acceptance witness was skipped.

Including the entire Layer-L package in vet additionally reports pre-existing
self-assignment in unchanged `dv04wg_l_test.go` at lines 53, 94 and 114. These
are outside H09 and were not modified. No full backend regression was rerun in
this final H09 session.

## Previously reproduced baseline failures (not rerun)

Existing log `/tmp/duckos-h09-baseline-failures.log`, baseline `5faa24c`:

- `internal/qa`: TestFRRRealOSPFv3Dataplane,
  TestIPv6FirewallAllowsEstablishedAndICMPv6, TestXrayVLESSRenderAndBoot.
- `tests/integration`: TestQoSLinuxApplyInNetns, TestQoSLinuxFQCoDelOnEgress.
- `tests/layer_l`: TestDV00ProductionPathRollback,
  TestDV03ProductionPathRollbackRestoresKernel,
  TestDV09_PublicEngineAgentChainAdvertise,
  TestDV09_PublicEngineAgentChainWithdraw,
  TestDV09_PublicEngineAgentChainRestartReconciles,
  TestDV09_NegativeInvalidConfig.

These remain separate from H09 acceptance. Earlier full-run Xray-dependent H05/H07
environment failures are not reclassified or retested here. No claim of a green
full backend regression is made.

## Operational limits and handoff

This service restores explicitly provisioned managed files, not arbitrary `/etc`
trees or canonical Engine state. Existing parent directories are required. Special
files, hard links, privilege-bearing modes and ACL/xattr-bearing files are rejected.
Bounds: 64 keys, 1 MiB/file, 32 MiB total before/after data, 1024 terminal receipts.
Receipt exhaustion fails closed; no automatic receipt deletion is implemented.
If an unsafe target prevents undo, the reservation stays blocked for safe recovery.
If restore IPC is unavailable during Engine startup, its reservation remains durable
and blocked rather than being silently released.

Crash evidence is process SIGKILL/restart, not physical power-loss certification.
Final read-only management check: eth0 172.18.4.149/16, default gateway 172.18.1.254,
SSH on IPv4/IPv6 port 22, unchanged. No UI or management-network changes were made.

An already-dirty `tests/layer_l/dv07_production_test.go` belongs to earlier H08
test work. It is preserved but excluded from this H09 commit and was not rerun.
