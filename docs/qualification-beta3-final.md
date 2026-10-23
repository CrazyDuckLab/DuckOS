# Beta.3 final Router Core qualification and release provenance

Status: **Router Core GO accepted by the release owner; publication pending**.
Qualified executable baseline: `2e0c5afa089649a3238c543a82624075a9bc816e`.

The release owner reports completed native Linux AMD64 Router Core regression,
firewall/NAT packet, installer, real systemd startup, authenticated readiness,
service restart and VM reboot persistence qualification on that exact baseline.
This section records that accepted prior qualification. No new live run is
claimed by the documentation/snapshot preparation. Older reports below are
historical evidence with their original SHA and limits, not additional passes.

The final release source commit adds documentation and snapshot hygiene only.
Release verification must prove executable sources, units, installer/build
scripts and lockfiles are byte-identical to this baseline. `BUILD-INFO.json`
records the final release SHA; the approval manifest records both SHAs.

| Finding | Remediation present in qualified code |
|---|---|
| F1 text-parser bypass | Whole-batch lexing with quote/comment/escape handling before authorization; adversarial regression fixtures |
| F2 JSON-wrapper ambiguity | Strict nested object schema, allowlists and duplicate-key rejection |
| F3 incomplete payload admission | Mandatory nft launcher binary/unit, required core ELF headers, executable modes and truncation checks |
| F4 PCP wire format | RFC-derived opcode/header/MAP/lifetime fixtures and corrected codec; ANNOUNCE/MAP subset only |
| F5 startup/recovery race | Dynamic launcher availability and bounded journal recovery retry |
| F6 failed-install bookkeeping | Created/pre-existing launcher handling and fault-injection regression coverage |
| F7 foreign reads | Owned-table capture and server-side filtered global observation |
| F8 resource pressure | Connection/execution/queued-byte bounds, admission deadlines and overload recovery tests |

F1/F2/F5/F7/F8 remediation is in `ebdf3cd`, F3/F6 in `62f46cf`, F4 in
`33ee2e8`; `2e0c5af` adds nft launcher `NoNewPrivileges=yes`. These references
identify source changes; they are not fabricated fresh execution logs.

AMD64 VM execution is distinct from fresh release cross-build/inventory checks.
ARM64 and ARMv7 have no native hardware qualification in this release. In-place
upgrades, physical power loss, arbitrary hardware and universal protocol-client
interoperability remain unqualified. Xray Center beta.4 is excluded.

## Superseded historical STOP report — preserved verbatim

The following report describes the earlier source and pre-remediation state.
Its privilege-boundary and publication conclusions are superseded for the
qualified Router Core baseline above; its recorded failures and limits are
retained rather than converted into passes. Current publication remains a
separate owner security and release approval decision.

---

# Beta.3 independent release gate

Status: **PUBLICATION BLOCKED**, 2026-10-15. Starting source: `ef8cf25`.
The public release remains beta.2. No beta.3 tag or public assets are authorized
while the installed privilege boundary is unresolved.

## Verified blockers and fixes

The independent Linux rerun reproduced both old NAT-PMP UDP failures. The
loopback tests conflicted with the explicit loopback-source rejection policy.
Inspection against [RFC 6886](https://www.rfc-editor.org/rfc/rfc6886.html)
also found real runtime defects: reserved bytes were mistaken for port fields,
the public-address request opcode was wrong, a one-byte packet could panic,
and the UDP source port was incorrectly required to equal the mapped port.
Lifetime zero created a lease instead of deleting it.

The isolated release branch fixes those defects, adds RFC byte fixtures,
short-packet fuzz seeds, deletion ownership/error tests, and nonloopback socket
tests that re-exec in a fresh Linux network namespace. Loopback-source refusal
remains explicitly tested. Runtime mapping admission now enforces the stored
network client allow-list rather than trusting the frontend descriptor.

Native Debian AMD64 regression tests passed for the corrected NAT-PMP socket
path, refusal handling and runtime allow-list boundary. These socket tests use
a memory mapping sink: they establish packet exchange and protocol semantics,
not privileged kernel mutation by the installed service.

The build now rejects malformed semver including leading-zero numeric
identifiers. Version tests run in a temporary Git repository and never
force-update shared release tags. Payload staging includes the DuckOS LICENSE
and COPYRIGHT.md alongside the existing mandatory Xray license.

## Mandatory installed-service blocker

`duck-agent.service` has an empty capability bounding set and
`NoNewPrivileges=yes`. The new firewall adapter and mapping backend execute
`nft` directly. An independent test in a disposable Linux network namespace
confirmed that `nft list tables` works as privileged root and fails with
`Operation not permitted` after all capabilities are dropped. The running
installed Agent also has CapEff/CapBnd zero.

Consequently privileged root component tests cannot qualify the installed
Agent-to-kernel path. Granting capabilities to the Agent would violate the
required boundary. A new or extended bounded privileged mechanism needs
security design, strict operation and ownership authorization, installation
wiring, crash/rollback testing and installed-boundary qualification. No such
mechanism was improvised or silently substituted in this release audit.

## Evidence boundaries

The scoped `duckos_fw` decoder, canonical empty state and semantic verification
fixes are present. Network, firewall and mapping tables have separate owners,
but their hooks affect the same traffic; separate names alone are no proof of
coexistence or concurrent rollback safety. Live tests must run in an explicitly
verified isolated namespace; the firewall live test helper now refuses the host
namespace even when its opt-in environment variable is set.

One authorized Debian AMD64 target was reachable. The other SSH alias timed
out; its previous address was not substituted. No management interface, SSH,
default route, installed services, operating system or existing worktree was
reconfigured. No VM was created, rebooted or reinstalled.

H08/DV07 systemd witnesses refuse to replace an existing installation. The unit, updater and witness source paths are unchanged from beta.2.
The independent audit changed installer artifact validation to accept clean
prerelease metadata and require DuckOS license files; that parser is covered
by synthetic packaging tests, not a fresh host installation.
Historical beta.2 qualification reports are relevant context, but are not
independent raw execution evidence for this candidate. No new fresh-host pass,
power-loss pass or physical ARM qualification is claimed.

The final release commit and official artifacts do not exist: mandatory pre-tag
gates have failed. Development builds and partial checks must remain labelled
as such. Do not reuse artifacts from `9b82faf` or label them beta.3.

## Scope

The intended release is network core only. UI Next, Xray Center, Subscription
Center, Stealth Admin, Core/Complete profiles and unmerged PPPoE/L2TP work are
excluded. This file records a release blocker, not a qualified feature claim.
