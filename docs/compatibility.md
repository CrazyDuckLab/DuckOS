# DuckOS compatibility

This page is the honest, evidence-backed summary of what `v0.1.0-beta.1`
is qualified to run on. It is intentionally conservative: a capability
that is not listed as qualified is not supported by this beta.

For the engineering policy that governs how a profile becomes
qualified, see [`hardware-support.md`](hardware-support.md). For the
runtime behavior that is and is not wired to live activation in V1
beta, see [`beta-limitations.md`](beta-limitations.md).

## Reading the status columns

| Status | Meaning |
|---|---|
| **QUALIFIED** | Reproducible install + observed-state + recovery evidence exists for the documented procedure. |
| **REFERENCE / CROSS-BUILD** | The artifact builds for this target, but native qualification evidence has not been recorded for this beta. |
| **EXPECTED BUT NOT QUALIFIED** | Documented as a target family, but no per-target evidence has been recorded for this beta. |
| **NOT CURRENTLY SUPPORTED / NOT YET QUALIFIED** | Either explicitly out of scope for this beta or pending evidence. |

## Host platform

| Platform | Status for `v0.1.0-beta.1` |
|---|---|
| **Linux amd64 on Debian 13 (trixie)** | **QUALIFIED.** The release artifact was built and verified against this profile. The supported installer (`install.sh`) refuses any other distribution or pre-existing DuckOS installation. |
| Linux arm64 on Debian 13 | **REFERENCE / CROSS-BUILD.** The build system produces an arm64 artifact, but native qualification on real arm64 hardware has not been recorded for this beta. |
| Linux armv7 on Debian 13 | **REFERENCE / CROSS-BUILD.** The build system produces an armv7 artifact, but native qualification on real armv7 hardware has not been recorded for this beta. |
| Other Linux distributions | **NOT CURRENTLY SUPPORTED.** The installer refuses non-Debian hosts. |
| Other architectures | **NOT CURRENTLY SUPPORTED.** |
| Non-Linux hosts (BSD, Windows, macOS, …) | **NOT CURRENTLY SUPPORTED.** |

The qualified beta is a **Linux amd64 on Debian 13 with systemd**
platform. The supported installer enforces that combination
explicitly.

## Hardware target class

| Class | Status for `v0.1.0-beta.1` |
|---|---|
| Generic amd64 mini-PC / NUC / VM / router-class machine | **EXPECTED BUT NOT QUALIFIED** for this beta. A real NIC and real Debian 13 amd64 base are the only required inputs; the qualified beta was tested in the project's documented clean Debian 13 amd64 VM flow. No per-device hardware matrix is part of this beta. |
| ARM64 SBCs, ARMv7 boards (including ODROID XU4 as the project's documented reference board) | **NOT CURRENTLY QUALIFIED** on physical hardware for this beta. Cross-build artifacts are produced; native evidence is pending. |
| x86 servers / large amd64 VMs | **EXPECTED BUT NOT QUALIFIED** for this beta. Capability discovery and bounded resource behavior are designed for this class but no matrix is part of this beta. |
| Random consumer Wi-Fi routers / ISP-issued appliances | **NOT CURRENTLY SUPPORTED.** DuckOS is not a router firmware replacement for arbitrary consumer hardware. |

## Peripheral and link-layer support

| Peripheral / link layer | Status for `v0.1.0-beta.1` |
|---|---|
| Wired Ethernet NICs supported by Debian 13 amd64 mainline | **EXPECTED**, contingent on the kernel/driver shipped by Debian 13. DuckOS does not introduce its own NIC driver. |
| Wi-Fi adapters | **NOT CURRENTLY QUALIFIED.** Wi-Fi chipset variance is high; the V1 beta does not test every chipset. Confirm the chipset against [`hardware-support.md`](hardware-support.md) before deploying on Wi-Fi. |
| LTE / cellular modems, USB mobile WAN | **NOT CURRENTLY SUPPORTED** in this beta. |
| WireGuard tunnels | The subsystem is wired into the dashboard and rendered against observed state. Live activation through Safe Apply is gated behind qualification — see [`beta-limitations.md`](beta-limitations.md). |
| Xray tunnels | The subsystem is rendered against observed state. Live activation through Safe Apply is gated behind qualification. |
| Bridges, bonds, VLANs (IEEE 802.1Q) | Rendered against observed state in the dashboard. Live activation through the public UI is gated. |
| Mobile / USB WAN, Multi-WAN failover, Path quality probes | Rendered against observed state. Live activation through the public UI is gated. |
| NetBird and other third-party tunnel providers | Rendered against observed state. Live activation through the public UI is gated. |

## Network and identity assumptions

| Layer | Status |
|---|---|
| IPv6-first data model | First-class. IPv4-only shortcuts are not in scope. |
| systemd-networkd | The supported network renderer on the host. DuckOS does not replace it; it observes its state and stages Safe Apply changes through the privileged Agent. |
| nftables | The firewall backend. DuckOS owns the `inet duckos` table. |
| dnsmasq | The DNS and DHCP service. Runs under a dedicated non-login identity (`duck-dnsmasq`). |
| frr (BGP / OSPF / IS-IS / BFD) | Rendered against observed state. Live activation through Safe Apply is gated. |
| systemd / systemd-networkd / systemd-resolved | The host must use systemd. The installer checks this. |

## What this beta does NOT promise

- Production certification or formal security review.
- A complete supported hardware list.
- Auto-discovery / mDNS / DNS-SD / LAN-friendly onboarding.
- A self-signed browser-trusted certificate or ACME client.
- A public OTA / update server. Updates are signed `.duck` payloads
  applied manually.
- Hosted / cloud-managed service.
- A stable, public CLI / API contract. The V1 beta public surface is
  the dashboard over the SSH local-forward.
- Native qualification on ARM, ARM64, or armv7 hardware.

See [`beta-limitations.md`](beta-limitations.md) for the full list.

## Before you install

Confirm:

1. The host is a **fresh Debian 13 amd64** install with systemd and
   no pre-existing DuckOS installation. The installer refuses
   otherwise.
2. The host has **out-of-band recovery access** (serial, console, or
   second management interface). The qualified anti-lockout guard
   reduces lockout risk, but the second path is your safety net.
3. The network on which the device will run is **isolated** from any
   production gateway during the beta.
4. You have read [`user/recovery.md`](user/recovery.md),
   [`user/troubleshooting.md`](user/troubleshooting.md), and
   [`user/safe-apply.md`](user/safe-apply.md) before making any
   non-trivial change.

## Reserved host identity

For `v0.1.0-beta.1`, the host service identity names `duckos` and
`duck-dnsmasq` are reserved. The supported installer refuses to
install on a Debian host where an unrelated interactive account or
group already uses either of these names.

Before installing DuckOS:

- Confirm no local administrator or interactive account on the
  Debian host is named `duckos` or `duck-dnsmasq`
  (`getent passwd duckos`, `getent group duckos`,
  `getent passwd duck-dnsmasq`, `getent group duck-dnsmasq`).
- If a name collision exists, choose a different Debian
  administrator username (or rename the conflicting local account
  via the platform's normal user-management tools) **before**
  running the installer.

DuckOS does **not** reuse or impersonate a pre-existing human
account. The installer does **not** automatically delete or rename
host accounts. Reusing a human `duckos` account for the service is
unsafe and is not supported.