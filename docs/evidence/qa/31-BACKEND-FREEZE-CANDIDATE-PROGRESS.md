# DuckOS V1 Backend Freeze Candidate — Progress (Batches 4–10)

**HEAD:** `7bb6b5e` (`V1 UI qualification branch`)
**Date:** 2026-09-12
**Status:** Backend freeze work continues; UI implementation not started.

This document supersedes the interim checkpoint at
`30-BACKEND-FREEZE-CANDIDATE-FINAL.md` for batches 4–10.

## Resolved gaps (this continuation)

| Gap | Resolution | Commit |
|---|---|---|
| G02 per-domain wiring | `internal/objectlisters` registers 19 supported kinds; `Engine.RegisterObjectLister` + `Engine.ObjectList`; stub listers carry explicit `unsupported_kind` issue | `bbfdf54`, `351acbe` |
| G06 HTTP Collection mount | `internal/api/objects.go` `handleObjects` for `GET /api/v1/objects/{kind}`; auth enforced in-handler (M-27 fix) | `351acbe` |
| G04 readiness agent write side | `internal/chaos/readiness_probes.go` Recorder + `internal/agent/readiness_host.go` IPC; 11 daemon probes registered on Linux | `25c18d1` |
| G08 dependency / ownership / provenance | `internal/lifecycle/provenance.go` OwnershipStore; `internal/agent/provenance_host.go` IPC; `duck provenance` CLI | `ea0088c` |
| G18 first-run persistence | `internal/firstrun` atomic on-disk state; `internal/agent/firstrun_host.go` IPC; `duck firstrun` CLI | `56c3ce9` |
| G21 authorization granularity | `internal/identity/policy.go` per-object overlay (deny wins over allow wins over role matrix) | `c7a4c32` |
| G19 contextual diagnostic job service | `internal/diagjob` bounded Store; `internal/agent/diagjob_host.go` IPC; `duck diagjobs` CLI | `b93ec69` |
| G23 session / reconnect continuity | `internal/identity/session_continuity.go` Touch / Renew / BoundSessions / ReconnectReport | `7bb6b5e` |

## Defects fixed

- **M-27**: `/api/v1/objects/{kind}` previously bypassed authentication
  (the route was matched before `serveAPI`). Fixed by in-handler auth.
- `internal/upgrade/torture_test.go` previously failed to build due to
  unused imports and a `store.List()` signature drift; fixed.

## Test additions

~100+ new torture subtests across 11 packages:

- `internal/objectlisters/torture_test.go` — 7
- `internal/api/objects_test.go` — 8
- `internal/chaos/readiness_probes_test.go` — 9
- `internal/agent/readiness_host_test.go` — 7
- `internal/engine/readiness_torture_test.go` — 8
- `internal/lifecycle/provenance_test.go` — 12
- `internal/agent/provenance_host_test.go` — 9
- `internal/firstrun/firstrun_test.go` — 17
- `internal/agent/firstrun_host_test.go` — 9
- `cmd/duck/provenance_test.go` — 8
- `internal/identity/policy_test.go` — 14
- `internal/diagjob/diagjob_test.go` — 17
- `internal/agent/diagjob_host_test.go` — 8
- `internal/identity/session_continuity_test.go` — 11

## Open gaps (still pending)

| Gap | Reason |
|---|---|
| G05 live health observations | Needs unified per-component probe framework with bounded history |
| G09 topology evidence | Topology host exists (partial); needs richer evidence fields |
| G10 dynamic-routing expert telemetry | FRR-specific; depends on daemon presence |
| G11 layered WAN probes | Multi-WAN probe framework |
| G14 bounded logging / trace jobs | Log retention + sampling |
| G16 upgrade activation public bridge | Upgrade journal exists; needs IPC + CLI parity |
| G17 portable/selective restore bridge | Backup journal exists; needs restore IPC |
| G20 measurement quality/history | Sampling quality metrics |
| G22 specialized DNS/DHCP/AAA/portal/flow/hardware operations | Per-domain ops |
| Provisioning / ZTP contract | Cryptographic secret generation, hashed storage, single-use, expiry + node-binding (UX spec requires this before enabling) |
| Anycast evidence contract | VIP + hysteresis + verified probe + actual advertisement/withdrawal evidence |

## Hardware-required / credential-required / network-required

- IPv6 ND, OSPFv3, multi-WAN failover (needs real NICs)
- MAC flapping, rogue DHCP/RA (needs switch mirroring)
- hostapd surveys (needs real radio)
- Xray 26.3.27 binary, Pebble 2.10.1 binary (not installed in CI)
- Real WAN/RADIUS/ACME/WG credentials
- Live DNS zone (registrar-side)

## CLI coverage (incremental additions this session)

New commands:

- `duck provenance --kind <kind> --id <id>` (json/table/text)
- `duck firstrun` (json/table/text)
- `duck diagjobs [--id <id>]` (json/table/text)

## Known limitations

- G02 per-domain listers are stubs; per-domain production listers
  must be wired as the underlying stores are exercised.
- Object kind validation in API uses canonical vocabulary but does
  not yet check identity enrolment depth (RBAC enforcement is
  role-matrix only; per-object policy is overlay).
- Diagjob store is in-memory only; persistence to disk is a
  follow-up.

## Stop condition status

Per the user's directive the program continues until:

- Backend FREEZE-CANDIDATE: **NO** (10 gaps still open)
- API CONTRACT FREEZE-CANDIDATE: **NO** (open gaps include
  G05, G09, G10, G11, G14, G16, G17, G20, G22)
- CLI CONTRACT FREEZE-CANDIDATE: **NO** (above gaps lack CLI parity)
- READY FOR INDEPENDENT BACKEND VERIFICATION AUDIT: deferred until all
  three reach YES.

The program continues to the next internal batch automatically
unless a quota interrupt, hardware/credential blocker, or unsafe
isolation state is observed.
