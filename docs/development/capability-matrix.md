# DuckOS Capability Matrix — beta.3 / beta.4 planning baseline

> **Beta.3 release baseline:** the F1–F8 nft launcher fixes and accepted AMD64
> Router Core qualification supersede the historical privilege-gap gate.
> This older planning matrix mixes historical implementation labels and future
> work; it is not an expanded release qualification claim. See the authoritative
> [beta.3 scope and limits](../qualification-beta3-final.md).

Status: living document, updated per subsystem during beta.3/beta.4 development.
Baseline: `main` @ `3388271` (beta.2 release `feee7c0` + GitHub presentation commits).

Legend: **IMPLEMENTED** (backend + tests), **PARTIAL** (working but incomplete surface),
**STUB** (model/contract only), **NOT IMPLEMENTED**, **NOT VERIFIED** (implemented, no live evidence).

## Port Mapping (beta.3 scope)

| Capability | State | Evidence |
|---|---|---|
| Mapping model (Protocol, TTL, ownership) | IMPLEMENTED | `internal/upnp/upnp.go` (Manager, Mapping, Network); beta.2 tests |
| beta.3 Transport field (tcp/udp) | IMPLEMENTED | `upnp.go` Transport type; empty value preserves beta.2 rendering |
| nftables mapping lifecycle (`duckos_upnp` table, staging + validate + load) | IMPLEMENTED | `internal/upnp/nftables_linux.go`; beta.2 live qualification; **NOT VERIFIED** on beta.3 Transport rendering |
| Trusted LAN registry (persistent, role-checked) | IMPLEMENTED | `internal/upnp/trusted_lan_registry.go`; boot re-check in `cmd/duck-agent/main.go` |
| Runtime (TTL expiry, revoke, events, backend states) | IMPLEMENTED | `internal/upnp/runtime.go` + tests |
| NAT-PMP wire protocol (RFC 6886) | IMPLEMENTED (beta.3) | `internal/upnp/proto/natpmp.go` + unit tests |
| PCP wire protocol (RFC 6887 subset: ANNOUNCE/MAP, no PEER, no THIRD_PARTY/PREFER_FAILURE/FILTER/PORT_SET) | IMPLEMENTED (beta.3) | `internal/upnp/proto/pcp.go` + unit tests |
| SSDP M-SEARCH parsing + responder | IMPLEMENTED (beta.3) | `internal/upnp/proto/ssdp.go`, `frontend/ssdp_linux.go`; multicast join scoped to trusted iface; **NOT VERIFIED** live |
| NAT-PMP/PCP UDP listeners (5351, demux by version) | IMPLEMENTED (beta.3) | `frontend/listeners_linux.go`, `udp_linux.go`; loopback round-trip tests (linux build tag) |
| UPnP IGD control (description XML + SOAP GetExternalIPAddress / AddPortMapping / DeletePortMapping / GetGenericPortMappingEntry) | IMPLEMENTED (beta.3) | `frontend/igd_linux.go`; same-client rule enforced; **NOT VERIFIED** with real IGD clients |
| Rate limiting / quotas / conflict detection | IMPLEMENTED (beta.3) | `frontend/model.go`, `natpmp.go`, `pcp.go` + `frontend_test.go` |
| Rejected-request journal (bounded ring, public surface) | IMPLEMENTED (beta.3) | `frontend.RejectionJournal`; exposed in state doc + UI |
| Agent IPC host (state/enable/disable/revoke, RBAC) | IMPLEMENTED | `internal/agent/upnp_host.go` + tests; beta.3 adds frontends reconcile + state extension |
| Public API proxy `/api/v1/upnp/*` | IMPLEMENTED | `internal/api/server.go` handleUPnPProxy (body cap, auth) |
| CLI `duck upnp state/enable/disable/revoke` | IMPLEMENTED (beta.3) | `cmd/duck/upnp_cli.go` |
| Web UI (ServiceDiscovery page: per-network enable, TTL, mappings revoke, listeners status, rejection journal) | IMPLEMENTED | `web/src/pages/ServiceDiscoveryPage.tsx`; i18n en/ru/de/es |
| Disabled-by-default | IMPLEMENTED | No sockets before operator enable; `Frontends.Reconcile` no-ops with empty set |
| External-address observation | PARTIAL | Source-address probe toward default route (`observeExternalIPv4`); no multi-WAN awareness — NOT VERIFIED |
| IPv6 frontends (PCP/NAT-PMP over IPv6, MLD) | NOT IMPLEMENTED | Out of beta.3 scope, documented gap |
| WAN-side discovery suppression | IMPLEMENTED (structural) | Listeners bind trusted-LAN addresses only; WAN refusal upstream in registry/runtime |

## beta.4 observation surfaces (implemented on develop/beta4-router-core)

| Capability | State | Evidence |
|---|---|---|
| DHCP lease observation `/api/v1/dhcp/leases` | IMPLEMENTED (beta.4) | `internal/dhcpsnoop/dnsmasq.go` (parser + tests), `internal/agent/dhcp_host.go`, Engine/API proxy, `duck dhcp leases`, DhcpLeasesPage real feed with fallback. **NOT VERIFIED** against a live dnsmasq. |
| Firewall owned-table snapshot `/api/v1/firewall/rules` | IMPLEMENTED (beta.4, read-only) | `internal/agent/firewall_host.go`, Engine/API proxy, `duck firewall rules`, FirewallRulesPage snapshot panel. Typed rule CRUD remains NOT IMPLEMENTED by contract. |
| Firewall rules CRUD (typed, inet duckos_fw, forward-only) | IMPLEMENTED (beta.4, journal-first) | `internal/fwrules` (model+render+tests) + `internal/agent/fw_host.go` as a `safeapply.Controller` instance of its own: durable journal, Preflight, atomic nft batch, readback Verify, Restore, 300s confirm window with automatic rollback, crash recovery. `/api/v1/firewall/rules` (+status/confirm/rollback/observed), `duck firewall …`, Web editor with tx phase and Confirm/Roll-back buttons. **NOT VERIFIED** against a live kernel. |
| Firewall input/output rule filtering | NOT IMPLEMENTED (deliberate V1 boundary) | Forward-only so a UI rule cannot cut management access; documented in `internal/fwrules`. |
| WAN manager (DHCP + static IPv4) | IMPLEMENTED (beta.4, journal-first apply) | `internal/wan/config.go` (validation+tests), `observe_linux.go`, `internal/agent/wan_host.go` — apply composes a network.Config candidate (static: iface+default route; DHCP: dhcpClient=ipv4) and admits it via NetworkService.SubmitCandidate; confirm window + automatic rollback; PPPoE/L2TP apply explicitly refused. `/api/v1/wan/*`, `duck wan apply --id`, P017 feeds. **NOT VERIFIED** against a live uplink. |
| WAN manager (PPPoE / L2TP) | STUB | Model contracts only, per beta.2 baseline. |

## Router core (beta.4 scope)

| Capability | State | Evidence |
|---|---|---|
| Management interface isolation (eth0 protected) | IMPLEMENTED | `internal/network/management*.go`, forward_guard; beta.2 qualified |
| LAN interfaces / enrollment / host snapshots | IMPLEMENTED | `internal/network/` (enrollment, adoption, host_snapshot) |
| Safe Apply transaction engine | IMPLEMENTED | `internal/safeapply/` (controller, diff, checkpoint, auto-restore) |
| Firewall (bounded nft ops, `inet/duckos` ownership) | IMPLEMENTED | `internal/network/firewall_host.go`, rule_host, nft_snapshot |
| Firewall rules CRUD public API (`/api/v1/firewall/rules`) | NOT IMPLEMENTED | `web/src/configurators/firewall.tsx` documents the missing route |
| Routing: static/kernel routes | IMPLEMENTED | `internal/network/route_host.go`, route_plan, managed_routing |
| Routing: dynamic (FRR) | IMPLEMENTED (live surface) | `internal/frr/`, `cmd/duck-frr-launcher`, `/api/v1/frr/*` |
| Routing: dynamicrt model | STUB | `internal/dynamicrt` (env-gated, model only) |
| WAN manager runtime (DHCP/static/PPPoE/L2TP clients) | STUB | `internal/wan/provider.go` — V1 contract model only, no Linux runtime |
| DHCP server management | PARTIAL | `internal/network/dns_service.go` manages dnsmasq lifecycle; leases API absent (`web/src/configurators/dns.tsx` notes P057 read-only pending `/api/v1/dhcp/leases`) |
| DHCP snooping model | STUB | `internal/dhcpsnoop` |
| DNS: resolver/dnsmasq host config | IMPLEMENTED | `internal/network/dns_host.go`, dns_service.go |
| DNS: DDNS | STUB | `internal/ddns` (provider model only) |
| Multi-WAN / QoS | IMPLEMENTED (surface) | `/api/v1/multiwan`, `/api/v1/qos(/apply|/rollback)` |
| Wi-Fi (hostapd) | IMPLEMENTED (surface) | `internal/hostapd`, `/api/v1/wifi` |

## Platform invariants verified for beta.3

- Public UI/API never touches Agent IPC directly: `/api/v1/upnp/*` → Engine proxy → Agent loopback IPC.
- RBAC: reads = Observe, mutations = ConfigureNetwork (`identity.Permission`) enforced in `UPnPHost`.
- No parallel NAT engine: frontends write exclusively through `upnp.Runtime` → `NftablesBackend` (`duckos_upnp` table). Static firewall rules (beta.4) live in a separate owned table — coexistence by construction, test evidence pending (module-compatibility.md).
- Management eth0 can never become a trusted LAN: registry role check at Trust and boot re-check.

## Known limitations / pending live qualification

- All beta.3 protocol listeners are unit + loopback-socket tested; **LAN-side live qualification (real SSDP clients, NAT-PMP/PCP renewals, IGD clients) is PENDING LIVE QUALIFICATION** and intentionally not executed against lab VMs in this task.
- Beta.3 nft execution is owned by `duck-nft-launcher`; the UID-0 Agent retains zero effective capabilities. See the final qualification scope, not historical planning labels above.
- IGD `GetGenericPortMappingEntry` returns mappings in Manager id order, not insertion order — acceptable for V1, documented for clients.
