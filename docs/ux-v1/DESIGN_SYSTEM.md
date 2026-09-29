# Visual design, icons, accessibility and responsive behavior

## Visual character

A professional instrument with a quiet mountain identity. Dense opaque working surfaces, readable typography and restrained color. No glass over tables, giant cards, gamer neon, crypto gradients or greeting copy. Full prioritizes exact comparison and action safety; Easy has more atmosphere without sacrificing evidence.

Mountains are a local static vector silhouette or approved local panorama, never fetched from a remote server. In Full they may occupy the lower 10–15% of unused workspace on a tall display, behind no text, controls or table cells. They are not a mandatory reserved 15% row. On short viewports, long tables, mobile emergency screens, contrast mode or user preference, suppress the atmospheric layer. Easy/login may use a larger atmospheric side/background area beneath an opaque reading surface. Dark and Light have separately tuned mountain values.

Primary identity stays the existing two-line ASCII mark from `pkg/brand/mark.txt`:

```text
<(o)__
 (___/
```

Preserve whitespace, monospace and disabled ligatures. Mark is aria-hidden beside the accessible word DuckOS. Optional glasses mascot belongs only to login/local registration/onboarding/tasteful release explanation; no mascot in serious tables, charts or QR codes. Asset creation is outside this documentation task. A missing optional asset does not become a fabricated DuckOS brand image or block local operation.

## Design tokens

These values are implementation decisions, not screenshots sampled from competitors. Relative rem sizes scale with user text settings.

| Token | Dark | Light | Purpose |
|---|---|---|---|
| workspace | #10151B | #F2F5F7 | base |
| surface | #182029 | #FFFFFF | opaque tables/forms |
| surface-raised | #202B36 | #E8EEF3 | inspector/control grouping |
| text-primary | #E8EDF2 | #18232E | normal text |
| text-secondary | #A9B5C1 | #526273 | useful secondary values, not tiny low-contrast labels |
| accent/link/focus | #71B7EC | #005E9F | selected action/link/focus, underline links |
| positive | #83CEA5 | #17663C | verified positive text + symbol |
| warning | #E9BE72 | #805000 | actionable uncertainty/impact |
| danger | #F08E8E | #A0212B | failed/critical + text |
| control-border | #667B8F | #73808E | interactive outlines |
| separator | #2B3845 | #D1D9E0 | decorative row separation only |

Tokens for status are foreground colors on base/surface, not permission to use colored text on any tinted background. Verify every actual pair; do not assume opacity preserves contrast. Primary buttons use accent background plus contrasting workspace-colored text. Error/warning surfaces stay opaque and readable. Current selection has a start-edge indicator and a textual/ARIA state, not just a blue fill.

Scale: 4/8/12/16/24/32 px spacing. Main nav width 208 px (optional 184–256 user resize); section nav 192 px; inspector 400 px default (320–560 allowed). Header 48 px; candidate strip minimum 40 px, grows for translations. Page padding 16 px Full / 24 px Easy, reduced safely at narrow widths.

Typography: local system sans stack (system-ui, Segoe UI, Arial, sans-serif) supporting Cyrillic, German and Spanish. Monospace (ui-monospace, SFMono-Regular, Consolas, Liberation Mono, monospace) for IP/MAC/prefix/table IDs/CLI. Body 14 px equivalent, line-height 1.45; dense table 13 px minimum at default scale; explanatory/help 14–16; page title 22; section 16. No normal content at 10 px. Tabular numerals for counters and timer. Use real bold sparingly. IPs remain selectable and do not wrap mid-octet; horizontal scroll or copy action preserves exact values.

Radii: 4 px controls, 6 px inspector/panels, 8 px modal. Avoid pill-shaped containers for every value. Border separators, not many floating card shadows. Motion 120–180 ms only for meaningful disclosure/state transition; no infinite ambient motion. Reduced-motion disables transitions and mascot animation.

## Component system

- AppShell, StableMainNav, SectionIndex, NodeHeader, GlobalHealthSummary, CandidateStrip.
- PageHeader (title, brief purpose, Help, primary action), FilterBar, DataTable, SavedViewControl, BulkActionBar.
- ObjectLink, ObjectInspector, DependencyList/Graph, OwnershipBadge, DesiredObservedPair, EvidenceTimestamp.
- IntentEditor, AdvancedFieldGroup, ValidationSummary, ImpactPreview, RevisionConflict, TransactionPanel.
- StatusBadge (support/intent/observation axes), PartialDataNotice, MissingCapabilityPanel, PermissionNotice.
- ExplainChain, BeforeAfterComparison, TopologyLens, RelationshipTable, MetricChart with gap semantics.
- ContextHelp, CommandPalette, SecretExportModal, FileReview, PortMapping, IncidentTimeline.
- Field primitives: IP/prefix; MAC; alias/object selector; service/port; route table/VRF; duration; rate+unit; secret reference; capability-constrained select.

Separate domain field schemas from presentation; preserve backend semantics and exact units. Do not implement one giant generic key/value editor for every domain. Share primitives and state behavior; page-specific fields come from the catalog and backend schemas.

## Table and form layout

At 1440 px the firewall table prioritizes order/name, traffic context, source, destination, service, action and owner. Counters and technical priority may be selectable columns or inspector details; fields are not lost. Group headers never hide unknown-count objects without an explicit count. Policy reordering has keyboard Move before/after.

Labels above fields, persistent units, error tied to field, concise hint and Help when meaningful. No placeholder-only labels. IP/network/service selectors allow human names but reveal exact resolved scope. Port ranges distinguish source from destination. Risky controls use action verbs, not unlabeled toggles that apply immediately.

Dialogs are reserved for focused decisions/reveal. Large configuration editors are route pages with staged footer. Confirmation button names the action and target; neutral Cancel receives initial focus for destructive confirmations. Do not force the user to scroll past a huge diff to find the live rollback timer.

## Icon grammar

Use original code-native SVG composed from stable base shapes: interface/endpoint, address block, route path, boundary/gate, document, identity, evidence. 20 px default nav artboard; 16 px dense-table variant designed specifically rather than scaling indiscriminately; 24 px for prominent actions. 1.5 px stroke at 20 px, optically adjusted at 16 px; consistent caps/joins; minimum open gap 2 px. Monochrome base glyph follows text. No raster icon dependency.

Qualifiers are semantic, not random decorations: tag tab for VLAN/label; compartments for VRF; gate before path for policy; sleeve for encapsulation; clock marker for expiry; document seal for certificate. A route is not a policy route, WAN is not verified Internet, certificate is not a private key, event is not an incident. Below is the full page-icon recommendation, including ambiguity to test.

Active navigation uses background/start marker, not a new filled icon that changes its meaning. State overlay, when needed, occupies one reserved bottom-right corner: pause/slash = intentionally disabled; triangle + ! = warning; octagon + × = failed; hollow circle + ? = unknown; small hourglass = pending. Do not stack three tiny badges; show the dominant status symbol plus text/details. Expiry warning stays a badge, never a clock-shaped replacement for the base certificate icon.

Decorative icons aria-hidden. Icon-only control has visible tooltip and localized accessible name. Status also has text. Never require interpreting color or icon alone for a destructive decision.

| Page | Glyph / metaphor | Ambiguity to prevent |
|---|---|---|
| P001 System Overview | `overview`: Instrument panel with two factual readouts | Avoid chart as generic success |
| P002 Candidate and Safe Apply | `candidate`: Two offset documents with a pending corner | Not an upload or committed check |
| P003 Configuration History | `history`: Revision nodes on a timeline | Not a generic clock/time service |
| P004 Dependencies and Ownership | `ownership`: Parent block linked to child block | Not a person or sharing permission |
| P005 Templates | `template`: Reusable stencil with small object slots | Not an imported file |
| P006 Import and Migration | `import`: Document entering a bounded review tray | Not applied configuration |
| P007 Interfaces | `interface`: RJ45-style port with distinct contacts | Not a whole switch chassis |
| P008 Bridges and FDB | `bridge`: Two port groups joined by a horizontal span | Not VLAN tagging |
| P009 Bonding / LACP | `bond`: Two parallel links joining one logical endpoint | Not a failover arrow |
| P010 Loop and Storm Protection | `loop`: Closed path with a stop gap | Not restart/reboot |
| P011 Discovery / LLDP | `discovery`: Neighbor pair with short broadcast rays | Not global search |
| P012 Port Security and Snooping | `port-security`: Port socket with a gate bar | Not general security posture |
| P013 Mirroring / SPAN | `mirror`: One ingress line branching to a separate monitor | Not a traffic graph |
| P014 MACsec | `macsec`: Two directly linked ports inside a closed sleeve | Not a routed tunnel |
| P015 Acceleration and SR-IOV | `acceleration`: Packet lanes alongside a small chip | Not generic system settings |
| P016 L2 Resilience / OAM | `resilience`: Ring with a distinct alternate segment | Not WAN failover |
| P017 WAN Connections | `wan`: Port connecting outward across a boundary | Not Internet verified globe |
| P018 Mobile / USB WAN | `modem`: USB stick with short cellular bars | Not Wi-Fi radio |
| P019 Multi-WAN / Failover | `failover`: Primary path plus lower alternate path | Not ECMP distribution; label strategy |
| P020 Path Quality / SD-WAN | `path-quality`: Two paths with measured latency ticks | Not QoS queue order |
| P021 DDNS | `ddns`: Name tag linked to a changing address pair | Not authoritative DNS record |
| P022 Networks / LAN / VLAN | `vlan`: Layered frame with a visible tag tab | Not bridge membership |
| P023 Address Plan / IPAM | `ipam`: Address blocks within a numbered grid | Not route table |
| P024 Clients / Devices | `client`: Endpoint screen with a small identity tag | Not administrator account |
| P025 Wi-Fi | `wifi`: Radio arcs over an access point dot | Never use arcs to imply signal measurement |
| P026 IPv6 Addressing / PD | `ipv6`: Address block with small 6 qualifier | Label remains IPv6, no family inference |
| P027 Routes / RIB / FIB | `route`: Fork with a chosen next-hop marker | Not policy match |
| P028 Policy Routing | `policy-route`: Match diamond before a next-hop fork | Not firewall decision |
| P029 VRF / Route Leaking | `vrf`: Two separated routing compartments | Not an ordinary flat routing table |
| P030 BGP | `bgp`: Two peer nodes with an AS-link connector | Not a generic tunnel |
| P031 OSPF | `ospf`: Area boundary with three adjacent nodes | Not physical topology |
| P032 IS-IS / RIP | `protocol`: Neighbor link with a protocol label space | Variant label carries IS-IS/RIP identity |
| P033 BFD | `bfd`: Peer link with short interval ticks | Not WAN speed |
| P034 Route Filters / Redistribution | `route-filter`: Prefix list entering a term gate | Not packet firewall |
| P035 RPKI / BMP / FlowSpec | `rpki`: Prefix block with a verification seal | Not TLS certificate |
| P036 MPLS / Segment Routing | `mpls`: Packet path carrying stacked label tabs | Not VLAN tag alone |
| P037 VXLAN / EVPN | `overlay`: Inner path over separated underlay anchors | Not physical link |
| P038 Multicast / Discovery Forwarding | `multicast`: One source branching to multiple receivers | Not load-balancing multiple exits |
| P039 IPv6 Transition / CGNAT | `transition`: Two address-family blocks through converter | Not NAT rule direction alone |
| P040 Gateway Redundancy / VRRP | `redundancy`: Shared virtual address over paired gateways | Not multiple WAN providers |
| P041 Access Policies | `access`: Source/service/destination blocks through gate | Not identity role administration |
| P042 Firewall Rules | `firewall`: Packet boundary with deliberate opening | Not generic shield for every page |
| P043 NAT / Published Services | `nat`: Original tuple transformed to a second tuple | Not routing fork |
| P044 Firewall Objects / Feeds | `set`: Stacked named address/service tokens | Not configuration template |
| P045 Connections / Conntrack | `connection`: Paired forward/reply lines with session dot | Not interface cable |
| P046 Firewall Analysis / Protection | `protection`: Boundary plus monitored control gate | Not a claim of antivirus protection |
| P047 WireGuard Servers | `wireguard`: Sleeved link marked WG in nearby text | Use label, not copied vendor logo |
| P048 WireGuard Peers / Exports | `peer`: Single remote endpoint at end of sleeved link | Not whole VPN service |
| P049 Xray Servers | `xray`: Proxy listener with routed outbound branch | Not WireGuard label substitution |
| P050 Xray Users / Profiles | `xray-profile`: Profile card linked to proxy listener | Not account identity or subscription |
| P051 Subscriptions | `subscription`: Profile stack linked to a delivery ticket | Never encode QR or token in icon |
| P052 Other Tunnels / NetBird | `tunnel`: Encapsulated path between endpoints | Plain tunnels need separate encryption status |
| P053 DNS Resolver / Forwarding | `dns`: Name glyph resolving to address block | Not DHCP lease allocation |
| P054 DNS Zones / Records | `dns-record`: Single named record line with value marker | Not certificate |
| P055 Policy DNS / RPZ | `dns-policy`: Name lookup entering a policy gate | Not IP route filter |
| P056 DHCP Scopes / Relay | `dhcp`: Address block allocated to an endpoint | Not name resolution |
| P057 Leases / Reservations | `lease`: Endpoint/address pair with time marker | Not authenticated session |
| P058 Services / Listeners | `service`: Listener socket attached to process block | Not global system gear |
| P059 Sites / Reverse Proxy | `site`: Hostname tab over an upstream link | Not Internet health |
| P060 Certificates / ACME | `certificate`: Document with public verification seal | Not private key |
| P061 Internal PKI / SSH CA | `pki`: Authority seal branching to certificates | Not a single certificate |
| P062 AAA / RADIUS / 802.1X | `aaa`: Identity passing a decision gate | Not local user list |
| P063 Time / NTP / NTS / PTP | `time`: Clock face with source anchor | Not history timeline |
| P064 Telemetry / Integrations | `integration`: Outbound connector with bounded destination | Not mandatory cloud |
| P065 UPnP / NAT-PMP / PCP / Wake | `discovery-service`: Local request opening a leased mapping | Label UPnP/Wake separately |
| P066 Portal | `portal`: Bounded entry door with identity marker | Not cloud login |
| P067 QoS / Traffic Scheduling | `qos`: Several queues ordered through scheduler | Not SD-WAN path selection |
| P068 Users / Roles | `user`: Person silhouette with administration tag | Not client device |
| P069 Sessions / Temporary Access | `session`: Identity badge with expiry marker | Not network conntrack |
| P070 API Tokens / Service Accounts / SSH Keys | `key`: Key shaft with distinct teeth | Not certificate seal |
| P071 Vault | `vault`: Closed compartment with key slot | Never imply secret revealed |
| P072 Updates | `update`: Version stack with forward activation arrow | Not backup download |
| P073 Backup / Restore | `backup`: Archive tray with recoverable copy | Not update activation |
| P074 Hardware Replacement / Port Mapping | `mapping`: Old/new port columns with explicit links | Not automatic hardware replacement |
| P075 Hardware / Storage / Power | `hardware`: Chassis/board with identifiable port edge | Not System abstract workspace |
| P076 Security Posture / Integrity | `security`: Control checklist with verified/unknown marks | Not firewall packet gate |
| P077 Maintenance / Scheduler | `schedule`: Calendar cell with bounded duration | Not raw wall-clock time |
| P078 Recovery / Reboot / Reset | `recovery`: Lifeline returning to a known revision | Not an always-safe undo |
| P079 Preferences / Local-first | `preferences`: Three aligned labeled sliders | Reserve generic controls for preferences |
| P080 First Run / Local Registration | `setup`: Ordered steps entering a local node | Not a marketing onboarding rocket |
| P081 Login / Locked Session | `login`: Local entry door with key slot | Not remote tunnel |
| P082 Incidents | `incident`: Grouped fault cluster with one root marker | Not an ordinary event |
| P083 Logs / Live Tail | `logs`: Stacked text lines with source gutter | Not analytics chart |
| P084 Events / Network Timeline | `event`: Single point on a timeline | Not root incident alarm |
| P085 Audit | `audit`: Immutable entry ledger with actor marker | Not raw diagnostic log |
| P086 Explain Why / What-if | `why`: Connected reasoning steps with question marker | Not an AI chat sparkle |
| P087 Topology | `topology`: Typed nodes and differentiated links | Not a generic networking product logo |
| P088 Network Tools / Capture | `capture`: Packet bracket entering a bounded sample tray | Not traffic line graph |
| P089 Monitoring / History / Traffic | `traffic`: Measured line with axis ticks | No implied live signal when missing |
| P090 Duck Speed Lab | `speed`: Measured path with sample ticks | Not a decorative speedometer score |
| P091 Duck Doctor / Support | `doctor`: Diagnostic checklist linked to evidence | Not medical promise |
| P092 Retention / Storage Pressure | `storage`: Bounded container with capacity line | Not unbounded archive |
| P093 Lab / Acceptance / Fault Evidence | `lab`: Isolated node box with test marker | Not production launch button |
| P094 Easy Home | `overview`: Instrument panel with two factual readouts | Avoid chart as generic success |
| P095 Easy Internet | `wan`: Port connecting outward across a boundary | Not Internet verified globe |
| P096 Easy Networks / Wi-Fi | `vlan`: Layered frame with a visible tag tab | Not bridge membership |
| P097 Easy Devices / Access | `client`: Endpoint screen with a small identity tag | Not administrator account |
| P098 Easy Publish | `nat`: Original tuple transformed to a second tuple | Not routing fork |
| P099 Easy Care / Recovery | `recovery`: Lifeline returning to a known revision | Not an always-safe undo |

## Responsive strategy

| Width / context | Behavior |
|---|---|
| ≥1920 px | Full-width dense workspace; table + inspector + optional evidence comparison; no max-width card island |
| 1280–1919 px | Persistent main navigation; section navigation; table; optional inspector that can become detail route |
| 768–1279 px | Main navigation may collapse to labeled drawer; section index retained/explicit chooser; prioritised columns plus horizontal scroll; touch-sized controls when pointer coarse |
| 360–767 px | Emergency-first layout; persistent Safe Apply; detail routes instead of squeezed inspector; health/WAN/tunnels/incidents/logs/tools/update recovery accessible; tables scroll inside labeled regions |
| 200–400% zoom | Reflow controls and prose, preserve 2-D data table as labeled scroll region; no inaccessible clipped buttons |
| Height <700 px | Remove mountains, shrink nonessential chrome, retain timer and form actions without covering focus |

Mobile is not forced to configure a huge BGP policy with tiny controls, but Full detail/editor routes still exist; say “Wide table; scroll or use object details,” not “use Linux console.” Emergency actions have full explanations and normal authorization. No mobile-only immediate reboot.

## Accessibility contract

Target WCAG 2.2 AA; verify rather than claim certification. All important information available without color, hover, animation or canvas. Text contrast ≥4.5:1 (large text ≥3:1), interactive graphical boundaries ≥3:1. Focus ring 2 px with 2 px offset, distinguishable on both themes and not hidden by sticky bars.

Use semantic tables for read-only lists. An interactive grid is allowed only with a fully implemented keyboard model: one Tab entry, arrows within, Home/End, selection shortcuts with clear scope, Escape to leave/edit-cancel. Provide native/paginated table alternative for virtualized long lists. Do not add grid roles merely for styling.

Every form has real labels, required state, field-linked errors and a summary linked to first error. Dialog focus is trapped only when actually modal; return focus to origin. Inspectors are not focus traps. Browser-native opening in new tab works for objects.

Global shortcuts: Cmd/Ctrl+K palette; / focuses page search only outside text fields and when shortcuts enabled; ? opens shortcut help outside editors; Escape closes the top transient layer without discarding submitted changes. No single-key destructive actions or keybinding that hijacks typing passwords. Provide visible alternative controls and a shortcuts-disable preference.

Live status uses polite announcements only on meaningful changes; do not read every counter. Announce critical rollback/recovery once, then leave persistent text. Countdown announcements at milestones, not every second. Charts have a data table/summary with units and gaps. Topology has node/edge table parity.

Reduced-motion and forced-colors get working surfaces, not missing affordances. Minimum pointer target 24×24 px where the AA spacing criterion permits; choose 44×44 for touch and critical mobile actions. Keyboard-only, screen-reader and 200/400% zoom tests are part of acceptance.

## Data visualization

Every chart defines question, metric, unit, source, timestamp, window, aggregation, missing-data treatment and whether live or historic. Throughput/latency/jitter/loss/queue drops/CPU/RAM/temperature/session count are allowed only when supplied by evidence. Default line charts have labeled axes, no smoothed false precision, no truncated rate axis that exaggerates changes.

Use small sparkline beside an object only if history exists. Hover values are also keyboard reachable. Charts do not trigger auto speed tests. Multiple WAN comparison uses aligned time ranges and units. Disconnected sample gaps stay gaps; unknown confidence is text, not a convincing line.

## Visual regression gates

Golden scenarios: 0/1/100/1000 objects; long German/Russian labels; IPv6; long alias; error/unknown/partial; empty filter; many selected rows; failed sink; QR; login; single-NIC; critical Safe Apply; uncertain clock. Capture Dark/Light at 390×844, 768×1024, 1440×900 and 2560×1440 plus reduced motion and 200% zoom.

Use test-only deterministic fixtures and frozen timestamps in screenshots, never production fake data. Re-run with actual browser fonts and all four locales. Compare layout structure, text clipping, focus, table priorities, icon discernibility and contrast, not only pixel difference. No screenshot may contain real secrets.
