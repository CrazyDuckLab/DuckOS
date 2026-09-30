# DuckOS

A local-first Linux network operating platform for routers, gateways,
VPS / edge nodes and self-hosted network appliances — one configuration
lifecycle across different hardware, above supported Debian.

[![License: GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Status: V1 Beta](https://img.shields.io/badge/status-V1%20Beta-orange)](docs/beta-limitations.md)
[![Platform: Linux amd64](https://img.shields.io/badge/platform-Linux%20amd64-informational)](docs/install.md)
[![Languages: EN · RU · DE · ES](https://img.shields.io/badge/i18n-EN%20%C2%B7%20RU%20%C2%B7%20DE%20%C2%B7%20ES-lightgrey)](docs/architecture.md)
[![Local-first](https://img.shields.io/badge/operation-local--first-brightgreen)](AGENTS.md)
[![No mandatory SaaS](https://img.shields.io/badge/dependencies-no%20mandatory%20SaaS-brightgreen)](docs/licensing.md)

---

## What is DuckOS?

DuckOS brings network management and recovery into a modern web
interface above supported Debian, for home networks, homelabs and
small-business administrators who want one configuration lifecycle
across different hardware.

The first public beta is **`v0.1.0-beta.1`**, qualified for **amd64 on
Debian 13**. It is **not** a production router. It is a beta of an
observability and configuration platform that has not yet activated
every catalog feature.

The first beta intentionally retains the qualified internal build
identifier `1.0.0-dev.<commit>` — the supported build and installer
contracts lock to that pattern, and patching the version logic was
explicitly out of scope for this release. See
[`docs/release-notes.md`](docs/release-notes.md) for the full
explanation.

## Highlights

- **Easy Mode** — one-page dashboard with the headline checks a home
  reader actually wants to see.
- **Full Mode** — by-domain catalog across System, Network, Security,
  DNS / DHCP, VPN / Tunnels, Updates, Audit / Help.
- **Owner account** — exactly one Owner per installation. First Owner
  is created during enrollment; subsequent roles inherit RBAC.
- **Safe Apply** — durable journal, independent confirmation
  watchdog, automatic rollback. The only path from candidate to
  active state.
- **Doctor** — deterministic diagnostics with named findings, not a
  black box.
- **One global search** (`⌘K` / `Ctrl+K`) and an **Inspector pane**
  (`?`) for contextual reference on every page.
- **Localized** in English, Russian, German, Spanish.
- **Local-first** — the management UI and Agent run on the device
  itself and bind only `127.0.0.1:8080`. No required SaaS, no automatic
  LAN exposure, no HTTPS interception by default.
- **Signed offline update foundations** — `.duck` packages,
  ed25519 signature verification, durable Active + Previous store,
  systemd restart with a signed `/api/v1/ready` health probe.

## Screenshots

Real captures against the installed V1 beta backend on a clean
Debian 13 amd64 VM. No concept art.

The flagship:

**![Full Mode System Overview](docs/assets/screenshots/02-system-overview.png)**

Representative captures:

| | |
|---|---|
| ![Easy Mode home](docs/assets/screenshots/01-easy-home.png) | ![Interfaces](docs/assets/screenshots/03-interfaces.png) |
| ![Routing](docs/assets/screenshots/04-routing.png) | ![Firewall](docs/assets/screenshots/05-firewall.png) |
| ![WireGuard](docs/assets/screenshots/06-wireguard.png) | ![Safe Apply](docs/assets/screenshots/09-safe-apply.png) |

Plus first-run enrollment (`10-first-run-enroll.png`), global search
(`07-search.png`), and the page-help Inspector (`08-inspector.png`)
in [`docs/assets/screenshots/`](docs/assets/screenshots/).

Each capture is a real browser (headless Chromium for Testing) over an
SSH tunnel to `127.0.0.1:8080` on the installed public RC backend.
Lab hostname `duck-beta-01` and lab addresses are demo data; no tokens,
passwords, browser devtools or debug overlays are visible.

The V1 beta SPA uses hash-based routing. The HTML document title is
set to `DuckOS · System overview` on initial load and does not change
between pages. The page body content is distinct per route. See
[`docs/assets/screenshots/README.md`](docs/assets/screenshots/README.md)
for the launch set, the capture method, the filename convention, and
the sanitization rules.

## Quick Start

1. **Download** the release archive:

   ```text
   duckos-v0.1.0-beta.1-linux-amd64.tar.gz
   SHA256SUMS
   ```

2. **Verify** the digest against `SHA256SUMS`:

   ```bash
   sha256sum -c SHA256SUMS
   ```

3. **Extract**:

   ```bash
   tar -xzf duckos-v0.1.0-beta.1-linux-amd64.tar.gz
   cd duckos-v0.1.0-beta.1
   ```

4. **Install** on a fresh, isolated **Debian 13 amd64** host:

   ```bash
   sudo bash install.sh --artifact "$(pwd -P)"
   ```

   The installer refuses any other distribution and any pre-existing
   DuckOS installation.

5. **Open** the dashboard through an SSH local-forward from an
   administrator host:

   ```bash
   ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
   ```

   Then in a modern Chromium or Firefox browser:

   ```
   http://127.0.0.1:8080
   ```

6. **Enroll** the first Owner and follow the
   [Getting started](docs/user/getting-started.md) guide.

See [`docs/install.md`](docs/install.md) for the full procedure,
[`docs/user/README.md`](docs/user/README.md) for the user guide, and
[`docs/beta-limitations.md`](docs/beta-limitations.md) for what the
beta does and does not yet do.

## Easy Mode / Full Mode

The frontend has two readers for the same data:

- **Easy Mode** ([guide](docs/user/easy-mode.md)) is the single-page
  dashboard — Home, Internet, Wi-Fi, Devices, VPN, Care / Recovery.
- **Full Mode** ([guide](docs/user/full-mode.md)) is the dense,
  by-domain catalog — System, Network, Security, DNS / DHCP, VPN /
  Tunnels, Updates, Audit / Help.

You switch between them at any time without losing session state.

## Safety: Safe Apply

Every change that touches the network or system goes through
[Safe Apply](docs/user/safe-apply.md). A Safe Apply transaction has:

- a **durable journal** of the candidate change,
- an **independent confirmation watchdog** that gates the change on
  your explicit approval,
- an **automatic rollback** path if health fails before, during, or
  after the change.

The dashboard makes the candidate, the staging area, and the
rollback path explicit. The same primitives back the offline
update flow.

## Documentation

- [User guide](docs/user/README.md) — getting started, Easy Mode,
  Full Mode, Safe Apply, Doctor, Recovery, Troubleshooting.
- [Documentation index](docs/README.md) — every guide, grouped.
- [Architecture](docs/architecture.md) — component ownership and
  dependency direction.
- [Engineering policy for coding agents](AGENTS.md) — required
  reading for code contributions.
- [Contributing](CONTRIBUTING.md) — practical rules, tests, templates.
- [Beta limitations](docs/beta-limitations.md) — what the beta is and
  is not.
- [Release notes](docs/release-notes.md) — what's in v0.1.0-beta.1.
- [Roadmap](docs/roadmap.md) — V1 beta, later, future.

### AI & automation assistants

External assistants and automation agents helping users evaluate,
install, configure, understand, or troubleshoot DuckOS should start
from [`llms.txt`](llms.txt) and the [assistant guide](docs/ai/README.md).
[`docs/ai/task-map.md`](docs/ai/task-map.md) maps user intent to the
exact DuckOS area, the canonical guide, and the safety note;
[`docs/ai/setup-recipes.md`](docs/ai/setup-recipes.md) gives
task-oriented recipes;
[`docs/ai/troubleshooting.md`](docs/ai/troubleshooting.md) is the
decision-oriented triage.

## Security

Read the [security model](docs/security-model.md) and the
[reporting policy](SECURITY.md). Never publish private keys,
passwords, client QR payloads, subscription URLs / tokens or
sensitive network details in issues. The management UI / API
bind only `127.0.0.1:8080` on the device and are **not** exposed
to the LAN automatically.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) describes engineering rules, tests
and review. Issue templates cover bugs, features, installation,
hardware and documentation. Test results identify the exact build
and distinguish native, virtualized and cross-build evidence.

The Contributor License Agreement ([`CLA.md`](CLA.md)) is **DRAFT**.
External code contributions are not accepted against this beta.

## License

DuckOS Community source is distributed under the **GNU General
Public License version 3 — only** (`GPL-3.0-only`). See
[`LICENSE`](LICENSE) for the canonical text.

- [`LICENSE`](LICENSE) — canonical GPL-3.0-only text.
- [`COPYRIGHT.md`](COPYRIGHT.md) — authorship and trademark
  separation.
- [`CLA.md`](CLA.md) — DRAFT contributor license agreement.
- [`TRADEMARKS.md`](TRADEMARKS.md) — DuckOS name, wordmark and
  rubber-duck logo use.
- [`docs/licensing.md`](docs/licensing.md) — commercial /
  dual-licensing narrative.

The license applies to source code in this repository from the
moment each commit is published; already-distributed copies remain
under the license in force at the time they were received.

## Project status

DuckOS V1 beta (`v0.1.0-beta.1`) is the first public beta. It is
qualified for **amd64** on **Debian 13**. ARM64 and ARMv7 cross-build
but have not been natively qualified on hardware for this beta.