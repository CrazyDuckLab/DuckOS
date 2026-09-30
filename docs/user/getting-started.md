# Getting started

A first-time administrator's path from a fresh release archive to a
running DuckOS V1 beta dashboard.

DuckOS V1 beta (`v0.1.0-beta.1`) is **not** a production router. It
is a beta of an observability and configuration platform. Install on
an isolated test network — never on a production gateway. Read
[`docs/beta-limitations.md`](../beta-limitations.md) before you
begin.

## 1. What DuckOS is

DuckOS is a local-first Linux network operating platform: a web
dashboard above supported Debian that observes network and system state
on routers, gateways, VPS / edge nodes and self-hosted network
appliances. The first public beta ships Easy Mode, Full Mode, the
Owner enrollment, Safe Apply, the Doctor diagnostics and the
foundational subsystem that does not yet activate every catalog feature.

The management UI and Agent bind only `127.0.0.1:8080` on the
device. There is **no** mDNS, link-local HTTPS, ACME client,
self-signed certificate, or automatic onboarding URL on a fresh
install.

## 2. Supported target for v0.1.0-beta.1

| Target | Status |
|---|---|
| Linux **amd64** on **Debian 13** (trixie) | **Qualified.** |
| Linux arm64 on Debian 13 | Reference-supported (cross-build; no native hardware qualification yet). |
| Linux armv7 on Debian 13 | Reference-supported (cross-build; no native qualification yet). |
| Other distributions, other architectures, non-Debian | Not supported by this beta. The installer refuses them. |

## 3. Minimum practical requirements

- **Hardware**: any amd64 machine or VM that supports Debian 13.
  A real NIC for the management link (or a VM virtual NIC bridged
  to a reachable network).
- **Host OS**: Debian 13 (trixie), fresh install, systemd-managed.
  The first install must be on a clean host — the installer refuses
  any pre-existing DuckOS installation.
- **Network**: the management link must be reachable from an
  administrator host you control via SSH.
- **An isolated test network**: never the same physical network as a
  production router.
- **An out-of-band recovery path**: a serial console, a second
  management interface, or a VM console. Safe Apply uses a
  management anti-lockout guard, but the second path is your safety
  net during the first install on new hardware.
- **An administrator workstation** with a modern browser (current
  Chromium or Firefox) and `ssh` capable of local port forwarding.
  Older browsers are not supported.

## 4. Download the release archive

Download the two files from the public release:

- `duckos-v0.1.0-beta.1-linux-amd64.tar.gz`
- `SHA256SUMS`

## 5. Verify SHA256

On the administrator workstation:

```bash
sha256sum -c SHA256SUMS
```

Expected:

```
duckos-v0.1.0-beta.1-linux-amd64.tar.gz: OK
```

If the verification reports a mismatch, do not proceed. Re-download
and try again.

## 6. Extract archive

```bash
tar -xzf duckos-v0.1.0-beta.1-linux-amd64.tar.gz
cd duckos-v0.1.0-beta.1
ls
```

The archive contains:

- `BUILD-INFO.json` — exact build metadata (commit, target, dirty
  flag, Go and Node toolchain versions, source date epoch).
- `VERSION` — qualified internal build identifier
  `1.0.0-dev.4d9b5e661408`.
- `SHA256SUMS` — checksums of every file in this directory; the
  installer verifies the artifact against them at install time.
- `bin/{duck-api,duck-agent,duck-updater,duck-restore}` — Linux
  amd64 ELF executables.
- `install.sh` — the supported installer.
- `systemd/` — six systemd unit files.
- `web/` — the bundled UI assets.

## 7. Run installer

```bash
sudo bash install.sh --artifact "$(pwd -P)"
```

The installer:

- Verifies the artifact shape and the internal `SHA256SUMS`.
- Verifies `VERSION` matches the build metadata
  (`^1\.0\.0-dev\.[0-9a-f]{12}(\.dirty)?$`).
- Verifies the host is Debian 13 amd64 with systemd.
- Refuses any pre-existing DuckOS installation.
- Stages the release into `/opt/duckos/releases/<id>/`, makes
  `/opt/duckos/current` point at it, installs systemd units, and
  enrolls the release into the updater's Active + Previous store.

Read-only preflight checks are also available:

```bash
sudo bash install.sh --check --artifact "$(pwd -P)"
sudo bash install.sh --verify-artifact --artifact "$(pwd -P)"
```

## 8. Reboot if required

If the installer prints a message that a reboot is required (for
example because it placed a new boot-time unit), reboot:

```bash
sudo reboot
```

For an unmodified fresh Debian 13 amd64 install, a reboot is
**not** required by the supported install path. The systemd units
are activated as part of the install.

## 9. Connect by SSH tunnel

From the administrator workstation:

```bash
ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
```

Use **local port 8080** — the API validates the management Host
header and rejects other ports. The forwarded connection is HTTP
over loopback; DuckOS does not terminate TLS, and the V1 beta does
not ship a self-signed certificate, ACME client, or browser-trusted
onboarding flow.

## 10. Open the dashboard

In the same browser, with the SSH tunnel still open:

```
http://127.0.0.1:8080
```

There is no TLS warning to accept because the connection is plain
HTTP over loopback.

The first screen is **Owner enrollment**.

## 11. Owner enrollment / first login

DuckOS has exactly one Owner per installation. The Owner is the only
account that can do everything, including first-time configuration
and the bootstrap authentication.

1. The UI prompts for a **username**, a **display name**, and a
   **password**.
2. Choose a strong password. There is no minimum enforced by the
   server; the UI recommends 14 characters or more.
3. Submit. The browser is redirected to the dashboard.

After enrollment, the browser holds a session token. The token lives
in `sessionStorage` only and expires automatically. Closing the
browser ends the session.

## 12. First things to inspect

The default landing page after enrollment is **Easy Mode**
(`/easy/home`):

- The headline surfaces the current health conclusion. In this
  beta the headline may say **Status check incomplete** on a
  healthy-looking system — this is intentional and documented in
  [`docs/beta-limitations.md`](../beta-limitations.md).
- The **System** tile surfaces "Some things need attention" or the
  honest incomplete verdict.
- The **Internet** tile links into Easy Mode's Internet wizard.
- **Wi-Fi**, **Devices**, **VPN**, and **About this device** tiles
  cover the day-to-day state.

If you want the dense by-domain catalog, click **Full** in the top
bar. You can switch back at any time with **Easy**.

Press **`⌘K`** / **`Ctrl+K`** to open the one global search.
Press **`?`** on any page to open the contextual Inspector.

## 13. Safe Apply — read this before any non-trivial change

Every change that touches the network or system goes through
[Safe Apply](safe-apply.md). A Safe Apply transaction has:

- a **durable journal** of the candidate change,
- an **independent confirmation watchdog** that gates the change on
  your explicit approval,
- an **automatic rollback** path if health fails before, during, or
  after the change.

In the V1 beta the public catalog lists many subsystems that are
**not yet** wired to live network activation through the public UI.
The Safe Apply integration for those subsystems is gated behind
qualification; see [`beta-limitations.md`](../beta-limitations.md) for
the current list. Until then, configuration must happen by editing
the relevant Linux adapters directly, with Safe Apply reserved for
the narrow paths that have passed qualification.

Do not script against undocumented Safe Apply internals. The V1 beta
public surface is the UI / API; the implementation details are
unstable.

## 14. Where to go next

- [Easy Mode reference](easy-mode.md) — the dashboard in detail.
- [Full Mode reference](full-mode.md) — the by-domain catalog.
- [Safe Apply](safe-apply.md) — the network change lifecycle.
- [Doctor and diagnostics](doctor-and-diagnostics.md) — interpreting
  the warnings Easy Mode surfaces.
- [Updates and rollback](updates-and-rollback.md) — signed offline
  update flow.

## 15. Recovery / troubleshooting

- [Recovery basics](recovery.md) — pre-networkd boot recovery,
  Safe Apply rollback, Owner reset.
- [Troubleshooting](troubleshooting.md) — triage guide.
- [`docs/security-model.md`](../security-model.md) — what DuckOS
  guarantees and what it doesn't.

If something looks wrong, prefer the recovery path over an
out-of-band reinstall. Safe Apply rollback is the only supported
way to recover from a bad candidate that already passed through the
candidate pipeline.