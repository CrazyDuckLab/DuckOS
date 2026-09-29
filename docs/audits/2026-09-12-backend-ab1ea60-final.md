# DuckOS V1 Backend Final Handoff — Linux L-Tier Remediation

- **Date (UTC)**: 2026-09-12
- **REMEDIATION_BASE_SHA**: `903a4cd` (Mac/S-tier baseline)
- **Final HEAD**: `ab1ea60` `fix(dv-runners): auto-fetch go 1.27 toolchain on Linux duck-dev-01`
- **Branch**: `V1 UI qualification branch`
- **Remote**: `ssh://duck-dev-01/tmp/duckos-test/repo`
- **Working tree**: clean (0 dirty files)

## 1. Environment

- **OS / kernel / arch**: Debian GNU/Linux 13 (trixie), kernel 6.12.107+deb13-amd64, x86_64
- **Host**: `duck-dev-01` (Hyper-V, VHDX backup taken + checkpoint before the run)
- **User**: `duckdev` (passwordless sudo, escalation used only inside netns / wg / frr / xray fixtures)
- **Go toolchain**: `go1.24.4` system; project pins `go 1.27.1` in go.mod → harness sets `GOTOOLCHAIN=auto` so the pinned toolchain is fetched on demand into `~/sdk/`
- **Test runner**: `scripts/test-backend-acceptance.sh --all --evidence /tmp/duckos-evidence`

### Dependency versions (dpkg / binaries)

| Tool                | Version                                  |
|---------------------|------------------------------------------|
| iproute2 (`ip`,`tc`)| 6.15.0-1 (Debian)                        |
| wireguard-tools     | 1.0.20210914-3                           |
| nftables            | 1.1.3-1                                  |
| FRR (zebra/bgpd/vtysh) | 10.3-3+deb13u1                        |
| Xray (synthetic)    | 1.8.20 (8deb953, go1.22.5, statically linked, downloaded from GitHub releases) |
| flock               | util-linux 2.41.5                        |
| systemd             | active, `sshd` unit listening on :22     |
| Go                  | 1.24.4 system + auto-fetched 1.27.1 toolchain |

No new packages were installed during the remediation; all required tools were already present on the pre-instrumented VM.

## 2. Management boundary — before & after

Asserted with the exact commands the contract requires:

| Check                         | Before                                         | After                                          |
|-------------------------------|------------------------------------------------|------------------------------------------------|
| `ip -br a`                    | `eth0 UP 172.18.4.149/16` + `lo UNKNOWN 127.0.0.1/8` | identical                                       |
| `ip route`                    | `default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149` + `172.18.0.0/16 dev eth0` | identical                                       |
| `ss -ltnp | grep :22`         | LISTEN on `0.0.0.0:22` and `[::]:22`           | identical                                       |
| `systemctl is-active ssh`     | `active`                                        | `active`                                        |

**Verdict**: management boundary intact. No eth0 / 172.18.4.149 / default-route / SSH drift.

## 3. Tally

```
PASS         = 56
FAIL         =  0
NOT_VERIFIED =  0
SKIP         =  0
```

Per-DV breakdown:

| DV          | tier-S PASS | tier-L PASS | FAIL | NOT_VERIFIED |
|-------------|-------------|-------------|------|--------------|
| DV00        | 1           | 1           | 0    | 0            |
| DV01        | 2           | 2           | 0    | 0            |
| DV02        | 4           | 2           | 0    | 0            |
| DV03        | 1           | 4           | 0    | 0            |
| DV04-WG     | 1           | 3           | 0    | 0            |
| DV04-XRAY   | 1           | 3           | 0    | 0            |
| DV05        | 5           | 0           | 0    | 0            |
| DV06        | 5           | 0           | 0    | 0            |
| DV07        | 2           | 3           | 0    | 0            |
| DV08        | 2           | 4           | 0    | 0            |
| DV09        | 1           | 3           | 0    | 0            |
| DV10        | 5           | 1           | 0    | 0            |
| **Total**   | **29**      | **27**      | **0**| **0**        |

## 4. Status per DV

Every subcase from the fixed backend acceptance manifest (`scripts/test-backend-acceptance.sh --list`) was executed with real evidence. Per-DV machine-readable evidence is in `/tmp/duckos-evidence/DV*/results.jsonl` and per-test `go test` logs in `/tmp/duckos-evidence/DV*/test-*.log`.

- **DV00** — production composition wiring anchors verified by `grep`; runtime oracle mutation observed by independent kernel interface (dummy0 / netns).
- **DV01** — every required V1 kind has a real source-backed lister (objectlisters package green); per-row RBAC envelope in `internal/api`; restart preserves identity in netns fixture; disabled daemon surfaces typed unavailable.
- **DV02** — engine idempotency, duplicate/payload-conflict refusal, stale-revision refusal, confirmation-deadline guard (S); preview/apply/confirm on disposable netns + Engine/Agent cross-process kill after admission (L).
- **DV03** — qos cancellation-and-truth guard (S); tc qdisc observable by independent oracle, partial apply does not commit, rollback failure surfaces recovery-required, denied apply produces zero tc mutations (L).
- **DV04-WG** — wireguard export-permission (S); real kernel `wg0`-style interface CRUD via `wg(8)`, restart does not resurrect deleted peer, redacted peer export contains no `PrivateKey` line (L).
- **DV04-XRAY** — incompatible protocol/security rejected (S); `xray run -c <cfg> -test` validates VLESS+TCP and rejects malformed config; subscription round-trips exact issued URL; restart does not resurrect deleted user (L).
- **DV05** — identity policy: wildcard-deny precedence, exact-deny beats wildcard-allow, PolicyStore persists + corrupt lines skipped (no permissive-empty fallback), denied export produces zero resolver calls, cmd/duck-api wires OpenPolicyStore + EnablePolicy.
- **DV06** — every `cmd/duck` command reaches `/api/v1/...`; no `--actor` flag; no interactive stdin read on non-TTY.
- **DV07** — corrupt/short/unsigned/wrong-arch refused (S); signed package stage/activate + build-marker change, health failure recovers known-good, kill at materialize/anchor/migration/service/health boundaries all green (L).
- **DV08** — corrupt archive refused + ENOSPC/rename never returns committed (S); portable+selective restore across different NIC names, selected/unselected comparison, cancelled apply restores B before-state, kill at snapshot/admission/partial apply/verify/commit reconciles (L).
- **DV09** — missing source surfaces not-advertised (S); netns-based route advertise / withdraw / health-failure-unadvertise / restart-reconciles-actual (L).
- **DV10** — production key loader (S); authentic issue/redeem, replay refusal, capacity fail-closed at MaxRedemptionJournal=256, corrupt-line skip (S); `flock(2)` inter-process exclusion + restart durability via Linux L-tier (L).

## 5. Commits from Linux remediation (since baseline `903a4cd`)

Linux-specific commits (coherent, minimal, each tied to one or more DV subcases):

```
ab1ea60 fix(dv-runners): auto-fetch go 1.27 toolchain on Linux duck-dev-01
c2b66b8 fix(dv-runners): detect Go toolchain + cache dirs per-host
b40fc1f fix(dv-runners): wire L-tier tests/layer_l invocations into Linux runs
b77831c fix(dv09): create dummy0 in health-failure + restart tests too
072786a fix(dv09): bring dummy0 up so device-attached routes can be added
2355052 fix(dv09): use dev dummy0 for routes (no gateway needed)
dc64e75 fix(dv04wg/dv09): short iface name + lo-up inside netns (not move from init)
9ed6080 fix(dv04wg/dv04xray/dv09): host ns for kernel WG, xray run -test, pre-clean netns
aa69f25 fix(ztp): flock(2) inter-process exclusion + Close method
```

Plus the prior Mac-side remediation chain (kept for context): `ac47966`, `74ecc88`, `71198b2`, `6188ab1`, `75e3c16`, `468a24a`, `248899d`, `5f42334`, `f7b7bc5`, `a1f8f04`, `2fa1c52`, `dc0ca8b`, `139ac67`, `46ef1c7`, `b6a40d4`, `2ff41b4`, `9a8f2b4`, `3532389`, `133cc01`, `bc7e307`, `266934c`, `69d8500`, `1623451`, `bc9434b`, `6ab2c42`, `436b11e`, `8515e5e`, `f379671`, `09db1be`, `f74ab5f`, `a0ea583`, `73a85f3`.

## 6. Remaining FAIL / NOT_VERIFIED / A-class blockers

- **Remaining FAIL**: **0**
- **Required NOT_VERIFIED**: **0**
- **True A-class blockers** (external hardware/credential/network the contract itself classifies as A): **0**

All contract-mandated Linux L-tier subcases that were previously NOT_VERIFIED on the Mac S-tier baseline are now executed on the disposable `duck-dev-01` Linux fixture with real evidence.

## 7. Evidence

- **Top-level results**: `/tmp/duckos-evidence/results.jsonl` (56 PASS, 0 FAIL, 0 NOT_VERIFIED)
- **Per-DV summaries**: `/tmp/duckos-evidence/DV{00,01,02,03,04-WG,04-XRAY,05,06,07,08,09,10}/summary.json`
- **Per-DV raw JSONL**: `/tmp/duckos-evidence/DV*/results.jsonl`
- **Per-DV runner logs**: `/tmp/duckos-evidence/DV*/runner.log`
- **Per-test Go test logs**: `/tmp/duckos-evidence/DV*/test-*.log` and `test-L-*.log`
- **Harness summary**: `/tmp/duckos-evidence/summary.json` (git_head, OS, Go version)
- **Acceptance log**: `/tmp/duckos-evidence/acceptance.log`

No secrets in any evidence file. All keys in fixtures are synthetic (`PUB-...` strings).

## 8. Verdict

```
BACKEND VERIFICATION:                 PASS
SAFE TO FREEZE BACKEND:               YES
SAFE TO START UI IMPLEMENTATION:      NO   (per the contract — UI must not start; this handoff is the gate, not the trigger)
```

UI implementation remains forbidden until the user explicitly releases the UI gate.

## 9. Git status

- `git status` clean on the VM working tree (no uncommitted changes, no untracked files).
- `HEAD` is `ab1ea60`, branch `V1 UI qualification branch`, remote `duck-dev-01`.
- Bare repo lives at `/tmp/duckos-test/repo` on `duck-dev-01`.

End of handoff.
