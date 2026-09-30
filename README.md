# DuckOS

**ONE REPOSITORY. ONE DUCKOS. ANY HARDWARE.**

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Status: V1 Beta](https://img.shields.io/badge/status-V1%20Beta-orange)](docs/beta-limitations.md)
[![Platform: Linux amd64](https://img.shields.io/badge/platform-Linux%20amd64-informational)](docs/install.md)
[![Languages: EN · RU · DE · ES](https://img.shields.io/badge/i18n-EN%20%C2%B7%20RU%20%C2%B7%20DE%20%C2%B7%20ES-lightgrey)](docs/launch/README.md)
[![Local-first](https://img.shields.io/badge/operation-local--first-brightgreen)](AGENTS.md)
[![No mandatory SaaS](https://img.shields.io/badge/dependencies-no%20mandatory%20SaaS-brightgreen)](docs/licensing.md)

## What is DuckOS?

DuckOS is a Linux network operating platform in development for routers,
gateways, VPS/edge nodes and self-hosted network appliances. It brings network
management and recovery into a modern web interface above supported Debian.
It is designed for home networks, homelabs and small-business administrators
who want one configuration lifecycle across different hardware.

**Not production-ready.** The installed runtime currently observes Linux state;
transactional network activation and the complete 1.0 feature set are still
under development. No public release exists. The intended open-source license
for DuckOS Community is **GPL-3.0-only** — see [License](#license).

## Why DuckOS?

- One API → Engine → Privileged Agent boundary for Web and CLI management.
- Transactional configuration with management anti-lockout and automatic rollback
  as core requirements, including single-NIC gateways.
- Local-first operation without a required cloud service or HTTPS interception.
- Shared architecture across amd64, arm64 and resource-constrained armv7 systems.
- Explicit distinction between measured health, unknown state and planned features.

## Features

| Area | Current state |
|---|---|
| System Overview | Real Linux hostname, uptime, CPU/load, RAM/storage, interfaces, addresses/routes, DNS settings and basic service/nftables observations through React → API → Engine → Agent. |
| Authentication | First Owner enrollment, password login, built-in RBAC, expiring/revocable sessions and optional TLS certificate loading. Hardening/recovery qualification remains incomplete. |
| WAN/LAN, VLAN, DHCP/DNS, static and policy routing | Bounded canonical validation and networkd/dnsmasq/nftables previews; not an enabled network configuration service. |
| Firewall, NAT and port forwarding | Scoped nftables host primitives, captured-state recovery and local tests; production activation remains gated. |
| Safe Apply | Durable journal, independent confirmation watchdog and recovery core; full live-network anti-lockout integration remains incomplete. |
| Multi-WAN, WireGuard, Xray and NetBird | Required transport, health and policy integration remains incomplete, not an available gateway claim. |
| Client provisioning and subscriptions | HAPP-compatible output, subscription QR, WireGuard QR/.conf and credential rotation are requirements, not currently usable features. |
| Port Exposure | Service-aware Allow on WAN and independent management policy are specified; final API/UI integration remains pending. |
| Operations and recovery | Monitoring history, Duck Doctor, Duck Speed Lab, backup/restore, signed .duck updates and Active/Previous rollback remain incomplete. |

See the [product catalog](docs/requirements/product.md),
[scope matrix](docs/product-spec-1.0.md) and [test evidence](docs/testing.md).

## Screenshots

Launch screenshots will show actual implemented screens with a recorded build
and sanitized test environment. No mock production screenshots are presented.
The [capture checklist](docs/launch/screenshots.md) covers Overview, Network,
Firewall, routing, VPN/client QR, subscriptions, Port Exposure, Safe Apply and diagnostics.

## Supported architectures

Build targets: **amd64 · arm64 · armv7**. ODROID XU4 is the minimum resource
reference. A successful cross-build is not native hardware qualification.
See [hardware support](docs/hardware-support.md) and the
[compatibility report model](docs/hardware-compatibility.md).

## Hardware / deployment scenarios

- [Home Router, Homelab Gateway and Small Business Gateway](docs/use-cases/home-gateway.md)
- [Multi-WAN Gateway and selective routing](docs/use-cases/routing.md)
- [VPS / Edge VPN Node and Single-NIC Gateway](docs/use-cases/edge-node.md)
- [ODROID / ARM Appliance](docs/use-cases/arm.md)
- [VPN client provisioning and subscriptions](docs/use-cases/client-provisioning.md)

These are deployment design guides, not assertions that unfinished features
are ready for installation. [Migration planning](docs/use-cases/migration.md)
explains current fit and limitations without promising drop-in replacement.

## Quick Start

For a local development checkout:

```sh
bash scripts/bootstrap.sh  # pinned tools inside .tools; no system installation
make deps
make check
make build-all             # production UI assets and Linux cross-builds
```

The [installation guide](docs/install.md) covers artifact verification and the
fresh Debian 13 amd64 TEST installer. Do not install on an existing production
gateway. Keep a recovery console and use an isolated test environment. An
installer success or API health response does not qualify packet forwarding.

## Documentation

[User guide](docs/user/README.md) · [Documentation index](docs/README.md) ·
[Architecture](docs/architecture.md) · [API](shared/api/README.md) ·
[Engineering policy for coding agents](AGENTS.md) ·
[Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) ·
[Licensing](docs/licensing.md) ·
[Roadmap](docs/roadmap.md) · [Beta limitations](docs/beta-limitations.md)

## Security

Read the [security model](docs/security-model.md) and
[reporting policy](SECURITY.md). Never publish private keys, passwords, client
QR payloads, subscription URLs/tokens or sensitive network details in issues.
Management listeners are not automatically exposed publicly.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) describes engineering rules, tests and review.
Issue templates cover bugs, features, installation, hardware and documentation.
Test results identify the exact build and distinguish native, virtualized
and cross-build evidence.

## License

DuckOS Community source is distributed under the **GNU General Public
License version 3 — only** (`GPL-3.0-only`). See [`LICENSE`](LICENSE)
for the canonical text.

- [`LICENSE`](LICENSE) — canonical GPL-3.0-only text.
- [`COPYRIGHT.md`](COPYRIGHT.md) — authorship and trademark separation.
- [`CLA.md`](CLA.md) — DRAFT contributor license agreement (code
  contributions are not accepted under this draft).
- [`TRADEMARKS.md`](TRADEMARKS.md) — DuckOS name, wordmark, and
  rubber-duck logo use.
- [`docs/licensing.md`](docs/licensing.md) — commercial /
  dual-licensing narrative.

A public release of DuckOS has not yet been published. The license
applies to source code in this repository from the moment each commit
is published; already-distributed copies remain under the license in
force at the time they were received.

## Development checkpoint

For continuation, read [CONTEXT.md](CONTEXT.md), then the current state and frozen
V1 roadmap. The checkpoint does not imply a release or native qualification.

## Project status

DuckOS is an internal development project toward **1.0.0**, not a released router
OS. There are no intermediate public product releases. Public repository
visibility, indexing, announcements and releases require separate launch approval
after technical readiness gates are satisfied.
