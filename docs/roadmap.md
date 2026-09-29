# Public roadmap

This page is the public roadmap. It exists to be honest about what
ships in V1 beta and what does not. It does not contain promises
beyond the V1 frozen frontend baseline.

## V1 BETA — now

The V1 beta is the **only** state a public visitor can use today.

- React / TypeScript UI with **Easy Mode** and **Full Mode**.
- Real Linux observation through React → API → Engine → privileged
  Agent for the **read-only** surfaces. The current system observes
  Linux state; transactional network activation is gated.
- First Owner enrollment, password login, built-in RBAC, expiring /
  revocable sessions.
- Doctor subsystem that surfaces findings.
- Global search and Inspector.
- English / Russian / German / Spanish.
- IPv6-first data model.
- Local-first operation; no mandatory SaaS dependency.
- `amd64` build target is qualified; `arm64` and `armv7` are
  reference-supported only.

The V1 beta does **not** include the following, even though they
appear in the V1 catalog:

- Live WAN / LAN / VLAN / DHCP / DNS / firewall / route / policy
  configuration through the public web UI.
- Multi-WAN live activation.
- WireGuard / Xray / NetBird live activation.
- Live Client provisioning and subscriptions.
- Doctor remediation automation.
- Update supervisor with the `Active + Previous` invariant on a
  running system.

These are tracked under [`docs/beta-limitations.md`](beta-limitations.md).

## LATER — V1 → 1.0

The LATER bucket is the same V1 catalog, not new features. It is the
list of subsystems that are not yet wired up to Safe Apply through the
public UI, in the rough order in which they would be wired up:

1. Live network configuration through the public UI.
2. Multi-WAN live activation with policy routing.
3. WireGuard server and peer live activation.
4. Xray live activation with subscription import.
5. Live client provisioning with QR / `.conf` exports.
6. Live update supervisor with the `Active + Previous` invariant
   reachable from the UI.
7. Bounded product logging, audit retention / export, support
   bundle redaction.

These items are engineering. They are not yet qualified for V1, and
their order is a working order, not a delivery commitment.

## FUTURE — beyond 1.0

The FUTURE bucket is **explicitly not part of V1 and not part of the
2026 work**. Items here are mentioned for context only.

- A central management / controller product (sometimes referred to in
  earlier internal notes as "UltraDuck") is **not** part of V1. It is
  not in the public catalog, not in the V1 frozen frontend, and not
  scheduled. Do not imply it exists.
- Cloud-hosted control plane. **Not** part of V1; not a roadmap
  commitment.
- A managed commercial offering. **Not** part of V1.

## What this roadmap deliberately does not do

- It does not promise dates.
- It does not promise a 1.0 release date.
- It does not promise feature parity with any other product.
- It does not promise that the FUTURE bucket will ever ship.

A clean roadmap is honest about scope. The LATER bucket is the only
place where engineering commitments are recorded. The FUTURE bucket
is for context only.