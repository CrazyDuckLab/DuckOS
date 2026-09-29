# ADR 0020: Forwarding barrier during network transactions

Status: Accepted for Agent primitives; composite activation remains gated.

## Decision

Use a reserved empty base chain, `duckos_apply_guard`, within the exclusively
enrolled `inet duckos` table during multi-resource network changes. Its forward
hook has priority -10 and policy drop. No rules may be present in this chain:
an inserted accept would defeat a drop policy and must fail verification.

Installation atomically replays the captured owned table plus the barrier,
preserving management input/output rules. Existing use of the reserved name is
a recovery/conflict condition. Before-state must already be durable and exact
kernel equality is checked before installation. Replaying the table resets
dynamic counters; counters are not configuration or rollback identity.

Guarded candidate application includes the barrier in the same nft batch as
the generated filtering/NAT changes. An empty candidate still retains a temporary
owned table with the barrier; deleting configuration must not reopen forwarding
halfway through a transaction. The Agent checks exact chain semantics and absence
of chain rules in kernel readback. A command timeout, missing barrier or altered
policy requires recovery, not success or blind replay.

After all required link/route/policy/DNS checks, the composite coordinator may
perform normal atomic firewall application to release the barrier. Safe Apply
confirmation must not accept a transaction whose barrier remains installed.
Rollback must first protect forwarding while restoring other resources, then
restore the original firewall snapshot last. Management route preservation and
independent deadline rollback are separate obligations; preserving input rules
alone does not prove the management connection survives a route change.

This is a routed-forwarding barrier, not a claim of universal packet isolation.
The activation preflight must account for foreign flowtables, hardware offload,
XDP/tc bypass paths, bridges and conflicting managers before relying on it.
No new public mutation endpoint or automatic resource takeover is enabled here.

## Verification

Portable tests inspect immutable before-state, reserved-name collisions, wrong
hook/priority/policy, injected accept rules, candidate and removal guard retention,
executor ordering, readback failure and recovery after lost acknowledgements.
The opt-in nftables namespace test now exercises actual guard installation,
guarded replacement/removal and original-state restoration. Mac tests and
cross-compilation do not qualify native traffic, offload behavior or management
connectivity; those remain explicit integration gates.
