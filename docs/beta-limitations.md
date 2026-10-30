# DuckOS v0.1.0-beta.4 limitations

**Prerelease; fresh installation only.** No beta.1/beta.2 → beta.4 in-place
upgrade is qualified. Do not use an installer archive as a signed `.duck` update.
There is no public OTA channel. Keep console access and an isolated test network.

Only native Linux AMD64 received the accepted Router Core qualification:
regressions, firewall/NAT packet tests, installer, real systemd startup,
authenticated readiness, service restart and VM reboot persistence. ARM64 and
ARMv7 have cross-build and artifact checks without native hardware qualification.
Physical power loss, arbitrary hardware, every install/upgrade failure and
universal client interoperability remain unqualified. See
[qualification](qualification-beta3-final.md) and [compatibility](compatibility.md).

The Agent is UID 0 with zero effective capabilities. Separate nft and WireGuard
launchers carry CAP_NET_ADMIN; FRR has its own constrained launcher. F1–F8
remediation closes the recorded nft launcher defects without broadening Agent
capabilities. This is not a completed product security audit.

Port Mapping is disabled until operator/trusted-LAN admission. NAT-PMP, PCP
ANNOUNCE/MAP and UPnP IGD/SSDP frontend sources are included; full PCP, PEER,
unsupported options, IPv6 frontends and every third-party client are not claimed.
External-address observation has no complete multi-WAN qualification.
Typed firewall rules are forward-only; input/output filtering and the wider
networking/UI catalog are not qualified by that fact alone.

Xray-core remains pinned to 26.3.27. Xray Center beta.4, Subscription Center,
Stealth Admin, profile expansion and separate PPPoE/L2TP development are excluded.
Historical WireGuard, Xray and FRR beta.2 evidence is not a new full beta.4 matrix.

The archive has seven service binaries, not the standalone `duck` CLI. Use the
source-built CLI only with its documented contract. Automatic LAN TLS onboarding,
mDNS discovery and a physical Wi-Fi/modem matrix are absent. API liveness is not
readiness or verified network health. Recovery can fail; retain out-of-band access.

The current source snapshot excludes laboratory reports. Previously published
history can still contain historical private topology; snapshot cleanup does not
erase it. No active credential compromise is demonstrated by that finding.
Sanitize bug reports and follow [SECURITY.md](../SECURITY.md) for vulnerabilities.
