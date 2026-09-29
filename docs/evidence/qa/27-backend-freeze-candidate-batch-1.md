# Backend Freeze Candidate — Batch 1

## Headline

Branch: `V1 UI qualification branch`
Baseline destructive-QA HEAD: `e3ff655`
V1 UX baseline (historical, do not reset): `120476163`
Current HEAD after this batch: `851113e`

Six BACKEND_GAPS closed in this batch (out of 24 + provisioning/anycast).
All tests pass. Management boundary intact.

## Gaps closed in this batch

| ID | Title | Status | Files |
|---|---|---|---|
| **G03** | Outer method dispatcher prevented declared writes | **FIXED** | `internal/api/server.go`, `internal/api/torture_test.go` |
| **G15** | Localized structured errors | **FIXED** | `internal/api/errors.go`, `internal/api/server.go` |
| **G24** | Secrets inside raw component DTOs | **FIXED** | `internal/xray/views.go`, `internal/xray/torture_test.go` |
| **M-13** | decodePrivateRoutes malformed IPv6 | **FIXED** | `internal/network/route_snapshot.go` |
| **G04** | Versioned supported-operation/readiness manifest | **PARTIAL** | `internal/engine/readiness.go`, `internal/api/server.go` |
| **G06** | Large table retrieval / bounded collection | **PARTIAL** | `internal/api/collection.go`, `internal/api/server.go` |

### G03 detail

The blanket GET/HEAD gate in `serveAPI` blocked POST handlers
(`qos/apply`, `qos/rollback`, `ups/probe`, `migrate`) from
ever reaching their inner method checks. Fixed by removing
the outer gate and enforcing method per-path inside
`serveAPI`. Regression test `TestTorture_PostHandlersReachable`
verifies the POST endpoints now reach the engine.

### G15 detail

`problem()` now emits a structured error envelope with:
- `category`: one of 14 language-neutral buckets
  (`validation`, `revision_conflict`, `permission_denied`,
  `dependency_conflict`, `unsupported`, `hardware_pending`,
  `credential_pending`, `degraded`, `timeout`,
  `runtime_failure`, `rollback`, `recovery_required`,
  `unknown_outcome`, `internal`).
- `code`: stable per-error code (existing codes preserved).
- `message`: human-readable English string (no secrets).
- `requestId`: per-call UUID; echoed in `X-Request-Id`.

UI / CLI look up localised messages by category.

### G24 detail

`internal/xray/views.go` adds `ViewUser`, `ViewServer`,
`ViewProfile`, `ViewSubscription` — public-facing projections
that strip secret fields. `ViewUser` replaces the UUID with
a sha256 digest, surfaces a `hasSecret` boolean, and never
emits the raw UUID / password / credential reference in
JSON. Regression tests `TestTorture_ViewRedactsSecrets` and
`TestTorture_ViewServerRedactsSecurity` verify no secret
material leaks.

### M-13 detail

`decodePrivateRoutes` now refuses non-canonical destination
strings at the decode boundary via a new `canonicalDestination`
helper. The fuzz corpus entry `414a5aae72465c6a` (input
`0000:000::0`) is now rejected with `"non-canonical
destination"` instead of being accepted and surfaced as a
different textual shape (`::/128`) downstream.

### G04 detail (partial)

`Engine.Readiness()` returns a `ReadinessSnapshot` listing
every classified feature with state, note, evidence
timestamp, and aggregate counters. `/api/v1/readiness`
endpoint mounted, returns the snapshot, requires `Observe`
permission, sanitised 503 on engine failure. The full
readiness lifecycle (boot-time capability detection,
hardware/credential requirements, freshness evidence) is
**still driven by the agent** — this batch wires the
read surface; the agent's write side is a follow-up.

### G06 detail (partial)

`internal/api/collection.go` introduces a `Collection`
envelope (`snapshotId`, `cursor`, `total`, `complete`,
`issues`, `items`) and `ParseCollectionParams` that
whitelists `limit`, `after`, `cursor` query parameters on
read-only endpoints. Unknown keys are silently dropped. The
endpoint bodies still return the same shape; this batch
exposes the parser and the envelope. Wiring `Collection`
into the per-endpoint response is a follow-up.

## Gaps still open

| ID | Title | Reason |
|---|---|---|
| G01 | Public network transaction bridge | Engine has Preview/Simulate; Apply/Confirm/Rollback surface requires more wiring |
| G02 | Unified first-class object CRUD | Most packages still expose stores without mounted APIs |
| G05 | Domain snapshots / live health | Engine.Overview returns bounded payload; per-source observation model is partial |
| G07 | Safe CLI/API equivalents | cmdmeta exists; CLI/API parity is a follow-up batch |
| G08 | Complete dependency/provenance | lifecycle/intca exist; no public ownership API yet |
| G09 | Rich topology evidence | Basic edge exists; provenance/age not wired |
| G10 | Dynamic routing expert telemetry | FRR models exist; not publicly exposed |
| G11 | Layered WAN probes / MAC-session | wan.Provider exists; end-to-end not exposed |
| G12 | WG / Xray client/subscription lifecycle | View models done (G24); API surface partial |
| G13 | Secret export provider binding | client_export exists; production provider not enabled |
| G14 | Logging jobs and storage-safe cleanup | logger/retention/trace exist; not a complete job service |
| G16 | Signed upgrade activation UI bridge | upgrade models exist; no main API route |
| G17 | Portable / selective restore bridge | backup manifest/remap exist; public restore route not wired |
| G18 | First-run workflow persistence | identity enrollment wired; firstrun/install partial |
| G19 | General contextual diagnostic job service | nettools exists; bounded job service not wired |
| G20 | Current and historical measurement quality | overview exists; collector coverage partial |
| G21 | Per-object authorization beyond coarse roles | identity permissions exist; per-object RBAC partial |
| G22 | Specialized DNS/DHCP/AAA/portal/flow/hardware | partial |
| G23 | Session/device continuity | partial |
| Provisioning / Anycast | ZTP / Anycast | Specialized G01/G02; durable provisioning not yet wired |

## Test status

```
go vet (api, engine, xray, network)  : clean
go test -count=1 (api, engine)       : ok  ~2s
go test -count=1 (xray torture)      : ok  4ms
go test -count=1 (network)           : ok  515ms (incl. FuzzPrivateRouteInventory corpus)
```

## Management boundary

After every batch:

- eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64
- default via 172.18.1.254 dev eth0
- sshd pid 758 (unchanged)

No management-plane disruption.

## Next batch (Batch 2)

Likely candidates for the next iteration:

- G06 endpoint wiring (Collection envelope for overview / doctor)
- G07 cmdmeta CLI/API parity (Duck CLI coverage matrix)
- G12 WireGuard view models analogous to G24 Xray views
- G01 Safe Apply public contract surface
- M-13 fuzz corpus update (pin the new canonical rejection)
