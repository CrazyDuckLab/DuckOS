# Interfaces

DuckOS V1's Interfaces page is the inventory of every network
interface observed on the installed system. It is in Full Mode under
**Network** → **Interfaces**.

The page is read-only at the V1 beta: it shows what the system has
discovered, not what you can change. Configuration is currently
gated behind the qualification gates.

## What the page shows

For each interface the page lists:

- **Name** — the kernel name (`eth0`, `wlan0`, `br-lan`, ...).
- **State** — `up`, `down`, `unknown`, or `notpresent`. Read from the
  kernel, not inferred.
- **MAC address** — the link-layer address. Sanitized in the public
  documentation.
- **MTU** — the configured MTU.
- **Addresses** — the IPv4 and IPv6 addresses currently assigned. The
  data model is IPv6-first; IPv4-only configurations are not
  preferred.
- **Driver / bus** — the kernel driver and bus type, when known.

The page also shows the management interface separately. The
management interface is special: it carries the management session
and is protected from accidental removal by Safe Apply's anti-lockout
guard.

## Common workflows

- **Find the management interface.** It is the interface whose
  description matches the management LAN. The UI marks it
  explicitly.
- **Verify link state.** Look at the State column. `up` means the link
  is up; `down` means the link is administratively or physically down;
  `unknown` means we have no recent observation.
- **Diagnose a slow path.** Open the Interfaces page; check the
  per-interface state. If a particular interface shows
  `notpresent` or `down`, the issue is at the link layer, not at the
  firewall or the routing layer.

## What you should NOT expect from the page

- The V1 beta does not yet support editing interface addresses
  through the public web UI.
- The page does not show **traffic counters** in V1 beta; that is a
  later capability.

See [`interfaces.md`](interfaces.md) for the design rationale and
[`docs/architecture.md`](../architecture.md) for the data model.