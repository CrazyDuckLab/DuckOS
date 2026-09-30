# DuckOS V1 — User Guide

This directory is the human-facing user guide for the DuckOS V1 beta.
It focuses on what each area does, the common workflow, the key
concepts, the warnings, and the related pages.

The guide is intentionally concise. Use [`docs/architecture.md`](../architecture.md)
and [`docs/architecture/adr/`](../adr/) for design rationale, and
[`docs/security-model.md`](../security-model.md) for the safety invariants.

## Reading order for a new administrator

1. [Getting started](getting-started.md) — first boot, finding DuckOS on
   the network, opening the UI, Owner enrollment.
2. [Easy Mode](easy-mode.md) — the simplified, single-page dashboard.
3. [Full Mode](full-mode.md) — the dense, by-domain pages.
4. [Safe Apply](safe-apply.md) — every change that touches the network
   goes through this. Read this before making any non-trivial change.

## By-area guides

| Topic                | Guide                                     |
|----------------------|-------------------------------------------|
| Easy Mode dashboard  | [easy-mode.md](easy-mode.md)              |
| Full Mode navigation | [full-mode.md](full-mode.md)              |
| Interfaces           | [interfaces.md](interfaces.md)            |
| WAN configuration   | [wan.md](wan.md)                          |
| LAN and VLAN         | [lan-vlan.md](lan-vlan.md)                |
| Routing              | [routing.md](routing.md)                  |
| Firewall             | [firewall.md](firewall.md)                |
| DNS and DHCP         | [dns-dhcp.md](dns-dhcp.md)                |
| WireGuard tunnels    | [wireguard.md](wireguard.md)              |
| Xray                 | [xray.md](xray.md)                        |
| Safe Apply           | [safe-apply.md](safe-apply.md)            |
| Global search and Inspector | [search-and-inspector.md](search-and-inspector.md) |
| System / Health      | [system.md](system.md)                    |
| Doctor / diagnostics | [doctor-and-diagnostics.md](doctor-and-diagnostics.md) |
| Updates / rollback   | [updates-and-rollback.md](updates-and-rollback.md) |
| Recovery basics      | [recovery.md](recovery.md)                |
| Troubleshooting      | [troubleshooting.md](troubleshooting.md)  |

## Caveats

This guide describes the **V1 beta** as it exists today. Many areas
listed below are partially implemented; the guide is honest about which
controls work end-to-end and which are gated behind the
qualification/activation gates. See [`docs/beta-limitations.md`](../beta-limitations.md)
and the [release notes](../release-notes.md) for the current
state.

The guide uses the same product terminology as the UI. Words like
"Easy Mode", "Full Mode", "Safe Apply", "Inspector", "Doctor", "Owner"
have the same meaning in the UI as in this document.

If the UI and this guide disagree, **the UI and the source are
authoritative**. File an issue against this guide rather than editing
it silently.