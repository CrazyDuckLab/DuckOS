# DuckOS beta.3 compatibility

Prerelease for fresh Debian 13 with systemd. No beta.1/beta.2 → beta.3 in-place
upgrade is qualified. Use an isolated evaluation network and out-of-band console.

| Target | Release packaging | Execution evidence |
|---|---|---|
| AMD64 / x86_64 | Cross-build, ELF, inventory and hash verification | Native Debian 13 AMD64 Router Core qualification |
| ARM64 / AArch64 | Cross-build, ELF, inventory and hash verification | No native hardware qualification |
| ARMv7 / armhf | Cross-build, ELF, inventory and hash verification | No native hardware qualification; `GOARCH=arm GOARM=7` |
| Other systems | No official target | Not qualified |

The accepted AMD64 scope is Router Core regressions, firewall/NAT packet tests,
installer, real systemd startup, authenticated readiness, service restart and VM
reboot persistence. See [qualification](qualification-beta3-final.md).
This does not establish physical power-loss, arbitrary NIC/board, Wi-Fi/modem,
every FRR protocol or universal UPnP/PCP-client behavior. Historical beta.2
WireGuard/Xray evidence remains historical and is not a new beta.3 full matrix.

Use an administrator identity separate from service identities. Do not delete
existing installation state or conflicting units to bypass installer preflight.
DuckOS is an installer payload on Debian, not a bootable image or arbitrary
consumer-router firmware. See [install](install.md) and
[limitations](beta-limitations.md).
