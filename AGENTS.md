# AGENTS.md — DuckOS engineering policy for coding agents

This document is the canonical engineering policy for any coding agent
(AI assistant, IDE bot, or human contributor working as one) operating in
the DuckOS repository. It is neutral with respect to vendor or model.

It is read together with:

- [`README.md`](README.md) — public landing page.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — practical contribution rules.
- [`SECURITY.md`](SECURITY.md) — vulnerability disclosure.
- [`docs/architecture.md`](docs/architecture.md) — component boundaries.
- [`docs/product-spec-1.0.md`](docs/product-spec-1.0.md) — V1 scope and readiness matrix.
- [`docs/adr/`](docs/adr/) — consequential architectural decisions.

A change that violates this document is a defect even if the build and tests pass.

---

# 1. Repository purpose

DuckOS is a Linux network operating platform for routers, gateways, VPS/edge
nodes, and self-hosted network appliances. The repository holds one coherent
implementation: the React/TypeScript UI, the public API + Engine, the
privileged Agent, the Linux adapters, the installer, the documentation, and
the engineering evidence.

# 2. Architecture boundaries

There are exactly four tiers. Do not collapse them.

```
  Browser UI (React / TypeScript)
        │  HTTPS, authenticated bearer
        ▼
  Public API  /  Engine          (cmd/duck-api, internal/api, internal/engine)
        │  authenticated, versioned, role-checked JSON over loopback / unix socket
        ▼
  Privileged Agent               (cmd/duck-agent, internal/agent)
        │  bounded, fixed, scoped Linux mutations only
        ▼
  Linux kernel + systemd + networkd + nftables + dnsmasq + frr + WireGuard + Xray
```

The browser MUST NOT call the privileged Agent IPC directly. The Agent MUST
NOT expose arbitrary shell or arbitrary command execution. The Linux
adapters MUST be the only path that mutates /etc, kernel state, network
configuration, or service lifecycle.

# 3. Protected invariants

The following invariants are load-bearing. A change that breaks one is a
safety defect, not a refactor.

1. **The UI communicates only through the authenticated public API / Engine.**
   Never add a direct browser → privileged Agent IPC path. Never expose the
   Agent IPC on a routable listener.
2. **The privileged Agent owns privileged Linux mutations.** No UI/CLI
   code, no Engine code, no test code outside `cmd/duck-agent/...` may issue
   mutations to /etc, kernel tables, network state, or service lifecycle.
3. **Generated objects remain inspectable** and carry ownership and
   provenance metadata where the system supports it. A mutation that
   removes audit breadcrumbs is a defect.
4. **Safe Apply / commit-confirmed semantics are not bypassable for network
   changes.** No shortcut, "apply now", or optimistic commit may skip the
   transaction model described in `docs/architecture.md` and
   `docs/update-system.md`.
5. **The active management path is never casually mutated.** A Safe Apply
   that touches the interface carrying the management session MUST be
   guarded by anti-lockout, preflight, and a rollback path.
6. **Configured / desired / observed / verified are separate states.**
   No code may collapse them. Render them as four distinct UI states.
7. **API reachable does NOT mean system healthy.** `/api/v1/health` proves
   reachability of the public API only.
8. **Doctor failure does NOT mean zero findings.** Doctor request error
   is "Doctor unavailable", never "0 findings".
9. **Missing observation does NOT mean fresh observation.** An empty or
   absent `overview.observedAt` is "incomplete", not "fresh".
10. **Future or invalid observation timestamp does NOT mean fresh.** A
    timestamp ahead of now (beyond a small skew tolerance) is "future /
    stale", not "fresh".
11. **Unknown evidence must not become positive evidence.** Doctor empty
    findings ≠ verified healthy. Future timestamp ≠ fresh observation.
12. **Localizations.** Exactly these UI languages: EN, RU, DE, ES. New
    locales require a translation review, not a guess.
13. **One global search.** Do not introduce a second command palette or
    omnibox. The existing `searchHelp` / `searchObjects` integration in
    `web/src/search/objects.ts` is the single canonical entry point.
14. **Easy Mode and Full Mode are two interfaces to the same configuration
    model.** They MUST NOT diverge in what they configure; they may differ
    only in how much of the configuration model they expose.
15. **Unsupported features must be represented honestly.** Pages that
    cannot be implemented truthfully against the current public API MUST
    appear as "design-only" / "not yet" — never as fake working pages.
16. **No mandatory SaaS dependency for core local operation.** The full
    feature set MUST be runnable without any third-party service.
17. **IPv6-first data model.** Every internal interface address, route,
    DNS record, and policy rule must support IPv6 natively. IPv4-only
    shortcuts are not acceptable.

# 4. Networking safety rules

- Network changes go through Safe Apply. There is no other path.
- The forwarding guard chain `duckos_apply_guard` is owned by Safe Apply
  and is released only by `FirewallHost.ReleaseForwardGuard`. Never
  release it from a different layer.
- Boot recovery runs **before** `systemd-networkd.service` so the host
  routes and rules can be reconstructed. The recovery unit is
  intentionally NOT `PrivateNetwork=yes` — it must inspect rtnetlink.
- Real systemd unit files (`packaging/systemd/duck-network-recover.service`)
  are the source of truth. The embedded Go unit accessor
  (`packaging/systemd/units.go`) reads them at compile time. Do not
  duplicate the unit strings inside Go.

# 5. Management-interface safety

- The management LAN or interface is named and treated specially. A Safe
  Apply that would remove the management anchor is refused at validate.
- The scoped networkd reload set keeps the management anchor outside the
  scoped write set, but the daemon host still sees the full durable
  before/next pair so Render + Validate can admit the candidate.
- The privileged Agent unit is bound by `PrivateNetwork=no`, `IPAddressDeny=any`,
  `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK`, and an
  empty `CapabilityBoundingSet`. Do not relax these in a cleanup PR.

# 6. Safe Apply rules

- A Safe Apply has exactly three observable states: `pending`,
  `applying`, `confirmed`. There is no fourth.
- The transaction is durable. A crashed Apply that did not Confirm must
  be re-discoverable from disk, not reconstructed in memory.
- The watchdog is independent of the Engine. The Engine MAY die during
  Apply; the Agent reports the actual outcome.
- The Apply handler MUST NOT auto-Confirm. Auto-Confirm would defeat the
  rollback path.

# 7. API / Agent privilege boundary

- The Agent accepts a bounded, fixed set of operation names. Reject any
  new "open a shell" / "run a script" / "execute" verb.
- `/agent/v1/*` is private IPC. The public CLI MUST NOT forward calls to
  it through the public HTTP route; doing so silently produces 404s.
- Every Agent call is authorized against the role that the caller proved
  possession of at the API layer. The Agent does not redo full RBAC, but
  it does enforce the bind to a specific operation ID and a known role.

# 8. Health-state semantics

The centralized derivation in `web/src/state/health.ts` is the single source
of truth. Pages MUST NOT derive their own overall verdict.

Verdict enum (no "good" member):

- `unreachable` — API cannot be reached.
- `attention` — Doctor or overview raised findings.
- `stale` — observation timestamp is too old or in the future.
- `incomplete` — evidence is insufficient.

Doctor findings are returned by `doctorFindingsFromResult` as
`DoctorFinding[] | null`. `null` means "Doctor did not respond", which is
**different** from `[]` ("Doctor explicitly reported zero findings").

`/api/v1/health` proves reachability. It carries no observation
freshness, no component health, no time trust.

# 9. Localization rules

- All user-visible strings live in `web/src/i18n/v4-strings.ts`.
- Add the key to the type definition, then provide the value in every
  locale: EN, RU, DE, ES.
- No hardcoded user-facing English strings in components. Lint the diff
  before committing.
- Locale-neutral symbols (icons, numerals, units) do not need translation.

# 10. Frontend rules

- React 19 with hooks. No class components. No Redux.
- The frozen frontend baseline is the branch's `275c2cb`. Do not redesign
  the V1 UI without an explicit product requirement and an ADR.
- TypeScript strict mode. `any` requires a one-line comment justifying it.
- One global search (`web/src/search/objects.ts`). No second palette.
- The `duckos-control.log` direct file write is removed. All structured
  logging goes through the existing DuckOS logger; ad-hoc `/tmp/*.log`
  files are not added.
- No browser-side secrets. Session tokens live in sessionStorage only,
  not localStorage. Never write a token into the DOM.

# 11. Backend rules

- Go 1.27.1. Repository-local toolchains (`make deps`) — never require a
  system-wide Go install beyond the bootstrap script.
- All mutations to /etc, kernel state, services, or the network go
  through a Linux adapter. No inline shell, no `exec.Command("sh", ...)`
  in the Engine or API layers.
- The Engine talks to the Agent over an authenticated, role-checked
  JSON loopback. New operation verbs require a security review.
- The branded ASCII mark is `pkg/brand/mark.txt`. Installer output, the
  web rubber-duck SVG, and the mark file all use it. Do not fork the
  mark across files.

# 12. Test expectations

- `make check` runs formatting, static analysis, Go race/unit/contract
  tests, frontend TypeScript and component tests, and installer
  preflight tests.
- `make build-all` produces the production UI bundle and Linux
  cross-builds for amd64 / arm64 / armv7.
- A change is not complete because tests pass; it must also declare the
  evidence it does NOT yet have (native Linux, real reboot, real
  forwarding, hardware variance).
- Tests must wait for async state. Use `findBy` / `waitFor` / controlled
  promises — never `findByText` after a `Promise.resolve` resolves
  synchronously, that does not test the loading state.
- Doctor / health tests must use a controlled Promise so the loading
  render is observable.

# 13. Release rules

- A release is a separate, reviewed decision. A green CI does not
  authorize a release tag, a published artifact, or a public
  announcement.
- Installer qualification must be performed on the exact commit being
  released. Cross-build success without platform-qualification is not
  release evidence.
- Release notes describe the *actual* state of the build, including
  which subsystems remain gated.
- Update flows MUST preserve the `Active + Previous` invariant.

# 14. Things agents must NEVER do

- Do not push to remote, open a PR, or tag a release. A human does that.
- Do not rewrite Git history.
- Do not merge a feature branch without explicit human review.
- Do not introduce a second search / command palette / omnibox.
- Do not invent a verified-health backend field. The frontend must wait
  for the backend to grow it.
- Do not relax `PrivateNetwork=no` on the recovery unit.
- Do not auto-Confirm a Safe Apply.
- Do not collapse configured / desired / observed / verified into a
  single UI state.
- Do not show "All systems healthy" or "Everything is working" without
  verified evidence in the production contract.
- Do not write secrets, tokens, real keys, real customer data, or
  production network details into the repository.
- Do not produce a fake release announcement. The code is the source of
  truth.
- Do not enable indexing, change repository visibility, or pick a
  license without a separate launch decision.
- Do not disable Safe Apply on the management interface.
- Do not introduce a new model or vendor reference in code or docs.

---

If a task appears to require any of the actions above, **stop and ask**.
A clean refusal is preferable to a clever violation.