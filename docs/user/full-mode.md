# Full Mode

Full Mode is the dense, by-domain DuckOS view. It contains the same
data that Easy Mode shows, plus the configuration views that Easy Mode
deliberately does not expose.

The full-mode page catalog is described in
[`docs/ux-v1/PAGES.md`](../ux-v1/PAGES.md). This page gives the
administrator-level mental model.

## Switching in and out

Click **Full** in the top bar to enter Full Mode. Click **Easy** to
return.

Full Mode state survives navigation; the top bar's mode toggle is the
only switch.

## The sections

Full Mode is grouped into sections. The section names match the page
header and the Inspector navigation:

- **System** — System Overview, Safe Apply / Candidate, Topology
  preview.
- **Network** — Interfaces, Bridges, Bonding, Loop protection,
  Discovery, MACsec, Mirroring, Acceleration, L2 resilience, WAN
  connections, Mobile WAN, Multi-WAN, Path quality, DDNS, Networks,
  Address plan, Clients / devices, Routing, Routes, Routing tables.
- **Security** — Firewall, NAT, Port forwarding, MACsec, port
  security, isolation, intrusion, RA guard, DHCP guard.
- **DNS and DHCP** — DNS, DHCP, dnsmasq integration.
- **VPN / Tunnels** — WireGuard, Xray, NetBird, subscription
  providers.
- **Updates** — Update system, Backup / restore, Doctor.
- **Audit / Help** — search, inspector, audit log, identity.

Pages that are not yet wired to live data show a **design-only** label
in the page header. The section above is therefore not a roadmap; it
is the current page catalog, with some pages rendered against the live
API and others rendered against the canonical model in code state.

## Configuration workflow

Every change goes through the same lifecycle:

1. **Open the page** in Full Mode.
2. **Edit the candidate** — the UI shows the current active
   configuration and the proposed candidate side by side.
3. **Apply** — Safe Apply captures the snapshot, installs the
   forwarding guard, applies the candidate, and waits for confirmation.
4. **Confirm** — the candidate becomes the active configuration.

See [`safe-apply.md`](safe-apply.md) for the lifecycle and
[`architecture.md`](../architecture.md) for the design.

## Mode-specific layout

Full Mode's navigation uses a side rail, with each section expanded
into its by-domain pages. Each page has a single H1, a status pill row,
the configuration views, and the action bar.

## When to use Full Mode

- You want to configure an interface, a route, or a firewall rule.
- You want to inspect a specific subsystem.
- You want to read the audit log or the topology preview.

For day-to-day "is it working?" questions, prefer Easy Mode.