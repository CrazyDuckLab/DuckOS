# DuckOS security policy

This page covers the supported versions of DuckOS, the current
implementation-level security model at a high level, and how to
report a vulnerability. The detailed trust boundaries, threat model,
and engineering controls are in [`docs/security-model.md`](docs/security-model.md).

## Supported versions

| Version | Supported |
|---|---|
| `v0.1.0-beta.2` (public beta) | **Best-effort community support** during the public beta. Security-relevant fixes land in the next beta or stable. See [`docs/beta-limitations.md`](docs/beta-limitations.md) for what is and is not wired to live data in this beta. |
| Source code at any commit | Not a supported release. Use the qualified artifact only. |
| Internal / unlabelled builds | Not supported. |

The qualifier "best-effort community support" means there is **no
service-level agreement, no formal security certification, and no
public security response timeline** for the public beta. A
vulnerability found today will be addressed in the next planned
release. Use the installation and recovery guides; never deploy
this beta as a production gateway.

## Current security model (summary)

The detailed model is in
[`docs/security-model.md`](docs/security-model.md). This section is
the high-level, public-facing summary.

### Management binding

- The API / Engine (`duck-api`) binds **only** `127.0.0.1:8080` on the
  device. No LAN / routable listener is opened by the installer.
- The privileged Agent (`duck-agent`) does **not** expose a routable
  listener. It communicates with the Engine over a Unix-domain
  socket at `/run/duckos-agent/agent.sock`.
- A V1 beta administrator reaches the dashboard by opening an SSH
  local-forward (`ssh -L 8080:127.0.0.1:8080 <user>@<duckos-ip>`)
  and visiting `http://127.0.0.1:8080` in the browser. There is no
  direct LAN HTTPS / mDNS / DNS-SD onboarding in this beta.

### Identity and authentication

- DuckOS has exactly **one Owner** per installation. The Owner is the
  only role that can perform the first-time configuration and the
  bootstrap authentication.
- The first Owner is enrolled through the first-run screen, once, by
  presenting the bootstrap API token from `/etc/duckos/api-token`.
  After enrollment the bootstrap token cannot enroll another Owner.
- The session token is held in `sessionStorage` and expires
  automatically. Closing the browser ends the session.
- There is **no** built-in MFA, passkey, or WebAuthn support in this
  beta. The Owner password is the only factor.
- Owner reset exists in the source-built CLI, which is not installed by the
  beta.2 archive. Follow [CLI availability](docs/cli-api.md) and the recovery
  guide; stop the API before offline recovery. It is logged in the audit log.

### Privilege separation

The four-tier boundary is enforced by the source code and the
unit systemd topology:

```
Browser UI (React / TypeScript)
   │   authenticated bearer
   ▼
Public API / Engine  (cmd/duck-api, internal/api, internal/engine)
   │   authenticated, versioned, role-checked
   ▼
Privileged Agent  (cmd/duck-agent, internal/agent)
   │   bounded, fixed, scoped Linux mutations
   ▼
Linux kernel + systemd + networkd + nftables + dnsmasq + frr + WireGuard + Xray
```

- The browser **must not** call the privileged Agent directly.
- The Agent **must not** expose arbitrary shell or arbitrary
  command execution. Its semantic RPC exposes bounded mutation
  operations only.
- The Linux adapters are the only path that mutates `/etc`, kernel
  state, network configuration, or service lifecycle.

### Safe Apply and rollback

Every change that touches the network or system goes through
**Safe Apply**: a durable journal, an independent confirmation
watchdog, and an automatic rollback path if health fails before,
during, or after the change. Auto-confirm does not exist. See
[`docs/user/safe-apply.md`](docs/user/safe-apply.md) and
[`docs/architecture.md`](docs/architecture.md).

A Safe Apply candidate that would orphan the management session is
rejected at validate, before the guard is installed. There is no
override.

### Update trust model

- `.duck` update payloads are signed with an ed25519 keypair.
  Public keys are installed at `/etc/duckos/update-keys`. The
  updater refuses a payload whose signature does not verify.
- The update supervisor keeps exactly one **Active** and one
  **Previous** release on disk. The supervisor rolls back
  automatically if the new Active does not reach steady state within
  the configured window.
- See [`docs/update-system.md`](docs/update-system.md) and
  [`docs/updater-supervisor.md`](docs/updater-supervisor.md).

### Recovery model

- Pre-networkd boot recovery unit (`duck-network-recover.service`)
  reconstructs host routes / rules from the durable journal before
  networkd comes up.
- Safe Apply rollback restores the snapshot captured at Apply start.
- Update rollback reverts to the previous `.duck` payload.
- The CLI exposes `sudo duck owner reset` for lost-Owner recovery.
  This is logged.

See [`docs/user/recovery.md`](docs/user/recovery.md).

## Reporting a vulnerability

Please report suspected vulnerabilities **privately**, not in a
public issue or discussion.

The preferred private reporting route, once the public repository
is live, is **GitHub Private Vulnerability Reporting**:

1. On the public DuckOS repository, open the **Security** tab.
2. Click **Report a vulnerability** to open a private advisory
   draft. The maintainer team is notified privately; the issue is
   never visible to the public until the maintainer chooses to
   disclose it.

If that route is not yet available to you, or you need a non-GitHub
route, contact the `CrazyDuckLab` repository owner through an
existing private channel and ask for a private reporting path.

Do **not** include the following in any public issue, discussion,
screenshot, or attachment:

- Exploit details, exploit payloads, or proof-of-concept code.
- Credentials, passwords, API tokens, session tokens, the bootstrap
  API token.
- Private keys (WireGuard, frr, RSA, ed25519), subscription URLs,
  client QR payloads.
- Production network addresses, customer data, packet captures, or
  internal hostnames.

If you have collected evidence and cannot reach a private channel,
**stop**. Do not publish. Open an issue requesting a private
contact channel; the maintainer team will reply with one.

## Acknowledgements

The DuckOS maintainer team is grateful to the security researchers
and administrators who report issues privately. We acknowledge
reports in the next planned release notes unless the reporter opts out.

## Limitations of this security policy

- The V1 beta is a beta. It is not a certified product. It has **no**
  formal security review, **no** CVE assignment process, **no**
  public security-response SLA, **no** bug-bounty program.
- The risk model assumes the host Debian system is otherwise
  up-to-date, that the SSH path to the device is authentic, and
  that the management workstation is trusted.
- The Owner password is the only authentication factor in this
  beta. Treat it as a high-value secret.
- Anti-lockout guards protect the management session, but a
  destructive configuration error can still be made by an
  authenticated Owner. Safe Apply's confirmation window is the
  primary safety mechanism. Always confirm a candidate only after
  verifying connectivity through the new state.

## Related documentation

- [`docs/security-model.md`](docs/security-model.md) — detailed trust
  boundaries and threat model.
- [`docs/user/safe-apply.md`](docs/user/safe-apply.md) — Safe Apply
  lifecycle.
- [`docs/user/recovery.md`](docs/user/recovery.md) — recovery
  primitives.
- [`docs/update-system.md`](docs/update-system.md) — update
  signature verification.
- [`docs/privacy-network-egress.md`](docs/privacy-network-egress.md)
  — what network traffic the installed service generates.