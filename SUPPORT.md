# DuckOS support

This page explains how to get help for the public beta
`v0.1.0-beta.1`. There is **no** commercial support contract for this
beta. Help is community / best-effort, through the channels below.

## Routes by topic

| Topic | Where to ask |
|---|---|
| **Bug** (something does not work as documented) | GitHub Issue with the `bug` template. Include the fields the template lists. |
| **Documentation error** (something in the docs is wrong, missing, or out-of-date) | GitHub Issue with the `documentation` template. |
| **Hardware / installation problem** (a board, NIC, Wi-Fi adapter, modem, or install path that misbehaves) | GitHub Issue with the `hardware / installation` template. |
| **Security vulnerability** (suspected weakness, exploitable behaviour) | **Private** — see [`SECURITY.md`](SECURITY.md). Do not open a public issue. |
| **General question** ("how do I…", "where is…", "is X supported?") | GitHub Discussions, if enabled on the public repository. If Discussions are not yet available, open a GitHub Issue with the question label and the team will route it. |
| **Compatibility with a specific board, NIC, or chipset** | GitHub Issue with the `hardware / installation` template. Do not assume undocumented hardware is supported. |

## What to include

A high-quality report starts with:

- The exact DuckOS release you are running (`v0.1.0-beta.1` or a
  branch / commit hash).
- The runtime build identifier (Easy Mode footer or
  `/api/v1/overview` payload, e.g. `1.0.0-dev.f698de16b4a9`).
- The architecture (`amd64`, `arm64`, or `armv7`).
- The Debian release on the device (this beta: Debian 13).
- The install method (`install.sh --artifact`, source build, etc.).
- Expected vs observed behaviour.
- Reproduction steps.
- Doctor findings (if any) — copy the component / code / message
  fields from the Doctor card.
- Screenshots, when useful. Sanitise them first.

## What to NOT include

Do **not** post, paste, or attach any of the following in any public
report:

- The Owner password or any user password.
- The bootstrap API token (`/etc/duckos/api-token`).
- API session tokens, refresh tokens, OAuth client secrets.
- WireGuard / frr / ed25519 / RSA private keys.
- Xray / subscription URLs, subscription credentials, or QR
  payloads that embed subscription URLs.
- Production network addresses, customer data, packet captures.
- Real hostnames or IP addresses of production gateways.

If your reproduction requires production data, reproduce it on an
isolated test network. If you cannot, see
[`SECURITY.md`](SECURITY.md) for the private reporting route.

## What we will NOT do

- Provide paid support. There is no commercial SLA for the public
  beta.
- Promise a fix or release date for an open issue. The maintainer
  team is small. Be patient.
- Provide per-customer deployment assistance, configuration
  services, or hardware-specific tuning beyond what the public
  documentation already says.
- Offer a Discord / Telegram / community chat. If community chat
  is added later, it will be linked from this page.
- Acknowledge a report that includes credentials, private keys, or
  exploit payloads. We will close the public item and ask you to
  use the private reporting route.

## What we WILL do

- Read and triage public reports in the order they arrive.
- Acknowledge reports in the relevant issue tracker.
- Fix confirmed bugs in the next planned release.
- Update documentation when an error is confirmed.
- Refuse to engage with hostile or abusive communication.
- Keep the public tracker focused on actionable, evidence-backed
  reports.

## Before opening a report

Please read the existing documentation and search the issue list:

- [`README.md`](README.md)
- [`docs/README.md`](docs/README.md)
- [`docs/beta-limitations.md`](docs/beta-limitations.md)
- [`docs/compatibility.md`](docs/compatibility.md)
- [`docs/user/troubleshooting.md`](docs/user/troubleshooting.md)
- [`docs/ai/troubleshooting.md`](docs/ai/troubleshooting.md) (for
  AI assistants helping users)
- Existing open and closed issues on the public repository

Many "is this a bug?" questions are answered in
[`docs/beta-limitations.md`](docs/beta-limitations.md) — V1 beta does
not activate every catalog feature, and the dashboard is honest
about which subsystems are wired to live data.

## Related documentation

- [`README.md`](README.md)
- [`docs/README.md`](docs/README.md)
- [`docs/user/README.md`](docs/user/README.md)
- [`docs/ai/README.md`](docs/ai/README.md)
- [`SECURITY.md`](SECURITY.md)
- [`CONTRIBUTING.md`](CONTRIBUTING.md)