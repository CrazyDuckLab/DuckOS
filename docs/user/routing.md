# Routing

DuckOS renders observed routes plus a canonical model of the desired
routing state. The **Routes** page is in Full Mode under **Network**
→ **Routing**.

## Concepts

- **Main table** — the kernel's main routing table. DuckOS does not
  modify the main table directly.
- **Private route tables** — DuckOS maintains policy-routing tables
  (`10000`, `10001`, ...) that select based on source address. Each
  private table may include a default route.
- **Source-based policy** — a policy rule that selects a private
  table by source prefix. The combination of policy rules + private
  tables implements source-based routing.
- **Scoped reload** — DuckOS reloads only the interfaces in the
  scoped set on a Safe Apply; the management anchor is always kept
  out of the scoped set.

## What the V1 beta does

- Renders the observed routing state against the live system.
- Validates a candidate against the canonical model.

## What the V1 beta does NOT do

- It does not yet write private routes through the public UI.

See [`docs/route-compilation.md`](../route-compilation.md) for the
deterministic renderer and [`docs/architecture.md`](../architecture.md)
for the data model.