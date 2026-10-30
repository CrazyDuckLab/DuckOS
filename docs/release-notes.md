# DuckOS v0.1.0-beta.4 — Xray Center prerelease

DuckOS beta.4 integrates the accepted Xray Center/node security lineage with
Router Core. It adds authenticated server, client, subscription and node
management, protected-stdin credential rotation/recovery, fail-closed identity,
trusted peer scope and durable revocation. Catalog commits now preserve
memory/disk agreement, stop on uncertain durability and retain unrelated objects
when a server is removed. P050 opens the Center UI in all four supported locales.

The Agent remains UID 0 with zero capabilities and separate bounded privileged
launchers. This residual UID-0 file-access risk was explicitly accepted by the
owner. Apple-style UI / beta.5 and unrelated RC9 features are excluded.

Linux AMD64, ARM64 and ARMv7 are build targets. Cross-build and hash verification
are distinct from installed execution and physical hardware qualification.
The exact-source evidence and mandatory publication gates are described in
[beta.4 qualification](qualification-beta4.md). Final public notes must identify
which installation, systemd, traffic, upgrade and rollback checks actually ran.

Use fresh Debian 13 with systemd, console access and a verified recovery snapshot.
Verify the selected `duckos-v0.1.0-beta.4-linux-<arch>.tar.gz` against `SHA256SUMS`.
The fresh installer refuses existing installations; archives do not substitute
for authenticated `.duck` updates. The standalone CLI is source-built separately.
See [installation](install.md), [limitations](beta-limitations.md) and
[release identity](release-identity.md).

## Русский

Beta.4 объединяет Router Core с принятой веткой Xray Center: серверы, клиенты,
подписки и узлы используют аутентифицированный API / Engine. Ротация и
восстановление credentials передают секрет через stdin. Сохраняются fail-closed
identity, доверенная peer scope, durable revocation и разделение привилегий.
Исправлены долговечность каталога, изоляция удаления сервера и маршрут P050.

Это prerelease для изолированной оценки. Cross-build не подтверждает работу на
физических ARM-платах. Публикация разрешена только после обязательных проверок
финального SHA; их фактический объём должен быть указан в публичном отчёте.
Установочный tarball не является подписанным пакетом обновления.

---

## Historical beta.3 release record

The following notes retain beta.3's own scope and qualification baseline;
they do not establish beta.4 qualification.

# DuckOS v0.1.0-beta.3

DuckOS beta.3 hardens the Router Core and its installed privilege boundary.
This is a **prerelease for isolated evaluation**, using fresh Debian 13 with
systemd. Publication is a separate owner-approved action; these notes do not
assert that this candidate is already downloadable.

## Router Core security and privileged execution

The Agent runs as **UID 0 with zero effective capabilities**; it is not a
non-root process. nftables kernel operations pass through the separate
`duck-nft-launcher`, limited to `CAP_NET_ADMIN`, with `NoNewPrivileges=yes`.
The launcher authenticates Unix socket peers with SO_PEERCRED and restricts
operations to owned `inet duckos`, `inet duckos_fw` and `inet duckos_upnp`
objects. It exposes no shell or arbitrary execution interface.

F1–F8 remediation includes whole-batch text validation, strict JSON ownership
and duplicate-key checks, mandatory launcher payload and ELF validation,
PCP wire-format corrections, delayed-launcher startup/recovery handling,
failed-install launcher bookkeeping, server-side filtering of global reads,
and bounded connections, execution, queued bytes and deadlines.

## Firewall, NAT and recovery

Typed forward firewall rules use atomic nftables batches, semantic readback
verification and a durable Safe Apply journal. Operator confirmation is
required; timeout, failure and explicit rollback retain the recovery path.
Empty configurations use a canonical absent-table state. Firewall and NAT
mapping ownership are separate, with scoped restore and coexistence checks.

Port Mapping includes Trusted LAN admission, TTL expiry, revoke and cleanup,
NAT-PMP, a PCP ANNOUNCE/MAP subset, and UPnP IGD/SSDP frontend sources.
PEER and unsupported PCP options are refused. This does not claim full PCP,
IPv6 frontend support or interoperability with every discovery client.
Existing WireGuard, FRR and Xray foundations remain; Xray-core is pinned to
**26.3.27**. Xray Center beta.4 development is excluded.

## Architectures and verification scope

| Target | Release packaging | Native execution qualification |
|---|---|---|
| Linux AMD64 | Fresh cross-build, ELF/inventory/hash verification | Debian 13 AMD64 Router Core scope below |
| Linux ARM64 | Fresh cross-build, ELF/inventory/hash verification | No native ARM hardware qualification |
| Linux ARMv7 (`GOARCH=arm GOARM=7`) | Fresh cross-build, ELF/inventory/hash verification | No native ARM hardware qualification |

The prior qualification accepted by the release owner covers native Linux
AMD64 Router Core regressions, firewall/NAT packet tests, installer
qualification, real systemd startup, authenticated readiness, service restart
and VM reboot persistence at code baseline
`2e0c5afa089649a3238c543a82624075a9bc816e`.
Release preparation does not repeat those live tests or turn historical
component reports into new execution evidence. Application sources, systemd units, build scripts and lockfiles are unchanged
against that baseline. A focused installer correction distinguishes ELF32 from
ELF64 and checks header/section/segment bounds without integer wraparound;
malformed fixtures and genuine binaries for all three targets receive separate
read-only verification. No new live install or reboot is claimed by these checks.

No claim is made for in-place upgrades, physical power loss, every rollback
failure, arbitrary NICs/boards, all client interoperability or production
readiness. This is not a completed product security audit.

## Fresh installation

Choose exactly one architecture archive and the accompanying `SHA256SUMS`:

- `duckos-v0.1.0-beta.3-linux-amd64.tar.gz`
- `duckos-v0.1.0-beta.3-linux-arm64.tar.gz`
- `duckos-v0.1.0-beta.3-linux-armv7.tar.gz`

Verify `sha256sum --check --ignore-missing SHA256SUMS` and require an `OK`
line for the selected archive. Extract it into a new directory; each archive
contains `duckos-v0.1.0-beta.3/`. Run the bundled installer's
`--verify-artifact`, then use root-owned staging and `--check` before a fresh
installation. The installer checks the closed inventory, hashes and ELF
architecture, including the required nft launcher binary and unit.
Follow [installation](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.3/docs/install.md).

The artifact `VERSION` and `BUILD-INFO.json` identify `v0.1.0-beta.3` and the
exact final source commit, with `dirty=false`. API and Agent embed that version
and SHA; helper identities are bound by the complete per-file manifest, following
the existing [identity contract](release-identity.md). The archive includes seven
DuckOS service binaries and the Web UI; the standalone `duck` CLI remains
source-built. Use SSH forwarding to the loopback API for enrollment.

## Upgrade restrictions and recovery

No beta.1/beta.2 → beta.3 in-place upgrade has been qualified. These archives
are installer payloads, not signed `.duck` update packages; no public OTA
channel is supplied. Do not overwrite installed services manually or remove
state to bypass fresh-install preflight.

Keep an out-of-band console and preserve the management interface. Inspect
the installer status and bounded systemd journal on failure. Do not assume
API liveness proves authenticated readiness or network health. Pre-networkd
recovery, the Safe Apply journal and Active + Previous update foundations
serve different failure paths; reboot alone is not a guarantee of recovery.
See [recovery](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.3/docs/user/recovery.md)
and [limitations](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.3/docs/beta-limitations.md).

## Source hygiene and known beta restrictions

The release snapshot excludes laboratory `.runtime-data` reports. Their
removal does **not** erase historical Git objects, including previously
public laboratory topology. No active credential compromise is established
by that historical-address finding. The owner authorized publication with
existing history preserved; no history rewrite is performed. Do not attach laboratory evidence,
credentials, backups or private runtime state to the release.

There is no automatic LAN TLS onboarding or mDNS discovery, no established
physical Wi-Fi/modem matrix, and no qualification of every page in the UI
catalog. API reachability, observed state and verified health remain distinct.
Source is GPL-3.0-only; bundled Xray licensing is included. Report vulnerabilities
privately under [SECURITY.md](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.3/SECURITY.md).

---

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
Live H10 BGP advertise/withdraw/restart passed on AMD64 at the final qualified
runtime source. FRR is an optional Debian package: the installer attempts to
provision it and reports routing protocols unavailable if provisioning fails.

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
- Other FRR protocols, physical board/chipset variance and power-loss behavior
  are not established by the final VM qualification.
- The archive does not install the standalone `duck` CLI; see [CLI/API](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.2/docs/cli-api.md).
- No public OTA delivery channel, automatic LAN TLS onboarding or mDNS discovery.

## Verification and source identity

The full live qualification used product commit
`cdaae71f276a055b1425d8a603931ace33f57906` and build
`1.0.0-dev.cdaae71f276a`. This supersedes the earlier `90be1e0` evidence.
See [the current qualification summary](https://github.com/CrazyDuckLab/DuckOS/blob/v0.1.0-beta.2/docs/qualification-beta2.md). Documentation/report-only follow-ups do not alter the
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
