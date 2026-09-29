# H08 independent updater supervisor — implementation progress

H08 supervisor remediation is implemented and its scoped Linux regressions pass. This is not backend freeze: H09 and the combined regression gate remain outstanding.

## Implemented

- Independent `cmd/duck-updater`, installed unit definition targeting `/usr/libexec/duckos/duck-updater`, outside `/opt/duckos/current`.
- Production Agent uses a Unix-only proxy. It no longer opens the writable release store in its production startup path. Agent systemd sandbox is unchanged.
- Root-only socket with Linux peer-credential verification; bounded typed stage/activate/rollback/status operations; no request-provided executable paths, URLs, units or shell commands.
- Supervisor authenticates signed packages through the existing upgrade verifier, prepares immutable native candidates, rejects archive escape/link/special entries, verifies native `--version`, and rechecks signed content before selecting a release.
- Committed Active remains the durable before-image while a candidate is provisional. Failed health restores the previous serving release using an independent context. Failed first activation clears selection. Startup pending recovery restores the committed release instead of promoting the candidate.
- Fixed systemd driver restarts only DuckOS Agent/API. Its health implementation compares actual `/proc/<MainPID>/exe` digests with selected executables and checks authenticated API readiness plus running build version/commit.
- Build/installer provision the immutable supervisor executable and unit without weakening Agent permissions. Trust provisioning is explicit. Signed installed-baseline enrollment verifies installed bytes and live build before establishing a rollback target; it does not replace files.
- The readiness response includes build identity from the running API executable.

## Executed evidence

- PASS: native ELF signed candidate preparation and real process execution; independent live Unix-socket version/build oracle; failed first activation; failed-next-candidate rollback; reopen with stopped runtime and automatic restart.
- PASS: signed malicious archive paths and invalid executable refusal; tampered signature/payload refusal.
- PASS: separate supervisor test process killed together with its fixture process group after candidate selection/start; startup restores the previous native runtime and independently verifies its build.
- PASS: real Unix listener accepts root, rejects uid 65534 using SO_PEERCRED even with socket filesystem permissions deliberately relaxed by the test. Production permissions remain 0600/0700.
- PASS: public API → Engine → real Agent Unix handler → updater Unix listener → production systemd driver → native fixture services in a disposable network namespace. Independent MainPID/digest and authenticated live readiness/build oracle passed. First-ever unhealthy activation left no Active; failed next candidate restored the prior release; explicit Previous rollback passed.
- PASS: signed installed-baseline enrollment and startup reconciliation regression.
- The temporary systemd unit names were used only after confirming both were absent; their files and services were removed afterward. No pre-existing DuckOS service was replaced. Host eth0/default route remained unchanged.
- PASS: targeted native supervisor regressions under race; vet of upgrade/Agent/API/composition; updater and Agent compilation.
- Helper-only subprocess tests skip when called directly and execute through their parent regressions.

## Final scoped verification

- PASS: native process SIGKILL/restart at provisional selection before start, after start, and after independent health. Each restart restores the committed Active runtime; no provisional candidate is promoted.
- PASS: accepted HTTP activation survives request cancellation; supervisor owns its independent operation lifetime.
- PASS: late cancellation cannot turn health into a successful activation; rollback gets a fresh context. Failed rollback remains pending/recovery-required and completes on restart.
- PASS: journal rename failure before selection produces no live process or selection changes.
- PASS: installer artifact/security regression: 23 passed, zero failed.
- PASS: H08 native tests under race, including all three subprocess crash boundaries; targeted API/Agent health/update tests under race; relevant vet and updater/Agent build.
- Tests use signed native fixture releases, including real systemd execution in an isolated network namespace. They do not claim a physical power-cut test or a deployed fleet upgrade. Public runtime execution and restart evidence are distinct from artifact integrity checks.

## Remaining remediation

H09 remains untouched: confinement/symlink safety; authoritative revision; durable before-image/undo; fresh rollback context; interrupted restore recovery; crash/restart and selective restore regressions.

After H09: closed-H fallout regressions, full backend/Layer-L regression, applicable race/vet/build, management boundary and final remediation handoff.
