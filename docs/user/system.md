# System Overview and health

The **System Overview** page in Full Mode is the canonical landing
page for administrators. It surfaces the observed state of the device
and the centralized health conclusion.

## What the page shows

- **Resources at a glance** — CPU, memory, interfaces, uptime.
- **Identity** — hostname, OS, kernel, architecture, environment,
  last-observed timestamp.
- **Storage**, **Services**, **DNS**, **nftables**, **issues** —
  each renders the observed value against the live system.

## Health conclusions

The System Overview page headline is one of:

- **Needs attention** — Doctor or the overview has raised a finding.
  Open Doctor for the cards.
- **Status incomplete** — the public health endpoint responded, but
  the production contracts do not yet provide enough evidence to
  claim verified health. The system may be working; the UI refuses
  to claim it is.
- **Unavailable** — the public health endpoint did not respond.

The page never claims **Healthy** or **Observation fresh** without
real evidence. Verified health and observation freshness are two
separate concepts, and the UI is honest about which one the current
production contracts support.

## Observation freshness

The page shows a freshness pill. The pill is **Observation fresh**
only when:

- The observation timestamp parsed successfully.
- The timestamp is not in the future (beyond a small skew tolerance).
- The timestamp is not older than the freshness threshold.

The pill is independent of Doctor findings. Doctor warnings do not
make a recent timestamp stale. A recent timestamp with Doctor
warnings is still a recent observation; the warnings are surfaced as
attention findings.

## What you should NOT expect from the page

- It is not a network activity monitor. The V1 beta does not show
  per-interface traffic counters in the System Overview.
- It is not a verified-health claim. The page will say
  "Status incomplete" even on a healthy-looking system until the
  backend grows verified-health evidence.