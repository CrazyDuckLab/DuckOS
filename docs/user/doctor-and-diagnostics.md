# Doctor and diagnostics

DuckOS includes a **Doctor** subsystem that runs health checks
periodically and surfaces findings. The Doctor is the only diagnostic
that lives in the Easy Mode story; everything else is in Full Mode.

Doctor is not an Artificial Intelligence tool. It is a deterministic
checker that emits findings with a component, a code, a severity, and a
message. DuckOS does not currently wire any generative model into
Doctor or anywhere else.

## What Doctor checks

Doctor checks a fixed set of subsystems. The exact list and the
exhaustiveness of each check is part of the qualification evidence.

Common finding categories include:

- Disk wear and SMART health.
- Filesystem and storage pressure.
- DNS resolver reachability.
- Service lifecycle (nftables, dnsmasq, frr, networkd, WireGuard).
- Configuration drift between observed state and durable intent.

## What Doctor findings are NOT

- Doctor findings are not the same as **verified system health**. A
  report with zero findings means "Doctor found nothing wrong" — it
  does NOT mean "DuckOS has verified the system is healthy". Verified
  health requires an explicit production contract that the backend
  does not currently provide.
- Doctor findings are not a substitute for Safe Apply. A Doctor
  finding about DNS reachability is a description of observed state;
  it does not propose a change.
- Doctor is not a generative remediation assistant. The Doctor UI
  surfaces findings and links to the relevant recovery action; it
  does not invent remediation steps.

## Doctor states in the UI

The UI distinguishes four Doctor states:

- **Loading** — Doctor has not yet responded. Easy Care renders
  "Checking Doctor…". The numeric findings count is **not shown**
  during loading. This is intentional: "0 findings" before Doctor has
  responded is a false claim.
- **Doctor unavailable** — the Doctor request errored. The UI does
  NOT fall back to "0 findings" or "No current Doctor findings"; it
  shows "Doctor status unavailable" with a Retry action.
- **No current Doctor findings** — Doctor responded successfully with
  zero findings. The findings count line shows "0 findings" and the
  empty-state card explains that Doctor reported nothing wrong but
  verified system health is not yet available.
- **Findings** — Doctor returned one or more findings. Each finding
  appears as a card with the component, the code, the message, and an
  Open Doctor action.

## Recovery actions

A Doctor finding card surfaces the relevant **recovery action**. The
recovery action depends on the subsystem Doctor flagged:

| Subsystem | Recovery action                                  |
|-----------|---------------------------------------------------|
| DNS       | Re-fetch resolvers; restart dnsmasq.              |
| Firewall  | Inspect the firewall state; rollback if needed.   |
| Service   | Inspect the unit; restart; check logs.            |
| Storage   | Inspect SMART; check mount pressure.              |
| Update    | Inspect update system; check Active + Previous.   |

Most recovery actions are documented under [`recovery.md`](recovery.md).

## When to look at the audit log

The audit log is in Full Mode under **Audit**. It records every
configuration transaction, every Doctor finding emitted, every
identity change, and every authentication event. The audit log is the
canonical source for "who changed what and when" questions.

## Operational checklist

- A Doctor finding is a signal, not a verdict. Combine it with the
  Easy Mode headline and the relevant subsystem state.
- Doctor empty does not mean verified healthy. Wait for the backend
  to grow verified-health evidence.
- Doctor error does not mean zero findings. The UI says so.
- Doctor loading does not mean zero findings. The UI says so.