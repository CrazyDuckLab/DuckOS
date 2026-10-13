# DuckOS development workflow

Status: beta.2 source and Linux qualification workflow
Last updated: 2026-10-13

This document defines local builds and how exact source revisions move from
the development Mac to GitHub and Linux build/test targets. Test
execution evidence is kept separately from source and cross-build claims.

## Repository identity

- GitHub repository: `CrazyDuckLab/DuckOS`
- Remote URL: `git@github.com:CrazyDuckLab/DuckOS.git`
- Integration branch: `main`
- Local checkout: `/path/to/DuckOS`

Git is authoritative for product source, build inputs, schemas, migrations,
packaging definitions, documentation, and release provenance. A future
installation's canonical configuration store is separately authoritative for
that installation's desired state; host `/etc` files are not a substitute for
either source.

## Development and target roles

| Surface | Role | Permitted source flow at this stage | Not its role |
|---|---|---|---|
| Apple Silicon Mac | Primary editing, review, local tooling, and orchestration host | Work in the local Git checkout; run repository-owned local checks once they exist | DuckOS Linux runtime or proof of Debian networking behavior |
| GitHub `CrazyDuckLab/DuckOS` | Canonical collaboration and committed-code transport | Receive reviewed commits/branches; identify exact revisions supplied to targets | Runtime canonical router configuration or a release merely because a commit exists |
| `duck-dev-01` Linux host | Future build/test target | Later obtain an exact committed revision from GitHub under a documented test procedure | Remote development workspace, source of uncommitted changes, or current installation target |

Remote SSH is not used for DuckOS development. Do not use `rsync`, SCP, shared
folders, editor Remote SSH, or manual copy/paste to make `duck-dev-01` differ
from a GitHub commit and then treat the result as evidence.

During repository bootstrap, **do not install, provision, configure, or
otherwise modify `duck-dev-01`**. Target access, isolation, dependency
installation, build execution, and result collection belong to a later stage
and require an explicit procedure. This restriction does not weaken the future
requirement for real Linux and native-architecture testing.

## Environment labels

- **DEV**: local source work, deterministic unit/contract tests, fixtures, UI
  design data, and disposable developer experiments. Mocked behavior is labeled.
- **TEST**: isolated Linux/VM/hardware targets with synthetic configuration and
  a known recovery method. Destructive tests run only on explicitly designated
  TEST assets.
- **PRODUCTION**: real network service and user data. DuckOS has no supported
  PRODUCTION build or procedure yet.

Never infer PRODUCTION readiness from a DEV screenshot, a cross-build, a CI
check, or one successful TEST run.

## Git workflow

Use `main` plus short-lived feature/fix/docs branches. There is no permanent
`develop` branch. Start ordinary work from a current `main`:

```sh
git switch main
git pull --ff-only
git switch -c feature/short-topic
```

Use a small branch with a cohesive reviewable outcome. Push the branch to
GitHub, run the applicable checks, review the diff and evidence, then merge to
`main`. Delete the branch after integration when it is no longer needed. The
empty-repository foundation bootstrap is the one expected direct commit to
`main`.

Before committing:

```sh
git status --short
git diff --check
git diff --cached
```

Commit messages are meaningful and imperative. Do not mix unrelated cleanup
with behavior. Never rewrite or discard another contributor's uncommitted work
without explicit coordination.

## Current command surface

Run the complete `make check` suite on Linux: several packages and tests require
Linux kernel interfaces. macOS supports editing, frontend/installer checks and
cross-builds; `GOOS=linux go vet ./...` provides cross-target static analysis,
not native runtime evidence. Do not treat clean-host witness refusals on an
installed target as passes. See [current qualification](qualification-beta2.md).

`bash scripts/bootstrap.sh` downloads SHA-256-pinned Go 1.27.1 and Node 24.20.0
into ignored `.tools/`. It supports macOS and Linux amd64/arm64, requires curl,
tar and a SHA-256 utility, and does not install host packages. Native Go race
tests require a C compiler (Apple Command Line Tools or Debian build-essential).
ARMv7 is a cross-build target; the current Node toolchain is not a native ARMv7
build-host promise. Runtime Go binaries are static (`CGO_ENABLED=0`).

```sh
bash scripts/bootstrap.sh
make deps                 # npm ci from the committed lockfile
make format               # gofmt and shared schema type generation
make check                # Go vet/race/tests + schema + frontend + installer
make build                # dist/duckos-linux-amd64
make build-all            # additionally Linux arm64 and armv7
```

All package versions are exact. `web/package-lock.json` records transitive
integrity; no Go module dependencies are currently needed. Toolchain archives
and checksum provenance are in `scripts/bootstrap.sh`. Dependency review uses
`npm --prefix web audit`; this is separate from release security qualification.

Each payload includes binaries, static UI, installer, unit templates, VERSION,
BUILD-INFO.json, and a complete SHA256SUMS inventory. Metadata identifies commit,
dirty state, tools and target. Internal versions `1.0.0-dev.<commit>[.dirty]`
are build identifiers, never public release tags. Build outputs are ignored;
prior local output is preserved under `dist/.previous-*` for inspection.
The same clean commit/toolchain produces deterministic payload bytes; archive
timestamp normalization and signed release provenance remain future work.

The API can run on Mac against an absent Agent to verify liveness and failure
UX; the Linux Agent refuses non-Linux execution. Tests use explicitly synthetic
fixtures only where isolating a component. A live Linux-state dashboard is
tested on a Linux target, not emulated with invented Mac data. Installation and
administrative loopback forwarding commands are in [install.md](install.md).

After pushing a verified integration commit, rebuild from its
clean SHA for a deployable payload. No uncommitted tree is target evidence.

## Linux build/test target contract

When a later roadmap gate authorizes `duck-dev-01`, each run must begin from a
GitHub-visible immutable commit (or an artifact traceable to it). The procedure
must record:

- Git commit and artifact digest;
- target architecture and whether execution is native, virtualized, or
  emulated;
- Debian, kernel, libc, systemd, nftables, and relevant tool versions;
- installed dependency provenance;
- sanitized configuration/fixture and test command;
- expected and observed results, including failures;
- resource measurements and retained evidence required by
  [`testing.md`](testing.md).

The target must not create authoritative source changes. Any fix is made on the
Mac, committed, pushed to GitHub, and then tested as a new exact revision.
Cross-compilation on either host is build evidence only; it does not prove
native runtime, packet-path, reboot, driver, storage, or power-loss behavior.

## Isolated network and destructive tests

Future networking tests require a router target with controlled WAN and
isolated LAN plus a separate client. The lab must prevent DHCP, RA, routing,
firewall, or fault-injection traffic from reaching the developer's normal
network. Use deterministic local upstream fixtures unless an Internet path is
part of the scenario.

Start in disposable VMs/namespaces, then use explicitly designated physical
targets for native drivers, media, reboot, and power-loss evidence. Snapshot or
back up before risky host operations, and document recovery before starting.
No such lab is created or modified during Stage 1.

## Secrets and fixtures

Never commit real credentials, SSH or signing private keys, provider tokens,
subscription URLs, PPP/VPN secrets, customer data, or production addresses.
Use reserved example domains/address ranges and clearly synthetic fixtures.
Local secret files remain ignored; `.env.example` or fixture templates may
contain names and safe placeholder values only.

Secrets must not appear in process arguments, logs, test output, screenshots,
support bundles, CI artifacts, or commit history. A secret accidentally added
to Git must be considered exposed even if a later commit deletes it.

## Documentation and decision discipline

Before behavior changes, identify canonical state, component ownership,
privilege, validation, transaction/rollback, reboot/power-loss, disk pressure,
IPv6, architecture, diagnostics, and UI implications. Update the living spec
or an ADR with the implementation and tests; do not allow them to diverge.

Read [`PROJECT.md`](../PROJECT.md) and [`CONTRIBUTING.md`](../CONTRIBUTING.md) for
the repository rules. Security findings follow [`SECURITY.md`](../SECURITY.md).
