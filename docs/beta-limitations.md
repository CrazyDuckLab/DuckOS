# V1 beta limitations

This page is the honest list of what V1 beta is **not**. It exists
because the public catalog lists many subsystems, and the V1 beta
renders many of them as observed state rather than as live
configuration.

Every item below is a real limitation in the current beta. Where a
subsystem is partially wired, this page says so. Where a subsystem
is qualified on one architecture but not another, this page says so.

## Architecture

- **Qualified**: `amd64` only, on a Debian 13 host target.
- **Reference-supported**: `arm64` and `armv7` — the code
  cross-builds, but native qualification on these targets has not
  been completed.
- **ODROID XU4** is the minimum resource reference for ARMv7.
- The **V1 beta** is not a production router. It is a beta of an
  observability and configuration platform that does not yet
  activate every catalog feature.

## Network configuration through the public UI

- The V1 beta **observes** the network. It does **not write** to
  `/etc/network/interfaces`, networkd, nftables, dnsmasq, frr,
  WireGuard or Xray through the public UI.
- Safe Apply is the only path from candidate to active state, and
  the Safe Apply integration for catalog subsystems is gated behind
  qualification. The V1 beta does not yet expose Safe Apply for
  every subsystem the public catalog lists.

## Multi-WAN

- Rendered against observed state.
- Live activation through the public UI is gated.

## WireGuard / Xray / NetBird

- Rendered against observed state.
- Live activation through the public UI is gated.
- Subscription URLs are stored as encrypted secrets. They are never
  displayed in cleartext. QR codes that embed subscription URLs are
  redacted.

## DNS and DHCP

- Rendered against observed state (lease file, resolver
  configuration).
- Live DHCP-scope editing through the public UI is gated.

## Updates

- The update supervisor exists in code. The `Active + Previous`
  invariant is implemented in tests.
- The release-channel surface in the public UI is gated.
- The V1 beta is a single bundled release. There is no
  auto-update channel.

## Doctor and diagnostics

- Doctor runs a fixed set of deterministic checks.
- Doctor does not have a generative remediation assistant.
- Doctor does not propose configuration changes. It surfaces
  findings and links to the relevant recovery action.

## Hardware support

- Wi-Fi hardware variance is high. The V1 beta does not test
  every chipset. Use a chipset listed in
  [`docs/hardware-support.md`](hardware-support.md) before deploying.
- Modems and LTE are not supported.
- Real ARMv7 native qualification is pending. Cross-build success
  is not native qualification.

## LAN onboarding

- The V1 beta binds the management UI/API to `127.0.0.1:8080` only.
  No mDNS / DNS-SD advertisement, no automatic LAN URL, no browser
  discovery, no self-signed HTTPS onboarding, no ACME client, and no
  zero-touch flow ship with this beta.
- Administrators reach the dashboard via SSH local-forward
  (`ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>`) and open
  `http://127.0.0.1:8080`. See [`docs/user/getting-started.md`](user/getting-started.md).
- LAN-friendly onboarding improvements may land in a later release;
  this beta makes no commitment to that surface.

- Wi-Fi hardware variance is high. The V1 beta does not test
  every chipset. Use a chipset listed in
  [`docs/hardware-support.md`](hardware-support.md) before deploying.
- Modems and LTE are not supported.
- Real ARMv7 native qualification is pending. Cross-build success
  is not native qualification.

## Recovery

- The pre-networkd boot recovery unit exists and is qualified in
  tests.
- Real-reboot and power-loss qualification is pending.
- Phone-scan and real-forwarding qualification is pending.
- Hardware-specific native qualification is pending.

## Installer

- A fresh Debian 13 amd64 TEST installer exists. It is qualified
  in tests.
- Real-world install-path qualification on a wider set of
  hardware is pending.
- The installer does not currently enable the network or DNS
  services at install time; the administrator must provision
  them through Safe Apply.

## What's NOT a beta limitation

These are not limitations — they are intentional choices:

- The UI is honest about which signals the backend currently
  provides. The Easy Mode "Status check incomplete" verdict is
  the conservative default and is not a bug.
- Safe Apply uses a forwarding guard. The guard is what makes a
  mid-Apply state safe. There is no "apply now" shortcut.
- The Agent unit is sandboxed through `PrivateNetwork=no`,
  `IPAddressDeny=any`, `RestrictAddressFamilies`, and an empty
  `CapabilityBoundingSet`. We do not relax these.
- IPv6 is first-class. IPv4-only shortcuts are not in scope.

## Reporting a bug or limitation you think is missing

Please open an issue against this repository following the bug
report template. Sanitize logs before attaching them. Do not
publish private keys, subscription URLs, or production network
details.

## Reporting a vulnerability

See [`SECURITY.md`](../SECURITY.md).