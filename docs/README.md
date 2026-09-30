# DuckOS documentation

This directory is the documentation hub for DuckOS V1 beta
(`v0.1.0-beta.1`). Every guide an administrator, contributor, or
reviewer needs is reachable from here.

## Release

- [`release-notes.md`](release-notes.md) — what's in
  `v0.1.0-beta.1`, honest beta limitations, install flow.
- [`beta-limitations.md`](beta-limitations.md) — what V1 beta is and
  is not.
- [`../README.md`](../README.md) — public landing page.

## Getting Started

For a new administrator, follow the guides in this order:

1. [`user/getting-started.md`](user/getting-started.md) — fresh install
   to first Owner login.
2. [`user/easy-mode.md`](user/easy-mode.md) — the simplified
   dashboard.
3. [`user/full-mode.md`](user/full-mode.md) — the by-domain catalog.
4. [`user/safe-apply.md`](user/safe-apply.md) — every change that
   touches the network or system.

## Everyday / Easy Mode

- [`user/getting-started.md`](user/getting-started.md)
- [`user/easy-mode.md`](user/easy-mode.md)
- [`user/search-and-inspector.md`](user/search-and-inspector.md) —
  global search and contextual Inspector.

## Networking

- [`user/interfaces.md`](user/interfaces.md)
- [`user/wan.md`](user/wan.md)
- [`user/lan-vlan.md`](user/lan-vlan.md)
- [`user/routing.md`](user/routing.md)
- [`user/dns-dhcp.md`](user/dns-dhcp.md)
- [`user/firewall.md`](user/firewall.md)

## VPN / Tunnels

- [`user/wireguard.md`](user/wireguard.md)
- [`user/xray.md`](user/xray.md)

## Security

- [`security-model.md`](security-model.md)
- [`../SECURITY.md`](../SECURITY.md) — vulnerability disclosure.
- [`tls.md`](tls.md) — TLS / mTLS / ACME posture in V1 beta.

## System

- [`user/system.md`](user/system.md)
- [`architecture.md`](architecture.md) — component ownership and
  dependency direction.
- [`config-model.md`](config-model.md) — declarative desired state.
- [`update-system.md`](update-system.md) — signed `.duck` packages,
  Active + Previous invariant, signed `/api/v1/ready` health probe.

## Diagnostics / Recovery

- [`user/doctor-and-diagnostics.md`](user/doctor-and-diagnostics.md)
- [`user/updates-and-rollback.md`](user/updates-and-rollback.md)
- [`user/recovery.md`](user/recovery.md) — pre-networkd boot recovery,
  Safe Apply rollback, Owner reset.
- [`user/troubleshooting.md`](user/troubleshooting.md)
- [`updater-supervisor.md`](updater-supervisor.md)

## Development / Contributing

- [`../AGENTS.md`](../AGENTS.md) — canonical engineering policy for
  coding agents (read this first).
- [`../CONTRIBUTING.md`](../CONTRIBUTING.md) — practical rules,
  tests, templates.
- [`development.md`](development.md) — local dev loop.
- [`architecture.md`](architecture.md) — component ownership.
- [`adr/`](adr/) — consequential architectural decisions.
- [`product-spec-1.0.md`](product-spec-1.0.md) — V1 scope and readiness
  matrix.
- [`requirements/product.md`](requirements/product.md) — product
  requirements.

## Install and hardware

- [`install.md`](install.md)
- [`hardware-support.md`](hardware-support.md)
- [`hardware-compatibility.md`](hardware-compatibility.md)
- [Home / homelab gateway](use-cases/home-gateway.md),
  [edge node](use-cases/edge-node.md),
  [ARM](use-cases/arm.md),
  [routing](use-cases/routing.md),
  [client provisioning](use-cases/client-provisioning.md)
- [Migration planning](use-cases/migration.md)

## Licensing

- [`../LICENSE`](../LICENSE) — canonical `GPL-3.0-only` text.
- [`../COPYRIGHT.md`](../COPYRIGHT.md) — authorship and trademark
  separation.
- [`../CLA.md`](../CLA.md) — DRAFT contributor license agreement.
- [`../TRADEMARKS.md`](../TRADEMARKS.md) — DuckOS marks and brand use.
- [`licensing.md`](licensing.md) — commercial / dual-licensing
  narrative.
- [`third-party-licenses.md`](third-party-licenses.md) — third-party
  dependency license inventory.

## Public project state

- [`roadmap.md`](roadmap.md) — V1 beta, LATER, FUTURE.
- [`testing.md`](testing.md) — engineering evidence.
- [`open-decisions.md`](open-decisions.md) — registered open decisions.

## UX system

- [`ux-principles.md`](ux-principles.md)
- [`ux-v1/`](ux-v1/) — V1 UX design system.
- [`ui/`](ui/) — UI implementation notes.
- [`brand-mark.md`](brand-mark.md) — brand / wordmark / logo.

## AI & automation assistants

External assistants and automation agents helping users evaluate,
install, configure, understand, or troubleshoot DuckOS should start
from the root [`../llms.txt`](../llms.txt) and these guides:

- [`ai/README.md`](ai/README.md) — assistant guide, source-of-truth
  order, machine-readable product facts.
- [`ai/task-map.md`](ai/task-map.md) — user intent → exact DuckOS
  area → canonical guide → safety note.
- [`ai/setup-recipes.md`](ai/setup-recipes.md) — task-oriented
  recipes (install, enrollment, SSH tunnel access, Safe Apply,
  recovery).
- [`ai/troubleshooting.md`](ai/troubleshooting.md) — decision-
  oriented triage.

---

Documentation is versioned with implementation. A behavior /
specification disagreement is a defect unless an intentional change is
recorded in the product specification or an ADR.