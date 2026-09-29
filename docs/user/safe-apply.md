# Safe Apply

Every change that touches the network on a running DuckOS installation
goes through **Safe Apply**. Safe Apply is the only path from a candidate
configuration to the active network state. There is no "apply now"
button, no optimistic commit, and no shortcut that bypasses the
transaction.

This document describes the user-visible behavior. The internal model
is documented in [`docs/architecture.md`](../architecture.md) and the
Linux adapters in `internal/network/`.

## States you will see

A Safe Apply transaction has three observable states:

| State      | Meaning                                                              |
|------------|----------------------------------------------------------------------|
| Pending    | The candidate has been recorded. The active configuration is unchanged. |
| Applying   | The candidate is being applied to the running system.                |
| Confirmed  | The candidate is now the active configuration.                       |

If the Apply fails or the watchdog intervenes, the transaction enters
**RolledBack**. The active configuration is restored to the snapshot
captured at Apply start. The rolled-back candidate remains in the
journal for inspection.

## The lifecycle

1. **Snapshot.** At Apply start, Safe Apply captures the live state of
   the firewall, the routes, and the relevant policy rules. The
   snapshot is durable on disk before any mutation runs.

2. **Forwarding guard.** The guard chain `duckos_apply_guard` is
   installed so the kernel will refuse to forward non-drop traffic
   while the candidate is being applied. The guard is what makes a
   mid-Apply state safe by construction.

3. **Apply.** Each scoped change is written and reloaded. The networkd
   daemon reloads only the interfaces in the candidate's scoped set;
   the management anchor stays outside the scope.

4. **Confirm window.** After Apply succeeds, the system holds the
   active state. There is a confirmation window during which the
   administrator must `Confirm` to finalize. **Auto-confirm does not
   exist.**

5. **Confirm.** The candidate becomes the active configuration. The
   guard chain is released. The snapshot is retained for post-mortem
   until it ages out.

6. **Rollback.** If anything goes wrong — failed reload, lost
   acknowledgement, lost management connectivity — the watchdog
   reverts to the snapshot and the transaction is marked rolled back.

## What you, the administrator, should expect in the UI

- The pending state is visible on the affected pages. The current
  configuration is still in force.
- The Apply phase shows progress. The guard chain is visible to anyone
  with `nft list chain inet duckos duckos_apply_guard`.
- During the confirmation window, the UI shows a prominent Confirm
  action. Rollback from the UI discards the candidate.
- After Confirm, the candidate is the active state. The journal keeps
  the last few transactions for audit.

## What happens if you lose connectivity

The watchdog is independent of the Engine. If the Engine dies or loses
its connection to the Agent during Apply, the Agent reports the
in-flight outcome to the journal. On the next boot, the boot recovery
unit (`duck-network-recover.service`) reconstructs host routes and
rules from the journal before networkd comes up.

This is why Safe Apply uses a one-shot pre-networkd recovery unit and
not the privileged Agent's regular lifecycle. The privileged Agent may
not be reachable during recovery, but the kernel state must be.

## Anti-lockout

The Safe Apply controller refuses to remove the management anchor from
the active set. A candidate that would orphan the management session is
rejected at validate, before the guard is installed. There is no
override that allows an Apply to proceed past this check.

## What Safe Apply does NOT do

- Safe Apply does not auto-confirm. A pending Apply with confirmation.
- Safe Apply does not bypass the journal. Every transaction is
  durable.
- Safe Apply does not silently retry. A failed reload produces a
  rolled-back transaction and a clear error in the UI.
- Safe Apply does not allow the management anchor to be silently
  removed. An anti-lockout validator rejects such candidates before
  the guard is installed.

## Operational checklist

Before clicking Apply:

1. Confirm the candidate diff matches what you intended.
2. Confirm the management anchor is in the candidate's "kept" set.
3. Confirm you can reach the device out-of-band (serial, console, or
   second management path).
4. Do not navigate away from the page during Apply; cancellation is a
   rollback, not a cancel-and-keep-active.

After Apply succeeds:

1. Verify connectivity through the new state.
2. Click **Confirm** in the UI.
3. Watch the watchdog reach steady state.

If anything looks wrong during or after Apply:

1. Click **Rollback** in the UI.
2. Confirm the rolled-back state matches the snapshot.
3. If the rollback itself fails, use the out-of-band console; the boot
   recovery unit will run on next reboot.