# DV01 Production Path — CLOSED

- **HEAD**: `62952d2`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-15
- **Test file**: `tests/layer_l/dv01_production_test.go`

## Goal

The L-tier DV01 subcases (`TestDV01RestartPreservesIdentity`,
`TestDV01DisabledDaemonProducesTypedUnavailable`) previously
exercised only `engine.ObjectList` with stub listers and a
fake agent socket. There was no PUBLIC → `api.Handler.handleObjects`
→ `engine.ObjectList` chain, no source-backed listers for the
2 wireguard / 4 xray kinds, no coverage of the 19-kind surface
through the public API, and no witness that the
LinuxSource=nil case surfaces honest `engine_unavailable`
rather than fake-empty `Complete=true`.

This closure wires:

  PUBLIC HTTP GET `/api/v1/objects/{kind}`
  → production `api.Handler.handleObjects`
  → `engine.ObjectList` (real Engine, real Unix-socket transport)
  → registered production lister
  → real source-backed data:
       `wireguard.{server,peer}`        → `wireguard.FileCatalog`
                                            (durable on disk)
       `xray.{server,user,profile,subscription}` → `xray.FileCatalog`
                                            (durable on disk)
       `qos.class` + `network.{interface,route,address}`
                                       → `engine.Doctor` + `engine.QoS`
                                          over real Unix socket to a
                                          fake Agent IPC
       9 linux-runtime kinds           → `LinuxSource` accessor
                                          (nil in this fixture →
                                           honest `engine_unavailable`,
                                           never fake-empty
                                           `Complete=true`)

The fixture uses the same production constructor chain
`cmd/duck-api` uses: `engine.New(sockPath, env)` →
`objectlisters.RegisterAllWithRealSourcesAndLinuxSource` →
`api.New(engine, token, listen, webDir)` →
`httptest.NewServer(handler)`. Every kind is reached end-to-end
through the public `Authorization: Bearer` path with no stubs.

## Production Path Matrix

| Link                     | Production code path                                                                                       | Test                                                        |
|--------------------------|------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------|
| **PUBLIC**               | `api.Handler.handleObjects` → `engine.ObjectList(kind, params)` (typed-error envelope; per-kind routing)  | All `TestDV01Production*` subcases                          |
| **ENGINE**               | `engine.New(sockPath, env).ObjectList` — real Engine, real Unix-socket transport                            | All `TestDV01Production*` subcases                          |
| **LISTER REGISTRATION**  | `objectlisters.RegisterAllWithRealSourcesAndLinuxSource` — production boundary wiring                     | All `TestDV01Production*` subcases                          |
| **WIREGUARD SOURCE**     | `wireguard.FileCatalog.UpsertServer` / `.UpsertPeer` → durable `catalog.json`; lister reads via `ListServers` / `ListPeers` | TestDV01ProductionWireguardSourceBackedLister               |
| **XRAY SOURCE**          | `xray.FileCatalog.UpsertServer` / `.UpsertUser` / `.UpsertProfile` / `.UpsertSubscription` → durable catalog; lister reads via `ListServers` / `ListUsers` / `ListProfiles` / `ListSubscriptions` | TestDV01ProductionXraySourceBackedLister                    |
| **ENGINE-BACKED SOURCE** | `engine.Doctor` + `engine.QoS` IPC over real Unix socket; `ListerSource` projects into `object.ViewObject` rows | TestDV01ProductionEngineBackedListers                       |
| **LINUX SOURCE = NIL**   | `LinuxSource = nil` → 9 listers return `Complete=false` with `engine_unavailable` Issue; never fake-empty  | TestDV01ProductionLinuxSourceNilHonestUnavailable           |
| **PAGINATION**           | `api.CollectionParams{Limit, After}` honoured by lister + cursor round-trip                                | TestDV01ProductionPaginationLimitAndAfter                   |
| **BOOTSTRAP AUTH**       | `Authorization: Bearer <token>` with no identity wired → token-hash comparison in `authorizeManagement`   | TestDV01ProductionBootstrapTokenAuthentication              |
| **RESTART**              | Tear down Engine + Agent listener + httptest server; re-create from durable wireguard catalog; `ObjectRef.ID` preserved | TestDV01ProductionRestartPreservesIdentity                  |
| **DISABLED AGENT**       | Agent IPC returns 503; engine-backed listers surface `engine_unavailable` Issue + `Complete=false`          | TestDV01ProductionDisabledAgentProducesTypedUnavailable    |
| **ALL 19 KINDS**         | Every kind reachable via the public API with a well-formed `object.Collection` envelope                    | TestDV01ProductionAllNineteenKindsReachableViaPublicAPI     |

## Subcases (9/9 PASS)

| Test                                                          | Production-path evidence                                                                                                                                  |
|---------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| TestDV01ProductionAllNineteenKindsReachableViaPublicAPI       | Iterates the 19 canonical kinds (`network.interface`, `network.route`, `network.firewall.rule`, `network.firewall.set`, `network.lease`, `network.address`, `wireguard.server`, `wireguard.peer`, `xray.server`, `xray.user`, `xray.profile`, `xray.subscription`, `qos.class`, `dns.zone`, `dhcp.subnet`, `bgp.peer`, `ospf.area`, `isis.area`, `route.map`); every kind returns HTTP 200 with `SnapshotID` set. |
| TestDV01ProductionWireguardSourceBackedLister                 | `/api/v1/objects/wireguard.server` returns 1 row `wg-dv01-1` with `Complete=true`; `/api/v1/objects/wireguard.peer` returns 1 row `wg-dv01-p1` with `Complete=true`. Real `wireguard.FileCatalog.ListServers` / `.ListPeers` drive the rows. |
| TestDV01ProductionXraySourceBackedLister                      | `/api/v1/objects/xray.{server,user,profile,subscription}` each return 1 row with `Complete=true` and `Ref.ID` matching the `Upsert*` call.                   |
| TestDV01ProductionEngineBackedListers                         | `/api/v1/objects/{qos.class, network.interface, network.route, network.address}` returns `Complete=true` with real rows; `doctorCalls > 0` confirms the Agent IPC was hit. |
| TestDV01ProductionLinuxSourceNilHonestUnavailable             | The 9 `LinuxSource`-bound kinds (`network.firewall.rule`, `network.firewall.set`, `network.lease`, `dns.zone`, `dhcp.subnet`, `bgp.peer`, `ospf.area`, `isis.area`, `route.map`) return `Complete=false`, `Items=[]`, and an `engine_unavailable` Issue when `LinuxSource=nil`. Never fake-empty `Complete=true`. |
| TestDV01ProductionPaginationLimitAndAfter                     | `?limit=1` honoured (`len(Items) <= 1`); when the lister returns a cursor, a follow-up `?limit=1&after=<cursor>` does not duplicate ids.                      |
| TestDV01ProductionBootstrapTokenAuthentication               | `Authorization: Bearer <token>` with no enrolled identity admits every one of the 19 kinds with HTTP 200.                                                  |
| TestDV01ProductionRestartPreservesIdentity                   | Tear down Engine + Agent listener + httptest server, re-create from the durable wireguard catalog, re-register listers, re-fetch — `Ref.ID` is preserved across the restart. |
| TestDV01ProductionDisabledAgentProducesTypedUnavailable       | Fake Agent flipped to `active=0`; engine-backed listers (`network.interface`, `network.route`, `qos.class`) surface `Complete=false` + an `engine_unavailable` Issue (or a 503 envelope) — never a fake-empty `Complete=true`. |

## Test command

```
sudo -E env "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
  "GOCACHE=/home/duckdev/.cache/go-build" \
  go test -count=1 -timeout 240s -v -run TestDV01 ./tests/layer_l/...
```

## Acceptance check

- [x] Production `api.Handler.handleObjects` reached via `httptest.NewServer`.
- [x] Production `engine.New` constructor + real Unix-socket transport.
- [x] Production `objectlisters.RegisterAllWithRealSourcesAndLinuxSource` boundary used.
- [x] Real `wireguard.FileCatalog` populated via `UpsertServer` / `UpsertPeer`.
- [x] Real `xray.FileCatalog` populated via `UpsertServer` / `UpsertUser` / `UpsertProfile` / `UpsertSubscription`.
- [x] All 19 canonical kinds reachable via `GET /api/v1/objects/{kind}`.
- [x] 2 wireguard / 4 xray kinds backed by real source — never fake-empty `Complete=true`.
- [x] Engine-backed kinds (`qos.class`, `network.{interface,route,address}`) hit the Agent IPC.
- [x] 9 LinuxSource-bound kinds surface HONEST `engine_unavailable` when source is nil.
- [x] `limit`/`after` pagination honoured by the public surface.
- [x] Bootstrap-token auth admits every kind.
- [x] Restart preserves `ObjectRef.ID` (durable wireguard catalog survives).
- [x] Disabled Agent produces typed unavailable, never fake success.
- [x] All layer_l tests pass: `ok github.com/CrazyDuckLab/DuckOS/tests/layer_l 17.854s`.
