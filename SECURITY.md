# DuckOS security policy

## Supported versions

DuckOS has no released, installable, or supported version yet.

| Version | Supported |
|---|---|
| Public releases | None exist |
| Repository bootstrap and internal development snapshots | No production support |

Do not expose or deploy the current repository as a router, security boundary,
or production management service. A document describing a planned control is
not evidence that the control exists.

## Reporting a vulnerability

Please report suspected vulnerabilities privately:

1. Use GitHub's **Report a vulnerability** action on the
   `CrazyDuckLab/DuckOS` Security page if it is available to you.
2. If that action is unavailable, contact a `CrazyDuckLab` repository owner
   through an existing private channel and ask for a private reporting path.
3. Do not include exploit details, credentials, private keys, tokens,
   subscription URLs, customer data, packet payloads, or production addresses
   in a public issue or discussion.

No dedicated security email, response-time SLA, bug-bounty program, or public
disclosure timeline has been established at bootstrap. This file must be
updated before the first external release with an operational contact and
maintainer process.

## Licensing and attribution

DuckOS Community source is licensed under **GPL-3.0-only** — see
[`LICENSE`](LICENSE) and [`COPYRIGHT.md`](COPYRIGHT.md). Trademarks
are governed by [`TRADEMARKS.md`](TRADEMARKS.md). The
commercial / dual-licensing narrative is in
[`docs/licensing.md`](docs/licensing.md).

A useful report includes, when safe:

- affected commit or artifact digest and target architecture;
- Debian/kernel and relevant dependency versions;
- the trust boundary and expected versus observed behavior;
- minimal synthetic reproduction steps;
- impact on confidentiality, integrity, availability, management access, or
  rollback/recovery;
- sanitized logs or evidence with all secrets removed.

## Security-sensitive areas

High-impact areas include authentication and RBAC, Engine-to-Agent IPC,
privileged operations, canonical state and transaction recovery, firewall and
management exposure, secret storage, update signatures and migrations,
installer/host ownership, backup/restore, support-bundle redaction, storage
exhaustion, plugins, and any future full-shell terminal.

The foundation threat model and unresolved decisions are maintained in
[`docs/security-model.md`](docs/security-model.md). Update and storage safety
contracts are in [`docs/update-system.md`](docs/update-system.md) and
[`docs/storage-policy.md`](docs/storage-policy.md).

## Safe research and testing

- Test only repository-owned DEV fixtures and explicitly designated isolated
  TEST systems that you are authorized to use.
- Never run destructive or network-control tests on a production system or an
  unrelated device/network.
- At this stage, do not install or test DuckOS on `duck-dev-01`; it is reserved
  for a later build/test-target stage and will receive committed code from
  GitHub.
- Do not probe third-party systems, publish working exploit details, or retain
  unnecessary sensitive traffic.
- Preserve evidence needed to reproduce and fix the issue, but sanitize and
  bound it according to the storage and privacy policies.

Security fixes must include the appropriate regression coverage and must not
weaken privilege separation, rollback/recovery, IPv6 handling, ARMv7 support,
or bounded resource behavior to hide a symptom.
