# DuckOS documentation

This directory contains maintained documentation for architecture, product
scope, safety invariants, development, and test evidence.

## Start here

- [`requirements/product.md`](requirements/product.md)
  defines the product requirements.
- [`product-spec-1.0.md`](product-spec-1.0.md) is the living DuckOS 1.0 scope and
  readiness matrix.
- [`architecture.md`](architecture.md) describes component ownership and
  dependency direction.
- [`adr/`](adr/) records consequential architectural decisions.

## For administrators

- [User guide](user/README.md) — getting started, Easy Mode,
  Full Mode, Safe Apply, Doctor, Recovery, Troubleshooting.
- [Safe Apply](user/safe-apply.md) — the only path from candidate
  to active state.
- [Recovery](user/recovery.md) — pre-networkd boot recovery, Safe
  Apply rollback, Owner reset.
- [Troubleshooting](user/troubleshooting.md) — triage guide.

## For developers

- [`AGENTS.md`](../AGENTS.md) — canonical engineering policy for
  coding agents. Read this first.
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — practical contribution
  rules.
- [`architecture.md`](architecture.md) — component ownership and
  dependency direction.
- [`adr/`](adr/) — consequential architectural decisions.
- [`security-model.md`](security-model.md) — threat model and
  trust boundaries.
- [`config-model.md`](config-model.md) — declarative desired state.
- [`route-compilation.md`](route-compilation.md) — deterministic
  renderer.
- [`network-understanding.md`](network-understanding.md)
- [`network-access-policy.md`](network-access-policy.md)
- [`update-system.md`](update-system.md)
- [`storage-policy.md`](storage-policy.md)
- [`ux-principles.md`](ux-principles.md)
- [`ux-v1/`](ux-v1/) — V1 UX design system.
- [`development.md`](development.md) — local dev loop.

## Public project state

- [`roadmap.md`](roadmap.md) — V1 beta, LATER, FUTURE.
- [`beta-limitations.md`](beta-limitations.md) — what V1 is **not**.
- [`release-notes-draft.md`](release-notes-draft.md) — v0.1.0-beta.1
  release-note draft (not yet released).
- [`launch/`](launch/) — private launch preparation.

## Licensing

- [`../LICENSE`](../LICENSE) — canonical GPL-3.0-only text.
- [`../COPYRIGHT.md`](../COPYRIGHT.md) — authorship and trademark
  separation.
- [`../CLA.md`](../CLA.md) — DRAFT contributor license agreement.
- [`../TRADEMARKS.md`](../TRADEMARKS.md) — DuckOS marks and brand use.
- [`licensing.md`](licensing.md) — commercial / dual-licensing
  narrative.
- [`third-party-licenses.md`](third-party-licenses.md) — third-party
  dependency license inventory.

## Install and hardware

- [`install.md`](install.md)
- [`hardware-support.md`](hardware-support.md)
- [`hardware-compatibility.md`](hardware-compatibility.md)
- [Deployment use cases](use-cases/home-gateway.md),
  [edge nodes](use-cases/edge-node.md),
  [ARM](use-cases/arm.md),
  [routing](use-cases/routing.md),
  [client provisioning](use-cases/client-provisioning.md)
- [Migration planning](use-cases/migration.md)

## Engineering evidence

- [`testing.md`](testing.md)
- [`evidence/executable-foundation.md`](evidence/executable-foundation.md)
- [`evidence/qa/`](evidence/qa/) — qualification evidence for each
  finding.
- [`audits/`](audits/) — date-stamped backend audit handoffs.
- [`requirements/product.md`](requirements/product.md) — the product
  requirements.
- [`product-spec-1.0.md`](product-spec-1.0.md) — the V1 scope and
  readiness matrix.
- [`open-decisions.md`](open-decisions.md) — registered open
  decisions.

Documentation is versioned with implementation. A behavior/specification
disagreement is a defect unless an intentional change is recorded in the
product specification or an ADR.

## Development continuation

[Checkpoint entry point](../CONTEXT.md) · [Project state](handoff/PROJECT_STATE.md) ·
[Frozen V1 scope](handoff/V1_SCOPE.md) · [Current roadmap](handoff/CURRENT_ROADMAP.md) ·
[Current architecture](handoff/ARCHITECTURE.md) · [Engineering rules](handoff/AGENT_RULES.md)
