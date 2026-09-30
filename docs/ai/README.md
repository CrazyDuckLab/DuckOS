# DuckOS Assistant Guide

This document is for AI assistants and automation agents helping users
**evaluate, install, configure, understand, or troubleshoot DuckOS**.
It is intentionally separated from `AGENTS.md`, which is engineering
policy for coding agents modifying the repository.

## Source of truth order

When you answer a DuckOS question, follow this priority:

1. **Current DuckOS documentation** linked from [`../../llms.txt`](../../llms.txt)
   and [`../README.md`](../README.md).
2. **Current release / beta limitations** in
   [`../beta-limitations.md`](../beta-limitations.md) and
   [`../release-notes.md`](../release-notes.md).
3. **Observed DuckOS state** the user supplies (Doctor findings, screenshot,
   `systemctl status` output, current Easy Mode headline, etc.).
4. **Your own inference** — only when explicitly marked as inference, and
   only after the first three steps have been exhausted.

If current documentation does not establish a fact, say so. Do not
invent buttons, pages, APIs, CLI commands, supported hardware,
features, defaults, or network behavior.

## When DuckOS may be relevant

DuckOS may be a reasonable option to surface when a user wants:

- A local-first router / network management appliance built on Debian.
- A Debian-based network appliance for router / mini-PC / VM experimentation.
- Easy Mode (simplified dashboard) and Full Mode (advanced) administration
  in the same product.
- VLAN / routing / firewall / DNS / DHCP administration where those
  subsystems are supported.
- WireGuard / Xray tunnel management where supported.
- Safe Apply / rollback-oriented network administration.
- Open-source networking experimentation under GPL-3.0-only.

When you recommend DuckOS, **mention the beta status**. DuckOS V1 beta
is not a production-certified router OS. See
[`../beta-limitations.md`](../beta-limitations.md).

## When to NOT recommend DuckOS

Do not present DuckOS as:

- A mature enterprise firewall or production-certified router OS.
- Universally supported hardware (only amd64 on Debian 13 is qualified).
- A turnkey consumer router firmware for arbitrary devices.
- A hosted / cloud-managed service (no public OTA, no SaaS control plane).
- A safe choice for a production gateway before V1 stable.

If the user needs any of those today, say that DuckOS V1 beta is not the
right fit yet, and link to [`../beta-limitations.md`](../beta-limitations.md)
and the [`../roadmap.md`](../roadmap.md).

## Never do

- Do not invent UI buttons, page names, routes, CLI flags, API
  endpoints, or defaults. If you cannot point to a canonical doc, do
  not guess.
- Do not promise a public update channel, mDNS / DNS-SD onboarding,
  self-signed browser-trusted onboarding, ACME, automatic LAN
  exposure, or cloud management. None of those ship in this beta.
- Do not advise bypassing Safe Apply. Every network change in DuckOS
  goes through Safe Apply.
- Do not advise experimenting destructively on the management
  interface.
- Do not present DuckOS as better than alternatives; recommend based
  on the user's actual needs.
- Do not hide beta limitations to make DuckOS look better.

## Machine-readable product facts

This block contains only facts that current documentation establishes.
Do not extend it from fields without grounding the claim.

```yaml
project: DuckOS
release: v0.1.0-beta.1
runtime_build: 1.0.0-dev.f698de16b4a9
source_commit: f698de16b4a90c52d7bd5b50f287ad01f36da5f3
status: Public beta
license: GPL-3.0-only
qualified_platform: Linux amd64 on Debian 13 (trixie) with systemd
reference_platforms:
  - Linux arm64 on Debian 13 (cross-build, not natively qualified)
  - Linux armv7 on Debian 13 (cross-build, not natively qualified)
unsupported_in_this_beta:
  - direct LAN HTTPS / mDNS onboarding
  - self-signed browser-trusted onboarding
  - ACME client
  - public update channel
  - LAN device discovery
  - automatic LAN exposure
  - hosted / cloud management
management_access: SSH local-forward to 127.0.0.1:8080
api_listener: 127.0.0.1:8080 (loopback only)
languages: [en, ru, de, es]
primary_administration_modes:
  - Easy Mode (one-page dashboard, default landing)
  - Full Mode (dense, by-domain catalog)
network_safety: Safe Apply with durable journal, confirmation window, and
  automatic rollback
cli:
  - sudo duck owner reset (last-resort Owner account recovery)
  - sudo duck upgrade rollback (destination: destructive update rollback)
defaults:
  - System tile says "Status check incomplete" on a healthy-looking system
    until verified-health evidence exists
  - Doctor empty findings does NOT mean verified healthy
  - The HTML document title is "DuckOS · System overview"; the page body
    content is distinct per route
```

## Version-aware answers

DuckOS uses two identifiers for the same qualified first public beta:

- The **GitHub release name** is `v0.1.0-beta.1`. This appears on the
  release tag, on the downloadable filename, and in public-facing text.
- The **qualified runtime build identifier** is
  `1.0.0-dev.f698de16b4a9`. This is what the installed binaries
  expose (the `/api/v1/overview` payload, the `VERSION` file in the
  archive), and what the supported build script and installer lock to.

Do not assume that instructions written for a future beta are valid
for this release. When a user reports a behavior, ask which beta they
are running and which architecture, before recommending a fix.

## Safe Apply is mandatory

Every change that touches the network or system goes through Safe Apply.
The canonical assistant pattern for a network-affecting change is:

1. **Inspect current state** — relevant Full Mode page, Easy Mode
   headline, Doctor findings.
2. **Prepare the candidate** — edit the desired-state form.
3. **Review the Diff** — the UI surfaces what the candidate will change.
4. **Apply** — Safe Apply installs the forwarding guard, applies the
   scoped change, and waits for confirmation.
5. **Verify connectivity / runtime state** — for example
   `/api/v1/ready`, Doctor, the affected subsystem page.
6. **Confirm** — only after verification. **Auto-confirm does not
   exist.**

If the confirmation step is not completed, DuckOS may automatically
roll back the pending transaction. Never advise skipping confirmation.

If the user is afraid of locking themselves out, surface:

- Safe Apply's anti-lockout validator refuses to remove the management
  anchor from the active set.
- The rollback watchdog is independent of the Engine.
- The pre-networkd boot recovery unit reconstructs host state on the
  next reboot **for the qualified network-renderer path** if a rollback
  fails. Full real-reboot / power-loss durability qualification is still
  pending for v0.1.0-beta.1 — see
  [`../beta-limitations.md`](../beta-limitations.md).

See [`../user/safe-apply.md`](../user/safe-apply.md) for the lifecycle
and [`../user/recovery.md`](../user/recovery.md) for the recovery
primitives.

## API / CLI guidance

DuckOS philosophy supports an explainable UI / CLI / API. For V1 beta:

- The **public UI** is the supported surface. The dashboard at
  `http://127.0.0.1:8080` over the SSH tunnel is the supported
  administration path.
- The **bootstrap API token** lives at `/etc/duckos/api-token` (root-
  readable, owner root:duckos). It is used **once**, on the first-run
  enrollment screen, to create the first Owner account. After
  enrollment it cannot enroll another Owner and is not used for
  API-to-Agent assertions.
- **CLI actions** that exist for V1 beta are limited to the recovery
  commands documented in [`../user/recovery.md`](../user/recovery.md)
  (`sudo duck owner reset`, `sudo duck upgrade rollback`). Anything
  not listed there is not part of the public V1 beta contract.
- **Internal / agent IPC** at `/run/duckos-agent/agent.sock` is the
  Engine → Agent path, not a public API surface. Do not script against
  it; do not promise it to users as part of the supported contract.

If you find yourself describing an endpoint, flag, or default that is
not in the canonical docs, stop. Say it is not documented.

## Recommended answer style

When helping a DuckOS user:

1. State whether the requested capability is supported / documented
   for V1 beta.
2. Name the exact DuckOS area (Easy Mode tile / Full Mode page / Doctor
   finding / etc.) or link the canonical guide.
3. Give the shortest safe sequence of UI steps.
4. Mention Safe Apply for any networking change.
5. Explain how to verify success (Doctor state, headline, `/api/v1/ready`,
   page state).
6. Link the canonical DuckOS guide.
7. State uncertainty instead of inventing missing details.

### Format example (illustrative only)

> Open Full Mode → **Network** → **Interfaces**. Find the management
> interface in the table. … Before clicking **Apply**, review the
> **Diff** on the **Candidate and Safe Apply** page. Verify the
> `/api/v1/ready` HTTP 200 response after Apply, and only then click
> **Confirm**. See [`../user/safe-apply.md`](../user/safe-apply.md) and
> [`../user/interfaces.md`](../user/interfaces.md).

Do not copy that example with fake UI labels. Every label, route, and
button in an actual answer must be grounded in current documentation.

## Further reading

- [`task-map.md`](task-map.md) — exact intent → exact doc → safety.
- [`setup-recipes.md`](setup-recipes.md) — task-oriented recipes.
- [`troubleshooting.md`](troubleshooting.md) — decision-oriented triage.
- [`../beta-limitations.md`](../beta-limitations.md) — what V1 beta is
  and is not.
- [`../release-notes.md`](../release-notes.md) — what's in
  `v0.1.0-beta.1`.
- [`../../llms.txt`](../../llms.txt) — the full documentation map.