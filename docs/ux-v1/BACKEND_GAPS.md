# Backend gaps and UI integration boundaries

This is a design integration register, not authorization to weaken existing code. Baseline `120476163e347f875f46098c546ee6022e5efbd1`. “Proposed” means contract design, not an existing endpoint. The next UI implementer must inspect the relevant current files for changes, not restart repository-wide design.

## Binding policy

Bind current reachable read/auth/preview APIs through validated typed adapters. Do not call private Agent URLs from the browser or route the UI around API authorization. A missing operation yields an explicit Unavailable result; never a resolved Promise with success-shaped fixture data. Unmounted internal WireGuard/Xray handlers are not equivalent to browser endpoints.

A capability has two distinct questions: “Does this build expose an operation?” and “Can this node execute it with current hardware/credentials/permissions?” A feature enum answers neither by itself. Hide unauthorized secret data at the server; keep generally discoverable product help where safe.

## Gap register

| ID | Missing or unsafe-to-assume contract | Evidence / pages | Required contract and acceptance |
|---|---|---|---|
| G01 | Public network transaction bridge and reliable progress | api/network.go only preview/simulate; cmd/duck-api startup; CLI points network commands at private /agent paths; P002/network editors | Authorized Engine-mediated candidate/status/apply/confirm/rollback, expected revision, request ID, receipt, safe admission and same logical CLI model; no private IPC calls from browser |
| G02 | Unified first-class object CRUD | Most packages expose in-memory stores/renderers rather than mounted object APIs; all editors | Typed object kind/ID/revision and declarative patch/intent, validation, persistence, bounded list/detail, dependencies and backend-supported actions |
| G03 | Outer method dispatcher prevents declared writes | api/server.go rejects non-GET/HEAD before QoS POST, UPS probe and migrate handlers; P006/P067/P075 | Correct method routing + regression auth/body/CSRF/origin checks before controls become enabled; do not “fix” by GET mutation |
| G04 | Versioned supported-operation/readiness manifest | capability and chaos status models; cmdmeta registry not mounted | Per feature: implementation availability, hardware/credentials, disabled intent, evidence/time, permissions and reasons; unknown preserved |
| G05 | Domain snapshots and meaningful live health | Several Agent hosts start with empty topology/zero timestamps; sites/wifi/multiwan | Per-source observations, age, state, failure reason and coverage; no configured-state-to-healthy conversion |
| G06 | Large table retrieval/search | Current overview whole bounded payload and no query parameters; network config caps 64 interfaces/512 routes/512 firewall rules | Cursor/snapshot/server filter/sort/total-known, selected-query semantics and limits; virtualize only retrieved data; never fake filtering beyond loaded data |
| G07 | Safe CLI/API equivalents | cmdmeta is a template registry; CLI has handlers for operations public API doesn't mount | Authoritative registered/supported operations, schema-aware escaping and secret references, staging/apply semantics and version; no invented shell/API snippets |
| G08 | Complete dependency/provenance/clone/detach | lifecycle/intca/canary components exist; no universal public ownership API | Forward and reverse edges, owner generator, generated IDs, completeness, impact, intentional detach, clone identity/key conflict validation |
| G09 | Rich topology evidence | Edge has from/to/kind/detail, not structured confidence/age/provenance; Graph capped 64 KiB | Per-edge observation kind/source/age/expiry/confidence, logical vs physical, namespace ID, bounded neighborhood retrieval and unknown nodes |
| G10 | Dynamic routing expert telemetry | frr/dynamicrt/isis/srmpls/evpn models; no mounted public monitoring APIs | Native neighbor/AFI/SAFI/LSDB/received/advertised/rejected/selected/FIB/recursion details, scoped refresh/reset, policy trace and capability honesty |
| G11 | Layered WAN probes / MAC-session identity | wan.Provider fields and health models; no verified public end-to-end acquisition | Source-bound link/L2/address/gateway/DNS/Internet/quality evidence, family/time, external-vantage/public-IP confidence; MAC clone only for verified local neighbor/session |
| G12 | Complete WireGuard/Xray client/subscription lifecycle | internal API handlers, export components and runtimes exist; startup mounting/read lifecycle incomplete | Public list/detail/intent/lifecycle with rollback, secret references, revision invalidation, compatible exports; no raw JSON normal management; refresh-required state |
| G13 | Secret export provider binding and security invariants | client export handler present; production provider not enabled in cmd/duck-api | Authenticated explicit reveal/export, same revision/payload for QR/download/copy, no-store, audit metadata only, role/object restrictions, rotation revocation; existing QR tests preserved |
| G14 | Logging jobs and storage-safe cleanup | logger/retention/trace models not a complete mounted operation service | Bounded filter/live stream/export, level lease with server cleanup on crash/cancel, sink-failure alert + tiny bounded fallback, drop counters and protected audit |
| G15 | Localized structured errors/help/preferences/search | Current errors mostly code/message English; no full four-locale registry | Stable message IDs/typed safe parameters, localized help, account preference scope, permission-filtered search; browser-only preference fallback explicitly labeled |
| G16 | Signed upgrade activation UI bridge | upgrade models/orchestrator/storage; no main API route | Signature/compatibility/space/schema checks, Active/Previous/Candidate evidence, progress, reboot connection semantics, rollback proof, local .duck path without cloud |
| G17 | Portable/selective restore and hardware mapping bridge | backup manifest/remap/plan components | Inspect encrypted upload, auth/limits, schema compatibility, secret/recovery policy, old/new NIC evidence, safe staged restore and connectivity receipt |
| G18 | First-run workflow persistence and network admission | identity enrollment mounted; firstrun/install/management components | Real step outcomes, source evidence, offline readiness, secure owner, server role without LAN, no arbitrary success flags |
| G19 | General contextual diagnostic job service | nettools request/result models; capture/speedlab managers and Doctor partially exposed | Source/VRF binding, authorize, traffic/resource limits, cancellation, job/result pagination, expiry, sensitive artifact access and redaction |
| G20 | Current and historical measurement quality | overview null CPU/sample snapshot; various collectors/stores | Units, sampling, reset/gap semantics, timestamps/time trust, bounded history, actual collector coverage; no interpolated fake runtime |
| G21 | Per-object authorization beyond coarse roles | identity supports built-in permissions; sites/wifi read classification currently ConfigureNetwork | Server-enforced per-object/action rights, safe permission discovery and no read-to-write escalation; UI policy must match actual build |
| G22 | Specialized DNS/DHCP/AAA/portal/flow/hardware operations | partial declarative stores and capability models | Protocol-specific fields, reliable activation/readback/failure/rollback and supported variants; read-only availability panels until qualified |
| G23 | Session/device continuity across browser sleep/reconnect | no complete public transaction timing envelope | Server remaining duration/monotonic anchor, final receipt, safe reconnect target identity/TLS verification; never widen origins or confirm unknown transaction |
| G24 | Secrets inside raw component DTOs or imported lines | Xray User includes UUID/Password fields; migration plans preserve Raw; application DTO reuse unsafe | Public allowlisted/redacted view models; no raw private fields in lists/diff/search/errors/support; import source is sensitive by default |

## Proposed view contracts

The following TypeScript is interface design, not a production implementation or existing wire schema. Actual adapters must validate known API responses and explicitly return gaps. Do not send these shapes to a guessed endpoint.

```ts
type Availability =
  | { kind: "available"; source: string; observedAt?: string }
  | { kind: "unavailable"; gapId: string; reasonId: string }
  | { kind: "hardware-pending" | "credential-pending" | "unsupported";
      reasonId: string; requirementIds: string[] }
  | { kind: "unknown"; reasonId: string };

type Evidence<T> =
  | { kind: "observed"; value: T; source: string; observedAt: string;
      freshness: "fresh" | "stale"; timeTrust: "trusted" | "untrusted" | "unknown" }
  | { kind: "unknown"; reasonId: string }
  | { kind: "failed"; reasonId: string; retryable: boolean };

type ObjectRef = { kind: string; id: string; revision: string };
type ViewObject<TConfig, TRuntime> = {
  ref: ObjectRef;
  displayName: string;
  desired: TConfig; // public allowlisted configuration only, no secrets
  observed: Evidence<TRuntime>;
  availability: Availability;
  owner?: ObjectRef;
  permissions: { observe: boolean; configure: boolean; exportSecret: boolean };
  supportedActions: string[];
};

type Collection<T> = {
  items: T[];
  snapshotId: string;
  cursor?: string;
  total: number | null;
  complete: boolean;
  issues: { source: string; reasonId: string }[];
};
```

Transactional view needs requestId, transactionId, expected/base/current/candidate revisions, actual phase, supported actions, validation findings, ownership/impact, management evidence, server deadline/remaining duration, outcome receipt, rollback/recovery status and time trust. UI may not manufacture missing fields.

Secret export needs a separate non-cacheable type whose value is only passed to reveal/download/QR components; do not embed it in ViewObject or Collection. Display metadata can be cached under normal authorization; payload cannot.

## Exact current API adapter rules

- Overview: `shared/api/overview.schema.json` is canonical; generated types are not hand-edited. CPU usage null stays null. Services and nftables status are observations with their actual scope.
- Auth: status/get/current/users/sessions/audit and enrollment/login/logout/password/revoke follow `internal/api/identity.go` method map. No query credentials; safe content type; existing trusted-host/origin/TLS restrictions preserved.
- Preview/simulate: POST only; enrolled identity and ConfigureNetwork, fixed body cap, no URL parameters. They do not apply network state.
- Doctor/topology/QoS/UPS/Multi-WAN/sites/Wi-Fi: runtime-validate exact current shape; no library presence → green mapping. Sites/Wi-Fi current permission mismatch is a backend issue, not reason to retry with more powerful credentials.
- Client export: unavailable until provider is intentionally mounted. Do not source private keys directly from Vault or internal Agent endpoints.
- No networking/API-origin relaxation, private socket access, raw shell, unbounded read or production fixture fallback as UI implementation shortcuts.

## Implementation sequence for the UI task

1. Typed response validation and availability registry for reachable contracts; failing contract tests for gaps.
2. Shell/navigation/routes, modes/preferences, locale infrastructure, tokens, reusable tables/help/status.
3. Existing reachable overview/auth/preview/simulation/Doctor/topology views.
4. Complete page shells/editors as designed, backed by actual operation capabilities. Preserve unavailable functionality as honest gaps, not pretend saves.
5. Wire active operations only as backend bridges become real and qualified; assert request/response semantics and RBAC.
6. End-to-end state, keyboard, locale, scale and visual-regression acceptance.

A complete design can precede a complete backend. It must not be reported as a completed DuckOS UI implementation until the actual supported operations and all acceptance gates are implemented.
## Provisioning and Anycast boundary details

ZTP and Anycast have explicit GUI locations in INFORMATION_ARCHITECTURE, not only package-index entries. Their existing in-memory stores do not establish secure enrollment enforcement or live advertisement. Provisioning must not enable token issuance until authenticated, scoped, cryptographically generated, hashed, single-use, expiry- and node-bound durable handling is supplied. Anycast needs timestamped health and actual routing advertisement/withdrawal evidence. These specialize G01/G02 and the common secrets/observations gaps; no separate configuration system is proposed.
