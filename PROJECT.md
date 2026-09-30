# DuckOS

## Project Goal

DuckOS is a local-first network operating platform for supported Debian systems.
It combines routing, security, VPN connectivity, diagnostics and recoverable
configuration in a coherent interface for homes, offices, hospitality networks,
branch locations and edge nodes. Normal administration should not require a
Linux shell. DuckOS is software above Debian, not a custom kernel or distribution.

## Product Principles

- **One Repository**: source, schemas, packaging, tests and documentation evolve together.
- **One DuckOS**: one configuration lifecycle, API, interface and update mechanism.
- **Any Hardware**: capabilities and measured resources determine feature availability,
  within the supported platform matrix; hardware does not create product forks.

## Supported Targets

- `amd64`
- `arm64`
- `armv7`, with ODROID XU4 as the minimum resource reference

These are build targets, not a claim of completed runtime qualification.
[Hardware support](docs/hardware-support.md) defines the evidence required.

## Architecture

```text
Web UI → API → Engine → Privileged Agent → Linux
```

The Engine owns canonical desired configuration and product semantics. The
unprivileged API, Web UI and CLI share that semantic path. The root Agent
authenticates its caller and independently validates narrow, versioned operations.
Linux files and kernel objects are rendered or observed state, not the canonical
business model. There is no arbitrary root command interface.

## Core Technology

- Go for the control plane and privileged service.
- React and TypeScript for the management interface.
- SQLite for the implemented client metadata catalog; bounded revision and
  identity stores retain the documented JSON journal. Broader migration requires
  explicit durability and compatibility design.
- nftables for firewall and NAT; iproute2 for routing and link operations.
- tc for traffic control; systemd/networkd and dnsmasq for supervised host services.

## Development Rules

- Preserve API-first architecture and the Engine/Agent security boundary.
- Use transactional network changes and Safe Apply for dangerous mutations.
- Preserve management access independently of WAN/LAN roles, including single-NIC nodes.
- Keep installation safe for remotely administered systems; installation alone
  must not replace networking/firewall state or alter SSH access.
- Use actual observations in production; synthetic fixtures belong only in tests.
- Pass applicable tests and review the diff before merge or push; never force-push main.
- Keep secrets out of the repository, ordinary reads, logs and support bundles.
- Maintain IPv4/IPv6 and cross-architecture compatibility and bounded resource use.
- Test crash, reboot, power loss and disk pressure; retain last-known-good recovery.
- Stage updates beside Active and preserve one Previous rollback release.
- Record consequential decisions in ADRs and distinguish planned behavior from evidence.
- Make common workflows beginner-obvious while retaining exact expert inspection,
  as defined by the [UX acceptance contract](docs/ux-principles.md).

## Product Scope

The [requirements catalog](docs/product-spec-1.0.md) covers WAN/LAN, VLAN,
DHCP/DNS, routing and multi-WAN, firewall/NAT, management policy, VPN transports,
client subscriptions and secret QR exports, devices, monitoring, diagnostics,
identity, backup, signed updates, hardware capabilities, sites and portal services.
The [scope matrix](docs/product-spec-1.0.md) records acceptance and qualification.
The [frozen V1 scope](docs/product-spec-1.0.md) supplies the current A/B/C delivery
classification; deferral is not permission to drop a requirement. All major
subsystems use intent-driven, visible, explainable and reversible management,
with shared GUI/CLI semantics and contextual actions.

## Completion Criteria

DuckOS 1.0 requires integrated, secure, recoverable and documented functionality
across its approved scope, with no unresolved critical blocker. Qualification
includes actual isolated client traffic, native supported-architecture evidence,
IPv4/IPv6, restart/reboot and fault recovery, management lockout prevention,
unauthorized-access and secret-leakage tests, bounded storage and resource use,
installer/update/backup recovery, and a complete accessible management interface.
Cross-compilation, a dashboard or successful unit tests alone do not establish readiness.
The first public product release is 1.0.0; earlier artifacts are internal builds.

## Developer and Operator Guides

- [Build and test](docs/development.md)
- [Install and recover](docs/install.md)
- [Contribute](CONTRIBUTING.md)
- [Architecture](docs/architecture.md) and [decisions](docs/adr/README.md)
- [Security model](docs/security-model.md) and [reporting](SECURITY.md)
- [Release/update model](docs/update-system.md)
- [Testing and evidence](docs/testing.md)

The repository is not currently a supported production router release. Public
licensing remains an explicit release decision; no license grant is implied.
