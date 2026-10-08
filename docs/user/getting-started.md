# Getting started

This page describes the path from a fresh install to a configured
DuckOS V1 beta system. It assumes you have already installed DuckOS
on supported hardware, a VM, or an edge node. The installation procedure
lives in [`docs/install.md`](../install.md); the hardware support matrix
lives in [`docs/hardware-support.md`](../hardware-support.md).

DuckOS V1 is an **early beta**. Many of the controls described in this
guide exist in the UI but are not yet wired to live network activation.
The current state of every area is documented in
[`docs/beta-limitations.md`](../beta-limitations.md). Do not deploy DuckOS
on a production gateway.

## What you should have ready

- The DuckOS installation image that matches your target architecture
  (`amd64`, `arm64`, or `armv7`). The V1 beta is qualified for **amd64**
  on Debian 13 host targets. ARM64 and ARMv7 are official artifacts with build verification, but
  did not receive the complete AMD64 live matrix. Fresh installation is required;
  there is no qualified beta.1 → beta.2 upgrade.
- An isolated test network — never the same physical network as a
  production router.
- A second way to reach the device out-of-band (a serial console, a
  second management interface, or a VM console). Safe Apply uses a
  management anti-lockout guard, but the second path is your safety
  net during the first install.
- A modern browser. DuckOS targets Chromium and Firefox at current
  versions; older browsers are not supported.

## Opening the UI

Beta.2 uses a loopback management listener, with no automatic mDNS discovery
or browser-trusted LAN TLS onboarding. From your administrator workstation:

```bash
ssh -N -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
```

Open `http://127.0.0.1:8080`. The first screen is Owner enrollment.
Read the host's `/etc/duckos/api-token` privately when prompted; never publish it.

## Owner enrollment

DuckOS has exactly one Owner per installation. The Owner is the only
account that can do everything, including first-time configuration and
the bootstrap authentication. The first Owner is created during
enrollment and is the only account that exists.

1. The UI prompts for a username, a display name, and a password.
2. Choose a strong password. There is no minimum enforced by the
   server; the UI recommends 14 characters or more.
3. Submit. The browser is redirected to the dashboard.

After enrollment, the browser holds a session token. The token lives in
sessionStorage only and expires automatically. Closing the browser
ends the session.

## The first screen: Easy Mode

The default landing page is **Easy Mode** (`/easy/home`). Easy Mode
shows a single-page summary of the system state:

- A headline that surfaces the current health conclusion.
- A System tile that surfaces "Some things need attention" or the
  honest incomplete verdict.
- An Internet tile that links into Easy Mode's Internet wizard.
- Wi-Fi, Devices, VPN, and About this device tiles for the rest of the
  day-to-day state.

If you are an administrator who needs the dense Full Mode view, click
**Full** in the top bar. You can always switch back with **Easy**.

## Initial configuration

Beta.2 includes qualified WireGuard and Xray runtime paths. This does not make
every catalog page a qualified configuration workflow. Check
[beta limitations](../beta-limitations.md) and [release notes](../release-notes.md).
Network changes require the documented validation, Safe Apply and confirmation
path; preserve the active management interface and console access. Do not apply
rendered Linux configuration by hand to bypass an unavailable UI operation.

## Search

DuckOS has **one** global search. Press `⌘K` (macOS) or `Ctrl+K`
(otherwise) to open it. Search covers pages, help, search, and the
real objects observed in the current Overview (interfaces, IPs, MACs,
VLANs, hostnames, services). The Inspector pane (the question-mark
icon) opens a contextual reference for the page you are on.

## Where to go next

- [Easy Mode reference](easy-mode.md) — the dashboard in detail.
- [Full Mode reference](full-mode.md) — the by-domain pages.
- [Safe Apply](safe-apply.md) — the network change lifecycle.
- [Doctor and diagnostics](doctor-and-diagnostics.md) — interpreting
  the warnings Easy Mode surfaces.
- [Troubleshooting](troubleshooting.md) — what to do when something
  is wrong.