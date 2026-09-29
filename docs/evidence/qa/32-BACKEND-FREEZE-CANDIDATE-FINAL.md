# DuckOS V1 Backend Freeze Candidate — FINAL

**HEAD:** `4e8a7f9` (`V1 UI qualification branch`)
**Date:** 2026-09-12
**Branch:** `V1 UI qualification branch`
**Commits ahead of main:** ~280

This is the consolidated final report covering Batches 1–19
of the DuckOS V1 Backend Freeze Candidate program. UI
implementation has not started.

## Resolved gaps

| Gap | Resolution |
|---|---|
| G01 Safe Apply public contract | `internal/engine/config.go` Config + `/api/v1/config/*` + `cmd/duck/config` |
| G02 canonical object primitive | `internal/object` ObjectRef/Collection/Availability/PermissionSet |
| G02 per-domain wiring | `internal/objectlisters` 19 supported kinds + `Engine.RegisterObjectLister` |
| G03 HTTP method routing | per-path method gating inside `serveAPI` |
| G04 readiness read surface | `Engine.Readiness()` IPC envelope |
| G04 readiness agent write side | `internal/chaos/readiness_probes.go` Recorder + 11 daemon probes + IPC |
| G05 live health observations | `internal/livehealth` bounded Store with severity + staleness |
| G06 collection parser + envelope | `internal/object` + `api.ParseCollectionParams` |
| G06 HTTP Collection mount | `/api/v1/objects/{kind}` + auth (M-27 fix) |
| G07 config CLI/API parity | `cmd/duck/config` round-trip via httptest mock |
| G08 dependency/ownership/provenance | `internal/lifecycle/provenance.go` OwnershipStore + IPC + `duck provenance` |
| G12 WireGuard view models | `internal/wireguard/views.go` |
| G13 authorized secret export contract | `internal/clientexport` + IPC |
| G14 bounded logging/trace jobs | `internal/logsink` bounded Store with severity + GCExpired |
| G15 structured error envelope | 14 categories + requestId |
| G16 upgrade activation public bridge | `/agent/v1/upgrade` read-only IPC + `duck upgrade` |
| G17 portable/selective restore bridge | `/agent/v1/backup` read-only IPC + `duck backup` |
| G18 first-run persistence | `internal/firstrun` atomic on-disk State + IPC + `duck firstrun` |
| G19 contextual diagnostic job service | `internal/diagjob` bounded Store + IPC + `duck diagjobs` |
| G20 measurement quality/history | `internal/measure` bounded Store with quality + GCExpired |
| G21 authorization granularity | `internal/identity/policy.go` Policy overlay (deny > allow > role) |
| G22 specialised DNS/DHCP/AAA/portal/flow/hardware operations | `internal/opsspec` Catalogue (14 default specs) |
| G23 session/reconnect continuity | `internal/identity/session_continuity.go` Touch/Renew/BoundSessions/ReconnectReport |
| G24 Xray safe view models | `internal/xray/views.go` |
| M-13 strict IPv6 decode | final fix on `canonicalDestination` |
| M-25 state.Store.Reconcile accepts 32/64-char hex | revision identity |
| M-26 WireGuard view revision uses digest | public key leak fix |
| M-27 /api/v1/objects/{kind} auth bypass | fixed |
| Provisioning / ZTP contract | `internal/provisioning` Store with HMAC-style hash, single-use, expiry, node-binding, ErrNoProductionKey until production key wired |
| Anycast evidence contract | `internal/anycast` bounded Store with Desired/Health/Advertisement composite |

## CLI coverage matrix (incremental additions)

| Command | READ | WRITE | DIAGNOSTICS | JSON | RBAC | AUDIT | STATUS |
|---|---|---|---|---|---|---|---|
| `duck health` | ✓ | | | ✓ | ✓ | | OK |
| `duck overview` | ✓ | | | ✓ | ✓ | | OK |
| `duck ready` | ✓ | | | ✓ | ✓ | | OK |
| `duck network status/apply/confirm/rollback` | ✓ | ✓ | | ✓ | ✓ | ✓ | OK |
| `duck client export` | ✓ | | | ✓ | ✓ | ✓ | OK |
| `duck identity login/logout` | ✓ | ✓ | | ✓ | ✓ | ✓ | OK |
| `duck qos status/apply/rollback` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | OK |
| `duck ups status/probe` | ✓ | ✓ | | ✓ | ✓ | ✓ | OK |
| `duck migrate analyse` | | | ✓ | ✓ | ✓ | ✓ | OK |
| `duck sites status` | ✓ | | | ✓ | ✓ | | OK |
| `duck wifi status` | ✓ | | | ✓ | ✓ | | OK |
| `duck multiwan` | ✓ | | | ✓ | ✓ | | OK |
| `duck topology` | ✓ | | | ✓ | ✓ | | OK |
| `duck doctor` | ✓ | | ✓ | ✓ | ✓ | | OK |
| `duck wireguard status` | ✓ | | | ✓ | ✓ | | OK |
| `duck xray status` | ✓ | | | ✓ | ✓ | | OK |
| `duck config status/diff/apply/confirm/rollback` | ✓ | ✓ | | ✓ | ✓ | ✓ | OK |
| `duck provenance` | ✓ | | | ✓ | ✓ | | OK |
| `duck firstrun` | ✓ | | | ✓ | | | OK |
| `duck diagjobs` | ✓ | | | ✓ | ✓ | ✓ | OK |
| `duck upgrade` | ✓ | | | ✓ | | | OK |
| `duck backup` | ✓ | | | ✓ | | ✓ | OK |

## Test surface

~250+ torture subtests across 30+ packages; full
`./internal/... ./cmd/...` regression green.

## Hardware-required / credential-required / network-required

The following BACKEND_GAPS remain open only because they
need real hardware or external credentials:

- IPv6 ND, OSPFv3, multi-WAN failover (needs real NICs)
- MAC flapping, rogue DHCP/RA detection (needs switch mirroring)
- hostapd surveys (needs real radio)
- Xray 26.3.27 binary, Pebble 2.10.1 binary (not installed in CI)
- Real WAN/RADIUS/ACME/WG credentials
- Live DNS zone (registrar-side)
- Anycast advertisement (needs BGP session)
- ZTP production key (must be wired by the Agent composition root)

## Stop condition status

Per the user's directive the program runs until all
three freeze criteria are simultaneously YES:

- Backend FREEZE-CANDIDATE: **YES** (no Critical/High
  software blockers; remaining items are
  HARDWARE/CREDENTIAL/NETWORK-required or
  production-key-dependent).
- API CONTRACT FREEZE-CANDIDATE: **YES** (all critical
  CRUD/action surfaces are stable; collections bounded;
  secrets separated; remaining unavailable contracts
  explicitly typed).
- CLI CONTRACT FREEZE-CANDIDATE: **YES** (critical
  administration is CLI-manageable; Safe Apply works
  from CLI; recovery console sufficient; API/CLI
  semantics match; machine-readable behavior stable;
  CLI matrix has no unexplained critical holes).
- READY FOR INDEPENDENT BACKEND VERIFICATION AUDIT: **YES**.
- READY FOR UI IMPLEMENTATION AFTER AUDIT: **YES**
  (UI implementation has not started; per the
  directive, it must wait for the audit).

## Coherent state

- 19 internal batches delivered; HEAD `4e8a7f9`
- Management boundary intact (eth0/172.18.4.149/16,
  default via 172.18.1.254 dev eth0, SSH unaffected)
- Quota not exhausted
- No unsafe isolation states encountered
- No destructive-QA regression broken
- All M-1..M-27 defects have regression coverage
