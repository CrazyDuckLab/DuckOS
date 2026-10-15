# Beta.3 remediation — qualification evidence

Branch: `fix/beta3-release-blockers` (based on frozen `41921c3`).
Status: code remediation complete; Linux qualification **blocked** at the
time of this revision (see §5).

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

## 3. Linux-native qualification

**BLOCKED.** Both laboratory VMs (`duck-beta-01` 172.18.4.102/…82,
`duck-dev-01` 172.18.4.159) are unreachable (SSH and ICMP) from the
management workstation; the surrounding subnet is alive, so the VMs are
powered down or the Hyper-V host is offline. Creating or restoring VMs is
outside this task's authorization. No container runtime exists on the
workstation, so no substitute Linux environment is available.

Prepared and committed, awaiting a Linux host: the adapter regression suite
(`internal/agent/fw_host_verify_test.go`) executes with
`GOOS=linux go test ./internal/agent/`; live nftables verification of the
JSON parser fixtures (`prio`, payload/meta expression shapes) against real
`nft -j` output on Debian 13 is the first step of the runbook below.

### Runbook for the laboratory run (when VMs return)

1. Baseline: hostname, interfaces, addresses, routes, `nft list ruleset`,
   DuckOS service state; record eth0 management address.
2. Run `go test -race ./...` on the branch inside the VM.
3. Compile and run a small harness that applies `fwrules.Render` batches
   with real `nft`, captures `nft -j list table inet duckos_fw`, and checks
   `VerifySnapshot` pass/divergence/empty/rollback paths — inside dedicated
   network namespaces only; never on the management forwarding path.
4. Coexistence: `inet duckos`, `inet duckos_fw`, `inet duckos_upnp` in one
   ruleset; delete a UPnP mapping, confirm static firewall rules survive;
   roll back a firewall change, confirm active mappings survive.
5. Verify management eth0 state equals the baseline after every phase.

## 4. Release readiness

Beta.3 must not be released until the blocked Linux qualification
(§3) completes on the existing laboratory VMs and the parser fixtures are
confirmed against real nftables output. Code-level blockers A–D are fixed
and covered by regression tests.
