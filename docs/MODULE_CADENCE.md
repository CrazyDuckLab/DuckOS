# DuckOS Beta Module Cadence Policy

This document is the lightweight product policy for picking
which catalog pages to graduate from DESIGN-ONLY to IMPLEMENTED
in each beta.

## Rule

Every beta aims to graduate approximately **TWO** catalog
modules end-to-end.

## Definition of "implemented"

A module counts as implemented only when ALL of the following
hold:

- the typed model exists in `internal/<module>`
- the platform runtime runs on Linux (and refuses cleanly
  on other GOOS/GOARCH where the contract requires platform
  support)
- the Agent IPC exposes the surface (engine, public API, audit
  hooks)
- the public API exposes the surface with typed schemas and
  appropriate authorization
- the CLI or an equivalent external tool can drive the module
  end-to-end (no internal-package-only code path)
- the Web UI renders real state with EN/RU/DE/ES i18n
- the runtime produces honest observed state; no fake success
  and no rendering of placeholders as live data
- tests cover the model, the runtime, the Agent, the API and
  the UI (Go + Vitest where applicable)
- the catalog entry `implemented: true` is the last edit

A page does NOT count as implemented merely because:

- the red placeholder is removed
- an API returns static JSON
- the UI has controls
- an internal package exists
- the spec is documented

## Selection preference

When picking the two modules for a beta, prefer modules that:

1. **already have meaningful internal model/runtime work** —
   the bounded change is much smaller.
2. are **high everyday usefulness** for the operator.
3. are **bounded scope** — avoid touching unrelated surfaces.
4. are **testable without exotic hardware** — VMs are fine;
   bare-metal ODROID XU4 is reserved for the ARMv7 installer
   round.
5. **fill a visible hole in the UI** — a module the operator
   notices is missing on first contact.

A difficult module may consume both slots. Two small modules
do not necessarily equal one big module — pick by impact, not
by line count.

## Discipline

- Do not implement unrelated catalog pages opportunistically.
- New findings during exploration are recorded for future beta
  selection, not silently added to the current round.
- The model layer is preserved even when the runtime lands in
  a later round — the catalog entry stays `implemented: false`
  until the runtime is real.
- Refactor work belongs in a dedicated round, not bundled into
  a feature round.

## History

| Round | Module #1 | Module #2 | Notes |
|---|---|---|---|
| Round 3 | B1 — vault Put HTTP API (F-009) | B2 — xray-core backend delivery (F-010, F-014, F-015) | new bounded code paths |
| Round 4 | P090 Duck Speed Lab runtime + tests | P065 UPnP/NAT-PMP/PCP/Wake runtime | backend landed; Agent + UI shells deferred |

Future rounds will follow the same cadence.

## How to read the catalog

`web/src/pages/registry.ts` lists every V1 page with an
`implemented: true|false` flag. The flag is the only canonical
truth for "is this page end-to-end?" — do not rely on the
existence of `internal/<module>` or a placeholder URL alone.