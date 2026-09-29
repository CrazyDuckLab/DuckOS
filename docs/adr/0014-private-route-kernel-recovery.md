# ADR-0014: Scoped private route capture and recovery primitives

- Status: Accepted for Agent primitives; activation remains gated
- Date: 2026-09-09

## Decision

Private route operations are limited to explicitly root-enrolled tables
10000–10999 and enrolled interface names. Main/local/default tables are excluded.
The production constructor requires root Linux. It executes the fixed iproute2
binary with bounded output and time, no shell and no logged command output.

Capture uses `ip -j -N -4/-6 route show table all`: numeric IDs avoid aliases;
a successful complete inventory distinguishes empty owned tables from a failed
table-specific query. The global inventory is bounded to 2 MiB/8192 entries per
family. Recovery admits at most 512 routes across 128 enrolled tables, only
reproducible static unicast routes with canonical destinations, explicit device,
optional same-family gateway, metric, supported scope and optional on-link flag.
Unknown owned attributes, dynamic routes, multipath, nexthop objects, preferred
sources, non-default IPv6 preferences and link-down/offload flags fail closed.
Foreign tables are not imported or modified.

Apply compares actual before-state, deletes only removed exact routes, adds only
new routes and verifies actual readback. It never flushes or replaces a table.
At most 256 commands are admitted before the first mutation. Commands are not an
atomic batch; an ambiguous result stops immediately and requires recovery, not
candidate replay. Restore accepts only surviving before/candidate members and
preserves unknown drift by refusing restoration. Repeated completed restoration
does not rewrite unchanged routes.

## Activation boundary

These are internal primitives, not an enabled API or installed Agent service.
The composite coordinator still must independently validate hardware bindings,
durably journal captured state, guard affected traffic, coordinate networkd and
RPDB ownership, verify management reconnection and recover after interruption.
Root enrollment is not a claim of exclusive ownership against arbitrary root
changes. Capture is not an atomic cross-family kernel snapshot; the enclosing
transaction must prevent competing managed writers and verify state again.

Full link/RPDB/DHCP/main-route recovery and management traffic protection remain
unfinished. In particular, link loss may require restoring links before route
capture/restore can succeed. Unsupported state remains recovery-required rather
than being silently discarded. Installed runtime remains observation-only.

## Evidence

Portable tests cover strict numeric inventory, IPv4/IPv6 normalization, bounds,
unknown-field refusal, table/interface scope, exact arguments, drift, lost
acknowledgement, partial restoration and idempotent completed restoration.
The opt-in native namespace test creates a disposable dummy link, exercises real
dual-stack route add/change/readback/restore and removes only its own link.
It does not prove packet forwarding or gateway reachability; Linux execution
must be recorded separately from Mac tests/cross-builds.

References: [iproute2 command options](https://raw.githubusercontent.com/iproute2/iproute2/main/man/man8/ip.8),
[route JSON implementation](https://raw.githubusercontent.com/iproute2/iproute2/main/ip/iproute.c),
[numeric identifiers](https://raw.githubusercontent.com/iproute2/iproute2/main/lib/rt_names.c).
