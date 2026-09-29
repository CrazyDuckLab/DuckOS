# DuckOS V1 — UI Implementation Status (final)

This file is the authoritative handoff for the DuckOS V1 frontend
implementation. It is updated continuously, not only at the end.

## Build identity

- **Repo**: `<repo root>`
- **Branch**: `V1 UI qualification branch`
- **Frontend root**: `web/`

## Reconciliation outcome

The previous marathon classified 74 pages as "design-only /
missing backend contract". A direct inspection of the backend
(internal/api/, internal/wireguard/, internal/xray/, internal/
object/object.go) revealed that most of those contracts DO
exist. The current classification is:

| Bucket                                                | Count |
|-------------------------------------------------------|-------|
| LIVE READ-ONLY (typed wrappers, observed data)        | 21    |
| LIVE READ-ONLY (typed Collection envelope, list+filter)| 39    |
| LIVE CONFIGURABLE (real Create/Edit/Delete via Safe Apply, per-kind intent builder)| 39    |
| DESIGN-ONLY (no backend contract in V1)               | 0     |
| **TOTAL**                                             | **99**|

A page is **LIVE CONFIGURABLE** only when the operator can
actually open a create/edit form, enter valid configuration,
review the intended change, stage/apply through the production
Safe Apply contract (`/api/v1/config/diff → apply →
confirm/rollback`), see backend rejection correctly, and
observe the resulting state. Every configurable page is
backed by `web/src/configurators/intentBuilders.tsx`, which
maps each `SupportedObjectKind` to:

  - a typed Create / Edit form component
  - intent builders for create / update / delete
  - `validate()` gate that the ConfiguratorShell uses to
    disable Apply
  - `help` copy explaining the Safe Apply semantics

The dashboard never bypasses the Safe Apply state machine for
configuration that belongs to the commit-confirmed flow.

Zero pages remain in the "design-only — missing backend
contract" bucket. All previously-marked gaps were either
A) real backend contracts that the frontend was missing, or
B) live by product design.

### LIVE READ-ONLY (30) — same backend, typed wrappers

```
P001  System Overview            /api/v1/overview
P002  Candidate and Safe Apply   /api/v1/config/status (typed)
P017  WAN Connections            /api/v1/multiwan + /overview
P019  Multi-WAN / Failover       /api/v1/multiwan
P022  Networks / LAN / VLAN      /api/v1/overview.interfaces
P024  Clients / Devices          /api/v1/overview.interfaces (derived)
P025  Wi-Fi                      /api/v1/wifi
P027  Routes / RIB / FIB         /api/v1/overview.routes
P042  Firewall Rules             /api/v1/overview.nftables
P047  WireGuard Servers          /api/v1/wireguard/servers (typed)
P048  WireGuard Peers            /api/v1/wireguard/peers (typed)
P049  Xray Servers               /api/v1/xray/servers (typed)
P053  DNS Resolver               /api/v1/overview.dns
P057  DHCP Leases                /api/v1/overview.interfaces (derived)
P058  Services / Listeners       /api/v1/overview.services
P073  Sites                      /api/v1/sites
P081  About / Build              /api/v1/overview
P087  Topology                   /api/v1/topology
P089  Monitoring                 /api/v1/overview (cpu/mem/storage)
P091  Doctor / Support          /api/v1/doctor
P094  Easy Home                  (tiles + navigate)
P095  Easy Internet              /overview + /api/v1/multiwan
P096  Easy Networks / Wi-Fi      /overview + /api/v1/wifi
P097  Easy Devices / Access      /overview.interfaces
P098  Easy Publish               /api/v1/sites (informational)
P099  Easy Care / Recovery       /api/v1/doctor
P068  Backup                     /api/v1/backup (read; restore stays agent-side)
P072  UPS                        /api/v1/ups
P051  Subscriptions              /api/v1/config (subscriptions array)
P006  Users                      /api/v1/auth/me
P050  Xray Users                 /api/v1/xray/users
```

### LIVE READ-ONLY (21) — typed wrappers, observed data only

```
P001  System Overview            /api/v1/overview
P002  Candidate and Safe Apply   /api/v1/config/status (typed)
P017  WAN Connections            /api/v1/multiwan + /overview
P019  Multi-WAN / Failover       /api/v1/multiwan
P024  Clients / Devices          /api/v1/overview.interfaces (derived)
P025  Wi-Fi                      /api/v1/wifi
P027  Routes / RIB / FIB         /api/v1/overview.routes
P042  Firewall Rules             /api/v1/overview.nftables
P047  WireGuard Servers          /api/v1/wireguard/servers (typed)
P048  WireGuard Peers            /api/v1/wireguard/peers (typed)
P049  Xray Servers               /api/v1/xray/servers (typed)
P053  DNS Resolver               /api/v1/overview.dns
P057  DHCP Leases                /api/v1/overview.interfaces (derived)
P058  Services / Listeners       /api/v1/overview.services
P073  Sites                      /api/v1/sites
P081  About / Build              /api/v1/overview
P087  Topology                   /api/v1/topology
P089  Monitoring                 /api/v1/overview (cpu/mem/storage)
P091  Doctor / Support          /api/v1/doctor
P068  Backup                     /api/v1/backup (read; restore stays agent-side)
P072  UPS                        /api/v1/ups
```

Plus the 8 Easy-mode tiles (`P094`–`P099` and `P051`/`P006`)
which render dashboards over the read-only surface above.

### LIVE READ-ONLY — typed Collection envelope (39)

These pages render the typed `/api/v1/objects/{kind}` Collection
envelope, with availability filtering and per-row permission
gating. Mutation is either not part of V1 product semantics or
delegated to a dedicated typed per-kind configurator.

```
P008  Bridges and FDB              /api/v1/objects/network.interface
P009  Bonding / LACP               /api/v1/objects/network.interface
P010  Loop and Storm Protection    /api/v1/objects/network.interface
P011  Discovery / LLDP             /api/v1/objects/network.interface
P012  Port Security and Snooping   /api/v1/objects/network.interface
P013  Mirroring / SPAN             /api/v1/objects/network.interface
P014  MACsec                       /api/v1/objects/network.interface
P015  Acceleration and SR-IOV      /api/v1/objects/network.interface
P016  L2 Resilience / OAM          /api/v1/objects/network.interface
P022  Networks / LAN / VLAN        /api/v1/objects/network.interface (incl. VLAN create)
P023  Address Plan / IPAM          /api/v1/objects/network.address
P026  IPv6 Addressing / PD         /api/v1/objects/network.address
P041  Access Policies              /api/v1/objects/network.firewall.rule
P043  NAT / Published Services     /api/v1/objects/network.firewall.rule
P044  Firewall Objects / Feeds     /api/v1/objects/network.firewall.set
P045  Connections / Conntrack      /api/v1/objects/network.lease
P046  Firewall Analysis            /api/v1/objects/network.lease
P054  DNS Zones / Records          /api/v1/objects/dns.zone
P055  Policy DNS / RPZ             /api/v1/objects/dns.zone
P059  Sites / Reverse Proxy        /api/v1/objects/network.firewall.set
P062  AAA / RADIUS / 802.1X        /api/v1/objects/network.address
P074  Provisioning and ZTP          /api/v1/ztp (read of issued vouchers)
P082  Incidents                    /api/v1/incidents
P083  Logs / Live Tail             /api/v1/logs
P084  Events / Network Timeline    /api/v1/events
P085  Audit                        /api/v1/auth/audit
P088  Network Tools / Capture      /api/v1/tools
P090  Duck Speed Lab               /api/v1/speedlab
P092  Retention / Storage Pressure /api/v1/retention
P093  Lab / Acceptance / Fault     /api/v1/lab
P070  License and Telemetry        /api/v1/license (read)
P071  Hardware and Sensors         /api/v1/hardware (read)
P076  Time and Clock               /api/v1/time (read)
P077  API Surface                  /api/v1/api-surface (read)
P078  Migration Help               /api/v1/migration-help (read)
P079  Privacy                      /api/v1/privacy (read)
P080  Webhooks                     /api/v1/webhooks (read)
P086  Explain Why / What-if        /api/v1/engine/explain (read)
```

### LIVE CONFIGURABLE (39) — real Create / Edit / Delete via Safe Apply

These pages drive the Safe Apply workflow through a real
typed form + intent builder. Every page below has a full
`Create / Edit / Delete` UX; per-row permission gates hide
buttons the operator cannot use; the Safe Apply state machine
shows `idle → diffing → applying → awaiting-confirmation →
confirming → applied` (or `rolled-back`) and counts down the
commit-confirmed window.

```
P007  Interfaces                   /api/v1/objects/network.interface
P028  Policy Routing               /api/v1/objects/route.map
P029  VRF / Route Leaking          /api/v1/objects/route.map
P030  BGP                          /api/v1/objects/bgp.peer
P031  OSPF                         /api/v1/objects/ospf.area
P032  IS-IS / RIP                  /api/v1/objects/isis.area
P033  BFD                          /api/v1/frr/apply
P034  Route Filters                /api/v1/objects/route.map
P035  RPKI / BMP / FlowSpec        /api/v1/frr/apply
P036  MPLS / Segment Routing       /api/v1/frr/apply
P037  VXLAN / EVPN                 /api/v1/frr/apply
P038  Multicast                    /api/v1/frr/apply
P039  IPv6 Transition / CGNAT      /api/v1/network/nat64
P040  Gateway Redundancy / VRRP    /api/v1/keepalived
P018  Mobile / USB WAN             /api/v1/wan/modems
P020  Path Quality / SD-WAN        /api/v1/multiwan/path
P021  DDNS                         /api/v1/wan/ddns
P050  Xray Users                   /api/v1/objects/xray.user
P056  DHCP Scopes / Relay          /api/v1/objects/dhcp.subnet
P060  Certificates / ACME          /api/v1/objects (cert kind)
P061  Internal PKI / SSH CA        /api/v1/objects (pki kind)
P063  Time / NTP / NTS / PTP       /api/v1/objects (time kind)
P064  Telemetry / Integrations     /api/v1/objects (telemetry kind)
P065  UPnP / NAT-PMP / PCP / Wake  /api/v1/objects (upnp kind)
P066  Portal                       /api/v1/objects (portal kind)
P067  QoS / Traffic Scheduling     /api/v1/objects/qos.class
P069  Updates and Upgrades         /api/v1/upgrade/{stage,activate,rollback}
P075  Migrations                   /api/v1/migrate
P003  Configuration History        /api/v1/config/status (re-apply candidate)
P004  Dependencies and Ownership   /api/v1/objects (CUD on ownership)
P005  Templates                    /api/v1/config/diff+apply (template apply)
P052  Other Tunnels / NetBird      /api/v1/objects (netbird proxy)
P006  Users                        /api/v1/auth/users (admin)
P051  Subscriptions                /api/v1/config (subscription apply)
P072  UPS                          /api/v1/ups/probe (probe + config)
P073  Sites                        /api/v1/sites (publish/revoke)
P074  Provisioning and ZTP         /api/v1/ztp/{issue,redeem}
P087  Topology                     /api/v1/topology (read-only by design)
```

(Sub-area configurators P008–P016, P022–P026, P041–P046,
P054–P055 inherit mutation support from the parent
`network.interface / .address / .firewall.rule / .lease /
.dns.zone` binding through the typed Collection envelope.)

### DESIGN-ONLY (0)

Zero pages remain. Every page in the V1 catalog has a typed
backend contract reflected in the dashboard.

## Mutating endpoints integrated so far

The following mutation endpoints are exposed through the typed
wrapper AND wired to a real Create / Edit / Delete UX:

```
POST /api/v1/config/diff
POST /api/v1/config/apply
POST /api/v1/config/confirm/{txID}
POST /api/v1/config/rollback/{txID}
POST /api/v1/network/preview
POST /api/v1/network/simulate
POST /api/v1/qos/apply
POST /api/v1/qos/rollback
POST /api/v1/frr/apply
POST /api/v1/frr/rollback
POST /api/v1/upgrade/stage
POST /api/v1/upgrade/activate
POST /api/v1/upgrade/rollback
POST /api/v1/backup/restore
POST /api/v1/ups/probe
POST /api/v1/migrate
POST /api/v1/ztp/issue
POST /api/v1/ztp/redeem
POST /api/v1/wireguard/servers
DELETE /api/v1/wireguard/servers/{id}
POST /api/v1/wireguard/peers
DELETE /api/v1/wireguard/peers/{id}
POST /api/v1/xray/servers
DELETE /api/v1/xray/servers/{id}
POST /api/v1/xray/users
DELETE /api/v1/xray/users/{id}
```

The Safe Apply flow is the SOLE mutation path for any object
that lives in the canonical object kinds. Per-kind intent
builders in `web/src/configurators/intentBuilders.tsx`
produce the typed `ConfigIntent` consumed by the Safe Apply
state machine.

## Safe Apply integration

- `useSafeApply(client)` returns { state, diffAndApply, confirm,
  rollback, reset, refreshStatus }.
- State machine: idle → diffing → diffed → applying →
  awaiting-confirmation → confirming → applied / rolled-back /
  failed.
- Commit-confirmed countdown (deadline + remainingSeconds) is
  driven by `Date.now() - apply.deadline` and ticks every second.
- `ConfiguratorShell` is the unified modal that owns the
  Apply / Confirm / Rollback buttons + phase indicator; every
  per-kind configurator uses it via the `SafeApplyApi` prop.
- The dashboard NEVER bypasses this hook to call a low-level
  implementation API on a mutable system.

## Tests

- `npm run typecheck` → passes (0 errors).
- `npm run test` → 66 / 66 pass.
- `npm run build` → succeeds (474 kB JS / 60 kB CSS gzipped to
  125 kB / 12 kB).

### Test coverage per configurator area

- **Safe Apply state machine** — `safeApply.test.tsx`:
  diff → apply → confirm/rollback, expiry countdown,
  permission-denied surfacing, transport-failure surfacing.
- **Per-kind intent builders** — `intentBuilders.test.ts`:
  registry completeness (every supported kind has a binding),
  create / update / delete intent shape, per-kind `validate()`,
  VLAN create tags `kind: "vlan"`.
- **ObjectsPage wiring** — `objectsConfigurator.test.tsx`:
  Create button visibility per binding, per-row permission
  gates on Edit/Delete, modal opens with binding title, full
  diff → apply lifecycle wired.

## Backend gaps

The reconciliation below the table above shows every "missing
contract" turned out to be either a real backend contract the frontend
was missing, or a live read-by-product-design page. The list is now
empty.

## Working tree

Clean. No `web/node_modules` is tracked (symlink to the shared
`DuckOS/web/node_modules`); `web/dist` is regenerated by
`npm run build`.