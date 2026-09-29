# Troubleshooting

This page is a triage guide. It does not replace the Doctor; it
explains what to do when the Doctor surfaces a finding or when the
UI shows a state you did not expect.

## First steps

1. **Easy Mode headline.** The headline tells you which surface to
   open first.
2. **System Overview headline.** Same surface, denser view.
3. **Easy Care.** Doctor findings cards live here.
4. **Doctor** in Full Mode. Same findings, full reference.
5. **Audit log.** The canonical record of who changed what and when.

## Common symptoms and what they mean

### "Status unavailable"

The public health endpoint did not respond. The browser can talk to
the page; the backend cannot. Verify:

- The duck-api unit is running (`systemctl status duck-api`).
- The management LAN can route to the API listener.
- The Token is valid (log in again).

### "Status incomplete"

The public health endpoint responded, but the production contracts
do not provide enough evidence to claim verified health. This is
**the default state** for a current DuckOS V1 beta. It does not mean
the system is broken; it means the system is honest about which
signals it has.

### "Some things need attention"

Doctor or the overview has raised a finding. Open Doctor for the
card list. The card's component, code, and message identify which
subsystem is involved.

### Easy Care shows "Doctor status unavailable"

The Doctor endpoint errored. Use the **Retry** action. If the
endpoint stays unavailable, the Doctor subsystem is the problem,
not the rest of the system.

### Easy Care shows "Checking Doctor…"

The Doctor endpoint has not yet responded. The numeric findings
count is **not** shown in this state. Wait for the first Doctor
response before drawing any conclusion.

### Easy Home headline says "Status check incomplete" but System
tile says "Everything is working"

That combination is structurally impossible in V1 beta. If you see
it, file a bug against the UI; the centralized health derivation in
`web/src/state/health.ts` is the source of truth and it cannot
produce "good" verdicts without verified-health evidence.

### Safe Apply hung in `Applying`

The Agent may be unreachable. Verify connectivity through the
management path. If the device is reachable but the Agent is not,
reboot the device — the boot recovery unit reconstructs host state.

### WireGuard peers cannot connect

Verify:

- The peer public key matches the public key DuckOS has on file.
- The endpoint address and port are correct.
- The Allowed IPs in the peer configuration match the Allowed IPs
  DuckOS is sending.
- The firewall allows the WireGuard UDP port.

### Xray routing rules not applied

The V1 beta does not yet write Xray routing rules through the public
UI.

## Escalation

If you have collected the symptoms above and you still cannot resolve
the issue, escalate through the private reporting channel described
in [`../SECURITY.md`](../../SECURITY.md). Include:

- The exact DuckOS commit hash.
- The architecture (`amd64`, `arm64`, `armv7`).
- The Debian / kernel version on the device.
- The Doctor findings (if any).
- A sanitized log excerpt.