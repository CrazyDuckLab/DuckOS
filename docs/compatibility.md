# DuckOS beta.2 compatibility

DuckOS beta.2 installs on Debian 13 with systemd. It is **fresh-install-only**;
there is no qualified beta.1 → beta.2 in-place upgrade.

## Platform and evidence matrix

| Platform | Official release artifact | Build/artifact verification | Complete beta.2 live qualification |
|---|---|---|---|
| Debian 13 AMD64 / x86_64 (`amd64`) | Yes | Yes | Yes, two lab VMs |
| Debian 13 ARM64 / AArch64 (`arm64`) | Yes | Yes | No |
| Debian 13 ARMv7 / 32-bit ARM (`armhf`) | Yes | Yes | No |
| Other distributions or architectures | No | No | No |

ARMv7 uses `GOARCH=arm GOARM=7`. Artifact names use `armv7`, while Go uses `arm`.
ARM64 and ARMv7 are official release targets; build verification does not imply
the same two-node live matrix as AMD64. The physical ODROID XU4 was not part of
final beta.2 qualification. No per-device physical hardware matrix is established.

The AMD64 matrix covers the qualified runtime and targeted networking flows,
including FRR H10 BGP advertise/withdraw/restart on AMD64. It does not cover
arbitrary NICs, Wi-Fi hardware or production readiness.
See [current qualification](qualification-beta2.md).

## Host and hardware boundaries

- A booted Debian 13 systemd host is required; non-Linux hosts are not install targets.
- Ethernet, storage and peripheral drivers remain Debian/kernel responsibilities.
- Wi-Fi chipset, modem/LTE and physical power-loss evidence is not supplied by this release.
- DuckOS is not firmware for arbitrary consumer routers or ISP appliances.
- Keep console access and preserve the management interface during evaluation.

## Service identities

Use an administrator identity separate from DuckOS service accounts.
The current installer tolerates existing `duckos` / `duck-dnsmasq` names;
pre-existing installation paths and conflicting units still cause refusal.
Do not delete host identities or installation state to bypass preflight.
See [installation](install.md) for exact staging and preflight commands.

## Runtime scope

[Beta limitations](beta-limitations.md) distinguishes qualified subsystem behavior
from the broader target specification. Port Mapping includes its backend and
Trusted LAN lifecycle, without UPnP IGD, SSDP/M-SEARCH, NAT-PMP or PCP frontends.
