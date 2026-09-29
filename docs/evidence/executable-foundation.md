# Executable foundation verification — 2026-09-07

This record belongs to the executable-foundation commit containing it. Its
parent is foundation commit `79c99e513eda8ef770b9a09f392b3848a217a9c7`.
Pre-commit artifacts are honestly marked `.dirty`; a clean post-commit build
embeds its exact SHA in `BUILD-INFO.json`. Artifact SHA256SUMS covers the whole
payload. No public tag or release is created.

## Scope and environment

- DEV verification host: macOS, Darwin arm64, Apple Command Line Tools present.
- Go 1.27.1, Node 24.20.0/npm 11.19.0; checksum-pinned local toolchains.
- React 19.2.8, TypeScript 7.0.2, Vite 8.2.2, Vitest 5.0.0; exact lockfile.
- Production runtime path: UI → authenticated API → Engine → Unix Agent →
  bounded real Linux collectors. No fixture fallback exists in these programs.
- Scope is read-only system observation, not managed networking or a qualified
  DuckOS 1.0 release. Related partial scope: M-002/003/007/028/029/030/032.

## Observed results

| Check | Result | Evidence boundary |
|---|---|---|
| `make check` | PASS | gofmt, Go vet, race-enabled Go tests, Go/schema compatibility, generated TS drift check, strict TypeScript, Prettier, UI tests, installer tests, shell syntax, diff whitespace |
| UI component tests | 12 PASS | Authentication, loading, current contract data, partial/error/stale responses, refresh/poll cancellation, filters, disconnect, no token persistence, formatting |
| Installer tests | 22 PASS | Real verifier with synthetic payloads; complete inventory/hash checks, traversal, links/FIFO, malformed metadata, wrong architecture, size and existing-host refusal |
| Go parser/API/Engine/Agent tests | PASS with race detector | Synthetic isolated data for parsers, auth/origin/listener/token permissions, IPC validation, bounded concurrency, coalescing, cache failure handling, cancellation |
| Observer fuzz smoke | PASS | 5-second budget, 978,781 generated executions; not an endurance/security qualification |
| IPC decoder fuzz smoke | PASS | 5-second budget, 539,503 generated executions; not an exhaustive proof |
| Linux read integration | SKIPPED on Mac | Two explicit skips: real Linux API→Engine→Agent collector and kernel peer-UID rejection/socket permissions |
| Production web build | PASS | About 221.17 kB JS / 68.09 kB gzip; 27.09 kB CSS / 6.83 kB gzip; static assets, no Node router runtime |
| Linux Go amd64, arm64, ARMv7 builds | PASS | Static cross-builds (`CGO_ENABLED=0`, `GOARM=7`); roughly 13–14 MiB payload per target; no native architecture claim |
| Real amd64 payload verifier | PASS | Actual compiled ELF binaries, web assets, unit templates and manifest; not installation execution |
| npm dependency audit | PASS, zero reported vulnerabilities | Registry audit at implementation time, not security qualification |
| Browser visual/interaction check | PASS | Desktop 1280px and mobile 375px, compiled UI via real API/Engine/Unix HTTP/Agent handler with explicitly synthetic TEST collector |
| Native Mac API executable smoke | PASS | Real compiled API serves UI/liveness; unauthenticated overview returns401, authenticated overview/readiness with absent Linux Agent return503 |
| First Debian installation/systemd sandbox | NOT RUN | `duck-dev-01` and all other Linux hosts were not accessed or modified |

Browser verification uses `tests/browser`, never a product build target. Its
hostname `SYNTHETIC-TEST.example` and expanded issue explicitly identify fixture
data. It exercises login and dashboard transport/rendering on Mac; it is not
Linux host-state evidence. Start it with
`bash -c 'source scripts/env.sh; go run ./tests/browser'` after `make build`.
Its test-only token is in that test source. Do not use it for an installation.

The GitHub workflow performs the same checks and cross-builds on an Ubuntu
Linux runner, including the real Linux read tests. A workflow definition is
not a passing CI run; check the run associated with the pushed commit. This
runner does not qualify the Debian installer or the root Agent sandbox.

## Review findings resolved

- Missing gateway is no longer inferred to mean an on-link route. Unsupported
  special/ECMP/nhid route shapes are omitted with an explicit partial-inventory
  issue; ordinary routes still appear. Rich route-type/nexthop modeling remains
  future work.
- An unavailable metric is not displayed as zero health; component issues and
  stale retained data are visible. Interface `up` is administrative state,
  not physical-carrier or reachability evidence.
- Readiness probes bypass the Engine's short cache and require valid Agent
  IPC. API liveness alone cannot complete installation.
- Installer failure stops newly started services, disables attempted enablement
  and retains evidence; dirty snapshots cannot be installed on a target.
- Product API is loopback-only and bearer-protected, with Host/Origin checks,
  bounded requests, no token logging and no browser token persistence.

## Known limits and next evidence

1. Native Debian 13 amd64 first install, service startup/restart, socket UID
   checks under the installed identities, nftables read privileges, reboot,
   disk-full and interrupted-install behavior require a designated Linux run.
2. Storage currently observes `/` only. CPU utilization is a short sampled
   interval; load average is separate. This is not historical monitoring.
3. Routes are bounded (4096 per family) and do not yet fully represent ECMP,
   nexthop objects or special forwarding actions. Partial data is flagged.
4. DNS lists host resolver configuration; systemd-resolved stub/upstream detail
   and active DNS probes are not implemented. nftables counts do not prove
   effective filtering. Only a fixed service allowlist is observed.
5. Full enrollment, TLS, per-user sessions/RBAC, Vault, persistent canonical
   configuration, Safe Apply, network mutation, updater/rollback, uninstall,
   release signatures, and 1.0 qualification remain unfinished. Loopback
   administrative forwarding and fresh TEST install are deliberate boundaries.
6. Agent capability `NET_ADMIN` is powerful even though its semantic interface
   is read-only. Source tests are not a replacement for native security review.
7. Full global journal retention remains Debian-owned; services rate-limit
   logs. No unbounded product metrics/history store is introduced here.

Installation commands and the GitHub-fed target procedure are in
[install.md](../install.md).

## Observed topology verification

The dashboard includes a topology section built from the existing timestamped
overview response. Host-to-interface connections mean inventory membership, not
physical cabling. Selecting an interface displays its observed addresses and
routes; Full mode adds interface index, MAC, MTU, route table, metric and protocol.
Missing and stale observations remain explicit. Default-route presence is not
Internet reachability, and interface names do not establish WAN or tunnel roles.

Component tests cover filtering, incomplete inventory, stale data and replacement
of a selected interface. Browser checks used the explicitly synthetic TEST
collector on Mac, including a 390px mobile viewport with all six navigation
destinations reachable. This is UI evidence, not a Linux topology qualification.
Bridge membership, VLAN parentage, DHCP identities, tunnel relationships and live
traffic require additional observed data before appearing as topology edges.
