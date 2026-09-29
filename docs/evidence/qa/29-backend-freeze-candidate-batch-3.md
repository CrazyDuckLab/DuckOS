# Backend Freeze Candidate — Batch 3 (interim)

## Headline

Branch: `V1 UI qualification branch`
HEAD: `74e3889`
Batch 2 final HEAD: `003373a`
Batch 1 final HEAD: `9dc55d0`

## Gaps advanced in this batch

### M-25 revision identity mismatch — **FIXED**

- `state.Store.Reconcile` (and the corresponding
  `validateDatabase` check) originally required `Before` and
  `Candidate` to be 32-char hex. The natural shape of
  `state.Revision.ID` is a 64-char sha256 hex, so the Engine
  surface could never reconcile a real outcome. The fix
  accepts either 32-char or 64-char hex.
- Regression: `TestG01_M25_ReconcileAcceptsRevisionIDs`
  exercises a Prepare → RollingBack → RecoveryRequired →
  Reconcile flow with the natural 64-char shape and asserts
  no error.

### G02 canonical object contract — **PRIMITIVE DONE**

- New package `internal/object/` with:
  - `ObjectRef{Kind, ID, Revision}` — canonical triple.
  - `Collection{SnapshotID, Cursor, Total *int, Complete, Issues, Items}` —
    bounded list envelope.
  - `Availability` — six language-neutral categories
    (`available`, `experimental`, `hardware-pending`,
    `credential-pending`, `unsupported`, `unknown`).
  - `PermissionSet{Observe, Configure, Action, ExportSecret, Delete}` —
    per-row RBAC projection.
  - `SupportedActions` — language-neutral action verbs.
  - `OwnerRef` — generator / owner pointer.
  - `ViewObject` — canonical row projection (ref, displayName,
    desired, observed, availability, owner, supportedActions,
    permissions, tags, updatedAt).
  - `NewID()` — fresh 16-byte hex id.
- 8 torture subtests pin the contract: round-trip, total
  unknown (null), availability categories, action
  membership, permission set, owner ref, id uniqueness
  (256 ids, all distinct), view-object envelope.

### G12 WireGuard views — **DONE**

- `internal/wireguard/views.go`:
  - `ServerView` — interface / listen / addresses / DNS /
    MTU / public-key digest (sha256[:8] hex) / hasPrivateKey
    (presence only) / peer count / supported actions.
  - `PeerView` — server ref / public-key digest /
    hasPresharedKey (presence only) / allowedIPs / endpoint /
    intent / enabled / supported actions.
  - `ViewServer` and `ViewPeer` never serialise the raw public
    key, the private key, the preshared key, or any VaultID
    / Revision / KeyID. ObjectRef.Revision is a digest, not the
    raw key.
- 4 torture subtests pin the redaction contract.

### Xray views also hardened (regression on M-26)

- `ViewUser` ObjectRef.Revision now uses `uuidDigest(uuid)` so
  the credential never reaches the JSON surface.
- Existing `ViewServer` / `ViewProfile` / `ViewSubscription`
  ObjectRefs preserved (no Revision field for those).

### G13 secret export torture — **REGRESSION DONE**

- 3 new subtests in
  `internal/api/secret_export_torture_test.go`:
  - `TestG13_ExportRejectsGet` — GET → 405 with `Allow: POST`.
  - `TestG13_ExportProviderErrorNeverLeaksMessage` — provider
    errors that may carry credential text are mapped to a
    sanitised 403, never serialised.
  - `TestG13_ExportSetsNoStore` — every export response
    carries `Cache-Control: no-store, private` and
    `Pragma: no-cache`, including the 405 path.

## Gaps remaining

| ID | Title | Status |
|---|---|---|
| G06 | Collection endpoint wiring | Parser + envelope from Batch 1; endpoint bodies still return existing shapes |
| G04 | Honest readiness evidence wiring | Read endpoint from Batch 1; agent write side pending |
| G08 | Dependency / provenance public surface | Pending |
| G05 | Live health observations | Pending |
| G09 | Rich topology evidence | Pending |
| G10 | Dynamic routing expert telemetry | Pending |
| G11 | Layered WAN probes | Pending |
| G14 | Bounded logging / trace jobs | Pending |
| G16 | Upgrade activation public bridge | Pending |
| G17 | Portable / selective restore public bridge | Pending |
| G18 | First-run workflow persistence | Pending |
| G19 | Contextual diagnostic job service | Pending |
| G20 | Measurement quality / history | Pending |
| G21 | Per-object RBAC beyond coarse roles | Partial (G02 set surfaces) |
| G22 | Specialised DNS / DHCP / AAA / portal / flow / hardware | Pending |
| G23 | Session / reconnect continuity | Pending |
| Provisioning / ZTP / Anycast | Secure enrollment, durable tokens, advertised evidence | Pending |

## Test status

```
go vet   (engine, api, xray, wireguard, object, state) : clean
go test  (engine -run TestG01_)                        : ok 90ms
go test  (api -run TestG13_)                           : ok 12ms
go test  (xray -run TestTorture_)                     : ok
go test  (wireguard -run TestTorture_)                 : ok
go test  (object -run TestTorture_)                    : ok
```

## Management boundary

After every commit in this batch:

- eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64
- default via 172.18.1.254 dev eth0
- sshd pid 758 (unchanged throughout)

No management-plane disruption.

## New findings this batch

- **M-26 (discovered, fixed)**: WireGuard `ViewServer.Revision`
  and `ViewPeer.Revision` were serialising the raw public
  key in the ObjectRef.Revision slot. Now uses a sha256
  digest. Caught by `TestTorture_ViewRevIncludesRevision`.

## Next batch

- G06 endpoint wiring: wrap existing read-only endpoints
  (overview, doctor, qos, sites, wifi, multiwan) in
  `Collection` envelope.
- G08 dependency / provenance public surface.
- Final freeze validation: race, full vet, full test
  (relevant scope), CLI/API parity, security regression,
  management boundary confirmation.
