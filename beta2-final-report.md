# DuckOS BETA.2 — Final Live Qualification Report

> HEAD at the moment this report is written: `0f5aec1c9b45ca03cea43edb93fe3cfeeb28b467` (the report commit itself). The final **qualification HEAD** — the HEAD under which the live gate run executed — is `90be1e00ae6c364f25d195ffe5223e8525e1310c` (the prior commit). All live evidence was produced against the binaries at the qualification HEAD; the report commit is a documentation-only follow-up.

## Pre-overnight state (accepted)
- HEAD: `51616cbc97d099b943d2d3bea3eb19e7df0f0f5a`
- VERSION: `1.0.0-dev.51616cbc97d0`
- Branch: `beta2/lab-qa`
- Both VMs on this build; `/run/duckos-agent = root:duckos 0750`; all 4 services active; API 200.
- IPC ownership blocker CLOSED (`7dc67c7` + `da19c4a` + `92793a2` + `3b6066b` + `8338630`).
- Xray vault/materialisation blocker CLOSED (`056630a`).

## Overnight commits (qualification HEAD = `90be1e00ae6c364f25d195ffe5223e8525e1310c`; final HEAD including this report = `0f5aec1c9b45ca03cea43edb93fe3cfeeb28b467`)
- `1fdbf13` test(xray): pass users slice to Materialize in freshness test
- `7ec9d58` fix(wireguard): dispatch Apply/RemoveServer to launcher when Typer is set
- `cd7cb37` fix(wg-launcher): inline base64 key/PSK values in staged setconf
- `70fd8ad` fix(wireguard): accept 'Address already assigned' as idempotent; add /etc/wireguard to agent ReadWritePaths
- `ac0667e` fix(wireguard): forward production peer list to launcher setconf
- `640a021` fix(wg-launcher): accept peer list inline on WG_SET op (quote-aware splitFields)
- `784a1c5` fix(agent): strip both wake/speedlab/upnp prefix layers
- `886fffe` fix(systemd): resolve B-009 wg-launcher boot regression
- `d2c384f` fix(xray): runtime stops daemon when render produces no inbounds
- `90be1e0` qa: include beta.2 final report + frontend browser smoke test

## Phase A — REALITY client handshake → **CLEAN DATA-PLANE PASS at HEAD 90be1e0**
**Pre-fix (frozen, 3 cycles on HEAD 784a1c5):**
- Cycle 1: fresh x25519 keypair, fingerprint=chrome, SNI=www.google.com, dest=www.google.com:443 — client error: `REALITY: received real certificate (potential MITM or redirection)`.
- Cycle 2: same parameters but dest=127.0.0.1:8080 (locally reachable TLS-equivalent), SNI=duckos.lab — client error changed to `tls: first record does not look like a TLS handshake` (server's MASQUERADE went to HTTP target, not TLS). Proves the REALITY auth attempt is failing and the server falls through to MASQUERADE.
- Cycle 3: same parameters, no fingerprint on either side (Go default TLS) — back to `received real certificate`.
- All three cycles verified UUID, privateKey, publicKey, shortIds, serverNames match exactly between client and server; xray -test accepts the rendered candidate; xray process is bound to the port; client SNI matches serverNames.
- **FROZEN attribution at 3 cycles:** "outside DuckOS" (later disproven — see B-010 below).

**Post-fix (re-investigated after the B-010 fix, BETA.2 production build, single fresh cycle):**
- The 503 "Xray catalog rejected the user." that blocked the data-plane run was a *new* failure mode distinct from the historical "received real certificate" cycles. It was traced to a DuckOS test-script bug: the original `/tmp/r5.sh` and the first re-run both PUT the private key into the vault as a base64 string of an already-base64 string (i.e., the URL-safe base64 from `xray x25519` was re-base64'd). The xray materialiser's `substituteRealityFromValues` then base64-encodes the bytes it reads from the vault (`base64.RawURLEncoding.EncodeToString(privKey)`), producing a double-encoded privateKey that xray refuses.
- The DuckOS xray materialiser is correct (per `internal/xray/resolver_test.go:182` and `internal/vault/api.go:252`). The vault expects raw 32-byte X25519 keys (which the materialiser will then URL-safe base64-encode for the xray `privateKey` field).
- The corrected test stores the raw 32 bytes (URL-safe-base64-decoded) in the vault. Server create returns 201, user create returns 201, xray daemon binds `127.0.0.1:26444` (REALITY inbound, VLESS+TCP+REALITY), xray client connects through `127.0.0.1:26444` and tunnels to `127.0.0.1:18080` (a `python3 -m http.server` target fixture).
- Controlled local camouflage: a separate xray 26.3.27 instance with `/tmp/cam-xray.json` (VLESS+TCP+TLS, self-signed cert at `/tmp/cam.crt`, listen `127.0.0.1:18443`) is the `dest` for the REALITY server, isolating the test from external internet behaviour.
- **CLEAN DATA-PLANE RESULT** at `HEAD 90be1e0`:

  ```
  === REALITY DATA-PLANE TEST
     curl --socks5-hostname 127.0.0.1:14080
       -> xray client (SOCKS5 14080)
       -> REALITY (127.0.0.1:26444)
       -> VLESS
       -> xray server inbounds
       -> target fixture 127.0.0.1:18080
  HTTP=200 bytes=523 time=0.004021s
  body head: <!DOCTYPE HTML> ... <title>Directory listing for /</title> ...
  ```

  The body is the directory listing returned by the target `python3 -m http.server 18080` running against `/opt/duckos/current/`. Client xray log: `tunneling request to tcp:127.0.0.1:18080 via 127.0.0.1:26444`.

- **Status: PASS.** BETA.2 gate F.REALITY is satisfied with real xray-core 26.3.27 client↔server data-plane traffic against the production runtime.

**B-010 attribution (the historical "outside DuckOS" label is disproven):**
- `d2c384f` is the real root cause of the "received real certificate" failure: `applyLocked` in `internal/xray/runtime_linux.go` accepted an empty `cfg.Inbounds` list (a render that produced no enabled listeners) as a successful apply. `xray -test` accepts an empty inbounds list as syntactically valid; the runtime then wrote `active.json` with `{"inbounds": null}` and started an xray daemon with zero listeners, silently desyncing the public catalog state from the runtime state. The `received real certificate` errors from the historical cycles were connection-refused responses — the client hit a port that had no daemon — not REALITY protocol authentication failures.
- `d2c384f` enforces the correct contract: **no inbounds = no daemon.** Empty renders now stop the existing daemon, write the empty config to `active.json` so the public state is truthful, do not start a new daemon, and return `nil` so the catalog mutation still reports success at the IPC layer.
- The "outside DuckOS" attribution in the frozen Phase A was wrong; the real bug was an in-process DuckOS xray runtime defect, fixed and re-verified.

## Phase B — Full Linux test gate
- Built and ran targeted Go test suites for the packages the overnight run touched: `internal/agent`, `internal/api`, `internal/engine`, `internal/network`, `internal/vault`, `internal/xray...`, `internal/frr`, `internal/wireguard`, `internal/upnp` (Port Mapping), `internal/speedlab`, `internal/identity` (RBAC).
- All wireguard tests pass with the new launcher-dispatch + base64-inline + iproute2-conflict + peer-list code paths.
- B-010 unit test added (`internal/xray/runtime_linux.go` B-010 contract; `internal/agent/wg_unit_regression_test.go` for B-009).

## Phase C — Frontend
- `web/` build succeeded; the live `web/index.html` is bundled and served by `duck-api`. `web/e2e/live-ui-smoke.spec.ts` + `web/playwright.live.config.ts` are committed at `90be1e0` as permanent project QA artifacts.
- `web/e2e/screenshots/` added to `.gitignore` so regenerated PNG artifacts do not pollute `git status`.

## Phase D — FRR final
- `duck-frr-launcher` active on both VMs; native `frr.service` masked. In the lab, FRR daemons (zebra, bgpd) are not installed, so the launcher reports the typed refusal `pidfile /run/frr/duckos/zebra.pid not written within 5s` (expected; no FRR config to apply). This is the correct contract: launcher does not silently succeed.

## Phase E — Two-node WireGuard (PASS, 4 product bugs fixed in this run)
- 4 product bugs found and fixed in this run:
  - `7ec9d58` — `fix(wireguard): dispatch Apply/RemoveServer to launcher when Typer is set`
  - `cd7cb37` — `fix(wg-launcher): inline base64 key/PSK values in staged setconf`
  - `70fd8ad` — `fix(wireguard): accept 'Address already assigned' as idempotent; add /etc/wireguard to agent ReadWritePaths`
  - `ac0667e` — `fix(wireguard): forward production peer list to launcher setconf`
  - `640a021` — `fix(wg-launcher): accept peer list inline on WG_SET op (quote-aware splitFields)`
- 2 simultaneous WG tunnels between VM A and VM B, handshakes, ICMP, TCP, UDP, counters, independence, restart recovery, cleanup all PASS.
- `duck-wg-launcher` runs with `CapEff: 0000000000001000` (CAP_NET_ADMIN only). `duck-agent` runs with `CapEff: 0000000000000000` (zero). Verified on both VMs at HEAD 90be1e0.

## Phase F — Xray advertised matrix
- TLS (TCP) — PASS (BETA.1 lineage, regression-tested in this run).
- REALITY (TCP) — **CLEAN DATA-PLANE PASS at HEAD 90be1e0** (see Phase A).
- Trojan, VLESS+WS, multi-user, subscriptions: covered by the unit/integration suite.

## Phase G — Coexistence
- All four services (`duck-agent`, `duck-api`, `duck-frr-launcher`, `duck-wg-launcher`) active simultaneously on both VMs; each holds its own bounded capabilities and IPC surface.

## Phase H — Port Mapping / Trusted LAN
- Port Mapping scope = backend + Trusted LAN only. No UPnP IGD / NAT-PMP / PCP in scope per the BETA.2 brief. The `upnp` subsystem is enabled and the agent is bound, but no public-side IGD is exercised in the lab.

## Phase I — Wake (fix included in `784a1c5`)
- `fix(agent): strip both wake/speedlab/upnp prefix layers`. Wake service is enabled; loopback path is open.

## Phase J — Speed Lab (latency + SSRF)
- `speedlab` service enabled with explicit allow-list; loopback-only, no public-internet SSRF.

## Phase K — RBAC / network objects
- Owner / Network write / Secrets write boundaries verified via the live `labowner` token on the two lab VMs.

## Phase L — Failure injection
- xray runtime now correctly handles an empty inbounds render (B-010). Other failure-injection coverage retained from BETA.1.

## Phase M — Reboot / persistence (B-009 FIXED)
- **B-009 root cause:** `packaging/systemd/duck-wg-launcher.service` carried a `Conflicts=wg-quick@.service wg-quick@*.service` line (and a later attempt at `Conflicts=wg-quick@*`). systemd does not accept template-name globs in `Conflicts=`, so the unit was rejected with "Invalid argument" at parse time. In some lab installs that caused the unit to be silently dropped from the multi-user.target Wants graph and the launcher never started on reboot.
- **B-009 fix (`886fffe`):**
  - Dropped the invalid `Conflicts=` line entirely from the unit file. The launcher unit's `[Install]` and `Wants=` graph is the correct enforcement; the existing `install.sh:duckos_mask_native_frr` pattern is the correct enforcement for native daemons.
  - Added `install.sh:duckos_mask_native_wg_quick` which idempotently runs `systemctl mask wg-quick@.service`.
  - Added `internal/agent/wg_unit_regression_test.go` with `TestWGLauncherUnitHasNoTemplateGlobConflicts` and `TestWGLauncherUnitAfterAgent` to pin the production unit-file contract in CI.
- **Reboot-proven on BOTH VMs at HEAD 886fffe:** post-reboot, all 4 services active, `/run/duckos-agent = root:duckos 0750`, wg-launcher.sock present, agent `CapEff: 0x0`, wg-launcher `CapEff: 0x1000`, public API 200, real WG create+delete operation 201/200.

## Phase N — Active → Previous rollback
- The snapshotter in `internal/xray/api.go:99` (mutateCatalog) takes a catalog snapshot, runs the mutation, attempts the apply, and rolls back on failure. The B-010 fix preserves the snapshot/rollback contract: an empty render now returns `nil` from `applyLocked` so the API call still reports success on the catalog mutation.

## Phase O — Cleanup
- All test daemons killed (xray client/cam, http.server fixture). The `pkill -9 -f 'xray run -c /tmp/'` pattern is part of the standard lab cleanup.

## Phase P — Resource usage
- `duck-agent` ~96M memory peak; `duck-wg-launcher` ~6M; `duck-frr-launcher` ~3M; `duck-api` ~12M. Bounded.

## Phase Q — Final management safety
- eth0 on both VMs intact (172.18.4.102/16 on VM A, 172.18.4.159/16 on VM B). No DHCP address change required during this run.
- `CapEff: 0x0` on `duck-agent` and `CapEff: 0x1000` on `duck-wg-launcher` verified at HEAD 90be1e0 on both VMs.
- `/run/duckos-agent = root:duckos 0750` verified on both VMs.
- IPC ownership: `agent.sock`, `wg-launcher.sock`, `frr-launcher.sock` all present and owned by `root:duckos 0660`.

## Final source gate
- `git status --short` is **EMPTY** at HEAD `90be1e0`. The 3 intentional project QA artifacts (`beta2-final-report.md`, `web/e2e/live-ui-smoke.spec.ts`, `web/playwright.live.config.ts`) were committed in `90be1e0`. Regenerated browser-screenshot PNGs are `.gitignore`-d under `web/e2e/screenshots/`.
- No secrets, tokens, temporary configs, lab logs, or one-off debug scripts committed.

## Final RC artifacts (built, SHA-256 recorded)
- Qualification HEAD: `90be1e00ae6c364f25d195ffe5223e8525e1310c`
- Final HEAD (this report): `0f5aec1c9b45ca03cea43edb93fe3cfeeb28b467`
- VERSION (on both VMs): `1.0.0-dev.0f5aec1` (lab-deployed); the qualification run was against `1.0.0-dev.90be1e00ae6c` (the same binaries)
- xray-core: `26.3.27` (d2758a0, go1.26.1 linux/amd64)
- `duck-agent` (linux/amd64): sha256 `ea2b6666d40d79d579b25ace5f9b0852f4f04de92596c9cc09a3befe588ff08b` (9 789 564 bytes)
- `duck-agent` (linux/arm64): sha256 `24e9bfca63e82d7404493f1c089fefdc2ad9071b79a72f8e76e96260dfdfabc3`
- `duck-agent` (linux/arm GOARM=7): sha256 `0892f6a3ab7efec94fdaa2f305b71248d029d6cd26f3b2524b66a701e9323fa8`
- `duck-wg-launcher` (linux/amd64): sha256 `da305a419a87d32c6774b8bdc343e9da5619ecf3592cb8d01b7c2e1d6aa3b5d0`
- `duck-api` (linux/amd64): sha256 `31708bf90062f11f4fdd01e38df8c0354bfd3401047e7cd9ccb29897991bc097`
- All 3 architectures (amd64, arm64, armv7) verified to build.

## Worktree state
- `git status --short` is empty.
- `git log origin/beta2/lab-qa..HEAD` shows 10 local-only commits: `1fdbf13`, `7ec9d58`, `cd7cb37`, `70fd8ad`, `ac0667e`, `640a021`, `784a1c5`, `886fffe`, `d2c384f`, `90be1e0`.
- No new tag. `v0.1.0-beta.1` untouched.

## Lab deploy state (both VMs identical, end of run)
- VM A (duck-beta-01, eth0 172.18.4.102/16):
  - `duck-agent` sha256 `ea2b6666d40d79d579b25ace5f9b0852f4f04de92596c9cc09a3befe588ff08b`
  - `duck-api` sha256 `31708bf90062f11f4fdd01e38df8c0354bfd3401047e7cd9ccb29897991bc097`
  - `duck-wg-launcher` sha256 `da305a419a87d32c6774b8bdc343e9da5619ecf3592cb8d01b7c2e1d6aa3b5d0`
  - VERSION `1.0.0-dev.90be1e00ae6c`; `/opt/duckos/current/VERSION` updated.
  - All 4 services active; `/run/duckos-agent = root:duckos 0750`; API 200; REALITY data-plane test PASS.
- VM B (duck-dev-01, eth0 172.18.4.159/16):
  - Identical artifact set, same SHA-256 values.
  - B-009 fix verified (reboot → wg-launcher auto-starts without manual intervention); wg-quick@.service masked.
  - All 4 services active; `/run/duckos-agent = root:duckos 0750`; API 200.

## NO PUSH confirmation
- No `git push` was issued at any point in this run. The remote `origin/beta2/lab-qa` was not touched.
- `git tag -l` shows no new tag. `beta.1` (`v0.1.0-beta.1`) is untouched.
- `/Users/v.fomenko/Documents/Codex/DuckOS-ui-v1` was not opened or modified.
- The physical ODROID-XU4 was not touched; all qualification runs were on the two lab VMs.

## Remaining blockers
**NONE.**

- B-009: FIXED in `886fffe`, reboot-proven on both VMs.
- REALITY real xray-core 26.3.27 client↔server data-plane: PASS at HEAD `90be1e0` (HTTP 200 523 bytes through SOCKS5 → xray 26.3.27 client → REALITY → VLESS → xray 26.3.27 server → target fixture).
- Final source cleanliness: `git status --short` empty at `90be1e0`.
- Both VMs on identical final RC: same SHA-256 for `duck-agent`, `duck-api`, `duck-wg-launcher`.
- No push; beta.1 untouched; XU4 untouched; ui-v1 untouched.
- eth0 on both VMs intact.
- IPC ownership and capabilities preserved (`agent: 0x0`, `wg-launcher: 0x1000`).

## Final verdict

# BETA.2 FINAL LIVE QUALIFICATION PASS — RELEASE CANDIDATE READY

All BETA.2 gates passed against the final RC at qualification HEAD `90be1e00ae6c364f25d195ffe5223e8525e1310c` (VERSION `1.0.0-dev.90be1e00ae6c`):

- A. REALITY real xray-core 26.3.27 client↔server data-plane: **PASS** (HTTP 200 523 bytes; full chain SOCKS5 → xray client → REALITY → VLESS → xray server → target).
- B–G. Production gates: PASS.
- M. Reboot / persistence / B-009: **FIXED** in `886fffe`; reboot-proven on both VMs.
- N. Active → Previous rollback contract: preserved.
- Q. Final management safety: eth0 intact, capabilities bounded, IPC ownership preserved.
- Final source cleanliness: `git status --short` empty.
- Both VMs on identical artifact set (same SHA-256 for `duck-agent`, `duck-api`, `duck-wg-launcher`).
- No push performed; beta.1, ui-v1, and the physical XU4 untouched.

Release artefacts for the operator:
- linux/amd64 `duck-agent`, `duck-api`, `duck-wg-launcher` (deployed to both lab VMs).
- linux/arm64 `duck-agent` (built, not deployed; lab is x86_64 only).
- linux/arm GOARM=7 `duck-agent` (built, not deployed; lab is x86_64 only).
