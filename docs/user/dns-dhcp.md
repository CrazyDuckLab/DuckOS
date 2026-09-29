# DNS and DHCP

DuckOS uses **dnsmasq** as the local DNS and DHCP service. The DNS and
DHCP service runs under a dedicated non-login identity
(`duck-dnsmasq`) with a strict sandbox.

## Concepts

- **Readiness marker** — the DNS and DHCP service writes a readiness
  marker (`/run/duckos-agent/network-ready`) only after it has
  validated its configuration. Downstream services must wait for
  this marker.
- **DHCP scopes** — each scope is a separate DNS / DHCP configuration
  bound to a LAN or VLAN. The scopes are owned by the DNS service.
- **DNS resolvers** — each LAN can specify the upstream resolvers.
  DuckOS does not silently override.

## What the V1 beta does

- Renders observed DHCP leases and DNS configuration.
- Validates a candidate scope against the canonical model.

## What the V1 beta does NOT do

- It does not yet edit DHCP scopes through the public UI.

See [`docs/network-understanding.md`](../network-understanding.md) and
[`docs/route-compilation.md`](../route-compilation.md) for the
underlying model.