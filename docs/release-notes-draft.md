# DuckOS V1 beta — release note draft (v0.1.0-beta.1)

> **Status**: draft, not yet released.
> This document describes what a V1 beta release WOULD say. The
> release itself is a separate decision and requires the launch
> gates listed in [`docs/launch/README.md`](launch/README.md) to be
> closed first.

## License

DuckOS Community source in this release is distributed under
**GPL-3.0-only** — see [`LICENSE`](../LICENSE) and
[`COPYRIGHT.md`](../COPYRIGHT.md). Trademarks and brand use are
governed separately by [`TRADEMARKS.md`](../TRADEMARKS.md). The
commercial / dual-licensing narrative is in
[`docs/licensing.md`](licensing.md). A finalized Contributor
License Agreement ([`CLA.md`](../CLA.md)) is not yet active; external
code contributions are not accepted against this draft release.

## What this release is

DuckOS V1 beta is a public beta of the DuckOS V1 frontend plus a
qualified subset of the V1 backend. It is not a production router. It
is a beta that observers and contributors can install, evaluate, and
report against.

The release target is **`v0.1.0-beta.1`**.

## What ships in v0.1.0-beta.1

- React / TypeScript UI with **Easy Mode** and **Full Mode**.
- Full Easy Mode page catalog (Home, Internet, Wi-Fi, Devices, VPN,
  Care / Recovery).
- Full Full Mode page catalog across System, Network, Security,
  DNS / DHCP, VPN / Tunnels, Updates, Audit / Help.
- One global search (`⌘K` / `Ctrl+K`).
- Inspector pane (`?`) for contextual reference on every page.
- First Owner enrollment, password login, built-in RBAC, expiring /
  revocable sessions.
- Doctor subsystem with deterministic findings.
- Localized in English, Russian, German, Spanish.

## What is qualified in v0.1.0-beta.1

- Build targets: `amd64` (qualified on Debian 13 host).
- Reference-supported targets: `arm64`, `armv7` (cross-build only,
  no native qualification yet).
- The fresh Debian 13 amd64 TEST installer.

## What is NOT in v0.1.0-beta.1

The following catalog subsystems are present in the UI as observed
state, but **not** as live configuration through the public web UI:

- Live WAN / LAN / VLAN / DHCP / DNS / firewall / route / policy
  configuration through Safe Apply.
- Multi-WAN live activation.
- WireGuard / Xray / NetBird live activation.
- Live Client provisioning and subscriptions.
- Doctor remediation automation.
- Update supervisor with the `Active + Previous` invariant on a
  running system.

See [`docs/beta-limitations.md`](beta-limitations.md) for the full
list and for what is intentionally not a limitation.

## Install path

The V1 beta install path is described in [`docs/install.md`](install.md).
The short version:

1. Verify the artifact digest against the published release manifest.
2. Install on an isolated test network — never on a production
   gateway.
3. Reach the dashboard through an SSH local-forward from an
   administrator host; the V1 beta binds only `127.0.0.1:8080`.
   See [`docs/user/getting-started.md`](user/getting-started.md) for
   the exact `ssh -L` command and browser URL. Direct LAN HTTPS
   onboarding is **not** part of this beta.
4. Complete first-time Owner enrollment.
5. The dashboard is at `http://127.0.0.1:8080` over the SSH tunnel.

## Known limitations

- The Easy Mode headline will say **Status check incomplete** on a
  healthy-looking system until the backend grows verified-health
  evidence. This is intentional.
- The Doctor does not have a generative remediation assistant.
- The DuckOS team has not completed native ARMv7 qualification.

## Upgrade note

This is the first public beta. There is no prior public release to
upgrade from. The current `Active + Previous` invariant is exposed in
tests but not yet exposed in the public UI.

## Recovery note

The pre-networkd boot recovery unit is qualified in tests and ships
with the install image. Real-reboot qualification is pending; the
administrator is still responsible for keeping an out-of-band
recovery console during the first install on new hardware.

See [`docs/user/recovery.md`](user/recovery.md) for the recovery
primitives.

## Reporting bugs

- Open an issue following the bug report template.
- Sanitize logs before attaching them. Do not publish private keys,
  subscription URLs, or production network details.
- For security vulnerabilities, follow [`SECURITY.md`](../SECURITY.md).

## What this release is NOT

- It is not a public production release.
- It is not feature-complete.
- It is not a substitute for the qualification work described in
  [`docs/launch/README.md`](launch/README.md).
- It is not an announcement of a future central management /
  controller product.