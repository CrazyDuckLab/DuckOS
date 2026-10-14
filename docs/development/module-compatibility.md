# Module Compatibility Matrix — beta.3 / beta.4

Status: living document. Every row must be backed by tests or concrete code
evidence; untested rows are marked NOT VERIFIED.

## Port Mapping ↔ rest of the system

| Pairing | Verdict | Evidence |
|---|---|---|
| Port Mapping ↔ nftables (own table) | COMPATIBLE (code evidence) | Dynamic mappings render exclusively into `inet duckos_upnp` (`internal/upnp/nftables_linux.go`); static firewall rules live in `inet/duckos` (`internal/network/firewall_host.go`). Distinct tables cannot collide by construction. Integration test pending: **NOT VERIFIED** live. |
| Port Mapping ↔ beta.4 static firewall/NAT | COMPATIBLE (by construction) | No shared chains; Remove() flushes only `duckos_upnp`. Required coexistence test on lab hardware: **PENDING LIVE QUALIFICATION**. |
| Port Mapping ↔ Safe Apply | OUT OF SCOPE (by design) | Dynamic mappings are runtime state with TTL, not configuration; they are never part of a config candidate and never rolled back. Documented in `internal/upnp/runtime.go`. |
| Port Mapping ↔ RBAC | TESTED | `internal/agent/upnp_host_test.go`: reads = Observe, mutations = ConfigureNetwork; beta.3 frontends inherit the same gate. |
| Port Mapping ↔ management eth0 | REFUSED (tested) | Registry role check rejects management/WAN roles (`trusted_lan_registry*` tests); boot re-check revokes lost roles (`cmd/duck-agent/main.go`). |
| Port Mapping ↔ UnavailableBackend | TESTED | Enable is refused when nft is missing; listeners never start (`unavailable_backend_test.go`, runtime EnableNetwork path). |

## beta.4 observation surfaces ↔ rest of the system

| Pairing | Verdict | Evidence |
|---|---|---|
| DHCP leases ↔ dnsmasq | COMPATIBLE (code evidence) | Read-only parse of the dnsmasq lease file (`internal/dhcpsnoop/dnsmasq.go` + tests). No competing DHCP daemon; malformed file lines are skipped, read is size-bounded (4 MiB). |
| DHCP leases ↔ lease lifecycle | OBSERVATION ONLY | No write path; the DHCP server remains the single owner of lease state. |
| Firewall snapshot ↔ nftables ownership | COMPATIBLE (code evidence) | `network.NewFirewallHost().Capture` reads only `inet/duckos` (`nft_snapshot.go` Decode validates object shape); no flush of global rulesets anywhere in the read path. |
| Firewall snapshot ↔ firewall mutation | READ-ONLY | The surface has no mutation verb; changes stay behind Safe Apply (`internal/safeapply/`). |
| UI ↔ Public API | TESTED | All new UI data flows through `/api/v1/*` with Bearer auth (`web/src/api.ts`); no direct Agent IPC access from the Web UI. |

## beta.4 firewall / WAN ↔ rest of the system

| Pairing | Verdict | Evidence |
|---|---|---|
| Firewall rules (inet duckos_fw) ↔ Port Mapping (inet duckos_upnp) ↔ management policy (inet duckos) | COMPATIBLE (by construction) | Three exclusively-owned tables; every mutation renders only its own table; no global flush anywhere. Cross-table integration test on live kernel: **PENDING LIVE QUALIFICATION**. |
| Firewall rules ↔ Safe Apply | INTEGRATED (host-level transaction) | Every mutation is staged → validated → applied as one atomic nft batch → verified by kernel readback → auto-restored on failure (fw_host.go). Journal-first safeapply store integration (state.OpenExisting) remains future work — documented. |
| Firewall rules ↔ management access | SAFE BY BOUNDARY | V1 renders FORWARD rules only; input filtering stays with the management policy owner, so UI rules cannot lock out eth0. |
| WAN staging ↔ managed routing | STAGED ONLY | WAN mutations validate + persist and are reported as staged; the apply verb returns 501 until the networkd enrollment + Safe Apply integration exists. No parallel network configuration engine. |
| WAN observation ↔ kernel | READ-ONLY | net.Interface + /proc/net/route reads only (observe_linux.go). |

## Known interaction risks

1. **`duckos_upnp` Remove() flushes the whole table** (`nft flush table inet duckos_upnp` on Remove). With multiple active mappings a single Remove momentarily drops sibling rules until the next Add re-renders them. Beta.2 behaviour, preserved unchanged; must be replaced with per-rule deletes before beta.3 live qualification. **PENDING LIVE QUALIFICATION.**
2. **IGD control port 53520** is bound per trusted address only; a host-level firewall that forwards WAN traffic to 53520 would expose it. The forward chain of `inet/duckos` must not contain WAN→53520 rules (default: none).
3. **NAT-PMP/PCP share UDP 5351** with any future third-party daemon; DuckOS binds the port only while a trusted network with those protocols is enabled. A competing listener causes an explicit bind failure (journaled, non-fatal to other networks).
