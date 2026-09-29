# WAN configuration

The **WAN connections** page in Full Mode (under **Network**) shows the
DuckOS uplink model. The V1 beta renders the page against the observed
state but does not yet write through the Safe Apply path.

## Concepts

- **Uplink** — the interface that carries traffic from the LAN to the
  Internet. A device can have multiple uplinks.
- **Source address selection** — DuckOS prefers IPv6 first; IPv4 is
  fallback. The system does not silently downgrade.
- **Multi-WAN** — the dedicated multi-WAN subsystem combines multiple
  uplinks with health and policy-routing hooks. The V1 beta renders
  the data but the activation gate is not yet open.
- **Path quality** — a per-uplink health measurement (latency, packet
  loss). The V1 beta renders the page; the live measurement is
  gated.

## Workflow

1. Identify the physical uplink interface in the **Interfaces** page.
2. Open **WAN connections** in Full Mode.
3. Confirm the uplink's address family, gateway, and DNS are correct.
4. When the activation gate opens, click **Apply** to run a Safe
   Apply with the multi-WAN configuration.

## What the V1 beta does NOT do

- It does not write firewall rules or policy routing from this page
  yet.
- It does not run live path-quality checks.

See [`routing.md`](routing.md) for the routing-side of multi-WAN, and
[`docs/network-understanding.md`](../network-understanding.md) for the
network model.