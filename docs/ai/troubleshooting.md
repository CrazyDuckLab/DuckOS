# DuckOS Troubleshooting Guide for Assistants

This document is for AI assistants and automation agents helping
DuckOS users diagnose problems. It is decision-oriented, not a Linux
cookbook. Before guessing commands, collect the current DuckOS
state. Prefer the DuckOS UI, Doctor findings, and the audit log over
random Linux surgery.

When the canonical DuckOS documentation does not establish a fix,
say so. Do not invent Linux commands to compensate.

## Collect before changing

Before recommending any change, ask the user for:

- **Easy Mode headline** — the single one-line verdict.
- **System Overview headline** in Full Mode — denser view of the
  same state.
- **Doctor findings** — open Doctor and capture the component,
  code, and message of each finding card.
- **Exact build identifier** — Easy Mode footer or
  `/api/v1/overview` payload (`build.commit`).
- **Architecture** — `amd64`, `arm64`, or `armv7`.
- **Debian release** on the device — for V1 beta this is Debian 13.
- **Sanitized log excerpt** — only when the canonical guide asks
  for it. Never include tokens, private keys, subscription URLs, or
  production network details.

If the user cannot supply these, the next step is "consult the
DuckOS documentation first", not "run a Linux command".

## Decision tree: UI cannot be reached

```
UI unreachable at http://127.0.0.1:8080
│
├─ Is the SSH tunnel open on the workstation?
│   └─ No → Forward local 8080: ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
│
├─ Does the browser say the connection was refused?
│   ├─ Yes, on loopback → Check duck-api.service on the device
│   │   └─ systemctl status duck-api.service
│   │       ├─ inactive → sudo systemctl start duck-api.service
│   │       └─ failed → sudo journalctl -u duck-api.service -n 100 --no-pager
│   └─ Yes, on a non-loopback host → The API listener is 127.0.0.1 only.
│       Use the SSH tunnel. LAN / HTTPS / mDNS onboarding are NOT in V1 beta.
│
├─ Does the browser say the host rejected the connection?
│   └─ The API validates the Host header. Use exactly port 8080.
│       Do not change the local port on the SSH forward.
│
└─ Does the browser say Status unavailable?
    └─ See Easy Mode headline = "Status unavailable" below.
```

**Related documentation:**

- [`../user/getting-started.md`](../user/getting-started.md) — steps
  9–11.
- [`../user/troubleshooting.md`](../user/troubleshooting.md) — the
  canonical triage guide.
- [`../tls.md`](../tls.md) — TLS posture.

## Decision tree: Easy Mode headline

```
Easy Mode headline
│
├─ "Status check incomplete"
│   └─ This is the **default** state for V1 beta on a healthy-looking
│       system. The public health endpoint responded, but the production
│       contracts do not yet provide enough evidence to claim verified
│       health. Not a bug.
│
├─ "Status unavailable"
│   └─ The public health endpoint did not respond. The browser can talk
│       to the page; the backend cannot.
│       ├─ systemctl status duck-api.service
│       ├─ Management LAN can route to the API listener
│       └─ Session token is valid (log in again)
│
├─ "Some things need attention"
│   └─ Doctor or the overview raised a finding. Open Doctor for the cards.
│       Each card has a component, code, message, and recovery action.
│
└─ "Everything is working" while Easy Home says "Status check incomplete"
    └─ Structurally impossible in V1 beta. If you see it, file a bug;
        the centralized health derivation in web/src/state/health.ts
        cannot produce a "good" verdict without verified-health evidence.
```

## Decision tree: Interface missing or down

```
Interface not appearing, or State = down / notpresent / unknown
│
├─ Check Full Mode → Network → Interfaces.
│
├─ State = up → the link is up; the issue is elsewhere.
│
├─ State = down → link layer down (cable, port, NIC).
│   └─ Check physical NIC, cable, switch port.
│
├─ State = notpresent → driver did not bind the device.
│   └─ Check lspci / lsusb / ip link; verify driver against
│       docs/hardware-support.md.
│
└─ State = unknown → no recent observation.
    └─ Wait for the next observation or open Doctor.
```

**Related documentation:**

- [`../user/interfaces.md`](../user/interfaces.md)
- [`../hardware-support.md`](../hardware-support.md)
- [`../beta-limitations.md`](../beta-limitations.md) — Wi-Fi chipset
  variance; modems / LTE not supported.

## Decision tree: Network change did not persist

```
A network change did not persist after I clicked Apply
│
├─ Was the candidate on Candidate and Safe Apply Confirmed?
│   ├─ No → Apply did not complete, or the transaction rolled back.
│   │       Open Candidate and Safe Apply and inspect the journal.
│   └─ Yes → the active state reflects the change; verify by reloading
│       the affected subsystem page.
│
├─ Was the confirmation window reached?
│   └─ If not, DuckOS may have auto-rolled-back the candidate when the
│       window closed. The journal will show RolledBack.
│
└─ Did you change something outside DuckOS on the device?
    └─ In V1 beta the public UI is the supported surface. Direct
        edits to /etc/network/interfaces, networkd, nftables,
        dnsmasq, frr, WireGuard, or Xray bypass Safe Apply and the
        journal. Re-apply through the UI to restore the supported
        state.
```

**Related documentation:**

- [`../user/safe-apply.md`](../user/safe-apply.md) — lifecycle, rollback.
- [`../user/recovery.md`](../user/recovery.md) — recovery primitives.

## Decision tree: Candidate rolled back

```
Candidate went to RolledBack
│
├─ Open Candidate and Safe Apply. The journal entry shows the snapshot,
│   the diff, the rollback reason, and the watchdog verdict.
│
├─ Common reasons:
│   ├─ Scoped reload failed (networkd or service unit failed to apply).
│   ├─ Agent lost acknowledgement (connectivity blip during Apply).
│   └─ Watchdog detected drift between expected and observed state.
│
├─ Do NOT blindly re-apply. Diagnose the cause first.
│
└─ If rollback itself failed:
    └─ Reboot — the duck-network-recover.service one-shot unit
        reconstructs host state from the durable journal before
        networkd comes up.
```

**Related documentation:**

- [`../user/safe-apply.md`](../user/safe-apply.md)
- [`../user/recovery.md`](../user/recovery.md)

## Decision tree: WireGuard unavailable

```
WireGuard page is empty / showing errors / peers cannot connect
│
├─ Page renders observed state only → live activation is gated.
│   └─ Check docs/beta-limitations.md for current qualification status.
│
├─ Peer cannot connect:
│   ├─ Peer public key matches DuckOS's on-file public key?
│   ├─ Endpoint address and port are correct?
│   ├─ Allowed IPs in the peer config match DuckOS's Allowed IPs?
│   └─ Firewall allows the WireGuard UDP port?
│
└─ WireGuard is a qualified subsystem in V1 beta; some activation
    paths remain gated. Do not invent CLI / wg-quick workflows.
```

**Related documentation:**

- [`../user/wireguard.md`](../user/wireguard.md)
- [`../user/troubleshooting.md`](../user/troubleshooting.md) —
  WireGuard peers cannot connect.
- [`../beta-limitations.md`](../beta-limitations.md)

## Decision tree: Update issue

```
Update did not apply / the device does not surface new releases
│
├─ Auto-update channel is NOT available in V1 beta.
│   └─ There is no public update server. Updates are signed .duck
│       payloads applied manually through the documented flow.
│
├─ Rollback triggered automatically:
│   └─ The supervisor rolled back to Previous because the new Active
│       did not reach steady state within the configured window.
│       Inspect the journal on the device.
│
├─ Need to roll back manually:
│   └─ Full Mode → Updates → Update system, OR the CLI:
│       sudo duck upgrade rollback (logged in the audit log).
│
└─ Need to roll back because the device is non-functional:
    └─ Reboot — boot recovery reconstructs Active + Previous.
```

**Related documentation:**

- [`../user/updates-and-rollback.md`](../user/updates-and-rollback.md)
- [`../update-system.md`](../update-system.md)
- [`../updater-supervisor.md`](../updater-supervisor.md)

## Decision tree: Lost Owner account

```
Owner account lost / cannot log in
│
├─ Is there another admin / Owner on the device?
│   └─ Have them reset or rotate the lost account.
│
├─ Otherwise, use the CLI recovery:
│   ├─ sudo systemctl stop duck-api
│   ├─ sudo duck owner reset
│   └─ sudo systemctl start duck-api
│   The reset deletes every Owner, preserves other users, and writes
│   a timestamped backup under /var/backups/duckos-owner before
│   deletion.
│
├─ Re-enroll a new Owner through the dashboard with the bootstrap
│   API token from /etc/duckos/api-token.
│
└─ Do NOT delete the identity directory manually — that loses
    identity / audit evidence and reopens enrollment without the
    bootstrap-token check.
```

**Related documentation:**

- [`../user/recovery.md`](../user/recovery.md) — Owner reset.
- [`../security-model.md`](../security-model.md) — local identity.

## When to escalate through the public reporting channel

If the canonical documentation does not resolve the symptom, escalate
through the channel in [`../../SECURITY.md`](../../SECURITY.md). Include:

- The exact DuckOS commit hash (the runtime build identifier, e.g.
  `1.0.0-dev.4d9b5e661408`).
- The architecture (`amd64`, `arm64`, or `armv7`).
- The Debian / kernel version on the device.
- The Doctor findings, if any.
- A sanitized log excerpt.

Do not include `connect-redacted` private keys, subscription URLs,
or production network details.

## What NOT to suggest as a "fix"

- Do not advise editing `/etc/network/interfaces`, dnsmasq, frr,
  nftables, networkd, or WireGuard directly to "fix" a UI / Safe
  Apply problem. Those edits bypass the journal and the supported
  contract.
- Do not advise disabling the forwarding guard
  (`duckos_apply_guard`) to "speed up" an Apply.
- Do not invent a public update channel or a LAN / mDNS onboarding
  flow. Neither ships in V1 beta.
- Do not advise running random `wg`, `nft`, `ip`, or `systemctl`
  commands to compensate for a UI limitation. If the canonical doc
  does not say to run it, do not recommend it.
- Do not advise destructive Owner resets to "recover" a forgotten
  password without first trying documented per-account recovery
  paths.