# Interaction and state contracts

## 1. One object model, several views

An object is identified by stable kind + ID + revision, never by translated name, array position or a mutable interface label. A list, inspector, Easy wizard, topology node and CLI/API view reference the same object. Changes are operations against a base revision, not replacement of fields that the current form cannot represent. Unknown advanced fields are preserved; unsupported mutations are refused.

Each page in PAGES has a typed list/detail/editor contract. Common object inspector sections are Summary, Configuration, Runtime, Dependencies, History and Help. Only semantically applicable sections appear, but ordinary important settings are never hidden under “More.” Persistent secondary navigation provides the full route directory.

## 2. Table behavior

Default rows are 32 px, dense 28 px, comfortable/touch 44 px. Name/alias and operational state stay visible. Full tables use available width; no arbitrary 1200 px max-width. Optional columns live in a labeled Columns control, not an unlabeled ellipsis. Configurable widths, multi-sort, saved filters and view reset are per user and per table.

Semantic stable ordering matters. A firewall's evaluation-order view sorts by actual hook/priority/sequence; clicking Bytes creates a clearly labeled analysis view and disables drag reorder. Move before/after still requires a chosen stable target and preview. Sequence numbers are not object identifiers.

Search supports prefix containment, IPv4/IPv6 exact/prefix, MAC normalization, name, tags and protocol. Parsing invalid network syntax returns a local validation message, not a misleading empty result. Structured filters have field/operator/value chips. View filters never silently become rule semantics. Any create-from-filter prefill is highlighted in the editor.

Multi-select means currently selected IDs in a known snapshot. “Select all matching” requires a backend snapshot/query-selection contract and shows exact scope; it cannot be faked by selecting the loaded page. Bulk editing shows eligibility, prohibited rows, proposed field changes, dependencies and aggregate risk. No all-or-nothing claim unless backend provides atomicity. Secret exports are never bulk-prefetched. Row actions and a visible selection toolbar duplicate context-menu functionality.

Live updates change cells without moving selected rows, clearing selection or resetting scroll. New/deleted rows appear as “12 changes available”; Apply refresh is explicit. Deleted selected objects stay as tombstone rows until acknowledged. Counters show source/age. Editing a row freezes its base revision but not the evidence age. Stale-revision conflicts require comparison, never last-writer-wins.

Pagination default 100, choices 50/100/250 within endpoint limits. For large RIB/FIB, request filtered cursor windows and virtualize only loaded rows; expose “loaded / total / total unknown,” cursor, snapshot and effective limits. Keep a paginated semantic-table mode for assistive technology. Never download a full Internet routing table to implement client-side search. The current canonical configuration caps are not bypassed by a larger GUI.

## 3. Navigation and editing

List row name is a normal link. Enter opens the object. Cmd/Ctrl-click opens a browser tab. Secondary details use a nonmodal inspector at desktop widths; complex editing gets a full route. The inspector has a shareable route and returns focus to the row. At most one modal: secret reveal, narrowly scoped confirmation, or short irreversible-action gate. Do not put tabs full of configuration inside nested modal stacks.

Field groups: Intent; Identity/scope; Dependencies; Advanced; Review. Advanced is an ordinary labeled section, not a hidden alternate product. Forms accept technical precision in Full: IP family, table/VRF, sequence, authentication reference, timer and units. Easy exposes purpose-oriented defaults with a readable generated-object summary.

Changing an object selection while dirty prompts Keep editing / Discard draft / Stage draft if valid. Route change never implies apply. Mode/theme/locale switches preserve drafts and do not mutate networking. Browser reload may discard unsaved secret entry; that is preferable to persisting it. Warn accurately.

## 4. Desired, observed, readiness and freshness

These are independent axes:

- Desired: enabled, disabled, absent, candidate change.
- Support: supported, experimental, hardware-pending, credential-pending, unsupported, unknown.
- Observation: working, partial, failed, unknown; native protocol state remains visible.
- Freshness: fresh, stale, disconnected, never observed.
- Change: clean, staged, applying, awaiting confirmation, rollback, recovery required.
- Time confidence: trusted, untrusted, unknown.

Existing capability and chaos enums need an adapter; a single “degraded” field cannot answer all these questions. Preserve raw code in expert details but translate its explanation. A disabled WAN is neutral with a pause/slash symbol, not a red alert. An unused missing PTP clock does not degrade global network health.

Global header: a small status symbol plus short accessible text (“2 issues” or “Evidence unavailable”), not a giant DEGRADED heading. Click opens incident/evidence summary. Color never carries the only meaning. A healthy label requires actual coverage; insufficient evidence cannot become green through a default branch.

## 5. Candidate / Safe Apply

Stage does not change runtime. The global strip is always present when a candidate or transaction exists:

`4 changed objects · base revision … · management affected · Review changes`

When active:

`Awaiting confirmation · 01:42 remaining (server estimate) · Confirm · Roll back now · Details`

It appears on every authenticated route and in mobile emergency view. It is outside scrollable content. Do not allow sticky bars to cover focused controls.

Review includes actor, base revision, all generated objects/owners, before/after, dependency changes, affected WAN/LAN/tunnels/clients, management path, validation findings and session impact. Counts must indicate known/unknown completeness; “47 known clients” is not all clients. Risk is Low/Medium/High/Critical with concrete reasons, not a score.

The ideal presentation states VALIDATING → APPLYING → VERIFYING → WAITING FOR CONFIRMATION → COMMITTED or ROLLING BACK → ROLLED BACK. These are presentation categories, not fabricated server phases. The current state model lacks all dedicated progress events; show “Applying; verification detail unavailable” instead of inventing a transition. Recovery-required has a separate persistent critical panel.

Submit once with expected revision and request ID. Lost response is “Outcome not yet known; checking transaction …”. Never automatically retry a dangerous write. Poll/reconnect the authoritative status and correlate ID/revision before offering another submission. Refresh, tab duplication, sleeping laptop and session expiry do not reset the timer. Backend watchdog owns rollback independently of UI.

Clock: use server remaining duration/monotonic anchor when available; otherwise show the deadline with uncertainty and re-query. At displayed zero, say “Deadline reached; verifying outcome.” Do not mark rolled back until a receipt arrives. Time-untrusted wall time cannot authorize expiry.

Confirm is explicit and bound to transaction ID/revision, authorization and current verification state. Roll back now is available when backend admits it. Extend is absent until a specific supported endpoint exists; no repeated apply to simulate extension. If login expires, keep a non-secret reconnect notice and restore status after authentication; no bypass of RBAC to make confirmation easier.

Conflicts: show Base / Current / My changes, affected paths and lock owner/expiry if authorized. Refresh and rebase are intentional; reconfirm validation and blast radius. No silent lock stealing. Revision/hash/receipt identifiers are copyable but never translated.

## 6. Risk and irreversible actions

Read-only: ordinary navigation, filter and inspection. Low: save display preferences. Medium: restart optional monitoring or revoke a selected lease. High: restart tunnel, kill a selected connection set, change a protection policy, rotate client credentials. Critical: management-path mutation, broad WAN exposure, factory reset, destructive storage or signing/trust changes. Actual dependency context may elevate risk.

For Low, no generic confirmation. Medium has a scoped inline consequence. High has an impact review with named targets, counts, session consequences and recovery path. Critical requires preview, recovery prerequisites and explicit confirmation; type node/object name for irreversible reset/decommission, not for every routine change. Reauthentication is a backend capability requirement; do not emulate it with a decorative password field.

Not all actions are reversible. Key revocation may require new client profiles; captured traffic sent to an external sink cannot be recalled; reset erases selected state; physical power operations may lose access. Label the actual recovery mechanism and limitations. A UI Cancel cancels a draft or supported diagnostic job, not a submitted kernel operation unless backend supports cancellation.

## 7. Firewall / NAT model

Form anatomy has four explicit blocks: Match, Action, Translation, Generated effect.

Match includes traffic context (To router / Through router / From router; actual hook in Full), ingress, source, destination and protocol/service. Action includes allow/drop/reject or supported advanced action. Translation shows original/translated tuples and NAT stage. Generated effect shows related allow/NAT/route rules, state behavior, owner and revision.

For a publication, show: “This will open TCP 443 on WAN1 and forward it to 192.168.10.20:443.” The label is parameterized, not hard-coded per locale. Include allowed source scope and whether it reaches router itself or a target service. Establishing a listener never implies WAN publishing.

The advanced rule list includes generated objects by default. Managed fields may be read-only with an Open owner action; Detach transfers ownership with dependency preview, not deletion of a label. Raw nftables is inspectable generated output, not the main editing UI. Do not offer arbitrary shell/config execution in the browser.

Connection tracking matters: existing sessions may use prior decisions. Any flush is a separate high-impact action; changing a firewall rule must not silently kill all connections. A simulation's new-flow result is labeled separately from existing conntrack flow.

## 8. Explain Why and what-if

Entry from client, WAN, rule, route, DNS, tunnel, incident or global search. Carry object IDs, interface/VRF and candidate revision into the question. Form: source client/IP/network; destination IP/hostname; protocol TCP/UDP/ICMP; optional port; traffic direction/context; observed or candidate revision.

Hostname simulation must not claim to be traffic-free if resolution performs a live query. Default to existing answer evidence with age; explicit Resolve now is a diagnostic operation. The backend must say which address was evaluated and cannot assume all rotating DNS answers share a path.

Show a plain-language conclusion (“The candidate selects WAN2 because …”; “Unknown: current FIB evidence missing”) and ordered evidence rows:

Identity → DNS/category where applicable → access intent → firewall → policy rule/table → route/RIB/FIB → tunnel/WAN → NAT.

Actual packet processing may order DNAT before route/filter and SNAT after route/filter. The displayed technical order must come from the backend trace, not this conceptual list. Distinguish logical explanation from execution trace. The first unresolved stage ends a definite result; downstream assumptions remain conditional.

Each step links to the responsible object/rule, revision, evidence source/time, decision and missing prerequisites. Human prose is backed by structured reason codes/parameters. Raw sanitized native output is an optional expert detail. Never translate unstructured logs into invented causal certainty.

Before/after compares identical question and explicitly named revisions/snapshots. Unknown old evidence stays unknown. “Why selected?” for BGP includes local preference/AS path/metric information only if the backend supplies it; a generic accepted flag cannot reconstruct the full best-path algorithm.

Probe now is visually separate and names traffic, source and budget. Simulation alone never confirms a Safe Apply.

## 9. WAN evidence and incident triage

WAN layers: physical carrier; L2/encapsulation; address assignment; gateway; DNS; Internet reachability; quality. Every result states target, interface/VRF, family, sample age and reason. No “Internet down” from one failed external ICMP target; blocked ICMP and DNS failure have different evidence. No automatic cloud reachability dependency.

MAC workflow: hardware / configured / effective shown simultaneously. “Clone this administrator device” requires a directly observed local client MAC bound to the session's network context. HTTP over a routed/VPN/proxy path does not reveal the remote laptop's MAC. If unverifiable, offer an observed/recent device selector or validated manual input; never invent one from IP. Multicast/broadcast/invalid overrides rejected before apply; show current sessions affected.

Incident detail starts with affected function, evidence-backed layer, what remains working, last relevant change, and the best bounded next diagnostic. Correlated children are grouped but searchable. Timeline order uses sequence and time trust; nearby events imply correlation, not proof of causation. The 30-second target is an acceptance goal, not achieved research evidence.

## 10. Topology

Three explicit lenses: Physical, Logical, Traffic path. Separate configured link, LLDP observation, FDB learning, ARP/ND presence, import and inference. Show source/time/confidence in the inspector and a compact edge style/legend. An ARP entry does not prove a direct cable. Shared prefixes do not prove L2 physical adjacency.

Default layout left-to-right on wide screens, focused ego-network on narrow screens; top-down toggle persists. Preserve pinned positions, selected object, viewport and filters during updates. New relationships queue a refresh instead of continuously moving nodes. Large graphs retrieve/filter neighborhoods rather than rendering all possible nodes.

Every graph has a semantic relationship table with equivalent navigation/actions. Full-screen does not remove details. Layered networks/VRFs get labels, not an unexplained cloud. Unknown nodes use neutral placeholders with their evidence, not a guessed device brand. Future fleet IDs may namespace node IDs, but no V2 fleet UI is built.

## 11. Secrets and reusable exports

Use one export contract: kind, object ID, revision, display metadata, raw payload, URL when applicable, downloadable bytes/filename/MIME and QR-compatible payload. Secret fields are obtained only after explicit authorized reveal/export, not with lists or hover previews.

WireGuard QR contains exactly the standard client configuration; downloaded .conf and Copy config use those same bytes. Subscription QR contains exactly its generated URL; Xray supported protocol URI/config follows its declared representation. Do not wrap a WireGuard config in a URL or add prose to the QR payload.

Modal: opaque surface; at least 320 px QR on desktop where payload permits, integer module rendering, four-module quiet zone, high-contrast black on white, no mascot/logo overlay. Display object/profile revision and “Anyone who scans this code can use this connection.” Long payload exceeding QR capacity gets an honest download/copy path, not truncation; required common mobile profiles must pass real decode/phone acceptance. Rotation or access change invalidates old export state, refetches authorized revision and shows remote re-apply required.

Keep payload in isolated memory only; clear component state on close/logout/rotation, revoke Blob URLs and cancel in-flight requests. JavaScript cannot guarantee physical memory erasure: do not claim it. No local/session storage, URL query, telemetry, screenshot automation against real secrets, public assets, service-worker cache or ordinary query cache. Clipboard transfer requires explicit user action and warning; do not silently overwrite system clipboard later. Audit viewing/exporting by object ID/revision/action, never secret bytes.

No raw credentials in Show CLI/API, ordinary diff, errors, breadcrumbs, search index, logs or support bundle. Display a secure-input placeholder/reference only, not a command containing a real token/password.

## 12. Diagnostics and bounded logging

Context carries source interface, VRF, client and object; the result repeats them. Diagnostic request names traffic/output, duration, rate/packet/byte limits and destination. Tests have exact cancellation/expiry behavior. Live capture is sensitive; no implicit capture from hovering an interface.

Temporary Trace requests identify exactly which subsystems/objects change level and for how long. The Agent must restore prior levels after completion/error/cancel/crash; UI cannot guarantee cleanup through a finally block. Where backend lacks this contract, disable automated trace rather than leaving verbose logging enabled.

Default optional logging has a tiny hard-bounded internal fallback (recommended product policy: at most 500 records AND 512 KiB, plus age limit; lower platform cap wins). It is not an implementation claim or permission to change protected audit retention. External destination failure/full disk raises an alert and drops into that bounded fallback only. Show dropped/suppressed counts. Never write indefinitely to OS root, RAM or browser history.

## 13. Loading, empty, partial and error contracts

Loading: preserve prior useful data with “Updating; last observed …”. Initial load uses static skeleton rows without fictitious counts. After timeout show retry/cancel controls; no infinite spinner. Abort obsolete reads. Backoff on capacity/429/503 without retrying writes.

Empty: distinguish no configured objects, no matching filter, no observed objects, collection unavailable, unauthorized and not supported. A missing collector cannot be presented as “0 clients.” Filter-empty offers Clear filters; never creates a dummy row.

Partial: per-section source failure and last-good timestamp; usable sections remain. Degraded: name the failed layer and impact, link evidence and test. Error: What / Where / Why known / What remains working / Next safe action / Rollback status / request ID. Sanitize native output. If rollback unknown, say unknown.

Read-only: list/detail still useful, controls explain required permission without disclosing hidden object data. Revoked permissions clear secrets and cached authorization-scoped data. Avoid route guards as the only access control.

Experimental: labeled alongside support evidence and limitations, requires explicit consent for consequential use. Hardware-pending names missing capability; credential-pending opens secure configuration; unsupported explains the reason and supported alternatives. Unknown states fail safe and remain visible.

## 14. Resource and browser behavior

Use bounded requests, cancellation, visibility-aware polling, exponential backoff and lazy route modules. One active transaction subscription/poller is shared globally. One live tail per view; bounded ring and pause preserve selection. No continuous re-layout from counters. No external fonts, analytics, CDN scripts or help dependencies.

Measurements have units, source, sampling, aggregation, window and gaps. Throughput uses delta counter / monotonic interval; reset creates gap, not a spike. Distinguish bits/s and bytes, GiB and GB, decimal speed labels and binary memory. Historical charts never extrapolate missing history. Default 15 min overview, 1 h object view, explicit 24 h if retained. Refresh chart data no faster than source sampling.

## 15. Equivalent CLI/API

Existing cmdmeta is a registry contract, not a guarantee that templates are registered or endpoints mounted. Render only verified equivalents; escape/validate values, redact credentials, show target revision and whether command stages or applies. If no equivalent exists, show “CLI/API operation not exposed in this build” and the backend gap ID. Never label a raw Linux command as Duck CLI.

API detail presents method, actual mounted route, schema and redacted body. No guessed /api/v1/bgp/... paths. UI routes and REST routes are separate namespaces. Proposed contract examples in documentation stay marked proposed until integrated and tested.

## 16. Acceptance boundaries

Visual completeness does not mean production feature completeness. A page shell with an unavailable typed capability is honest scaffolding, not an implemented subsystem. A feature becomes actionable only with authorization, validation, correct operation reachability, evidence, revision semantics and tests. Frontend implementation must not loosen existing security gates to make the design appear functional.
