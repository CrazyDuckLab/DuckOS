# Information architecture

## Stable main navigation

Full has exactly ten persistent main areas, in this order:

1. Interfaces
2. WAN
3. LAN / VLAN
4. Routing
5. Firewall
6. Tunnels
7. DNS / DHCP
8. Services
9. System
10. Logs / Diagnostics

The brand link opens System Overview. A compact global health control links Incidents. Candidate status occupies its own global strip. These are shortcuts, not extra competing main categories. Current node identity stays in the header; no fleet selector is invented for V1.

Each area's landing route exposes a persistent labeled second-level list. Up to eight short siblings can use a horizontal section bar; larger areas use a 192 px local index with grouped headings. It scrolls independently and retains selection; no overflow “More” hiding ordinary destinations. Local index collapses to an explicit “Routing pages” chooser on narrow screens, with every route searchable.

## Route rules

The page catalog is normative for 99 landing/workflow routes. Each list with first-class objects supports detail routes `<base>/:objectId`, editor `<base>/:objectId/edit`, and `<base>/new` where creation is permitted. Resolve static children before dynamic IDs (e.g. WireGuard peers vs a server ID). IDs in links are non-secret opaque object IDs, never subscription tokens or private configuration.

Detail sections have stable subpaths where applicable: summary (default), configuration, runtime, dependencies, history, help. A protocol family page with several named features has stable semantically named sections: MPLS transport/VPNs/segment policies/capabilities; BGP peers/routes/policy/security; OSPF instances/areas/interfaces/neighbors/LSDB; L2 resilience MC-LAG/OAM/rings; hardware ports/sensors/storage/power/capabilities. These sections are all visible in the local page index; unsupported sections explain why.

Linking to an object from another section preserves source context through a safe return descriptor in memory, not raw packet/credential data in the URL. Reload opens the canonical object route; browser Back restores list filter/scroll/selection when authorized. Non-secret view preferences may be local; security events and full search queries need not enter browser history.

Top-level routes never move because the mode, screen size or current health changes. Deep links remain valid across theme/locale. Use stable English route segments; labels localize.

## Easy navigation

Easy has Home, Internet, Networks, Devices, Publish and Care. Wi-Fi is a named network workflow, not a mandatory warning on servers without radio. Devices includes Give access and links to real WireGuard/Xray/profile objects. Care includes Updates, Backup/Restore and Diagnosis; this grouping does not remove their Full locations.

“Open in Full” maps to the same canonical object or relevant page, preserving revision and draft. When an Easy form cannot represent an advanced setting, display “Custom configuration retained” with Inspect, not a reset to simple defaults. Toggling modes changes views only.

## Contextual object relationships

- Interface → VLAN/bridge/bond/VRF → addresses → DHCP/DNS → access policy → routes → clients → traffic/incidents.
- WAN → interface/provider → probes → exit group → routes/policy/NAT → tunnels/published services.
- Client → observed identities/leases → access/routing intent → WireGuard/Xray profile → exports → subscription membership.
- BGP peer → VRF/interface → BFD → import/export filters → received/advertised routes → selected RIB/FIB → incident.
- Certificate → key reference/issuer/challenge → names → services/listeners → subscriptions → expiry/renewal events.
- Change → candidate objects → owners/dependencies → management path → verification → receipt/history.
- Backup → schema/secrets → logical roles → physical mapping → candidate → verification → confirmed or rolled back.

Every relationship has direction, ID, revision/source and completeness. “Used by” is reverse dependency, not another manual search. Unknown dependency coverage blocks high-risk delete/detach.

## Search and command navigation

Cmd/Ctrl+K opens a local palette. Tabs: All, Objects, Actions, Pages, Help, Events. Search by alias, protocol, IP/prefix, normalized MAC, hostname, rule ID, certificate fingerprint and keyword. Permission filtering occurs at the backend for hidden objects; UI must not fetch then hide secrets.

Results show type icon, name, parent scope, match reason and readiness. Exact ID/address matches precede fuzzy labels; ranking must not move navigation. Recent objects and saved views are explicit small groups, not a dynamic replacement sidebar. Never index credentials, subscription URLs, WireGuard configs or packet payloads.

Examples: “create VLAN”, “BGP rejected routes”, “ping 1.1.1.1”, “Xray logs”. Action search opens the usual form/review; it never executes from Enter on an ambiguous result. Unsupported actions appear with precise unavailable reason and relevant help. Destructive results require normal permissions and confirmation.

No search route is invented for every query; palette is a global component, with detailed results under the relevant list or tools page. Events/log search is scoped and bounded. Show current search scope and allow clearing filters.

## Page-to-feature map

The following generated directory is authoritative. Each entry links its full 44-dimension contract in PAGES. The structured page catalog contains field groups and filters as well as these paths.

| Page | Full / Easy home | Route |
|---|---|---|
| [P001 System Overview](PAGES.md#p001) | System | `/system/overview` |
| [P002 Candidate and Safe Apply](PAGES.md#p002) | System | `/system/changes` |
| [P003 Configuration History](PAGES.md#p003) | System | `/system/history` |
| [P004 Dependencies and Ownership](PAGES.md#p004) | System | `/system/objects` |
| [P005 Templates](PAGES.md#p005) | System | `/system/templates` |
| [P006 Import and Migration](PAGES.md#p006) | System | `/system/import` |
| [P007 Interfaces](PAGES.md#p007) | Interfaces | `/interfaces` |
| [P008 Bridges and FDB](PAGES.md#p008) | Interfaces | `/interfaces/bridges` |
| [P009 Bonding / LACP](PAGES.md#p009) | Interfaces | `/interfaces/bonds` |
| [P010 Loop and Storm Protection](PAGES.md#p010) | Interfaces | `/interfaces/loop-protection` |
| [P011 Discovery / LLDP](PAGES.md#p011) | Interfaces | `/interfaces/discovery` |
| [P012 Port Security and Snooping](PAGES.md#p012) | Interfaces | `/interfaces/port-security` |
| [P013 Mirroring / SPAN](PAGES.md#p013) | Interfaces | `/interfaces/mirroring` |
| [P014 MACsec](PAGES.md#p014) | Interfaces | `/interfaces/macsec` |
| [P015 Acceleration and SR-IOV](PAGES.md#p015) | Interfaces | `/interfaces/acceleration` |
| [P016 L2 Resilience / OAM](PAGES.md#p016) | Interfaces | `/interfaces/resilience` |
| [P017 WAN Connections](PAGES.md#p017) | WAN | `/wan/connections` |
| [P018 Mobile / USB WAN](PAGES.md#p018) | WAN | `/wan/mobile` |
| [P019 Multi-WAN / Failover](PAGES.md#p019) | WAN | `/wan/failover` |
| [P020 Path Quality / SD-WAN](PAGES.md#p020) | WAN | `/wan/path-quality` |
| [P021 DDNS](PAGES.md#p021) | WAN | `/wan/ddns` |
| [P022 Networks / LAN / VLAN](PAGES.md#p022) | LAN / VLAN | `/lan/networks` |
| [P023 Address Plan / IPAM](PAGES.md#p023) | LAN / VLAN | `/lan/address-plan` |
| [P024 Clients / Devices](PAGES.md#p024) | LAN / VLAN | `/lan/clients` |
| [P025 Wi-Fi](PAGES.md#p025) | LAN / VLAN | `/lan/wifi` |
| [P026 IPv6 Addressing / PD](PAGES.md#p026) | LAN / VLAN | `/lan/ipv6` |
| [P027 Routes / RIB / FIB](PAGES.md#p027) | Routing | `/routing/routes` |
| [P028 Policy Routing](PAGES.md#p028) | Routing | `/routing/policies` |
| [P029 VRF / Route Leaking](PAGES.md#p029) | Routing | `/routing/vrfs` |
| [P030 BGP](PAGES.md#p030) | Routing | `/routing/bgp` |
| [P031 OSPF](PAGES.md#p031) | Routing | `/routing/ospf` |
| [P032 IS-IS / RIP](PAGES.md#p032) | Routing | `/routing/other-protocols` |
| [P033 BFD](PAGES.md#p033) | Routing | `/routing/bfd` |
| [P034 Route Filters / Redistribution](PAGES.md#p034) | Routing | `/routing/filters` |
| [P035 RPKI / BMP / FlowSpec](PAGES.md#p035) | Routing | `/routing/bgp-security` |
| [P036 MPLS / Segment Routing](PAGES.md#p036) | Routing | `/routing/mpls` |
| [P037 VXLAN / EVPN](PAGES.md#p037) | Routing | `/routing/evpn` |
| [P038 Multicast / Discovery Forwarding](PAGES.md#p038) | Routing | `/routing/multicast` |
| [P039 IPv6 Transition / CGNAT](PAGES.md#p039) | Routing | `/routing/transition` |
| [P040 Gateway Redundancy / VRRP](PAGES.md#p040) | Routing | `/routing/redundancy` |
| [P041 Access Policies](PAGES.md#p041) | Firewall | `/firewall/access` |
| [P042 Firewall Rules](PAGES.md#p042) | Firewall | `/firewall/rules` |
| [P043 NAT / Published Services](PAGES.md#p043) | Firewall | `/firewall/nat` |
| [P044 Firewall Objects / Feeds](PAGES.md#p044) | Firewall | `/firewall/objects` |
| [P045 Connections / Conntrack](PAGES.md#p045) | Firewall | `/firewall/connections` |
| [P046 Firewall Analysis / Protection](PAGES.md#p046) | Firewall | `/firewall/protection` |
| [P047 WireGuard Servers](PAGES.md#p047) | Tunnels | `/tunnels/wireguard` |
| [P048 WireGuard Peers / Exports](PAGES.md#p048) | Tunnels | `/tunnels/wireguard/peers` |
| [P049 Xray Servers](PAGES.md#p049) | Tunnels | `/tunnels/xray` |
| [P050 Xray Users / Profiles](PAGES.md#p050) | Tunnels | `/tunnels/xray/profiles` |
| [P051 Subscriptions](PAGES.md#p051) | Services | `/services/subscriptions` |
| [P052 Other Tunnels / NetBird](PAGES.md#p052) | Tunnels | `/tunnels/other` |
| [P053 DNS Resolver / Forwarding](PAGES.md#p053) | DNS / DHCP | `/dns/resolver` |
| [P054 DNS Zones / Records](PAGES.md#p054) | DNS / DHCP | `/dns/zones` |
| [P055 Policy DNS / RPZ](PAGES.md#p055) | DNS / DHCP | `/dns/policies` |
| [P056 DHCP Scopes / Relay](PAGES.md#p056) | DNS / DHCP | `/dhcp/scopes` |
| [P057 Leases / Reservations](PAGES.md#p057) | DNS / DHCP | `/dhcp/leases` |
| [P058 Services / Listeners](PAGES.md#p058) | Services | `/services` |
| [P059 Sites / Reverse Proxy](PAGES.md#p059) | Services | `/services/sites` |
| [P060 Certificates / ACME](PAGES.md#p060) | Services | `/services/certificates` |
| [P061 Internal PKI / SSH CA](PAGES.md#p061) | Services | `/services/pki` |
| [P062 AAA / RADIUS / 802.1X](PAGES.md#p062) | Services | `/services/aaa` |
| [P063 Time / NTP / NTS / PTP](PAGES.md#p063) | Services | `/services/time` |
| [P064 Telemetry / Integrations](PAGES.md#p064) | Services | `/services/integrations` |
| [P065 UPnP / NAT-PMP / PCP / Wake](PAGES.md#p065) | Services | `/services/discovery` |
| [P066 Portal](PAGES.md#p066) | Services | `/services/portal` |
| [P067 QoS / Traffic Scheduling](PAGES.md#p067) | Routing | `/routing/qos` |
| [P068 Users / Roles](PAGES.md#p068) | System | `/system/users` |
| [P069 Sessions / Temporary Access](PAGES.md#p069) | System | `/system/sessions` |
| [P070 API Tokens / Service Accounts / SSH Keys](PAGES.md#p070) | System | `/system/credentials` |
| [P071 Vault](PAGES.md#p071) | System | `/system/vault` |
| [P072 Updates](PAGES.md#p072) | System | `/system/updates` |
| [P073 Backup / Restore](PAGES.md#p073) | System | `/system/backups` |
| [P074 Hardware Replacement / Port Mapping](PAGES.md#p074) | System | `/system/restore/mapping` |
| [P075 Hardware / Storage / Power](PAGES.md#p075) | System | `/system/hardware` |
| [P076 Security Posture / Integrity](PAGES.md#p076) | System | `/system/security` |
| [P077 Maintenance / Scheduler](PAGES.md#p077) | System | `/system/maintenance` |
| [P078 Recovery / Reboot / Reset](PAGES.md#p078) | System | `/system/recovery` |
| [P079 Preferences / Local-first](PAGES.md#p079) | System | `/system/preferences` |
| [P080 First Run / Local Registration](PAGES.md#p080) | System | `/setup` |
| [P081 Login / Locked Session](PAGES.md#p081) | System | `/login` |
| [P082 Incidents](PAGES.md#p082) | Logs / Diagnostics | `/diagnostics/incidents` |
| [P083 Logs / Live Tail](PAGES.md#p083) | Logs / Diagnostics | `/diagnostics/logs` |
| [P084 Events / Network Timeline](PAGES.md#p084) | Logs / Diagnostics | `/diagnostics/events` |
| [P085 Audit](PAGES.md#p085) | Logs / Diagnostics | `/diagnostics/audit` |
| [P086 Explain Why / What-if](PAGES.md#p086) | Logs / Diagnostics | `/diagnostics/explain` |
| [P087 Topology](PAGES.md#p087) | Logs / Diagnostics | `/diagnostics/topology` |
| [P088 Network Tools / Capture](PAGES.md#p088) | Logs / Diagnostics | `/diagnostics/tools` |
| [P089 Monitoring / History / Traffic](PAGES.md#p089) | Logs / Diagnostics | `/diagnostics/monitoring` |
| [P090 Duck Speed Lab](PAGES.md#p090) | Logs / Diagnostics | `/diagnostics/speedlab` |
| [P091 Duck Doctor / Support](PAGES.md#p091) | Logs / Diagnostics | `/diagnostics/doctor` |
| [P092 Retention / Storage Pressure](PAGES.md#p092) | Logs / Diagnostics | `/diagnostics/retention` |
| [P093 Lab / Acceptance / Fault Evidence](PAGES.md#p093) | System | `/system/lab` |
| [P094 Easy Home](PAGES.md#p094) | Easy | `/easy/home` |
| [P095 Easy Internet](PAGES.md#p095) | Easy | `/easy/internet` |
| [P096 Easy Networks / Wi-Fi](PAGES.md#p096) | Easy | `/easy/networks` |
| [P097 Easy Devices / Access](PAGES.md#p097) | Easy | `/easy/devices` |
| [P098 Easy Publish](PAGES.md#p098) | Easy | `/easy/publish` |
| [P099 Easy Care / Recovery](PAGES.md#p099) | Easy | `/easy/care` |

## Wireframes

Full, 1440 px; optional inspector keeps context:

```text
+ DuckOS / node-01        Search [Cmd K]     health: 2 issues   user +
+ 4 candidate objects | management affected | Review changes         +
| MAIN          | Routing pages | BGP / Peers          + Create peer |
| Interfaces    | Routes        | Search...  Filters  Columns  View  |
| WAN           | Policies      | Peer   AS    State   Accepted Age  |
| LAN / VLAN    | VRFs          | peer-a ...  Idle     —        3s   |
| Routing       | BGP           | peer-b ...  Estab.   143     10s   |
| Firewall      | OSPF          |                                    |
| ...           | ...           | selected rows   | Object inspector |
|               |               |                | Why / Test / Logs|
+ optional subtle mountain horizon in unused lower workspace        +
```

Firewall form, not four unrelated configuration screens:

```text
Publish HTTPS for NAS                         [Help]
Intent        NAS -> outside on WAN1
Match         allowed sources / TCP 443 / WAN1 address
Translation   WAN1:443 -> NAS 192.168.10.20:443
Permission    generated allow -> target (not router management)
Dependencies  DNAT + allow + target route [inspect each]
Local access  split DNS / optional hairpin [explain]
Review        exact exposure sentence / risk / management
              [Stage changes] [Cancel]
```

Mobile emergency:

```text
node-01  [Pages] [Search] [2 issues]
AWAITING CONFIRMATION 01:42
[Confirm] [Roll back now] [Details]
WAN1: address assigned; DNS failing
Last evidence 8s ago, IPv4, probe ...
[Test DNS] [Logs] [Open WAN]
```

Topology, ultrawide: filters and legend above; left-to-right graph takes remaining width; 360–480 px inspector on the right; relationship table toggle always available; no giant mandatory vertical tree or wallpaper.
## Named advanced tabs

P005 Templates has a stable **Provisioning (ZTP)** tab at `/system/templates?tab=provisioning`. Show credential ID, scope, node binding, creation/expiry/consumption time and authoritative state; never list tokens. Issue/export is privileged, audited without payload, memory-only and no-store. A stored expiry/consumed flag is not an enforcement guarantee. The backend needs cryptographically generated secrets, hashed storage, atomic single use, expiry and node-binding enforcement plus durable audit before enabling provisioning. Do not expose the current raw ZTPCredential model as a response. Diagnose rejected enrollment using redacted reason/clock evidence. Easy first-run remains local account setup, not mandatory external provisioning.

P027 Routes has a stable **Anycast** tab at `/routing/routes?tab=anycast`. Show name, VIP, local node, configured hysteresis, observed probe age and separate desired/observed advertisement. Create/edit stages VIP and health/withdrawal intent; preview BGP/route/service dependencies and management blast radius. Never equate the current Healthy boolean with a successful probe or announcement. Inspect service health, hold-down, accepted route and neighbor advertisement evidence; show Unknown where absent. Withdrawal requires impact review and the shared transaction contract; no guessed live action. Example: announce a DNS VIP only while a verified resolver probe succeeds, then withdraw after configured hysteresis. Both tabs inherit all 44 dimensions of their owning page, appear in local navigation and command search, and remain unavailable with an integration reason until public contracts exist.
