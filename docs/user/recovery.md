# Recovery basics

DuckOS V1 is designed so that an installed system can be recovered
without a full reinstall. This page describes the recovery primitives
that the V1 beta exposes.

## The recovery primitives

| Primitive                     | Lives in                                | Purpose                                                  |
|------------------------------|------------------------------------------|----------------------------------------------------------|
| Pre-network boot recovery   | `duck-network-recover.service` (systemd)  | Reconstruct host routes / rules from the durable journal. |
| Safe Apply rollback           | Engine + Agent                              | Restore the snapshot captured at Apply start.           |
| Update rollback               | Update supervisor                         | Revert to the previous `.duck` payload.                 |
| Owner reset                   | CLI                                       | Recover from a lost Owner account (Owner accounts only). |
| Service restart               | systemd (and Agent CLI)                    | Restart an unhealthy service.                            |

## Boot recovery

The `duck-network-recover.service` is a one-shot systemd unit that runs
**before** `systemd-networkd.service`. It is intentionally NOT
`PrivateNetwork=yes` because it needs to inspect host routes and rules
through rtnetlink. The unit is sandboxed through `IPAddressDeny=any`,
`RestrictAddressFamilies`, and an empty `CapabilityBoundingSet` so it
cannot reach the public network even though it has rtnetlink access.

The recovery reads the journal the Safe Apply controller writes, and
restores host routing state from it. The boot recovery is the safety
net for the case where the Engine dies or loses its connection to the
Agent during an Apply.

If the boot recovery fails, the system falls back to a minimal
configuration with the management anchor preserved. The device is
still reachable on the management LAN; from there you can use the
Safe Apply UI to inspect the journal and roll back.

## Safe Apply rollback

Every Apply captures a snapshot. The snapshot is durable on disk
before any mutation runs. Rollback restores the snapshot.

Rollback happens automatically when:

- A scoped reload fails.
- The Agent loses its acknowledgement.
- The watchdog detects a drift between expected and observed state.

You can also rollback manually from the Safe Apply UI before
Confirming.

If a rollback itself fails, the boot recovery unit reconstructs the
state on the next reboot.

## Update rollback

The update supervisor keeps the previous `.duck` payload on disk. If
the new payload fails to come up — the unit does not start, the Engine
does not respond, or the Agent does not reach steady state within the
window — the supervisor rolls back to the previous payload.

See [`docs/update-system.md`](../update-system.md) for the full
contract and the `Active + Previous` invariant.

## Owner reset

The beta.2 archive does not install `duck`. These commands require the
[source-built CLI](../cli-api.md); stop `duck-api` before offline reset.
Keep console access and review the consequences before proceeding.


If the Owner account is lost and no other admin can recover it, the
CLI exposes a destructive one-shot recovery: `sudo duck owner reset`.
This deletes every Owner account on the device. Other users are
preserved. Backups are written to `/var/backups/duckos-owner` before
the deletion.

After a reset, the device must be re-enrolled as a new Owner. Network
services are not restarted by the reset itself.

`owner reset` is deliberately not exposed in the web UI. It is a
last-resort CLI action and it is logged in the audit log.

## What recovery does NOT do

- Recovery does not roll back the durable journal. The journal is
  retained for inspection.
- Recovery does not delete Doctor findings. Findings are observations;
  the system keeps them.
- Recovery does not reset the audit log.
- Recovery does not delete the update `Active + Previous` pair.

## When to escalate

If the boot recovery fails, if a rollback fails, if the device is not
reachable on the management LAN after a recovery, or if the audit log
shows transactions you do not recognize — escalate through the public
private reporting channel described in [`../SECURITY.md`](../../SECURITY.md).
Include the device's exact commit hash and architecture.