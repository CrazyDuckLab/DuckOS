# ADR 0019: Owned routing-policy mutation and recovery

Status: Accepted for Agent primitives; composite network activation remains gated.

## Decision

Capture and reproduce only supported policy rules inside explicitly root-enrolled
priority ranges. Connected-network protection occupies 10000–12047; private-table
lookup/guard pairs occupy 20000–21999. A priority-range grant is separate from the
private routing-table grant. Standard local/main/default rules and foreign ranges
are never imported or deleted by this adapter.

Read bounded numeric iproute2 JSON separately for IPv4 and IPv6. Preserve source
and destination prefixes, priority, lookup table, prefix suppression, unreachable
action and protocol identifier. Unknown selectors/actions in owned ranges,
duplicate family/priority identities and unsupported table references are conflicts,
not fields to omit from recovery. Snapshots allow interrupted single-rule states;
requiring complete pairs during recovery would make a partially applied operation
impossible to recover.

Candidate rules come from the same policy instructions as networkd generation and
offline evaluation. The iproute2 executor creates static rules with protocol 4 and
verifies the actual readback; this does not assume a daemon adopted the rules.
Existing protocol identifiers are preserved in captured recovery state.

Apply checks exact before-state first. Remove changed lookups before their guards;
install new guards before their lookups. A selector replacement still has gaps,
so the composite controller must journal recovery evidence and establish an
independent traffic guard before invoking this primitive. No command batch is
claimed atomic, and no root mutation endpoint is enabled by this decision.

At most 256 rule commands are admitted before the first mutation, with a 30-second
operation budget and bounded subprocess output. Snapshots are limited to 3072
rules and 1 MiB; combined network recovery capacity must be admitted separately.
No shell, flush, arbitrary table name or caller-selected command is exposed.

After an uncertain result, recovery captures surviving rules and accepts only
members of the durable before/candidate union. Unrecognized drift remains intact
and requires reconciliation. Repeated restoration of an already restored snapshot
is read-only. Exclusive in-process execution does not replace global Agent
transaction ownership or conflict checks against other network managers.

## Verification

Portable tests cover numeric inventory, IPv4/IPv6 and wildcard normalization,
unsupported selectors, paired ordering, duplicate priorities, ungranted tables,
lost acknowledgements, unknown drift, replay rejection, idempotent recovery,
capacity admission and cancellation. An opt-in Linux namespace test exercises
actual add/readback/restore. Mac compilation of that test is not native execution.
Management connectivity, power interruption, networkd adoption and the complete
cross-resource transaction remain separate qualification requirements.

## References

- [iproute2 rule inventory and mutation implementation](https://github.com/iproute2/iproute2/blob/main/ip/iprule.c)
- [Shared policy evaluation](0018-shared-network-evaluation.md)
- [Private route recovery](0014-private-route-kernel-recovery.md)
