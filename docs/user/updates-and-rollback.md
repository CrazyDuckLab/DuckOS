# Updates and rollback

DuckOS uses a signed `.duck` payload for updates. The update
supervisor keeps the current payload and the previous payload on
disk. The supervisor enforces the `Active + Previous` invariant: at
all times, two known-good payloads are available.

## Update flow

1. The administrator downloads a signed `.duck` payload.
2. The update supervisor validates the signature.
3. The supervisor writes the new payload as **Previous** (replacing
   the previous Previous) and the current payload becomes
   **Active**.
4. The new Active payload is started.
5. The supervisor waits for the new Active to reach steady state
   within a configured window.
6. If steady state is reached, the new payload becomes the new
   Active. If steady state is **not** reached, the supervisor rolls
   back to the previous payload.

## Active + Previous invariant

DuckOS does not allow three concurrent payloads. There is exactly one
**Active** and one **Previous**. A failed update keeps the prior
**Active** as the new **Previous**, so the device always has two
known-good states to choose from.

## Rollback

Rollback can be triggered:

- Manually from the UI, in Full Mode under **Updates**.
- Automatically by the supervisor, when the new Active does not
  reach steady state.
- By the recovery primitives when the device reboots into a
  non-functional state.

The CLI exposes a destructive rollback path: `sudo duck upgrade
rollback`. This action is logged in the audit log.

## What the V1 beta does NOT do

- It does not yet auto-download new releases.
- It does not yet surface the rollback UI as a Full Mode page.

See [`docs/update-system.md`](../update-system.md) and
[`docs/updater-supervisor.md`](../updater-supervisor.md).