# Changelog

## v0.1.0-beta.2

Beta prerelease; fresh installation only. No qualified beta.1 → beta.2 in-place upgrade.

- Added official AMD64, ARM64 and ARMv7 release artifacts with platform-specific verification.
- Bundled xray-core 26.3.27; qualified real VLESS/TLS and VLESS/REALITY client traffic on AMD64.
- Protected Xray credential materialization through Vault; corrected empty-inbound daemon lifecycle.
- Improved WireGuard launcher dispatch, peer materialization, address idempotence and boot startup; qualified two simultaneous two-node tunnels.
- Kept duck-agent at zero Linux capabilities and duck-wg-launcher at CAP_NET_ADMIN only.
- Added FRR launcher ownership and native daemon conflict handling.
- Delivered Port Mapping backend, Trusted LAN enforcement, nftables lifecycle, TTL/revoke/cleanup, Wake-on-LAN and Speed Lab.
- Fixed RBAC/network objects, Easy / Full UI and runtime IPC/systemd reliability; retained EN/RU/DE/ES.

ARM64 and ARMv7 have build/artifact evidence but did not receive the complete AMD64 live matrix.
No UPnP IGD, SSDP/M-SEARCH, NAT-PMP or PCP protocol frontends are included.
See [release notes](docs/release-notes.md) and [qualification evidence](beta2-final-report.md).

## v0.1.0-beta.1

First public beta. Its historical scope, identity and limitations are preserved
in [the release record](docs/release-notes.md#duckos-v010-beta1).
