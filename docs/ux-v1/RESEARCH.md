# Research and design decisions

## Evidence and limitations

The evidence combines official operational documentation, first-person administrator discussions, public product screenshots embedded in documentation, and the actual repository. Documentation establishes mechanisms; forum posts establish reported friction, not prevalence or verified defects. Older examples are labeled and used as durable failure patterns, not claims that current versions remain defective. No live vendor management session, representative survey or recruited usability test was performed.

Sources were accessed 2026-09-11. Exact product versions matter: Junos commit behavior is not a specification for DuckOS; pf/PF rules are not nftables; cloud topology restrictions are not a reason to require a controller in a local-first product. Search results that could not be opened are explicitly marked snippet-only. No vendor appearance or icon asset is copied.

## Competitor findings

### MikroTik RouterOS / WinBox

The official WinBox documentation describes stacked network-aware filters and configurable route columns. These support an expert's repeated task: isolate a prefix or routing table without rebuilding context. A recent first-person discussion asks how to sort devices by comments/names; its response exposes a distinction between comment headers and sortable columns. Another long WinBox 4 discussion explicitly values low row padding. These are qualitative signals for configurable density, separate alias/comment fields, and per-table view preferences—not evidence that every administrator wants minimum spacing.[1][2][3]

DuckOS keeps stable object names, copyable addresses, saved filters and a 32 px default Full row. A 28 px dense option remains optional; 44 px controls are used for touch. Do not reproduce a desktop multi-window UI inside a browser; use URL-addressable objects, nonmodal inspectors and ordinary browser tabs. Safe Mode's general recovery value is learned through workflow research, but DuckOS uses its own durable transaction and watchdog model.

### VyOS

The rolling CLI documentation distinguishes candidate changes, explicit confirmation, comparison and configurable rollback behavior. The important UI lesson is not to expose a tree of set/delete commands, but to make pending state and the recovery action unambiguous. A November 2025 forum report describes uncertainty about a pending confirmation after a failed commit and an ensuing reboot. That is one operator report, not a proven universal current defect.[4][5]

DuckOS displays the authoritative transaction and recovery state globally even after a validation error, navigation, reconnect or failed acknowledgement. It never recommends blindly confirming an unknown state. Repeated Apply is not a retry strategy. The proposal prefers status reconciliation and a single request ID over optimistic resubmission.

### Juniper Junos

Junos commit-confirmed formalizes temporary configuration activation with automatic rollback if confirmation does not arrive. Public community discussions about extending/canceling confirmation show why timer ownership and the exact effect of each action need explicit language.[6][7]

DuckOS adopts a persistent, inspectable candidate and clearly separates validation, application, verification, awaiting confirmation and final receipt. The design does not inherit Junos timeout defaults or commands that may implicitly confirm. Extend appears only when DuckOS exposes that supported operation. A browser countdown is a display of backend time, never the authority that protects management access.

### UniFi Network

Current zone-based documentation offers a source/destination matrix and explicitly distinguishes gateway-directed traffic from traffic passing through the gateway. It also documents built-in and generated policy behavior and shows navigation paths that differ between versions. Older first-person reports value seeing source/destination and enabling/disabling rules directly; some describe difficulty finding moved firewall functions. Those historical complaints are not treated as descriptions of current Network 9 behavior.[8][9][10]

DuckOS uses an optional access matrix as an entry point, not the only rule manager. “To router” remains distinct from “Through router.” Normal tables show generated objects and owner links by default. The ten permanent areas do not migrate because of a visual refresh. The Easy access wizard and Full rule table use the same objects.

### OPNsense

The official rules documentation exposes multiple processing groups, ordering and automatic rules; its inspection view can reveal system rules and statistics. It describes migration between old/new rule implementations. A current forum discussion asks about the distinction between an interface filter on the list and an interface in a rule; this is a concrete naming/context hazard.[11][12]

DuckOS makes “Filter this view” visually different from “Match packets arriving on.” Creation may prefill from the list filter, but the form states that prefill and shows its semantic effect before saving. Generated rules are not hidden behind an inspection toggle. Table sort and evaluation order are separate, and reordering is disabled while an incompatible sort obscures the true sequence.

### pfSense

The official NAT documentation explains processing order and associated filtering. Its troubleshooting guide distinguishes an internal test from an external test of a port forward; return routing and upstream filtering also matter. Split DNS and NAT reflection are different solutions to local access via a public name.[13][14][15]

DuckOS's publishing review displays original tuple, translation, matching allow policy, source restrictions, target return path, DNS/certificate dependencies and the optional hairpin decision. A green NAT row cannot stand for “reachable from Internet.” The external test explicitly names its vantage point. No pf-specific ordering is copied into DuckOS; the Engine supplies actual nftables-stage evidence.

### FortiGate / FortiOS

The documented Interface Pair and Sequence views serve different inspection needs. Policy lookup identifies the policy that matches a supplied traffic tuple, subject to routing context. Fortinet also documents a version-specific GUI loading/fast-scroll issue in large policy lists, and a display issue in which policies are missing from a particular view. These reports motivate bounded retrieval and visible completeness, not a claim that all current FortiGate tables are unreliable.[16][17][18]

DuckOS provides evaluation-order view plus filtered/grouped presentations over the same stable IDs. “Showing 100 of 12,450; snapshot …” is explicit. Policy lookup opens the actual matching object and preserves the original search. A long-running policy page preserves selection and scroll position while counters update.

### Cisco Meraki

Official topology documentation identifies assumptions and limitations of discovered links, including LLDP/CDP interactions, third-party distinctions, aging and large-network limits. The switch-port documentation puts errors, LLDP and packet tools near a port. This supports evidence-driven object inspection rather than decorative topology.[19][20]

A May 2026 discussion of a navigation refresh includes a vendor explanation of accessibility/performance intent. This is a useful counterweight to “never change anything”: preserve object locations and deep links while improving implementation and accessibility.[21] DuckOS therefore stabilizes IA, not every pixel. No controller or cloud login is introduced.

### Aruba Central

The reviewed on-premises 2.5.4 topology documentation provides device/link detail panes, VLAN overlays, search and orientation controls, but also specifies discovery prerequisites and display limitations. Those are version-specific, not a survey of all Aruba offerings.[22]

DuckOS retains details in full-screen topology, allows left-to-right or top-down layouts, and exposes unknown/unlinked devices rather than silently dropping them. A physical link requires relevant evidence; a shared subnet draws a logical relationship, not a fabricated cable.

## Cross-product decision register

All rows below are DuckOS design judgments, not empirical proof of fastest performance.

| Decision | Evidence/reasoning | Alternative and rejection | Acceptance |
|---|---|---|---|
| D01 Stable ten-area Full IA | Repeated moved-setting friction; expert spatial memory [9][10][21] | Task-only sidebar hides ordinary advanced objects | Every feature has a stable route and search alias |
| D02 Dense tables + optional inspector | WinBox filters/density and policy-view needs [1][2][3][16] | Card wall loses cross-row comparison | Core match/action columns visible at 1440 px |
| D03 Intent editor creates visible owned objects | Generated rules and NAT association complexity [8][11][13] | Invisible automation makes troubleshooting unverifiable | Preview and normal tables show identical IDs/owners |
| D04 Explicit router vs transit context | Firewall-direction documentation [8][11] | “LAN In” alone assumes vendor-specific knowledge | Beginner can distinguish protecting router from blocking LAN access |
| D05 Separate rule order from view sort | Policy sequence and filter friction [11][12][16] | Dragging a filtered list ambiguously changes execution | Move before/after uses stable IDs and candidate diff |
| D06 Persistent transaction strip | Junos recovery pattern and VyOS uncertainty [4][5][6] | Toast-only apply status disappears at critical moment | Reload/reconnect retrieves same transaction and deadline |
| D07 Layered WAN status | Port-forward prerequisites and real repository models [14] | A single red Internet label loses the failure layer | Link/address/gateway/DNS/Internet independently explained |
| D08 Topology evidence and freshness | Meraki/Aruba discovery constraints [19][22] | Treat inferred adjacency as verified physical link | Each edge shows source, age and relationship kind |
| D09 Contextual Test/Why/Logs | Port tools and policy lookup [16][20] | Disconnected tools page requires remembering identifiers | Source context carried into tool request and result |
| D10 Persistent selected rows during refresh | Large policy view problems [17][18] | Resort on every counter update causes action mistakes | No selected row moves until explicit refresh/re-sort |
| D11 Side inspector, route-based editor, small confirmation modal | W3C focus guidance [23] plus context-preservation requirement | Nested large forms in modal stacks | Browser Back restores table and keyboard focus |
| D12 Readiness separate from health | Repository capability/chaos enums disagree in dimensions | Unsupported unused hardware makes whole system degraded | Explicit intent + support + observation states |
| D13 Keyboard and touch without flattening desktop | W3C keyboard and target guidance [24][25] | Mobile-first cards replace expert tables everywhere | Emergency operations work at 390 px; desktop stays dense |
| D14 Semantic icons + text | Carbon icon sizing/consistency; Meraki object shape distinctions [19][26] | Generic shield/globe on every page | Blind-label identification trial and small-size snapshots |
| D15 Canonical protocol terms, localized prose | Cisco RU/DE/ES operational terminology [27][28][29] | Literal translations of every networking token | Protocol strings/API values unchanged across locales |
| D16 Mountains outside reading surfaces | Current brand requirement; contrast/accessibility constraints [25] | Reserved wallpaper consumes 15% even on short screens | No overlay or obscured rows; atmosphere off at low height |
| D17 No fake timing or risk scores | Local transaction/probe contracts; real failure uncertainty | Animated progress to 99%, “risk 73” without evidence | Only real phases, evidence and qualitative risk reasons |
| D18 Graph plus accessible relationship table | Map uncertainty + semantic table guidance [19][22][30] | Canvas-only topology excludes keyboard/screen reader use | Every node/edge accessible as a named row |
| D19 Typed backend gaps, not simulated services | Public dispatch differs from component APIs in source | Treat existence of Go method as reachable GUI action | API reachability tests required before enabling controls |
| D20 Local-first defaults | Existing product architecture and offline acceptance scenarios | External analytics/fonts/help or cloud-only onboarding | Offline browser loads assets/help and manual package flow |

## Conflicting signals and remaining validation

Density must not compromise target size, focus visibility or scanability. Default 32 px rows are a compromise; users choose denser rows, and touch uses 44 px targets. Large screens benefit from simultaneous table/detail views; small screens use a detail route without reducing field coverage.

Some operators prefer more intuitive zones; others need exact hooks, counters and order. DuckOS supplies both as representations of one model, not two configuration systems. Expert editing is not forced through wizards. Beginner flows explain implications without deleting unfamiliar fields.

Forum participants are self-selected. No frequency percentages, vendor rankings or measured time savings are inferred. The biggest untested assumptions are the ten-area second-level discoverability, bilingual technical terminology, understanding of separate NAT/authorization, and source-confidence interpretation in topology. The validation plan specifies professional and beginner studies before claiming success.

## Sources

1. MikroTik, [WinBox](https://help.mikrotik.com/docs/spaces/ROS/pages/328129/WinBox), versioned legacy documentation with current-manual redirect notice; filters and configurable columns.
2. MikroTik community, [Winbox4 — sort by comment/name?](https://forum.mikrotik.com/t/winbox4-sort-by-comment-name-cant-find-my-routers/272751), September 2026 discussion; first-person discoverability report.
3. MikroTik community, [WinBox 4 discussion, page 90](https://forum.mikrotik.com/t/winbox-4-is-here/178358?page=90), 2025-era discussion; search-accessible excerpt on row padding, not a measured preference study.
4. VyOS, [Command Line Interface, rolling](https://docs.vyos.io/en/rolling/cli.html), living documentation.
5. VyOS forum, [How do I cancel a commit-confirm if the commit fails?](https://forum.vyos.io/t/how-do-i-cancel-a-commit-confirm-if-the-commit-fails/17151), 22–25 November 2025.
6. Juniper, [commit command](https://www.juniper.net/documentation/us/en/software/junos/cli-reference/topics/ref/command/commit.html), living CLI reference.
7. Juniper community, [commit confirmed](https://community.juniper.net/discussion/commit-confirmed), older discussion; question-pattern evidence only.
8. Ubiquiti, [Zone-Based Firewalls in UniFi](https://help.ui.com/hc/en-us/articles/115003173168-Zone-Based-Firewalls-in-UniFi), Network 9 version-qualified documentation.
9. Reddit r/Ubiquiti, [Legacy Dashboard Firewall Rules](https://www.reddit.com/r/Ubiquiti/comments/18y3kl4), January 2024; search excerpt only, full retrieval unavailable.
10. Reddit r/Ubiquiti, [Cannot find firewall rules](https://www.reddit.com/r/Ubiquiti/comments/re4a6u), December 2021; historical first-person navigation complaint.
11. OPNsense, [Rules](https://docs.opnsense.org/manual/firewall.html), living documentation, including current/new rule implementations.
12. OPNsense forum, [Firewall Rules [new] — Interface filtering and purpose](https://forum.opnsense.org/index.php?topic=51666.0), 2026 discussion.
13. Netgate, [Ordering of NAT and Firewall Processing](https://docs.netgate.com/pfsense/en/latest/nat/process-order.html), living pfSense documentation.
14. Netgate, [Troubleshooting NAT Port Forwards](https://docs.netgate.com/pfsense/en/latest/troubleshooting/nat-port-forwards.html), living documentation.
15. Netgate, [Accessing Port Forwards from Local Networks](https://docs.netgate.com/pfsense/en/latest/recipes/port-forwards-from-local-networks.html), living documentation.
16. Fortinet, [Policy views and policy lookup](https://docs.fortinet.com/document/fortigate/7.0.12/administration-guide/497952/policy-views-and-policy-lookup), FortiOS 7.0.12 reference, not universal current behavior.
17. Fortinet community technical tip, [Policy GUI loading state, FortiOS 7.6.7](https://community.fortinet.com/fortigate-3/troubleshooting-tip-the-firewall-policy-gui-page-gets-stuck-in-loading-state-on-fortios-v7-6-7-228532), 2026, search excerpt.
18. Fortinet community technical tip, [GUI not displaying all configured policies in interface pair view](https://community.fortinet.com/fortigate-3/troubleshooting-tip-gui-not-displaying-all-configured-policies-in-firewall-policy-interface-pair-view-209026), 2025, search excerpt.
19. Cisco Meraki, [Network Topology](https://documentation.meraki.com/Switching/MS_-_Switches/Operate_and_Maintain/Monitoring_and_Reporting/Network_Topology), living documentation.
20. Cisco Meraki, [Switch Port View](https://documentation.meraki.com/Switching/MS_-_Switches/Operate_and_Maintain/Monitoring_and_Reporting/Switch_Port_View), living documentation.
21. Cisco community, [New look and feel for Meraki Dashboard](https://community.cisco.com/t5/cloud-networking-platform/new-look-and-feel-for-meraki-dashboard/m-p/5549726), May 2026 discussion, search excerpt.
22. Aruba, [Monitoring Sites in the Topology Tab](https://help.centralon-prem.arubanetworks.com/2.5.4/documentation/online_help/content/nms-on-prem/topology/mon_topology.htm), Central on-premises 2.5.4.
23. W3C WAI, [Dialog Modal Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/).
24. W3C WAI, [Developing a Keyboard Interface](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/).
25. W3C WAI, [What's New in WCAG 2.2](https://www.w3.org/WAI/standards-guidelines/wcag/new-in-22/).
26. IBM Carbon, [Icons usage](https://carbondesignsystem.com/elements/icons/usage/). Design guidance, not a required DuckOS dependency.
27. Cisco, [Russian administration guide](https://www.cisco.com/c/dam/en/us/td/docs/voice_ip_comm/cuipph/8800-series/3pcc/localizations/p881_b_8800-mpp-ag-11_ru_RU.pdf), firmware 11.0-era terminology sample, not current protocol guidance.
28. Cisco, [Konfigurieren von VLAN-übergreifendem Routing](https://www.cisco.com/c/de_de/support/docs/lan-switching/inter-vlan-routing/41260-189.html), updated 23 January 2024; translation disclosure acknowledged.
29. Cisco, [Configuración de interfaces VXLAN](https://www.cisco.com/c/es_mx/support/docs/security/secure-firewall-management-center/221043-configure-vxlan-interfaces-on-secure-ftd.html), Spanish operational terminology sample.
30. W3C WAI, [Tables Tutorial](https://www.w3.org/WAI/tutorials/tables/).

Terminology samples are insufficient to establish a single universal RU/DE/ES industry glossary; a native-speaker network-admin review is required. Some forum sites blocked retrieval, so no conclusion depends exclusively on inaccessible Netgate/Airheads content.
