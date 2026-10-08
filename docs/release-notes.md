# DuckOS v0.1.0-beta.2

DuckOS beta.2 expands the public platform builds and strengthens the local
networking runtime, privilege boundaries and operational interface.
**This is a beta prerelease. Fresh installation is required; no beta.1 → beta.2
in-place upgrade has been qualified.**

## Highlights

- Official Linux AMD64, ARM64 and ARMv7 archives.
- Bundled **xray-core 26.3.27** with real VLESS/TLS and VLESS/REALITY traffic
  qualification on the AMD64 production runtime.
- Two simultaneous WireGuard tunnels qualified between two nodes, including
  handshakes, ICMP/TCP/UDP traffic, counters, restart and cleanup.
- Separate WireGuard and FRR privileged launchers; zero Linux capabilities for
  `duck-agent`, and only `CAP_NET_ADMIN` for `duck-wg-launcher`.
- Port Mapping backend and Trusted LAN enforcement, with nftables lifecycle,
  TTL expiry, revoke and cleanup; Wake-on-LAN and Speed Lab.
- RBAC and network-object fixes, Easy / Full UI improvements, EN / RU / DE / ES,
  and improved IPC, systemd startup and reboot recovery.

## Platform support

| Platform | Release archive | Build/artifact verification | Complete final live matrix |
|---|---|---|---|
| Linux AMD64 / x86_64 | `duckos-v0.1.0-beta.2-linux-amd64.tar.gz` | Yes | Yes, two Debian 13 VMs |
| Linux ARM64 / AArch64 | `duckos-v0.1.0-beta.2-linux-arm64.tar.gz` | Yes | No |
| Linux ARMv7 / 32-bit ARM | `duckos-v0.1.0-beta.2-linux-armv7.tar.gz` | Yes | No |

Identify the architecture with `uname -m` and `dpkg --print-architecture`:
`x86_64` / `amd64` selects AMD64; `aarch64` / `arm64` selects ARM64;
`armv7l` / `armhf` selects ARMv7. The ARMv7 Go target is
`GOARCH=arm GOARM=7`. ARM builds are official targets with a narrower evidence
level than AMD64. The physical ODROID XU4 was excluded from final qualification.

## Networking and privilege boundaries

WireGuard operations use a constrained privileged launcher rather than granting
network capabilities to the Agent. Native `wg-quick@.service` is masked to avoid
competing lifecycle ownership. Peer materialization, idempotent address handling
and reboot startup defects were corrected.

FRR uses a separate privileged launcher and native `frr.service` is masked.
The final lab verified launcher startup and explicit refusal behavior; zebra
and bgpd were not installed there, so this is not a live BGP routing claim.

Port Mapping ships the backend, Trusted LAN Registry and nftables mapping
lifecycle with TTL, revoke and cleanup. **UPnP IGD, SSDP/M-SEARCH, NAT-PMP and PCP
network protocol frontends are not included.**

## Xray and security

The release bundles xray-core **26.3.27**. AMD64 qualification passed real
VLESS/TLS traffic and real VLESS/REALITY traffic using an xray-core client and a
controlled TLS camouflage endpoint. Credentials and key material are resolved
through the protected Vault path. Empty rendered inbounds now stop the daemon,
keeping catalog and runtime behavior consistent.

RBAC boundaries, constrained launcher IPC and systemd runtime ownership received
fixes and verification. The Agent effective capability set is zero; the
WireGuard launcher effective capability set is `CAP_NET_ADMIN` only.
These results do not constitute a completed product security audit.

## UI and operations

Easy and Full modes share the configuration model. This beta improves
network-object handling, authentication/role behavior and operational UI flows
across English, Russian, German and Spanish. Wake-on-LAN and Speed Lab remain
bounded by their configured network and target policies. API availability is
separate from network health; incomplete observations remain visible.

## Installation and checksums

**Fresh installation only, on Debian 13 with systemd.** Use the architecture
archive above and the release's top-level `SHA256SUMS`.
Follow [the installation guide](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.2/docs/install.md)
for download, SHA-256 verification, root-owned staging, preflight and installation.
Each archive extracts to `duckos-v0.1.0-beta.2/`.

```bash
sha256sum --check --ignore-missing SHA256SUMS
```

Require an `OK` result for the archive you downloaded. The supplied installer
also checks the internal manifest and architecture-specific ELF headers.
Checksums detect corruption; they do not independently authenticate a publisher.
No signed `.duck` beta.1 → beta.2 upgrade package is supplied.

## Known limitations

- Beta software; isolated evaluation and out-of-band console access recommended.
- Fresh install required; no qualified beta.1 → beta.2 in-place upgrade.
- No UPnP IGD / SSDP-M-SEARCH, NAT-PMP or PCP protocol frontends.
- ARM64 and ARMv7 did not undergo the complete AMD64 final two-node live matrix.
- FRR daemon data-plane routing, physical board/chipset variance and power-loss
  behavior are not established by the final VM qualification.
- No public OTA delivery channel, automatic LAN TLS onboarding or mDNS discovery.

## Verification and source identity

The full live qualification used product commit
`90be1e00ae6c364f25d195ffe5223e8525e1310c` and build
`1.0.0-dev.90be1e00ae6c`. Documentation/report-only follow-ups do not alter the
qualified product behavior. Final archives must be built from the same release
commit as the annotated tag; `BUILD-INFO.json` records that full source commit,
and `VERSION` uses `1.0.0-dev.<12-character-commit>`.

See the [final qualification report](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.2/beta2-final-report.md)
for the evidence and its limits. The top-level checksum file covers all three
public archives; each archive also contains an internal `SHA256SUMS`.

Source remains GPL-3.0-only; Xray licensing follows its upstream distribution.
Report bugs with sanitized logs; follow [SECURITY.md](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.2/SECURITY.md)
for vulnerabilities.

---

# DuckOS v0.1.0-beta.1

First public beta of DuckOS.

DuckOS V1 beta is a public beta of the DuckOS V1 frontend plus a
qualified subset of the V1 backend. It is **not** a production router.
It is a beta that observers and contributors can install, evaluate, and
report against on an isolated test network.

## License

DuckOS Community source in this release is distributed under
**GPL-3.0-only** — see [`LICENSE`](../LICENSE) and
[`COPYRIGHT.md`](../COPYRIGHT.md). Trademarks and brand use are
governed separately by [`TRADEMARKS.md`](../TRADEMARKS.md). The
commercial / dual-licensing narrative is in
[`docs/licensing.md`](licensing.md). A finalized Contributor License
Agreement ([`CLA.md`](../CLA.md)) is **DRAFT**; external code
contributions are not accepted against this beta.

## What ships in v0.1.0-beta.1

- React / TypeScript UI with **Easy Mode** and **Full Mode**.
- Full Easy Mode page catalog (Home, Internet, Wi-Fi, Devices, VPN,
  Care / Recovery).
- Full Full Mode page catalog across System, Network, Security,
  DNS / DHCP, VPN / Tunnels, Updates, Audit / Help.
- One global search (`⌘K` / `Ctrl+K`).
- Inspector pane (`?`) for contextual reference on every page.
- First Owner enrollment, password login, built-in RBAC, expiring /
  revocable sessions.
- Doctor subsystem with deterministic findings.
- Localized in English, Russian, German, Spanish.
- Signed offline update foundations: `.duck` packages, ed25519
  signature verification, durable Active + Previous store, systemd
  restart with signed `/api/v1/ready` health probe.
- Local-first operation: management UI and Agent bind only
  `127.0.0.1:8080` on the device; no required SaaS, no automatic LAN
  exposure.

## What is qualified in v0.1.0-beta.1

- Build target: **`amd64`** on a **Debian 13** host.
- Reference-supported targets: `arm64`, `armv7` (cross-build only,
  no native qualification yet).
- The fresh Debian 13 amd64 TEST installer.

## What is NOT in v0.1.0-beta.1

The following catalog subsystems are present in the UI as observed
state, but **not** as live configuration through the public web UI:

- Live WAN / LAN / VLAN / DHCP / DNS / firewall / route / policy
  configuration through Safe Apply.
- Multi-WAN live activation.
- WireGuard / Xray / NetBird live activation.
- Live Client provisioning and subscriptions.
- Doctor remediation automation.
- Update supervisor with the `Active + Previous` invariant on a
  running system surfaced in the public UI.

The V1 beta does **not** ship:

- mDNS or link-local discovery
- built-in TLS / ACME / browser-trusted onboarding
- a public OTA delivery channel
- native ARMv7 qualification

See [`beta-limitations.md`](beta-limitations.md) for the full list
and for what is intentionally **not** a limitation.

## Install path

The V1 beta install path is described in
[`docs/install.md`](install.md). The short version:

1. Verify the artifact digest against `SHA256SUMS` next to this
   archive:

   ```bash
   sha256sum -c SHA256SUMS
   ```

   Expected line for v0.1.0-beta.1:

   ```
   ae5d21aa19e3b39efaee1ba070158781df4aae5605adae096b8563f241dcdc7d  duckos-v0.1.0-beta.1-linux-amd64.tar.gz
   ```

2. Install on an isolated test network — never on a production
   gateway:

   ```bash
   tar -xzf duckos-v0.1.0-beta.1-linux-amd64.tar.gz
   cd duckos-v0.1.0-beta.1
   sudo bash install.sh --artifact "$(pwd -P)"
   ```

3. Reach the dashboard through an SSH local-forward from an
   administrator host. The V1 beta binds only `127.0.0.1:8080`:

   ```bash
   ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
   ```

   Then in a modern Chromium or Firefox browser:

   ```
   http://127.0.0.1:8080
   ```

4. Complete first-time Owner enrollment.
5. Follow [`docs/user/getting-started.md`](user/getting-started.md).

## Known limitations

- The Easy Mode headline may say **Status check incomplete** on a
  healthy-looking system until the backend grows verified-health
  evidence. This is intentional.
- The Doctor does not yet have a generative remediation assistant.
- The DuckOS team has not completed native ARMv7 qualification.

## Upgrade note

This is the first public beta. There is no prior public release to
upgrade from.

## Recovery note

The pre-networkd boot recovery unit is qualified in tests and ships
with the install image. Real-reboot qualification is pending; the
administrator is still responsible for keeping an out-of-band
recovery console during the first install on new hardware.

See [`docs/user/recovery.md`](user/recovery.md) for the recovery
primitives.

## Reporting bugs

- Open an issue following the bug report template.
- Sanitize logs before attaching them. Do not publish private keys,
  subscription URLs, or production network details.
- For security vulnerabilities, follow [`../SECURITY.md`](../SECURITY.md).

## Build identity

This release corresponds to internal qualified build
**`1.0.0-dev.f698de16b4a9`** from public source commit
**`f698de16b4a90c52d7bd5b50f287ad01f36da5f3`**.

The `v0.1.0-beta.1` identifier is the public release name (the Git
tag and the downloadable filename). The runtime reports the qualified
internal identifier `1.0.0-dev.f698de16b4a9` exactly as it did
throughout qualification. This is intentional and **not** an error:

- The supported build script (`scripts/build.sh`) hardcodes the
  `1.0.0-dev.<commit>` prefix.
- The supported installer (`scripts/install.sh`) rejects any
  `VERSION` that does not match `^1\.0\.0-dev\.[0-9a-f]{12}(\.dirty)?$`.
- Patching the version logic was explicitly out of scope for this
  release.

The four control binaries — `duck-updater`, `duck-api`, `duck-agent`,
`duck-restore` — match the authoritative hashes for the
`f698de16b4a90c52d7bd5b50f287ad01f36da5f3` qualified build recorded
in [`RELEASE_MANIFEST.txt`](https://github.com/DuckOS/DuckOS/releases/download/v0.1.0-beta.1/RELEASE_MANIFEST.txt).
The bundled `SHA256SUMS` (inside the artifact) verifies every file in
the `duckos-v0.1.0-beta.1/` directory against the manifest that the
installer checks at install time.

## What this release is NOT

- It is not a public production release.
- It is not feature-complete.
- It is not a substitute for the qualification work described in
  [`docs/beta-limitations.md`](beta-limitations.md) and the project's
  qualification record for this release.
- It is not an announcement of a future central management /
  controller product.