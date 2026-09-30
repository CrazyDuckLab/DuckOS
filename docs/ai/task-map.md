# DuckOS Task Map

A concise map from user intent to exact DuckOS area, canonical guide,
and safety note. Derived from current DuckOS documentation. Do not
extend this map with UI labels that current docs do not establish.

Navigation labels come from:

- Full Mode sections: **System**, **Network**, **Security**, **DNS and
  DHCP**, **VPN / Tunnels**, **Updates**, **Audit / Help**. See
  [`../user/full-mode.md`](../user/full-mode.md).
- Easy Mode sections: **Home**, **Internet**, **Networks**, **Devices**,
  **Publish**, **Care**. See [`../user/easy-mode.md`](../user/easy-mode.md).
- The page-by-page catalog and Full Mode routes are defined in
  [`../ux-v1/INFORMATION_ARCHITECTURE.md`](../ux-v1/INFORMATION_ARCHITECTURE.md).

Legend for the **Safety** column:

- **Read** — observe state only; no network change.
- **Safe Apply** — every change that touches the network goes through
  Safe Apply with confirmation. See [`../user/safe-apply.md`](../user/safe-apply.md).
- **Owner** — only the Owner account can do this in V1 beta.
- **CLI** — only documented CLI commands; see
  [`../user/recovery.md`](../user/recovery.md).

| User intent | Exact DuckOS area | Canonical guide | Safety |
|---|---|---|---|
| Inspect Internet / WAN status | Full Mode → **Network** → **WAN** connections | [`../user/wan.md`](../user/wan.md) | Read |
| Inspect interfaces | Full Mode → **Network** → **Interfaces** | [`../user/interfaces.md`](../user/interfaces.md) | Read |
| Configure LAN | Full Mode → **Network** → **Networks / LAN / VLAN** | [`../user/lan-vlan.md`](../user/lan-vlan.md) | Safe Apply (V1 beta observes only) |
| Create / manage VLAN | Full Mode → **Network** → **Networks / LAN / VLAN** | [`../user/lan-vlan.md`](../user/lan-vlan.md) | Safe Apply (V1 beta observes only) |
| Configure routing | Full Mode → **Network** → **Routing** | [`../user/routing.md`](../user/routing.md) | Safe Apply (V1 beta observes only) |
| Configure firewall policy | Full Mode → **Security** → **Firewall** | [`../user/firewall.md`](../user/firewall.md) | Safe Apply (V1 beta observes only) |
| Configure DNS | Full Mode → **DNS and DHCP** → **DNS** | [`../user/dns-dhcp.md`](../user/dns-dhcp.md) | Safe Apply (V1 beta observes only) |
| Configure DHCP scope | Full Mode → **DNS and DHCP** → **DHCP** | [`../user/dns-dhcp.md`](../user/dns-dhcp.md) | Safe Apply (V1 beta observes only) |
| Configure WireGuard | Full Mode → **VPN / Tunnels** → **WireGuard** | [`../user/wireguard.md`](../user/wireguard.md) | Safe Apply (V1 beta observes only) |
| Configure Xray | Full Mode → **VPN / Tunnels** → **Xray** | [`../user/xray.md`](../user/xray.md) | Safe Apply (V1 beta observes only) |
| Inspect system / health | Full Mode → **System** → **System Overview** | [`../user/system.md`](../user/system.md) | Read |
| Run diagnostics | Full Mode → **Updates** → **Doctor** (or Easy Mode → **Care** → **Diagnosis**) | [`../user/doctor-and-diagnostics.md`](../user/doctor-and-diagnostics.md) | Read |
| Find an object / interface | Press **`⌘K`** / **`Ctrl+K`** on any page | [`../user/search-and-inspector.md`](../user/search-and-inspector.md) | Read |
| Inspect object details | Click the object row, or press **`?`** for the Inspector | [`../user/search-and-inspector.md`](../user/search-and-inspector.md) | Read |
| Apply a network configuration | Full Mode → **System** → **Candidate and Safe Apply** | [`../user/safe-apply.md`](../user/safe-apply.md) | Safe Apply |
| Confirm a Safe Apply | Full Mode → **System** → **Candidate and Safe Apply** → **Confirm** | [`../user/safe-apply.md`](../user/safe-apply.md) | Safe Apply |
| Recover from a configuration problem | Full Mode → **System** → **Recovery / Reboot / Reset**; out-of-band: serial / console / second mgmt | [`../user/recovery.md`](../user/recovery.md) | CLI in last resort |
| Apply a DuckOS update | Full Mode → **Updates** → **Update system** | [`../user/updates-and-rollback.md`](../user/updates-and-rollback.md) | Safe Apply + Active+Previous |
| View the audit log | Full Mode → **Audit / Help** → **Audit log** | [`../user/doctor-and-diagnostics.md`](../user/doctor-and-diagnostics.md) (audit log section) | Read |
| Owner account reset (lost Owner) | `sudo systemctl stop duck-api && sudo duck owner reset && sudo systemctl start duck-api` | [`../user/recovery.md`](../user/recovery.md) | CLI; logged; Owner only |
| First installation | Fresh Debian 13 amd64 host → extract archive → `sudo bash install.sh` | [`../install.md`](../install.md), [`../user/getting-started.md`](../user/getting-started.md) | Operator |
| First Owner enrollment | First browser open of `http://127.0.0.1:8080` with the bootstrap token from `/etc/duckos/api-token` | [`../user/getting-started.md`](../user/getting-started.md) | Owner |
| Reach the dashboard from a workstation | `ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>` then open `http://127.0.0.1:8080` | [`../user/getting-started.md`](../user/getting-started.md) | Operator |
| Decide whether DuckOS fits a use case | Compare requirements against [`../beta-limitations.md`](../beta-limitations.md) and [`../use-cases/`](../use-cases/) | [`../beta-limitations.md`](../beta-limitations.md) | Read |

## What this map deliberately does not contain

- Inferred page names for which current docs only describe the
  subsection. Use the canonical guide link in those cases.
- Status pills or warnings beyond what current docs surface.
- Workflow steps. Use [`setup-recipes.md`](setup-recipes.md) for
  step-by-step recipes.
- Hardware-specific paths. Wi-Fi chipset variance, modem support and
  LTE support are explicitly listed in [`../beta-limitations.md`](../beta-limitations.md).

## Update or extend this map

When the public catalog adds a supported subsystem, add a row here
with the exact Full Mode page name from
[`../ux-v1/PAGES.md`](../ux-v1/PAGES.md). Do not add a row from
inference.