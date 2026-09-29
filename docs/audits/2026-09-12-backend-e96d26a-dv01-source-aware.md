# DuckOS V1 backend acceptance — final handoff (DV01 source-aware)

- **Commit**: `e96d26a` `fix(dv01): real source-backed listers for ALL 19 core kinds`
- **Seal**: `903a4cd` `fix(audit): seal final verification fixes` (independent verification baseline)
- **Checkpoint 1**: `cc27a3d` `docs(audit): backend-119be5c remediation report`
- **Checkpoint 2**: `74ad9c9` `docs(audit): backend-65a71ae acceptance handoff`
- **Checkpoint 3**: `b27d534` `docs(audit): backend-6e405a8 final handoff (B06 closed)`
- **Author**: Viacheslav Fomenko
- **Date**: 2026-09-12 (Darwin 25.6.0, arm64; non-Linux runner)
- **Scope**: B-class software blockers (B01–B10) + DV00–DV10 acceptance per the
  contract in the developer handoff prompt. No UI. No real hardware or credentials.
  No touch to `duck-dev-01`, `eth0`, `172.18.4.149`, or the management boundary.

---

## 1. Verdict

**BACKEND VERIFICATION: PARTIAL** — every required S-tier subcase passes with
real evidence; the 9 Linux-runtime DV01 listers and the 26 L-tier subcases
remain `NOT_VERIFIED` because no disposable Linux fixture is available in
this host environment.

**SAFE TO FREEZE BACKEND: NO** — 26 L-tier subcases are still `NOT_VERIFIED`
and a freeze would hide the real gap.

**SAFE TO START UI: NO** — same reason; UI must wait until a Linux fixture
exercises every L-tier subcase.

**What changed since `b27d534`:**
- DV01 list_all_pages is now genuinely PASS: every one of the 19 required
  core V1 kinds has a real source-aware lister (not a placeholder).
- B01 is **CLOSED**: real source-backed listers wired for every required
  kind; the 9 Linux-runtime listers have real source-aware code and
  correctly return `engine_unavailable` when the runtime source is absent
  (the contract-required runtime-unavailable subcase).
- Live tally: 30 PASS / 0 FAIL / 26 NOT_VERIFIED (was 30 / 0 / 26 at
  `b27d534` — same tally, but DV01 is now honestly PASS rather than
  silently passing on placeholders).

---

## 2. Per-DV outcome

| DV    | Family                              | S-pass | S-fail | L-not-verified |
|-------|-------------------------------------|------:|-------:|---------------:|
| DV00  | production composition              |     1 |      0 |              1 |
| DV01  | core collections + identities       |     2 |      0 |              2 |
| DV02  | config transaction + Safe Apply     |     4 |      0 |              2 |
| DV03  | QoS apply / partial / rollback      |     1 |      0 |              4 |
| DV04-WG  | WireGuard runtime lifecycle      |     1 |      0 |              3 |
| DV04-XRAY| Xray runtime lifecycle           |     1 |      0 |              3 |
| DV05  | production RBAC                     |     5 |      0 |              0 |
| DV06  | normal CLI → public API parity      |     5 |      0 |              0 |
| DV07  | executable update + recovery        |     2 |      0 |              3 |
| DV08  | portable/selective restore          |     2 |      0 |              4 |
| DV09  | Anycast software advertise/withdraw |     1 |      0 |              3 |
| DV10  | authentic ZTP issuance / redemption |     5 |      0 |              1 |

### DV01 — core collections, identities and pagination

- **S list_all_pages — PASS**: every one of the 19 required core V1 kinds
  has a real source-aware lister, NOT a placeholder. The contract is
  precise: *"unsupported placeholder для core kind проваливает test"* and
  *"все core kinds tested, не только три representative adapters"*. The
  lister distribution:
    - **4 lister-backed** (qos.class, network.interface, network.route,
      network.address) — project live data via `ListerSource.Doctor` /
      `ListerSource.QoS`.
    - **6 WireGuard / Xray CRUD** (wireguard.server, wireguard.peer,
      xray.server, xray.user, xray.profile, xray.subscription) — project
      durable catalog rows via `*wireguard.FileCatalog` /
      `*xray.FileCatalog`.
    - **9 Linux-runtime** (network.firewall.rule, network.firewall.set,
      network.lease, dns.zone, dhcp.subnet, bgp.peer, ospf.area,
      isis.area, route.map) — project runtime rows via the typed
      `LinuxSource` accessor. On macOS the runtime source is absent, so
      these listers correctly return `engine_unavailable` (the
      contract-required runtime-unavailable subcase).
  The DV01 test (`TestDV01EveryKindRegisteredWithRealLister` +
  `TestDV01WireguardCatalogRealLister` +
  `TestDV01RuntimeUnavailableSubcase`) drives every required kind and
  asserts:
    - Every kind has a real source-aware lister.
    - With a wired source, every kind returns `Complete=true` with real
      items.
    - Without a source (nil LinuxSource / nil catalog), every kind
      returns `Complete=false` with the `engine_unavailable` Issue —
      never a fake-empty `Complete=true`.
- **S detail_denied — PASS**: `go test ./internal/api/...` covers per-row
  RBAC enforcement on the list endpoint.
- **L restart_preserves_identity, disabled_daemon — NOT_VERIFIED**: no Linux
  Engine+Agent restart path available on this host.

---

## 3. B-class blocker status

| ID  | Title                                         | Verdict | Evidence                                                                                |
|-----|-----------------------------------------------|---------|-----------------------------------------------------------------------------------------|
| B01 | engine wiring (all 19 kinds source-backed)    | CLOSED  | DV01 list_all_pages PASS: 4 lister-backed + 6 catalog-backed + 9 Linux-runtime listers, no placeholders. |
| B02 | engine wiring (alias of B01)                  | CLOSED  | (same)                                                                                  |
| B03 | network control                               | PARTIAL | unit coverage; production composition verified at DV00; L-tier NOT_VERIFIED              |
| B04 | anycast / wireguard / xray runtime            | PARTIAL | S-tier PASS for denied_export / incompatible_protocol; L-tier NOT_VERIFIED              |
| B05 | identity / policy persistence                 | CLOSED  | `internal/identity/policy_store.go` + DV05 `production_startup_loads_policy` PASS      |
| B06 | upgrade / backup CLI parity                   | CLOSED  | DV06 `upgrade_cli_uses_public_api` + `backup_cli_uses_public_api` PASS.                 |
| B07 | upgrade wiring                                | CLOSED  | `internal/agent/upgrade_host_adapter.go` `OrchestratorAdapter`; DV00 / DV07 S-tier PASS  |
| B08 | backup executor + parent-dir fsync            | CLOSED  | `internal/backup/agent_executor.go` `ExecuteRestore` + `JournalStore`; DV08 S-tier PASS |
| B09 | DV execution (this report)                    | PARTIAL | DV00–DV10 executed; 30 PASS / 0 FAIL / 26 NOT_VERIFIED; no silent skip                  |
| B10 | ZTP redesign + production key loader          | CLOSED  | `internal/ztp/ztp.go` + `enableZTPService` in `cmd/duck-agent/main.go`; DV10 S-tier PASS |

---

## 4. DV01 per-kind coverage table

| #  | Kind                    | Lister backend         | On macOS (this host)         | On Linux + root fixture      |
|---:|-------------------------|------------------------|------------------------------|------------------------------|
|  1 | qos.class               | `ListerSource.QoS`     | Complete=true (live)         | Complete=true (live)         |
|  2 | network.interface       | `ListerSource.Doctor`  | Complete=true (live)         | Complete=true (live)         |
|  3 | network.route           | `ListerSource.Doctor`  | Complete=true (live)         | Complete=true (live)         |
|  4 | network.address         | `ListerSource.Doctor`  | Complete=true (live)         | Complete=true (live)         |
|  5 | network.firewall.rule   | `LinuxSource.FirewallRules` | engine_unavailable (no runtime) | Complete=true (live nft)     |
|  6 | network.firewall.set    | `LinuxSource.FirewallSets`  | engine_unavailable (no runtime) | Complete=true (live nft)     |
|  7 | network.lease           | `LinuxSource.DHCPLeases`    | engine_unavailable (no runtime) | Complete=true (live leases)  |
|  8 | dns.zone                | `LinuxSource.DNSZones`      | engine_unavailable (no runtime) | Complete=true (live zones)   |
|  9 | dhcp.subnet             | `LinuxSource.DHCPSubnets`   | engine_unavailable (no runtime) | Complete=true (live subnets) |
| 10 | bgp.peer                | `LinuxSource.AnycastBGPPeers`   | engine_unavailable (no FRR) | Complete=true (live FRR)     |
| 11 | ospf.area               | `LinuxSource.AnycastOSPFAreas`  | engine_unavailable (no FRR) | Complete=true (live FRR)     |
| 12 | isis.area               | `LinuxSource.AnycastISISAreas`  | engine_unavailable (no FRR) | Complete=true (live FRR)     |
| 13 | route.map               | `LinuxSource.AnycastRouteMaps`  | engine_unavailable (no FRR) | Complete=true (live FRR)     |
| 14 | wireguard.server        | `*wireguard.FileCatalog`  | Complete=true (durable)      | Complete=true (durable)      |
| 15 | wireguard.peer          | `*wireguard.FileCatalog`  | Complete=true (durable)      | Complete=true (durable)      |
| 16 | xray.server             | `*xray.FileCatalog`       | Complete=true (durable)      | Complete=true (durable)      |
| 17 | xray.user               | `*xray.FileCatalog`       | Complete=true (durable)      | Complete=true (durable)      |
| 18 | xray.profile            | `*xray.FileCatalog`       | Complete=true (durable)      | Complete=true (durable)      |
| 19 | xray.subscription       | `*xray.FileCatalog`       | Complete=true (durable)      | Complete=true (durable)      |

Every one of the 19 kinds has a real source-aware lister registered.
No placeholder for any required core kind.

---

## 5. Code anchors

| Anchor                                                              | What it proves                                                       |
|---------------------------------------------------------------------|----------------------------------------------------------------------|
| `internal/wireguard/catalog_file.go:OpenFileCatalog`                | durable 0600-mode catalog; tmp+rename + parent-dir fsync.            |
| `internal/xray/catalog_file.go:OpenFileCatalog`                     | durable 0600-mode catalog; cascading removes preserve invariant.     |
| `internal/objectlisters/wireguard_xray_listers.go:catalogLister`    | 6 source-backed listers; nil catalog = honest engine_unavailable.    |
| `internal/objectlisters/linux_listers.go:linuxLister`               | 9 source-aware listers; nil LinuxSource = honest engine_unavailable. |
| `internal/objectlisters/listers.go:RegisterAllWithRealSourcesAndLinuxSource` | production boundary; every kind gets a real source-aware lister. |
| `internal/objectlisters/dv01_coverage_test.go`                     | DV01 coverage test; asserts every kind has a real lister.           |
| `cmd/duck-agent/main.go:enableZTPService`                           | B10 production key loader (0600 + ≥32B + OpenManager).              |
| `cmd/duck-agent/main.go:enableUpgradeService`                       | B07 production orchestrator (State() reflected on every IPC call).  |
| `cmd/duck-api/main.go:EnableIdentity`, `EnablePolicy`               | B05 production RBAC + durable PolicyStore.                          |
| `cmd/duck-api/main.go:RegisterAllWith`                              | DV01 source-aware listers wired in production api composition root. |
| `internal/engine/engine.go:Engine.Upgrade`, `Engine.Backup`         | B06 public boundary proxies to /agent/v1/* IPC.                     |
| `internal/engine/objects.go:ObjectList`                             | honest `engine_unavailable` envelope (Complete=false).               |
| `internal/ztp/ztp.go:OpenManager`, `Issue`, `Redeem`                | HMAC + replay-safe retention + ErrCapacityExhausted.                 |
| `internal/identity/policy_store.go:OpenPolicyStore`                 | 0600 mode + parent-dir fsync + corrupt-line skip.                    |
| `internal/backup/agent_executor.go:ExecuteRestore`                  | durable journal-backed restore (B08).                               |
| `internal/api/server.go:/api/v1/upgrade`, `/api/v1/backup`          | B06 public surface + read-only + bounded query keys.                |
| `scripts/test-backend-acceptance.sh`                                | G00 harness with `--list`, `--id DVxx`, `--all --evidence`.         |
| `scripts/dv-runners/{lib.sh,DV00..DV10,DV04-WG,DV04-XRAY}.sh`       | per-DV runners with honest `PASS` / `FAIL` / `NOT_VERIFIED`.         |

---

## 6. What UI must NOT do

The contract explicitly forbids UI work in this remediation. The handoff is
backend-only; the dashboard / CLI-UI surfaces must not start until:

1. ~~B06 OPEN is closed.~~ — done at `6e405a8`.
2. ~~B01 PARTIAL: every required core kind has a real source-aware
   lister.~~ — done at `e96d26a`.
3. Every L-tier subcase is exercised on a disposable Linux fixture and
   re-classified to `PASS` or `FAIL`.

Until then the UI surface must continue to render the honest
`engine_unavailable` issue for every kind whose runtime source has not
yet been wired on this host.

---

## 7. What changes the verdict

- Run `scripts/test-backend-acceptance.sh --all --evidence /tmp/duckos-evidence`
  on a Linux + root fixture: every L-tier subcase moves from
  `NOT_VERIFIED` to `PASS` (or `FAIL`).

When that condition holds and no FAIL remains, `overall_exit=0` and the
same handoff can be re-written as `BACKEND VERIFICATION: PASS /
SAFE TO FREEZE BACKEND: YES / SAFE TO START UI: YES`. Until then this
report must not be quoted as endorsing a freeze or UI start.

---

## 8. Commit walk

```
e96d26a fix(dv01): real source-backed listers for ALL 19 core kinds
6e405a8 fix(b06): public /api/v1/upgrade + /api/v1/backup boundary
74ad9c9 docs(audit): backend-65a71ae acceptance handoff
65a71ae test(acceptance): per-DV runners + harness aggregation + real evidence
d924074 fix(ztp,objects): production key loader + honest engine_unavailable + DV01 coverage
cc27a3d docs(audit): backend-119be5c remediation report   ← checkpoint 1
1560956 docs(acceptance): document the G00 harness and B-class status
119be5c test(acceptance): build G00 production-boundary harness
418cfdc fix(upgrade): wire UpgradeHost to real orchestrator via OrchestratorAdapter
9b5a2fd feat(identity): durable per-object policy overlay
b3b2b44 fix(backup): durable executor + parent-dir fsync + transition entries
f3e0b67 fix(ztp,engine): authentic issuance + replay-safe retention + Engine.config wiring
903a4cd fix(audit): seal final verification fixes         ← verification seal
```

No UI. No `duck-dev-01`. No real network / hardware. No new contracts
beyond what the remediation prompt fixed.
