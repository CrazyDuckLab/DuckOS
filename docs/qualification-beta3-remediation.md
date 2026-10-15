# Beta.3 remediation — qualification evidence

Branch: `fix/beta3-release-blockers` (based on frozen `41921c3`).
Status: code remediation complete; Linux qualification **executed** on the
existing laboratory VMs (2026-10-15, see §3/§3a).

## 1. Release blockers and their resolution

| # | Codex finding | Resolution | Commit |
|---|---------------|------------|--------|
| A | `DecodeSnapshotObjects` rejects `inet duckos_fw` snapshots because `network.DecodeNFTSnapshot` validates only `inet duckos` | `network.DecodeNFTSnapshotForTable(raw, table)` parameterizes the owned-table contract; the network decoder is unchanged and stays scoped to `duckos`; `fwrules.DecodeSnapshotObjects` decodes strictly within `inet duckos_fw` (fail-closed on foreign tables/chains/rules, malformed JSON, unknown fields) | `60e5ec9` |
| B | `Render` of an empty ruleset creates table+forward chain while `Verify` expects no objects | Canonical empty state is "owned table absent": `Render` of a ruleset with zero enabled rules emits an idempotent `add table`+`delete table` batch; `Render`/`Apply`/readback/`Verify` agree | `60e5ec9` |
| C | `readbackFwrTable` suppresses decoding errors (nil = absent = error) | Readback now returns `(objects, absent, error)`; a nonzero nft exit counts as "absent" only when nft itself reports the missing table; decode failures and other execution failures are errors and are surfaced by `Verify` and by `GET /fwrules/v1/observed` (HTTP 502 `observation_failed`) | `60e5ec9` |
| D | Verification compares object counts only | `fwrules.VerifySnapshot` compares the full semantic state: table ownership, single forward chain with `type filter / hook forward / prio 0 / policy accept`, per-rule action, protocol, src/dst CIDR, in/out interface, dst port, ordering, and the `duckos_fw:<id>` ownership comment; missing, extra, reordered or diverging rules all fail | `60e5ec9` |

Additional remediation on the same branch:

- Release version identity: `DUCKOS_RELEASE_VERSION` (semver tag bound to
  the exact source commit; clean tree enforced; mismatched/missing tag
  rejected) via `scripts/version.sh` (`8b9af4d`). Development builds are
  unchanged.
- Pre-existing prettier violation in `web/src/configurators/firewall.tsx`
  fixed (`6cac0de`).
- Linux adapter regression tests with injectable readback
  (`test(agent)` commit) — require a Linux host to execute.

## 2. Automated source qualification (macOS arm64, Go 1.27.1, Node 24.20.0)

- `gofmt -l cmd internal pkg packaging tests` — clean.
- `go vet ./...` (GOOS=linux) — clean. Note: `internal/agent` and its
  dependents do not compile on darwin (linux-only files) — pre-existing at
  `41921c3`, not a regression.
- `go test -race -count=1` on all darwin-buildable packages — pass; the
  remaining packages (agent, api, engine, integration, duck-agent/
  duck-restore/duck-updater/duck-api) must execute on Linux (blocked).
- Frontend: `tsc --noEmit`, prettier check, vitest 193/193, production
  build — pass.
- `node scripts/schema.mjs --check`, `go mod verify`, `git diff --check` —
  pass.
- Installer: `node --test tests/installer/*.test.mjs` — 31/31 (previously
  30/1; the single failure was a missing `dist/duckos-linux-amd64/SHA256SUMS`
  caused by absent build preparation in the isolated worktree, not an
  installer defect — resolved by running the official build).
- Shell: `tests/installer/version.test.sh` — 8/8; `bash -n scripts/*.sh`.
- Architecture builds: amd64, arm64, armv7 produced via
  `bash scripts/build.sh all`; ELFs verified (`file`): x86-64, aarch64,
  ARM EABI5, statically linked; SHA256SUMS present in each artifact.

## 3. Linux-native qualification (2026-10-15)

Executed on the existing laboratory VMs after they returned:

- duck-beta-01 — Debian 13.7, kernel 6.12.107+deb13-amd64, amd64, nftables
  1.1.3, Go 1.27.1; eth0 172.18.4.82/16 via 172.18.1.254 (DHCP); DuckOS
  beta.2 install running (duck-agent/duck-api/launchers active). Baseline
  captured before any test; eth0, default route, SSH and host nftables
  ruleset unchanged after every phase (host `nft list tables` stayed empty).
- duck-dev-01 — Debian 13.6, kernel 6.12.107+deb13-amd64, amd64; eth0
  172.18.4.159/16; used as secondary verifier for the core suites.

Source transferred via `git archive` of the branch HEAD; no VM reboot, no
reinstall, no host networking changes; all nftables qualification ran
inside dedicated network namespaces (`fwq-*`), cleaned up afterwards.

### 3a. Results

Go suites, `go test -race -count=1 ./...` on duck-beta-01: **122 packages
ok**; failures (all pre-existing at the frozen `41921c3`, verified by
re-running the base source, or documented witness behaviour):

- `TestH08PublicSystemdSupervisor` + `TestDV07_*` (cmd/duck-agent,
  tests/layer_l): refuse to install/replace over the INSTALLED DuckOS —
  correct fail-closed behaviour; they require a bare host (BLOCKED by
  design on these machines).
- `TestNATPMPUDP*` (internal/upnp/frontend): loopback UDP listener gets no
  response on BOTH VMs and at `41921c3` — environmental, not a regression;
  the same tests pass on the macOS build host.

duck-dev-01 (as root via sudo): internal/agent, fwrules, network,
safeapply — ok.

Live nftables suite (`DUCKOS_LIVE_NFT=1`, inside `fwq-*` namespaces) —
**all PASS**:

- `TestLiveApplyVerifyExact` — apply + exact semantic verify against the
  real kernel (chain props, matches, verdicts, ordering, comments).
- `TestLiveVerifyRejectsForeignDrift` — out-of-band rule change with the
  same rule count is rejected (count-only verification would pass).
- `TestLiveEmptyCandidateRemovesTable` — empty candidate leaves the owned
  table absent; Verify agrees.
- `TestLiveRestoreKnownGood` — restore to prior non-empty and to empty.
- `TestLiveReadbackAbsentClassification` — absent vs error classification.
- `TestLiveSafeApplyLifecycle` — journal-first pipeline on the real
  kernel: admit → apply → confirm; explicit rollback; genuine process
  death (child process exits without confirming) followed by journal
  reopen → pending transaction expired, known-good baseline restored;
  automatic rollback after confirm-window expiry.

Live qualification fixed real defects found only against the kernel
(committed): nft 1.x JSON shapes (rule-level comments, bare `{"drop":null}`
verdicts, `{"prefix":{addr,len}}` values) now parsed by `VerifySnapshot`;
first-boot store creation in `NewFirewallRulesService` (the service could
never create its journal directory on a fresh host); several linux-only
test defects (dhcp 403 stub, WAN fixture gateway prefix, service journal
directory).

Packet-level qualification (`tests/integration/fw_live_netns.sh`,
namespaces fwq-qual/fwq-cli/fwq-srv) — **all PASS**: baseline forward,
accept-policy behaviour, drop-rule blocking, coexistence (UPnP DNAT
mapping survives firewall rollback and vice versa; mapping deletion
preserves firewall rules), canonical empty state (table absent, forwarding
resumes), first-rule insertion on a clean table.

Artifacts rebuilt at final HEAD `9b82fafefcdc`
(`1.0.0-dev.9b82fafefcdc`), `--verify-artifact` PASS for amd64/arm64/armv7.

### Known environment-blocked items

- Witness tests (above) need a bare Debian host — not authorized here.
- NAT-PMP loopback UDP on both VMs — environmental, pre-existing.
- ARM64/ARMv7 remain artifact-verified only (no live hardware in scope).

## 4. Release readiness

Code blockers A–D are fixed and live-qualified on Debian 13 with real
nftables 1.1.3, including safeapply confirm/rollback/crash-recovery and
cross-table coexistence. Remaining before release: independent Codex audit
of `fix/beta3-release-blockers`; witness-class tests on a bare host and
the NAT-PMP loopback anomaly are environment items, not code blockers.
