# Contributing to DuckOS

DuckOS beta.2 includes a Linux networking runtime and a qualified subset of
operator workflows. Contributions must preserve privilege boundaries and Safe
Apply, and distinguish implementation from observed and qualified behavior.
See [current qualification](docs/qualification-beta2.md) and [limitations](docs/beta-limitations.md).

## Licensing model and contributor rights

DuckOS Community source is distributed under **GPL-3.0-only** — see
[`LICENSE`](LICENSE). Copyright in each contribution stays with the
contributor; see [`COPYRIGHT.md`](COPYRIGHT.md).

The DuckOS project intends to keep two parallel tracks open: the
GPL-3.0-only Community edition, and a future commercial / OEM /
proprietary-integration track for organizations whose distribution
model does not fit GPL obligations. The open-source Community edition
is not affected by the commercial track, and already-released
Community versions cannot be relicensed retroactively. See
[`docs/licensing.md`](docs/licensing.md).

External **code** contributions that would feed future relicensing are
gated by a Contributor License Agreement described in
[`CLA.md`](CLA.md). Until the CLA is finalized:

- **Issues, testing reports, and documentation feedback** are welcome
  under the rules below. No CLA is required for those.
- **External code contributions** that would flow into future
  relicensing are not accepted yet. If you want to propose a code
  change, please open an issue first to discuss scope, design, and
  the timing relative to the finalized CLA.

Contributors are never asked to surrender ownership of their work.
The CLA grants the project the rights it needs to operate both
tracks; authorship and copyright remain with the original contributor.

Trademark use is a separate policy. Contributing code does not grant
any right to use the DuckOS name, wordmark, rubber-duck logo, or
other product identity. See [`TRADEMARKS.md`](TRADEMARKS.md).

Follow the [community conduct policy](CODE_OF_CONDUCT.md) in project discussions.

## Before making a change

1. Read [`PROJECT.md`](PROJECT.md).
2. Read the relevant living specification and accepted ADRs under [`docs/`](docs/).
3. Consult the product requirements in
   [`docs/product-spec-1.0.md`](docs/product-spec-1.0.md)
   when traceability or intent is unclear.
4. Identify the owning subsystem, canonical state, privilege boundary,
   rollback/reboot behavior, storage growth, IPv6 impact, and ARMv7 impact.
5. Distinguish DEV, TEST, and PRODUCTION claims. At present, no PRODUCTION
   behavior or release is supported.

Do not silently resolve a registered open decision in code. A consequential
choice belongs in an ADR, and an intentional scope change belongs in
[`docs/product-spec-1.0.md`](docs/product-spec-1.0.md).

## Git workflow

`main` is the integration branch. Use short-lived branches from current `main`
for normal work; do not create a permanent `develop` branch. Prefer a concise
name such as `feature/config-envelope`, `fix/journal-recovery`, or
`docs/update-contract`.

```sh
git switch main
git pull --ff-only
git switch -c feature/short-topic
```

Keep a branch focused, rebase or otherwise bring it up to date deliberately,
and merge only after its required checks and review pass. The initial
documentation bootstrap may be committed directly to the otherwise empty
`main`; it is an explicit one-time repository-creation step.

Commit messages should state the meaningful outcome in the imperative mood.
Avoid mixed refactors, generated artifacts, credentials, or unrelated local
files in one commit. Inspect the staged diff before committing.

## Local and Linux target workflow

The editing and orchestration checkout lives on the Mac. Remote SSH development
and ad-hoc file synchronization are not part of the workflow. A Linux system
such as `duck-dev-01` is a build/test target only: it receives an exact
committed revision from `CrazyDuckLab/DuckOS` through GitHub.

Do not install, configure, or modify a shared qualification target without
explicit authorization. Use disposable isolated Linux environments for tests
that require a clean host; installed-system witness refusals are not passes.
Results from Linux count as evidence only when tied to
the exact Git commit and environment described by
[`docs/testing.md`](docs/testing.md).

## Implementation discipline

- Keep API/UI/CLI mutations on one Engine semantic path.
- Do not introduce arbitrary command execution across the privileged boundary.
- Treat `/etc` and kernel/runtime state as rendered or observed state, not the
  canonical DuckOS model.
- Preserve coherent transactions and a last-known-good recovery path.
- Preserve the update `Active + Previous` invariant.
- Bound every growing log, metric, event, backup, snapshot, cache, upload, and
  temporary data set.
- Keep IPv4 and IPv6 in core models even when delivery is staged.
- Never hard-code interface names or infer support only from a board model.
- Use synthetic fixtures and reserved example domains/addresses. Never commit
  real keys, tokens, subscription URLs, passwords, customer data, or production
  network details.
- Review maintenance, license, architecture support, security history, and
  resource cost before adding a dependency.

## Tests and evidence

A change is not complete because it builds or an API returns success. Add the
lowest-level deterministic tests with the change and identify the higher
evidence still required. Networking, security, update, storage, and recovery
claims eventually require real Linux boundaries, isolated client traffic,
reboot/failure tests, and native architecture evidence.

On Linux, run `make check` for formatting, static analysis, Go race/unit/contract tests,
TypeScript and browser component tests, and installer preflight tests. Run
`make build-all` for the production frontend and Linux architecture builds.
Applicable integration/security checks remain mandatory; do not turn a skipped
native Linux check into a pass. See [testing](docs/testing.md) and [current qualification](docs/qualification-beta2.md).

## Pull request checklist

- [ ] Scope is focused and the owning subsystem is identified.
- [ ] Relevant product spec, ADR, and operational documentation are updated.
- [ ] No requirement or open decision is silently discarded.
- [ ] No secrets, production identifiers, or unrelated files are included.
- [ ] Tests are included or the missing evidence and reason are explicit.
- [ ] Failure, rollback, reboot, disk-full, ARMv7, and IPv6 implications were
      considered where applicable.
- [ ] User-visible status and errors are honest; mock data is visibly confined
      to DEV/TEST fixtures.
- [ ] The staged diff and generated/dependency changes were reviewed.

Security-sensitive findings should follow [`SECURITY.md`](SECURITY.md), not a
public issue containing exploit details or secrets.
