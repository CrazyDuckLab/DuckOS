# DuckOS v0.1.0-beta.2 limitations

**Beta software. Fresh installation required. No qualified beta.1 → beta.2
in-place upgrade.** Use an isolated network and preserve console access.

## Platforms and qualification

AMD64, ARM64 and ARMv7 are official release targets with build/artifact
verification. Only AMD64 received the complete final two-node live matrix on
Debian 13 VMs. ARMv7 is `GOARCH=arm GOARM=7`; the physical ODROID XU4 was excluded.
Cross-build and artifact evidence does not prove native hardware behavior.
See [compatibility](compatibility.md) and [the report](../beta2-final-report.md).

## Networking

Beta.2 qualified two simultaneous WireGuard tunnels and real Xray VLESS/TLS and
VLESS/REALITY client traffic on AMD64. These are subsystem-specific results,
not a claim that every page in the V1 catalog is a qualified network workflow.
Safe Apply, explicit confirmation and management protection remain required.
Do not infer a complete multi-WAN, DHCP/DNS, NetBird or arbitrary hardware matrix
from the release highlights.

FRR launcher startup and refusal behavior were verified. The final lab lacked
zebra/bgpd; live BGP routing is not established by that evidence.

Port Mapping includes the backend, Trusted LAN Registry, nftables lifecycle,
TTL expiry, revoke and cleanup. **UPnP IGD, SSDP/M-SEARCH, NAT-PMP and PCP network
protocol frontends are not included.**
Wake-on-LAN and Speed Lab are bounded by configured network/target policy.

## Installation, updates and recovery

- The supported installer requires fresh Debian 13 with systemd and matching architecture.
- Existing DuckOS installation paths/units are refused; do not bypass preflight.
- The release tarball is not a signed `.duck` upgrade package.
- There is no public OTA channel and no qualified beta.1 → beta.2 upgrade.
- Reboot/service recovery was exercised on the AMD64 lab VMs. Physical power-loss,
  hardware variance and wider install-path qualification remain separate work.

## Management and UI

- Use SSH local-forward to loopback; automatic LAN HTTPS onboarding and mDNS are absent.
- Easy and Full modes share the configuration model and support EN/RU/DE/ES.
- API reachability alone does not prove network health. Missing/stale observations
  remain visible; an incomplete headline is not proof of a broken network.
- Doctor provides deterministic findings; a generative remediation assistant is absent.
- Wi-Fi chipset and modem/LTE support are not established by the final VM matrix.

## Security and reporting

The Agent has zero effective Linux capabilities. WireGuard privileged mutations
use the separate CAP_NET_ADMIN-only launcher; FRR has its own constrained launcher.
Vault-backed secrets and RBAC fixes are part of beta.2, without claiming a complete
product security audit. Never publish private keys, credentials or session data.
Use repository issue templates for sanitized bugs and [SECURITY.md](../SECURITY.md)
for vulnerabilities.
