# Feature inventory and repository evidence

## Immutable baseline

Inspected branch: `V1 UI qualification branch`. Baseline: `120476163e347f875f46098c546ee6022e5efbd1`. There are **96 tracked internal Go package directories** with non-test Go files at this baseline. Empty leftover directories and historical “146 packages” reports are not counted as implemented packages. The checkout advanced during inspection; source links point to the current local paths for convenience, while the recorded commit is the reproducible inventory authority.

Method: enumerate tracked internal files, inspect exported object/operation declarations, follow public API dispatch and executable startup, inspect network/WAN/tunnel/identity/topology/capability/acceptance models, and compare historical QA evidence with actual code. This is a UX inventory and integration-boundary review, not a fresh security audit or runtime qualification campaign. No full tests or Linux operations were run for the design.

Read [BACKEND_GAPS](BACKEND_GAPS.md) before implementing calls. A method named Apply/Rollback may mutate an in-memory model only. A renderer may emit a real daemon format but not activate it. A registry may list a test but not execute it. API declarations that are not mounted are not reachable endpoints.

## Public surface found

| Surface | Current reachable behavior / gate | GUI home |
|---|---|---|
| GET /api/v1/health | Handler/schema health; not Internet/system/network health | P001, startup |
| GET /api/v1/overview | Authenticated Engine → Agent observations, bounded response | P001, P007, P027, P053 |
| GET /api/v1/ready | Agent readiness; not full product qualification | P001, P076 |
| GET /api/v1/doctor | Doctor snapshot via Agent; source completeness must be shown | P091 |
| GET /api/v1/topology | Bounded graph; richer per-edge confidence contract absent | P087 |
| GET /api/v1/multiwan | Registered empty topology by default; evaluator data not equivalent to live probes/kernel policy | P019 |
| GET /api/v1/qos | Status component exists; inspect provenance rather than infer applied state | P067 |
| GET /api/v1/ups | UPS state; live probe is a separate operation | P075 |
| GET /api/v1/sites | Requires ConfigureNetwork in current readOnly classification; default empty topology, ProbedAt=0 | P059 |
| GET /api/v1/wifi | Same current permission classification; default no reported wireless capability | P025 |
| POST /api/v1/network/preview, /simulate | Mounted before read-only method gate; local Owner enrollment and ConfigureNetwork required; preview/simulation are not mutation | P002, P086 |
| POST /api/v1/clients/export | Handler mounted; provider wiring required and unavailable by default startup | P048, P050, P051 |
| /api/v1/auth/status, enroll, login, me, logout, sessions, revoke, users, password, audit | Explicit method map and identity authorization, startup identity option required | P068–P071, P080–P081, P085 |
| QoS apply/rollback, UPS probe, migrate analysis | Inner POST handlers exist but outer ServeHTTP rejects non-GET/HEAD before reaching them; not usable HTTP operations at baseline | P067, P075, P006 |
| /agent/v1/network/... | Privileged IPC/control domain, not public browser API; API startup does not expose complete mutation bridge | P002, network editors |
| WireGuard/Xray component APIs | Internal handlers and CLI commands exist; not mounted in public main dispatch inspected; list/lifecycle coverage incomplete | P047–P052 |

Source anchors: [public handler](../../internal/api/server.go), [network handler](../../internal/api/network.go), [identity handler](../../internal/api/identity.go), [API startup](../../cmd/duck-api/main.go), [Agent startup](../../cmd/duck-agent/main.go), [CLI](../../cmd/duck/main.go), [Xray API](../../internal/xray/api.go), [WireGuard API](../../internal/wireguard/api.go).

The existing React foundation has overview/auth/topology/export components and shared overview types, not this complete route architecture. Existing code should be extended safely, not replaced merely because it is small.

## Feature-to-page inventory

Each row identifies capabilities, responsible objects/packages, administrator questions, proposed location, mode exposure, dependencies, observation semantics and diagnostics. The linked 44-field page contract adds operations, errors, credentials and hardware handling. These are the intended GUI homes; internal component coverage does not upgrade to public runtime support.

| Page / location | Features / objects | Source families | Operator / question | Easy exposure |
|---|---|---|---|---|
| [P001 System Overview](PAGES.md#p001) · `/system/overview` | hostname; uptime; CPU/RAM/storage; interface summaries; sessions; Internet evidence; pending changes | model observe doctor | What is failing now, what remains working, and is a change pending? | Home health summary without invented success |
| [P002 Candidate and Safe Apply](PAGES.md#p002) · `/system/changes` | candidate; diff; commit; commit-confirmed; locks; conflicts; canary; blast radius; transaction ordering | safeapply state engine agent canary secpolicy | What will change, can management survive, and what must I confirm? | Review changes with concrete effects and global timer |
| [P003 Configuration History](PAGES.md#p003) · `/system/history` | revisions; checkpoints; network time machine; compare; rollback | state safeapply incident canary | Which change preceded the problem and what would restoring change? | Recent changes and Restore known-good |
| [P004 Dependencies and Ownership](PAGES.md#p004) · `/system/objects` | dependency graph; reverse dependencies; provenance; generated ownership; detach; aliases; tags; clone | lifecycle intca canary | Who manages this object and what depends on it? | Managed-by details on related object |
| [P005 Templates](PAGES.md#p005) · `/system/templates` | templates; wizard-generated ordinary objects; parameter validation; ZTP one-time provisioning credentials | intca lifecycle | What will this template create and will later edits be overwritten? | Guided presets without hidden configuration |
| [P006 Import and Migration](PAGES.md#p006) · `/system/import` | VyOS; OpenWrt UCI; MikroTik; Kerio; generic Linux import; recognized/convertible/attention/unsupported | migrate backup | What was understood, what can convert, and what still needs a decision? | Guided import with no automatic apply |
| [P007 Interfaces](PAGES.md#p007) · `/interfaces` | physical; virtual; addresses; link; MTU; MAC; SFP; interface identity | network model hardware | Which port is this, what owns it, and what runs over it? | Ports and role assignment with physical guidance |
| [P008 Bridges and FDB](PAGES.md#p008) · `/interfaces/bridges` | bridge; FDB; static MAC; port membership; VLAN filtering | vlan network | Which ports share L2 and where was this MAC learned? | Network ports as part of LAN workflow |
| [P009 Bonding / LACP](PAGES.md#p009) · `/interfaces/bonds` | bonding; LACP; member failures; hash policy | bonding | Which links participate and what happens if a member fails? | Link aggregation guided explanation; Full edit |
| [P010 Loop and Storm Protection](PAGES.md#p010) · `/interfaces/loop-protection` | STP; RSTP; MSTP; BPDU/root/loop guards; storm control; MAC flapping | stp | Is a blocked port protecting the network or failing? | Loop warning plus safe recommended action |
| [P011 Discovery / LLDP](PAGES.md#p011) · `/interfaces/discovery` | LLDP; LLDP-MED; neighbors; evidence confidence | hardware nac topology | What device is on this port and how reliable is that claim? | Connected equipment detail |
| [P012 Port Security and Snooping](PAGES.md#p012) · `/interfaces/port-security` | port security; DHCP snooping; Option 82 trust; DAI; RA Guard; MAC limits | nac dhcpsnoop | Which ports may supply addressing and what was rejected? | Protect guest/network wizard |
| [P013 Mirroring / SPAN](PAGES.md#p013) · `/interfaces/mirroring` | SPAN; ERSPAN; capture destinations | nac capture | What traffic is copied, to where, and what is the resource/privacy impact? | Contextual diagnostic only |
| [P014 MACsec](PAGES.md#p014) · `/interfaces/macsec` | MACsec; secure associations; key lifecycle; replay protection | macsec vault | Is this link encrypted and is the peer authenticated? | Full-only with understandable link security status |
| [P015 Acceleration and SR-IOV](PAGES.md#p015) · `/interfaces/acceleration` | hardware offload; DSA; switchdev; devlink; SR-IOV; XDP/eBPF; IRQ/RSS/RPS/XPS; NUMA | nac hardware chaos | What bypasses software policy and which observability is lost? | Performance explanation only; advanced editing in Full |
| [P016 L2 Resilience / OAM](PAGES.md#p016) · `/interfaces/resilience` | MC-LAG; peer link; CFM; Ethernet OAM; ERPS; MRP | mclag oam | Is the redundant L2 path coordinated and where is continuity lost? | Incident evidence only |
| [P017 WAN Connections](PAGES.md#p017) · `/wan/connections` | DHCP; static; PPPoE; L2TP; MAC clone; addressing; provider; CGNAT; MTU/PMTU/MSS | wan network mtu ddns | At which layer does this uplink fail? | Internet setup workflow |
| [P018 Mobile / USB WAN](PAGES.md#p018) · `/wan/mobile` | modem; SIM; APN; signal; data budget; PIN; backup WAN | hardware capability wan | Can this modem provide a usable backup and at what cost? | Backup Internet setup when hardware present |
| [P019 Multi-WAN / Failover](PAGES.md#p019) · `/wan/failover` | route groups; ECMP; balancing; preferred exit; session impact; backup activation | multiwan vrrp | Why was this WAN chosen and what happens to existing sessions? | Primary/backup wizard |
| [P020 Path Quality / SD-WAN](PAGES.md#p020) · `/wan/path-quality` | IP SLA; latency; jitter; loss; path scoring; brownout; application class; SD-WAN | ipsla sdwan | Is a path technically up but unsuitable for this application? | Internet quality and backup reason |
| [P021 DDNS](PAGES.md#p021) · `/wan/ddns` | DDNS providers; external IP changes; CGNAT constraints | ddns sites | Does this name point to the reachable address? | Publish-service prerequisite |
| [P022 Networks / LAN / VLAN](PAGES.md#p022) · `/lan/networks` | LAN; VLAN; QinQ; coherent DHCP/DNS/ports/isolation; segmentation | network vlan | Who belongs to this network and what may they reach? | Create home/guest/segmented network |
| [P023 Address Plan / IPAM](PAGES.md#p023) · `/lan/address-plan` | IPAM; prefixes; pools; overlaps; reservations; assignments | nac ipv6 | Where can I allocate a subnet without overlap? | Suggest safe subnet with uncertainty visible |
| [P024 Clients / Devices](PAGES.md#p024) · `/lan/clients` | device identity; aliases; tags; active sessions; quarantine; NAC; reservations; trust | devices catalog nac accesslifecycle | Who is this device and why does it have this access? | Devices with clear online/last-seen distinction |
| [P025 Wi-Fi](PAGES.md#p025) · `/lan/wifi` | AP; station; SSID; security; hostapd; channels; radio capabilities | wifi hostapd hardware | Is radio service available and why is this client struggling? | Wi-Fi setup/status where supported, otherwise explanatory entry |
| [P026 IPv6 Addressing / PD](PAGES.md#p026) · `/lan/ipv6` | DHCPv6; SLAAC; RA; prefix delegation; IPv6 isolation | ipv6 network | Which delegated prefixes reach which networks and when will they expire? | IPv6 automatic with explicit status |
| [P027 Routes / RIB / FIB](PAGES.md#p027) · `/routing/routes` | configured routes; active routes; RIB; FIB; recursion; ECMP; route selection; Anycast VIP advertisement intent | network dynamicrt vrf nac | Why was this next hop selected and is it installed? | Where traffic goes summary |
| [P028 Policy Routing](PAGES.md#p028) · `/routing/policies` | IP rules; preferred exit; selective routing; route groups; fallback | network multiwan | Which match and priority sent this traffic to this table? | Send device/application through selected exit |
| [P029 VRF / Route Leaking](PAGES.md#p029) · `/routing/vrfs` | VRF; independent tables; members; route leaking | vrf | What is isolated and which prefixes may cross VRFs? | Explain tenant isolation; Full editing |
| [P030 BGP](PAGES.md#p030) · `/routing/bgp` | BGP peers; AFI/SAFI; received/advertised/accepted/rejected; attributes; reset | frr dynamicrt bgpadv | Is the peer established, what did it send, and why did routes get rejected? | Full only; incident prose in Easy |
| [P031 OSPF](PAGES.md#p031) · `/routing/ospf` | OSPFv2; OSPFv3; instances; areas; neighbors; LSDB; redistribution | frr dynamicrt | Why did this neighbor not reach Full and what routes changed? | Failure explanation only |
| [P032 IS-IS / RIP](PAGES.md#p032) · `/routing/other-protocols` | IS-IS; levels; NET; RIP; RIPng | isis dynamicrt | Which instance and neighbor provide these routes? | Full only |
| [P033 BFD](PAGES.md#p033) · `/routing/bfd` | BFD sessions; detection timers; dependent protocols | dynamicrt ipsla | Which consumers will react to this failure detector? | Dependent incident detail only |
| [P034 Route Filters / Redistribution](PAGES.md#p034) · `/routing/filters` | route filters; prefix lists; communities; redistribution; accepted/rejected explanation | dynamicrt frr bgpadv | Which term accepts or changes this prefix and who uses it? | Full only; policies explained by alias |
| [P035 RPKI / BMP / FlowSpec](PAGES.md#p035) · `/routing/bgp-security` | RPKI validators; ROAs; origin validation; BMP; FlowSpec; large communities | bgpadv | Is origin validation current and what distributed policy will affect traffic? | Full only; security summary |
| [P036 MPLS / Segment Routing](PAGES.md#p036) · `/routing/mpls` | MPLS; LDP; L3VPN; L2VPN; pseudowire; SR-MPLS; SRv6 | srmpls tunnels | Which labels/segments and VPN bindings carry this service? | Full only |
| [P037 VXLAN / EVPN](PAGES.md#p037) · `/routing/evpn` | VXLAN; VTEP; EVPN route types; multihoming; DF election | evpn tunnels | How is this tenant bridged/routed and who forwards on the segment? | Full only |
| [P038 Multicast / Discovery Forwarding](PAGES.md#p038) · `/routing/multicast` | PIM; IGMP; MLD; snooping; multicast routes; mDNS reflector | multicast mcast | Where should discovery/multicast go and why is it missing? | Share selected discovery services between networks |
| [P039 IPv6 Transition / CGNAT](PAGES.md#p039) · `/routing/transition` | NAT64; DNS64; 464XLAT; DS-Lite; 6rd; MAP-E; MAP-T; NPTv6; CGNAT pools | ipv6 ipv6trans | Which address families are translated and is capacity exhausted? | Provider setup explanation; Full controls |
| [P040 Gateway Redundancy / VRRP](PAGES.md#p040) · `/routing/redundancy` | VRRP; virtual addresses; master/backup; tracked health | vrrp | Which node currently owns this address and why? | Backup gateway status only |
| [P041 Access Policies](PAGES.md#p041) · `/firewall/access` | human-friendly access; Staff to NAS SMB; guest Internet only; tunnel isolation | network devices publish wireguard xray | Who may reach what and is router access separate? | Allow/block and access presets |
| [P042 Firewall Rules](PAGES.md#p042) · `/firewall/rules` | nftables; filter chains/hooks; rule order; generated rules; counters; raw advanced rules | network fwanalysis | Which rule sees this packet, in what order, and with what effect? | Access-policy equivalent, not raw chain wizard |
| [P043 NAT / Published Services](PAGES.md#p043) · `/firewall/nat` | DNAT; SNAT; masquerade; hairpin; NETMAP; publish local service | network publish sites fwanalysis | Which address changes, where is permission granted, and can outside clients connect? | Publish guided TCP/UDP intent with exact exposure sentence |
| [P044 Firewall Objects / Feeds](PAGES.md#p044) · `/firewall/objects` | sets; aliases; FQDN; Geo/ASN/threat feeds; service groups; address groups | fwanalysis multicast network | What addresses does this alias currently mean and who relies on it? | Named devices/networks/services |
| [P045 Connections / Conntrack](PAGES.md#p045) · `/firewall/connections` | sessions; conntrack; helper/ALG; selected kill; capacity | conntrack | Which sessions exist and what will terminating them interrupt? | Active sessions for selected client |
| [P046 Firewall Analysis / Protection](PAGES.md#p046) · `/firewall/protection` | shadow/conflict; firewall analyzer; DoS/flood; uRPF; CoPP; quarantine; optional IDS/IPS | fwanalysis secpolicy nac | What is unsafe, redundant or blocking legitimate traffic? | Security issues and recommended preview |
| [P047 WireGuard Servers](PAGES.md#p047) · `/tunnels/wireguard` | WireGuard interfaces; servers; key refs; listen; routes; isolation; MTU | wireguard vault | Who can connect and what can tunnel clients reach? | Give remote access wizard |
| [P048 WireGuard Peers / Exports](PAGES.md#p048) · `/tunnels/wireguard/peers` | peer lifecycle; QR; conf; copy; rotate; expire; client profile revision | wireguard clientexport accesslifecycle catalog | Is this peer authorized, is its client profile current, and can I export it safely? | Native mobile-scannable access card |
| [P049 Xray Servers](PAGES.md#p049) · `/tunnels/xray` | VLESS; Trojan; REALITY; TLS; transports; inbounds; native object management | xray vault certs | What protocol/security is listening and which dependencies expose it? | Guided private access server |
| [P050 Xray Users / Profiles](PAGES.md#p050) · `/tunnels/xray/profiles` | users; profiles; UUID/password; expiry; revoke; rotate; clone; tags; vless/trojan URI | xray accesslifecycle clientexport | Which identity/profile grants access and who needs refreshed credentials? | Give someone access with profile choices |
| [P051 Subscriptions](PAGES.md#p051) · `/services/subscriptions` | HAPP; Xray profile membership; token URL; QR; regenerate; export; expiry | subscription xray clientexport | What profiles does this URL expose and is its token still valid? | Subscription access card |
| [P052 Other Tunnels / NetBird](PAGES.md#p052) · `/tunnels/other` | IPsec/IKEv2; GRE; GRETAP; IPIP; VTI; GRE over IPsec; L2TP/IPsec; OpenVPN import; NetBird | tunnels capability | What traffic uses this tunnel and what is its actual protection? | Simple supported VPN workflows; unavailable explanation |
| [P053 DNS Resolver / Forwarding](PAGES.md#p053) · `/dns/resolver` | resolver; forwarding; upstreams; split DNS; DoT; DoH; DNSSEC; cache | network sites | Why did this client receive this answer and from which resolver? | DNS behavior in network/Internet setup |
| [P054 DNS Zones / Records](PAGES.md#p054) · `/dns/zones` | authoritative/reverse zones; local records; aliases; DDNS updates | network ddns | Which source owns this answer and where is it authoritative? | Friendly local names |
| [P055 Policy DNS / RPZ](PAGES.md#p055) · `/dns/policies` | RPZ; category/domain policy; selective DNS; feeds | network multicast | Was this answer blocked, rewritten or forwarded by policy? | Family/guest DNS policy with transparent limitations |
| [P056 DHCP Scopes / Relay](PAGES.md#p056) · `/dhcp/scopes` | DHCPv4/v6; scopes; pools; relay; Option 82; HA; DNS options | network dhcpsnoop nac | What will this network hand out and is a competing server present? | Combined Create network workflow |
| [P057 Leases / Reservations](PAGES.md#p057) · `/dhcp/leases` | leases; static reservations; hostnames; DDNS; IP conflicts | network devices dhcpsnoop | Who holds this address and can it be reserved safely? | Client reservation |
| [P058 Services / Listeners](PAGES.md#p058) · `/services` | runtime services; dependency-aware start/stop/reload; service state | svcstate model sites | What is listening, what is exposed, and what survives a restart? | Services with clear exposure scope |
| [P059 Sites / Reverse Proxy](PAGES.md#p059) · `/services/sites` | sites; reverse proxy; local/public services; TLS; DDNS | sites publish acme | Which hostname reaches which upstream and is access public? | Publish website/service safely |
| [P060 Certificates / ACME](PAGES.md#p060) · `/services/certificates` | certificate manager; import; ACME; renewal; trust chain; TLS dependencies | certs acme intca | Which services rely on this certificate and why did renewal fail? | Fix certificate from service context |
| [P061 Internal PKI / SSH CA](PAGES.md#p061) · `/services/pki` | CA; issuance; revocation; internal trust; SSH CA | intca certs chaos | Who trusts this authority and what does revocation affect? | Full only; trust instructions contextual |
| [P062 AAA / RADIUS / 802.1X](PAGES.md#p062) · `/services/aaa` | RADIUS; AAA; 802.1X; realms; CoA; clients; NAC | radiusd hardware nac | Which identity decision assigned this access and is the server reachable? | Enterprise access wizard if supported |
| [P063 Time / NTP / NTS / PTP](PAGES.md#p063) · `/services/time` | NTP client/server; NTS; PTP; PHC; GPS source; TIME_UNTRUSTED | ntp ptpd timeintegrity chaos hardware | Can security decisions trust time and which source supplies it? | Time setup with offline trust warning |
| [P064 Telemetry / Integrations](PAGES.md#p064) · `/services/integrations` | SNMPv3; remote syslog; NetFlow/IPFIX/sFlow; webhooks; TR-369/USP optional | snmp secpolicy chaos logging | What leaves this node, to whom, and what if collection fails? | Notifications with explicit data disclosure |
| [P065 UPnP / NAT-PMP / PCP / Wake](PAGES.md#p065) · `/services/discovery` | UPnP; NAT-PMP; PCP; Wake-on-LAN | upnp devices capability | Who requested this mapping and when does it expire? | Device wake and requested service access where supported |
| [P066 Portal](PAGES.md#p066) · `/services/portal` | captive portal; identity/trust entry; local-first portal architecture | nac accesslifecycle sites | What access is granted before and after sign-in? | Guest access workflow only when implemented |
| [P067 QoS / Traffic Scheduling](PAGES.md#p067) · `/routing/qos` | CAKE; fq_codel; HTB; HFSC; shaping; policing; DSCP; hierarchy; per-VRF/tenant | qos qos2 | Which queue constrains this traffic and is congestion controlled? | Prioritize calls / control bandwidth |
| [P068 Users / Roles](PAGES.md#p068) · `/system/users` | local owners/admin/operators/viewers; RBAC; accounts; password lifecycle | identity accesslifecycle | Who can administer the node and what can they do? | People who manage this node |
| [P069 Sessions / Temporary Access](PAGES.md#p069) · `/system/sessions` | sessions; temporary admin; emergency access; revocation | identity accesslifecycle recovery | Which access is active, how long, and can I revoke it? | Revoke access clearly |
| [P070 API Tokens / Service Accounts / SSH Keys](PAGES.md#p070) · `/system/credentials` | API tokens; service accounts; SSH keys; host key rotation; clone protection | intca identity chaos | Which automation credentials exist and what can they reach? | Full only, generated access explained |
| [P071 Vault](PAGES.md#p071) · `/system/vault` | secret references; key IDs; encrypted storage; rotation/revoke; recovery | vault agent | Which services depend on this secret and can it be recovered safely? | Secret required instructions contextual only |
| [P072 Updates](PAGES.md#p072) · `/system/updates` | Stable/Preview/Nightly; signed .duck; OTA; offline package; Active/Previous/Candidate; SBOM | upgrade integrity storagewatch timeintegrity | What will upgrade change and can I return to the previous version? | Guided safe update |
| [P073 Backup / Restore](PAGES.md#p073) · `/system/backups` | portable backup; encrypted secrets; selective sections; drag/drop restore; schema migration | backup vault upgrade | What is inside this backup and what changes on this node? | One-file replacement workflow |
| [P074 Hardware Replacement / Port Mapping](PAGES.md#p074) · `/system/restore/mapping` | old-to-new NIC role mapping; MAC/link/speed/driver; identify physical port | backup install network hardware | Which new port replaces each old logical role without losing access? | Guided replacement steps |
| [P075 Hardware / Storage / Power](PAGES.md#p075) · `/system/hardware` | capabilities; SFP; PoE/watchdog; USB; GPS; Bluetooth; crypto; virtualization; sensors; SMART/NVMe; RAID; UPS | hardware integrity capability ups | What hardware is present, limited or failing and what is the impact? | Hardware issue and practical next step |
| [P076 Security Posture / Integrity](PAGES.md#p076) · `/system/security` | guardrails; readiness; TPM; Secure Boot; SBOM; kernel modules; firmware; override | secpolicy integrity chaos capability | Which protection is active, unavailable or deliberately overridden? | Security summary in plain language |
| [P077 Maintenance / Scheduler](PAGES.md#p077) · `/system/maintenance` | maintenance window; scheduled jobs; bounded suppressions; canary; quotas | maintenance scheduler canary secpolicy | What will run when and what will still alert during maintenance? | Schedule update/backup with consequences |
| [P078 Recovery / Reboot / Reset](PAGES.md#p078) · `/system/recovery` | emergency mode; reboot; shutdown; factory reset; decommission; crash recovery | recovery powercrash canary secpolicy | What remains recoverable and exactly what will be erased or interrupted? | Emergency recovery task list |
| [P079 Preferences / Local-first](PAGES.md#p079) · `/system/preferences` | EN/RU/DE/ES; Light/Dark/System; Easy/Full; density; offline; telemetry privacy | localfirst chaos firstrun | How do I adapt this workspace without changing network behavior? | Appearance/language and mode choice |
| [P080 First Run / Local Registration](PAGES.md#p080) · `/setup` | first owner; language/theme; Router/Server/Custom; NIC safety; MAC clone; real tests | firstrun install identity management | How can I establish safe local administration and connectivity? | Primary first-run workflow |
| [P081 Login / Locked Session](PAGES.md#p081) · `/login` | authenticated entry; session expired; neutral mascot; local-first login | identity | Can I securely sign in to this node? | Same login for both modes |
| [P082 Incidents](PAGES.md#p082) · `/diagnostics/incidents` | root incidents; child events; health correlation; 30-second triage | incident canary doctor secpolicy | What failed first, who is affected, and what should I test next? | Something is wrong with factual impact |
| [P083 Logs / Live Tail](PAGES.md#p083) · `/diagnostics/logs` | bounded logs; per-object levels; live tail; search; filters; drop counters; external fallback | logging support secpolicy | What happened to this object and is logging itself losing evidence? | Relevant logs from issue/client/service context |
| [P084 Events / Network Timeline](PAGES.md#p084) · `/diagnostics/events` | configuration/network time machine; events; before/after correlation | incident state canary | What changed around the loss of connectivity? | Recent meaningful changes |
| [P085 Audit](PAGES.md#p085) · `/diagnostics/audit` | actor; before/after; source; reason; result; transaction; rollback; secret access metadata | identity catalog accesslifecycle state | Who changed or exported what and what was the result? | Access/change history |
| [P086 Explain Why / What-if](PAGES.md#p086) · `/diagnostics/explain` | explainability; policy simulation; DNS/category; access; firewall; route; tunnel; NAT; before/after | explain network dynamicrt | Why is this path selected or blocked, and what would the candidate do? | Why blocked/Why WAN2 guided question |
| [P087 Topology](PAGES.md#p087) · `/diagnostics/topology` | physical/logical topology; confidence; links; bridges; VLANs; WAN/tunnels; devices | topology hardware nac explain | What is connected or configured here and what evidence supports each edge? | Small focused topology with same evidence semantics |
| [P088 Network Tools / Capture](PAGES.md#p088) · `/diagnostics/tools` | ping; traceroute; TCP probe; DNS query; route lookup; PCAP; PMTU; trace logging | nettools capture mtu | Can I test this exact context safely and what did the test prove? | Guided connection test |
| [P089 Monitoring / History / Traffic](PAGES.md#p089) · `/diagnostics/monitoring` | throughput; latency/jitter/loss; resources; sessions; queue drops; flow telemetry | observe doctor observability qos ipsla logging | What changed over time and where is the bottleneck? | Traffic and quality summary |
| [P090 Duck Speed Lab](PAGES.md#p090) · `/diagnostics/speedlab` | latency; throughput; saturation; bounded benchmark; QoS validation | speedlab qos mtu | Is slowness the local path, ISP, DNS, or queueing? | Why Internet is slow guided test |
| [P091 Duck Doctor / Support](PAGES.md#p091) · `/diagnostics/doctor` | health findings; subsystem diagnostics; support bundle; privacy redaction | doctor observability support | What evidence can I collect safely for diagnosis? | One coherent help workflow |
| [P092 Retention / Storage Pressure](PAGES.md#p092) · `/diagnostics/retention` | hard-bounded logging; external sink failure; RAM/disk quotas; degradation order | storagewatch logging secpolicy canary | What will be dropped before routing is affected? | Storage warning with safe cleanup only |
| [P093 Lab / Acceptance / Fault Evidence](PAGES.md#p093) · `/system/lab` | lab mode; acceptance runner; chaos; canary; performance/readiness; crash evidence; watchdog | acceptance canary chaos qa secpolicy powercrash | What is actually qualified and can this test affect management? | Read-only readiness explanation; no chaos actions |
| [P094 Easy Home](PAGES.md#p094) · `/easy/home` | Internet; health; active backup; clients; traffic; incident; Safe Apply | model doctor multiwan | Is normal use working and what needs attention? | Primary Easy start |
| [P095 Easy Internet](PAGES.md#p095) · `/easy/internet` | WAN setup; backup; DNS; MTU auto; quality; MAC clone | wan multiwan sdwan mtu | Which step stops Internet working and can a backup help? | Primary Internet workflow |
| [P096 Easy Networks / Wi-Fi](PAGES.md#p096) · `/easy/networks` | home/guest/segmented network; VLAN; DHCP; DNS; Wi-Fi; isolation | network wifi | Who should share this network and what should they reach? | Primary network intent workflow |
| [P097 Easy Devices / Access](PAGES.md#p097) · `/easy/devices` | devices; access presets; WireGuard; Xray; subscriptions; QR lifecycle | devices wireguard xray subscription | Who needs access, for how long, and how do they connect? | Primary client workflow |
| [P098 Easy Publish](PAGES.md#p098) · `/easy/publish` | publish service; public exposure; certificate/DNS dependencies | publish sites network | What exactly will become reachable from outside? | Primary safe publication workflow |
| [P099 Easy Care / Recovery](PAGES.md#p099) · `/easy/care` | updates; backups; incident troubleshooting; restore; emergency | upgrade backup doctor recovery | How do I keep this node safe or recover it without shell commands? | Primary maintenance workflow |

## No-contract and partial-contract features

These are preserved in the GUI design, not invented implementation claims.

| Capability | Baseline evidence limit | Discoverable location / behavior |
|---|---|---|
| Complete native BGP/OSPF monitoring | FRR/dynamicrt structs and renderers; current generic session state cannot supply full neighbor/LSDB/FIB telemetry | P030/P031; partial data and missing detail, no guessed neighbors |
| BFD | Dynamic session flag exists; complete BFD session telemetry/action contract not verified | P033 available-information panel |
| MPLS/LDP/SRv6/L2VPN/pseudowire | srmpls includes capability values; actual detailed live configuration differs by technology | P036 technology-specific supported/experimental/pending states |
| DoH/DoT/DNSSEC/RPZ/authoritative zones | Baseline common network DNS only upstreams and name/address records; complete public resolver/zone/policy API absent | P053–P055; keep requested subpages without fake DNS service |
| DHCP relay/HA/Option 82/advanced v6 | Partial snooping/HA/PD models; no universal activated service API | P012/P026/P056 |
| NetBird/OpenVPN interoperability | Requested architecture and generic tunnel concepts; mounted native management/import not verified | P052 explicit unsupported/not exposed, not raw JSON |
| IDS/IPS, Geo/FQDN/threat feeds | Some analysis/L7 concepts; no verified public feed or IDS runtime contract | P044/P046 distinguish available analysis from absent enforcement |
| SNMP/flow telemetry/remote syslog/USP | Several config/state stores; mounted delivery/flow-read APIs not verified | P064/P089 |
| Captive portal and Wake-on-LAN | Requirement home; no mounted operation established from inspected startup | P065/P066 |
| Hardware functions | Capability models for SFP/PoE/PTP/SR-IOV/TPM etc; probes/runtime integration vary | P015/P075/P076 per-feature source/reason; no global green from enum |
| Full CRUD/clone/ownership/detach | Component graph/provenance models; public revision-aware object API incomplete | P004 and contextual actions, disabled with G02/G08 |
| Localization, search, table cursors and help metadata | Frontend/platform contracts required; no comprehensive API registry | All routes, G04/G06/G07/G15 |

## Ownership and operations inventory by package

This exhaustive symbol index prevents unusual backend objects from disappearing into a generic “System” label. Boilerplate constructors, validators and serializers are omitted from operations where they do not create user actions. Component methods named Set/Add/Render are never automatically bound to buttons. Composite packages intentionally own multiple logical product objects; no duplicate parallel backend is proposed.

### acceptance

GUI locations: P093. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [scenarios.go](../../internal/acceptance/scenarios.go), [scenarios_r12.go](../../internal/acceptance/scenarios_r12.go).

Objects/contracts: ScenarioID, Outcome, Result.

Operations/helpers: AllScenarios.

### accesslifecycle

GUI locations: P024, P048, P050, P066, P068, P069, P085. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [accesslifecycle.go](../../internal/accesslifecycle/accesslifecycle.go).

Objects/contracts: SubjectKind, Subject, HistoryEntry, Manager.

Operations/helpers: Active, Add, Rotate, Revoke, ExpireSweep, Get, List, ActiveAt, History.

### acme

GUI locations: P059, P060. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [acme.go](../../internal/acme/acme.go).

Objects/contracts: ChallengeKind, DirectoryEntry, Account, Order, Challenge, DNSProvider, Transport, StubTransport, Provider, RenewalState, Renewal, Manager, Result.

Operations/helpers: GetDirectory, GetChallenges, FinalizeOrder, DownloadCert, Revoke, KeyAuthorization, DNSName, Add, Get, List, Renew, MarshalState, VerifyNoPlaintext.

### agent

GUI locations: P002, P071. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [doctor.go](../../internal/agent/doctor.go), [doctor_host.go](../../internal/agent/doctor_host.go), [managed_routing.go](../../internal/agent/managed_routing.go), [migrate.go](../../internal/agent/migrate.go), [migrate_host.go](../../internal/agent/migrate_host.go), [multiwan.go](../../internal/agent/multiwan.go), [multiwan_host.go](../../internal/agent/multiwan_host.go), [network.go](../../internal/agent/network.go), [qos.go](../../internal/agent/qos.go), [qos_host.go](../../internal/agent/qos_host.go), [server.go](../../internal/agent/server.go), [sites.go](../../internal/agent/sites.go), [sites_host.go](../../internal/agent/sites_host.go), [socket_linux.go](../../internal/agent/socket_linux.go), [socket_other.go](../../internal/agent/socket_other.go), [topology.go](../../internal/agent/topology.go), [topology_host.go](../../internal/agent/topology_host.go), [ups.go](../../internal/agent/ups.go), [ups_host.go](../../internal/agent/ups_host.go), [vault_lifecycle.go](../../internal/agent/vault_lifecycle.go), [wifi.go](../../internal/agent/wifi.go), [wifi_host.go](../../internal/agent/wifi_host.go).

Objects/contracts: DoctorObserver, DoctorHost, EvidenceCollector, AdapterFactory, ManagedRoutingDeps, ManagementEvidenceAdapter, MigrateObserver, MigrateObservable, MigrateHost, MultiWANObserver, MultiWANHost, NetworkStatus, NetworkApply, NetworkService, QoSObserver, QoSController, QoSObservable, QoSStateSink, QoSControllerFactory, QoSHost, Observer, Handler, SitesObserver, SitesObservable, SitesHost, TopologyObserver, TopologyHost, InterfaceProvider, DeviceProvider, WireGuardProvider, XrayProvider, UPSObserver, UPSProber, UPSObservable, UPSProberFactory, UPSHost, VaultLifecycle, VaultDeps, VaultResolver, WifiObserver, WifiObservable, WifiHost.

Operations/helpers: ServeDoctor, Doctor, Collect, ResolveNetworkStateDir, ServeMigrate, MigrateFile, Migrate, ServeMultiWAN, Topology, UpdateTopology, RecordProbe, LastSelections, MemberHealth, MultiWAN, State, Error, EnableNetwork, StartupVerify, Close, MarkRecovery, ServeQoS, ServeQoSApply, ServeQoSRollback, QoS, Apply, Rollback, PreviewState, SetSink, EnableDoctor, EnableTopology, EnableMultiWAN, EnableQoS, EnableUPS, EnableMigrate, EnableSites, EnableWifi, ServeHTTP, ServeSites, Sites, Accept, Listen, ServeTopology, ServeUPS, ServeUPSProbe, UPS, Probe, OpenVault, Resolver, KeyID, List, Revoke, Put, Resolve, ServeWifi, Wifi, UpdateCapability.

### api

GUI locations: P081, P001. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [client_export.go](../../internal/api/client_export.go), [identity.go](../../internal/api/identity.go), [network.go](../../internal/api/network.go), [server.go](../../internal/api/server.go), [tls.go](../../internal/api/tls.go).

Objects/contracts: Engine, Handler.

Operations/helpers: EnableIdentity, LoadToken, Close, ServeHTTP, LoadTLSConfig, EnableTLS.

### backup

GUI locations: P006, P073, P074, P099. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [backup.go](../../internal/backup/backup.go), [restore.go](../../internal/backup/restore.go), [verify.go](../../internal/backup/verify.go).

Objects/contracts: Kind, Entry, Manifest, Envelope, BackupMode, InterfaceBinding, LogicalManifest, RemapRequest, RestorePlan, MigratorFunc, Migrators, Verifier, Ed25519Verifier, StaticVerifier.

Operations/helpers: Build, Digest, Remap, PlanRestore, Register, Apply, Verify, Inspect, VerifyEnvelope, VerifyDigest.

### bgpadv

GUI locations: P030, P034, P035. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [bgpadv.go](../../internal/bgpadv/bgpadv.go).

Objects/contracts: Community, LargeCommunity, RPKIState, ROA, RPKIResult, RPKIServer, RPKIManager, BMPTarget, FlowSpec, FlowSpecMatch, FlowSpecActionKind, FlowSpecAction, FlowSpecManager.

Operations/helpers: String, SetServer, SetROAs, ROAs, Add, Revoke, SweepExpired, List, OriginAllowed, PrefixLen.

### bonding

GUI locations: P009. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [bonding.go](../../internal/bonding/bonding.go).

Objects/contracts: Mode, HashMode, Member, Bond, Manager.

Operations/helpers: Set, Get, List, Delete.

### canary

GUI locations: P002, P003, P004, P077, P078, P082, P084, P092, P093. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [canary.go](../../internal/canary/canary.go).

Objects/contracts: ApplyScope, Canary, CanaryStore, TxnStep, TxnPlan, Incident, IncidentStore, PathSnapshot, PathDiff, PathCapture, StaleObject, StaleDetector, DestructiveAction, ActionCatalog, RetentionTier, RetentionStore, Decommission, DecommissionStore, Lab, LabStore, ScenarioResult, ScenarioRun, AccRunner.

Operations/helpers: Add, Promote, All, PlanTopo, Open, Correlate, Close, CaptureBefore, CaptureAfter, Diff, Register, Get, SetTiers, Tiers, Advance, Record, Counts.

### capability

GUI locations: P018, P052, P065, P075, P076. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [capability.go](../../internal/capability/capability.go).

Objects/contracts: Feature, Status, Report, Probe, Manager, ActionResult.

Operations/helpers: AllFeatures, RegisterProbe, SetReport, Detect, ReportFor, CheckAction.

### capture

GUI locations: P013, P088. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [capture.go](../../internal/capture/capture.go), [capture_linux.go](../../internal/capture/capture_linux.go), [capture_other.go](../../internal/capture/capture_other.go).

Objects/contracts: Request, Result, State, Capture, ActiveRef, Host, Manager.

Operations/helpers: Start, Status, Cancel, List.

### catalog

GUI locations: P024, P048, P085. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [client.go](../../internal/catalog/client.go), [clients.go](../../internal/catalog/clients.go), [files.go](../../internal/catalog/files.go), [history.go](../../internal/catalog/history.go), [schema.go](../../internal/catalog/schema.go), [store.go](../../internal/catalog/store.go).

Objects/contracts: Client, Entry, Store.

Operations/helpers: ExportEligible, Get, List, Save, Changes, History, Initialize, Open, Close.

### certs

GUI locations: P049, P060, P061. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [certs.go](../../internal/certs/certs.go).

Objects/contracts: Kind, Source, ImportedCert.

Operations/helpers: Fingerprint, GenerateKey, MakeSelfSigned.

### chaos

GUI locations: P015, P061, P063, P064, P070, P076, P079, P093. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [chaos.go](../../internal/chaos/chaos.go).

Objects/contracts: Readiness, FeatureReadiness, ReadinessStore, Fault, FaultManager, TelemetryPref, TelemetryStore, CrashEvidence, CrashStore, IRQCPUPerf, PerfStore, NTSState, NTSStore, USPState, USPStore, SSHHostKey, SSHKeyStore, CloneProtection, CloneStore, ConsoleCommand, ConsoleCatalog.

Operations/helpers: Set, Get, All, Add, Record, Register.

### clientexport

GUI locations: P048, P050, P051. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [document.go](../../internal/clientexport/document.go), [service.go](../../internal/clientexport/service.go), [subscription.go](../../internal/clientexport/subscription.go), [wireguard.go](../../internal/clientexport/wireguard.go).

Objects/contracts: Document, Access, Provider, Service, WireGuardClient.

Operations/helpers: Reveal, Filename, ID, Revision, Kind, Download, String, GoString, LogValue, MarshalJSON, Export, SubscriptionURL, GenerateWireGuardKey, WireGuard.

### cmdmeta

GUI locations: P004, P086. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [cmdmeta.go](../../internal/cmdmeta/cmdmeta.go).

Objects/contracts: Operation, OpKind, Entry, Registry.

Operations/helpers: Register, Get, ByObject, List, RenderCLI.

### conntrack

GUI locations: P045. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [conntrack.go](../../internal/conntrack/conntrack.go).

Objects/contracts: Action, Filter, Entry, Stats, Manager, ChangeImpact.

Operations/helpers: SetStats, Stats, List, KillSelected.

### ddns

GUI locations: P017, P021, P054. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ddns.go](../../internal/ddns/ddns.go).

Objects/contracts: ProviderKind, Record, State, Manager.

Operations/helpers: SetRecord, SetState, GetRecord, GetState, List, Delete, CanEnableForPublicIP.

### devices

GUI locations: P024, P041, P057, P065, P097. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [devices.go](../../internal/devices/devices.go).

Objects/contracts: Role, Policy, Device, Catalog, Observation.

Operations/helpers: IsValidRole, Record, ListDevices, GetDevice, ForgetDevice, UpsertPolicy, RemovePolicy, LookupPolicy, EffectiveRole, IsQuarantined, Close.

### dhcpsnoop

GUI locations: P012, P056, P057. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [dhcpsnoop.go](../../internal/dhcpsnoop/dhcpsnoop.go).

Objects/contracts: TrustClass, PortClass, Lease, ARPBinding, RAObservation, Manager.

Operations/helpers: SetPort, SetLease, ClassOf, LeaseForMAC, BindingForIP, RecordRA, RogueRAs.

### doctor

GUI locations: P001, P082, P089, P091, P094, P099. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [doctor.go](../../internal/doctor/doctor.go), [io.go](../../internal/doctor/io.go), [network_linux.go](../../internal/doctor/network_linux.go), [network_other.go](../../internal/doctor/network_other.go), [system_linux.go](../../internal/doctor/system_linux.go), [system_other.go](../../internal/doctor/system_other.go).

Objects/contracts: Snapshot, Schema, System, Resources, Memory, Storage, Network, Interface, Throughput, Route, DNS, Firewall, Vault, Identity, Devices, WireGuard, Xray, Logging, Subsystem, Issue, Source, Builder, NetworkSource, NetworkSource, SystemSource, SystemSource.

Operations/helpers: GeneratedAt, Uptime, Environment, SetSchema, SetSystem, SetResources, SetNetwork, SetDNS, SetFirewall, SetVault, SetIdentity, SetDevices, SetWireGuard, SetXray, SetLogging, AddSubsystem, AddIssue, Snapshot, Name, Contribute.

### dynamicrt

GUI locations: P027, P030, P031, P032, P033, P034, P086. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [dynamicrt.go](../../internal/dynamicrt/dynamicrt.go).

Objects/contracts: Protocol, AS, Session, SessionState, Route, Filter, FilterAction, TraceStep, FilterTrace, LookingGlass.

Operations/helpers: SetSession, SetRoute, SetFilter, Sessions, Session, Routes, Filters, Trace.

### engine

GUI locations: P002. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [client_export.go](../../internal/engine/client_export.go), [engine.go](../../internal/engine/engine.go), [network.go](../../internal/engine/network.go), [network_control.go](../../internal/engine/network_control.go), [network_simulation.go](../../internal/engine/network_simulation.go).

Objects/contracts: Engine, Networking, NetworkState.

Operations/helpers: EnableClientExports, ExportClient, Overview, Ready, Doctor, Topology, MultiWAN, QoS, QoSApply, QoSRollback, UPS, UPSProbe, Migrate, Sites, Wifi, PreviewNetwork, EnableNetworking, Close, Apply, Status, Confirm, Rollback, SimulateNetwork.

### evpn

GUI locations: P037. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [evpn.go](../../internal/evpn/evpn.go).

Objects/contracts: MultihomingMode, VTEP, EthernetSegment, VNI, Config, DFState, DFResult, Manager.

Operations/helpers: Render, Set, SetDF, DF, DFAll.

### explain

GUI locations: P086, P087. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [chain.go](../../internal/explain/chain.go), [expand.go](../../internal/explain/expand.go).

Objects/contracts: EvidenceKind, HopKind, Decision, Hop, Chain, Endpoint, Engine, TopologySnapshot, InterfaceInfo, VLANInfo, DeviceInfo, WANInfo, RouteInfo, RuleInfo, NATInfo, TunnelInfo, Builder, ServiceEndpoint, VRFHop, BGPSessionHop, OSPFSessionHop, RouteFilterHop, LoopHop, Expand, ChainExpand, ExplainBuilder, FilterRule.

Operations/helpers: Build, Label, AddVRF, AddBGP, AddOSPF, AddRouteFilter, AddLoop, AddHop, ClassifyRouteFilter, DetectRouteLoop, DetectL2Loop.

### firstrun

GUI locations: P079, P080. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [firstrun.go](../../internal/firstrun/firstrun.go).

Objects/contracts: Profile, ConnectivityState, Stage, State, Manager.

Operations/helpers: OfflineReady, SetProfile, SetStage, State, Stages, SuggestProfile.

### frr

GUI locations: P030, P031, P034. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [frr.go](../../internal/frr/frr.go).

Objects/contracts: Daemon, AS, Neighbor, Network, BGPSession, OSPFArea, OSPFInstance, Config, State, LifecycleEvent, Manager.

Operations/helpers: Render, Set, Rollback, History.

### fwanalysis

GUI locations: P042, P043, P044, P046. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [fwanalysis.go](../../internal/fwanalysis/fwanalysis.go).

Objects/contracts: RuleSeverity, Finding, FirewallRule, NATRule, Analyzer, URPFMode, URPF, CoPP, Quarantine, QuarantineManager.

Operations/helpers: AnalyzeRules, AnalyzeNAT, Add, Remove, SweepExpired, Active.

### hardware

GUI locations: P007, P011, P015, P018, P025, P062, P063, P074, P075, P087. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [hardware.go](../../internal/hardware/hardware.go).

Objects/contracts: LLDPMode, LLDPSettings, NICCapabilities, OffloadState, SFPInfo, PoEClass, PoEState, PoEWatchdog, RADIUSServer, ModemSummary, UPSSummary, Capability, Manager, Snapshot.

Operations/helpers: SetLLDP, SetNIC, SetPoEState, SetPoEWatchdog, SetRADIUS, SetModem, SetUPS, SetCapabilities, Snapshot.

### hostapd

GUI locations: P025. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [hostapd.go](../../internal/hostapd/hostapd.go).

Objects/contracts: Band, Security, BSS, Config, Station, Manager.

Operations/helpers: Render, Set, Rollback, SetStation, Stations.

### identity

GUI locations: P068, P069, P070, P080, P081, P085. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [client_export.go](../../internal/identity/client_export.go), [password.go](../../internal/identity/password.go), [roles.go](../../internal/identity/roles.go), [service.go](../../internal/identity/service.go).

Objects/contracts: Role, Permission, User, Audit, Session, Service.

Operations/helpers: RecordClientExport, Allowed, Open, Close, Enrolled, Enroll, Login, Authorize, Current, Logout, Sessions, Revoke, Users, CreateUser, UpdateUser, ChangePassword, Audit.

### incident

GUI locations: P003, P082, P084. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [incident.go](../../internal/incident/incident.go).

Objects/contracts: Severity, ChangeEvent, TimelineEvent, HealthObject, Checkpoint, View, Sources, Builder, FuncSources.

Operations/helpers: Order, Build, Health, Changes, Timeline, Checkpoints.

### install

GUI locations: P074, P080. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [detect.go](../../internal/install/detect.go), [role.go](../../internal/install/role.go).

Objects/contracts: Detector, Proposal, Role, Decision, NetworkSnapshot, Interface.

Operations/helpers: Propose, JSON.

### intca

GUI locations: P004, P005, P060, P061, P070. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [intca.go](../../internal/intca/intca.go).

Objects/contracts: CA, IssuedCert, CAStore, APIToken, APITokenStore, Provenance, ProvenanceStore, Template, TemplateStore, ZTPCredential, ZTPStore.

Operations/helpers: AddCA, Issue, Revoke, CAs, Issued, Add, Touch, All, Set, TakeOverControl, ReleaseControl, Get, Consume, Active.

### integrity

GUI locations: P072, P075, P076. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [integrity.go](../../internal/integrity/integrity.go).

Objects/contracts: SecureBootState, TPMState, Integrity, IntegrityStore, SBOMComponent, SBOMStore, KernelModule, Firmware, KernelFWStore, Temp, Fan, SMART, HWHealth, Disk, RAID, StorageStore.

Operations/helpers: Set, Get, Add, All, SetModule, SetFirmware, MissingRequired, Modules, FirmwareAll, ObserveTemp, ObserveFan, SetSMART, Temps, Fans, SMARTAll, SetDisk, SetRAID, Disks, RAIDs.

### ipsla

GUI locations: P020, P033, P089. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ipsla.go](../../internal/ipsla/ipsla.go).

Objects/contracts: ProbeKind, Probe, State, Sample, ProbeState, Manager, Track.

Operations/helpers: Set, Get, State, States, Ingest.

### ipv6

GUI locations: P023, P026, P039. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ipv6.go](../../internal/ipv6/ipv6.go).

Objects/contracts: AddressMode, PDState, Assignment, PDConfig, NAT64Config, DNS64Config, Manager.

Operations/helpers: SetPD, PD, ListPD, SetNAT64, NAT64, SetDNS64, DNS64.

### ipv6trans

GUI locations: P039. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ipv6trans.go](../../internal/ipv6trans/ipv6trans.go).

Objects/contracts: TransitionKind, Capability, Prefix, CGNATPool, ExhaustionState, CGNATManager, TransitionManager.

Operations/helpers: Supports, AddPool, GetPool, SetExhaustion, Exhaustion, SetCapability, AddPrefix, Prefixes.

### isis

GUI locations: P032. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [isis.go](../../internal/isis/isis.go).

Objects/contracts: Level, Instance, Neighbor, Config, Manager.

Operations/helpers: Render, Set, Rollback.

### lifecycle

GUI locations: P004, P005. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [dependencies.go](../../internal/lifecycle/dependencies.go), [serve.go](../../internal/lifecycle/serve.go).

Objects/contracts: ObjectKind, Object, Reference, Graph, DeletionPlan.

Operations/helpers: Add, Remove, Dependents, Targets, Preflight, Serve.

### localfirst

GUI locations: P079. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [localfirst.go](../../internal/localfirst/localfirst.go).

Objects/contracts: Connectivity, Subsystem, Declaration, Manager, CloudIndependentReport.

Operations/helpers: AllSubsystems, Set, Get, List, Evaluate.

### logging

GUI locations: P064, P083, P089, P092. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [logger.go](../../internal/logging/logger.go).

Objects/contracts: Level, Severity, Record, Subsystem, Config, Logger.

Operations/helpers: String, DefaultConfig, DefaultReplacer, SetLevel, Level, Log, Snapshot, Counts, Subscribe, Unsubscribe, Close, ListLevels, Errorf, Infof, Debugf, Tracef, LevelFor, Subsystems.

### macsec

GUI locations: P014. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [macsec.go](../../internal/macsec/macsec.go).

Objects/contracts: Capability, SA, Manager.

Operations/helpers: SetCapability, Capability, AddSA, RemoveSA, SAs.

### maintenance

GUI locations: P077. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [maintenance.go](../../internal/maintenance/maintenance.go).

Objects/contracts: SuppressionKind, Session, AuditEntry, Manager.

Operations/helpers: Open, Close, AppendAudit, Active, IsSuppressed.

### management

GUI locations: P080. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [plane.go](../../internal/management/plane.go).

Objects/contracts: AccessRole, PathKind, Path, Policy, Manager.

Operations/helpers: Policy, Set, AddPath, RemovePath, Allows, ListPaths.

### mcast

GUI locations: P038. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [mcast.go](../../internal/mcast/mcast.go).

Objects/contracts: Mode, Network, MDNSReflectorPolicy, Manager.

Operations/helpers: SetNetwork, SetMDNS, Networks, MDNS, CanReflect.

### mclag

GUI locations: P016. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [mclag.go](../../internal/mclag/mclag.go).

Objects/contracts: Role, PeerLink, Domain, Manager.

Operations/helpers: Set, Get, List, SetPeerLink.

### migrate

GUI locations: P006. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [analyse.go](../../internal/migrate/analyse.go), [migrate.go](../../internal/migrate/migrate.go), [vyos.go](../../internal/migrate/vyos.go).

Objects/contracts: FormatKind, Status, Confidence, Source, Item, Plan, Summary.

Operations/helpers: Analyse, IsSuccess, Summarise, SortItemsByLine, DetectFormat.

### model

GUI locations: P001, P007, P058, P094. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [overview.go](../../internal/model/overview.go), [validate.go](../../internal/model/validate.go).

Objects/contracts: Overview, System, CPU, Memory, Storage, Interface, Address, Route, DNS, Service, NFTables, Issue.

Operations/helpers: no exported operations beyond construction/validation/encoding.

### mtu

GUI locations: P017, P088, P090, P095. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [mtu.go](../../internal/mtu/mtu.go).

Objects/contracts: LinkMTU, PMTUReport, Probe, Manager.

Operations/helpers: ClampMSS, SetLink, SetReport, Links, Reports, ClampMSSForLink.

### multicast

GUI locations: P038, P044, P055. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [mcastrt.go](../../internal/multicast/mcastrt.go).

Objects/contracts: Protocol, RP, Entry, State, Manager, L7Rule, L7Manager.

Operations/helpers: SetProtocol, AddRP, SetIGMPSnoop, SetState, RPF, Snapshot, Add, List.

### multiwan

GUI locations: P019, P028, P094, P095. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ecmp.go](../../internal/multiwan/ecmp.go), [multiwan.go](../../internal/multiwan/multiwan.go).

Objects/contracts: SelectionPolicy, GroupMember, RouteGroup, FlowKey, Manager, ConntrackImpact, Strategy, HealthMethod, Member, Probe, Group, Topology, ProbeResult, ProbeRunner, Evaluator, Selection.

Operations/helpers: Select, Set, Get, List, Delete, Build, Topology, UpdateTopology, RecordProbe, LastSelections, MemberHealth, ShortSummary, IsHealthy.

### nac

GUI locations: P011, P012, P013, P015, P023, P024, P046, P056, P062, P066, P087. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [nac.go](../../internal/nac/nac.go).

Objects/contracts: Device, Manager, Prefix, IPAMStore, Pool, PortSecurity, LLDPNeighbor, PortSecStore, SPANSession, SPANStore, AccelBackend, AccelStore, SRIOVPort, SRIOVStore, DHCPHAPeer, DHCPHAStore, AnycastService, AnycastStore.

Operations/helpers: Observe, Approve, Quarantine, Devices, AddPrefix, AddPool, Prefixes, Pools, Set, ObserveLLDP, Neighbors, Add, Remove, All, Get.

### nettools

GUI locations: P088. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [nettools.go](../../internal/nettools/nettools.go).

Objects/contracts: NetworkContext, PingRequest, PingResult, TracerouteRequest, Hop, TracerouteResult, TCPProbeRequest, TCPProbeResult, DNSQueryRequest, DNSQueryResult, RouteLookupRequest, RouteLookupResult, LiveCaptureRequest.

Operations/helpers: no exported operations beyond construction/validation/encoding.

### network

GUI locations: P007, P008, P017, P022, P026, P027, P028, P041, P042, P043, P044, P053, P054, P055, P056, P057, P074, P086, P096, P098. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [artifacts.go](../../internal/network/artifacts.go), [boot_guard.go](../../internal/network/boot_guard.go), [boot_recovery.go](../../internal/network/boot_recovery.go), [boot_validation.go](../../internal/network/boot_validation.go), [config.go](../../internal/network/config.go), [dns_host.go](../../internal/network/dns_host.go), [dns_service.go](../../internal/network/dns_service.go), [enrolled_firewall.go](../../internal/network/enrolled_firewall.go), [enrollment.go](../../internal/network/enrollment.go), [firewall_adapter.go](../../internal/network/firewall_adapter.go), [firewall_host.go](../../internal/network/firewall_host.go), [forward_guard.go](../../internal/network/forward_guard.go), [forward_program.go](../../internal/network/forward_program.go), [host_snapshot.go](../../internal/network/host_snapshot.go), [host_snapshot_linux.go](../../internal/network/host_snapshot_linux.go), [host_snapshot_other.go](../../internal/network/host_snapshot_other.go), [managed_routing.go](../../internal/network/managed_routing.go), [management.go](../../internal/network/management.go), [management_evidence.go](../../internal/network/management_evidence.go), [management_evidence_linux.go](../../internal/network/management_evidence_linux.go), [management_evidence_other.go](../../internal/network/management_evidence_other.go), [management_verifier.go](../../internal/network/management_verifier.go), [networkd_host.go](../../internal/network/networkd_host.go), [networkd_reload.go](../../internal/network/networkd_reload.go), [nft_snapshot.go](../../internal/network/nft_snapshot.go), [policy.go](../../internal/network/policy.go), [render.go](../../internal/network/render.go), [route_host.go](../../internal/network/route_host.go), [route_mutation.go](../../internal/network/route_mutation.go), [route_plan.go](../../internal/network/route_plan.go), [route_snapshot.go](../../internal/network/route_snapshot.go), [routed_kernel.go](../../internal/network/routed_kernel.go), [rule_host.go](../../internal/network/rule_host.go), [rule_snapshot.go](../../internal/network/rule_snapshot.go), [simulate.go](../../internal/network/simulate.go), [testfixture.go](../../internal/network/testfixture.go).

Objects/contracts: ArtifactDirectory, BootKernel, RoutedBootKernel, Config, Interface, DHCPServer, Reservation, Route, Firewall, Forward, Rule, DNS, Record, DNSServiceSnapshot, DNSServiceHost, EnrolledFirewallAdapter, Enrollment, PhysicalBinding, FirewallAdapter, FirewallHost, HostSnapshot, HostLink, ManagedRoutingAdapter, ManagementPolicy, ManagementRule, NetlinkSnapshotter, ProcNetReader, LLDPProbe, ManagementEvidenceCollector, ManagementAuthority, ManagedInterface, ManagementEvidence, ManagementLinkEvidence, ManagementAddressEvidence, ManagementListenerEvidence, ManagementNeighborEvidence, NetworkdLink, NetworkdReloadHost, NFTSnapshot, PolicyRule, Plan, PrivateRouteHost, KernelRoute, RouteSnapshot, RoutedKernel, PolicyRuleHost, KernelPolicyRule, PolicySnapshot, TrafficQuestion, TrafficStep, TrafficExplanation.

Operations/helpers: OpenArtifactDirectory, OpenDNSArtifacts, Close, Check, Replace, Restore, VerifyRoutingBootGuard, RecoverRoutingArtifacts, EnrolledTables, CaptureRoutes, CaptureRules, CheckDNSMasq, Capture, SetActive, QuiesceForRecovery, Write, Preflight, Apply, Verify, RestoreCaptured, CheckBindings, ReadEnrollment, InstallForwardGuard, ApplyGuarded, PhysicalBindings, CaptureHost, AttachDNSArtifacts, Collect, Listeners, Error, VerifyManagementOwnership, CheckNetworkdOwnership, ReadNetworkdInventory, Reload, ReloadRestored, Fingerprint, RestoreBatch, Render, PrivateRoutesForConfig, PolicySnapshotForConfig, Simulate, PrivateTempDir.

### ntp

GUI locations: P063. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ntp.go](../../internal/ntp/ntp.go).

Objects/contracts: SourceKind, Source, Status, ServerConfig, PTPConfig, Manager, TrustDecision.

Operations/helpers: SetSource, RemoveSource, Sources, SetStatus, Statuses, SetServer, Server, SetPTP, PTP, EvaluateTrust.

### oam

GUI locations: P016. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [oam.go](../../internal/oam/oam.go).

Objects/contracts: CFMMD, CFMMA, CFMState, OAMState, Manager, RingState, RingManager.

Operations/helpers: SetMD, SetMA, RecordCFM, CFMAll, Set, Get, All.

### observability

GUI locations: P089, P091. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [observability.go](../../internal/observability/observability.go).

Objects/contracts: Severity, Object, Doctor, Source, Registry, FuncSource.

Operations/helpers: Order, Add, Sort, Snapshot, Register, Collect, Kind.

### observe

GUI locations: P001, P089. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [collector.go](../../internal/observe/collector.go), [collector_linux.go](../../internal/observe/collector_linux.go), [collector_other.go](../../internal/observe/collector_other.go), [drift.go](../../internal/observe/drift.go), [parse.go](../../internal/observe/parse.go).

Objects/contracts: Collector, DriftKind, ReconciliationPolicy, Drift, Store, Report.

Operations/helpers: Write, Overview, Set, Get, List, Delete, Detect.

### powercrash

GUI locations: P078, P093. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [powercrash.go](../../internal/powercrash/powercrash.go).

Objects/contracts: Operation, Outcome, JournalEntry, RecoveryReason, Report, Journal, AtomicPublisher, Stages, RecoveryState, Decider.

Operations/helpers: Begin, End, Abort, Inspect, Decide.

### ptpd

GUI locations: P063. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [ptpd.go](../../internal/ptpd/ptpd.go).

Objects/contracts: Timestamping, Profile, Clock, PHCSys, Capability, Manager.

Operations/helpers: Render, RenderPHCSys, SetClocks, Clocks, SetCapability, Capabilities.

### publish

GUI locations: P041, P043, P059, P098. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [publish.go](../../internal/publish/publish.go).

Objects/contracts: Protocol, Intent, Status, Plan, Published, Manager.

Operations/helpers: ComputePlan, Set, Get, List, Delete, PlanJSON.

### qa

GUI locations: P093. Test-only infrastructure; never a production configuration object.

Sources: [lab_linux.go](../../internal/qa/lab_linux.go), [lab_other.go](../../internal/qa/lab_other.go), [lab_types.go](../../internal/qa/lab_types.go), [secretcanary.go](../../internal/qa/secretcanary.go).

Objects/contracts: ManagementBoundary, Lab, Canary.

Operations/helpers: CaptureManagementBoundary, AssertBoundaryEqual, VethPair, VethBetween, Run, RunShell, CleanupLeaked, LinkExists, TempTree, DefaultManagementBoundary, MustSeedFile, ScanForLeak, AssertNoLeak, ContainsLeak, AssertNoLeakInString, TempDir, GoroutineCount.

### qos

GUI locations: P067, P089, P090. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [adapter_linux.go](../../internal/qos/adapter_linux.go), [adapter_other.go](../../internal/qos/adapter_other.go), [controller.go](../../internal/qos/controller.go), [errors.go](../../internal/qos/errors.go), [qos.go](../../internal/qos/qos.go), [render.go](../../internal/qos/render.go).

Objects/contracts: Executor, ExecRunner, LinuxRunner, Executor, LinuxRunner, ApplyOutcome, Runner, RunnerState, Controller, DiffResult, StepDiff, Algorithm, Direction, Priority, Shaper, Class, Filter, Topology, ShaperSummary, State, Status, StepKind, Step, Plan.

Operations/helpers: Run, Apply, Rollback, State, Current, Previous, Last, PreviewState, DiffPlan, HasShapers, Build, Summarise, Topology, UpdateTopology, MarkApplied, MarkError, Status, Render, PlanJSON, ToText.

### qos2

GUI locations: P067. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [qos2.go](../../internal/qos2/qos2.go).

Objects/contracts: QDiscKind, Scope, Class, Policy.

Operations/helpers: Render.

### radiusd

GUI locations: P062. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [radiusd.go](../../internal/radiusd/radiusd.go).

Objects/contracts: Server, Client, Realm, CoAState, CoARecord, Manager.

Operations/helpers: Render, Set, Rollback, RecordCoA, CoARecords.

### recovery

GUI locations: P069, P078, P099. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [recovery.go](../../internal/recovery/recovery.go).

Objects/contracts: ResetKind, EmergencyAccess, Environment, RebootReason, RebootEvent, ResetRequest, ResetImpact, Manager, ShutdownStep, ShutdownPlan.

Operations/helpers: Active, Impact, SetEnvironment, Environment, OpenEmergency, CloseEmergency, ActiveEmergency, RecordReboot, RebootHistory, DefaultShutdownPlan.

### safeapply

GUI locations: P002, P003. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [checkpoint.go](../../internal/safeapply/checkpoint.go), [controller.go](../../internal/safeapply/controller.go), [diff.go](../../internal/safeapply/diff.go), [lock.go](../../internal/safeapply/lock.go), [static_adapter.go](../../internal/safeapply/static_adapter.go), [worker.go](../../internal/safeapply/worker.go).

Objects/contracts: CheckpointID, Checkpoint, CheckpointStore, Adapter, SnapshotAdapter, Controller, ChangeKind, DiffObject, Impact, Diff, SessionID, Session, LockManager, StaticAdapter, Job, Worker.

Operations/helpers: Put, Get, GetByLabel, List, Remove, Open, Status, Apply, ApplyRequest, Confirm, Rollback, Close, ComputeDiff, Begin, End, Active, Authorize, Preflight, Verify, Restore, RejectAdmission, Submit, SubmitRequest.

### scheduler

GUI locations: P077. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [scheduler.go](../../internal/scheduler/scheduler.go).

Objects/contracts: JobKind, Schedule, Job, Manager, TickResult.

Operations/helpers: Add, Remove, Disable, Get, List, Tick.

### sdwan

GUI locations: P020, P095. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [sdwan.go](../../internal/sdwan/sdwan.go).

Objects/contracts: Criteria, Weight, ApplicationClass, Path, Policy, PathScore, Brownout, Health, Manager.

Operations/helpers: DetectBrownout, Score, Set, Evaluate, Health.

### secpolicy

GUI locations: P002, P046, P064, P076, P077, P078, P082, P083, P092, P093. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [secpolicy.go](../../internal/secpolicy/secpolicy.go).

Objects/contracts: Policy, Violation, PolicyStore, PostureFinding, Posture, PostureStore, BlastRadius, BlastStore, CrashState, CrashRecord, CrashManager, WatchdogState, WatchdogManager, ResourceBudget, BudgetStore, Quota, QuotaStore, Webhook, WebhookStore.

Operations/helpers: Set, Override, All, Get, Record, Reset, Add.

### sites

GUI locations: P021, P043, P053, P058, P059, P066, P098. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [sites.go](../../internal/sites/sites.go).

Objects/contracts: Protocol, DDNSProvider, Listener, Target, TLS, DDNS, Site, Topology, Health, Status, SiteSummary.

Operations/helpers: Build, Summarise.

### snmp

GUI locations: P064. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [snmp.go](../../internal/snmp/snmp.go).

Objects/contracts: Version, AuthProtocol, PrivProtocol, User, Trap, Manager.

Operations/helpers: SetUser, SetTrap, Users, Traps.

### speedlab

GUI locations: P090. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [speedlab.go](../../internal/speedlab/speedlab.go).

Objects/contracts: Kind, Target, Request, Summary, LatencyStats, ThroughStats, State, Probe, ActiveRef, Host, Manager.

Operations/helpers: Start, Status, Cancel, List, Finalize, ReduceLatency.

### srmpls

GUI locations: P036. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [srmpls.go](../../internal/srmpls/srmpls.go).

Objects/contracts: Capability, CapabilitySet, RouteDistinguisher, RouteTarget, L3VPN, MPLSPath, SRPolicy, Config, Manager, Capabilities.

Operations/helpers: Supports, String, Render, Set, Get.

### state

GUI locations: P002, P003, P084, P085. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [store.go](../../internal/state/store.go).

Objects/contracts: Revision, Phase, Transaction, Snapshot, Outcome, Store.

Operations/helpers: Reconcile, RejectRequest, Open, OpenExisting, Close, Snapshot, Prepare, PrepareWithRecovery, PrepareRequest, Transition, Commit, RolledBack.

### storagewatch

GUI locations: P072, P092. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [storagewatch.go](../../internal/storagewatch/storagewatch.go).

Objects/contracts: Mode, PathStat, Priority, Subsystem, Manager, PreflightResult, DegradationPlan.

Operations/helpers: Compute, Order, SetStat, SetSubsystem, Stats, WorstPath, UpdatePreflight, PlanDegradation.

### stp

GUI locations: P010. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [stp.go](../../internal/stp/stp.go).

Objects/contracts: Mode, Bridge, Port, StormConfig, MACMove, MacFlap, Manager.

Operations/helpers: SetBridge, SetPort, SetStorm, RecordMove, Flaps.

### subscription

GUI locations: P051, P097. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [service.go](../../internal/subscription/service.go).

Objects/contracts: Subscription, Service, XraySnapshot.

Operations/helpers: PublicURL, SetClock, SetSnapshot, Create, Get, List, Update, Revoke, Delete, Render, TokenFor, CertHealth.

### support

GUI locations: P083, P091. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [support.go](../../internal/support/support.go).

Objects/contracts: Pattern, Redactor, File, Bundle, BundleBuilder.

Operations/helpers: MustPattern, DefaultPatterns, AddPattern, Patterns, Redact, RedactBytes, Add, Build.

### svcstate

GUI locations: P058. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [svcstate.go](../../internal/svcstate/svcstate.go).

Objects/contracts: State, Service, ValidateFunc, ApplyFunc, Manager, HealthSnapshot.

Operations/helpers: Healthy, Set, Get, List, ReloadConfig, Rollback, Snapshot.

### testfixture

GUI locations: P093. Test-only infrastructure; never a production configuration object.

Sources: [overview.go](../../internal/testfixture/overview.go).

Objects/contracts: none exported.

Operations/helpers: Overview.

### timeintegrity

GUI locations: P063, P072. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [timeintegrity.go](../../internal/timeintegrity/timeintegrity.go).

Objects/contracts: State, Source, SourceReport, Tracker, Status.

Operations/helpers: State, DriftMS, HaveAnchor, Reset, Observe, AnchorManual, RequireTrusted, StatusSnapshot.

### topology

GUI locations: P011, P087. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [graph.go](../../internal/topology/graph.go).

Objects/contracts: NodeKind, EdgeKind, Node, Edge, Graph, InterfaceSpec, DeviceSpec, WireGuardSpec, XraySpec, Builder.

Operations/helpers: AddInterface, AddDevice, AddWireGuard, AddXray, Snapshot.

### tunnels

GUI locations: P036, P037, P052. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [tunnels.go](../../internal/tunnels/tunnels.go).

Objects/contracts: Kind, State, Tunnel, IPSecConfig, VXLANConfig, MPLSConfig, Manager.

Operations/helpers: Set, Get, List, Delete, HasCapability.

### upgrade

GUI locations: P072, P073, P099. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [healthcheck.go](../../internal/upgrade/healthcheck.go), [migrate.go](../../internal/upgrade/migrate.go), [orchestrator.go](../../internal/upgrade/orchestrator.go), [package.go](../../internal/upgrade/package.go), [signature.go](../../internal/upgrade/signature.go), [storage.go](../../internal/upgrade/storage.go).

Objects/contracts: CheckRunner, CheckResult, Executor, MigrationStep, MigrationChain, MigrationPlan, RunResult, Channel, Policy, HealthCheck, HistoryEntry, State, Snapshot, Plan, Release, Preflight, ReleaseStorage, Result, Orchestrator, Manifest, ReleaseID, Header, Package, SignatureVerifier, Ed25519Verifier, StaticVerifier, ReleaseStore, ReleaseMeta.

Operations/helpers: RunAll, Run, Add, Plan, SetState, State, Stage, Activate, ActivateAndVerify, Rollback, SetChannel, SetPolicy, History, Verify, FormatVersion, Marshal, Fingerprint, FormatArch, GenerateSigningKey, Materialize, List, ReadMeta, ReadPayload, Remove, FreeBytes, VerifyFingerprint, String.

### upnp

GUI locations: P065. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [upnp.go](../../internal/upnp/upnp.go).

Objects/contracts: Protocol, Network, Mapping, Manager.

Operations/helpers: EnableNetwork, DisableNetwork, IsEnabled, AddMapping, RevokeMapping, Mappings, Networks.

### ups

GUI locations: P075. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [runner.go](../../internal/ups/runner.go), [ups.go](../../internal/ups/ups.go).

Objects/contracts: Executor, ExecRunner, ProbeResult, Runner, Health, Device, Variable, Topology, Status, State, DeviceSummary.

Operations/helpers: Run, Probe, ProbeAll, IsCritical, Build, Topology, UpdateTopology, RecordProbe, MarkError, Status, HealthOf, Summarise, ClassifyHealth.

### vault

GUI locations: P014, P047, P049, P071, P073. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [key.go](../../internal/vault/key.go), [record.go](../../internal/vault/record.go), [store.go](../../internal/vault/store.go).

Objects/contracts: MasterKey, Reference, Secret, Store.

Operations/helpers: String, GoString, LogValue, MarshalJSON, Clear, OpenVault, InitializeVault, ProvisionMasterKey, LoadMasterKey, Reveal, Seal, Open, InitializeStore, OpenStore, Close, Get, Put, ListReferences, Revoke, IsRevoked, KeyID.

### vlan

GUI locations: P008, P022. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [vlan.go](../../internal/vlan/vlan.go).

Objects/contracts: PVID, PortMode, Port, Bridge, StaticMAC, Manager.

Operations/helpers: Set, Get, List, Delete.

### vrf

GUI locations: P027, P029. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [vrf.go](../../internal/vrf/vrf.go).

Objects/contracts: ID, Table, VRF, Member, Route, Leak, Lookup, LookupResult, Manager.

Operations/helpers: Set, Get, List, Delete, Lookup, JSON.

### vrrp

GUI locations: P019, P040. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [vrrp.go](../../internal/vrrp/vrrp.go).

Objects/contracts: State, Group, Election, Manager.

Operations/helpers: Decide, Set, Get, List, Delete, Transition.

### wan

GUI locations: P017, P018, P095. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [provider.go](../../internal/wan/provider.go).

Objects/contracts: Mode, MAC, Source, MACConfig, MTUConfig, StaticConfig, PPPoEConfig, L2TPConfig, Provider.

Operations/helpers: JSON.

### wifi

GUI locations: P025, P096. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [wifi.go](../../internal/wifi/wifi.go).

Objects/contracts: Band, Security, Capability, AP, Station, Topology, Health, APSummary, Status.

Operations/helpers: IsLegacy, Build, Summarise.

### wireguard

GUI locations: P041, P047, P048, P097. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [adapter.go](../../internal/wireguard/adapter.go), [adapter_linux.go](../../internal/wireguard/adapter_linux.go), [adapter_other.go](../../internal/wireguard/adapter_other.go), [api.go](../../internal/wireguard/api.go), [server.go](../../internal/wireguard/server.go).

Objects/contracts: KeyResolver, KernelAdapter, Runner, Filesystem, RecoverySnapshot, IPRouteRunner, LocalFilesystem, VaultKeyResolver, IPRouteRunner, LocalFilesystem, VaultKeyResolver, Access, CatalogPersistence, API, Server, KeyRef, Peer.

Operations/helpers: ApplyServer, RemoveServer, MarshalRecoverySnapshot, Run, WriteAtomic, Remove, Resolve, Error, ServeHTTP, IsValidIntent, GenerateServerKey, IsExpired, AllowedIPsForIntent.

### xray

GUI locations: P041, P049, P050, P051, P097. Internal component; exported names are not proof of mounted API or live qualification.

Sources: [api.go](../../internal/xray/api.go), [render.go](../../internal/xray/render.go), [runtime_linux.go](../../internal/xray/runtime_linux.go), [runtime_other.go](../../internal/xray/runtime_other.go), [server.go](../../internal/xray/server.go).

Objects/contracts: Access, CatalogPersistence, API, Config, Inbound, Outbound, Routing, RoutingRule, DNS, Sniffing, Runtime, State, Runtime, State, Protocol, Transport, Security, Server, KeyRef, User, Profile, Subscription.

Operations/helpers: ServeHTTP, RenderConfig, BuildShareLink, State, Apply, Stop, Rollback, Error, IsValidProfileIntent, IsValidProtocol, IsValidTransport, IsValidSecurity, IsExpired, SortServers, SortUsers.


## Test / readiness evidence

The acceptance registry contains the original A–R-named scenarios (including separate ACME success/failure) plus numbered 1–65 scenarios. Registry presence is not PASS evidence. GUI must display run outcome pass/fail/skip/pending, build revision, target, evidence, time and hardware/credential constraints. Historical QA reports are source-specific records, not current global qualification.

The design explicitly maps all acceptance themes: single-NIC/offline, MAC binding, CGNAT/DNS failure, certificates/time, Xray/WireGuard lifecycle, management loss, two-admin conflicts, drift, dependency delete, replacement restore, incompatible schema, upgrade/power loss, disk-full/read-only, Multi-WAN/session impact, IPv6 leak/RA, L2 loops/flaps/storm/STP, PMTU, hairpin/split DNS, publish lifecycle, daemon crashes, bounded watchdog/maintenance/emergency, hardware pending, BGP/OSPF/VRRP/LACP, QinQ, redaction, secret backup and incident triage.
