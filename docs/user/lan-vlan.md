# LAN and VLAN

The **Networks / LAN / VLAN** page in Full Mode shows the DuckOS
LAN model. DuckOS does not require a single management LAN; it can
have many VLANs, each with its own addressing and policy.

## Concepts

- **Management LAN** — the interface that carries the management
  session. It is the only interface Safe Apply will refuse to remove
  from the active set.
- **User LAN** — any other LAN. DuckOS does not assume that the
  management LAN is the only LAN.
- **VLAN** — IEEE 802.1Q VLAN. DuckOS addresses VLANs by `<parent>.<id>`
  in the kernel and reflects that into the page.
- **DHCP / DNS** — each LAN can have its own DHCP scope and DNS
  configuration. The dedicated DNS and DHCP service runs as a
  separate identity.

## Workflow

1. Identify the management interface in the **Interfaces** page.
2. Open **Networks / LAN / VLAN**.
3. Confirm the LAN exists with the expected addressing.
4. When the activation gate opens, edit the candidate and run Safe
   Apply.

## What the V1 beta does NOT do

- It does not yet edit DHCP scopes through the public UI.
- It does not yet bind DNS resolvers per LAN.

See [`docs/network-understanding.md`](../network-understanding.md) and
[`docs/route-compilation.md`](../route-compilation.md) for the
underlying model.