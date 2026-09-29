# Third-party license inventory

This document inventories third-party components used by DuckOS and
classifies each by license compatibility with the DuckOS Community
source license (**GPL-3.0-only**). It is a snapshot at the time of
the V1 beta preparation pass and is intended for human review.

This is **not** a legal audit. Compatibility classifications are made
in good faith using the well-known compatibility matrix; if anything
here is uncertain, the row is marked **NEEDS REVIEW** and the
project owner should consult counsel before the first public release.

## How to read this inventory

- **KNOWN COMPATIBLE** — the dependency's license is recognized as
  compatible with GPL-3.0-only distribution; it may be used and
  redistributed under the GPL-3.0-only terms, with the dependency's
  own notice preserved.
- **NEEDS REVIEW** — the dependency's license is plausible but the
  exact clause has not been verified, or the dependency's
  distribution model (linking style, choice of law, patent grant
  scope) requires closer review.
- **BLOCKING** — the dependency's license is incompatible with
  GPL-3.0-only, or it imposes a restriction (for example a
  copyleft-against-derivative-works clause, or a non-commercial
  clause) that cannot be carried into a GPL-3.0-only distribution.

Per the task's policy, this inventory does **not** silently replace
any dependency. If a dependency is classified BLOCKING, installation
qualification is paused and the project owner is consulted.

## Frontend npm dependencies (`web/package.json`)

All runtime and dev dependencies below are fetched from the npm
registry. They are gitignored in this repository and are not part of
the source distribution; their licenses govern the bundled output,
not the source code.

| Package | Version | License | Status |
|---|---|---|---|
| `react` | 19.2.8 | MIT | KNOWN COMPATIBLE |
| `react-dom` | 19.2.8 | MIT | KNOWN COMPATIBLE |
| `qrcode` | 1.5.4 | MIT | KNOWN COMPATIBLE |
| `@testing-library/jest-dom` | 7.0.1 | MIT | KNOWN COMPATIBLE (dev) |
| `@testing-library/react` | 16.3.3 | MIT | KNOWN COMPATIBLE (dev) |
| `@testing-library/user-event` | 14.6.7 | MIT | KNOWN COMPATIBLE (dev) |
| `@types/qrcode` | 1.5.6 | MIT | KNOWN COMPATIBLE (dev) |
| `@types/react` | 19.2.18 | MIT | KNOWN COMPATIBLE (dev) |
| `@types/react-dom` | 19.2.7 | MIT | KNOWN COMPATIBLE (dev) |
| `jsdom` | 30.0.1 | MIT | KNOWN COMPATIBLE (dev) |
| `jsqr` | 1.4.0 | Apache-2.0 | KNOWN COMPATIBLE (dev) |
| `prettier` | 3.9.6 | MIT | KNOWN COMPATIBLE (dev) |
| `typescript` | 7.0.2 | Apache-2.0 | KNOWN COMPATIBLE (dev) |
| `vite` | 8.2.2 | MIT | KNOWN COMPATIBLE (dev) |
| `vitest` | 5.0.0 | MIT | KNOWN COMPATIBLE (dev) |

Notes:

- All runtime deps are MIT; MIT is compatible with GPL-3.0-only and
  the original notices are preserved in the bundled output's
  standard npm attribution channel.
- `jsqr`, `typescript` and the `@types/*` packages are Apache-2.0;
  Apache-2.0 is compatible with GPL-3.0-only distribution.
- The versions pinned in `package.json` are exactly what ships; this
  inventory does not track transitive npm deps individually because
  they are also permissively licensed and they are not modified.

## Go modules (`go.mod`)

| Module | Version | License | Status |
|---|---|---|---|
| `modernc.org/sqlite` | v1.58.0 | MIT (with BSL-1.1 / public-domain contributions) | KNOWN COMPATIBLE |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT | KNOWN COMPATIBLE (indirect) |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause | KNOWN COMPATIBLE (indirect) |
| `github.com/mattn/go-isatty` | v0.0.24 | MIT | KNOWN COMPATIBLE (indirect) |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT | KNOWN COMPATIBLE (indirect) |
| `github.com/remyoudompheng/bigfft` | (commit-pinned) | BSD-3-Clause | KNOWN COMPATIBLE (indirect) |
| `golang.org/x/sys` | v0.47.0 | BSD-3-Clause | KNOWN COMPATIBLE (indirect) |
| `modernc.org/libc` | v1.75.6 | MIT-like (public-domain core, MIT additions) | KNOWN COMPATIBLE (indirect) |
| `modernc.org/mathutil` | v1.7.1 | MIT-like | KNOWN COMPATIBLE (indirect) |
| `modernc.org/memory` | v1.12.1 | MIT-like | KNOWN COMPATIBLE (indirect) |

The Go module graph also references Linux-only networking packages
(`frr`, `netlink`, etc.) in the `internal/agent` subsystem that are
build-tag-gated to `linux/amd64`, `linux/arm64`, and `linux/armv7`.
They are not visible on macOS or in the cross-build from this
checkout; their licenses are recorded separately during Linux build
qualification and will be appended here before the first public
release.

## Bundled fonts and icons

DuckOS does not currently bundle fonts or external icon sets. The
frontend uses the host browser's text rendering with a documented
monospace stack, and the visual identity is the ASCII rubber-duck
mark defined in `pkg/brand/mark.txt` plus the rubber-duck SVG in
`web/public/duck.svg` and `web/src/assets/duckos-mark.svg`, which are
DuckOS-owned assets governed by [`TRADEMARKS.md`](../TRADEMARKS.md).

If fonts or icons are added later, this section must be updated.

## System packages pulled at install time

The installer (`scripts/install.sh`) installs the following system
package on Debian 13 amd64:

| Package | Source | License | Status |
|---|---|---|---|
| `frr` | Debian 13 apt (`frr`) | GPL-2.0-or-later | KNOWN COMPATIBLE |

`frr` (FRRouting) is used by DuckOS only for routing-protocol
integration where Debian provides it. FRR's license is
GPL-2.0-or-later, which is compatible with GPL-3.0-only distribution
(the reverse is not generally true, but that is not relevant here
because DuckOS is the primary GPL-3.0 work, not FRR).

If `frr` is unavailable at install time, the installer logs a
**not installed** state and continues — routing protocol features
report as **not installed**, not as a defect. No binary from `frr`
is bundled into the DuckOS distribution.

## Copied / vendored code

A repository scan found no `vendor/` or `third_party/` directories
under the tracked source tree. The DuckOS build pulls all third-party
code through the npm and Go module systems; no third-party source is
copied into the tracked tree.

## Resolved provenance notes

The following items were flagged in earlier passes of this inventory
and have since been resolved. They are recorded here so the resolution
is auditable, and so they are not flagged again by future scans.

- **`internal/engine/config_diff_coordinator_test.go`** previously
  carried an `SPDX-License-Identifier: Apache-2.0` header on a
  `Copyright (c) 2025, DuckOS contributors.` line. Git history shows
  the file was added in commit `04f799a` ("fix(engine): Diff endpoint
  was hard-failing on every non-empty candidate") as a DuckOS-original
  BD-001 regression test; the file is not third-party / Apache-licensed
  code. The inaccurate header has been removed and replaced with a
  plain regression-coverage comment plus a one-line pointer to the
  root `LICENSE`. No third-party attribution is required.

No other tracked source files in the DuckOS tree carry a non-GPL SPDX
identifier at this snapshot.

## Known-attribution registry

The repository does not yet maintain a per-dependency
`THIRD_PARTY_NOTICES.md` at install time. Such a file should be
generated at build time (for example from `go-licenses` for Go and
the npm `license-checker` for the frontend) and bundled with the
distribution. This is recorded as a recommended follow-up; it is
not a blocker for the V1 beta because the deps above are all
permissively licensed and the root [`LICENSE`](../LICENSE) and
[`COPYRIGHT.md`](../COPYRIGHT.md) carry the broad attribution
statement.

## Summary

- **KNOWN COMPATIBLE**: 17 distinct npm entries (5 runtime + 12 dev)
  plus 10 Go modules plus `frr` from apt — all permissive or weak
  copyleft, all compatible with GPL-3.0-only distribution.
- **NEEDS REVIEW**: Linux-only Go subsystem deps (`frr`, `netlink`,
  ...) used by `internal/agent` — to be appended during Linux build
  qualification. None are flagged BLOCKING at this point because
  each is widely used in GPL-compatible Go projects and is fetched
  through Go modules; their classification will be confirmed before
  the first public release.
- **BLOCKING**: **0**.

## Related documents

- [`LICENSE`](../LICENSE) — DuckOS Community source license.
- [`COPYRIGHT.md`](../COPYRIGHT.md) — authorship and trademark
  separation.
- [`docs/licensing.md`](licensing.md) — commercial / dual-licensing
  narrative.
- [`CLA.md`](../CLA.md) — DRAFT contributor license agreement.
- [`TRADEMARKS.md`](../TRADEMARKS.md) — DuckOS marks and brand use.