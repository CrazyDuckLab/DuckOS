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
  on Debian 13 host targets. ARM targets are reference-supported only.
- An isolated test network — never the same physical network as a
  production router.
- A second way to reach the device out-of-band (a serial console, a
  second management interface, or a VM console). Safe Apply uses a
  management anti-lockout guard, but the second path is your safety
  net during the first install.
- A modern browser. DuckOS targets Chromium and Firefox at current
  versions; older browsers are not supported.

## Finding DuckOS on the network

After the first boot, DuckOS advertises itself on the management LAN
through:

- **mDNS / DNS-SD** under `duckos-<hostname>.local`.
- A link-local IPv6 address on the management interface.

The host terminal prints the management interface name and its current
address. Use the printed URL or `https://duckos-<hostname>.local` from a
browser on the same management network.

## Opening the UI

1. From a browser on the management LAN, navigate to the URL the
   install terminal printed.
2. The browser will warn that the TLS certificate is not trusted. This
   is expected for a first install; the certificate is self-signed.
   Accept the warning **only on a private management LAN** and continue.
3. The first screen is **Owner enrollment**.

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

DuckOS V1 beta observes Linux state and renders it. It does not yet
write network configuration through the public UI on a running
system — the Safe Apply integration is gated behind the qualification
gates. The current beta is therefore useful as a viewer for an
already-installed system, not as a configurator.

When the gates open, the path is:

1. Open the **Interfaces** page in Full Mode.
2. Pick the interface you want to configure.
3. Edit its fields in the candidate UI.
4. Click **Apply**. The Safe Apply flow described in
   [`safe-apply.md`](safe-apply.md) takes over.
5. After Apply succeeds, click **Confirm**.

Until the gates open, configuration has to happen by editing the
relevant Linux adapters directly, with the Safe Apply flow reserved
for the narrow paths that have passed qualification. See
[`docs/beta-limitations.md`](../beta-limitations.md) for the current
list of which subsystems are wired up.

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