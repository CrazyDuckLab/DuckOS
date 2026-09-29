# DV04-WG Production Path — CLOSED

- **HEAD**: `d10f6a9`
- **Branch**: `V1 UI qualification branch`
- **Date**: 2026-09-13

## Goal

WireGuard L-tier previously checked the kernel/runtime fixture directly.
The production path was missing:

  PUBLIC → ENGINE → AGENT → KERNEL WG → INDEPENDENT ORACLE → RESTART → NEGATIVE

## Production Path Matrix

| Link            | Production code path                                                                  | Test                                          |
|-----------------|---------------------------------------------------------------------------------------|-----------------------------------------------|
| **PUBLIC**      | `api.Handler.ServeHTTP` → `handleWireGuardProxy` → `/api/v1/wireguard/*`              | TestDV04WG_PublicEngineAgentChain (Unix socket) |
| **ENGINE**      | `engine.Engine.WireGuardProxy` (Unix socket client; 1 MiB cap; typed-error envelope)   | TestDV04WG_PublicEngineAgentChain             |
| **AGENT IPC**   | `agent.Handler.ServeHTTP` → `WireguardHost.ServeHTTP` (ConfigureNetwork gate)         | TestDV04WG_Production* (9 tests)               |
| **AGENT HOST**  | `internal/agent/wg_host.go` (production host, reconcile, nil-host stub)               | TestDV04WG_ProductionNegativeAgentUnavailable |
| **RUNTIME**     | `wireguard.API` → `KernelAdapter` → `IPRouteRunner` (/sbin/ip) + `/usr/bin/wg` (private-key + peer ops via stdin/temp file) | TestDV04WG_ProductionCrudKernel, TestDV04WG_ProductionPeerAllowedIPs |
| **INDEPENDENT ORACLE** | `os/exec` → `/usr/bin/wg show` + `/sbin/ip link show` (separate process, no shared code with adapter) | TestDV04WG_ProductionCrudKernel, TestDV04WG_ProductionPeerAllowedIPs, TestDV04WG_ProductionDeletePeerRemoved |
| **RESTART/DURABILITY** | `WireguardHost.ReconcileServer` → `KernelAdapter.ApplyServer` + `ApplyPeer` for every enabled entry in the durable `FileCatalog` (re-opened after process restart) | TestDV04WG_ProductionRestartReconciles |
| **NEGATIVE**    | `wireguard.API` `validate()` rejects invalid keys / AllowedIPs / orphan server refs; `WireguardHost` nil-stub returns 503 `wireguard_unavailable`; engine returns 503 `agent_unavailable`; kernel write failures surface 422 with the typed message | TestDV04WG_ProductionNegativeInvalidPeer, TestDV04WG_ProductionNegativeWrongServerRef, TestDV04WG_ProductionNegativeAgentUnavailable |

## Subcases (all PASS, 10/10)

| Test                                        | Production-path evidence                                                              |
|---------------------------------------------|---------------------------------------------------------------------------------------|
| TestDV04WG_ProductionCrudKernel             | httptest → agent.NewWireguardHost → wireguard.API → KernelAdapter.ApplyServer → kernel; /usr/bin/wg show sees the public key + listening port |
| TestDV04WG_ProductionPeerAllowedIPs         | peer upsert through API → ApplyPeer → kernel AllowedIPs visible in independent /usr/bin/wg show |
| TestDV04WG_ProductionDeletePeerRemoved      | peer DELETE through API → catalog lookup (ServerID + PublicKey) → ApplyPeer remove → kernel shows AllowedIPs removed |
| TestDV04WG_ProductionExportRedacted         | GET /servers/{id}/export through API → ExportServerConfig emits no PrivateKey line (not even a redaction sentinel) |
| TestDV04WG_ProductionNegativeInvalidPeer    | peer with malformed AllowedIPs rejected by `peer.Validate`; kernel is not touched |
| TestDV04WG_ProductionNegativeWrongServerRef | peer referencing non-existent server rejected; error names "server" |
| TestDV04WG_ProductionNegativeAgentUnavailable | nil WireguardHost returns 503 `wireguard_unavailable` |
| TestDV04WG_ProductionRestartReconciles      | tear down kernel link → re-open durable catalog → ReconcileServer → /usr/bin/wg show sees the interface + peer set back |
| TestDV04WG_ProductionListPersists           | GET /servers through API returns the catalog state the upsert put there |
| TestDV04WG_PublicEngineAgentChain           | full PUBLIC → ENGINE.WireGuardProxy (Unix socket) → AGENT IPC → KERNEL chain, with reconcile path |

## Coherent Commits

```
d10f6a9 fix(wireguard-test): update for new PrivateKey-less export + __wg__ prefix
3af0344 fix(dv04-wg): WireguardHost strips /agent/v1/wireguard prefix before wireguard.API
ab936c6 fix(dv04-wg-test): add net and filepath imports for socket test
17cdb75 fix(dv04-wg-test): end-to-end PUBLIC -> ENGINE -> AGENT chain test
1dffc45 fix(dv04-wg): Engine.WireGuardProxy + public /api/v1/wireguard/* route
6d5648c fix(dv04-wg): reconcile peers + peer delete looks up ServerID + export adds peer
34d51c2 fix(dv04-wg): use stdlib base64; ExportServerConfig drops PrivateKey line
223a035 fix(dv04-wg-test): oracle uses /usr/bin/wg (where it lives on duck-dev-01)
d50c596 fix(wireguard): verify private-key temp file via stat after write
65e333a debug(wireguard): add stat-debug to private-key temp file
4da4776 fix(wireguard): extractSecretStdin APPENDS temp path after flag
49d99d4 fix(wireguard): drop unused os import
46a1957 fix(wireguard): extractSecretStdin handles private-key AND preshared-key
050f082 fix(wireguard): adapter now uses /sbin/wg for key/peer ops, /sbin/ip for links
59f35a0 fix(dv04-wg-test): rename local randomSuffix to wgRandomSuffix
23657a4 fix(dv04-wg): production-path integration test for DV04-WG
d751f0b fix(dv04-wg): wire production WireGuard host into Agent IPC
```

## Production Boundary Guarantees

- The PUBLIC API only accepts mutations after the
  ConfigureNetwork gate is satisfied (agent-side access
  check); the public loopback cannot bypass it through
  a stale session token.
- The KernelAdapter never logs private key bytes; the
  private-key and preshared-key bytes are forwarded
  via 0600 temp files (or stdin) that are unlinked
  before the function returns.
- The `wg(8)` and `ip(8)` invocations are split by a
  prefix dispatcher (`__wg__` -> `/usr/bin/wg`, else
  -> `/sbin/ip`) so the production code never relies
  on shell wrappers or the wrong iproute2 syntax
  (the previous code called `ip set <iface> private-key
  ...` which `ip` does not own — it is a `wg(8)`
  operation).
- The catalog is the durable source of truth; the
  kernel is best-effort. Restart durability is enforced
  by `WireguardHost.ReconcileServer`, which iterates the
  re-opened catalog and re-applies every enabled server
  + peer through `KernelAdapter`.
- Negative cases return typed errors: invalid
  AllowedIPs -> `peer.Validate` rejects with
  `invalid_peer`; orphan server -> catalog lookup
  fails with `not_found` / `server` message; nil host
  -> 503 `wireguard_unavailable`; Agent unavailable ->
  503 `agent_unavailable`. No fake success / no fake
  200 with empty body.

## Management Boundary

eth0 / 172.18.4.149 / default via 172.18.1.254 / SSH :22
untouched. The fixture uses the host WireGuard namespace
on duck-dev-01 (kernel module does not load inside
per-netns link_ops on this VM; the kernel WG runtime
runs in the host ns, the same boundary the production
composition root uses).

End of DV04-WG handoff.
