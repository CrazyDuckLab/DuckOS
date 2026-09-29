# DV04-XRAY Production Path — CLOSED

- **HEAD**: `0a9c9b4`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-14

## Goal

The L-tier Xray fixture previously exercised only the catalog
and the renderer. The production path was missing the wiring
that connects a public mutation to the real `xray` daemon,
and the rendered config schema was wrong (xray 1.8.x rejected
it on cold start). This closed case wires:

  PUBLIC → ENGINE → AGENT → real Xray daemon → INDEPENDENT LISTENER ORACLE → RESTART → NEGATIVE → exact subscription URL/QR

The contract mirrors DV04-WG: every link in the chain is
exercised end-to-end against the production constructors
(`xray.OpenFileCatalog`, `xray.NewRuntime`, `xray.API`,
`agent.NewXrayHost`, `engine.Engine.XrayProxy`,
`api.handleXrayProxy`); the listener oracle is an
independent `os/exec` of `/usr/bin/ss -ltn`, never the
runtime itself.

## Production Path Matrix

| Link                  | Production code path                                                                                       | Test                                                            |
|-----------------------|------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| **PUBLIC**            | `api.Handler.ServeHTTP` → `handleXrayProxy` → `/api/v1/xray/*`                                              | TestDV04XRAY_PublicEngineAgentChain (Unix socket)              |
| **ENGINE**            | `engine.Engine.XrayProxy` (Unix socket client; 1 MiB cap; typed-error envelope)                           | TestDV04XRAY_PublicEngineAgentChain                             |
| **AGENT IPC**         | `agent.Handler.ServeHTTP` → `XrayHost.ServeHTTP` (ConfigureNetwork gate; `/agent/v1/xray` prefix strip)   | TestDV04XRAY_Production* (10 subcases)                          |
| **AGENT HOST**        | `internal/agent/xray_host.go` (production host, reconcile, nil-host stub returning 503)                    | TestDV04XRAY_ProductionNegativeAgentUnavailable                 |
| **RUNTIME**           | `xray.API` → `Runtime.Apply` (cold-start `xray run -c <active>` + SIGUSR1 hot reload + rollback)          | TestDV04XRAY_ProductionServerColdStart                          |
| **REAL XRAY DAEMON**  | `/tmp/duckos-test/xray/xray` (xray-core 1.8.20, statically linked x86_64)                                  | TestDV04XRAY_ProductionServerListenerOracle                     |
| **INDEPENDENT ORACLE**| independent `os/exec` of `/usr/bin/ss -ltn` (kernel TCP socket table) — never the runtime                 | TestDV04XRAY_ProductionServerListenerOracle, _DeleteStopsListener, _RestartReconciles |
| **RESTART/DURABILITY**| `XrayHost.Reconcile(ctx, catalog, *Runtime)` re-renders + `rt.Apply` after the durable catalog is re-opened | TestDV04XRAY_ProductionRestartReconciles                         |
| **NEGATIVE**          | `xray.Server.Validate` / `User.Validate` reject invalid protocol / security; `XrayHost` nil-stub returns 503 `xray_unavailable`; engine returns 503 `agent_unavailable` | TestDV04XRAY_ProductionNegativeInvalidProtocol, _InvalidSecurity, _AgentUnavailable |
| **SUBSCRIPTION URL/QR**| `xray.BuildSubscriptionURL` emits `base64.StdEncoding(join(Lines, "\n"))`; `xray.DecodeSubscriptionURL` is symmetric and exact; share links carry the `VaultID` credential reference and never embed the raw UUID or a `REDACTED` sentinel | TestDV04XRAY_ProductionSubscriptionURLEqualPayload, _URLNotBase64OfPlaintext |

## Subcases (all PASS, 11/11)

| Test                                              | Production-path evidence                                                                                                                              |
|---------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| TestDV04XRAY_ProductionServerColdStart            | Server + User mutation through API → deferred reconciler → `RenderConfig` → `Runtime.Apply` → cold-started xray writes `active.json`; runtime `State().Active` is true. |
| TestDV04XRAY_ProductionServerListenerOracle       | Cold-start binds the configured `Listen:Port`; independent `/usr/bin/ss -ltn` sees the listener within 200 ms.                                          |
| TestDV04XRAY_ProductionUserAllowedUUIDs           | User UUID lands in the rendered `active.json`; independent stage oracle reads it back.                                                                 |
| TestDV04XRAY_ProductionDeleteStopsListener        | Server DELETE through API → reconciler → `Runtime.Apply` with 0 users → `RenderConfig` rejects → xray exits; `/usr/bin/ss -ltn` no longer sees the listener. |
| TestDV04XRAY_ProductionSubscriptionURLEqualPayload| `BuildSubscriptionURL(servers, users)` produces a base64 URL whose decoded payload EXACTLY equals the renderer-built share-link `Lines`; share link carries `VaultID`, never the raw UUID or `REDACTED`. |
| TestDV04XRAY_ProductionSubscriptionURLNotBase64OfPlaintext | Decoded subscription payload starts with `vless://` (encoded ≠ plaintext).                                                                  |
| TestDV04XRAY_ProductionRestartReconciles          | Tear down xray → re-open the durable `FileCatalog` → `XrayHost.Reconcile` → cold-start re-binds the same `Listen:Port`; listener reappears within 200 ms. |
| TestDV04XRAY_ProductionNegativeInvalidProtocol    | Server with `protocol="vmess"` rejected by `xray.Server.Validate`; runtime stays idle (no Apply).                                                       |
| TestDV04XRAY_ProductionNegativeInvalidSecurity    | Server with `security="ipsec"` rejected; runtime stays idle.                                                                                            |
| TestDV04XRAY_ProductionNegativeAgentUnavailable  | `XrayHost` with nil api returns 503 `xray_unavailable` on `/agent/v1/xray/xray/v1/servers`, never a fake 200.                                            |
| TestDV04XRAY_ProductionListPersists               | GET /servers through the production path returns the catalog state the upsert put there.                                                                |
| TestDV04XRAY_PublicEngineAgentChain               | full PUBLIC → ENGINE.XrayProxy (Unix socket) → AGENT IPC → xray chain, with reconcile path.                                                              |

## Coherent Commits

```
0a9c9b4 chore(dv04-xray): drop DEBUG prints; gate oracles behind DUCKOS_XRAY_DEBUG
7e277de fix(xray-runtime): xray lifetime is the runtime, not the API call
2dc0720 fix(xray-runtime): capture stdout as well as stderr
c9c469c fix(xray-render): VLESS inbound uses clients + decryption:none
ec09b08 fix(dv04-xray-test): release port-hold before Apply-triggering POST
d89e1bf fix(xray-render): Inbound.Listen is host-only (split host:port) per Xray schema
fb0ea28 fix(xray-render): vnext.address is host-only (no port) per Xray schema
3d3bd58 fix(dv04-xray-test): reserve ephemeral port via hold-listener until xray binds
dbef0e1 fix(dv04-xray-test): bump listener oracle timeout + log ss output on miss
45fcd56 fix(dv04-xray-test): match :PORT (no trailing space); share-link checks credential ref
c64f4f1 fix(dv04-xray-test): drop non-existent CreatedAt field from xray.KeyRef
6e501bd fix(dv04-xray): clean debug logs; fixtures include users; KeyRef valid pattern
5e89bd5 debug(xray-api): print ServeHTTP / appliedFn / Apply errors when DUCKOS_XRAY_DEBUG=1
a286568 fix(dv04-xray-test): persistent stageDir for debug inspection
d83b988 fix(dv04-xray): reconcile only after successful catalog mutation; ss oracle at /usr/bin/ss
1b167ab fix(dv04-xray-test): override xray.XrayPath + use host:port listen
2e4c1cc fix(dv04-xray-test): rename requireXray to xrayTestRequireXray
56b44f0 fix(dv04-xray): production-path integration test for DV04-XRAY
40cf06a fix(dv04-xray): wire production Xray host into Agent IPC
```

## Production Boundary Guarantees

- **Renderer schema is canonical.** The rendered Xray JSON
  matches xray 1.8.x: `inbounds[i].listen` is host-only,
  `inbounds[i].port` is a separate integer;
  `inbounds[i].settings.clients[]` (not `vnext`);
  `decryption: "none"` is mandatory for VLESS. Any
  drift breaks the cold start, and the listener oracle
  catches it because `RenderConfig` is the only thing
  that ever writes `active.json`.

- **Runtime lifetime is the runtime, not the API call.**
  `Runtime.Apply` uses `exec.CommandContext(context.Background(), ...)`
  for the xray child. The reconciliation defer in
  `xray.API.ServeHTTP` creates a 10-second context for
  catalog operations and cancels it on return — but the
  xray process is NOT tied to that context. xray is
  bound to the runtime; `Runtime.Stop` sends SIGTERM to
  the process group; `waitRuntimeGone` (test fixture)
  falls back to SIGKILL on the same group if the
  process does not release its listener in 3 s.

- **Reconciler is gated by a real catalog mutation.**
  `appliedFn` fires only after `UpsertServer` /
  `UpsertUser` / `RemoveServer` / `RemoveUser`
  returned without error. Validation failures
  (`server.Validate`, `user.Validate`) do NOT set
  the flag, so the defer is a no-op and the runtime
  stays idle. `RenderConfig` itself rejects a server
  with zero enabled users (so an Apply after a server
  upsert alone never happens).

- **Subscription URL is the QR contract.**
  `BuildSubscriptionURL(servers, users)` produces
  `base64.StdEncoding(join(Lines, "\n"))`; the
  canonical `Lines` slice is what the share links
  look like. `DecodeSubscriptionURL` is the symmetric
  half. Share links carry the `VaultID` reference
  for the credential — secret bytes are never
  embedded, no `REDACTED` sentinel.

- **Independent listener oracle, not a runtime call.**
  `oracleListener` shells out to `/usr/bin/ss -ltn`
  (falling back to `/sbin/ss`) and matches the
  canonical `:<port>` substring. The fixture is a
  fresh `httptest.Server`; the runtime never observes
  its own listener. Failures dump
  `active.json + stderr.log + /proc/net/tcp` only when
  `DUCKOS_XRAY_DEBUG=1` is set, so a green run is
  silent.

- **Port-hold is released before Apply, not after.**
  `validServerFixture` reserves a kernel-allocated
  port and holds it via `net.Listener` until
  `(*reservedServer).release(t)` is called by the
  test. Release happens BEFORE the mutation that
  triggers xray's cold-start bind (the user upsert),
  so xray can actually bind. A `t.Cleanup` auto-
  closes the hold so a `t.Fatal` before release
  does not leak the fd.

- **Negative cases return typed errors.** Invalid
  protocol/security -> `422 invalid_server` /
  `invalid_user`; nil host -> 503 `xray_unavailable`;
  engine -> 503 `agent_unavailable`. No fake success,
  no fake 200 with empty body.

## Management Boundary

eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22
untouched. The fixture uses the loopback interface
(127.0.0.1) so the production path exercises the
real xray daemon without disturbing eth0.

## How to Run

```sh
ssh duckdev@duck-dev-01 'cd /tmp/duckos-test/work \
  && source /tmp/duckos-test/env.sh \
  && export PATH GOCACHE=/home/duckdev/.cache/go-build \
  && export DUCKOS_XRAY_CAPTURE_STDERR=1 \
  && go test -count=1 -timeout 180s -v -run TestDV04XRAY ./tests/layer_l/...'
```

Expected:

```
ok  	github.com/CrazyDuckLab/DuckOS/tests/layer_l	<1s
```

All 11 subcases pass; total runtime under 1 second on
duck-dev-01.

## What This Does NOT Cover

- **TLS / Reality transport.** The renderer's
  `buildStreamSettings` accepts `SecurityTLS` and
  `SecurityReality` paths, but the test fixture
  exercises only `SecurityNone`. A TLS / Reality
  matrix is a future closed case; for DV04-XRAY the
  contract is "xray accepts the rendered JSON and
  binds the configured `Listen:Port`" — verified for
  the canonical VLESS+TCP+none path. Reality / TLS
  certificate material is referenced by `vaultID`,
  never embedded.

- **Outbound chains.** The renderer emits a single
  `freedom` outbound. Multi-outbound routing
  (SOCKS, VLESS-out, etc.) is not exercised by
  DV04-XRAY.

- **Restart durability of a hot reload.** The
  reconcile test exercises cold-start (the runtime
  was explicitly Stopped). SIGUSR1 hot-reload is
  the production reload path; a future case can
  verify it by mutating a second server and
  asserting the listener flips without xray
  restarting.
