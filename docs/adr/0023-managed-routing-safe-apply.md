# ADR 0023: Persistent routing changes through Safe Apply

Status: Accepted for implementation; runtime activation gated.

## Scope

The managed routing adapter connects the durable Safe Apply controller to the
kernel coordinator, networkd artifacts and networkd reload/convergence. Its
initial admitted scope is firewall/NAT, private static route tables and policy
rules on already owned static interfaces. It rejects DHCP/RA interfaces, link
changes, main-table changes, DNS changes and changes to management intent.
These restrictions prevent claiming recovery of resources not yet captured by
this adapter.

## Transaction

Preflight verifies exact existing artifacts, local ownership and management
access. Capture requires actual routes/rules to match the canonical before-state
and binds kernel recovery evidence to both canonical configurations. Safe Apply
persists the evidence before mutation.

Apply changes the kernel under the forwarding barrier, durably replaces the
owned networkd files, reloads networkd, and waits for configured owned profiles.
It verifies files, bindings, kernel routes/rules, filtering and management access
before removing the barrier. The controller then performs its verification and
waits for explicit confirmation; a successful command is not confirmation.

Recovery installs the old firewall's input/output behavior with a forwarding
barrier, restores exact before/candidate artifact unions, reloads the restored
profiles, restores the captured kernel state and verifies every stage before
releasing the barrier. A failed owned networkd profile may participate in this
recovery path; a foreign profile or replacement link is never adopted.
Unknown artifact/route/rule drift remains a recovery error.

## Manager authority and boot safety

`networkctl reload` is manager-wide: it can reconfigure interfaces matching
changed profiles and create netdevs. It does not remove netdevs whose files were
deleted. Therefore local root enrollment must explicitly include
`networkdReload: true`; existing per-interface enrollment does not grant reload
authority. The installer does not set this flag or reload host networking.
Interface retirement remains refused by this component.

The Agent constructor requires trusted management-verification and boot-guard
dependencies. No API-supplied boolean can stand in for these checks. The boot
guard must ensure pending file transactions are recovered before networkd can
load candidate files after a restart. This startup path is not yet wired into
the installed services, so the adapter is not exposed for runtime mutation.
Physical identity and daemon ownership are rechecked with real Linux inventory.

Portable tests run the actual Safe Apply controller and artifact store against
fault-injected kernel/daemon executors. They cover confirmation, rollback,
daemon acknowledgement loss, management failure, uncertain barrier release and
recovery with a new adapter instance. Native daemon and boot-order qualification
remain separate integration requirements.

Reference: [systemd 257 networkctl reload and reconfigure semantics](https://github.com/systemd/systemd/blob/v257/man/networkctl.xml).
