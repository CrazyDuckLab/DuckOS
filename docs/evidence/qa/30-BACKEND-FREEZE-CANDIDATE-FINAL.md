# BACKEND FREEZE CANDIDATE — FINAL VALIDATION

**Branch / HEAD**: `V1 UI qualification branch` @ `46e79c2`
**Commits ahead of main**: 271
**Destructive-QA baseline**: `e3ff655`
**Batch 1 final**: `9dc55d0`
**Batch 2 final**: `003373a`
**Batch 3 final**: `46e79c2`

## Final validation results

```
go vet  (10 packages: backup, identity, api, chaos,
         invariants, xray, wireguard, object, engine, state) : clean
go test -count=1 (10 packages)                                  : all ok
go test -count=1 -short ./internal/identity/...                 : ok 11.9s
go test -count=1 -run TestG01_ ./internal/engine/...            : ok 90ms
go test -count=1 -run TestG06_ ./internal/engine/...            : ok
go test -count=1 -run TestG13_ ./internal/api/...               : ok
go test -count=1 -run TestTorture_ (xray, wireguard, object)   : ok
```

## Gaps closed this session

| ID | Title | Resolution |
|---|---|---|
| G03 | Outer method dispatcher | `internal/api/server.go` per-path method gating |
| G15 | Localized structured errors | `internal/api/errors.go` + `problem()` rewrite |
| G18 | Secret-safe DTO (Xray) | `internal/xray/views.go` |
| G24 | (alias) Same as G18 | |
| M-13 | IPv6 decoder | `internal/network/route_snapshot.go canonicalDestination` |
| G04 (read side) | Readiness manifest | `internal/engine/readiness.go` + `/api/v1/readiness` |
| G06 (parser + envelope) | Collection parser + `object.Collection` | `internal/api/collection.go`, `internal/object/object.go` |
| G01 | Safe Apply public contract | `internal/engine/config.go` + `/api/v1/config/*` + `cmd/duck/config.go` |
| G07 (config) | Duck CLI/API parity | `cmd/duck/config_parity_test.go` |
| M-25 | Revision identity mismatch | `internal/state/store.go` accepts 32/64-char hex |
| G02 (primitive) | Canonical kind/id/revision | `internal/object/object.go` + 8 torture subtests |
| G12 (view side) | WireGuard + Xray views | `internal/wireguard/views.go`, hardened `internal/xray/views.go` |
| G13 (regression) | Secret export torture | `internal/api/secret_export_torture_test.go` |
| G06 (wiring) | ObjectList engine surface | `internal/engine/objects.go` + 4 torture subtests |

## New defects found

| ID | Description | Status |
|---|---|---|
| M-25 | `state.Store.Reconcile` required 32-char hex; `state.Revision.ID` is 64-char sha256 | **FIXED** |
| M-26 | WireGuard `ViewServer.Revision` / `ViewPeer.Revision` leaked the raw public key | **FIXED** |

## Gaps still open (with reasons)

| ID | Title | Why |
|---|---|---|
| G02 (wiring) | Per-domain CRUD endpoints (network, route, firewall, lease, WG, Xray) | Not wired in this session; `object.Collection` primitive ready; representative domains require per-store wiring that crosses many packages |
| G04 (write side) | Agent → readiness classification | Agent composition root not extended in this session; read-side wired |
| G05 | Live health observations | Per-source observation not extracted |
| G06 (endpoints) | `GET /api/v1/objects/{kind}` HTTP endpoint | Engine surface ready; HTTP mount deferred |
| G08 | Dependency / provenance public surface | No backend provider in this session |
| G09 | Rich topology evidence | Topology exists in `internal/topology`; evidence metadata not added |
| G10 | Dynamic routing expert telemetry | FRR models exist; not mounted |
| G11 | Layered WAN probes | wan.Provider exists; not exposed |
| G14 | Bounded logging / trace jobs | Not in scope this session |
| G16 | Upgrade activation public bridge | upgrade models exist; not mounted |
| G17 | Portable / selective restore public bridge | backup exists; not exposed |
| G18 | First-run workflow persistence | firstrun package exists; not wired |
| G19 | Contextual diagnostic job service | nettools exists; bounded job service not wired |
| G20 | Measurement quality / history | overview exists; collector coverage partial |
| G21 | Per-object RBAC | PermissionSet primitive ready (G02); per-object gate not implemented |
| G22 | Specialised DNS / DHCP / AAA / portal / flow / hardware | Partial |
| G23 | Session / reconnect continuity | Partial |
| Provisioning / ZTP / Anycast | Secure enrollment, durable tokens, advertised evidence | Not started |

## CLI coverage matrix (factual)

| Family | List | Show | Create | Edit | Delete | Action | Diagnostics | JSON | RBAC | Safe Apply | API parity |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `duck health` | – | Y | – | – | – | – | – | Y | (token) | – | Y |
| `duck overview` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck ready` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck doctor` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck topology` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck multiwan` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck network status/apply/confirm/rollback` | Y | Y | Y | – | Y | Y | – | Y | (token) | Y | Y |
| `duck qos status/apply/rollback` | Y | Y | Y | – | – | Y | – | Y | (token) | Y | Y |
| `duck ups status/probe` | Y | – | – | – | – | Y | – | Y | (token) | Y | Y |
| `duck migrate analyse` | – | – | – | – | – | Y | – | Y | (token) | – | Y |
| `duck sites status` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck wifi status` | Y | – | – | – | – | – | – | Y | (token) | – | Y |
| `duck identity login/logout/enroll/users/etc` | Y | Y | Y | Y | Y | Y | – | Y | (identity) | – | Y |
| `duck wireguard servers/peers/...` | Y | Y | Y | Y | Y | Y | – | Y | (identity) | – | Y |
| `duck xray ...` | Y | Y | Y | Y | Y | Y | – | Y | (identity) | – | Y |
| **`duck config status/diff/apply/confirm/rollback`** (Batch 2) | Y | Y | Y | Y | Y | Y | – | Y | (token) | Y | **Y (parity tested)** |
| `duck config` (history, checkpoints) | – | – | – | – | – | – | – | – | – | – | – |
| `duck readiness` | – | – | – | – | – | – | – | – | – | – | – |
| `duck objects {kind}` | – | – | – | – | – | – | – | – | – | – | – |
| `duck secret-export` | – | – | – | – | – | – | – | – | – | – | – |

The CLI does not currently expose `history`, `checkpoints`, `readiness`,
`objects`, or `secret-export` as direct subcommands. Recovery
must still be possible via `overview` / `doctor` / `network
status` / `config status` and the existing `state` tooling.

## Management boundary

After every commit in this session:

- `eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64`
- `default via 172.18.1.254 dev eth0`
- `758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups`

No management-plane disruption across the entire program
(3 batches, ~30 commits, ~270+ commits ahead of main).

## Known software defects remaining

- None discovered in this session that are critical / high
  for the closed subset (G01, G02-primitive, G03, G04-read,
  G06-primitive, G07, G12-views, G13, G15, G18, G24, M-13, M-25, M-26).
- The closed subset has full unit + torture + parity + race
  coverage where applicable.

## Hardware-required items

- Real NICs for production IPv6 ND, OSPFv3, multi-WAN
  failover, BGP live path monitoring.
- Real switch mirroring for MAC flapping, rogue DHCP,
  rogue RA detection.
- Real radio for hostapd runtime.

## Credential / network-required items

- Xray 26.3.27 binary (github.com egress closed).
- Pebble 2.10.1 binary (github.com egress closed).
- Real WAN credentials, RADIUS shared secrets, ACME account
  keys, WireGuard PSKs.
- Live DNS zone with API access for split-DNS / DDNS.

## Security regression coverage

- M-1 / M-2 / M-3 / M-4 / M-5 / M-6 / M-7 / M-8 / M-9 / M-10 /
  M-12 / M-13 / M-15 / M-19 / M-20 / M-21 / M-22 / M-23 / M-24 /
  M-25 / M-26 — all have regression tests; the destructive-QA
  evidence is preserved at `docs/evidence/qa/01-19` plus
  `20-30` from the freeze-candidate session.
- M-13 fuzz corpus entry `414a5aae72465c6a` no longer fails
  (canonical-destination boundary).
- M-26 caught: WireGuard view revisions now use a digest,
  not the raw public key.

## Final qualification

```
Backend FREEZE-CANDIDATE              : NO (16 of 24 gaps still open)
API CONTRACT FREEZE-CANDIDATE          : PARTIAL (config/* stable;
                                          G02 primitive stable; rest
                                          partial)
CLI CONTRACT FREEZE-CANDIDATE          : PARTIAL (config stable;
                                          parity tested; coverage
                                          matrix has explicit gaps
                                          for history/checkpoints/
                                          readiness/objects/secret-
                                          export)
READY FOR INDEPENDENT BACKEND VERIFICATION AUDIT : YES — closed subset is
                                          reviewable; open subset is
                                          documented with reasons.
READY FOR UI IMPLEMENTATION AFTER AUDIT   : NO — G02 per-domain
                                          wiring + G06 endpoint
                                          mount + G04 agent
                                          wiring are prerequisites
                                          for honest UI.
```

## Next batch (Batch 4) candidates

- G06 endpoint wiring: `GET /api/v1/objects/{kind}` with
  per-domain listers.
- G04 agent write side: feed `chaos.ReadinessStore` from
  actual subsystem evidence (FRR presence, nft presence,
  Xray binary presence, …).
- G08 dependency / provenance public surface: thin
  read-side wrapper over `internal/lifecycle` + `internal/intca`.
- CLI recovery-console additions: `duck history`, `duck
  checkpoints`, `duck readiness`.
- G16 / G17 / G18 / G19 / G20 / G21 / G22 / G23 — each a
  dedicated follow-up batch.
