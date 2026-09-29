# Workflow walkthroughs and validation

## Results and counting method

These are **analytical walkthrough results of the design**, not executed UI tests, recruited-user observations or measurements of the unfinished backend. All 20 requested A–T tasks were walked through against the proposed route/component model. Most real execution remains blocked by the backend gaps shown below. No “passed usability test” claim is made.

Counts are **activation-step estimates**, not total physical mouse clicks: each named navigation/action/choice in the sequence counts once. Typing, focusing fields, file chooser internals and value-selection repetitions are excluded; they must be included in later observed physical-click measurements. Opening a menu and choosing a value may add actual clicks. This honest convention permits comparison of architectural friction without pretending an interface exists.

Context switches count moves into a different logical object workspace, excluding steps within one intent wizard. Page transitions count meaningful list/detail/editor/review/result workspace changes, excluding in-form selectors and modals. They are design estimates and must be instrumented in the implemented router. Count of **required hidden-knowledge dependencies in the designed flow is zero by design intent**, not a proven user result: inline explanations expose every listed prerequisite, and unavailable backend behavior blocks rather than being invented.

| Task | Designed activation sequence | Steps | Context switches | Page transitions | Friction removed / remaining critical point | Runtime gates |
|---|---|---:|---:|---:|---|---|
| A Create DHCP WAN + laptop MAC clone | WAN → Connections → Create WAN → DHCP → Clone verified local client → Review → Apply → Confirm | 8 | 0 | 3 | Unverifiable session MAC becomes explicit selection/manual fallback, not guessed MAC. | G01/G11 |
| B Create VLAN30 + DHCP | LAN / VLAN → Networks → Create network → Enable DHCP → Review → Apply → Confirm | 7 | 0 | 3 | One editor includes ports, tagging, subnet, DHCP and DNS; no separate DHCP trip. | G01/G02/G22 |
| C VLAN30 Internet only, block LAN | LAN / VLAN → Networks → VLAN30 → Access → Internet-only preset → Review → Apply → Confirm | 8 | 0 | 4 | Review explicitly separates router/DNS/DHCP exceptions, IPv6 and peer access. | G01/G02 |
| D WireGuard user and QR | Tunnels → WireGuard → Server → Add peer → Review → Apply → Confirm → Show QR | 8 | 0 | 4 | Peer row owns access and lifecycle; reveal requires separate authorization. | G01/G12/G13 |
| E Xray user/subscription | Tunnels → Xray → Server → Add user/profile → Review → Apply → Create subscription → Review → Apply → Show QR | 10 | 1 | 5 | Contextual subscription creation preselects profile; token never appears in ordinary list. | G02/G12/G13 |
| F Publish internal HTTPS | Services → Sites → Local service → Publish → Review exposure → Apply → Confirm → External test | 8 | 0 | 4 | NAT/allow/DNS/cert dependencies previewed together; external test is a distinct step. | G01/G02/G19 |
| G Dual-WAN failover | WAN → Failover → Create exit group → Choose primary → Choose backup → Review → Apply → Confirm | 8 | 0 | 3 | Health defaults and session impact visible; route/rule tables generated and linked. | G01/G11 |
| H Why a client uses WAN2 | LAN / VLAN → Clients → Client → Why? → Run explanation → Open deciding policy | 6 | 1 | 4 | Source/path already filled; unknown FIB state does not yield a confident answer. | G09/G10/G11/G19 |
| I BGP peer + accepted/rejected routes | Routing → BGP → Create peer → Review → Apply → Confirm → Peer routes → Rejected filter | 8 | 0 | 4 | No hidden default export; peer state and route acceptance shown independently. | G01/G10 |
| J Remote management VLAN change | Interfaces → Management port → Edit VLAN mapping → Review management impact → Apply → Reconnect verified node → Confirm | 7 | 0 | 4 | Countdown persists; reconnect is not confirmation; lack of evidence blocks apply. | G01/G23 |
| K Import VyOS | System → Import → Upload file → Analyze → Resolve mappings → Review supported subset → Stage candidate | 7 | 0 | 3 | Unrecognized lines remain visible; classification is not successful conversion. | G03/G17 |
| L Restore on new NIC names | System → Backups → Upload backup → Inspect → Map ports → Review → Apply → Verify connectivity → Confirm | 9 | 1 | 5 | No automatic eth0 assumption; physical Identify action available if supported. | G01/G17 |
| M Internet slow incident | Health → Incident → Inspect affected WAN → Run scoped quality test → Compare DNS/path/queue evidence | 5 | 1 | 3 | Avoid forty independent alarms and an automatic bandwidth-consuming speed test. | G11/G19/G20 |
| N Certificate renewal failure | Health → Incident → Certificate → Challenge evidence → Test prerequisite → Retry renewal | 6 | 1 | 3 | Time, DNS, reachability and issuer limits distinguished; no blind retry storm. | G02/G19/G22 |
| O Upgrade then rollback | System → Updates → Inspect package → Preflight → Activate → Reconnect → Verify receipt → Previous slot → Review rollback → Activate rollback | 10 | 0 | 5 | Download, stage, active and verified distinct; schema rollback compatibility required. | G16/G23 |
| P Find device by MAC/IP | Open palette → Enter IP/MAC and select result → Client details | 3 | 0 | 1 | Normalized exact identifier match before fuzzy aliases; source/last-seen visible. | G06/G15 |
| Q Why traffic blocked | Open palette → Explain path → Select source/destination → Run explanation → Open blocking rule | 5 | 1 | 3 | Packet context distinguishes router vs transit and new vs existing flow. | G01/G19 |
| R Full disk, preserve routing | Health → Storage incident → Retention evidence → Preview eligible cleanup → Confirm cleanup | 5 | 1 | 3 | Never offers delete OS files/log everything; shows protected data and bounded fallback. | G14/G19 |
| S Single-NIC VPS setup | Setup → Local Owner → Server profile → Keep management → Service intent → Review → Apply if supported → Verify → Choose mode | 9 | 0 | 4 | No missing-LAN or mandatory Wi-Fi; public listener scope explicit. | G01/G18 |
| T Beginner basic network | Setup → Language/appearance → Owner → Router profile → Ports → WAN → LAN/DHCP/access → Tests → Review → Apply → Confirm → Mode | 12 | 0 | 4 | Required concepts explained inline; advanced defaults visible in review, not hidden chores. | G01/G18/G22 |

## Specific refinements from walkthroughs

- A: clone-current cannot assume a laptop MAC from an HTTP session. The fallback remains within WAN creation and explicitly explains the missing evidence.
- B/C: network editor includes DHCP/DNS/ports/access; a source/destination policy editor stays available without a second rule system. Router/DNS/DHCP exceptions are reviewed, not silently broad permissions.
- D/E: create identity/profile and reveal QR are separate security actions. Changing access or keys regenerates artifacts and identifies clients that must import again.
- F: publication presents original tuple, NAT, permission and return path together. Certificate issuance and public reachability have separate evidence.
- H/I/Q: Explain and route policy stay linked to stable rule IDs. Missing telemetry yields Unknown, not a fictional packet path.
- J/L: changes to management and port remapping retain global transaction context. Restore does not blindly reuse device names.
- M/R: one root incident with child evidence, not independent alert cards; only bounded diagnostics and safe eligible cleanup.
- N/O: trusted time, signature and compatibility block unsafe claims; downloading is not activation, renewal job creation is not certificate issuance.
- S/T: server profile and offline readiness are legitimate endpoints; no unnecessary LAN/radio wizard steps.

## Confusion-risk register

| Feature | Likely confusion / why | UX prevention | Context help | Explain Why support |
|---|---|---|---|---|
| NAT | Translation mistaken for permission | Match/action/translation/effect blocks and generated allow link | NAT is address change, not authorization | Original/reply tuple and actual nft stage |
| Firewall direction | Vendor In/Out names interpreted from wrong side | To router / Through router / From router plus technical hook | Packet origin vs destination example | Trace interface, hook and deciding rule |
| VRF | Leaking route mistaken for granting access | Independent routing and authorization summaries | Tables isolated, leaks explicit | Selected VRF/table and separate firewall result |
| VLAN tagged/untagged/native | Port PVID confused with egress tag set | Port membership matrix labels ingress classification vs egress tagging | Example VLAN30 tagged uplink, access client | Show observed tag/PVID and network membership |
| BGP filters | Accepted in config assumed advertised/installed | Received, accepted, advertised, RIB and FIB tabs | Default export risk and explicit policies | Matched filter term and selected/rejected reason |
| Route preference | Metric compared across protocols without context | Preference/source/table alongside metric | Explain protocol-specific meaning | Backend-provided selection and recursion |
| CGNAT | Public site expected from local DNAT | Evidence-qualified CGNAT warning in publishing | Provider upstream address sharing | Failed external stage distinguished from local firewall |
| Hairpin NAT | Local success assumed external success | Separate local/public-name and external tests | Split DNS alternative and return path | Pre/post translation path for each vantage |
| Split DNS | Different answers assumed resolver failure | Show client view/upstream/answer provenance | Intentional views and loop risk | Query chain and policy ID |
| IPv6 PD | IPv4 works so IPv6 presumed safe | Family-specific health/access | Lifetimes, withdrawals, LAN allocations | Prefix and RA source; v6 policy route |
| MTU/PMTU | Link-up hides large-packet failure | Auto with manual override and scoped probe | Encapsulation and TCP MSS distinction | Packet size/path evidence, ICMP unknowns |
| Tunnel MTU | Same MTU assumed for underlay and tunnel | Display configured/effective with overhead source | Auto estimate vs measured PMTU | Test source/size and tunnel/underlay stages |
| ACME | Retry expected to solve every failure | Challenge/time/credential/rate-limit separate | HTTP/DNS proof requirements | Dependency failure chain and last evidence |
| Certificates | Installed certificate assumed trusted everywhere | SAN/issuer/time/trust/client distinctions | Local CA trust distribution | Service→cert→issuer/time failure |
| Safe Apply | Timer in browser mistaken for rollback authority | Global server-backed status and receipt | Confirm/rollback vs discard draft | Which verification failed; actual rollback outcome |
| Offload | Rule counters missing assumed no traffic | Effective acceleration and bypass coverage | Hardware forwarding differs from software visibility | Unknown/unobserved stage explicitly marked |
| Generated objects | Auto-created rule looks mysterious or safely editable | Visible Managed by and detach impact | Owner's intent and overwrite behavior | Owner→generated rule→packet result |
| BGP Active | Word Active mistaken for established session | Native FSM state plus explanatory text | Active is a connection-attempt state | Adjacency/timer evidence |
| Intentional disable | Disabled treated as outage | Neutral intent icon and no unrelated global degradation | Disabled vs failed | Change actor/time and affected dependencies |
| Source clock | Recent-looking wall time trusted despite bad clock | Sequence/relative duration/time-trust labels | TIME_UNTRUSTED effects | Correlation confidence and missing order |
| Secret QR | QR treated as harmless image | Explicit warning, no public file/cache, reveal authorization | Anyone with payload gets access | Audit action metadata, never payload |
| Import convertible | Parsed line assumed full working import | Recognition/conversion/readiness distinct | Unsupported settings stay visible | Proposed objects and unresolved effects |
| Policy route | Preferred exit assumed access allowed | Routing and authorization displayed separately | Advanced separation is explicit | Both deciding rule/table and firewall |
| Storage cleanup | Any large directory considered safe to delete | Protected sets and eligible artifact list | Bounded retention before full disk | Sink/quota/drop evidence and routing unaffected status |

## Implemented-UI test plan

### Contract and state tests

For each page, test valid current schema, wrong version, malformed response, null values, unsupported/unknown enum, empty configuration, filter-empty, partial source failure, stale cache, authorization denial, permission revocation, revision conflict, credential-required and hardware-pending. No fixture should accidentally compile into production routes.

Use real mounted API contract tests for method/auth/content type/body bounds and fail-closed behavior. Add explicit regression tests for currently unreachable declared POST handlers before enabling their controls. Show CLI/API tests must compare actual command metadata and mounted operation schemas, not invented expected strings.

### Safety and security

- Apply exactly once; lost acknowledgement triggers status reconciliation; duplicate tabs/reload/sleep retain same transaction; no auto confirmation.
- Expired/untrusted-time/unknown outcome leaves persistent warnings; zero browser countdown is not rollback proof.
- Before/after edits preserve unknown advanced fields, object IDs and base revision. Conflicts never overwrite silently.
- RBAC on every action, explicit secret export permission, no list-secret access and no unauthorized result leakage from search.
- Decode QR independently and compare bytes to downloaded .conf/URL; rotation invalidates prior export state. Standard mobile WireGuard scan is a separate device test.
- No QR/private key/token in logs, audit payloads, URL, ordinary cache, analytics, support bundles or visual-test artifacts.
- Temporary trace restores prior state on success/failure/cancel/crash through backend lease; unavailable cleanup contract blocks activation.
- Full external log sink does not grow internal/OS logs beyond hard bound; UI reports drops.
- Factory reset/management changes show exact scope and real recovery prerequisites, not cosmetic confirmation.
- Mode/locale/theme change makes zero configuration mutations.

### Usability protocol to execute later

Recruit at least six experienced network administrators across routing/firewall backgrounds and six beginners with ordinary home-network familiarity; include keyboard/screen-reader users and at least two native reviewers per non-English locale for high-risk text. This is a proposed study, not completed recruitment.

Counterbalance task order; use isolated realistic scenarios and no production secrets. Record task success, error/recovery, physical clicks, keyboard activations, page transitions, context switches, time to first correct diagnosis, help use and confidence. Ask participants to explain NAT vs permission, route vs authorization and observed vs configured state. Do not equate speed with safe understanding.

Acceptance targets: professionals locate an unfamiliar named capability through navigation/search without outside documentation; root incident identification around 30 seconds in a seeded scenario; all participants notice management risk before apply; no false confirmation of unknown result; no dangerous action triggered accidentally. Numerical time/click targets become evidence only after observation, and severe safety errors fail regardless of speed.

### Scale, accessibility and visual checks

Use 1k interfaces/clients in test-only presentation datasets, 10k firewall rules and 100k+ route-result backing fixtures through paginated test providers, while preserving actual backend caps in production. Confirm bounded DOM/memory, stable selection and no full-data fetch. Those fixture sizes do not raise supported DuckOS configuration limits.

Keyboard-only and screen-reader table/detail/secret-dialog flows; focus after Back and modal close; 200/400% zoom; target sizing; no color-only states; reduced-motion; all locale key/parameter/plural checks. Visual snapshots listed in DESIGN_SYSTEM cover both themes and small/large widths.

## Completion interpretation

These documents complete the design deliverable, not production UI, backend integration or empirical usability qualification. The implementation prompt must retain this distinction. No public release, Linux configuration, network mutation or external credential use follows from this design task.

## Specification verification (2026-09-11)

These checks validate the design artifacts, not an implemented UI:

- Structured catalog: 99 unique page IDs and 99 unique primary routes.
- Expanded contracts: every page has dimensions 1–44 exactly once (4,356 dimension assignments).
- All relative Markdown file links in the specification resolve; P001–P099 page anchors resolve.
- Every catalog page has a defined icon recommendation. Anycast and ZTP also have named, searchable stable tabs, with explicit unimplemented security/runtime boundaries.
- All 11 Markdown documents passed the structural link check; the JSON catalog parsed successfully.
- Token contrast, computed with WCAG relative luminance over all three specified opaque surfaces: minimum text ratio 6.14:1 Dark / 5.35:1 Light; minimum interactive border ratio 3.28:1 Dark / 3.45:1 Light. Actual rendered focus, charts, overlays and disabled states still need implementation-time visual/accessibility testing.
- Documentation whitespace checks passed. No production UI, backend, Linux target or system configuration was modified by this UX work.

The A–T scenarios remain analytical walkthroughs, not measured browser sessions or participant studies. No claim of production readiness, runtime integration, full translation completion or WCAG conformance follows from these document checks.
