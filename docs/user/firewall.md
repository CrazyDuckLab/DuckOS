# Firewall

DuckOS uses **nftables** as the firewall backend. The **Firewall** page
in Full Mode shows the observed firewall state and the canonical model.

## Concepts

- **Tables** — DuckOS owns the `inet duckos` table.
- **Chains** — `input`, `forward`, `output`, plus the
  `duckos_apply_guard` chain that Safe Apply installs.
- **Forwarding guard** — the temporary chain that refuses to forward
  non-drop traffic during a Safe Apply. The guard is what makes a
  mid-Apply state safe by construction.
- **Policy** — a small number of canonical chains. The system does
  not invent arbitrary chains at runtime.

## Safe Apply integration

Every firewall change goes through Safe Apply. The forwarding guard
chain is installed at Apply start, the new rules are committed, the
guard is released only after confirmation. If anything goes wrong,
the snapshot taken at Apply start is restored.

See [`safe-apply.md`](safe-apply.md) for the lifecycle and
[`docs/architecture.md`](../architecture.md) for the firewall adapter.

## What the V1 beta does NOT do

- It does not yet write firewall rules through the public UI. The page
  renders observed state.