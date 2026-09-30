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