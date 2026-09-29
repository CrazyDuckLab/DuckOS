# Page-by-page specification

Baseline and backend gaps: FEATURE_INVENTORY and BACKEND_GAPS. These 99 design pages do not imply 99 working API subsystems. Each contains all 44 requested dimensions; common contracts are deliberately shared rather than copied into 99 inconsistent implementations.

## Shared contracts referenced by page cells

These references are normative, not omitted specification. INTERACTIONS provides full behavior and acceptance. Every page below explicitly assigns every requested dimension.

- **C01 Table:** stable IDs; 32/28/44 px density; columns/resize; typed filters; IP-aware search; saved views; bounded cursor retrieval; preserve selection/scroll on updates. View filter is not packet match. No full-data download beyond limits.
- **C02 Bulk:** explicitly selected IDs/snapshot; backend-supported actions only; eligible/skipped list; aggregate risk and atomicity truth. No secret prefetch or inferred backend operation.
- **C03 Safety:** scope, sessions, dependencies, management path, validation and recovery. High/critical review; target-name confirmation for irreversible reset. No generic confirmation for reads/preferences.
- **C04 Transaction:** stage against base revision; preview generated effects; submit once; reconcile unknown outcome; show backend phase/deadline/receipt globally; no auto-confirm or fake progress; rollback only on supported operation.
- **C05 Evidence:** desired/support/observation/freshness/change/time-trust are independent. Disabled is neutral; stale/unknown never zero/success. Native protocol state remains visible.
- **C06 Relationships:** forward/reverse object IDs, owner, completeness and linked inspector. Missing impact coverage blocks destructive changes. Browser Back restores context.
- **C07 Equivalents:** actual supported Duck CLI/mounted API metadata only; no raw shell or guessed endpoint; redact secrets. Otherwise label Not exposed and link G01/G07.
- **C08 History:** actor, object/revision, sequence/time trust, source, reason, result, transaction and rollback; secrets only action metadata; authorized bounded export.
- **C09 Empty:** distinguish none configured, none matching filter, none observed, inaccessible and unsupported. Clear filters or supported create; no fixture row.
- **C10 Loading:** initial skeleton; retain last-good data with age; cancel obsolete reads, bounded read retry/backoff; timeout notice rather than eternal spinner.
- **C11 Partial/error:** retain good sections; per-source failure; what/where/why known/what remains/next action/rollback known-or-unknown; safe localized summary + diagnostic ID. No native secret leakage.
- **C12 Permissions:** server-authorized read/write/export/action scope; read-only useful detail, disabled-action reason; secret/scoped cache cleared on revocation; no fetch-then-hide.
- **C13 Conflict:** Base/Current/My changes and lock scope; keep non-secret draft; intentional rebase/revalidation; no overwriting unknown advanced fields.
- **C14 Availability:** missing hardware/credential/version capability gets its own reason and next step. Unused unsupported feature is not an outage. Experimental operations need real support evidence and explicit risk consent.
- **C15 Diagnostics:** carry object/interface/VRF into request and result; explicit traffic/privacy/resource/time budget; cancel/expire via backend; trace logging restored server-side.
- **C16 Charts:** only question-driven measured series with units/source/window/sampling/gaps; no decorative or interpolated-success graph; tabular equivalent.
- **C17 Accessibility:** semantic table/forms; complete keyboard; text+symbol status; visible focus; labels/errors; no hover/canvas-only content. See DESIGN_SYSTEM.
- **C18 Responsive:** desktop dense table + optional inspector; narrow screen detail route and labeled horizontal table; touch controls; persistent accessible Safe Apply.
- **C19 Keyboard:** Tab/Shift+Tab, Enter object link, Cmd/Ctrl+K, Escape transient layer, keyboard reorder; no single-key destructive action; don't intercept editor/password typing.
- **C20 Locale:** EN/RU/DE/ES messages/parameters/plurals; canonical technical tokens/IDs/values; 40% expansion; no translated-string logic.
- **C21 Help:** seven answers—What, When, Changes, Risk, Dependencies, Example, Diagnose—using page-specific values below. Inline hint → nonmodal expanded local help → local documentation; fully localization-ready.

Risk classes: **R** read-only (linked operations separately assessed); **L** preference/login presentation; **M** scoped interruption; **H** session/access/evidence impact; **C** potential management/trust/broad-exposure/irreversible impact. Actual context may elevate risk; none of these letters is an automatic authorization.

<a id="p001"></a>

## P001 — System Overview

Source families: model observe doctor. **Editor field groups:** Pinned panels; time window; display units only.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | System Overview |
| 2 | URL / route | `/system/overview`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → System Overview |
| 4 | Purpose | Coherent management/inspection of hostname; uptime; CPU/RAM/storage; interface summaries; sessions; Internet evidence; pending changes. |
| 5 | Administrator questions | What is failing now, what remains working, and is a change pending? |
| 6 | Main objects | hostname; uptime; CPU/RAM/storage; interface summaries; sessions; Internet evidence; pending changes |
| 7 | Table / list columns | Object; evidence; observed age; current value; trend; issue |
| 8 | Filters | component; severity; observed age |
| 9 | Search | C01; component; severity; observed age. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Inspect active incident (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open WAN; interfaces; resource pressure; transaction |
| 15 | Dangerous actions | R; C03. Impact depends on observations; incidents; transaction. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from CPU usage may be null; load is not usage; health endpoint is not Internet. |
| 19 | Runtime health / status | CPU usage may be null; load is not usage; health endpoint is not Internet C05. |
| 20 | Dependencies | observations; incidents; transaction |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open observations; incidents; transaction through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is failing now, what remains working, and is a change pending? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: System Overview. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for CPU usage may be null; load is not usage; health endpoint is not Internet. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Doctor; probe layer details. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: CPU usage is unavailable while uptime is observed: keep uptime visible and label CPU unknown. Diagnose: cell 37. |
| 37 | Diagnostics | Doctor; probe layer details; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Home health summary without invented success; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep hostname protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `overview` — Instrument panel with two factual readouts. Avoid chart as generic success. See DESIGN_SYSTEM status grammar. |

<a id="p002"></a>

## P002 — Candidate and Safe Apply

Source families: safeapply state engine agent canary secpolicy. **Editor field groups:** Reason; expected revision; supported confirmation window; change selection.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Candidate and Safe Apply |
| 2 | URL / route | `/system/changes`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Candidate and Safe Apply |
| 4 | Purpose | Coherent management/inspection of candidate; diff; commit; commit-confirmed; locks; conflicts; canary; blast radius; transaction ordering. |
| 5 | Administrator questions | What will change, can management survive, and what must I confirm? |
| 6 | Main objects | candidate; diff; commit; commit-confirmed; locks; conflicts; canary; blast radius; transaction ordering |
| 7 | Table / list columns | Object; operation; owner; before; after; risk reason; validation |
| 8 | Filters | actor; object type; risk; changed only |
| 9 | Search | C01; actor; object type; risk; changed only. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Review and apply candidate (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Show CLI/API; compare; discard selected draft changes; roll back now |
| 15 | Dangerous actions | C; C03. Impact depends on revision; management evidence; journal; dependencies. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Reason; expected revision; supported confirmation window; change selection. |
| 18 | Desired vs observed | C05; configured fields separate from Show authoritative transaction ID, phase, deadline and reconciliation; no synthesized phases. |
| 19 | Runtime health / status | Show authoritative transaction ID, phase, deadline and reconciliation; no synthesized phases C05. |
| 20 | Dependencies | revision; management evidence; journal; dependencies |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open revision; management evidence; journal; dependencies through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will change, can management survive, and what must I confirm? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Candidate and Safe Apply. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Show authoritative transaction ID, phase, deadline and reconciliation; no synthesized phases. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Before/after path; management probe; validation evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Reason; expected revision; supported confirmation window; change selection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Changing VLAN30 affects the current management path: review that path before temporary apply. Diagnose: cell 37. |
| 37 | Diagnostics | Before/after path; management probe; validation evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Review changes with concrete effects and global timer; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep candidate protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `candidate` — Two offset documents with a pending corner. Not an upload or committed check. See DESIGN_SYSTEM status grammar. |

<a id="p003"></a>

## P003 — Configuration History

Source families: state safeapply incident canary. **Editor field groups:** Before revision; after revision; checkpoint label; restore reason.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Configuration History |
| 2 | URL / route | `/system/history`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Configuration History |
| 4 | Purpose | Coherent management/inspection of revisions; checkpoints; network time machine; compare; rollback. |
| 5 | Administrator questions | Which change preceded the problem and what would restoring change? |
| 6 | Main objects | revisions; checkpoints; network time machine; compare; rollback |
| 7 | Table / list columns | Revision; actor; sequence/time trust; objects; result; checkpoint |
| 8 | Filters | actor; object; result; time; checkpoint |
| 9 | Search | C01; actor; object; result; time; checkpoint. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Compare revisions (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Restore as candidate; label checkpoint; open incident |
| 15 | Dangerous actions | C; C03. Impact depends on canonical revisions; schema; known-good evidence. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Before revision; after revision; checkpoint label; restore reason. |
| 18 | Desired vs observed | C05; configured fields separate from Historical configuration is not current runtime; restore goes through new validation. |
| 19 | Runtime health / status | Historical configuration is not current runtime; restore goes through new validation C05. |
| 20 | Dependencies | canonical revisions; schema; known-good evidence |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open canonical revisions; schema; known-good evidence through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which change preceded the problem and what would restoring change? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Configuration History. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Historical configuration is not current runtime; restore goes through new validation. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Impact comparison; connectivity verification. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Before revision; after revision; checkpoint label; restore reason. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Compare the last known-good revision with the revision preceding WAN loss, then restore as a new candidate. Diagnose: cell 37. |
| 37 | Diagnostics | Impact comparison; connectivity verification; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Recent changes and Restore known-good; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep revisions protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `history` — Revision nodes on a timeline. Not a generic clock/time service. See DESIGN_SYSTEM status grammar. |

<a id="p004"></a>

## P004 — Dependencies and Ownership

Source families: lifecycle intca canary. **Editor field groups:** Alias; tags; comment; ownership transfer reason; clone name.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Dependencies and Ownership |
| 2 | URL / route | `/system/objects`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Dependencies and Ownership |
| 4 | Purpose | Coherent management/inspection of dependency graph; reverse dependencies; provenance; generated ownership; detach; aliases; tags; clone. |
| 5 | Administrator questions | Who manages this object and what depends on it? |
| 6 | Main objects | dependency graph; reverse dependencies; provenance; generated ownership; detach; aliases; tags; clone |
| 7 | Table / list columns | Name; kind; owner; references; used by; drift; revision |
| 8 | Filters | kind; owner; dangling references; tags |
| 9 | Search | C01; kind; owner; dangling references; tags. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect dependency graph (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open owner; detach with preview; duplicate where safe |
| 15 | Dangerous actions | H; C03. Impact depends on canonical object IDs; owner generator; policy graph. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Alias; tags; comment; ownership transfer reason; clone name. |
| 18 | Desired vs observed | C05; configured fields separate from Graph may be incomplete; unknown impact blocks destructive actions. |
| 19 | Runtime health / status | Graph may be incomplete; unknown impact blocks destructive actions C05. |
| 20 | Dependencies | canonical object IDs; owner generator; policy graph |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open canonical object IDs; owner generator; policy graph through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who manages this object and what depends on it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Dependencies and Ownership. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Graph may be incomplete; unknown impact blocks destructive actions. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Dangling references; orphan detection; generated diff. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Alias; tags; comment; ownership transfer reason; clone name. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Published Service web01 owns DNAT and an allow rule; open its owner before changing the generated rule. Diagnose: cell 37. |
| 37 | Diagnostics | Dangling references; orphan detection; generated diff; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Managed-by details on related object; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep dependency graph protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `ownership` — Parent block linked to child block. Not a person or sharing permission. See DESIGN_SYSTEM status grammar. |

<a id="p005"></a>

## P005 — Templates

Source families: intca lifecycle. **Editor field groups:** Template version; named parameters; target scope; owner behavior; ZTP credential ID, scope, expiry and optional node binding.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Templates |
| 2 | URL / route | `/system/templates`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Templates |
| 4 | Purpose | Coherent management/inspection of templates; wizard-generated ordinary objects; parameter validation; ZTP one-time provisioning credentials. |
| 5 | Administrator questions | What will this template create and will later edits be overwritten? |
| 6 | Main objects | templates; wizard-generated ordinary objects; parameter validation; ZTP one-time provisioning credentials |
| 7 | Table / list columns | Template; version; parameters; generated objects; owner |
| 8 | Filters | kind; version; installed |
| 9 | Search | C01; kind; version; installed. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Preview template (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | View generated objects; clone; compare versions; Provisioning tab; issue scoped enrollment credential |
| 15 | Dangerous actions | H; C03. Impact depends on generator; schema; references. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Template version; named parameters; target scope; owner behavior; ZTP credential ID, scope, expiry and optional node binding. |
| 18 | Desired vs observed | C05; configured fields separate from Template existence does not prove successful apply; ZTP stored consumed/expiry flags are not proof of enrollment or enforcement. |
| 19 | Runtime health / status | Template existence does not prove successful apply; ZTP stored consumed/expiry flags are not proof of enrollment or enforcement C05. |
| 20 | Dependencies | generator; schema; references |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open generator; schema; references through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will this template create and will later edits be overwritten? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Templates. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Template existence does not prove successful apply; ZTP stored consumed/expiry flags are not proof of enrollment or enforcement. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Validate parameters; dependency diff. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Template version; named parameters; target scope; owner behavior; ZTP credential ID, scope, expiry and optional node binding. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Apply a guest-network template and inspect the actual VLAN, DHCP and access objects it proposes. Diagnose: cell 37. |
| 37 | Diagnostics | Validate parameters; dependency diff; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided presets without hidden configuration; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep templates protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `template` — Reusable stencil with small object slots. Not an imported file. See DESIGN_SYSTEM status grammar. |

<a id="p006"></a>

## P006 — Import and Migration

Source families: migrate backup. **Editor field groups:** Source format; upload; mapping; selected sections; acknowledgement of omissions.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Import and Migration |
| 2 | URL / route | `/system/import`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Import and Migration |
| 4 | Purpose | Coherent management/inspection of VyOS; OpenWrt UCI; MikroTik; Kerio; generic Linux import; recognized/convertible/attention/unsupported. |
| 5 | Administrator questions | What was understood, what can convert, and what still needs a decision? |
| 6 | Main objects | VyOS; OpenWrt UCI; MikroTik; Kerio; generic Linux import; recognized/convertible/attention/unsupported |
| 7 | Table / list columns | Source line; object; classification; reason; proposed mapping |
| 8 | Filters | format; classification; object kind; unresolved |
| 9 | Search | C01; format; classification; object kind; unresolved. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Analyze file (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect source safely; map objects; stage understood subset |
| 15 | Dangerous actions | H; C03. Impact depends on parser; source schema; candidate; interface mapping. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Source format; upload; mapping; selected sections; acknowledgement of omissions. |
| 18 | Desired vs observed | C05; configured fields separate from Current parser classifies lines; convertible must not imply complete executable conversion. |
| 19 | Runtime health / status | Current parser classifies lines; convertible must not imply complete executable conversion C05. |
| 20 | Dependencies | parser; source schema; candidate; interface mapping |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open parser; source schema; candidate; interface mapping through related object links. |
| 23 | Explain Why integration | Seed this question/context: What was understood, what can convert, and what still needs a decision? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Import and Migration. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Current parser classifies lines; convertible must not imply complete executable conversion. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Validation; source coverage; unresolved fields. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Source format; upload; mapping; selected sections; acknowledgement of omissions. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Import a VyOS file containing understood address lines and unsupported policy terms; preserve both classifications. Diagnose: cell 37. |
| 37 | Diagnostics | Validation; source coverage; unresolved fields; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided import with no automatic apply; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep VyOS protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `import` — Document entering a bounded review tray. Not applied configuration. See DESIGN_SYSTEM status grammar. |

<a id="p007"></a>

## P007 — Interfaces

Source families: network model hardware. **Editor field groups:** Alias; admin state; role; addresses; MTU auto/manual; MAC override.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Interfaces |
| 2 | URL / route | `/interfaces`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Interfaces |
| 4 | Purpose | Coherent management/inspection of physical; virtual; addresses; link; MTU; MAC; SFP; interface identity. |
| 5 | Administrator questions | Which port is this, what owns it, and what runs over it? |
| 6 | Main objects | physical; virtual; addresses; link; MTU; MAC; SFP; interface identity |
| 7 | Table / list columns | Name/alias; type; role; admin; link; speed; MAC; MTU; addresses; owner |
| 8 | Filters | type; role; link; driver; missing binding |
| 9 | Search | C01; type; role; link; driver; missing binding. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect interface (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | +VLAN; assign role; identify port; capture; neighbors; routes |
| 15 | Dangerous actions | C; C03. Impact depends on physical enrollment; logical roles; bridge/VRF; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Alias; admin state; role; addresses; MTU auto/manual; MAC override. |
| 18 | Desired vs observed | C05; configured fields separate from Separate administrative enabled from carrier and address evidence. |
| 19 | Runtime health / status | Separate administrative enabled from carrier and address evidence C05. |
| 20 | Dependencies | physical enrollment; logical roles; bridge/VRF; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open physical enrollment; logical roles; bridge/VRF; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which port is this, what owns it, and what runs over it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Interfaces. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Separate administrative enabled from carrier and address evidence. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Ping from interface; link errors; LLDP; capture. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Alias; admin state; role; addresses; MTU auto/manual; MAC override. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Use port identification, driver and MAC evidence to select the physical adapter before assigning WAN. Diagnose: cell 37. |
| 37 | Diagnostics | Ping from interface; link errors; LLDP; capture; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Ports and role assignment with physical guidance; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep physical protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `interface` — RJ45-style port with distinct contacts. Not a whole switch chassis. See DESIGN_SYSTEM status grammar. |

<a id="p008"></a>

## P008 — Bridges and FDB

Source families: vlan network. **Editor field groups:** Name; members; VLAN filtering; management address; static FDB entries.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Bridges and FDB |
| 2 | URL / route | `/interfaces/bridges`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Bridges and FDB |
| 4 | Purpose | Coherent management/inspection of bridge; FDB; static MAC; port membership; VLAN filtering. |
| 5 | Administrator questions | Which ports share L2 and where was this MAC learned? |
| 6 | Main objects | bridge; FDB; static MAC; port membership; VLAN filtering |
| 7 | Table / list columns | Bridge; ports; filtering; MAC; learned port; age; owner |
| 8 | Filters | bridge; VLAN; dynamic/static; MAC |
| 9 | Search | C01; bridge; VLAN; dynamic/static; MAC. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create bridge (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Edit membership; +VLAN; FDB lookup; topology |
| 15 | Dangerous actions | C; C03. Impact depends on ports; VLAN membership; STP; management address. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; members; VLAN filtering; management address; static FDB entries. |
| 18 | Desired vs observed | C05; configured fields separate from FDB entry is observed learning, not verified client identity. |
| 19 | Runtime health / status | FDB entry is observed learning, not verified client identity C05. |
| 20 | Dependencies | ports; VLAN membership; STP; management address |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open ports; VLAN membership; STP; management address through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which ports share L2 and where was this MAC learned? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Bridges and FDB. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for FDB entry is observed learning, not verified client identity. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: MAC movement; STP; L2-loop diagnosis. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; members; VLAN filtering; management address; static FDB entries. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A NAS MAC learned on a bridge port links to that FDB observation, not an asserted authenticated user. Diagnose: cell 37. |
| 37 | Diagnostics | MAC movement; STP; L2-loop diagnosis; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Network ports as part of LAN workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep bridge protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `bridge` — Two port groups joined by a horizontal span. Not VLAN tagging. See DESIGN_SYSTEM status grammar. |

<a id="p009"></a>

## P009 — Bonding / LACP

Source families: bonding. **Editor field groups:** Name; mode; members; hash policy; minimum links where supported.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Bonding / LACP |
| 2 | URL / route | `/interfaces/bonds`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Bonding / LACP |
| 4 | Purpose | Coherent management/inspection of bonding; LACP; member failures; hash policy. |
| 5 | Administrator questions | Which links participate and what happens if a member fails? |
| 6 | Main objects | bonding; LACP; member failures; hash policy |
| 7 | Table / list columns | Bond; mode; members; active members; hash; partner; last transition |
| 8 | Filters | bond; member state; mode |
| 9 | Search | C01; bond; member state; mode. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create bond (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect partner; remove member; test redundancy |
| 15 | Dangerous actions | C; C03. Impact depends on physical ports; switch-side configuration; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; mode; members; hash policy; minimum links where supported. |
| 18 | Desired vs observed | C05; configured fields separate from Configured members vs negotiated aggregation; do not infer peer agreement. |
| 19 | Runtime health / status | Configured members vs negotiated aggregation; do not infer peer agreement C05. |
| 20 | Dependencies | physical ports; switch-side configuration; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open physical ports; switch-side configuration; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which links participate and what happens if a member fails? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Bonding / LACP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Configured members vs negotiated aggregation; do not infer peer agreement. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: LACP state; counters; staged failure test only in lab. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; mode; members; hash policy; minimum links where supported. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Inspect which LACP member failed without assuming that a configured bond negotiated successfully. Diagnose: cell 37. |
| 37 | Diagnostics | LACP state; counters; staged failure test only in lab; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Link aggregation guided explanation; Full edit; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep bonding protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `bond` — Two parallel links joining one logical endpoint. Not a failover arrow. See DESIGN_SYSTEM status grammar. |

<a id="p010"></a>

## P010 — Loop and Storm Protection

Source families: stp. **Editor field groups:** Mode; priority; edge port; guard options; storm thresholds and units; MST instances if supported.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Loop and Storm Protection |
| 2 | URL / route | `/interfaces/loop-protection`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Loop and Storm Protection |
| 4 | Purpose | Coherent management/inspection of STP; RSTP; MSTP; BPDU/root/loop guards; storm control; MAC flapping. |
| 5 | Administrator questions | Is a blocked port protecting the network or failing? |
| 6 | Main objects | STP; RSTP; MSTP; BPDU/root/loop guards; storm control; MAC flapping |
| 7 | Table / list columns | Bridge; port; mode; role; state; root; guard; storm rate; MAC moves |
| 8 | Filters | bridge; port; guard event; flap interval |
| 9 | Search | C01; bridge; port; guard event; flap interval. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure protection (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Explain blocked port; inspect root; trace MAC movement |
| 15 | Dangerous actions | H; C03. Impact depends on bridge topology; VLAN/MST mapping; supported hardware. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Mode; priority; edge port; guard options; storm thresholds and units; MST instances if supported. |
| 18 | Desired vs observed | C05; configured fields separate from STP blocking is expected; separate guard trigger from link-down. |
| 19 | Runtime health / status | STP blocking is expected; separate guard trigger from link-down C05. |
| 20 | Dependencies | bridge topology; VLAN/MST mapping; supported hardware |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open bridge topology; VLAN/MST mapping; supported hardware through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is a blocked port protecting the network or failing? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Loop and Storm Protection. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for STP blocking is expected; separate guard trigger from link-down. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Root election; flap timeline; broadcast counters. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Mode; priority; edge port; guard options; storm thresholds and units; MST instances if supported. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A redundant port is STP-blocking by design; inspect its role before treating it as a link fault. Diagnose: cell 37. |
| 37 | Diagnostics | Root election; flap timeline; broadcast counters; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Loop warning plus safe recommended action; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep STP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `loop` — Closed path with a stop gap. Not restart/reboot. See DESIGN_SYSTEM status grammar. |

<a id="p011"></a>

## P011 — Discovery / LLDP

Source families: hardware nac topology. **Editor field groups:** Receive/transmit mode; per-port scope; advertised fields; TTL where supported.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Discovery / LLDP |
| 2 | URL / route | `/interfaces/discovery`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Discovery / LLDP |
| 4 | Purpose | Coherent management/inspection of LLDP; LLDP-MED; neighbors; evidence confidence. |
| 5 | Administrator questions | What device is on this port and how reliable is that claim? |
| 6 | Main objects | LLDP; LLDP-MED; neighbors; evidence confidence |
| 7 | Table / list columns | Local port; chassis ID; remote port; system; capabilities; TTL; source |
| 8 | Filters | port; capability; expired; vendor |
| 9 | Search | C01; port; capability; expired; vendor. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect neighbor (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open port; topology; enable receive/transmit with exposure preview |
| 15 | Dangerous actions | M; C03. Impact depends on LLDP probe; interface; management disclosure policy. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Receive/transmit mode; per-port scope; advertised fields; TTL where supported. |
| 18 | Desired vs observed | C05; configured fields separate from LLDP is unauthenticated evidence; expired neighbor is stale not still connected. |
| 19 | Runtime health / status | LLDP is unauthenticated evidence; expired neighbor is stale not still connected C05. |
| 20 | Dependencies | LLDP probe; interface; management disclosure policy |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open LLDP probe; interface; management disclosure policy through related object links. |
| 23 | Explain Why integration | Seed this question/context: What device is on this port and how reliable is that claim? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Discovery / LLDP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for LLDP is unauthenticated evidence; expired neighbor is stale not still connected. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Probe source; topology mismatch. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Receive/transmit mode; per-port scope; advertised fields; TTL where supported. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An LLDP neighbor expired: show last seen and source rather than a current connected cable. Diagnose: cell 37. |
| 37 | Diagnostics | Probe source; topology mismatch; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Connected equipment detail; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep LLDP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `discovery` — Neighbor pair with short broadcast rays. Not global search. See DESIGN_SYSTEM status grammar. |

<a id="p012"></a>

## P012 — Port Security and Snooping

Source families: nac dhcpsnoop. **Editor field groups:** Trust role; MAC limit; violation action; DAI; RA guard; Option 82 handling.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Port Security and Snooping |
| 2 | URL / route | `/interfaces/port-security`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Port Security and Snooping |
| 4 | Purpose | Coherent management/inspection of port security; DHCP snooping; Option 82 trust; DAI; RA Guard; MAC limits. |
| 5 | Administrator questions | Which ports may supply addressing and what was rejected? |
| 6 | Main objects | port security; DHCP snooping; Option 82 trust; DAI; RA Guard; MAC limits |
| 7 | Table / list columns | Port; trust; MAC limit; bindings; guard; violations; action |
| 8 | Filters | trusted/untrusted; violation; VLAN |
| 9 | Search | C01; trusted/untrusted; violation; VLAN. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure port protections (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect binding; rogue RA; quarantine client; logs |
| 15 | Dangerous actions | H; C03. Impact depends on DHCP scope/relay; VLANs; bindings; hardware capabilities. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Trust role; MAC limit; violation action; DAI; RA guard; Option 82 handling. |
| 18 | Desired vs observed | C05; configured fields separate from Learned bindings need source/age; policy configured is not enforced proof. |
| 19 | Runtime health / status | Learned bindings need source/age; policy configured is not enforced proof C05. |
| 20 | Dependencies | DHCP scope/relay; VLANs; bindings; hardware capabilities |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open DHCP scope/relay; VLANs; bindings; hardware capabilities through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which ports may supply addressing and what was rejected? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Port Security and Snooping. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Learned bindings need source/age; policy configured is not enforced proof. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: DAI drops; DHCP spoof test; rogue RA evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Trust role; MAC limit; violation action; DAI; RA guard; Option 82 handling. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Only an uplink toward the DHCP server is trusted; preview effects before changing access-port trust. Diagnose: cell 37. |
| 37 | Diagnostics | DAI drops; DHCP spoof test; rogue RA evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Protect guest/network wizard; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep port security protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `port-security` — Port socket with a gate bar. Not general security posture. See DESIGN_SYSTEM status grammar. |

<a id="p013"></a>

## P013 — Mirroring / SPAN

Source families: nac capture. **Editor field groups:** Source ports; direction; SPAN/ERSPAN; destination; truncation; duration; rate budget.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Mirroring / SPAN |
| 2 | URL / route | `/interfaces/mirroring`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Mirroring / SPAN |
| 4 | Purpose | Coherent management/inspection of SPAN; ERSPAN; capture destinations. |
| 5 | Administrator questions | What traffic is copied, to where, and what is the resource/privacy impact? |
| 6 | Main objects | SPAN; ERSPAN; capture destinations |
| 7 | Table / list columns | Session; sources; direction; destination; encapsulation; rate; expiry |
| 8 | Filters | source; destination; active/expired |
| 9 | Search | C01; source; destination; active/expired. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create mirror session (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Capture; stop; inspect destination |
| 15 | Dangerous actions | H; C03. Impact depends on source interfaces; sink/route; device capability; storage budget. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Source ports; direction; SPAN/ERSPAN; destination; truncation; duration; rate budget. |
| 18 | Desired vs observed | C05; configured fields separate from Mirror active must be evidence-backed; no silent persistent capture. |
| 19 | Runtime health / status | Mirror active must be evidence-backed; no silent persistent capture C05. |
| 20 | Dependencies | source interfaces; sink/route; device capability; storage budget |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open source interfaces; sink/route; device capability; storage budget through related object links. |
| 23 | Explain Why integration | Seed this question/context: What traffic is copied, to where, and what is the resource/privacy impact? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Mirroring / SPAN. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Mirror active must be evidence-backed; no silent persistent capture. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Dropped mirrored packets; destination reachability. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Source ports; direction; SPAN/ERSPAN; destination; truncation; duration; rate budget. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Mirror one interface for 60 seconds to a specified diagnostic sink, then stop automatically. Diagnose: cell 37. |
| 37 | Diagnostics | Dropped mirrored packets; destination reachability; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Contextual diagnostic only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep SPAN protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `mirror` — One ingress line branching to a separate monitor. Not a traffic graph. See DESIGN_SYSTEM status grammar. |

<a id="p014"></a>

## P014 — MACsec

Source families: macsec vault. **Editor field groups:** Link; peer association; key reference; cipher capability; replay policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | MACsec |
| 2 | URL / route | `/interfaces/macsec`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → MACsec |
| 4 | Purpose | Coherent management/inspection of MACsec; secure associations; key lifecycle; replay protection. |
| 5 | Administrator questions | Is this link encrypted and is the peer authenticated? |
| 6 | Main objects | MACsec; secure associations; key lifecycle; replay protection |
| 7 | Table / list columns | Link; association; peer; active key ID; encryption; replay/errors |
| 8 | Filters | link; state; expiry |
| 9 | Search | C01; link; state; expiry. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure secure link (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Rotate key; inspect peer; test; rollback |
| 15 | Dangerous actions | C; C03. Impact depends on capable NIC/kernel; peer; Vault; MTU. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Link; peer association; key reference; cipher capability; replay policy. |
| 18 | Desired vs observed | C05; configured fields separate from SA configured vs secure channel observed; never show key in table. |
| 19 | Runtime health / status | SA configured vs secure channel observed; never show key in table C05. |
| 20 | Dependencies | capable NIC/kernel; peer; Vault; MTU |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open capable NIC/kernel; peer; Vault; MTU through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is this link encrypted and is the peer authenticated? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: MACsec. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for SA configured vs secure channel observed; never show key in table. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Replay counters; association health; MTU. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Link; peer association; key reference; cipher capability; replay policy. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Rotate a MACsec association only after showing the peer dependency and recoverable prior configuration. Diagnose: cell 37. |
| 37 | Diagnostics | Replay counters; association health; MTU; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full-only with understandable link security status; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep MACsec protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `macsec` — Two directly linked ports inside a closed sleeve. Not a routed tunnel. See DESIGN_SYSTEM status grammar. |

<a id="p015"></a>

## P015 — Acceleration and SR-IOV

Source families: nac hardware chaos. **Editor field groups:** Per-feature desired enablement; VF count and mapping; CPU affinity; auto tune opt-in.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Acceleration and SR-IOV |
| 2 | URL / route | `/interfaces/acceleration`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → Acceleration and SR-IOV |
| 4 | Purpose | Coherent management/inspection of hardware offload; DSA; switchdev; devlink; SR-IOV; XDP/eBPF; IRQ/RSS/RPS/XPS; NUMA. |
| 5 | Administrator questions | What bypasses software policy and which observability is lost? |
| 6 | Main objects | hardware offload; DSA; switchdev; devlink; SR-IOV; XDP/eBPF; IRQ/RSS/RPS/XPS; NUMA |
| 7 | Table / list columns | Interface; driver; capability; requested; effective; VF; bypass warning |
| 8 | Filters | interface; backend; unsupported; mismatch |
| 9 | Search | C01; interface; backend; unsupported; mismatch. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect acceleration (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Preview change; view firewall coverage; VF mapping; performance |
| 15 | Dangerous actions | C; C03. Impact depends on driver; firmware; IOMMU; policy; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Per-feature desired enablement; VF count and mapping; CPU affinity; auto tune opt-in. |
| 18 | Desired vs observed | C05; configured fields separate from No offload success without observed evidence; hardware pending is not network degraded. |
| 19 | Runtime health / status | No offload success without observed evidence; hardware pending is not network degraded C05. |
| 20 | Dependencies | driver; firmware; IOMMU; policy; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open driver; firmware; IOMMU; policy; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: What bypasses software policy and which observability is lost? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Acceleration and SR-IOV. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No offload success without observed evidence; hardware pending is not network degraded. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Feature probe; dropped packets; CPU/IRQ; bypass check. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Per-feature desired enablement; VF count and mapping; CPU affinity; auto tune opt-in. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Enabling offload may remove a packet from software counters; show the effective enforcement coverage. Diagnose: cell 37. |
| 37 | Diagnostics | Feature probe; dropped packets; CPU/IRQ; bypass check; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Performance explanation only; advanced editing in Full; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep hardware offload protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `acceleration` — Packet lanes alongside a small chip. Not generic system settings. See DESIGN_SYSTEM status grammar. |

<a id="p016"></a>

## P016 — L2 Resilience / OAM

Source families: mclag oam. **Editor field groups:** Domain; peers; role; peer link; CFM level/MA/MEP; ring protocol supported fields.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | L2 Resilience / OAM |
| 2 | URL / route | `/interfaces/resilience`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Interfaces → L2 Resilience / OAM |
| 4 | Purpose | Coherent management/inspection of MC-LAG; peer link; CFM; Ethernet OAM; ERPS; MRP. |
| 5 | Administrator questions | Is the redundant L2 path coordinated and where is continuity lost? |
| 6 | Main objects | MC-LAG; peer link; CFM; Ethernet OAM; ERPS; MRP |
| 7 | Table / list columns | Domain; peer; role; peer-link; ring; maintenance association; continuity |
| 8 | Filters | protocol; domain; fault; role |
| 9 | Search | C01; protocol; domain; fault; role. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create resilience domain (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect peer/ring; continuity test; dependencies |
| 15 | Dangerous actions | C; C03. Impact depends on peer switch; VLANs; dedicated peer link; capability. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Domain; peers; role; peer link; CFM level/MA/MEP; ring protocol supported fields. |
| 18 | Desired vs observed | C05; configured fields separate from Model state is not measured continuity; absence of OAM evidence explicit. |
| 19 | Runtime health / status | Model state is not measured continuity; absence of OAM evidence explicit C05. |
| 20 | Dependencies | peer switch; VLANs; dedicated peer link; capability |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open peer switch; VLANs; dedicated peer link; capability through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is the redundant L2 path coordinated and where is continuity lost? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: L2 Resilience / OAM. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Model state is not measured continuity; absence of OAM evidence explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: CFM; peer-link health; ring transition. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Domain; peers; role; peer link; CFM level/MA/MEP; ring protocol supported fields. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A peer-link failure changes MC-LAG behavior; inspect peer and member impact together. Diagnose: cell 37. |
| 37 | Diagnostics | CFM; peer-link health; ring transition; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Incident evidence only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep MC-LAG protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `resilience` — Ring with a distinct alternate segment. Not WAN failover. See DESIGN_SYSTEM status grammar. |

<a id="p017"></a>

## P017 — WAN Connections

Source families: wan network mtu ddns. **Editor field groups:** Connection type; interface; static/DHCP; PPP credentials; MAC source; DNS; MTU auto/manual; probe/failover intent.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | WAN Connections |
| 2 | URL / route | `/wan/connections`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | WAN → WAN Connections |
| 4 | Purpose | Coherent management/inspection of DHCP; static; PPPoE; L2TP; MAC clone; addressing; provider; CGNAT; MTU/PMTU/MSS. |
| 5 | Administrator questions | At which layer does this uplink fail? |
| 6 | Main objects | DHCP; static; PPPoE; L2TP; MAC clone; addressing; provider; CGNAT; MTU/PMTU/MSS |
| 7 | Table / list columns | WAN; link; addressing; gateway; DNS; Internet; public IP; provider; effective MAC |
| 8 | Filters | mode; enabled; failed layer; CGNAT; interface |
| 9 | Search | C01; mode; enabled; failed layer; CGNAT; interface. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create WAN (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Clone verified local-client MAC; reset hardware MAC; probe; routes |
| 15 | Dangerous actions | C; C03. Impact depends on interface; credentials; DNS; routes; NAT; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Connection type; interface; static/DHCP; PPP credentials; MAC source; DNS; MTU auto/manual; probe/failover intent. |
| 18 | Desired vs observed | C05; configured fields separate from Seven separate layers; probe target/time/source; CGNAT suspected vs evidenced. |
| 19 | Runtime health / status | Seven separate layers; probe target/time/source; CGNAT suspected vs evidenced C05. |
| 20 | Dependencies | interface; credentials; DNS; routes; NAT; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open interface; credentials; DNS; routes; NAT; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: At which layer does this uplink fail? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: WAN Connections. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Seven separate layers; probe target/time/source; CGNAT suspected vs evidenced. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Gateway ping; DNS query; external IP evidence; PMTU. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Connection type; interface; static/DHCP; PPP credentials; MAC source; DNS; MTU auto/manual; probe/failover intent. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for interface; credentials; DNS; routes; NAT; management. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: DHCP succeeded but DNS probes fail: show Addressing working and DNS failing, not Cable disconnected. Diagnose: cell 37. |
| 37 | Diagnostics | Gateway ping; DNS query; external IP evidence; PMTU; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Internet setup workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep DHCP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `wan` — Port connecting outward across a boundary. Not Internet verified globe. See DESIGN_SYSTEM status grammar. |

<a id="p018"></a>

## P018 — Mobile / USB WAN

Source families: hardware capability wan. **Editor field groups:** APN; credential reference; PIN secure input; roaming consent; data cap; role.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Mobile / USB WAN |
| 2 | URL / route | `/wan/mobile`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | WAN → Mobile / USB WAN |
| 4 | Purpose | Coherent management/inspection of modem; SIM; APN; signal; data budget; PIN; backup WAN. |
| 5 | Administrator questions | Can this modem provide a usable backup and at what cost? |
| 6 | Main objects | modem; SIM; APN; signal; data budget; PIN; backup WAN |
| 7 | Table / list columns | Modem; SIM masked; registration; signal; carrier; address; data budget |
| 8 | Filters | device; registration; roaming; capability |
| 9 | Search | C01; device; registration; roaming; capability. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure modem (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test connectivity; set backup; inspect radio; disconnect |
| 15 | Dangerous actions | H; C03. Impact depends on USB/modem driver; SIM; carrier; credentials; route group. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: APN; credential reference; PIN secure input; roaming consent; data cap; role. |
| 18 | Desired vs observed | C05; configured fields separate from USB detected is not registered/data-connected; unsupported modem explained. |
| 19 | Runtime health / status | USB detected is not registered/data-connected; unsupported modem explained C05. |
| 20 | Dependencies | USB/modem driver; SIM; carrier; credentials; route group |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open USB/modem driver; SIM; carrier; credentials; route group through related object links. |
| 23 | Explain Why integration | Seed this question/context: Can this modem provide a usable backup and at what cost? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Mobile / USB WAN. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for USB detected is not registered/data-connected; unsupported modem explained. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Registration reason; signal history; layer probes. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: APN; credential reference; PIN secure input; roaming consent; data cap; role. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for USB/modem driver; SIM; carrier; credentials; route group. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A modem is detected over USB but not registered with the carrier; do not mark backup Internet ready. Diagnose: cell 37. |
| 37 | Diagnostics | Registration reason; signal history; layer probes; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Backup Internet setup when hardware present; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep modem protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `modem` — USB stick with short cellular bars. Not Wi-Fi radio. See DESIGN_SYSTEM status grammar. |

<a id="p019"></a>

## P019 — Multi-WAN / Failover

Source families: multiwan vrrp. **Editor field groups:** Members; priorities; weights; hysteresis; failback; session policy; safety thresholds.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Multi-WAN / Failover |
| 2 | URL / route | `/wan/failover`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | WAN → Multi-WAN / Failover |
| 4 | Purpose | Coherent management/inspection of route groups; ECMP; balancing; preferred exit; session impact; backup activation. |
| 5 | Administrator questions | Why was this WAN chosen and what happens to existing sessions? |
| 6 | Main objects | route groups; ECMP; balancing; preferred exit; session impact; backup activation |
| 7 | Table / list columns | Group; strategy; members; priority/weight; selected; reason; health; sessions |
| 8 | Filters | group; strategy; selected member; failed |
| 9 | Search | C01; group; strategy; selected member; failed. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create exit group (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Simulate failure; inspect member; view generated routes; recovery preference |
| 15 | Dangerous actions | H; C03. Impact depends on WAN; probes; route tables; NAT; conntrack. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Members; priorities; weights; hysteresis; failback; session policy; safety thresholds. |
| 18 | Desired vs observed | C05; configured fields separate from Selection model vs installed route/NAT; failover does not promise session continuity. |
| 19 | Runtime health / status | Selection model vs installed route/NAT; failover does not promise session continuity C05. |
| 20 | Dependencies | WAN; probes; route tables; NAT; conntrack |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open WAN; probes; route tables; NAT; conntrack through related object links. |
| 23 | Explain Why integration | Seed this question/context: Why was this WAN chosen and what happens to existing sessions? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Multi-WAN / Failover. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Selection model vs installed route/NAT; failover does not promise session continuity. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Probe timeline; flow path; conntrack impact. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Members; priorities; weights; hysteresis; failback; session policy; safety thresholds. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: WAN2 is selected after WAN1 probe failure; explain the hold-down and existing-session impact. Diagnose: cell 37. |
| 37 | Diagnostics | Probe timeline; flow path; conntrack impact; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary/backup wizard; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep route groups protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `failover` — Primary path plus lower alternate path. Not ECMP distribution; label strategy. See DESIGN_SYSTEM status grammar. |

<a id="p020"></a>

## P020 — Path Quality / SD-WAN

Source families: ipsla sdwan. **Editor field groups:** Probe type; target; interface/VRF; interval; thresholds; weights; hold-down.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Path Quality / SD-WAN |
| 2 | URL / route | `/wan/path-quality`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | WAN → Path Quality / SD-WAN |
| 4 | Purpose | Coherent management/inspection of IP SLA; latency; jitter; loss; path scoring; brownout; application class; SD-WAN. |
| 5 | Administrator questions | Is a path technically up but unsuitable for this application? |
| 6 | Main objects | IP SLA; latency; jitter; loss; path scoring; brownout; application class; SD-WAN |
| 7 | Table / list columns | Probe; source; target; latency; jitter; loss; score components; selected policy |
| 8 | Filters | source; application; threshold violation; time |
| 9 | Search | C01; source; application; threshold violation; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create health policy (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test from path; compare candidates; explain score |
| 15 | Dangerous actions | H; C03. Impact depends on targets; WAN/VRF; timing; routing intent. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Probe type; target; interface/VRF; interval; thresholds; weights; hold-down. |
| 18 | Desired vs observed | C05; configured fields separate from Show measurement availability, criteria weights and sample window; no opaque risk number. |
| 19 | Runtime health / status | Show measurement availability, criteria weights and sample window; no opaque risk number C05. |
| 20 | Dependencies | targets; WAN/VRF; timing; routing intent |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open targets; WAN/VRF; timing; routing intent through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is a path technically up but unsuitable for this application? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Path Quality / SD-WAN. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Show measurement availability, criteria weights and sample window; no opaque risk number. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Probe evidence; threshold crossings; before/after path. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Probe type; target; interface/VRF; interval; thresholds; weights; hold-down. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A voice policy avoids a low-loss but high-jitter path according to visible measured criteria. Diagnose: cell 37. |
| 37 | Diagnostics | Probe evidence; threshold crossings; before/after path; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Internet quality and backup reason; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep IP SLA protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `path-quality` — Two paths with measured latency ticks. Not QoS queue order. See DESIGN_SYSTEM status grammar. |

<a id="p021"></a>

## P021 — DDNS

Source families: ddns sites. **Editor field groups:** Provider; hostname; zone; credential ref; IP source; family; refresh.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | DDNS |
| 2 | URL / route | `/wan/ddns`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | WAN → DDNS |
| 4 | Purpose | Coherent management/inspection of DDNS providers; external IP changes; CGNAT constraints. |
| 5 | Administrator questions | Does this name point to the reachable address? |
| 6 | Main objects | DDNS providers; external IP changes; CGNAT constraints |
| 7 | Table / list columns | Record; provider; desired name; detected IP; published IP; last update; reason |
| 8 | Filters | provider; stale; family; WAN |
| 9 | Search | C01; provider; stale; family; WAN. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create DDNS record (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Query public DNS; rotate credential; open dependent service |
| 15 | Dangerous actions | H; C03. Impact depends on provider secret; WAN public address; time; resolver. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Provider; hostname; zone; credential ref; IP source; family; refresh. |
| 18 | Desired vs observed | C05; configured fields separate from Successful update does not prove inbound reachability; mask tokens. |
| 19 | Runtime health / status | Successful update does not prove inbound reachability; mask tokens C05. |
| 20 | Dependencies | provider secret; WAN public address; time; resolver |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open provider secret; WAN public address; time; resolver through related object links. |
| 23 | Explain Why integration | Seed this question/context: Does this name point to the reachable address? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: DDNS. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Successful update does not prove inbound reachability; mask tokens. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Public DNS query; provider failure; CGNAT. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Provider; hostname; zone; credential ref; IP source; family; refresh. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for provider secret; WAN public address; time; resolver. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: DDNS updated the address but external HTTPS still fails; inspect CGNAT and publishing separately. Diagnose: cell 37. |
| 37 | Diagnostics | Public DNS query; provider failure; CGNAT; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Publish-service prerequisite; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep DDNS providers protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `ddns` — Name tag linked to a changing address pair. Not authoritative DNS record. See DESIGN_SYSTEM status grammar. |

<a id="p022"></a>

## P022 — Networks / LAN / VLAN

Source families: network vlan. **Editor field groups:** Name; VLAN ID; parent/bridge; outer tag when supported; subnet; ports/tagging; DHCP; DNS; access; management.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Networks / LAN / VLAN |
| 2 | URL / route | `/lan/networks`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | LAN / VLAN → Networks / LAN / VLAN |
| 4 | Purpose | Coherent management/inspection of LAN; VLAN; QinQ; coherent DHCP/DNS/ports/isolation; segmentation. |
| 5 | Administrator questions | Who belongs to this network and what may they reach? |
| 6 | Main objects | LAN; VLAN; QinQ; coherent DHCP/DNS/ports/isolation; segmentation |
| 7 | Table / list columns | Network; subnet; VLAN/S-VLAN; bridge; port membership; DHCP; access; clients |
| 8 | Filters | VLAN; bridge; role; overlapping; family |
| 9 | Search | C01; VLAN; bridge; role; overlapping; family. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create network (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Duplicate; +DHCP; edit access; clients; packet path |
| 15 | Dangerous actions | C; C03. Impact depends on physical ports; bridge; VRF; DHCP; DNS; firewall; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; VLAN ID; parent/bridge; outer tag when supported; subnet; ports/tagging; DHCP; DNS; access; management. |
| 18 | Desired vs observed | C05; configured fields separate from Tag assignment vs observed traffic; QinQ outer/inner distinct. |
| 19 | Runtime health / status | Tag assignment vs observed traffic; QinQ outer/inner distinct C05. |
| 20 | Dependencies | physical ports; bridge; VRF; DHCP; DNS; firewall; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open physical ports; bridge; VRF; DHCP; DNS; firewall; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who belongs to this network and what may they reach? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Networks / LAN / VLAN. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Tag assignment vs observed traffic; QinQ outer/inner distinct. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Tagged/untagged evidence; DHCP test; reachability. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; VLAN ID; parent/bridge; outer tag when supported; subnet; ports/tagging; DHCP; DNS; access; management. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Create Guest VLAN30 with tagged uplink, access ports, DHCP and Internet-only policy in one review. Diagnose: cell 37. |
| 37 | Diagnostics | Tagged/untagged evidence; DHCP test; reachability; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Create home/guest/segmented network; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep LAN protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `vlan` — Layered frame with a visible tag tab. Not bridge membership. See DESIGN_SYSTEM status grammar. |

<a id="p023"></a>

## P023 — Address Plan / IPAM

Source families: nac ipv6. **Editor field groups:** Prefix; VRF; role; allocation owner; pool range; reservation.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Address Plan / IPAM |
| 2 | URL / route | `/lan/address-plan`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | LAN / VLAN → Address Plan / IPAM |
| 4 | Purpose | Coherent management/inspection of IPAM; prefixes; pools; overlaps; reservations; assignments. |
| 5 | Administrator questions | Where can I allocate a subnet without overlap? |
| 6 | Main objects | IPAM; prefixes; pools; overlaps; reservations; assignments |
| 7 | Table / list columns | Prefix; family; VRF; owner; pool; used/free/unknown; conflicts |
| 8 | Filters | VRF; family; allocated; overlaps |
| 9 | Search | C01; VRF; family; allocated; overlaps. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Allocate prefix (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Create network; reserve address; inspect leases; export |
| 15 | Dangerous actions | H; C03. Impact depends on network inventory; DHCP; routes; leases. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Prefix; VRF; role; allocation owner; pool range; reservation. |
| 18 | Desired vs observed | C05; configured fields separate from Unknown occupancy is not free; imported allocation distinct from observation. |
| 19 | Runtime health / status | Unknown occupancy is not free; imported allocation distinct from observation C05. |
| 20 | Dependencies | network inventory; DHCP; routes; leases |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open network inventory; DHCP; routes; leases through related object links. |
| 23 | Explain Why integration | Seed this question/context: Where can I allocate a subnet without overlap? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Address Plan / IPAM. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Unknown occupancy is not free; imported allocation distinct from observation. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Overlap; duplicate IP; route conflicts. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Prefix; VRF; role; allocation owner; pool range; reservation. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Allocate a new guest prefix only after checking overlaps in the intended VRF; unknown occupancy is not free. Diagnose: cell 37. |
| 37 | Diagnostics | Overlap; duplicate IP; route conflicts; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Suggest safe subnet with uncertainty visible; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep IPAM protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `ipam` — Address blocks within a numbered grid. Not route table. See DESIGN_SYSTEM status grammar. |

<a id="p024"></a>

## P024 — Clients / Devices

Source families: devices catalog nac accesslifecycle. **Editor field groups:** Alias; tags; reservation; access intent; expiry; quarantine reason.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Clients / Devices |
| 2 | URL / route | `/lan/clients`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | LAN / VLAN → Clients / Devices |
| 4 | Purpose | Coherent management/inspection of device identity; aliases; tags; active sessions; quarantine; NAC; reservations; trust. |
| 5 | Administrator questions | Who is this device and why does it have this access? |
| 6 | Main objects | device identity; aliases; tags; active sessions; quarantine; NAC; reservations; trust |
| 7 | Table / list columns | Alias; IPs; MAC; network; identity source; last seen; access; sessions |
| 8 | Filters | IP/MAC/name; network; tags; quarantine; freshness |
| 9 | Search | C01; IP/MAC/name; network; tags; quarantine; freshness. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect client (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Access policy; reservation; logs; trace; forget observation; clone policy |
| 15 | Dangerous actions | H; C03. Impact depends on DHCP; ARP/ND/FDB; authentication; policy; aliases. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Alias; tags; reservation; access intent; expiry; quarantine reason. |
| 18 | Desired vs observed | C05; configured fields separate from Connected requires evidence; DHCP lease alone not presence; randomized MAC not stable person. |
| 19 | Runtime health / status | Connected requires evidence; DHCP lease alone not presence; randomized MAC not stable person C05. |
| 20 | Dependencies | DHCP; ARP/ND/FDB; authentication; policy; aliases |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open DHCP; ARP/ND/FDB; authentication; policy; aliases through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who is this device and why does it have this access? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Clients / Devices. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Connected requires evidence; DHCP lease alone not presence; randomized MAC not stable person. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Explain access; traffic path; DNS; IP conflict. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Alias; tags; reservation; access intent; expiry; quarantine reason. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Find a device by MAC, reserve its address and assign NAS-only access without switching unrelated sections. Diagnose: cell 37. |
| 37 | Diagnostics | Explain access; traffic path; DNS; IP conflict; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Devices with clear online/last-seen distinction; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep device identity protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `client` — Endpoint screen with a small identity tag. Not administrator account. See DESIGN_SYSTEM status grammar. |

<a id="p025"></a>

## P025 — Wi-Fi

Source families: wifi hostapd hardware. **Editor field groups:** Country; radio; SSID; security; Vault credential; channel auto/manual; VLAN; isolation; enterprise AAA.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Wi-Fi |
| 2 | URL / route | `/lan/wifi`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | LAN / VLAN → Wi-Fi |
| 4 | Purpose | Coherent management/inspection of AP; station; SSID; security; hostapd; channels; radio capabilities. |
| 5 | Administrator questions | Is radio service available and why is this client struggling? |
| 6 | Main objects | AP; station; SSID; security; hostapd; channels; radio capabilities |
| 7 | Table / list columns | Radio; SSID; band/channel; security; clients; health; signal |
| 8 | Filters | radio; band; SSID; degraded; capability |
| 9 | Search | C01; radio; band; SSID; degraded; capability. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create Wi-Fi network (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Stations; logs; survey; access; RADIUS dependency |
| 15 | Dangerous actions | H; C03. Impact depends on radio; regulatory domain; LAN; hostapd; credentials. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Country; radio; SSID; security; Vault credential; channel auto/manual; VLAN; isolation; enterprise AAA. |
| 18 | Desired vs observed | C05; configured fields separate from AP configured vs beacon vs association vs client traffic. |
| 19 | Runtime health / status | AP configured vs beacon vs association vs client traffic C05. |
| 20 | Dependencies | radio; regulatory domain; LAN; hostapd; credentials |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open radio; regulatory domain; LAN; hostapd; credentials through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is radio service available and why is this client struggling? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Wi-Fi. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for AP configured vs beacon vs association vs client traffic. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Association reason; signal; interference evidence; DHCP. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Country; radio; SSID; security; Vault credential; channel auto/manual; VLAN; isolation; enterprise AAA. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for radio; regulatory domain; LAN; hostapd; credentials. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An AP is configured but no beacon evidence exists; show the missing radio/runtime result. Diagnose: cell 37. |
| 37 | Diagnostics | Association reason; signal; interference evidence; DHCP; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Wi-Fi setup/status where supported, otherwise explanatory entry; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep AP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `wifi` — Radio arcs over an access point dot. Never use arcs to imply signal measurement. See DESIGN_SYSTEM status grammar. |

<a id="p026"></a>

## P026 — IPv6 Addressing / PD

Source families: ipv6 network. **Editor field groups:** PD source; requested size; LAN allocation IDs; RA/DHCPv6 policy; lifetimes where supported.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | IPv6 Addressing / PD |
| 2 | URL / route | `/lan/ipv6`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | LAN / VLAN → IPv6 Addressing / PD |
| 4 | Purpose | Coherent management/inspection of DHCPv6; SLAAC; RA; prefix delegation; IPv6 isolation. |
| 5 | Administrator questions | Which delegated prefixes reach which networks and when will they expire? |
| 6 | Main objects | DHCPv6; SLAAC; RA; prefix delegation; IPv6 isolation |
| 7 | Table / list columns | WAN; delegated prefix; preferred/valid lifetimes; LAN assignment; RA; state |
| 8 | Filters | WAN; LAN; expired; family |
| 9 | Search | C01; WAN; LAN; expired; family. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure prefix assignment (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Probe IPv6; inspect guards; affected tunnels |
| 15 | Dangerous actions | C; C03. Impact depends on ISP PD; RA; DHCPv6; LAN; v6 firewall; DNS. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: PD source; requested size; LAN allocation IDs; RA/DHCPv6 policy; lifetimes where supported. |
| 18 | Desired vs observed | C05; configured fields separate from IPv4 success never implies IPv6 success; withdrawn prefix shown. |
| 19 | Runtime health / status | IPv4 success never implies IPv6 success; withdrawn prefix shown C05. |
| 20 | Dependencies | ISP PD; RA; DHCPv6; LAN; v6 firewall; DNS |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open ISP PD; RA; DHCPv6; LAN; v6 firewall; DNS through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which delegated prefixes reach which networks and when will they expire? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: IPv6 Addressing / PD. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for IPv4 success never implies IPv6 success; withdrawn prefix shown. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: RA evidence; prefix lifecycle; v6 leak test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: PD source; requested size; LAN allocation IDs; RA/DHCPv6 policy; lifetimes where supported. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An ISP prefix expires: identify affected LAN assignments and client exports before accepting new addressing. Diagnose: cell 37. |
| 37 | Diagnostics | RA evidence; prefix lifecycle; v6 leak test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | IPv6 automatic with explicit status; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep DHCPv6 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `ipv6` — Address block with small 6 qualifier. Label remains IPv6, no family inference. See DESIGN_SYSTEM status grammar. |

<a id="p027"></a>

## P027 — Routes / RIB / FIB

Source families: network dynamicrt vrf nac. **Editor field groups:** Prefix; table/VRF; interface; gateway; metric; route type; multipath when supported; Anycast name, VIP, local node, health source and hysteresis.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Routes / RIB / FIB |
| 2 | URL / route | `/routing/routes`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → Routes / RIB / FIB |
| 4 | Purpose | Coherent management/inspection of configured routes; active routes; RIB; FIB; recursion; ECMP; route selection; Anycast VIP advertisement intent. |
| 5 | Administrator questions | Why was this next hop selected and is it installed? |
| 6 | Main objects | configured routes; active routes; RIB; FIB; recursion; ECMP; route selection; Anycast VIP advertisement intent |
| 7 | Table / list columns | Prefix; table/VRF; source; preference; metric; next hops; selected; installed; reason |
| 8 | Filters | prefix contains; family; VRF/table; source; rejected; not installed |
| 9 | Search | C01; prefix contains; family; VRF/table; source; rejected; not installed. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create static route (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Why selected; simulate packet; looking glass; dependencies; Anycast tab; inspect advertisement/withdrawal evidence |
| 15 | Dangerous actions | C; C03. Impact depends on interface; gateway; recursion; policy rules; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Prefix; table/VRF; interface; gateway; metric; route type; multipath when supported; Anycast name, VIP, local node, health source and hysteresis. |
| 18 | Desired vs observed | C05; configured fields separate from Configured, RIB-selected and FIB-installed are separate; unavailable RIB stays unavailable; Anycast Healthy model flag does not prove live health or route advertisement. |
| 19 | Runtime health / status | Configured, RIB-selected and FIB-installed are separate; unavailable RIB stays unavailable; Anycast Healthy model flag does not prove live health or route advertisement C05. |
| 20 | Dependencies | interface; gateway; recursion; policy rules; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open interface; gateway; recursion; policy rules; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Why was this next hop selected and is it installed? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Routes / RIB / FIB. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Configured, RIB-selected and FIB-installed are separate; unavailable RIB stays unavailable; Anycast Healthy model flag does not prove live health or route advertisement. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Recursive resolution; path lookup; next-hop probe. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Prefix; table/VRF; interface; gateway; metric; route type; multipath when supported; Anycast name, VIP, local node, health source and hysteresis. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A configured route is not installed in FIB; inspect the provided next-hop or selection failure. Diagnose: cell 37. |
| 37 | Diagnostics | Recursive resolution; path lookup; next-hop probe; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Where traffic goes summary; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep configured routes protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `route` — Fork with a chosen next-hop marker. Not policy match. See DESIGN_SYSTEM status grammar. |

<a id="p028"></a>

## P028 — Policy Routing

Source families: network multiwan. **Editor field groups:** Match aliases; IP family; priority; preferred exit; fallback or fail closed; table advanced.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Policy Routing |
| 2 | URL / route | `/routing/policies`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → Policy Routing |
| 4 | Purpose | Coherent management/inspection of IP rules; preferred exit; selective routing; route groups; fallback. |
| 5 | Administrator questions | Which match and priority sent this traffic to this table? |
| 6 | Main objects | IP rules; preferred exit; selective routing; route groups; fallback |
| 7 | Table / list columns | Priority; name; source; destination; mark/context; action/table; fallback; owner |
| 8 | Filters | source; destination; table; priority; owner |
| 9 | Search | C01; source; destination; table; priority; owner. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create routing intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Why WAN2; view generated rule; reorder before; clone |
| 15 | Dangerous actions | C; C03. Impact depends on access policy; route group; routing tables; tunnel health. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Match aliases; IP family; priority; preferred exit; fallback or fail closed; table advanced. |
| 18 | Desired vs observed | C05; configured fields separate from Matched rule is not proof of successful route; failure policy must be explicit. |
| 19 | Runtime health / status | Matched rule is not proof of successful route; failure policy must be explicit C05. |
| 20 | Dependencies | access policy; route group; routing tables; tunnel health |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open access policy; route group; routing tables; tunnel health through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which match and priority sent this traffic to this table? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Policy Routing. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Matched rule is not proof of successful route; failure policy must be explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: What-if; actual rules vs desired; fail-closed check. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Match aliases; IP family; priority; preferred exit; fallback or fail closed; table advanced. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Staff traffic chooses a private routing table by policy priority, while authorization remains separate. Diagnose: cell 37. |
| 37 | Diagnostics | What-if; actual rules vs desired; fail-closed check; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Send device/application through selected exit; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep IP rules protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `policy-route` — Match diamond before a next-hop fork. Not firewall decision. See DESIGN_SYSTEM status grammar. |

<a id="p029"></a>

## P029 — VRF / Route Leaking

Source families: vrf. **Editor field groups:** Name; table ID; interfaces; import/export prefix selectors; filters.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | VRF / Route Leaking |
| 2 | URL / route | `/routing/vrfs`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → VRF / Route Leaking |
| 4 | Purpose | Coherent management/inspection of VRF; independent tables; members; route leaking. |
| 5 | Administrator questions | What is isolated and which prefixes may cross VRFs? |
| 6 | Main objects | VRF; independent tables; members; route leaking |
| 7 | Table / list columns | VRF; table; members; routes; imports; exports; policy; conflicts |
| 8 | Filters | VRF; table; member; leak direction |
| 9 | Search | C01; VRF; table; member; leak direction. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create VRF (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Add member; create leak; lookup; inspect dependencies |
| 15 | Dangerous actions | C; C03. Impact depends on interfaces; unique tables; route filters; firewall. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; table ID; interfaces; import/export prefix selectors; filters. |
| 18 | Desired vs observed | C05; configured fields separate from Leaking routes does not authorize traffic; show direction and enforcement. |
| 19 | Runtime health / status | Leaking routes does not authorize traffic; show direction and enforcement C05. |
| 20 | Dependencies | interfaces; unique tables; route filters; firewall |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open interfaces; unique tables; route filters; firewall through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is isolated and which prefixes may cross VRFs? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: VRF / Route Leaking. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Leaking routes does not authorize traffic; show direction and enforcement. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: VRF lookup; loop; overlapping prefix test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; table ID; interfaces; import/export prefix selectors; filters. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Leak the NAS prefix from Services VRF to Staff VRF, then explicitly review firewall access. Diagnose: cell 37. |
| 37 | Diagnostics | VRF lookup; loop; overlapping prefix test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Explain tenant isolation; Full editing; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep VRF protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `vrf` — Two separated routing compartments. Not an ordinary flat routing table. See DESIGN_SYSTEM status grammar. |

<a id="p030"></a>

## P030 — BGP

Source families: frr dynamicrt bgpadv. **Editor field groups:** Peer; ASNs; source; VRF; families; authentication ref; import/export policy; max prefixes; timers; BFD.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | BGP |
| 2 | URL / route | `/routing/bgp`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → BGP |
| 4 | Purpose | Coherent management/inspection of BGP peers; AFI/SAFI; received/advertised/accepted/rejected; attributes; reset. |
| 5 | Administrator questions | Is the peer established, what did it send, and why did routes get rejected? |
| 6 | Main objects | BGP peers; AFI/SAFI; received/advertised/accepted/rejected; attributes; reset |
| 7 | Table / list columns | Peer; VRF; local/remote AS; family; state; uptime; received/accepted/advertised; last error |
| 8 | Filters | peer; AS; VRF; family; state; prefix |
| 9 | Search | C01; peer; AS; VRF; family; state; prefix. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create BGP peer (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Received routes; advertised routes; Why rejected; soft refresh; hard reset |
| 15 | Dangerous actions | C; C03. Impact depends on FRR; addressing; authentication; filters; RPKI; VRF; BFD. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Peer; ASNs; source; VRF; families; authentication ref; import/export policy; max prefixes; timers; BFD. |
| 18 | Desired vs observed | C05; configured fields separate from Daemon-running, adjacency and usable prefixes separate; counters may be unknown. |
| 19 | Runtime health / status | Daemon-running, adjacency and usable prefixes separate; counters may be unknown C05. |
| 20 | Dependencies | FRR; addressing; authentication; filters; RPKI; VRF; BFD |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open FRR; addressing; authentication; filters; RPKI; VRF; BFD through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is the peer established, what did it send, and why did routes get rejected? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: BGP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Daemon-running, adjacency and usable prefixes separate; counters may be unknown. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Peer ping/traceroute; looking glass; filter trace; timers. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Peer; ASNs; source; VRF; families; authentication ref; import/export policy; max prefixes; timers; BFD. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for FRR; addressing; authentication; filters; RPKI; VRF; BFD. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Peer adjacency is Established but a prefix is rejected by an import term; open that term from the route. Diagnose: cell 37. |
| 37 | Diagnostics | Peer ping/traceroute; looking glass; filter trace; timers; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; incident prose in Easy; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep BGP peers protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `bgp` — Two peer nodes with an AS-link connector. Not a generic tunnel. See DESIGN_SYSTEM status grammar. |

<a id="p031"></a>

## P031 — OSPF

Source families: frr dynamicrt. **Editor field groups:** Router ID; version; areas; interfaces; costs; passive; auth ref; timers; redistribution.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | OSPF |
| 2 | URL / route | `/routing/ospf`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → OSPF |
| 4 | Purpose | Coherent management/inspection of OSPFv2; OSPFv3; instances; areas; neighbors; LSDB; redistribution. |
| 5 | Administrator questions | Why did this neighbor not reach Full and what routes changed? |
| 6 | Main objects | OSPFv2; OSPFv3; instances; areas; neighbors; LSDB; redistribution |
| 7 | Table / list columns | Instance; version; area; interface; neighbor; state; dead timer; cost; transition |
| 8 | Filters | instance; area; version; state; interface |
| 9 | Search | C01; instance; area; version; state; interface. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create OSPF instance (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Neighbors; LSDB; routes; capture; reset scoped adjacency |
| 15 | Dangerous actions | C; C03. Impact depends on FRR; interface; area; authentication; MTU; filters. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Router ID; version; areas; interfaces; costs; passive; auth ref; timers; redistribution. |
| 18 | Desired vs observed | C05; configured fields separate from Expose native neighbor state; current generic session model cannot invent OSPF detail. |
| 19 | Runtime health / status | Expose native neighbor state; current generic session model cannot invent OSPF detail C05. |
| 20 | Dependencies | FRR; interface; area; authentication; MTU; filters |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open FRR; interface; area; authentication; MTU; filters through related object links. |
| 23 | Explain Why integration | Seed this question/context: Why did this neighbor not reach Full and what routes changed? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: OSPF. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Expose native neighbor state; current generic session model cannot invent OSPF detail. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: MTU/timer/area mismatch; adjacency logs. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Router ID; version; areas; interfaces; costs; passive; auth ref; timers; redistribution. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for FRR; interface; area; authentication; MTU; filters. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An OSPF neighbor stops before Full: inspect area, timers and MTU evidence without guessing a cause. Diagnose: cell 37. |
| 37 | Diagnostics | MTU/timer/area mismatch; adjacency logs; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Failure explanation only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep OSPFv2 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `ospf` — Area boundary with three adjacent nodes. Not physical topology. See DESIGN_SYSTEM status grammar. |

<a id="p032"></a>

## P032 — IS-IS / RIP

Source families: isis dynamicrt. **Editor field groups:** Protocol; NET/level for IS-IS; RIP family; interfaces; timers; authentication; policies.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | IS-IS / RIP |
| 2 | URL / route | `/routing/other-protocols`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → IS-IS / RIP |
| 4 | Purpose | Coherent management/inspection of IS-IS; levels; NET; RIP; RIPng. |
| 5 | Administrator questions | Which instance and neighbor provide these routes? |
| 6 | Main objects | IS-IS; levels; NET; RIP; RIPng |
| 7 | Table / list columns | Protocol; instance; level/area; neighbor; interface; state; prefixes |
| 8 | Filters | protocol; instance; state |
| 9 | Search | C01; protocol; instance; state. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create protocol instance (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Neighbor diagnostics; routes; filter; reset |
| 15 | Dangerous actions | C; C03. Impact depends on daemon capability; interfaces; authentication; redistribution. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Protocol; NET/level for IS-IS; RIP family; interfaces; timers; authentication; policies. |
| 18 | Desired vs observed | C05; configured fields separate from Unsupported protocol stays discoverable with reason; no generic Established claim. |
| 19 | Runtime health / status | Unsupported protocol stays discoverable with reason; no generic Established claim C05. |
| 20 | Dependencies | daemon capability; interfaces; authentication; redistribution |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open daemon capability; interfaces; authentication; redistribution through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which instance and neighbor provide these routes? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: IS-IS / RIP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Unsupported protocol stays discoverable with reason; no generic Established claim. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Adjacency; protocol counters; loop diagnostics. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Protocol; NET/level for IS-IS; RIP family; interfaces; timers; authentication; policies. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for daemon capability; interfaces; authentication; redistribution. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: IS-IS level settings must match the intended instance; show unsupported runtime rather than a green adjacency. Diagnose: cell 37. |
| 37 | Diagnostics | Adjacency; protocol counters; loop diagnostics; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep IS-IS protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `protocol` — Neighbor link with a protocol label space. Variant label carries IS-IS/RIP identity. See DESIGN_SYSTEM status grammar. |

<a id="p033"></a>

## P033 — BFD

Source families: dynamicrt ipsla. **Editor field groups:** Peer; source; VRF; transmit/receive intervals; multiplier; supported mode.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | BFD |
| 2 | URL / route | `/routing/bfd`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → BFD |
| 4 | Purpose | Coherent management/inspection of BFD sessions; detection timers; dependent protocols. |
| 5 | Administrator questions | Which consumers will react to this failure detector? |
| 6 | Main objects | BFD sessions; detection timers; dependent protocols |
| 7 | Table / list columns | Session; peer; source; VRF; state; interval; multiplier; consumers |
| 8 | Filters | peer; VRF; down; consumer |
| 9 | Search | C01; peer; VRF; down; consumer. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure BFD session (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect consumers; probe; timing evidence |
| 15 | Dangerous actions | H; C03. Impact depends on protocol peer; path; daemon support. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Peer; source; VRF; transmit/receive intervals; multiplier; supported mode. |
| 18 | Desired vs observed | C05; configured fields separate from BFD enable flag alone is not session telemetry. |
| 19 | Runtime health / status | BFD enable flag alone is not session telemetry C05. |
| 20 | Dependencies | protocol peer; path; daemon support |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open protocol peer; path; daemon support through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which consumers will react to this failure detector? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: BFD. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for BFD enable flag alone is not session telemetry. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Detection timing; packet counters; correlated adjacency changes. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Peer; source; VRF; transmit/receive intervals; multiplier; supported mode. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A BFD detector feeds BGP: show which routing session will react to a detection timeout. Diagnose: cell 37. |
| 37 | Diagnostics | Detection timing; packet counters; correlated adjacency changes; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Dependent incident detail only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep BFD sessions protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `bfd` — Peer link with short interval ticks. Not WAN speed. See DESIGN_SYSTEM status grammar. |

<a id="p034"></a>

## P034 — Route Filters / Redistribution

Source families: dynamicrt frr bgpadv. **Editor field groups:** Match prefix/length/AS-path/community; action; local-pref/MED/community modifications; redistribution source.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Route Filters / Redistribution |
| 2 | URL / route | `/routing/filters`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → Route Filters / Redistribution |
| 4 | Purpose | Coherent management/inspection of route filters; prefix lists; communities; redistribution; accepted/rejected explanation. |
| 5 | Administrator questions | Which term accepts or changes this prefix and who uses it? |
| 6 | Main objects | route filters; prefix lists; communities; redistribution; accepted/rejected explanation |
| 7 | Table / list columns | Sequence; term; match; action; attribute changes; references; owner |
| 8 | Filters | policy; protocol; prefix; action; referenced |
| 9 | Search | C01; policy; protocol; prefix; action; referenced. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create route policy (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test prefix; move before; duplicate; inspect consumers |
| 15 | Dangerous actions | C; C03. Impact depends on peers; protocol instances; route schema; RPKI. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Match prefix/length/AS-path/community; action; local-pref/MED/community modifications; redistribution source. |
| 18 | Desired vs observed | C05; configured fields separate from Evaluation order never changes with table sorting; show native effective policy. |
| 19 | Runtime health / status | Evaluation order never changes with table sorting; show native effective policy C05. |
| 20 | Dependencies | peers; protocol instances; route schema; RPKI |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open peers; protocol instances; route schema; RPKI through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which term accepts or changes this prefix and who uses it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Route Filters / Redistribution. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Evaluation order never changes with table sorting; show native effective policy. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Trace prefix; shadow/conflict; before/after advertisements. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Match prefix/length/AS-path/community; action; local-pref/MED/community modifications; redistribution source. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Test an export filter with a specific prefix before changing all advertisements to a peer. Diagnose: cell 37. |
| 37 | Diagnostics | Trace prefix; shadow/conflict; before/after advertisements; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; policies explained by alias; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep route filters protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `route-filter` — Prefix list entering a term gate. Not packet firewall. See DESIGN_SYSTEM status grammar. |

<a id="p035"></a>

## P035 — RPKI / BMP / FlowSpec

Source families: bgpadv. **Editor field groups:** Validator/collector; trust scope; ROA policy; FlowSpec match/action; expiry; limits; communities.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | RPKI / BMP / FlowSpec |
| 2 | URL / route | `/routing/bgp-security`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → RPKI / BMP / FlowSpec |
| 4 | Purpose | Coherent management/inspection of RPKI validators; ROAs; origin validation; BMP; FlowSpec; large communities. |
| 5 | Administrator questions | Is origin validation current and what distributed policy will affect traffic? |
| 6 | Main objects | RPKI validators; ROAs; origin validation; BMP; FlowSpec; large communities |
| 7 | Table / list columns | Object; type; endpoint/prefix; AS; validity; freshness; action; expiry |
| 8 | Filters | type; validator; valid/invalid/not-found/unknown; expiry |
| 9 | Search | C01; type; validator; valid/invalid/not-found/unknown; expiry. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Add validation/telemetry object (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect ROA; evaluate route; revoke FlowSpec; view consumer |
| 15 | Dangerous actions | C; C03. Impact depends on BGP; reachable validator/collector; time; firewall capability. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Validator/collector; trust scope; ROA policy; FlowSpec match/action; expiry; limits; communities. |
| 18 | Desired vs observed | C05; configured fields separate from Unknown/stale validator not invalid; FlowSpec entries need installed evidence. |
| 19 | Runtime health / status | Unknown/stale validator not invalid; FlowSpec entries need installed evidence C05. |
| 20 | Dependencies | BGP; reachable validator/collector; time; firewall capability |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open BGP; reachable validator/collector; time; firewall capability through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is origin validation current and what distributed policy will affect traffic? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: RPKI / BMP / FlowSpec. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Unknown/stale validator not invalid; FlowSpec entries need installed evidence. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: ROA matching; refresh; FlowSpec action impact. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Validator/collector; trust scope; ROA policy; FlowSpec match/action; expiry; limits; communities. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for BGP; reachable validator/collector; time; firewall capability. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: RPKI validator data is stale: distinguish Unknown validation from an Invalid origin. Diagnose: cell 37. |
| 37 | Diagnostics | ROA matching; refresh; FlowSpec action impact; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; security summary; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep RPKI validators protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `rpki` — Prefix block with a verification seal. Not TLS certificate. See DESIGN_SYSTEM status grammar. |

<a id="p036"></a>

## P036 — MPLS / Segment Routing

Source families: srmpls tunnels. **Editor field groups:** Technology; RD; RTs; VRF; interface; labels or SIDs; endpoint/color; preference; peer.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | MPLS / Segment Routing |
| 2 | URL / route | `/routing/mpls`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → MPLS / Segment Routing |
| 4 | Purpose | Coherent management/inspection of MPLS; LDP; L3VPN; L2VPN; pseudowire; SR-MPLS; SRv6. |
| 5 | Administrator questions | Which labels/segments and VPN bindings carry this service? |
| 6 | Main objects | MPLS; LDP; L3VPN; L2VPN; pseudowire; SR-MPLS; SRv6 |
| 7 | Table / list columns | Service; RD; import/export RT; VRF; labels/segments; next hop; capability; installed |
| 8 | Filters | VPN type; VRF; RD/RT; technology; pending |
| 9 | Search | C01; VPN type; VRF; RD/RT; technology; pending. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create transport/VPN intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect path; label lookup; dependency preview; test |
| 15 | Dangerous actions | C; C03. Impact depends on kernel/FRR capability; underlay; MTU; interfaces; peer. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Technology; RD; RTs; VRF; interface; labels or SIDs; endpoint/color; preference; peer. |
| 18 | Desired vs observed | C05; configured fields separate from Model/renderer is not forwarding proof; each transport has own capability gate. |
| 19 | Runtime health / status | Model/renderer is not forwarding proof; each transport has own capability gate C05. |
| 20 | Dependencies | kernel/FRR capability; underlay; MTU; interfaces; peer |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open kernel/FRR capability; underlay; MTU; interfaces; peer through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which labels/segments and VPN bindings carry this service? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: MPLS / Segment Routing. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Model/renderer is not forwarding proof; each transport has own capability gate. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Label/SID path; reachability; MTU; failover. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Technology; RD; RTs; VRF; interface; labels or SIDs; endpoint/color; preference; peer. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A requested SRv6 path has no runtime support: retain its specification and explain the missing capability. Diagnose: cell 37. |
| 37 | Diagnostics | Label/SID path; reachability; MTU; failover; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep MPLS protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `mpls` — Packet path carrying stacked label tabs. Not VLAN tag alone. See DESIGN_SYSTEM status grammar. |

<a id="p037"></a>

## P037 — VXLAN / EVPN

Source families: evpn tunnels. **Editor field groups:** VNI; VTEP source; peers; bridge/VRF; RD/RT; ESI; multihoming mode.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | VXLAN / EVPN |
| 2 | URL / route | `/routing/evpn`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → VXLAN / EVPN |
| 4 | Purpose | Coherent management/inspection of VXLAN; VTEP; EVPN route types; multihoming; DF election. |
| 5 | Administrator questions | How is this tenant bridged/routed and who forwards on the segment? |
| 6 | Main objects | VXLAN; VTEP; EVPN route types; multihoming; DF election |
| 7 | Table / list columns | VNI; VTEP; VRF/bridge; RD/RT; ESI; mode; DF; learned routes |
| 8 | Filters | VNI; VTEP; VRF; ESI; route type |
| 9 | Search | C01; VNI; VTEP; VRF; ESI; route type. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create overlay network (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect type 2/3/5; DF; underlay path; MAC move |
| 15 | Dangerous actions | C; C03. Impact depends on BGP EVPN; underlay; bridges; VRF; MTU; peers. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: VNI; VTEP source; peers; bridge/VRF; RD/RT; ESI; multihoming mode. |
| 18 | Desired vs observed | C05; configured fields separate from Configured ESI/DF model not actual election; imported routes labeled. |
| 19 | Runtime health / status | Configured ESI/DF model not actual election; imported routes labeled C05. |
| 20 | Dependencies | BGP EVPN; underlay; bridges; VRF; MTU; peers |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open BGP EVPN; underlay; bridges; VRF; MTU; peers through related object links. |
| 23 | Explain Why integration | Seed this question/context: How is this tenant bridged/routed and who forwards on the segment? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: VXLAN / EVPN. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Configured ESI/DF model not actual election; imported routes labeled. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Underlay ping; BGP; FDB; duplicate MAC; MTU. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: VNI; VTEP source; peers; bridge/VRF; RD/RT; ESI; multihoming mode. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Inspect a VNI's underlay reachability and BGP EVPN routes independently of its configured bridge. Diagnose: cell 37. |
| 37 | Diagnostics | Underlay ping; BGP; FDB; duplicate MAC; MTU; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep VXLAN protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `overlay` — Inner path over separated underlay anchors. Not physical link. See DESIGN_SYSTEM status grammar. |

<a id="p038"></a>

## P038 — Multicast / Discovery Forwarding

Source families: multicast mcast. **Editor field groups:** Protocol; RP; groups/sources; networks; snooping; service-limited mDNS policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Multicast / Discovery Forwarding |
| 2 | URL / route | `/routing/multicast`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → Multicast / Discovery Forwarding |
| 4 | Purpose | Coherent management/inspection of PIM; IGMP; MLD; snooping; multicast routes; mDNS reflector. |
| 5 | Administrator questions | Where should discovery/multicast go and why is it missing? |
| 6 | Main objects | PIM; IGMP; MLD; snooping; multicast routes; mDNS reflector |
| 7 | Table / list columns | Group/source; network; RP; incoming/outgoing interfaces; members; RPF; owner |
| 8 | Filters | group; source; interface; protocol; RPF failure |
| 9 | Search | C01; group; source; interface; protocol; RPF failure. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create multicast policy (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Membership; RPF Why; scoped mDNS reflect; capture |
| 15 | Dangerous actions | H; C03. Impact depends on interfaces; unicast RPF; receiver membership; isolation. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Protocol; RP; groups/sources; networks; snooping; service-limited mDNS policy. |
| 18 | Desired vs observed | C05; configured fields separate from mDNS reflection is not general inter-network access; membership expires. |
| 19 | Runtime health / status | mDNS reflection is not general inter-network access; membership expires C05. |
| 20 | Dependencies | interfaces; unicast RPF; receiver membership; isolation |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open interfaces; unicast RPF; receiver membership; isolation through related object links. |
| 23 | Explain Why integration | Seed this question/context: Where should discovery/multicast go and why is it missing? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Multicast / Discovery Forwarding. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for mDNS reflection is not general inter-network access; membership expires. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: IGMP/MLD capture; RPF; discovery test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Protocol; RP; groups/sources; networks; snooping; service-limited mDNS policy. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Reflect only selected discovery services between Staff and Printers, not unrestricted network access. Diagnose: cell 37. |
| 37 | Diagnostics | IGMP/MLD capture; RPF; discovery test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Share selected discovery services between networks; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep PIM protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `multicast` — One source branching to multiple receivers. Not load-balancing multiple exits. See DESIGN_SYSTEM status grammar. |

<a id="p039"></a>

## P039 — IPv6 Transition / CGNAT

Source families: ipv6 ipv6trans. **Editor field groups:** Mechanism; provider parameters; prefixes; pool ranges; capacity limits; DNS64 linkage.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | IPv6 Transition / CGNAT |
| 2 | URL / route | `/routing/transition`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → IPv6 Transition / CGNAT |
| 4 | Purpose | Coherent management/inspection of NAT64; DNS64; 464XLAT; DS-Lite; 6rd; MAP-E; MAP-T; NPTv6; CGNAT pools. |
| 5 | Administrator questions | Which address families are translated and is capacity exhausted? |
| 6 | Main objects | NAT64; DNS64; 464XLAT; DS-Lite; 6rd; MAP-E; MAP-T; NPTv6; CGNAT pools |
| 7 | Table / list columns | Mechanism; prefix; upstream; pool; allocation; exhaustion; capability |
| 8 | Filters | mechanism; family; pool; exhaustion |
| 9 | Search | C01; mechanism; family; pool; exhaustion. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure transition intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect mapping; probe v4/v6; capacity; dependencies |
| 15 | Dangerous actions | C; C03. Impact depends on provider mechanism; kernel; DNS64; pools; routing; firewall. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Mechanism; provider parameters; prefixes; pool ranges; capacity limits; DNS64 linkage. |
| 18 | Desired vs observed | C05; configured fields separate from Each mechanism individually gated; never display all as supported because enum exists. |
| 19 | Runtime health / status | Each mechanism individually gated; never display all as supported because enum exists C05. |
| 20 | Dependencies | provider mechanism; kernel; DNS64; pools; routing; firewall |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open provider mechanism; kernel; DNS64; pools; routing; firewall through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which address families are translated and is capacity exhausted? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: IPv6 Transition / CGNAT. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Each mechanism individually gated; never display all as supported because enum exists. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Dual-stack probes; pool exhaustion; DNS synthesis; MTU. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Mechanism; provider parameters; prefixes; pool ranges; capacity limits; DNS64 linkage. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: DNS64 synthesizes answers for a NAT64 deployment; verify both DNS synthesis and the forwarding path. Diagnose: cell 37. |
| 37 | Diagnostics | Dual-stack probes; pool exhaustion; DNS synthesis; MTU; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Provider setup explanation; Full controls; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep NAT64 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `transition` — Two address-family blocks through converter. Not NAT rule direction alone. See DESIGN_SYSTEM status grammar. |

<a id="p040"></a>

## P040 — Gateway Redundancy / VRRP

Source families: vrrp. **Editor field groups:** VRID; interface; addresses; priority; preemption; timers; health tracks.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Gateway Redundancy / VRRP |
| 2 | URL / route | `/routing/redundancy`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → Gateway Redundancy / VRRP |
| 4 | Purpose | Coherent management/inspection of VRRP; virtual addresses; master/backup; tracked health. |
| 5 | Administrator questions | Which node currently owns this address and why? |
| 6 | Main objects | VRRP; virtual addresses; master/backup; tracked health |
| 7 | Table / list columns | Group; interface; VRID; virtual addresses; priority; desired/observed role; transition |
| 8 | Filters | group; interface; role; family |
| 9 | Search | C01; group; interface; role; family. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create redundancy group (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect election; tracked probe; controlled failover |
| 15 | Dangerous actions | C; C03. Impact depends on peer; interface; virtual address; advertisement; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: VRID; interface; addresses; priority; preemption; timers; health tracks. |
| 18 | Desired vs observed | C05; configured fields separate from No automatic routing redundancy claim from local election model. |
| 19 | Runtime health / status | No automatic routing redundancy claim from local election model C05. |
| 20 | Dependencies | peer; interface; virtual address; advertisement; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open peer; interface; virtual address; advertisement; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which node currently owns this address and why? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Gateway Redundancy / VRRP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No automatic routing redundancy claim from local election model. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Advertisement capture; split-brain evidence; takeover impact. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: VRID; interface; addresses; priority; preemption; timers; health tracks. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A local VRRP election model says backup; current master ownership still needs observed evidence. Diagnose: cell 37. |
| 37 | Diagnostics | Advertisement capture; split-brain evidence; takeover impact; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Backup gateway status only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep VRRP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `redundancy` — Shared virtual address over paired gateways. Not multiple WAN providers. See DESIGN_SYSTEM status grammar. |

<a id="p041"></a>

## P041 — Access Policies

Source families: network devices publish wireguard xray. **Editor field groups:** Source identities/networks; destinations; services; peer/router permission; Internet/full/selected/custom; exit policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Access Policies |
| 2 | URL / route | `/firewall/access`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → Access Policies |
| 4 | Purpose | Coherent management/inspection of human-friendly access; Staff to NAS SMB; guest Internet only; tunnel isolation. |
| 5 | Administrator questions | Who may reach what and is router access separate? |
| 6 | Main objects | human-friendly access; Staff to NAS SMB; guest Internet only; tunnel isolation |
| 7 | Table / list columns | Policy; source alias; destination alias; service; action; routing intent; owner; effect |
| 8 | Filters | source; destination; service; owner; enabled |
| 9 | Search | C01; source; destination; service; owner; enabled. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create access intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Why blocked; simulate; edit generated rules; clone |
| 15 | Dangerous actions | C; C03. Impact depends on identity/network aliases; services; firewall; routes; tunnel profiles. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Source identities/networks; destinations; services; peer/router permission; Internet/full/selected/custom; exit policy. |
| 18 | Desired vs observed | C05; configured fields separate from Client routing intent is not authorization; L2-local traffic may bypass routed firewall. |
| 19 | Runtime health / status | Client routing intent is not authorization; L2-local traffic may bypass routed firewall C05. |
| 20 | Dependencies | identity/network aliases; services; firewall; routes; tunnel profiles |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open identity/network aliases; services; firewall; routes; tunnel profiles through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who may reach what and is router access separate? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Access Policies. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Client routing intent is not authorization; L2-local traffic may bypass routed firewall. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Packet path; rule compiler; IPv6 and peer isolation. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Source identities/networks; destinations; services; peer/router permission; Internet/full/selected/custom; exit policy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Staff -> NAS -> SMB generates explicit access rules and shows whether router access remains denied. Diagnose: cell 37. |
| 37 | Diagnostics | Packet path; rule compiler; IPv6 and peer isolation; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Allow/block and access presets; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep human-friendly access protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `access` — Source/service/destination blocks through gate. Not identity role administration. See DESIGN_SYSTEM status grammar. |

<a id="p042"></a>

## P042 — Firewall Rules

Source families: network fwanalysis. **Editor field groups:** Match context; source; destination; protocol/service; action; logging bound; ordering; advanced state.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Firewall Rules |
| 2 | URL / route | `/firewall/rules`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → Firewall Rules |
| 4 | Purpose | Coherent management/inspection of nftables; filter chains/hooks; rule order; generated rules; counters; raw advanced rules. |
| 5 | Administrator questions | Which rule sees this packet, in what order, and with what effect? |
| 6 | Main objects | nftables; filter chains/hooks; rule order; generated rules; counters; raw advanced rules |
| 7 | Table / list columns | Order; ID/name; hook/chain; ingress; source; destination; service; action; hits; owner |
| 8 | Filters | chain/hook; family; source/destination; disabled; owner; hit count |
| 9 | Search | C01; chain/hook; family; source/destination; disabled; owner; hit count. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create rule (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Move before/after; duplicate; quick comment; disable; inspect generated |
| 15 | Dangerous actions | C; C03. Impact depends on hook/priority; interfaces; sets; NAT; conntrack; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Match context; source; destination; protocol/service; action; logging bound; ordering; advanced state. |
| 18 | Desired vs observed | C05; configured fields separate from Stable ID distinct from sequence; evaluation order fixed despite sort/filter; counters source/age. |
| 19 | Runtime health / status | Stable ID distinct from sequence; evaluation order fixed despite sort/filter; counters source/age C05. |
| 20 | Dependencies | hook/priority; interfaces; sets; NAT; conntrack; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open hook/priority; interfaces; sets; NAT; conntrack; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which rule sees this packet, in what order, and with what effect? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Firewall Rules. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Stable ID distinct from sequence; evaluation order fixed despite sort/filter; counters source/age. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: What-if; shadow/conflict analyzer; matched rule; bounded trace. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Match context; source; destination; protocol/service; action; logging bound; ordering; advanced state. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Sort by hits to investigate traffic, then return to evaluation order before moving a rule. Diagnose: cell 37. |
| 37 | Diagnostics | What-if; shadow/conflict analyzer; matched rule; bounded trace; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Access-policy equivalent, not raw chain wizard; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep nftables protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `firewall` — Packet boundary with deliberate opening. Not generic shield for every page. See DESIGN_SYSTEM status grammar. |

<a id="p043"></a>

## P043 — NAT / Published Services

Source families: network publish sites fwanalysis. **Editor field groups:** WAN/address; TCP/UDP; external port; target host/port; source restrictions; hairpin opt-in; optional DNS/cert.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | NAT / Published Services |
| 2 | URL / route | `/firewall/nat`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → NAT / Published Services |
| 4 | Purpose | Coherent management/inspection of DNAT; SNAT; masquerade; hairpin; NETMAP; publish local service. |
| 5 | Administrator questions | Which address changes, where is permission granted, and can outside clients connect? |
| 6 | Main objects | DNAT; SNAT; masquerade; hairpin; NETMAP; publish local service |
| 7 | Table / list columns | Intent/rule; ingress; original tuple; translated tuple; allow rule; owner; status |
| 8 | Filters | type; WAN; target; family; owner |
| 9 | Search | C01; type; WAN; target; family; owner. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Publish local service (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | External test; split DNS vs hairpin; generated allow; disable; clone |
| 15 | Dangerous actions | C; C03. Impact depends on WAN/public-IP; target return route; firewall; certificate optional; DNS. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: WAN/address; TCP/UDP; external port; target host/port; source restrictions; hairpin opt-in; optional DNS/cert. |
| 18 | Desired vs observed | C05; configured fields separate from Listener, NAT installed and outside reachability separate; CGNAT shown before publish. |
| 19 | Runtime health / status | Listener, NAT installed and outside reachability separate; CGNAT shown before publish C05. |
| 20 | Dependencies | WAN/public-IP; target return route; firewall; certificate optional; DNS |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open WAN/public-IP; target return route; firewall; certificate optional; DNS through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which address changes, where is permission granted, and can outside clients connect? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: NAT / Published Services. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Listener, NAT installed and outside reachability separate; CGNAT shown before publish. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: External vantage test; conntrack; capture; hairpin path. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: WAN/address; TCP/UDP; external port; target host/port; source restrictions; hairpin opt-in; optional DNS/cert. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: WAN1 TCP443 forwards to 192.168.10.20:443; show the associated allow and optional local hairpin choice. Diagnose: cell 37. |
| 37 | Diagnostics | External vantage test; conntrack; capture; hairpin path; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Publish guided TCP/UDP intent with exact exposure sentence; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep DNAT protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `nat` — Original tuple transformed to a second tuple. Not routing fork. See DESIGN_SYSTEM status grammar. |

<a id="p044"></a>

## P044 — Firewall Objects / Feeds

Source families: fwanalysis multicast network. **Editor field groups:** Name; type; members/source; update interval; retention/expiry; safe failure behavior.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Firewall Objects / Feeds |
| 2 | URL / route | `/firewall/objects`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → Firewall Objects / Feeds |
| 4 | Purpose | Coherent management/inspection of sets; aliases; FQDN; Geo/ASN/threat feeds; service groups; address groups. |
| 5 | Administrator questions | What addresses does this alias currently mean and who relies on it? |
| 6 | Main objects | sets; aliases; FQDN; Geo/ASN/threat feeds; service groups; address groups |
| 7 | Table / list columns | Name; kind; configured source; resolved members; refreshed; expiry; consumers |
| 8 | Filters | kind; stale; referenced; source; tag |
| 9 | Search | C01; kind; stale; referenced; source; tag. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create object (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Inspect resolved members; refresh; duplicate; dependency view |
| 15 | Dangerous actions | H; C03. Impact depends on DNS; feed provenance; credentials if private; time; limits. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; type; members/source; update interval; retention/expiry; safe failure behavior. |
| 18 | Desired vs observed | C05; configured fields separate from FQDN answers are time-bound; Geo confidence and feed freshness explicit. |
| 19 | Runtime health / status | FQDN answers are time-bound; Geo confidence and feed freshness explicit C05. |
| 20 | Dependencies | DNS; feed provenance; credentials if private; time; limits |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open DNS; feed provenance; credentials if private; time; limits through related object links. |
| 23 | Explain Why integration | Seed this question/context: What addresses does this alias currently mean and who relies on it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Firewall Objects / Feeds. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for FQDN answers are time-bound; Geo confidence and feed freshness explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Resolution; feed parse errors; affected-rule preview. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; type; members/source; update interval; retention/expiry; safe failure behavior. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An FQDN alias changed addresses: show current members, age and all affected policies. Diagnose: cell 37. |
| 37 | Diagnostics | Resolution; feed parse errors; affected-rule preview; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Named devices/networks/services; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep sets protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `set` — Stacked named address/service tokens. Not configuration template. See DESIGN_SYSTEM status grammar. |

<a id="p045"></a>

## P045 — Connections / Conntrack

Source families: conntrack. **Editor field groups:** Filter scope; selected IDs; helper assignment advanced; explicit termination reason.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Connections / Conntrack |
| 2 | URL / route | `/firewall/connections`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → Connections / Conntrack |
| 4 | Purpose | Coherent management/inspection of sessions; conntrack; helper/ALG; selected kill; capacity. |
| 5 | Administrator questions | Which sessions exist and what will terminating them interrupt? |
| 6 | Main objects | sessions; conntrack; helper/ALG; selected kill; capacity |
| 7 | Table / list columns | Original/reply tuple; state; rule; NAT; timeout; bytes; helper; mark |
| 8 | Filters | IP; port; state; helper; NAT; WAN |
| 9 | Search | C01; IP; port; state; helper; NAT; WAN. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect session (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Why path; kill selected with count; capture; related rule |
| 15 | Dangerous actions | H; C03. Impact depends on kernel conntrack; rule/route provenance; permission. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Filter scope; selected IDs; helper assignment advanced; explicit termination reason. |
| 18 | Desired vs observed | C05; configured fields separate from Sessions historical state can outlive changed rules; missing count not zero. |
| 19 | Runtime health / status | Sessions historical state can outlive changed rules; missing count not zero C05. |
| 20 | Dependencies | kernel conntrack; rule/route provenance; permission |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open kernel conntrack; rule/route provenance; permission through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which sessions exist and what will terminating them interrupt? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Connections / Conntrack. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Sessions historical state can outlive changed rules; missing count not zero. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Capacity; stale state; asymmetric return path. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Filter scope; selected IDs; helper assignment advanced; explicit termination reason. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Terminate only selected stale sessions after reviewing their clients; do not flush all conntrack silently. Diagnose: cell 37. |
| 37 | Diagnostics | Capacity; stale state; asymmetric return path; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Active sessions for selected client; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep sessions protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `connection` — Paired forward/reply lines with session dot. Not interface cable. See DESIGN_SYSTEM status grammar. |

<a id="p046"></a>

## P046 — Firewall Analysis / Protection

Source families: fwanalysis secpolicy nac. **Editor field groups:** Control-plane classes/rates; uRPF mode; flood limits; quarantine scope/expiry; IDS detect/prevent explicit.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Firewall Analysis / Protection |
| 2 | URL / route | `/firewall/protection`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Firewall → Firewall Analysis / Protection |
| 4 | Purpose | Coherent management/inspection of shadow/conflict; firewall analyzer; DoS/flood; uRPF; CoPP; quarantine; optional IDS/IPS. |
| 5 | Administrator questions | What is unsafe, redundant or blocking legitimate traffic? |
| 6 | Main objects | shadow/conflict; firewall analyzer; DoS/flood; uRPF; CoPP; quarantine; optional IDS/IPS |
| 7 | Table / list columns | Finding; affected rule/object; severity reason; evidence; coverage; suggested action |
| 8 | Filters | severity; family; analyzer; acknowledged; capability |
| 9 | Search | C01; severity; family; analyzer; acknowledged; capability. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; actual evaluation order default, analysis sort labeled; no ambiguous reorder while filtered/sorted. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Analyze candidate (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Explain; stage fix; quarantine with expiry; inspect control-plane limit |
| 15 | Dangerous actions | C; C03. Impact depends on rules; routes; management; capability; signatures/feed; resource budget. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Control-plane classes/rates; uRPF mode; flood limits; quarantine scope/expiry; IDS detect/prevent explicit. |
| 18 | Desired vs observed | C05; configured fields separate from Static warning not live exploit proof; IDS unavailable not healthy protection. |
| 19 | Runtime health / status | Static warning not live exploit proof; IDS unavailable not healthy protection C05. |
| 20 | Dependencies | rules; routes; management; capability; signatures/feed; resource budget |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open rules; routes; management; capability; signatures/feed; resource budget through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is unsafe, redundant or blocking legitimate traffic? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Firewall Analysis / Protection. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Static warning not live exploit proof; IDS unavailable not healthy protection. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Rule conflict; flood counters; uRPF route evidence; scan budget. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Control-plane classes/rates; uRPF mode; flood limits; quarantine scope/expiry; IDS detect/prevent explicit. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A shadowed rule finding links to the earlier deciding rule and remains a static-analysis conclusion. Diagnose: cell 37. |
| 37 | Diagnostics | Rule conflict; flood counters; uRPF route evidence; scan budget; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Security issues and recommended preview; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep shadow/conflict protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `protection` — Boundary plus monitored control gate. Not a claim of antivirus protection. See DESIGN_SYSTEM status grammar. |

<a id="p047"></a>

## P047 — WireGuard Servers

Source families: wireguard vault. **Editor field groups:** Name; listen interface/port; tunnel addresses; key generation; MTU; peer/router/Internet access.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | WireGuard Servers |
| 2 | URL / route | `/tunnels/wireguard`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Tunnels → WireGuard Servers |
| 4 | Purpose | Coherent management/inspection of WireGuard interfaces; servers; key refs; listen; routes; isolation; MTU. |
| 5 | Administrator questions | Who can connect and what can tunnel clients reach? |
| 6 | Main objects | WireGuard interfaces; servers; key refs; listen; routes; isolation; MTU |
| 7 | Table / list columns | Server; listen; interface; addresses; enabled; peers; handshake age; traffic |
| 8 | Filters | server; state; listen; owner |
| 9 | Search | C01; server; state; listen; owner. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create WireGuard server (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | +Peer; test listener; access; routes; rotate server key |
| 15 | Dangerous actions | C; C03. Impact depends on key Vault; listen interface; firewall; routes; management; MTU. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; listen interface/port; tunnel addresses; key generation; MTU; peer/router/Internet access. |
| 18 | Desired vs observed | C05; configured fields separate from Server configured vs kernel present vs handshakes/traffic distinct. |
| 19 | Runtime health / status | Server configured vs kernel present vs handshakes/traffic distinct C05. |
| 20 | Dependencies | key Vault; listen interface; firewall; routes; management; MTU |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open key Vault; listen interface; firewall; routes; management; MTU through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who can connect and what can tunnel clients reach? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: WireGuard Servers. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Server configured vs kernel present vs handshakes/traffic distinct. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Handshake; route; port exposure; PMTU. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; listen interface/port; tunnel addresses; key generation; MTU; peer/router/Internet access. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for key Vault; listen interface; firewall; routes; management; MTU. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Create a WireGuard server with peer-to-peer traffic denied unless deliberately enabled. Diagnose: cell 37. |
| 37 | Diagnostics | Handshake; route; port exposure; PMTU; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Give remote access wizard; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep WireGuard interfaces protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `wireguard` — Sleeved link marked WG in nearby text. Use label, not copied vendor logo. See DESIGN_SYSTEM status grammar. |

<a id="p048"></a>

## P048 — WireGuard Peers / Exports

Source families: wireguard clientexport accesslifecycle catalog. **Editor field groups:** Name; server; address allocation; allowed destinations; advanced client routes; expiry; tags; optional endpoint.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | WireGuard Peers / Exports |
| 2 | URL / route | `/tunnels/wireguard/peers`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Tunnels → WireGuard Peers / Exports |
| 4 | Purpose | Coherent management/inspection of peer lifecycle; QR; conf; copy; rotate; expire; client profile revision. |
| 5 | Administrator questions | Is this peer authorized, is its client profile current, and can I export it safely? |
| 6 | Main objects | peer lifecycle; QR; conf; copy; rotate; expire; client profile revision |
| 7 | Table / list columns | Peer; server; alias; access scope; expiry; last handshake; bytes; profile revision; refresh required |
| 8 | Filters | server; enabled; expired; stale profile; tag |
| 9 | Search | C01; server; enabled; expired; stale profile; tag. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create peer (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Show QR; Download .conf; Copy config; Rotate keys; clone; revoke |
| 15 | Dangerous actions | H; C03. Impact depends on server; Vault; identity; export permission; routes/enforcement. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; server; address allocation; allowed destinations; advanced client routes; expiry; tags; optional endpoint. |
| 18 | Desired vs observed | C05; configured fields separate from Handshake not active use; configuration changed means remote must reapply; no download on list. |
| 19 | Runtime health / status | Handshake not active use; configuration changed means remote must reapply; no download on list C05. |
| 20 | Dependencies | server; Vault; identity; export permission; routes/enforcement |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open server; Vault; identity; export permission; routes/enforcement through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is this peer authorized, is its client profile current, and can I export it safely? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: WireGuard Peers / Exports. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Handshake not active use; configuration changed means remote must reapply; no download on list. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Exact QR/conf equality; access path; routing mismatch. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; server; address allocation; allowed destinations; advanced client routes; expiry; tags; optional endpoint. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for server; Vault; identity; export permission; routes/enforcement. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: An iPhone peer export changes after access scope changes; QR and .conf share the new exact bytes. Diagnose: cell 37. |
| 37 | Diagnostics | Exact QR/conf equality; access path; routing mismatch; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Native mobile-scannable access card; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep peer lifecycle protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `peer` — Single remote endpoint at end of sleeved link. Not whole VPN service. See DESIGN_SYSTEM status grammar. |

<a id="p049"></a>

## P049 — Xray Servers

Source families: xray vault certs. **Editor field groups:** Protocol; compatible transport/security; listen; TLS refs; REALITY key/short-ID generation; destination/SNI; fingerprint; advanced overrides.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Xray Servers |
| 2 | URL / route | `/tunnels/xray`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Tunnels → Xray Servers |
| 4 | Purpose | Coherent management/inspection of VLESS; Trojan; REALITY; TLS; transports; inbounds; native object management. |
| 5 | Administrator questions | What protocol/security is listening and which dependencies expose it? |
| 6 | Main objects | VLESS; Trojan; REALITY; TLS; transports; inbounds; native object management |
| 7 | Table / list columns | Server; protocol; transport/security; listen; cert/REALITY key ID; process; listener; probe |
| 8 | Filters | protocol; security; state; interface; owner |
| 9 | Search | C01; protocol; security; state; interface; owner. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create Xray server (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | +User; profile impact; test; logs; rotate key; dependencies |
| 15 | Dangerous actions | C; C03. Impact depends on Vault; certificate/time; supported transport; firewall; routing; DNS. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Protocol; compatible transport/security; listen; TLS refs; REALITY key/short-ID generation; destination/SNI; fingerprint; advanced overrides. |
| 18 | Desired vs observed | C05; configured fields separate from Process running is not listener or usable proxy; unsupported combinations rejected. |
| 19 | Runtime health / status | Process running is not listener or usable proxy; unsupported combinations rejected C05. |
| 20 | Dependencies | Vault; certificate/time; supported transport; firewall; routing; DNS |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open Vault; certificate/time; supported transport; firewall; routing; DNS through related object links. |
| 23 | Explain Why integration | Seed this question/context: What protocol/security is listening and which dependencies expose it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Xray Servers. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Process running is not listener or usable proxy; unsupported combinations rejected. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Syntax test; listener; authenticated end-to-end probe where available. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Protocol; compatible transport/security; listen; TLS refs; REALITY key/short-ID generation; destination/SNI; fingerprint; advanced overrides. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for Vault; certificate/time; supported transport; firewall; routing; DNS. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A VLESS/REALITY server needs compatible transport and key references; listener health is checked separately. Diagnose: cell 37. |
| 37 | Diagnostics | Syntax test; listener; authenticated end-to-end probe where available; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided private access server; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep VLESS protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `xray` — Proxy listener with routed outbound branch. Not WireGuard label substitution. See DESIGN_SYSTEM status grammar. |

<a id="p050"></a>

## P050 — Xray Users / Profiles

Source families: xray accesslifecycle clientexport. **Editor field groups:** Alias; server; credentials generated/ref; access/exit intent; expiry; tags; advanced routing separate warning.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Xray Users / Profiles |
| 2 | URL / route | `/tunnels/xray/profiles`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Tunnels → Xray Users / Profiles |
| 4 | Purpose | Coherent management/inspection of users; profiles; UUID/password; expiry; revoke; rotate; clone; tags; vless/trojan URI. |
| 5 | Administrator questions | Which identity/profile grants access and who needs refreshed credentials? |
| 6 | Main objects | users; profiles; UUID/password; expiry; revoke; rotate; clone; tags; vless/trojan URI |
| 7 | Table / list columns | User; profile; server; access/exit; expiry; enabled; revision; subscriptions |
| 8 | Filters | server; user; expiry; tags; refresh required |
| 9 | Search | C01; server; user; expiry; tags; refresh required. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create user and profile (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | QR; copy URI; compatible export; rotate; revoke; clone |
| 15 | Dangerous actions | H; C03. Impact depends on server; Vault; access/routing; subscriptions; RBAC. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Alias; server; credentials generated/ref; access/exit intent; expiry; tags; advanced routing separate warning. |
| 18 | Desired vs observed | C05; configured fields separate from Profile declared is not remotely installed; transport limitations explicit. |
| 19 | Runtime health / status | Profile declared is not remotely installed; transport limitations explicit C05. |
| 20 | Dependencies | server; Vault; access/routing; subscriptions; RBAC |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open server; Vault; access/routing; subscriptions; RBAC through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which identity/profile grants access and who needs refreshed credentials? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Xray Users / Profiles. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Profile declared is not remotely installed; transport limitations explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Authenticated test; profile impact; route explanation. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Alias; server; credentials generated/ref; access/exit intent; expiry; tags; advanced routing separate warning. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for server; Vault; access/routing; subscriptions; RBAC. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Rotating an Xray user's credential identifies every profile and subscription that must refresh. Diagnose: cell 37. |
| 37 | Diagnostics | Authenticated test; profile impact; route explanation; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Give someone access with profile choices; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep users protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `xray-profile` — Profile card linked to proxy listener. Not account identity or subscription. See DESIGN_SYSTEM status grammar. |

<a id="p051"></a>

## P051 — Subscriptions

Source families: subscription xray clientexport. **Editor field groups:** Name; selected profile IDs; expiry; output representation; token generation; enabled.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Subscriptions |
| 2 | URL / route | `/services/subscriptions`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Subscriptions |
| 4 | Purpose | Coherent management/inspection of HAPP; Xray profile membership; token URL; QR; regenerate; export; expiry. |
| 5 | Administrator questions | What profiles does this URL expose and is its token still valid? |
| 6 | Main objects | HAPP; Xray profile membership; token URL; QR; regenerate; export; expiry |
| 7 | Table / list columns | Name; enabled; profiles; compatibility; expiry; revision; credential state |
| 8 | Filters | enabled; expiry; profile/server; format |
| 9 | Search | C01; enabled; expiry; profile/server; format. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create subscription (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Show QR; Copy URL; Export; edit members; revoke token; rotate |
| 15 | Dangerous actions | H; C03. Impact depends on profile revisions; Vault; serving origin/TLS; identity; client compatibility. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; selected profile IDs; expiry; output representation; token generation; enabled. |
| 18 | Desired vs observed | C05; configured fields separate from URL secret never appears in table/query/log; regenerated URL and QR must agree. |
| 19 | Runtime health / status | URL secret never appears in table/query/log; regenerated URL and QR must agree C05. |
| 20 | Dependencies | profile revisions; Vault; serving origin/TLS; identity; client compatibility |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open profile revisions; Vault; serving origin/TLS; identity; client compatibility through related object links. |
| 23 | Explain Why integration | Seed this question/context: What profiles does this URL expose and is its token still valid? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Subscriptions. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for URL secret never appears in table/query/log; regenerated URL and QR must agree. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Fetch validation without exposing payload; cert health; stale member detection. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; selected profile IDs; expiry; output representation; token generation; enabled. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for profile revisions; Vault; serving origin/TLS; identity; client compatibility. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A subscription contains two profiles; Copy URL and Show QR reveal the same current token URL. Diagnose: cell 37. |
| 37 | Diagnostics | Fetch validation without exposing payload; cert health; stale member detection; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Subscription access card; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep HAPP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `subscription` — Profile stack linked to a delivery ticket. Never encode QR or token in icon. See DESIGN_SYSTEM status grammar. |

<a id="p052"></a>

## P052 — Other Tunnels / NetBird

Source families: tunnels capability. **Editor field groups:** Kind; endpoints; authentication; interface; routes; access scope; MTU; controller when needed.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Other Tunnels / NetBird |
| 2 | URL / route | `/tunnels/other`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Tunnels → Other Tunnels / NetBird |
| 4 | Purpose | Coherent management/inspection of IPsec/IKEv2; GRE; GRETAP; IPIP; VTI; GRE over IPsec; L2TP/IPsec; OpenVPN import; NetBird. |
| 5 | Administrator questions | What traffic uses this tunnel and what is its actual protection? |
| 6 | Main objects | IPsec/IKEv2; GRE; GRETAP; IPIP; VTI; GRE over IPsec; L2TP/IPsec; OpenVPN import; NetBird |
| 7 | Table / list columns | Name; kind; endpoints; encryption; routes; peer state; MTU; owner |
| 8 | Filters | kind; encryption; state; interface |
| 9 | Search | C01; kind; encryption; state; interface. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create supported tunnel (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test; import supported profile; access; dependencies; revoke |
| 15 | Dangerous actions | C; C03. Impact depends on protocol runtime; peer credentials; routes; firewall; MTU; optional controller. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Kind; endpoints; authentication; interface; routes; access scope; MTU; controller when needed. |
| 18 | Desired vs observed | C05; configured fields separate from Plain GRE/IPIP not encrypted; NetBird controller optional subsystem not core dependency. |
| 19 | Runtime health / status | Plain GRE/IPIP not encrypted; NetBird controller optional subsystem not core dependency C05. |
| 20 | Dependencies | protocol runtime; peer credentials; routes; firewall; MTU; optional controller |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open protocol runtime; peer credentials; routes; firewall; MTU; optional controller through related object links. |
| 23 | Explain Why integration | Seed this question/context: What traffic uses this tunnel and what is its actual protection? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Other Tunnels / NetBird. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Plain GRE/IPIP not encrypted; NetBird controller optional subsystem not core dependency. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Peer negotiation; route lookup; PMTU; capture. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Kind; endpoints; authentication; interface; routes; access scope; MTU; controller when needed. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for protocol runtime; peer credentials; routes; firewall; MTU; optional controller. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A GRE tunnel is not encrypted: show that explicitly even when the path is operational. Diagnose: cell 37. |
| 37 | Diagnostics | Peer negotiation; route lookup; PMTU; capture; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Simple supported VPN workflows; unavailable explanation; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep IPsec/IKEv2 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `tunnel` — Encapsulated path between endpoints. Plain tunnels need separate encryption status. See DESIGN_SYSTEM status grammar. |

<a id="p053"></a>

## P053 — DNS Resolver / Forwarding

Source families: network sites. **Editor field groups:** Listen scope; upstreams; transport/security; split domain routes; DNSSEC; cache limits.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | DNS Resolver / Forwarding |
| 2 | URL / route | `/dns/resolver`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | DNS / DHCP → DNS Resolver / Forwarding |
| 4 | Purpose | Coherent management/inspection of resolver; forwarding; upstreams; split DNS; DoT; DoH; DNSSEC; cache. |
| 5 | Administrator questions | Why did this client receive this answer and from which resolver? |
| 6 | Main objects | resolver; forwarding; upstreams; split DNS; DoT; DoH; DNSSEC; cache |
| 7 | Table / list columns | Resolver/view; domain; upstream; protocol; validation; latency; cache age; source |
| 8 | Filters | view; client network; domain; response; upstream |
| 9 | Search | C01; view; client network; domain; response; upstream. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure resolver intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Query through resolver; inspect cache; split view; bounded flush |
| 15 | Dangerous actions | H; C03. Impact depends on WAN; time; certificates; listener; LAN; policy. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Listen scope; upstreams; transport/security; split domain routes; DNSSEC; cache limits. |
| 18 | Desired vs observed | C05; configured fields separate from Configured resolver and successful query separated; DNSSEC/time error explicit. |
| 19 | Runtime health / status | Configured resolver and successful query separated; DNSSEC/time error explicit C05. |
| 20 | Dependencies | WAN; time; certificates; listener; LAN; policy |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open WAN; time; certificates; listener; LAN; policy through related object links. |
| 23 | Explain Why integration | Seed this question/context: Why did this client receive this answer and from which resolver? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: DNS Resolver / Forwarding. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Configured resolver and successful query separated; DNSSEC/time error explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Query trace; dependency loop; validation failure; upstream test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Listen scope; upstreams; transport/security; split domain routes; DNSSEC; cache limits. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for WAN; time; certificates; listener; LAN; policy. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Query a hostname through the resolver used by the selected client, not a random external resolver. Diagnose: cell 37. |
| 37 | Diagnostics | Query trace; dependency loop; validation failure; upstream test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | DNS behavior in network/Internet setup; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep resolver protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `dns` — Name glyph resolving to address block. Not DHCP lease allocation. See DESIGN_SYSTEM status grammar. |

<a id="p054"></a>

## P054 — DNS Zones / Records

Source families: network ddns. **Editor field groups:** Zone; name; type; value; TTL; ownership; update policy and secret ref.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | DNS Zones / Records |
| 2 | URL / route | `/dns/zones`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | DNS / DHCP → DNS Zones / Records |
| 4 | Purpose | Coherent management/inspection of authoritative/reverse zones; local records; aliases; DDNS updates. |
| 5 | Administrator questions | Which source owns this answer and where is it authoritative? |
| 6 | Main objects | authoritative/reverse zones; local records; aliases; DDNS updates |
| 7 | Table / list columns | Zone; name; type; value; TTL; owner; serial; update state |
| 8 | Filters | zone; record type; owner; conflict |
| 9 | Search | C01; zone; record type; owner; conflict. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create record/zone (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Query; reverse lookup; inspect consumers; clone |
| 15 | Dangerous actions | H; C03. Impact depends on authoritative runtime; resolver views; address plan; update credentials. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Zone; name; type; value; TTL; ownership; update policy and secret ref. |
| 18 | Desired vs observed | C05; configured fields separate from Local record is not public-zone publication; no implied authority from UI form. |
| 19 | Runtime health / status | Local record is not public-zone publication; no implied authority from UI form C05. |
| 20 | Dependencies | authoritative runtime; resolver views; address plan; update credentials |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open authoritative runtime; resolver views; address plan; update credentials through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which source owns this answer and where is it authoritative? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: DNS Zones / Records. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Local record is not public-zone publication; no implied authority from UI form. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Forward/reverse consistency; zone validation; update log. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Zone; name; type; value; TTL; ownership; update policy and secret ref. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for authoritative runtime; resolver views; address plan; update credentials. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Create nas.home.arpa for a local network and do not imply that it publishes a public DNS record. Diagnose: cell 37. |
| 37 | Diagnostics | Forward/reverse consistency; zone validation; update log; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Friendly local names; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep authoritative/reverse zones protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `dns-record` — Single named record line with value marker. Not certificate. See DESIGN_SYSTEM status grammar. |

<a id="p055"></a>

## P055 — Policy DNS / RPZ

Source families: network multicast. **Editor field groups:** Client/network aliases; domain/category source; action; override; expiry; DNS view.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Policy DNS / RPZ |
| 2 | URL / route | `/dns/policies`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | DNS / DHCP → Policy DNS / RPZ |
| 4 | Purpose | Coherent management/inspection of RPZ; category/domain policy; selective DNS; feeds. |
| 5 | Administrator questions | Was this answer blocked, rewritten or forwarded by policy? |
| 6 | Main objects | RPZ; category/domain policy; selective DNS; feeds |
| 7 | Table / list columns | Policy; client scope; domain/feed; action; answer effect; source; freshness |
| 8 | Filters | scope; action; domain; feed; stale |
| 9 | Search | C01; scope; action; domain; feed; stale. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create DNS policy (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Explain response; test domain; inspect affected clients |
| 15 | Dangerous actions | H; C03. Impact depends on resolver; domain feed; access/routing policy; time. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Client/network aliases; domain/category source; action; override; expiry; DNS view. |
| 18 | Desired vs observed | C05; configured fields separate from DNS category match not proof of application identity; HTTPS bypass limitations visible. |
| 19 | Runtime health / status | DNS category match not proof of application identity; HTTPS bypass limitations visible C05. |
| 20 | Dependencies | resolver; domain feed; access/routing policy; time |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open resolver; domain feed; access/routing policy; time through related object links. |
| 23 | Explain Why integration | Seed this question/context: Was this answer blocked, rewritten or forwarded by policy? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Policy DNS / RPZ. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for DNS category match not proof of application identity; HTTPS bypass limitations visible. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Query with client context; policy trace; feed health. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Client/network aliases; domain/category source; action; override; expiry; DNS view. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for resolver; domain feed; access/routing policy; time. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Explain a blocked DNS answer using the matched client view and policy source. Diagnose: cell 37. |
| 37 | Diagnostics | Query with client context; policy trace; feed health; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Family/guest DNS policy with transparent limitations; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep RPZ protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `dns-policy` — Name lookup entering a policy gate. Not IP route filter. See DESIGN_SYSTEM status grammar. |

<a id="p056"></a>

## P056 — DHCP Scopes / Relay

Source families: network dhcpsnoop nac. **Editor field groups:** Network; range; lease time; gateway/DNS; options; relay targets; Option 82 policy; HA peer if supported.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | DHCP Scopes / Relay |
| 2 | URL / route | `/dhcp/scopes`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | DNS / DHCP → DHCP Scopes / Relay |
| 4 | Purpose | Coherent management/inspection of DHCPv4/v6; scopes; pools; relay; Option 82; HA; DNS options. |
| 5 | Administrator questions | What will this network hand out and is a competing server present? |
| 6 | Main objects | DHCPv4/v6; scopes; pools; relay; Option 82; HA; DNS options |
| 7 | Table / list columns | Scope; network; family; pool; gateway; DNS; leases; server/relay; HA |
| 8 | Filters | network; family; server/relay; pool pressure; conflict |
| 9 | Search | C01; network; family; server/relay; pool pressure; conflict. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create DHCP scope (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Reservations; leases; options; relay trust; test |
| 15 | Dangerous actions | C; C03. Impact depends on network subnet; address plan; DNS; daemon; trusted ports. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Network; range; lease time; gateway/DNS; options; relay targets; Option 82 policy; HA peer if supported. |
| 18 | Desired vs observed | C05; configured fields separate from Pool declared vs server responding; leases don't prove current presence. |
| 19 | Runtime health / status | Pool declared vs server responding; leases don't prove current presence C05. |
| 20 | Dependencies | network subnet; address plan; DNS; daemon; trusted ports |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open network subnet; address plan; DNS; daemon; trusted ports through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will this network hand out and is a competing server present? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: DHCP Scopes / Relay. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Pool declared vs server responding; leases don't prove current presence. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Discovery; pool conflict; rogue server; relay path. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Network; range; lease time; gateway/DNS; options; relay targets; Option 82 policy; HA peer if supported. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for network subnet; address plan; DNS; daemon; trusted ports. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Create DHCP for VLAN30 with a validated pool and DNS options while preserving management. Diagnose: cell 37. |
| 37 | Diagnostics | Discovery; pool conflict; rogue server; relay path; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Combined Create network workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep DHCPv4/v6 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `dhcp` — Address block allocated to an endpoint. Not name resolution. See DESIGN_SYSTEM status grammar. |

<a id="p057"></a>

## P057 — Leases / Reservations

Source families: network devices dhcpsnoop. **Editor field groups:** Client identifier; address; hostname; scope; reservation comment.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Leases / Reservations |
| 2 | URL / route | `/dhcp/leases`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | DNS / DHCP → Leases / Reservations |
| 4 | Purpose | Coherent management/inspection of leases; static reservations; hostnames; DDNS; IP conflicts. |
| 5 | Administrator questions | Who holds this address and can it be reserved safely? |
| 6 | Main objects | leases; static reservations; hostnames; DDNS; IP conflicts |
| 7 | Table / list columns | IP; MAC/DUID; hostname; network; lease start/expiry; reservation; source |
| 8 | Filters | network; hostname; IP/MAC/DUID; expiry; conflict |
| 9 | Search | C01; network; hostname; IP/MAC/DUID; expiry; conflict. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Reserve selected client (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open client; logs; revoke lease with warning; DNS record |
| 15 | Dangerous actions | M; C03. Impact depends on scope; client identity; address plan; update owner. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Client identifier; address; hostname; scope; reservation comment. |
| 18 | Desired vs observed | C05; configured fields separate from Expired lease may still use address; deleting lease is not disconnect or revocation. |
| 19 | Runtime health / status | Expired lease may still use address; deleting lease is not disconnect or revocation C05. |
| 20 | Dependencies | scope; client identity; address plan; update owner |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open scope; client identity; address plan; update owner through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who holds this address and can it be reserved safely? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Leases / Reservations. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Expired lease may still use address; deleting lease is not disconnect or revocation. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Duplicate address; DNS consistency; lease log. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Client identifier; address; hostname; scope; reservation comment. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A lease exists for a laptop that has not been seen recently; do not call it connected. Diagnose: cell 37. |
| 37 | Diagnostics | Duplicate address; DNS consistency; lease log; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Client reservation; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep leases protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `lease` — Endpoint/address pair with time marker. Not authenticated session. See DESIGN_SYSTEM status grammar. |

<a id="p058"></a>

## P058 — Services / Listeners

Source families: svcstate model sites. **Editor field groups:** Enablement; listen address/interface; exposure; credential refs; reload strategy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Services / Listeners |
| 2 | URL / route | `/services`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Services / Listeners |
| 4 | Purpose | Coherent management/inspection of runtime services; dependency-aware start/stop/reload; service state. |
| 5 | Administrator questions | What is listening, what is exposed, and what survives a restart? |
| 6 | Main objects | runtime services; dependency-aware start/stop/reload; service state |
| 7 | Table / list columns | Service; admin; process; listener; health probe; exposure; dependencies; logs |
| 8 | Filters | enabled; listening; failed layer; owner |
| 9 | Search | C01; enabled; listening; failed layer; owner. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect service (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Enable; publish; test; logs; restart; dependency impact |
| 15 | Dangerous actions | H; C03. Impact depends on identity; certificates; ports; firewall; runtime. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Enablement; listen address/interface; exposure; credential refs; reload strategy. |
| 18 | Desired vs observed | C05; configured fields separate from Service process, listener and end-to-end health separate. |
| 19 | Runtime health / status | Service process, listener and end-to-end health separate C05. |
| 20 | Dependencies | identity; certificates; ports; firewall; runtime |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open identity; certificates; ports; firewall; runtime through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is listening, what is exposed, and what survives a restart? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Services / Listeners. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Service process, listener and end-to-end health separate. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Config validation; socket; probe; bounded logs. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Enablement; listen address/interface; exposure; credential refs; reload strategy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Xray's process is running but its listener probe failed: present both facts. Diagnose: cell 37. |
| 37 | Diagnostics | Config validation; socket; probe; bounded logs; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Services with clear exposure scope; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep runtime services protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `service` — Listener socket attached to process block. Not global system gear. See DESIGN_SYSTEM status grammar. |

<a id="p059"></a>

## P059 — Sites / Reverse Proxy

Source families: sites publish acme. **Editor field groups:** Hostname; local target; listener; exposure; TLS ref; redirect; access policy; DDNS.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Sites / Reverse Proxy |
| 2 | URL / route | `/services/sites`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Sites / Reverse Proxy |
| 4 | Purpose | Coherent management/inspection of sites; reverse proxy; local/public services; TLS; DDNS. |
| 5 | Administrator questions | Which hostname reaches which upstream and is access public? |
| 6 | Main objects | sites; reverse proxy; local/public services; TLS; DDNS |
| 7 | Table / list columns | Site; hostname; listener; target; TLS; DNS; exposure; health |
| 8 | Filters | hostname; listener; target; certificate; public/private |
| 9 | Search | C01; hostname; listener; target; certificate; public/private. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create site (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Publish; test internal/external; logs; renew certificate |
| 15 | Dangerous actions | C; C03. Impact depends on DNS; certificate; target return route; firewall/NAT; time. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Hostname; local target; listener; exposure; TLS ref; redirect; access policy; DDNS. |
| 18 | Desired vs observed | C05; configured fields separate from HTTP listener is not reachable from Internet; certificates not auto trusted. |
| 19 | Runtime health / status | HTTP listener is not reachable from Internet; certificates not auto trusted C05. |
| 20 | Dependencies | DNS; certificate; target return route; firewall/NAT; time |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open DNS; certificate; target return route; firewall/NAT; time through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which hostname reaches which upstream and is access public? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Sites / Reverse Proxy. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for HTTP listener is not reachable from Internet; certificates not auto trusted. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Upstream probe; hostname/TLS; external-vantage evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Hostname; local target; listener; exposure; TLS ref; redirect; access policy; DDNS. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for DNS; certificate; target return route; firewall/NAT; time. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Publish a named internal HTTPS service with separate internal upstream and external TLS checks. Diagnose: cell 37. |
| 37 | Diagnostics | Upstream probe; hostname/TLS; external-vantage evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Publish website/service safely; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep sites protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `site` — Hostname tab over an upstream link. Not Internet health. See DESIGN_SYSTEM status grammar. |

<a id="p060"></a>

## P060 — Certificates / ACME

Source families: certs acme intca. **Editor field groups:** Names/SANs; ACME directory; HTTP/DNS challenge; credential ref; import; renewal policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Certificates / ACME |
| 2 | URL / route | `/services/certificates`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Certificates / ACME |
| 4 | Purpose | Coherent management/inspection of certificate manager; import; ACME; renewal; trust chain; TLS dependencies. |
| 5 | Administrator questions | Which services rely on this certificate and why did renewal fail? |
| 6 | Main objects | certificate manager; import; ACME; renewal; trust chain; TLS dependencies |
| 7 | Table / list columns | Name; SANs; issuer; fingerprint; validity; time trust; renewal; consumers |
| 8 | Filters | expiry; source; issuer; service; failed challenge |
| 9 | Search | C01; expiry; source; issuer; service; failed challenge. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Obtain/import certificate (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Renew; test chain; inspect challenge; replace; revoke |
| 15 | Dangerous actions | H; C03. Impact depends on clock; DNS; challenge reachability; provider secret; CA. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Names/SANs; ACME directory; HTTP/DNS challenge; credential ref; import; renewal policy. |
| 18 | Desired vs observed | C05; configured fields separate from Untrusted clock prevents valid/expired certainty; renewal job not issuance proof. |
| 19 | Runtime health / status | Untrusted clock prevents valid/expired certainty; renewal job not issuance proof C05. |
| 20 | Dependencies | clock; DNS; challenge reachability; provider secret; CA |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open clock; DNS; challenge reachability; provider secret; CA through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which services rely on this certificate and why did renewal fail? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Certificates / ACME. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Untrusted clock prevents valid/expired certainty; renewal job not issuance proof. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Challenge DNS/HTTP; rate limit; chain; hostname mismatch. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Names/SANs; ACME directory; HTTP/DNS challenge; credential ref; import; renewal policy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for clock; DNS; challenge reachability; provider secret; CA. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: ACME renewal failed due to DNS challenge evidence; retain the current certificate's actual validity status. Diagnose: cell 37. |
| 37 | Diagnostics | Challenge DNS/HTTP; rate limit; chain; hostname mismatch; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Fix certificate from service context; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep certificate manager protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `certificate` — Document with public verification seal. Not private key. See DESIGN_SYSTEM status grammar. |

<a id="p061"></a>

## P061 — Internal PKI / SSH CA

Source families: intca certs chaos. **Editor field groups:** Purpose; subject; key generation/ref; validity; allowed usages; issuing policy; revocation.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Internal PKI / SSH CA |
| 2 | URL / route | `/services/pki`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Internal PKI / SSH CA |
| 4 | Purpose | Coherent management/inspection of CA; issuance; revocation; internal trust; SSH CA. |
| 5 | Administrator questions | Who trusts this authority and what does revocation affect? |
| 6 | Main objects | CA; issuance; revocation; internal trust; SSH CA |
| 7 | Table / list columns | Authority; purpose; fingerprint; issued count; expiry; revocations; consumers |
| 8 | Filters | purpose; issuer; expired; revoked |
| 9 | Search | C01; purpose; issuer; expired; revoked. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create/import authority (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Issue scoped credential; revoke; inspect trust; export public cert |
| 15 | Dangerous actions | C; C03. Impact depends on Vault; time; trust distribution; policy. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Purpose; subject; key generation/ref; validity; allowed usages; issuing policy; revocation. |
| 18 | Desired vs observed | C05; configured fields separate from CA exists does not mean clients trust it; private key non-export default. |
| 19 | Runtime health / status | CA exists does not mean clients trust it; private key non-export default C05. |
| 20 | Dependencies | Vault; time; trust distribution; policy |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open Vault; time; trust distribution; policy through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who trusts this authority and what does revocation affect? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Internal PKI / SSH CA. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for CA exists does not mean clients trust it; private key non-export default. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Chain validation; revocation distribution; SSH trust evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Purpose; subject; key generation/ref; validity; allowed usages; issuing policy; revocation. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for Vault; time; trust distribution; policy. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Issuing a local certificate does not install trust on a remote client; provide the trust-distribution step. Diagnose: cell 37. |
| 37 | Diagnostics | Chain validation; revocation distribution; SSH trust evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only; trust instructions contextual; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep CA protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `pki` — Authority seal branching to certificates. Not a single certificate. See DESIGN_SYSTEM status grammar. |

<a id="p062"></a>

## P062 — AAA / RADIUS / 802.1X

Source families: radiusd hardware nac. **Editor field groups:** Servers; NAS clients; secret refs; realms; methods; fallback; VLAN assignment; CoA source policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | AAA / RADIUS / 802.1X |
| 2 | URL / route | `/services/aaa`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → AAA / RADIUS / 802.1X |
| 4 | Purpose | Coherent management/inspection of RADIUS; AAA; 802.1X; realms; CoA; clients; NAC. |
| 5 | Administrator questions | Which identity decision assigned this access and is the server reachable? |
| 6 | Main objects | RADIUS; AAA; 802.1X; realms; CoA; clients; NAC |
| 7 | Table / list columns | Server/realm; client; auth method; reachable; reject reason; CoA; network |
| 8 | Filters | realm; NAS; accept/reject; source; time |
| 9 | Search | C01; realm; NAS; accept/reject; source; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure AAA service (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test identity safely; inspect reject; assign network; logs |
| 15 | Dangerous actions | C; C03. Impact depends on Vault; certificates; time; switch/AP support; management fallback. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Servers; NAS clients; secret refs; realms; methods; fallback; VLAN assignment; CoA source policy. |
| 18 | Desired vs observed | C05; configured fields separate from Service responding not successful authorization; test secrets never logged. |
| 19 | Runtime health / status | Service responding not successful authorization; test secrets never logged C05. |
| 20 | Dependencies | Vault; certificates; time; switch/AP support; management fallback |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open Vault; certificates; time; switch/AP support; management fallback through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which identity decision assigned this access and is the server reachable? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: AAA / RADIUS / 802.1X. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Service responding not successful authorization; test secrets never logged. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: RADIUS transaction; EAP/TLS; CoA impact. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Servers; NAS clients; secret refs; realms; methods; fallback; VLAN assignment; CoA source policy. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for Vault; certificates; time; switch/AP support; management fallback. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A RADIUS reject should identify the authorization layer without logging the supplied password. Diagnose: cell 37. |
| 37 | Diagnostics | RADIUS transaction; EAP/TLS; CoA impact; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Enterprise access wizard if supported; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep RADIUS protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `aaa` — Identity passing a decision gate. Not local user list. See DESIGN_SYSTEM status grammar. |

<a id="p063"></a>

## P063 — Time / NTP / NTS / PTP

Source families: ntp ptpd timeintegrity chaos hardware. **Editor field groups:** Sources; authentication; server scope; PTP domain/profile; timestamping mode; bounded manual anchor.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Time / NTP / NTS / PTP |
| 2 | URL / route | `/services/time`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Time / NTP / NTS / PTP |
| 4 | Purpose | Coherent management/inspection of NTP client/server; NTS; PTP; PHC; GPS source; TIME_UNTRUSTED. |
| 5 | Administrator questions | Can security decisions trust time and which source supplies it? |
| 6 | Main objects | NTP client/server; NTS; PTP; PHC; GPS source; TIME_UNTRUSTED |
| 7 | Table / list columns | Source; protocol; selected; offset; delay; authenticated; last sync; PHC capability |
| 8 | Filters | protocol; selected; trust; stale |
| 9 | Search | C01; protocol; selected; trust; stale. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure time sources (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test source; inspect trust; PTP hardware; source failover |
| 15 | Dangerous actions | H; C03. Impact depends on network; trusted source; certificates bootstrap; timestamp hardware. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Sources; authentication; server scope; PTP domain/profile; timestamping mode; bounded manual anchor. |
| 18 | Desired vs observed | C05; configured fields separate from Wall time vs monotonic duration distinct; no greetings/time-based green without trust. |
| 19 | Runtime health / status | Wall time vs monotonic duration distinct; no greetings/time-based green without trust C05. |
| 20 | Dependencies | network; trusted source; certificates bootstrap; timestamp hardware |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open network; trusted source; certificates bootstrap; timestamp hardware through related object links. |
| 23 | Explain Why integration | Seed this question/context: Can security decisions trust time and which source supplies it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Time / NTP / NTS / PTP. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Wall time vs monotonic duration distinct; no greetings/time-based green without trust. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Offset/drift; NTS authentication; PHC availability. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Sources; authentication; server scope; PTP domain/profile; timestamping mode; bounded manual anchor. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for network; trusted source; certificates bootstrap; timestamp hardware. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A node starts with untrusted time; local management remains available while certificate validity is unknown. Diagnose: cell 37. |
| 37 | Diagnostics | Offset/drift; NTS authentication; PHC availability; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Time setup with offline trust warning; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep NTP client/server protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `time` — Clock face with source anchor. Not history timeline. See DESIGN_SYSTEM status grammar. |

<a id="p064"></a>

## P064 — Telemetry / Integrations

Source families: snmp secpolicy chaos logging. **Editor field groups:** Protocol; destination; data scopes; credential refs; retention/queue bound; retry policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Telemetry / Integrations |
| 2 | URL / route | `/services/integrations`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Telemetry / Integrations |
| 4 | Purpose | Coherent management/inspection of SNMPv3; remote syslog; NetFlow/IPFIX/sFlow; webhooks; TR-369/USP optional. |
| 5 | Administrator questions | What leaves this node, to whom, and what if collection fails? |
| 6 | Main objects | SNMPv3; remote syslog; NetFlow/IPFIX/sFlow; webhooks; TR-369/USP optional |
| 7 | Table / list columns | Destination; protocol; data scope; authentication; queue; drops; last success; quota |
| 8 | Filters | protocol; enabled; destination; failure; privacy |
| 9 | Search | C01; protocol; enabled; destination; failure; privacy. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Add integration (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test delivery; inspect payload schema; rotate secret; disable |
| 15 | Dangerous actions | H; C03. Impact depends on destination; Vault; time; firewall; resource budgets. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Protocol; destination; data scopes; credential refs; retention/queue bound; retry policy. |
| 18 | Desired vs observed | C05; configured fields separate from Opt-in; failed sink falls back only to bounded local buffer; no cloud account needed. |
| 19 | Runtime health / status | Opt-in; failed sink falls back only to bounded local buffer; no cloud account needed C05. |
| 20 | Dependencies | destination; Vault; time; firewall; resource budgets |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open destination; Vault; time; firewall; resource budgets through related object links. |
| 23 | Explain Why integration | Seed this question/context: What leaves this node, to whom, and what if collection fails? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Telemetry / Integrations. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Opt-in; failed sink falls back only to bounded local buffer; no cloud account needed. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Connectivity; TLS; queue/drop counters. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Protocol; destination; data scopes; credential refs; retention/queue bound; retry policy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for destination; Vault; time; firewall; resource budgets. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A remote syslog sink fills: alert and use only the small bounded internal fallback. Diagnose: cell 37. |
| 37 | Diagnostics | Connectivity; TLS; queue/drop counters; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Notifications with explicit data disclosure; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep SNMPv3 protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `integration` — Outbound connector with bounded destination. Not mandatory cloud. See DESIGN_SYSTEM status grammar. |

<a id="p065"></a>

## P065 — UPnP / NAT-PMP / PCP / Wake

Source families: upnp devices capability. **Editor field groups:** Allowed networks; protocols; mapping limits; lease cap; WoL target/interface.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | UPnP / NAT-PMP / PCP / Wake |
| 2 | URL / route | `/services/discovery`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → UPnP / NAT-PMP / PCP / Wake |
| 4 | Purpose | Coherent management/inspection of UPnP; NAT-PMP; PCP; Wake-on-LAN. |
| 5 | Administrator questions | Who requested this mapping and when does it expire? |
| 6 | Main objects | UPnP; NAT-PMP; PCP; Wake-on-LAN |
| 7 | Table / list columns | Client; protocol; external/internal tuple; lease; owner; network |
| 8 | Filters | network; client; protocol; expiry |
| 9 | Search | C01; network; client; protocol; expiry. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect dynamic mapping (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Revoke; allow trusted-network protocol; Wake known device |
| 15 | Dangerous actions | H; C03. Impact depends on trusted LAN; NAT/firewall; observed MAC; broadcast capability. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Allowed networks; protocols; mapping limits; lease cap; WoL target/interface. |
| 18 | Desired vs observed | C05; configured fields separate from No WAN enable by default; Wake request not proof of boot; WoL API gap explicit. |
| 19 | Runtime health / status | No WAN enable by default; Wake request not proof of boot; WoL API gap explicit C05. |
| 20 | Dependencies | trusted LAN; NAT/firewall; observed MAC; broadcast capability |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open trusted LAN; NAT/firewall; observed MAC; broadcast capability through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who requested this mapping and when does it expire? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: UPnP / NAT-PMP / PCP / Wake. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No WAN enable by default; Wake request not proof of boot; WoL API gap explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Mapping test; client request log; expiry. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Allowed networks; protocols; mapping limits; lease cap; WoL target/interface. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for trusted LAN; NAT/firewall; observed MAC; broadcast capability. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A UPnP mapping expires for a trusted client; show requester, target and lease rather than an anonymous open port. Diagnose: cell 37. |
| 37 | Diagnostics | Mapping test; client request log; expiry; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Device wake and requested service access where supported; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep UPnP protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `discovery-service` — Local request opening a leased mapping. Label UPnP/Wake separately. See DESIGN_SYSTEM status grammar. |

<a id="p066"></a>

## P066 — Portal

Source families: nac accesslifecycle sites. **Editor field groups:** Network; local identity method; preauth destinations; postauth policy; session limit; TLS.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Portal |
| 2 | URL / route | `/services/portal`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Services → Portal |
| 4 | Purpose | Coherent management/inspection of captive portal; identity/trust entry; local-first portal architecture. |
| 5 | Administrator questions | What access is granted before and after sign-in? |
| 6 | Main objects | captive portal; identity/trust entry; local-first portal architecture |
| 7 | Table / list columns | Portal; network; listener; identity method; policy; session; expiry |
| 8 | Filters | network; identity; active/expired |
| 9 | Search | C01; network; identity; active/expired. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Configure supported portal (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test unauthenticated/authenticated path; revoke session |
| 15 | Dangerous actions | C; C03. Impact depends on NAC; DNS; certificates; firewall; local identity. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Network; local identity method; preauth destinations; postauth policy; session limit; TLS. |
| 18 | Desired vs observed | C05; configured fields separate from No captive-portal runtime found in public API; unavailable not pretend portal. |
| 19 | Runtime health / status | No captive-portal runtime found in public API; unavailable not pretend portal C05. |
| 20 | Dependencies | NAC; DNS; certificates; firewall; local identity |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open NAC; DNS; certificates; firewall; local identity through related object links. |
| 23 | Explain Why integration | Seed this question/context: What access is granted before and after sign-in? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Portal. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No captive-portal runtime found in public API; unavailable not pretend portal. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Redirect; TLS; access boundary; session expiry. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Network; local identity method; preauth destinations; postauth policy; session limit; TLS. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for NAC; DNS; certificates; firewall; local identity. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Guest access before portal authentication is a separate explicit policy from access after authentication. Diagnose: cell 37. |
| 37 | Diagnostics | Redirect; TLS; access boundary; session expiry; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guest access workflow only when implemented; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep captive portal protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `portal` — Bounded entry door with identity marker. Not cloud login. See DESIGN_SYSTEM status grammar. |

<a id="p067"></a>

## P067 — QoS / Traffic Scheduling

Source families: qos qos2. **Editor field groups:** Interface/direction; link bandwidth; algorithm; classes/rates; DSCP classifier; tenant scope.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | QoS / Traffic Scheduling |
| 2 | URL / route | `/routing/qos`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Routing → QoS / Traffic Scheduling |
| 4 | Purpose | Coherent management/inspection of CAKE; fq_codel; HTB; HFSC; shaping; policing; DSCP; hierarchy; per-VRF/tenant. |
| 5 | Administrator questions | Which queue constrains this traffic and is congestion controlled? |
| 6 | Main objects | CAKE; fq_codel; HTB; HFSC; shaping; policing; DSCP; hierarchy; per-VRF/tenant |
| 7 | Table / list columns | Policy; interface/direction; qdisc; rate; class; DSCP; backlog; drops; owner |
| 8 | Filters | interface; direction; algorithm; VRF; class |
| 9 | Search | C01; interface; direction; algorithm; VRF; class. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create QoS intent (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Explain classifier; test latency under load; hierarchy; rollback |
| 15 | Dangerous actions | H; C03. Impact depends on interface speed; tc capability; VRF; resource budget. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Interface/direction; link bandwidth; algorithm; classes/rates; DSCP classifier; tenant scope. |
| 18 | Desired vs observed | C05; configured fields separate from Rendered tc is not applied; bandwidth cap and measured throughput distinct. |
| 19 | Runtime health / status | Rendered tc is not applied; bandwidth cap and measured throughput distinct C05. |
| 20 | Dependencies | interface speed; tc capability; VRF; resource budget |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open interface speed; tc capability; VRF; resource budget through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which queue constrains this traffic and is congestion controlled? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: QoS / Traffic Scheduling. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Rendered tc is not applied; bandwidth cap and measured throughput distinct. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Queue stats; drops; Speed Lab with traffic consent. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Interface/direction; link bandwidth; algorithm; classes/rates; DSCP classifier; tenant scope. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A call-priority queue is configured: observe actual queue drops before claiming it fixed latency. Diagnose: cell 37. |
| 37 | Diagnostics | Queue stats; drops; Speed Lab with traffic consent; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Prioritize calls / control bandwidth; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep CAKE protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `qos` — Several queues ordered through scheduler. Not SD-WAN path selection. See DESIGN_SYSTEM status grammar. |

<a id="p068"></a>

## P068 — Users / Roles

Source families: identity accesslifecycle. **Editor field groups:** Username; role; expiry; enablement; password secure input; recovery requirements.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Users / Roles |
| 2 | URL / route | `/system/users`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Users / Roles |
| 4 | Purpose | Coherent management/inspection of local owners/admin/operators/viewers; RBAC; accounts; password lifecycle. |
| 5 | Administrator questions | Who can administer the node and what can they do? |
| 6 | Main objects | local owners/admin/operators/viewers; RBAC; accounts; password lifecycle |
| 7 | Table / list columns | User; role; enabled; expiry; last activity; sessions |
| 8 | Filters | role; enabled; temporary; expiry |
| 9 | Search | C01; role; enabled; temporary; expiry. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create user (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Change role; disable; sessions; audit; password reset |
| 15 | Dangerous actions | C; C03. Impact depends on local identity; last-owner guard; permissions; audit. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Username; role; expiry; enablement; password secure input; recovery requirements. |
| 18 | Desired vs observed | C05; configured fields separate from UI hiding is not authorization; server enforces; no account existence leak at login. |
| 19 | Runtime health / status | UI hiding is not authorization; server enforces; no account existence leak at login C05. |
| 20 | Dependencies | local identity; last-owner guard; permissions; audit |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open local identity; last-owner guard; permissions; audit through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who can administer the node and what can they do? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Users / Roles. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for UI hiding is not authorization; server enforces; no account existence leak at login. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Permission preview; session audit. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Username; role; expiry; enablement; password secure input; recovery requirements. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for local identity; last-owner guard; permissions; audit. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Disabling the last usable Owner must be rejected rather than merely hidden in the form. Diagnose: cell 37. |
| 37 | Diagnostics | Permission preview; session audit; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | People who manage this node; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep local owners/admin/operators/viewers protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `user` — Person silhouette with administration tag. Not client device. See DESIGN_SYSTEM status grammar. |

<a id="p069"></a>

## P069 — Sessions / Temporary Access

Source families: identity accesslifecycle recovery. **Editor field groups:** Subject; minimal scope; duration; reason; revocation target.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Sessions / Temporary Access |
| 2 | URL / route | `/system/sessions`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Sessions / Temporary Access |
| 4 | Purpose | Coherent management/inspection of sessions; temporary admin; emergency access; revocation. |
| 5 | Administrator questions | Which access is active, how long, and can I revoke it? |
| 6 | Main objects | sessions; temporary admin; emergency access; revocation |
| 7 | Table / list columns | Subject; role/scope; source; created; expiry; last seen; current session |
| 8 | Filters | user; role; temporary; expired; source |
| 9 | Search | C01; user; role; temporary; expired; source. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect session (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Revoke selected; issue bounded temporary access; audit |
| 15 | Dangerous actions | H; C03. Impact depends on identity; management access; trusted expiry; audit. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Subject; minimal scope; duration; reason; revocation target. |
| 18 | Desired vs observed | C05; configured fields separate from Current session marked; expiry not trustworthy from browser clock alone. |
| 19 | Runtime health / status | Current session marked; expiry not trustworthy from browser clock alone C05. |
| 20 | Dependencies | identity; management access; trusted expiry; audit |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open identity; management access; trusted expiry; audit through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which access is active, how long, and can I revoke it? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Sessions / Temporary Access. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Current session marked; expiry not trustworthy from browser clock alone. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Auth failures; access expiry; emergency cleanup. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Subject; minimal scope; duration; reason; revocation target. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for identity; management access; trusted expiry; audit. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Revoke a temporary administrator session and show which session is the current one. Diagnose: cell 37. |
| 37 | Diagnostics | Auth failures; access expiry; emergency cleanup; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Revoke access clearly; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep sessions protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `session` — Identity badge with expiry marker. Not network conntrack. See DESIGN_SYSTEM status grammar. |

<a id="p070"></a>

## P070 — API Tokens / Service Accounts / SSH Keys

Source families: intca identity chaos. **Editor field groups:** Name; principal; scopes; expiry; public key upload; rotation grace; secure one-time reveal.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | API Tokens / Service Accounts / SSH Keys |
| 2 | URL / route | `/system/credentials`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → API Tokens / Service Accounts / SSH Keys |
| 4 | Purpose | Coherent management/inspection of API tokens; service accounts; SSH keys; host key rotation; clone protection. |
| 5 | Administrator questions | Which automation credentials exist and what can they reach? |
| 6 | Main objects | API tokens; service accounts; SSH keys; host key rotation; clone protection |
| 7 | Table / list columns | Name; kind; scope; fingerprint/key ID; expiry; last used; owner |
| 8 | Filters | kind; scope; expired; service account |
| 9 | Search | C01; kind; scope; expired; service account. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create scoped credential (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Reveal once where supported; rotate; revoke; public fingerprint |
| 15 | Dangerous actions | C; C03. Impact depends on identity; Vault; trust clients; audit. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name; principal; scopes; expiry; public key upload; rotation grace; secure one-time reveal. |
| 18 | Desired vs observed | C05; configured fields separate from Raw secrets excluded from tables/history/search; SSH host change affects trust. |
| 19 | Runtime health / status | Raw secrets excluded from tables/history/search; SSH host change affects trust C05. |
| 20 | Dependencies | identity; Vault; trust clients; audit |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open identity; Vault; trust clients; audit through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which automation credentials exist and what can they reach? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: API Tokens / Service Accounts / SSH Keys. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Raw secrets excluded from tables/history/search; SSH host change affects trust. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Scope test; key trust; clone identity check. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name; principal; scopes; expiry; public key upload; rotation grace; secure one-time reveal. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for identity; Vault; trust clients; audit. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Issue a token for observation only, display it once if supported, then retain only metadata. Diagnose: cell 37. |
| 37 | Diagnostics | Scope test; key trust; clone identity check; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Full only, generated access explained; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep API tokens protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `key` — Key shaft with distinct teeth. Not certificate seal. See DESIGN_SYSTEM status grammar. |

<a id="p071"></a>

## P071 — Vault

Source families: vault agent. **Editor field groups:** Secret kind; secure value entry; ownership; rotation; recovery policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Vault |
| 2 | URL / route | `/system/vault`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Vault |
| 4 | Purpose | Coherent management/inspection of secret references; key IDs; encrypted storage; rotation/revoke; recovery. |
| 5 | Administrator questions | Which services depend on this secret and can it be recovered safely? |
| 6 | Main objects | secret references; key IDs; encrypted storage; rotation/revoke; recovery |
| 7 | Table / list columns | Reference; type; key ID; revision; revoked; consumers; recovery state |
| 8 | Filters | kind; key ID; referenced; revoked |
| 9 | Search | C01; kind; key ID; referenced; revoked. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect secret metadata (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Rotate; revoke after dependency preview; recovery/export separately authorized |
| 15 | Dangerous actions | C; C03. Impact depends on master key; private storage; identity; backup. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Secret kind; secure value entry; ownership; rotation; recovery policy. |
| 18 | Desired vs observed | C05; configured fields separate from Never list secret plaintext; metadata availability distinct from ability to decrypt. |
| 19 | Runtime health / status | Never list secret plaintext; metadata availability distinct from ability to decrypt C05. |
| 20 | Dependencies | master key; private storage; identity; backup |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open master key; private storage; identity; backup through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which services depend on this secret and can it be recovered safely? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Vault. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Never list secret plaintext; metadata availability distinct from ability to decrypt. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Vault access check without values; key loss diagnosis. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Secret kind; secure value entry; ownership; rotation; recovery policy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for master key; private storage; identity; backup. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A secret reference is revoked: show affected services without exposing the secret. Diagnose: cell 37. |
| 37 | Diagnostics | Vault access check without values; key loss diagnosis; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Secret required instructions contextual only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep secret references protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `vault` — Closed compartment with key slot. Never imply secret revealed. See DESIGN_SYSTEM status grammar. |

<a id="p072"></a>

## P072 — Updates

Source families: upgrade integrity storagewatch timeintegrity. **Editor field groups:** Channel; local .duck upload; activation window; prerequisites; acknowledge reboot; rollback selection.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Updates |
| 2 | URL / route | `/system/updates`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Updates |
| 4 | Purpose | Coherent management/inspection of Stable/Preview/Nightly; signed .duck; OTA; offline package; Active/Previous/Candidate; SBOM. |
| 5 | Administrator questions | What will upgrade change and can I return to the previous version? |
| 6 | Main objects | Stable/Preview/Nightly; signed .duck; OTA; offline package; Active/Previous/Candidate; SBOM |
| 7 | Table / list columns | Slot; version; architecture; signature; health; size; channel; rollback |
| 8 | Filters | slot; channel; compatibility; result |
| 9 | Search | C01; slot; channel; compatibility; result. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Check/import update (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Release notes; verify signature; stage; activate; rollback; integrity |
| 15 | Dangerous actions | C; C03. Impact depends on trusted signing keys; free space; compatible schema; health checks; recovery. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Channel; local .duck upload; activation window; prerequisites; acknowledge reboot; rollback selection. |
| 18 | Desired vs observed | C05; configured fields separate from Downloaded/staged/active/verified separate; offline no-cloud fully usable; no apt UI. |
| 19 | Runtime health / status | Downloaded/staged/active/verified separate; offline no-cloud fully usable; no apt UI C05. |
| 20 | Dependencies | trusted signing keys; free space; compatible schema; health checks; recovery |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open trusted signing keys; free space; compatible schema; health checks; recovery through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will upgrade change and can I return to the previous version? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Updates. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Downloaded/staged/active/verified separate; offline no-cloud fully usable; no apt UI. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Signature; compatibility; storage; health rollback. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Channel; local .duck upload; activation window; prerequisites; acknowledge reboot; rollback selection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for trusted signing keys; free space; compatible schema; health checks; recovery. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A .duck package is staged but not active; show Active and Previous separately before activation. Diagnose: cell 37. |
| 37 | Diagnostics | Signature; compatibility; storage; health rollback; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided safe update; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep Stable/Preview/Nightly protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `update` — Version stack with forward activation arrow. Not backup download. See DESIGN_SYSTEM status grammar. |

<a id="p073"></a>

## P073 — Backup / Restore

Source families: backup vault upgrade. **Editor field groups:** Sections; include secrets consent; encryption/recovery credential; upload; mappings; diff; verification.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Backup / Restore |
| 2 | URL / route | `/system/backups`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Backup / Restore |
| 4 | Purpose | Coherent management/inspection of portable backup; encrypted secrets; selective sections; drag/drop restore; schema migration. |
| 5 | Administrator questions | What is inside this backup and what changes on this node? |
| 6 | Main objects | portable backup; encrypted secrets; selective sections; drag/drop restore; schema migration |
| 7 | Table / list columns | Backup; version/schema; source hardware; sections; secrets; integrity; size |
| 8 | Filters | type; age; schema; secrets included |
| 9 | Search | C01; type; age; schema; secrets included. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create portable backup (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Download; inspect upload; select sections; compare; restore candidate |
| 15 | Dangerous actions | C; C03. Impact depends on Vault recovery; schema; storage; port mapping; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Sections; include secrets consent; encryption/recovery credential; upload; mappings; diff; verification. |
| 18 | Desired vs observed | C05; configured fields separate from Digest valid not trusted origin; encrypted backup key separate from file; restore not blind replace. |
| 19 | Runtime health / status | Digest valid not trusted origin; encrypted backup key separate from file; restore not blind replace C05. |
| 20 | Dependencies | Vault recovery; schema; storage; port mapping; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open Vault recovery; schema; storage; port mapping; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is inside this backup and what changes on this node? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Backup / Restore. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Digest valid not trusted origin; encrypted backup key separate from file; restore not blind replace. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Integrity; schema migration; connectivity; secret recovery. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Sections; include secrets consent; encryption/recovery credential; upload; mappings; diff; verification. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for Vault recovery; schema; storage; port mapping; management. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Inspect whether a portable backup contains secrets and whether the recovery credential is available. Diagnose: cell 37. |
| 37 | Diagnostics | Integrity; schema migration; connectivity; secret recovery; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | One-file replacement workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep portable backup protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `backup` — Archive tray with recoverable copy. Not update activation. See DESIGN_SYSTEM status grammar. |

<a id="p074"></a>

## P074 — Hardware Replacement / Port Mapping

Source families: backup install network hardware. **Editor field groups:** Old/new bindings; preserve management path; alias; accepted mapping; verification endpoints.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Hardware Replacement / Port Mapping |
| 2 | URL / route | `/system/restore/mapping`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Hardware Replacement / Port Mapping |
| 4 | Purpose | Coherent management/inspection of old-to-new NIC role mapping; MAC/link/speed/driver; identify physical port. |
| 5 | Administrator questions | Which new port replaces each old logical role without losing access? |
| 6 | Main objects | old-to-new NIC role mapping; MAC/link/speed/driver; identify physical port |
| 7 | Table / list columns | Logical role; old name/MAC; new port; speed/driver; evidence; conflicts; management |
| 8 | Filters | unmapped; role; link; driver; physical/virtual |
| 9 | Search | C01; unmapped; role; link; driver; physical/virtual. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Map logical interfaces (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Blink port if supported; inspect dependencies; preview; test |
| 15 | Dangerous actions | C; C03. Impact depends on backup manifest; detected NICs; enrollment; route safety. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Old/new bindings; preserve management path; alias; accepted mapping; verification endpoints. |
| 18 | Desired vs observed | C05; configured fields separate from Never bind by eth0 string alone; auto suggestions require confirmation; no missing-LAN fault on VPS. |
| 19 | Runtime health / status | Never bind by eth0 string alone; auto suggestions require confirmation; no missing-LAN fault on VPS C05. |
| 20 | Dependencies | backup manifest; detected NICs; enrollment; route safety |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open backup manifest; detected NICs; enrollment; route safety through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which new port replaces each old logical role without losing access? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Hardware Replacement / Port Mapping. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Never bind by eth0 string alone; auto suggestions require confirmation; no missing-LAN fault on VPS. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Link identify; source path; connectivity after restore. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Old/new bindings; preserve management path; alias; accepted mapping; verification endpoints. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Map old WAN role to a different NIC using MAC/driver/link evidence; do not blindly bind to eth0. Diagnose: cell 37. |
| 37 | Diagnostics | Link identify; source path; connectivity after restore; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided replacement steps; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep old-to-new NIC role mapping protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `mapping` — Old/new port columns with explicit links. Not automatic hardware replacement. See DESIGN_SYSTEM status grammar. |

<a id="p075"></a>

## P075 — Hardware / Storage / Power

Source families: hardware integrity capability ups. **Editor field groups:** Capability-specific supported settings; PoE budget/watchdog; UPS shutdown policy; storage thresholds.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Hardware / Storage / Power |
| 2 | URL / route | `/system/hardware`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Hardware / Storage / Power |
| 4 | Purpose | Coherent management/inspection of capabilities; SFP; PoE/watchdog; USB; GPS; Bluetooth; crypto; virtualization; sensors; SMART/NVMe; RAID; UPS. |
| 5 | Administrator questions | What hardware is present, limited or failing and what is the impact? |
| 6 | Main objects | capabilities; SFP; PoE/watchdog; USB; GPS; Bluetooth; crypto; virtualization; sensors; SMART/NVMe; RAID; UPS |
| 7 | Table / list columns | Component; identity; capability; evidence source; health; temperature/power; last seen |
| 8 | Filters | type; capability; pending; health; driver |
| 9 | Search | C01; type; capability; pending; health; driver. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect hardware (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Port details; power cycle with scope; UPS test; storage health |
| 15 | Dangerous actions | H; C03. Impact depends on drivers; sensors; permissions; power supply; storage. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Capability-specific supported settings; PoE budget/watchdog; UPS shutdown policy; storage thresholds. |
| 18 | Desired vs observed | C05; configured fields separate from No sensor reading is not healthy; PoE power-off interrupts dependent devices. |
| 19 | Runtime health / status | No sensor reading is not healthy; PoE power-off interrupts dependent devices C05. |
| 20 | Dependencies | drivers; sensors; permissions; power supply; storage |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open drivers; sensors; permissions; power supply; storage through related object links. |
| 23 | Explain Why integration | Seed this question/context: What hardware is present, limited or failing and what is the impact? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Hardware / Storage / Power. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No sensor reading is not healthy; PoE power-off interrupts dependent devices. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: SMART/NVMe; transceiver; UPS; firmware; bounded watchdog. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Capability-specific supported settings; PoE budget/watchdog; UPS shutdown policy; storage thresholds. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A missing temperature sensor is Unknown, not 0 degrees and not automatically an overheating incident. Diagnose: cell 37. |
| 37 | Diagnostics | SMART/NVMe; transceiver; UPS; firmware; bounded watchdog; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Hardware issue and practical next step; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep capabilities protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `hardware` — Chassis/board with identifiable port edge. Not System abstract workspace. See DESIGN_SYSTEM status grammar. |

<a id="p076"></a>

## P076 — Security Posture / Integrity

Source families: secpolicy integrity chaos capability. **Editor field groups:** Policy controls; scoped exceptions; expiry/reason; privacy; integrity trust settings.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Security Posture / Integrity |
| 2 | URL / route | `/system/security`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Security Posture / Integrity |
| 4 | Purpose | Coherent management/inspection of guardrails; readiness; TPM; Secure Boot; SBOM; kernel modules; firmware; override. |
| 5 | Administrator questions | Which protection is active, unavailable or deliberately overridden? |
| 6 | Main objects | guardrails; readiness; TPM; Secure Boot; SBOM; kernel modules; firmware; override |
| 7 | Table / list columns | Finding; control; desired; observed; reason; evidence; override expiry |
| 8 | Filters | severity; control; capability; override; freshness |
| 9 | Search | C01; severity; control; capability; override; freshness. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect security finding (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Evidence; remediation preview; scoped override; SBOM |
| 15 | Dangerous actions | C; C03. Impact depends on runtime probes; trusted keys; TPM/firmware; audit; management. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Policy controls; scoped exceptions; expiry/reason; privacy; integrity trust settings. |
| 18 | Desired vs observed | C05; configured fields separate from Unsupported unused security feature not global incident; local report not attestation. |
| 19 | Runtime health / status | Unsupported unused security feature not global incident; local report not attestation C05. |
| 20 | Dependencies | runtime probes; trusted keys; TPM/firmware; audit; management |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open runtime probes; trusted keys; TPM/firmware; audit; management through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which protection is active, unavailable or deliberately overridden? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Security Posture / Integrity. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Unsupported unused security feature not global incident; local report not attestation. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Signature; trust chain; required modules; policy violation. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Policy controls; scoped exceptions; expiry/reason; privacy; integrity trust settings. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Secure Boot support is absent on unused hardware; explain the constraint without calling routing broken. Diagnose: cell 37. |
| 37 | Diagnostics | Signature; trust chain; required modules; policy violation; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Security summary in plain language; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep guardrails protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `security` — Control checklist with verified/unknown marks. Not firewall packet gate. See DESIGN_SYSTEM status grammar. |

<a id="p077"></a>

## P077 — Maintenance / Scheduler

Source families: maintenance scheduler canary secpolicy. **Editor field groups:** Action kind; scope; schedule/timezone; duration; suppression categories; reason; resource limits.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Maintenance / Scheduler |
| 2 | URL / route | `/system/maintenance`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Maintenance / Scheduler |
| 4 | Purpose | Coherent management/inspection of maintenance window; scheduled jobs; bounded suppressions; canary; quotas. |
| 5 | Administrator questions | What will run when and what will still alert during maintenance? |
| 6 | Main objects | maintenance window; scheduled jobs; bounded suppressions; canary; quotas |
| 7 | Table / list columns | Job/window; action; scope; next run; time trust; last result; suppression expiry |
| 8 | Filters | kind; scope; enabled; failed; time |
| 9 | Search | C01; kind; scope; enabled; failed; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create maintenance/job (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Preview effect; disable; run authorized action; audit |
| 15 | Dangerous actions | H; C03. Impact depends on clock; permitted actions; transaction; quotas; recovery. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Action kind; scope; schedule/timezone; duration; suppression categories; reason; resource limits. |
| 18 | Desired vs observed | C05; configured fields separate from Maintenance suppresses notifications, not protected audit or safety checks. |
| 19 | Runtime health / status | Maintenance suppresses notifications, not protected audit or safety checks C05. |
| 20 | Dependencies | clock; permitted actions; transaction; quotas; recovery |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open clock; permitted actions; transaction; quotas; recovery through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will run when and what will still alert during maintenance? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Maintenance / Scheduler. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Maintenance suppresses notifications, not protected audit or safety checks. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Time trust; missed schedule; expiry restoration. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Action kind; scope; schedule/timezone; duration; suppression categories; reason; resource limits. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A maintenance window suppresses selected notifications until expiry, not audit or rollback protection. Diagnose: cell 37. |
| 37 | Diagnostics | Time trust; missed schedule; expiry restoration; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Schedule update/backup with consequences; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep maintenance window protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `schedule` — Calendar cell with bounded duration. Not raw wall-clock time. See DESIGN_SYSTEM status grammar. |

<a id="p078"></a>

## P078 — Recovery / Reboot / Reset

Source families: recovery powercrash canary secpolicy. **Editor field groups:** Reset scope; keep management/identity options; backup acknowledgement; node-name confirmation; shutdown plan.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Recovery / Reboot / Reset |
| 2 | URL / route | `/system/recovery`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Recovery / Reboot / Reset |
| 4 | Purpose | Coherent management/inspection of emergency mode; reboot; shutdown; factory reset; decommission; crash recovery. |
| 5 | Administrator questions | What remains recoverable and exactly what will be erased or interrupted? |
| 6 | Main objects | emergency mode; reboot; shutdown; factory reset; decommission; crash recovery |
| 7 | Table / list columns | Recovery item; current state; affected resources; evidence; last known good; action |
| 8 | Filters | reason; subsystem; pending; severity |
| 9 | Search | C01; reason; subsystem; pending; severity. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect recovery options (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Export backup; reboot; restore known good; reset scoped sections; decommission |
| 15 | Dangerous actions | C; C03. Impact depends on management path; durable journal; backup; storage; power. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Reset scope; keep management/identity options; backup acknowledgement; node-name confirmation; shutdown plan. |
| 18 | Desired vs observed | C05; configured fields separate from Do not promise automatic rollback for physical poweroff or irreversible reset. |
| 19 | Runtime health / status | Do not promise automatic rollback for physical poweroff or irreversible reset C05. |
| 20 | Dependencies | management path; durable journal; backup; storage; power |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open management path; durable journal; backup; storage; power through related object links. |
| 23 | Explain Why integration | Seed this question/context: What remains recoverable and exactly what will be erased or interrupted? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Recovery / Reboot / Reset. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Do not promise automatic rollback for physical poweroff or irreversible reset. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Journal; boot gate; previous slot; connectivity; support evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Reset scope; keep management/identity options; backup acknowledgement; node-name confirmation; shutdown plan. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for management path; durable journal; backup; storage; power. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A factory reset preview lists exactly what will be lost and whether management identity is preserved. Diagnose: cell 37. |
| 37 | Diagnostics | Journal; boot gate; previous slot; connectivity; support evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Emergency recovery task list; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep emergency mode protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `recovery` — Lifeline returning to a known revision. Not an always-safe undo. See DESIGN_SYSTEM status grammar. |

<a id="p079"></a>

## P079 — Preferences / Local-first

Source families: localfirst chaos firstrun. **Editor field groups:** Locale; theme; density; motion; timezone display; opt-in telemetry; favorites.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Preferences / Local-first |
| 2 | URL / route | `/system/preferences`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Preferences / Local-first |
| 4 | Purpose | Coherent management/inspection of EN/RU/DE/ES; Light/Dark/System; Easy/Full; density; offline; telemetry privacy. |
| 5 | Administrator questions | How do I adapt this workspace without changing network behavior? |
| 6 | Main objects | EN/RU/DE/ES; Light/Dark/System; Easy/Full; density; offline; telemetry privacy |
| 7 | Table / list columns | Preference; account/device scope; current choice; effect |
| 8 | Filters | scope; category |
| 9 | Search | C01; scope; category. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Save preferences (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Preview language/theme; reset view preferences; export no secrets |
| 15 | Dangerous actions | L; C03. Impact depends on account preference store; browser preferences fallback. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | Preferences/auth entry do not change network config; linked mutations use C04. |
| 18 | Desired vs observed | C05; configured fields separate from No mode change rewrites network; offline mode not automatically system failure. |
| 19 | Runtime health / status | No mode change rewrites network; offline mode not automatically system failure C05. |
| 20 | Dependencies | account preference store; browser preferences fallback |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open account preference store; browser preferences fallback through related object links. |
| 23 | Explain Why integration | Seed this question/context: How do I adapt this workspace without changing network behavior? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Preferences / Local-first. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No mode change rewrites network; offline mode not automatically system failure. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Translation coverage; connectivity declaration. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Locale; theme; density; motion; timezone display; opt-in telemetry; favorites. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Switch from Russian Dark Full to German Light Easy without emitting any network mutation. Diagnose: cell 37. |
| 37 | Diagnostics | Translation coverage; connectivity declaration; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Appearance/language and mode choice; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep EN/RU/DE/ES protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `preferences` — Three aligned labeled sliders. Reserve generic controls for preferences. See DESIGN_SYSTEM status grammar. |

<a id="p080"></a>

## P080 — First Run / Local Registration

Source families: firstrun install identity management. **Editor field groups:** Locale; theme; owner; role; NIC mapping; WAN/LAN intent; DNS; management; Easy/Full.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | First Run / Local Registration |
| 2 | URL / route | `/setup`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → First Run / Local Registration |
| 4 | Purpose | Coherent management/inspection of first owner; language/theme; Router/Server/Custom; NIC safety; MAC clone; real tests. |
| 5 | Administrator questions | How can I establish safe local administration and connectivity? |
| 6 | Main objects | first owner; language/theme; Router/Server/Custom; NIC safety; MAC clone; real tests |
| 7 | Table / list columns | Step; proposed object; evidence; unresolved dependency; risk |
| 8 | Filters | step status; affected interface |
| 9 | Search | C01; step status; affected interface. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Review and configure (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Back without loss; Help; choose offline setup; inspect ports |
| 15 | Dangerous actions | C; C03. Impact depends on local enrollment; current management path; interface detection; time trust. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Locale; theme; owner; role; NIC mapping; WAN/LAN intent; DNS; management; Easy/Full. |
| 18 | Desired vs observed | C05; configured fields separate from Local account only; no required cloud registration; real stage results not percentage. |
| 19 | Runtime health / status | Local account only; no required cloud registration; real stage results not percentage C05. |
| 20 | Dependencies | local enrollment; current management path; interface detection; time trust |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open local enrollment; current management path; interface detection; time trust through related object links. |
| 23 | Explain Why integration | Seed this question/context: How can I establish safe local administration and connectivity? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: First Run / Local Registration. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Local account only; no required cloud registration; real stage results not percentage. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Link/address/gateway/DNS/Internet; safe management; trusted time. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Locale; theme; owner; role; NIC mapping; WAN/LAN intent; DNS; management; Easy/Full. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | C14 + secret contract; credential refs for local enrollment; current management path; interface detection; time trust. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Finish local setup on a single-NIC server without Internet; report local readiness and unverified Internet. Diagnose: cell 37. |
| 37 | Diagnostics | Link/address/gateway/DNS/Internet; safe management; trusted time; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary first-run workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep first owner protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `setup` — Ordered steps entering a local node. Not a marketing onboarding rocket. See DESIGN_SYSTEM status grammar. |

<a id="p081"></a>

## P081 — Login / Locked Session

Source families: identity. **Editor field groups:** Username; password; supported factor only; locale/theme; no third-party tracking.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Login / Locked Session |
| 2 | URL / route | `/login`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Login / Locked Session |
| 4 | Purpose | Coherent management/inspection of authenticated entry; session expired; neutral mascot; local-first login. |
| 5 | Administrator questions | Can I securely sign in to this node? |
| 6 | Main objects | authenticated entry; session expired; neutral mascot; local-first login |
| 7 | Table / list columns | No inventory before authentication; node identity only if safe to disclose |
| 8 | Filters | none |
| 9 | Search | C01; none. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Sign in (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Language; theme; local recovery instructions |
| 15 | Dangerous actions | L; C03. Impact depends on local identity; TLS; time; session. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | Preferences/auth entry do not change network config; linked mutations use C04. |
| 18 | Desired vs observed | C05; configured fields separate from Uniform failure text/timing; animation based only on focus/input type, never valid username. |
| 19 | Runtime health / status | Uniform failure text/timing; animation based only on focus/input type, never valid username C05. |
| 20 | Dependencies | local identity; TLS; time; session |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open local identity; TLS; time; session through related object links. |
| 23 | Explain Why integration | Seed this question/context: Can I securely sign in to this node? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Login / Locked Session. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Uniform failure text/timing; animation based only on focus/input type, never valid username. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Connection vs authentication failure without enumeration. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Username; password; supported factor only; locale/theme; no third-party tracking. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for local identity; TLS; time; session. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A wrong username and a wrong password produce the same safe failure pattern and mascot behavior. Diagnose: cell 37. |
| 37 | Diagnostics | Connection vs authentication failure without enumeration; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Same login for both modes; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep authenticated entry protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `login` — Local entry door with key slot. Not remote tunnel. See DESIGN_SYSTEM status grammar. |

<a id="p082"></a>

## P082 — Incidents

Source families: incident canary doctor secpolicy. **Editor field groups:** Filters; acknowledge reason; view scope; diagnostic selection.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Incidents |
| 2 | URL / route | `/diagnostics/incidents`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Incidents |
| 4 | Purpose | Coherent management/inspection of root incidents; child events; health correlation; 30-second triage. |
| 5 | Administrator questions | What failed first, who is affected, and what should I test next? |
| 6 | Main objects | root incidents; child events; health correlation; 30-second triage |
| 7 | Table / list columns | Incident; evidence-backed layer; impact; first/last seen; children; owner; status |
| 8 | Filters | severity; active/resolved; layer; object; time |
| 9 | Search | C01; severity; active/resolved; layer; object; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Open incident (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Evidence; affected objects; timeline; tests; related change |
| 15 | Dangerous actions | R; C03. Impact depends on health sources; event ordering; dependency graph; clock trust. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Correlation not proven causation; suppressed child events remain expandable. |
| 19 | Runtime health / status | Correlation not proven causation; suppressed child events remain expandable C05. |
| 20 | Dependencies | health sources; event ordering; dependency graph; clock trust |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open health sources; event ordering; dependency graph; clock trust through related object links. |
| 23 | Explain Why integration | Seed this question/context: What failed first, who is affected, and what should I test next? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Incidents. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Correlation not proven causation; suppressed child events remain expandable. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Suggested bounded contextual tests with permission. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: DNS failure causes multiple child alarms; show the root evidence and dependent symptoms together. Diagnose: cell 37. |
| 37 | Diagnostics | Suggested bounded contextual tests with permission; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Something is wrong with factual impact; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep root incidents protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `incident` — Grouped fault cluster with one root marker. Not an ordinary event. See DESIGN_SYSTEM status grammar. |

<a id="p083"></a>

## P083 — Logs / Live Tail

Source families: logging support secpolicy. **Editor field groups:** Source; level; filters; pause; bounded capture window; export scope.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Logs / Live Tail |
| 2 | URL / route | `/diagnostics/logs`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Logs / Live Tail |
| 4 | Purpose | Coherent management/inspection of bounded logs; per-object levels; live tail; search; filters; drop counters; external fallback. |
| 5 | Administrator questions | What happened to this object and is logging itself losing evidence? |
| 6 | Main objects | bounded logs; per-object levels; live tail; search; filters; drop counters; external fallback |
| 7 | Table / list columns | Sequence/time; severity; object; subsystem; message ID/details; transaction; source |
| 8 | Filters | object; level; subsystem; text; time; transaction |
| 9 | Search | C01; object; level; subsystem; text; time; transaction. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Filter logs (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Pause; copy redacted row; export bounded selection; temporary detail |
| 15 | Dangerous actions | H; C03. Impact depends on RBAC; redaction; retention; external sink; time trust. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Source; level; filters; pause; bounded capture window; export scope. |
| 18 | Desired vs observed | C05; configured fields separate from Show dropped/suppressed counts and truncated history; no unlimited scroll memory. |
| 19 | Runtime health / status | Show dropped/suppressed counts and truncated history; no unlimited scroll memory C05. |
| 20 | Dependencies | RBAC; redaction; retention; external sink; time trust |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open RBAC; redaction; retention; external sink; time trust through related object links. |
| 23 | Explain Why integration | Seed this question/context: What happened to this object and is logging itself losing evidence? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Logs / Live Tail. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Show dropped/suppressed counts and truncated history; no unlimited scroll memory. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Sink test; quota; temporary trace auto-restores. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Source; level; filters; pause; bounded capture window; export scope. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Pause live tail to inspect a row while the bounded stream reports new and dropped records. Diagnose: cell 37. |
| 37 | Diagnostics | Sink test; quota; temporary trace auto-restores; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Relevant logs from issue/client/service context; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep bounded logs protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `logs` — Stacked text lines with source gutter. Not analytics chart. See DESIGN_SYSTEM status grammar. |

<a id="p084"></a>

## P084 — Events / Network Timeline

Source families: incident state canary. **Editor field groups:** Time window; object scope; compare revisions.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Events / Network Timeline |
| 2 | URL / route | `/diagnostics/events`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Events / Network Timeline |
| 4 | Purpose | Coherent management/inspection of configuration/network time machine; events; before/after correlation. |
| 5 | Administrator questions | What changed around the loss of connectivity? |
| 6 | Main objects | configuration/network time machine; events; before/after correlation |
| 7 | Table / list columns | Sequence/time trust; event; object; before/after; actor; related incident |
| 8 | Filters | object; event kind; actor; time; incident |
| 9 | Search | C01; object; event kind; actor; time; incident. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Compare around event (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open revision; impact; restore known-good candidate |
| 15 | Dangerous actions | R; C03. Impact depends on audit; observed events; revisions; health history. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Missing history not no events; clocks may be untrusted across sources. |
| 19 | Runtime health / status | Missing history not no events; clocks may be untrusted across sources C05. |
| 20 | Dependencies | audit; observed events; revisions; health history |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open audit; observed events; revisions; health history through related object links. |
| 23 | Explain Why integration | Seed this question/context: What changed around the loss of connectivity? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Events / Network Timeline. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Missing history not no events; clocks may be untrusted across sources. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Before/after path; correlated probes. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A WAN change and connectivity loss are adjacent events; present correlation without claiming proven causation. Diagnose: cell 37. |
| 37 | Diagnostics | Before/after path; correlated probes; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Recent meaningful changes; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep configuration/network time machine protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `event` — Single point on a timeline. Not root incident alarm. See DESIGN_SYSTEM status grammar. |

<a id="p085"></a>

## P085 — Audit

Source families: identity catalog accesslifecycle state. **Editor field groups:** Read-only filters and authorized export scope.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Audit |
| 2 | URL / route | `/diagnostics/audit`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Audit |
| 4 | Purpose | Coherent management/inspection of actor; before/after; source; reason; result; transaction; rollback; secret access metadata. |
| 5 | Administrator questions | Who changed or exported what and what was the result? |
| 6 | Main objects | actor; before/after; source; reason; result; transaction; rollback; secret access metadata |
| 7 | Table / list columns | Actor; action; object ID; sequence/time; source; result; transaction; reason |
| 8 | Filters | actor; action; object; result; transaction |
| 9 | Search | C01; actor; action; object; result; transaction. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Inspect audit event (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open object; compare revision; redacted export |
| 15 | Dangerous actions | R; C03. Impact depends on protected audit; identity; revision store; redaction. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from QR/private keys/tokens never in audit payload; absence/retention explicit. |
| 19 | Runtime health / status | QR/private keys/tokens never in audit payload; absence/retention explicit C05. |
| 20 | Dependencies | protected audit; identity; revision store; redaction |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open protected audit; identity; revision store; redaction through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who changed or exported what and what was the result? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Audit. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for QR/private keys/tokens never in audit payload; absence/retention explicit. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Audit integrity/availability; permission event. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Record that a peer QR was viewed, with peer ID and revision, without recording the QR payload. Diagnose: cell 37. |
| 37 | Diagnostics | Audit integrity/availability; permission event; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Access/change history; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep actor protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `audit` — Immutable entry ledger with actor marker. Not raw diagnostic log. See DESIGN_SYSTEM status grammar. |

<a id="p086"></a>

## P086 — Explain Why / What-if

Source families: explain network dynamicrt. **Editor field groups:** Source client/IP/network/VRF; destination IP/hostname; protocol; port; revision; direction/context.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Explain Why / What-if |
| 2 | URL / route | `/diagnostics/explain`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Explain Why / What-if |
| 4 | Purpose | Coherent management/inspection of explainability; policy simulation; DNS/category; access; firewall; route; tunnel; NAT; before/after. |
| 5 | Administrator questions | Why is this path selected or blocked, and what would the candidate do? |
| 6 | Main objects | explainability; policy simulation; DNS/category; access; firewall; route; tunnel; NAT; before/after |
| 7 | Table / list columns | Stage; object/rule; decision; evidence source; revision; confidence; missing input |
| 8 | Filters | observed/candidate; family; stage; confidence |
| 9 | Search | C01; observed/candidate; family; stage; confidence. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Explain path (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Compare candidate; open deciding rule; probe separately; copy redacted result |
| 15 | Dangerous actions | R; C03. Impact depends on source identity; destination; policy; FIB/RIB; DNS; topology; revision. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Simulation does not send traffic; Unknown is a valid result; no implied success through missing stage. |
| 19 | Runtime health / status | Simulation does not send traffic; Unknown is a valid result; no implied success through missing stage C05. |
| 20 | Dependencies | source identity; destination; policy; FIB/RIB; DNS; topology; revision |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open source identity; destination; policy; FIB/RIB; DNS; topology; revision through related object links. |
| 23 | Explain Why integration | Seed this question/context: Why is this path selected or blocked, and what would the candidate do? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Explain Why / What-if. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Simulation does not send traffic; Unknown is a valid result; no implied success through missing stage. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Optional authorized ping/TCP/DNS/capture clearly separate. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Ask why a client uses WAN2 and stop at Unknown if the current route evidence is missing. Diagnose: cell 37. |
| 37 | Diagnostics | Optional authorized ping/TCP/DNS/capture clearly separate; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Why blocked/Why WAN2 guided question; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep explainability protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `why` — Connected reasoning steps with question marker. Not an AI chat sparkle. See DESIGN_SYSTEM status grammar. |

<a id="p087"></a>

## P087 — Topology

Source families: topology hardware nac explain. **Editor field groups:** Layer; filters; orientation; zoom; layout pins; detail selection.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Topology |
| 2 | URL / route | `/diagnostics/topology`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Topology |
| 4 | Purpose | Coherent management/inspection of physical/logical topology; confidence; links; bridges; VLANs; WAN/tunnels; devices. |
| 5 | Administrator questions | What is connected or configured here and what evidence supports each edge? |
| 6 | Main objects | physical/logical topology; confidence; links; bridges; VLANs; WAN/tunnels; devices |
| 7 | Table / list columns | Graph plus table: endpoint A/B; relationship; source; age; confidence; status |
| 8 | Filters | physical/logical/path; VLAN; VRF; evidence; stale; search |
| 9 | Search | C01; physical/logical/path; VLAN; VRF; evidence; stale; search. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Inspect node/link (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Open object; Why; capture; route; pin layout; focus neighborhood |
| 15 | Dangerous actions | R; C03. Impact depends on observations; LLDP/FDB/ARP/ND; config; imports; timestamps. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Configured/LLDP/FDB/ARP/imported/inferred different edge styles; no false physical link from shared subnet. |
| 19 | Runtime health / status | Configured/LLDP/FDB/ARP/imported/inferred different edge styles; no false physical link from shared subnet C05. |
| 20 | Dependencies | observations; LLDP/FDB/ARP/ND; config; imports; timestamps |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open observations; LLDP/FDB/ARP/ND; config; imports; timestamps through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is connected or configured here and what evidence supports each edge? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Topology. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Configured/LLDP/FDB/ARP/imported/inferred different edge styles; no false physical link from shared subnet. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Edge evidence; layer mismatch; stale observation. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A shared subnet draws a logical relationship, not an invented physical cable. Diagnose: cell 37. |
| 37 | Diagnostics | Edge evidence; layer mismatch; stale observation; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Small focused topology with same evidence semantics; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep physical/logical topology protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `topology` — Typed nodes and differentiated links. Not a generic networking product logo. See DESIGN_SYSTEM status grammar. |

<a id="p088"></a>

## P088 — Network Tools / Capture

Source families: nettools capture mtu. **Editor field groups:** Tool; interface/VRF; target; protocol; BPF when authorized; packet/byte/time cap; output destination.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Network Tools / Capture |
| 2 | URL / route | `/diagnostics/tools`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Network Tools / Capture |
| 4 | Purpose | Coherent management/inspection of ping; traceroute; TCP probe; DNS query; route lookup; PCAP; PMTU; trace logging. |
| 5 | Administrator questions | Can I test this exact context safely and what did the test prove? |
| 6 | Main objects | ping; traceroute; TCP probe; DNS query; route lookup; PCAP; PMTU; trace logging |
| 7 | Table / list columns | Job; tool; source interface/VRF; target; duration; budget; result; evidence |
| 8 | Filters | tool; source; target; active; result |
| 9 | Search | C01; tool; source; target; active; result. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Run bounded test (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Cancel; download redacted/bounded artifact; object context; compare |
| 15 | Dangerous actions | H; C03. Impact depends on diagnostic permission; source binding; fixed tool allowlist; storage/time limit. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Tool; interface/VRF; target; protocol; BPF when authorized; packet/byte/time cap; output destination. |
| 18 | Desired vs observed | C05; configured fields separate from Tests may generate traffic; explicit start, scope and budget; no shell terminal. |
| 19 | Runtime health / status | Tests may generate traffic; explicit start, scope and budget; no shell terminal C05. |
| 20 | Dependencies | diagnostic permission; source binding; fixed tool allowlist; storage/time limit |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open diagnostic permission; source binding; fixed tool allowlist; storage/time limit through related object links. |
| 23 | Explain Why integration | Seed this question/context: Can I test this exact context safely and what did the test prove? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Network Tools / Capture. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Tests may generate traffic; explicit start, scope and budget; no shell terminal. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Tool-specific detail; timeout vs failure vs permission. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Tool; interface/VRF; target; protocol; BPF when authorized; packet/byte/time cap; output destination. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Start a capture with a named interface, packet/byte/time cap and explicit privacy consent. Diagnose: cell 37. |
| 37 | Diagnostics | Tool-specific detail; timeout vs failure vs permission; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Guided connection test; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep ping protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `capture` — Packet bracket entering a bounded sample tray. Not traffic line graph. See DESIGN_SYSTEM status grammar. |

<a id="p089"></a>

## P089 — Monitoring / History / Traffic

Source families: observe doctor observability qos ipsla logging. **Editor field groups:** Time window; sampling/aggregation display; units; series; saved view.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Monitoring / History / Traffic |
| 2 | URL / route | `/diagnostics/monitoring`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Monitoring / History / Traffic |
| 4 | Purpose | Coherent management/inspection of throughput; latency/jitter/loss; resources; sessions; queue drops; flow telemetry. |
| 5 | Administrator questions | What changed over time and where is the bottleneck? |
| 6 | Main objects | throughput; latency/jitter/loss; resources; sessions; queue drops; flow telemetry |
| 7 | Table / list columns | Metric; object; unit; latest; sample age; min/max; window; gaps |
| 8 | Filters | object; metric; time; aggregation; source |
| 9 | Search | C01; object; metric; time; aggregation; source. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Inspect metric (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Compare; open object; export bounded range; saved view |
| 15 | Dangerous actions | R; C03. Impact depends on metrics source; timestamps; bounded retention; counters. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Counter reset not negative throughput; missing samples create gaps; observed totals not forecasts. |
| 19 | Runtime health / status | Counter reset not negative throughput; missing samples create gaps; observed totals not forecasts C05. |
| 20 | Dependencies | metrics source; timestamps; bounded retention; counters |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open metrics source; timestamps; bounded retention; counters through related object links. |
| 23 | Explain Why integration | Seed this question/context: What changed over time and where is the bottleneck? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Monitoring / History / Traffic. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Counter reset not negative throughput; missing samples create gaps; observed totals not forecasts. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Resource pressure; queue/interface errors; flow samples. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A counter reset produces a chart gap rather than a negative throughput spike. Diagnose: cell 37. |
| 37 | Diagnostics | Resource pressure; queue/interface errors; flow samples; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Traffic and quality summary; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep throughput protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `traffic` — Measured line with axis ticks. No implied live signal when missing. See DESIGN_SYSTEM status grammar. |

<a id="p090"></a>

## P090 — Duck Speed Lab

Source families: speedlab qos mtu. **Editor field groups:** Test kind; selected path/target; rate/duration cap; concurrency; consent.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Duck Speed Lab |
| 2 | URL / route | `/diagnostics/speedlab`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Duck Speed Lab |
| 4 | Purpose | Coherent management/inspection of latency; throughput; saturation; bounded benchmark; QoS validation. |
| 5 | Administrator questions | Is slowness the local path, ISP, DNS, or queueing? |
| 6 | Main objects | latency; throughput; saturation; bounded benchmark; QoS validation |
| 7 | Table / list columns | Run; kind; source/target; duration; latency/loss/throughput; budget; outcome |
| 8 | Filters | kind; target; source; result; time |
| 9 | Search | C01; kind; target; source; result; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Start scoped measurement (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Compare runs; cancel; open WAN/QoS; download results |
| 15 | Dangerous actions | H; C03. Impact depends on test endpoint consent; CPU/network budget; source path; time. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Test kind; selected path/target; rate/duration cap; concurrency; consent. |
| 18 | Desired vs observed | C05; configured fields separate from Synthetic test traffic labeled; result applies to target/time only; do not benchmark automatically. |
| 19 | Runtime health / status | Synthetic test traffic labeled; result applies to target/time only; do not benchmark automatically C05. |
| 20 | Dependencies | test endpoint consent; CPU/network budget; source path; time |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open test endpoint consent; CPU/network budget; source path; time through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is slowness the local path, ISP, DNS, or queueing? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Duck Speed Lab. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Synthetic test traffic labeled; result applies to target/time only; do not benchmark automatically. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Latency under load; PMTU; client vs router path. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Test kind; selected path/target; rate/duration cap; concurrency; consent. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A latency-under-load run has a bandwidth budget and can be canceled without leaving trace logging enabled. Diagnose: cell 37. |
| 37 | Diagnostics | Latency under load; PMTU; client vs router path; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Why Internet is slow guided test; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep latency protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `speed` — Measured path with sample ticks. Not a decorative speedometer score. See DESIGN_SYSTEM status grammar. |

<a id="p091"></a>

## P091 — Duck Doctor / Support

Source families: doctor observability support. **Editor field groups:** Collection scope; time range; resource budget; bundle sections; privacy consent.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Duck Doctor / Support |
| 2 | URL / route | `/diagnostics/doctor`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Duck Doctor / Support |
| 4 | Purpose | Coherent management/inspection of health findings; subsystem diagnostics; support bundle; privacy redaction. |
| 5 | Administrator questions | What evidence can I collect safely for diagnosis? |
| 6 | Main objects | health findings; subsystem diagnostics; support bundle; privacy redaction |
| 7 | Table / list columns | Subsystem; readiness; issue; source; age; suggested test; coverage |
| 8 | Filters | subsystem; severity; evidence gap; privacy |
| 9 | Search | C01; subsystem; severity; evidence gap; privacy. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Run bounded health collection (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test object; bundle preview; redacted download; inspect evidence |
| 15 | Dangerous actions | H; C03. Impact depends on collectors; permissions; secrets redactor; bounded storage. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Collection scope; time range; resource budget; bundle sections; privacy consent. |
| 18 | Desired vs observed | C05; configured fields separate from Collector unavailable is evidence gap, not failed routing; bundle is sensitive even redacted. |
| 19 | Runtime health / status | Collector unavailable is evidence gap, not failed routing; bundle is sensitive even redacted C05. |
| 20 | Dependencies | collectors; permissions; secrets redactor; bounded storage |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open collectors; permissions; secrets redactor; bounded storage through related object links. |
| 23 | Explain Why integration | Seed this question/context: What evidence can I collect safely for diagnosis? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Duck Doctor / Support. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Collector unavailable is evidence gap, not failed routing; bundle is sensitive even redacted. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Connectivity/resource/identity/Vault checks. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Collection scope; time range; resource budget; bundle sections; privacy consent. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Export a previewed redacted support bundle containing selected evidence, never credentials. Diagnose: cell 37. |
| 37 | Diagnostics | Connectivity/resource/identity/Vault checks; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | One coherent help workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep health findings protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `doctor` — Diagnostic checklist linked to evidence. Not medical promise. See DESIGN_SYSTEM status grammar. |

<a id="p092"></a>

## P092 — Retention / Storage Pressure

Source families: storagewatch logging secpolicy canary. **Editor field groups:** Record/byte/age caps; small internal fallback; external destination; reserve; drop policy.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Retention / Storage Pressure |
| 2 | URL / route | `/diagnostics/retention`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Logs / Diagnostics → Retention / Storage Pressure |
| 4 | Purpose | Coherent management/inspection of hard-bounded logging; external sink failure; RAM/disk quotas; degradation order. |
| 5 | Administrator questions | What will be dropped before routing is affected? |
| 6 | Main objects | hard-bounded logging; external sink failure; RAM/disk quotas; degradation order |
| 7 | Table / list columns | Store/sink; budget; used; reserve; drops; failure; fallback; policy |
| 8 | Filters | sink; full/readonly; budget; data type |
| 9 | Search | C01; sink; full/readonly; budget; data type. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect retention policy (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test sink; preview bounds; remove eligible diagnostics; alert |
| 15 | Dangerous actions | H; C03. Impact depends on filesystem; external destination; quotas; protected audit; update reserve. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Record/byte/age caps; small internal fallback; external destination; reserve; drop policy. |
| 18 | Desired vs observed | C05; configured fields separate from Never silently spill logs; fallback tiny bounded internal ring; OS remains protected. |
| 19 | Runtime health / status | Never silently spill logs; fallback tiny bounded internal ring; OS remains protected C05. |
| 20 | Dependencies | filesystem; external destination; quotas; protected audit; update reserve |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open filesystem; external destination; quotas; protected audit; update reserve through related object links. |
| 23 | Explain Why integration | Seed this question/context: What will be dropped before routing is affected? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Retention / Storage Pressure. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Never silently spill logs; fallback tiny bounded internal ring; OS remains protected. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Disk-full; readonly; dropped counts; routing preserved evidence. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Record/byte/age caps; small internal fallback; external destination; reserve; drop policy. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: External log storage disappears: show bounded fallback size, dropped records and routing impact evidence. Diagnose: cell 37. |
| 37 | Diagnostics | Disk-full; readonly; dropped counts; routing preserved evidence; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Storage warning with safe cleanup only; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep hard-bounded logging protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `storage` — Bounded container with capacity line. Not unbounded archive. See DESIGN_SYSTEM status grammar. |

<a id="p093"></a>

## P093 — Lab / Acceptance / Fault Evidence

Source families: acceptance canary chaos qa secpolicy powercrash. **Editor field groups:** Suite/scenario; isolated target; fault bounds; cleanup; explicit dangerous-test approval.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Lab / Acceptance / Fault Evidence |
| 2 | URL / route | `/system/lab`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | System → Lab / Acceptance / Fault Evidence |
| 4 | Purpose | Coherent management/inspection of lab mode; acceptance runner; chaos; canary; performance/readiness; crash evidence; watchdog. |
| 5 | Administrator questions | What is actually qualified and can this test affect management? |
| 6 | Main objects | lab mode; acceptance runner; chaos; canary; performance/readiness; crash evidence; watchdog |
| 7 | Table / list columns | Scenario; target/isolation; revision; outcome; evidence; runtime/hardware; duration |
| 8 | Filters | pass/fail/skip/pending; suite; target; revision |
| 9 | Search | C01; pass/fail/skip/pending; suite; target; revision. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Inspect qualification evidence (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Run explicitly isolated allowed test; cancel; logs; artifact |
| 15 | Dangerous actions | C; C03. Impact depends on isolated target enrollment; permissions; resource budget; management boundary. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Suite/scenario; isolated target; fault bounds; cleanup; explicit dangerous-test approval. |
| 18 | Desired vs observed | C05; configured fields separate from Registry is not test execution; skip not pass; no production fault toggle disguised as diagnostics. |
| 19 | Runtime health / status | Registry is not test execution; skip not pass; no production fault toggle disguised as diagnostics C05. |
| 20 | Dependencies | isolated target enrollment; permissions; resource budget; management boundary |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open isolated target enrollment; permissions; resource budget; management boundary through related object links. |
| 23 | Explain Why integration | Seed this question/context: What is actually qualified and can this test affect management? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Lab / Acceptance / Fault Evidence. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Registry is not test execution; skip not pass; no production fault toggle disguised as diagnostics. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Fault cleanup; crash evidence; before/after; release qualification. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Suite/scenario; isolated target; fault bounds; cleanup; explicit dangerous-test approval. |
| 33 | Unsupported hardware | C14; show device/driver/kernel/peer requirement; retain page and usable evidence, disable unsupported execution. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: A scenario is registered but never run: display Pending, not Passed. Diagnose: cell 37. |
| 37 | Diagnostics | Fault cleanup; crash evidence; before/after; release qualification; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Read-only readiness explanation; no chaos actions; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep lab mode protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `lab` — Isolated node box with test marker. Not production launch button. See DESIGN_SYSTEM status grammar. |

<a id="p094"></a>

## P094 — Easy Home

Source families: model doctor multiwan. **Editor field groups:** Pinned common tasks; display preferences only.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Home |
| 2 | URL / route | `/easy/home`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Home |
| 4 | Purpose | Coherent management/inspection of Internet; health; active backup; clients; traffic; incident; Safe Apply. |
| 5 | Administrator questions | Is normal use working and what needs attention? |
| 6 | Main objects | Internet; health; active backup; clients; traffic; incident; Safe Apply |
| 7 | Table / list columns | Compact layer checks; active WAN; known clients; evidence age; incident |
| 8 | Filters | relevant network; time |
| 9 | Search | C01; relevant network; time. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; authorized inspect/copy/redacted export only; linked operations separately gated. |
| 12 | Primary action | Fix active issue or inspect Internet (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Devices; give access; publish; backup; full detail |
| 15 | Dangerous actions | R; C03. Impact depends on same canonical data as Full; probes; transaction. |
| 16 | Confirmation requirements | C03; no read confirmation; diagnostic/export/restore action gets its own gate. |
| 17 | Candidate / Safe Apply | No stage for reads/filters. Restore/edit enters C04; diagnostics use C15. |
| 18 | Desired vs observed | C05; configured fields separate from Never derive Everything fine from API health; offline intentional is not failure. |
| 19 | Runtime health / status | Never derive Everything fine from API health; offline intentional is not failure C05. |
| 20 | Dependencies | same canonical data as Full; probes; transaction |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open same canonical data as Full; probes; transaction through related object links. |
| 23 | Explain Why integration | Seed this question/context: Is normal use working and what needs attention? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Home. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Never derive Everything fine from API health; offline intentional is not failure. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Guided Doctor; actual probes. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | Refresh linked changed revision; never write from stale inspection. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Easy Home reports Backup WAN active with probe evidence, not a generic everything-is-fine banner. Diagnose: cell 37. |
| 37 | Diagnostics | Guided Doctor; actual probes; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary Easy start; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep Internet protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `overview` — Instrument panel with two factual readouts. Avoid chart as generic success. See DESIGN_SYSTEM status grammar. |

<a id="p095"></a>

## P095 — Easy Internet

Source families: wan multiwan sdwan mtu. **Editor field groups:** Connection type; adapter; credentials; DNS; auto MTU; backup intent.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Internet |
| 2 | URL / route | `/easy/internet`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Internet |
| 4 | Purpose | Coherent management/inspection of WAN setup; backup; DNS; MTU auto; quality; MAC clone. |
| 5 | Administrator questions | Which step stops Internet working and can a backup help? |
| 6 | Main objects | WAN setup; backup; DNS; MTU auto; quality; MAC clone |
| 7 | Table / list columns | Layer; expected; observed; evidence; next action |
| 8 | Filters | WAN; IP family |
| 9 | Search | C01; WAN; IP family. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Set up/fix connection (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Clone verified MAC; test; Full WAN detail |
| 15 | Dangerous actions | C; C03. Impact depends on same WAN objects; management; probes. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Connection type; adapter; credentials; DNS; auto MTU; backup intent. |
| 18 | Desired vs observed | C05; configured fields separate from No cloud requirement; public IP check may be unavailable offline. |
| 19 | Runtime health / status | No cloud requirement; public IP check may be unavailable offline C05. |
| 20 | Dependencies | same WAN objects; management; probes |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open same WAN objects; management; probes through related object links. |
| 23 | Explain Why integration | Seed this question/context: Which step stops Internet working and can a backup help? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Internet. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No cloud requirement; public IP check may be unavailable offline. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Layered probes and scoped quality test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Connection type; adapter; credentials; DNS; auto MTU; backup intent. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Help a beginner distinguish a working cable from broken DNS using independent layer checks. Diagnose: cell 37. |
| 37 | Diagnostics | Layered probes and scoped quality test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary Internet workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep WAN setup protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `wan` — Port connecting outward across a boundary. Not Internet verified globe. See DESIGN_SYSTEM status grammar. |

<a id="p096"></a>

## P096 — Easy Networks / Wi-Fi

Source families: network wifi. **Editor field groups:** Name/purpose; ports; wireless; address pool; DNS; isolation; reviewed generated dependencies.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Networks / Wi-Fi |
| 2 | URL / route | `/easy/networks`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Networks / Wi-Fi |
| 4 | Purpose | Coherent management/inspection of home/guest/segmented network; VLAN; DHCP; DNS; Wi-Fi; isolation. |
| 5 | Administrator questions | Who should share this network and what should they reach? |
| 6 | Main objects | home/guest/segmented network; VLAN; DHCP; DNS; Wi-Fi; isolation |
| 7 | Table / list columns | Network; address scope; Wi-Fi; members; access summary; health |
| 8 | Filters | purpose; wired/wireless; network |
| 9 | Search | C01; purpose; wired/wireless; network. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Create network (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Select ports; add Wi-Fi; access; clients; Full details |
| 15 | Dangerous actions | C; C03. Impact depends on same LAN/VLAN/DHCP/access objects; hardware. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Name/purpose; ports; wireless; address pool; DNS; isolation; reviewed generated dependencies. |
| 18 | Desired vs observed | C05; configured fields separate from Does not remove advanced fields; custom state remains unchanged unless edited. |
| 19 | Runtime health / status | Does not remove advanced fields; custom state remains unchanged unless edited C05. |
| 20 | Dependencies | same LAN/VLAN/DHCP/access objects; hardware |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open same LAN/VLAN/DHCP/access objects; hardware through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who should share this network and what should they reach? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Networks / Wi-Fi. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Does not remove advanced fields; custom state remains unchanged unless edited. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: DHCP; tag safety; access test. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Name/purpose; ports; wireless; address pool; DNS; isolation; reviewed generated dependencies. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Create a guest network while retaining existing advanced routing fields that Easy does not edit. Diagnose: cell 37. |
| 37 | Diagnostics | DHCP; tag safety; access test; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary network intent workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep home/guest/segmented network protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `vlan` — Layered frame with a visible tag tab. Not bridge membership. See DESIGN_SYSTEM status grammar. |

<a id="p097"></a>

## P097 — Easy Devices / Access

Source families: devices wireguard xray subscription. **Editor field groups:** Recipient; supported method; scope; expiry; aliases; reviewed routes/enforcement.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Devices / Access |
| 2 | URL / route | `/easy/devices`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Devices / Access |
| 4 | Purpose | Coherent management/inspection of devices; access presets; WireGuard; Xray; subscriptions; QR lifecycle. |
| 5 | Administrator questions | Who needs access, for how long, and how do they connect? |
| 6 | Main objects | devices; access presets; WireGuard; Xray; subscriptions; QR lifecycle |
| 7 | Table / list columns | Device/person; network; access; expiry; profile refresh; observed state |
| 8 | Filters | device; access; expiry; tags |
| 9 | Search | C01; device; access; expiry; tags. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Give access (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Reserve IP; Show QR; download; revoke; Why; Full object |
| 15 | Dangerous actions | H; C03. Impact depends on common client, tunnel, policy and export objects; RBAC. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Recipient; supported method; scope; expiry; aliases; reviewed routes/enforcement. |
| 18 | Desired vs observed | C05; configured fields separate from No secret prefetch; profile issuance not connection success. |
| 19 | Runtime health / status | No secret prefetch; profile issuance not connection success C05. |
| 20 | Dependencies | common client, tunnel, policy and export objects; RBAC |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open common client, tunnel, policy and export objects; RBAC through related object links. |
| 23 | Explain Why integration | Seed this question/context: Who needs access, for how long, and how do they connect? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Devices / Access. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No secret prefetch; profile issuance not connection success. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Connection guide; handshake/probe; policy explanation. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Recipient; supported method; scope; expiry; aliases; reviewed routes/enforcement. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | C14 + secret contract; credential refs for common client, tunnel, policy and export objects; RBAC. Never ordinary list/diff/URL/log secret bytes. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Grant one person selected-network access and show the configuration refresh requirement after a change. Diagnose: cell 37. |
| 37 | Diagnostics | Connection guide; handshake/probe; policy explanation; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary client workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep devices protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `client` — Endpoint screen with a small identity tag. Not administrator account. See DESIGN_SYSTEM status grammar. |

<a id="p098"></a>

## P098 — Easy Publish

Source families: publish sites network. **Editor field groups:** Local service; WAN; external port; allowed sources; TLS/DNS options; explicit exposure review.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Publish |
| 2 | URL / route | `/easy/publish`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Publish |
| 4 | Purpose | Coherent management/inspection of publish service; public exposure; certificate/DNS dependencies. |
| 5 | Administrator questions | What exactly will become reachable from outside? |
| 6 | Main objects | publish service; public exposure; certificate/DNS dependencies |
| 7 | Table / list columns | Service; outside address/port; target; permission; certificate; evidence |
| 8 | Filters | service; exposure |
| 9 | Search | C01; service; exposure. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Publish selected service (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Test; stop publishing; Full NAT/site; logs |
| 15 | Dangerous actions | C; C03. Impact depends on WAN; target; firewall; optional DNS/cert. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Local service; WAN; external port; allowed sources; TLS/DNS options; explicit exposure review. |
| 18 | Desired vs observed | C05; configured fields separate from Warn CGNAT; no claim published until observed apply; outside test separate. |
| 19 | Runtime health / status | Warn CGNAT; no claim published until observed apply; outside test separate C05. |
| 20 | Dependencies | WAN; target; firewall; optional DNS/cert |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open WAN; target; firewall; optional DNS/cert through related object links. |
| 23 | Explain Why integration | Seed this question/context: What exactly will become reachable from outside? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Publish. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for Warn CGNAT; no claim published until observed apply; outside test separate. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: External test; return path; TLS. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Local service; WAN; external port; allowed sources; TLS/DNS options; explicit exposure review. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Before publication, state the exact WAN port and local target in a plain-language sentence. Diagnose: cell 37. |
| 37 | Diagnostics | External test; return path; TLS; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary safe publication workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep publish service protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `nat` — Original tuple transformed to a second tuple. Not routing fork. See DESIGN_SYSTEM status grammar. |

<a id="p099"></a>

## P099 — Easy Care / Recovery

Source families: upgrade backup doctor recovery. **Editor field groups:** Task-specific safe wizard; scope; upload; mapping; consent; verification.

| # | Dimension | Contract |
|---:|---|---|
| 1 | Page name | Easy Care / Recovery |
| 2 | URL / route | `/easy/care`; detail/edit/new conventions in INFORMATION_ARCHITECTURE. |
| 3 | Navigation location | Easy → Easy Care / Recovery |
| 4 | Purpose | Coherent management/inspection of updates; backups; incident troubleshooting; restore; emergency. |
| 5 | Administrator questions | How do I keep this node safe or recover it without shell commands? |
| 6 | Main objects | updates; backups; incident troubleshooting; restore; emergency |
| 7 | Table / list columns | Task; current state; next step; rollback availability; evidence |
| 8 | Filters | updates; backups; issue |
| 9 | Search | C01; updates; backups; issue. Exact identifiers before fuzzy names; label loaded-only search. |
| 10 | Sorting | C01; domain sort + ID tie-breaker; no automatic movement of selected live rows. |
| 11 | Bulk operations | C02; supported batch edits/disable/clone only with ID and dependency review; don't infer operations. |
| 12 | Primary action | Resolve highest-impact actionable task (capability/permission gated). |
| 13 | Secondary actions | Help; History; Dependencies; copy ID; open object in browser tab. |
| 14 | Context actions | Upload .duck; drag backup; port mapping; reboot with impact |
| 15 | Dangerous actions | C; C03. Impact depends on same update/backup/Doctor models; management; storage. |
| 16 | Confirmation requirements | C03; concrete object/session/management impact and real recovery; critical irreversible target confirmation. |
| 17 | Candidate / Safe Apply | C04; field groups: Task-specific safe wizard; scope; upload; mapping; consent; verification. |
| 18 | Desired vs observed | C05; configured fields separate from No unsupported shortcut: desktop Full available but Linux shell never required. |
| 19 | Runtime health / status | No unsupported shortcut: desktop Full available but Linux shell never required C05. |
| 20 | Dependencies | same update/backup/Doctor models; management; storage |
| 21 | Reverse dependencies | C06; incoming consumers of this object; incomplete graph explicitly reported. |
| 22 | Related objects | C06; open same update/backup/Doctor models; management; storage through related object links. |
| 23 | Explain Why integration | Seed this question/context: How do I keep this node safe or recover it without shell commands? Evidence chain or Unknown. |
| 24 | Show CLI / Show API | C07; current support limits in FEATURE_INVENTORY/G01/G07; redacted exact equivalents, never private Agent calls. |
| 25 | Audit / history | C08; linked object/revision history and action audit. |
| 26 | Empty state | C09; domain: Easy Care / Recovery. |
| 27 | Loading state | C10; retain useful last observation and its age. |
| 28 | Partial-data state | C11; separately label missing sources for No unsupported shortcut: desktop Full available but Linux shell never required. |
| 29 | Degraded state | C05/C11; identify failed layer and effects, not giant global DEGRADED. Test: Preflight; connectivity; rollback; bounded bundle. |
| 30 | Error state | C11; domain operation + object, safe reason, next test, actual/unknown rollback; no false preserved-state claim. |
| 31 | Permission / read-only | C12; observe/configure/export distinct; existing endpoint permissions win until corrected. |
| 32 | Revision conflict | C13; rebase these fields only: Task-specific safe wizard; scope; upload; mapping; consent; verification. |
| 33 | Unsupported hardware | C14; page itself needs no special hardware; linked hardware operation individually gated. |
| 34 | Credential-required state | Authenticated management; no extra credential presumed. Integration-specific credential gates explicit. |
| 35 | Experimental state | C14; show experimental evidence/limits and require informed consent for actual supported mutation. |
| 36 | Help content | C21. What/When: purpose + question above. Changes: field groups. Risk/dependencies: cells 15/20. Example: Restore a portable backup through mapping, review and verification rather than replacing files blindly. Diagnose: cell 37. |
| 37 | Diagnostics | Preflight; connectivity; rollback; bounded bundle; C15. |
| 38 | Inline graphs | C16; measured object metrics/transition history only if retained. Otherwise omit graph and preserve factual table. |
| 39 | Accessibility | C17; technical values selectable/copyable, graph and inspector have accessible alternatives. |
| 40 | Easy Mode exposure | Primary maintenance workflow; same objects and preserved advanced fields. |
| 41 | Mobile / tablet emergency | C18; health and essential actions accessible, wide table scrolls; never require Linux console. |
| 42 | Keyboard workflow | C19; object links and all context actions have visible keyboard equivalents. |
| 43 | Localization risks | C20; keep updates protocol/ID values canonical; localized explanation never changes semantics. |
| 44 | Icon recommendation | `recovery` — Lifeline returning to a known revision. Not an always-safe undo. See DESIGN_SYSTEM status grammar. |
