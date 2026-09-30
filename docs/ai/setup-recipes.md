# DuckOS Setup Recipes

Task-oriented recipes for the supported V1 beta capabilities. Each
recipe follows the same shape: **Goal**, **Before you start**,
**Where to go**, **Steps**, **Verify**, **Safety**, **Related
documentation**.

Where current documentation only describes how to *observe* a
subsystem (because the activation gate is not yet open), the recipe
honestly says so. Do not invent configuration steps for subsystems
the V1 beta only renders against observed state — see
[`../beta-limitations.md`](../beta-limitations.md).

Replace placeholder values such as `<duckos-ip>`, `<user>`, `<hostname>`,
`<address>` with the actual values for the user's environment. Never
paste real tokens, private keys, or production network details into
chat, screenshots, logs, or documentation.

---

## First installation

**Goal:** Install DuckOS V1 beta (`v0.1.0-beta.1`) on a fresh,
isolated Debian 13 amd64 host with systemd.

### Before you start

- Use a fresh Debian 13 (trixie) amd64 host. The installer refuses
  any pre-existing DuckOS installation.
- Confirm the host is **not** a production gateway. Install on an
  isolated test network.
- Have an out-of-band recovery path (serial console, second
  management interface, or VM console).
- Have an administrator workstation with `ssh` (capable of local
  port forwarding) and a modern Chromium or Firefox browser.
- Have the two release files: the tarball and the matching
  `SHA256SUMS`.

### Where to go

On the administrator workstation: nothing to open. On the Debian host:
the bash installer `install.sh` in the extracted archive.

### Steps

1. Verify the archive:
   ```bash
   sha256sum -c SHA256SUMS
   ```
2. Extract:
   ```bash
   tar -xzf duckos-v0.1.0-beta.1-linux-amd64.tar.gz
   cd duckos-v0.1.0-beta.1
   ```
3. On the Debian host, install:
   ```bash
   sudo bash install.sh --artifact "$(pwd -P)"
   ```
4. Optional preflight only:
   ```bash
   sudo bash install.sh --check --artifact "$(pwd -P)"
   sudo bash install.sh --verify-artifact --artifact "$(pwd -P)"
   ```

### Verify

- `systemctl status duck-agent.service duck-api.service` reports both
  units active.
- `curl --fail http://127.0.0.1:8080/api/v1/health` returns HTTP 200.
- Authenticated `/api/v1/ready` returns HTTP 200 with a build commit
  matching `f698de16b4a9…`.

### Safety

The installer only takes effect from public release artifacts that
match `SHA256SUMS`. Checksums authenticate integrity, not the
publisher. Review the source tree and trust the transfer channel.

### Related documentation

- [`../install.md`](../install.md) — qualified installer flow.
- [`../user/getting-started.md`](../user/getting-started.md) —
  full step-by-step, including the SSH tunnel and Owner enrollment.
- [`../beta-limitations.md`](../beta-limitations.md) — installer
  limitations and the bootstrap-token lifecycle.

---

## First Owner enrollment

**Goal:** Create the first Owner account on a freshly installed DuckOS
V1 beta.

### Before you start

- Have completed First installation.
- Have the bootstrap API token from `/etc/duckos/api-token` on the
  device (root-readable, owner root:duckos).
- Have an SSH session with port 8080 forwarded.

### Where to go

First browser open of `http://127.0.0.1:8080` over the SSH tunnel.

### Steps

1. From the administrator workstation, open the tunnel:
   ```bash
   ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
   ```
2. Open `http://127.0.0.1:8080` in the browser.
3. Paste the bootstrap API token into the first-run screen exactly
   once.
4. Choose a username and a display name.
5. Choose a strong password (the UI recommends ≥ 14 characters; the
   server does not enforce a minimum).
6. Submit. The browser is redirected to the Easy Mode dashboard.

### Verify

- Easy Mode lands at `/easy/home` and surfaces the headline.
- The browser holds the session credential in `sessionStorage`. Closing
  the browser (or that tab) clears that locally stored credential. The
  V1 beta does not implement guaranteed server-side revocation on
  browser close; idle sessions expire server-side after 30 minutes and
  can be revoked explicitly via `/api/v1/auth/logout` or
  `/api/v1/auth/revoke`.

### Safety

The bootstrap token can be used exactly once to create the first
Owner. After enrollment it cannot enroll another account and is not
used for API-to-Agent assertions. Never paste the token into a
screenshot, log, chat, or file you would not want published.

### Related documentation

- [`../user/getting-started.md`](../user/getting-started.md) — steps
  9–11.
- [`../security-model.md`](../security-model.md) — local identity,
  bootstrap-token, audit log.

---

## Access DuckOS remotely through the SSH tunnel

**Goal:** Open the dashboard at `http://127.0.0.1:8080` from a remote
workstation.

### Before you start

- DuckOS is installed and the management LAN is reachable from the
  administrator host.
- The browser is current Chromium or Firefox.
- Local port 8080 on the workstation is free.

### Where to go

Nothing — this is a workstation action that produces the address the
browser then visits.

### Steps

1. Forward local port 8080 to the device's loopback:
   ```bash
   ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
   ```
2. Open `http://127.0.0.1:8080` in the browser.

### Verify

- The first-run screen, login screen, or Easy Mode home page renders.

### Safety

- The management API validates the management Host header and rejects
  other ports. Use exactly local port 8080.
- The connection is plain HTTP over loopback. The V1 beta does not
  ship a self-signed certificate, ACME client, or browser-trusted
  onboarding flow.
- There is no automatic LAN exposure. The dashboard is reachable only
  through an authenticated tunnel from an administrator you control.

### Related documentation

- [`../user/getting-started.md`](../user/getting-started.md) — step 9.
- [`../tls.md`](../tls.md) — TLS / mTLS / ACME posture in V1 beta.

---

## Inspect WAN status

**Goal:** See the current DuckOS uplink state and the configured
addresses, gateway, and DNS.

### Before you start

None — observation only.

### Where to go

Full Mode → **Network** → **WAN** connections.

### Steps

1. Click **Full** in the top bar.
2. Open **Network** in the main navigation.
3. Open **WAN** → **WAN connections**.

### Verify

The page lists the observed uplinks, their state, address family,
gateway, and DNS. The V1 beta renders observed state; live path
quality and activation are gated — see
[`../beta-limitations.md`](../beta-limitations.md).

### Safety

Read-only. No network change.

### Related documentation

- [`../user/wan.md`](../user/wan.md)
- [`../user/system.md`](../user/system.md)

---

## Inspect an interface

**Goal:** Inspect every network interface observed on the device.

### Before you start

None — observation only.

### Where to go

Full Mode → **Network** → **Interfaces**.

### Steps

1. Click **Full** in the top bar.
2. Open **Network** in the main navigation.
3. Open **Interfaces**.

### Verify

The table shows each interface's Name, State, MAC address, addresses,
and Driver / bus. The management interface is marked explicitly. The
V1 beta does not yet show per-interface traffic counters — see
[`../beta-limitations.md`](../beta-limitations.md).

### Safety

Read-only. The V1 beta does not yet edit interface addresses through
the public UI.

### Related documentation

- [`../user/interfaces.md`](../user/interfaces.md)

---

## Find eth0 using global search

**Goal:** Locate a specific interface or object quickly.

### Before you start

None.

### Where to go

The single global search opened with **`⌘K`** (macOS) or
**`Ctrl+K`** (elsewhere). There is exactly one.

### Steps

1. Press **`⌘K`** / **`Ctrl+K`** on any page.
2. Type the identifier (for example `eth0`, `192.0.2.10`, an
   interface name, a hostname, a rule ID, or a page name).
3. Pick the relevant result.

### Verify

The relevant page opens. Selecting an object result opens its
context page; selecting a page result navigates to it; selecting a
help result opens the Inspector pane.

### Safety

Read-only. The search palette is the only canonical entry point.
DuckOS does not introduce a second search bar.

### Related documentation

- [`../user/search-and-inspector.md`](../user/search-and-inspector.md)

---

## Inspect LAN / VLAN configuration

**Goal:** Inspect the DuckOS LAN / VLAN model and the addressing of
each LAN.

### Before you start

None — observation only.

### Where to go

Full Mode → **Network** → **Networks / LAN / VLAN**.

### Steps

1. Click **Full** in the top bar.
2. Open **Network** → **Networks / LAN / VLAN**.

### Verify

The page lists each LAN / VLAN with its addressing and the parent
interface. Identify the management interface from the Interfaces
page first. The V1 beta observes LAN / VLAN state but does not yet
edit DHCP scopes or per-LAN DNS resolvers through the public UI.

### Safety

Read-only. Live LAN / VLAN editing through the public UI is gated
behind qualification.

### Related documentation

- [`../user/lan-vlan.md`](../user/lan-vlan.md)
- [`../network-understanding.md`](../network-understanding.md)
- [`../route-compilation.md`](../route-compilation.md)

---

## Inspect routing state

**Goal:** Inspect the observed kernel routing state plus the
canonical desired-state model.

### Before you start

None — observation only.

### Where to go

Full Mode → **Network** → **Routing**.

### Steps

1. Click **Full** in the top bar.
2. Open **Network** → **Routing** → **Routes**.

### Verify

The page shows the kernel's main table plus the private routing
tables (`10000`, `10001`, …). The V1 beta validates a candidate
against the canonical model but does not yet write private routes
through the public UI.

### Safety

Read-only.

### Related documentation

- [`../user/routing.md`](../user/routing.md)
- [`../route-compilation.md`](../route-compilation.md)

---

## Inspect firewall state

**Goal:** Inspect the observed nftables state owned by DuckOS and the
Safe Apply guard chain.

### Before you start

None — observation only.

### Where to go

Full Mode → **Security** → **Firewall**.

### Steps

1. Click **Full** in the top bar.
2. Open **Security** → **Firewall**.

### Verify

The page surfaces the `inet duckos` table and the `input`,
`forward`, `output`, and `duckos_apply_guard` chains. While a Safe
Apply is in flight, the guard chain is installed and is visible
to anyone with `nft list chain inet duckos duckos_apply_guard`.

### Safety

Read-only. Live firewall rule editing through the public UI is
gated.

### Related documentation

- [`../user/firewall.md`](../user/firewall.md)
- [`../user/safe-apply.md`](../user/safe-apply.md)

---

## Inspect DNS / DHCP state

**Goal:** Inspect the observed DNS resolver configuration and DHCP
leases.

### Before you start

None — observation only.

### Where to go

Full Mode → **DNS and DHCP** → **DNS** and **DHCP**.

### Steps

1. Click **Full** in the top bar.
2. Open **DNS and DHCP** → **DNS** (resolver configuration, zones,
   policies).
3. Open **DNS and DHCP** → **DHCP** (scopes, leases, reservations).

### Verify

The DNS service is `dnsmasq` under a dedicated non-login identity
(`duck-dnsmasq`). It writes the readiness marker
`/run/duckos-agent/network-ready` only after it has validated its
configuration. The V1 beta renders observed state but does not yet
edit DHCP scopes through the public UI.

### Safety

Read-only.

### Related documentation

- [`../user/dns-dhcp.md`](../user/dns-dhcp.md)

---

## Inspect WireGuard / tunnel state

**Goal:** Inspect observed WireGuard servers, peers, endpoints, and
allowed IPs.

### Before you start

None — observation only.

### Where to go

Full Mode → **VPN / Tunnels** → **WireGuard**.

### Steps

1. Click **Full** in the top bar.
2. Open **VPN / Tunnels** → **WireGuard** (servers).
3. Open **WireGuard** → **Peers / Exports** for peers.

### Verify

The page surfaces the server's address range, listen port, and DNS,
plus each peer's public key, endpoint, and Allowed IPs. Private
keys live in the encrypted vault and are never exported in clear.
Public keys and pre-shared keys are exported through the
secret-export flow with audit logging.

### Safety

Read-only. Live WireGuard activation through Safe Apply is a
qualified subsystem; check [`../beta-limitations.md`](../beta-limitations.md)
before relying on it.

### Related documentation

- [`../user/wireguard.md`](../user/wireguard.md)
- [`../user/xray.md`](../user/xray.md)
- [`../security-model.md`](../security-model.md) — secret export
  posture.

---

## Inspect system / health

**Goal:** Inspect the centralized health conclusion for the device.

### Before you start

None.

### Where to go

Full Mode → **System** → **System Overview**.

Or: Easy Mode → **Home** for the headline-only form.

### Steps

1. Click **Full** in the top bar.
2. Open **System** → **System Overview**.

### Verify

The headline is one of:

- **Needs attention** — Doctor or the overview has raised a finding.
  Open Doctor.
- **Status incomplete** — the public health endpoint responded, but
  the production contracts do not yet provide enough evidence to
  claim verified health. This is the default state for V1 beta on a
  healthy-looking system.
- **Unavailable** — the public health endpoint did not respond.

The page never claims **Healthy** or **Observation fresh** without
real evidence. The freshness pill is independent of Doctor findings.

### Safety

Read-only.

### Related documentation

- [`../user/system.md`](../user/system.md)
- [`../user/easy-mode.md`](../user/easy-mode.md)

---

## Run Doctor / diagnostics

**Goal:** Surface deterministic health findings.

### Before you start

None.

### Where to go

Full Mode → **Updates** → **Doctor**. Or: Easy Mode → **Care** →
**Diagnosis**.

### Steps

1. Click **Full** in the top bar.
2. Open **Updates** → **Doctor**.

### Verify

Doctor renders one of four states:

- **Loading** — Doctor has not yet responded. The findings count is
  not shown.
- **Doctor unavailable** — the request errored. Use **Retry**.
- **No current Doctor findings** — Doctor responded with zero
  findings.
- **Findings** — one or more finding cards, each with a component,
  code, message, and Open Doctor action.

Doctor is a deterministic checker; it is **not** a generative
remediation assistant. A finding is a description of observed state,
not a proposed change.

### Safety

Read-only.

### Related documentation

- [`../user/doctor-and-diagnostics.md`](../user/doctor-and-diagnostics.md)

---

## Make a network change safely with Safe Apply

**Goal:** Apply a network-affecting candidate safely, including the
confirmation watchdog.

### Before you start

- You have an out-of-band recovery path (serial, console, or second
  management interface).
- You have reviewed the candidate diff.
- The management anchor is in the candidate's "kept" set.

### Where to go

Full Mode → **System** → **Candidate and Safe Apply** for the
review and confirm action. Edit the candidate on the relevant
subsystem page (for example, **Interfaces** or **Routing**).

### Steps

1. **Inspect current state** on the relevant subsystem page.
2. **Prepare the candidate** by editing the desired-state form.
3. **Open the candidate review** at **System** → **Candidate and Safe
   Apply**. Review the Diff.
4. **Click Apply.** Safe Apply installs the forwarding guard, applies
   the scoped change, and waits for confirmation.
5. **Verify connectivity / runtime state** (`/api/v1/ready`, Doctor,
   affected subsystem page).
6. **Click Confirm** to finalize. Auto-confirm does not exist.
7. If anything looks wrong, **click Rollback** in the UI. If rollback
   fails, the pre-networkd boot recovery unit reconstructs host state
   on the next reboot **for the qualified network-renderer path**.
   Full real-reboot / power-loss durability qualification is still
   pending for v0.1.0-beta.1 — see
   [`../beta-limitations.md`](../beta-limitations.md).

### Verify

- The candidate is recorded as `Confirmed` on the **Candidate and Safe
  Apply** page.
- The watchdog reaches steady state.
- The relevant subsystem page reflects the active state.

### Safety

- A Safe Apply candidate that would orphan the management session is
  rejected at validate, before the guard is installed. There is no
  override.
- The rollback watchdog is independent of the Engine. If the Engine
  dies mid-Apply, the Agent reports the outcome to the journal; on
  the next boot the boot recovery unit reconstructs host state **for
  the qualified network-renderer path**. Full real-reboot / power-loss
  durability qualification is still pending for v0.1.0-beta.1.

### Related documentation

- [`../user/safe-apply.md`](../user/safe-apply.md) — lifecycle, anti-lockout,
  guard chain.
- [`../user/recovery.md`](../user/recovery.md) — when rollback fails.
- [`../architecture.md`](../architecture.md) — Engine + Agent + journal.

---

## Recover when an unconfirmed network change is rolled back

**Goal:** Recover cleanly when a Safe Apply rolls back automatically.

### Before you start

None, but be ready to use the out-of-band recovery path if the dashboard
is unreachable.

### Where to go

Full Mode → **System** → **Candidate and Safe Apply** for the journal.
If the dashboard is unreachable, use the out-of-band path: serial
console, second management interface, or VM console.

### Steps

1. Open **Candidate and Safe Apply**. The rolled-back transaction is
   visible in the journal with its snapshot, its diff, and the reason
   for rollback.
2. Diagnose: the failure reason is in the journal and in the affected
   subsystem page. Common causes are scoped reload failure, lost
   acknowledgement, and watchdog-detected drift.
3. Either edit the candidate and try a new Safe Apply, or roll back
   to a known-good revision from **System** → **Configuration
   History**.
4. If rollback itself fails, reboot — the
   `duck-network-recover.service` one-shot unit reconstructs host
   state from the durable journal before networkd comes up **for the
   qualified network-renderer path**. Full real-reboot / power-loss
   durability qualification is still pending for v0.1.0-beta.1 — see
   [`../beta-limitations.md`](../beta-limitations.md).

### Verify

The active configuration matches the snapshot taken at Apply start.
The journal reflects `RolledBack`.

### Safety

- Recovery does not roll back the durable journal. The journal is
  retained for inspection.
- Recovery does not delete Doctor findings.
- Recovery does not reset the audit log.
- Recovery does not delete the update Active + Previous pair.

### Related documentation

- [`../user/recovery.md`](../user/recovery.md) — pre-networkd boot
  recovery, Safe Apply rollback, update rollback, Owner reset.
- [`../user/safe-apply.md`](../user/safe-apply.md) — rollback semantics.
- [`../architecture.md`](../architecture.md) — Engine + Agent + journal.