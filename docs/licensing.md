# DuckOS licensing

This page describes how the DuckOS Community edition is licensed, how
that license differs from commercial use of the DuckOS marks, and what
to expect from any future commercial / dual-licensing track.

It is a plain-language summary that points back to the canonical
documents. It is not a substitute for legal advice.

## DuckOS Community source license

DuckOS Community source is distributed under the **GNU General Public
License version 3 — only** (no later version).

- Canonical text: [`LICENSE`](../LICENSE)
- SPDX identifier: `GPL-3.0-only`
- Project intent: `GPL-3.0-only`, not `GPL-3.0-or-later`

Under the GPL-3.0-only you may, subject to the license terms:

- use DuckOS Community for any purpose, including commercial purposes;
- study and modify the source;
- redistribute the source and your modifications, under the same
  GPL-3.0-only terms, including the obligation to provide source to
  recipients of any binary you distribute.

The GPL does not transfer ownership of the code. Individual
contributors retain copyright in their contributions; see
[`COPYRIGHT.md`](../COPYRIGHT.md) and [`CLA.md`](../CLA.md).

## Why "only" and not "or-later"

The DuckOS project owner has selected `GPL-3.0-only` rather than
`GPL-3.0-or-later`. The practical effect for recipients is the same
for version 3 of the GPL. The difference matters for the project:

- Each released DuckOS Community version is licensed to recipients
  under the exact terms in force at release time.
- The project retains the freedom to choose a different license model
  for future versions, including commercial or dual licensing, without
  retroactively changing the license under which any already-released
  DuckOS Community version was received.
- Already-released GPL versions remain genuinely open source under
  GPL-3.0-only; they are not converted, relicensed, or revoked by any
  future commercial decision.

## Commercial / dual-licensing

The DuckOS project intends to preserve **two parallel tracks**:

1. **DuckOS Community** — the open-source edition under GPL-3.0-only,
   available to anyone who accepts the GPL terms.
2. **Separate commercial licensing** — for organizations whose
   redistribution, integration, or product model is not compatible with
   the GPL's source-sharing obligations.

Examples where a separate commercial agreement may make sense include:

- closed-source redistribution of DuckOS as part of a proprietary
  product or appliance;
- white-label hardware appliances shipped under a non-DuckOS brand;
- OEM and embedded products where the licensee wants predictable
  terms rather than GPL obligations flowing through their distribution
  chain;
- negotiated commercial integration, support, or custom engineering
  arrangements.

> Commercial licensing information will be published by the project
> owner as the project approaches commercial availability. No pricing,
> no sales contact, and no company identity is implied by this
> document or by the existence of the open-source release.

Commercial licensing for **already-released DuckOS Community versions**
is not possible: those versions were received by their recipients
under GPL-3.0-only, and the GPL cannot be unilaterally revoked for
already-distributed copies. Future versions, including new features
and contributions added after the commercial track is published, may
be made available under commercial terms at the project owner's
discretion.

## Contributor licensing

External code contributions are accepted under a **Contributor License
Agreement** described in [`CLA.md`](../CLA.md). Until the CLA is
finalized, see [`CONTRIBUTING.md`](../CONTRIBUTING.md) for the current
status and what kinds of contribution are welcomed now.

Under the intended CLA model:

- the contributor retains copyright and authorship;
- the contributor grants the project owner a broad, perpetual,
  worldwide, royalty-free license sufficient to use, modify,
  distribute, sublicense, and relicense the contribution across both
  the open-source Community track and any future commercial track;
- attribution is preserved in the project's contribution records.

## Trademarks and brand

Source-code licensing and trademark / brand use are **separate**.

GPL-3.0-only lets you use, modify, and redistribute the source under
GPL terms. It does **not** grant any right to use the **DuckOS** name,
the DuckOS wordmark, the rubber-duck logo, or other product identity
assets to identify your build as an official DuckOS release.

Factual references such as *"based on DuckOS"* or *"compatible with
DuckOS"* are permitted when accurate and not misleading. See
[`TRADEMARKS.md`](../TRADEMARKS.md) for the full policy.

## Third-party components

DuckOS distributes third-party components that carry their own
notices and licenses. These notices are preserved in the relevant
component locations and remain authoritative for those components.
See [`COPYRIGHT.md`](../COPYRIGHT.md) for how third-party notices are
handled at the project level, and `THIRD_PARTY_NOTICES.md` (if present
in a given distribution) for the per-component inventory.

## Reporting issues with licensing or attribution

If you believe a file or component is missing a required notice,
carries an incorrect attribution, or conflicts with its stated
license, please open an issue. For security-sensitive concerns, see
[`SECURITY.md`](../SECURITY.md).

## Related documents

- [`LICENSE`](../LICENSE) — canonical GPL-3.0-only text.
- [`COPYRIGHT.md`](../COPYRIGHT.md) — authorship and trademark
  separation.
- [`CLA.md`](../CLA.md) — DRAFT contributor license agreement.
- [`TRADEMARKS.md`](../TRADEMARKS.md) — DuckOS marks and brand use.
- [`CONTRIBUTING.md`](../CONTRIBUTING.md) — practical contribution
  policy.
- [`SECURITY.md`](../SECURITY.md) — vulnerability reporting.

## Source-tree license identification (SPDX)

The repository currently relies on the root [`LICENSE`](../LICENSE) and
the documents listed above for license identification, rather than on
per-file SPDX headers. The repository does not have an established
per-file SPDX convention; only one source file carries an SPDX header,
and there is no convention about its form or placement.

For the **V1 beta** this is sufficient: the root `LICENSE` and the
documents above are normative. New contributors will be expected to
follow the contributor licensing process described in
[`CLA.md`](../CLA.md) once it is finalized; per-file SPDX headers are
not required for beta acceptance.

**Recommended future work** (not part of this release preparation):

- Decide whether every source file should carry an SPDX header
  (`SPDX-License-Identifier: GPL-3.0-only`), or whether an explicit
  notice on root files and on each top-level directory is enough.
- If per-file SPDX is adopted, define a lightweight convention
  (single-line SPDX at the top of each file, with copyright lines
  optional and rolled up to a per-year contributor list).
- Audit any stray headers that pre-date the GPL-3.0-only decision
  and reconcile them with the project license. As of the legal
  hygiene pass before the V1 beta, the previously-flagged stray
  `Apache-2.0` SPDX header on
  `internal/engine/config_diff_coordinator_test.go` has been
  classified as an accidental / stale header on a DuckOS-original
  file (added in commit `04f799a` by the DuckOS team as a BD-001
  regression test) and removed; see
  [`docs/third-party-licenses.md`](third-party-licenses.md).

This recommendation is recorded here so it is not lost. The stray
header above has been resolved during the legal hygiene pass; the
remaining items are deferred.