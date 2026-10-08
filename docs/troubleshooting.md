# Troubleshooting

Start with the exact commit, artifact BUILD-INFO, architecture and
[installation preflight](install.md). DuckOS is not a supported production router.

## Build or installation failure

Run checks in [development.md](development.md). Preserve the first failing step
and sanitized error instead of retrying blindly. Verify artifact integrity before
host changes. Do not delete conflicting configuration to make preflight pass;
identify ownership and establish recovery first.

## Dashboard unavailable or stale

API liveness differs from readiness and Agent availability. Check the documented
loopback address, service state, socket ownership and session expiry. A stale
observation is not a current measurement. Do not expose the API publicly to work
around local authentication or administrative forwarding.

## Network preview

The [preview API](../shared/api/network-preview.md) is not an apply endpoint.
Unsupported configurations are refused. Preview success does not prove Linux
ownership, daemon readiness or connectivity. Do not execute preview output by
hand on a production gateway; full network activation remains gated.

## Reports

Include build, environment, sanitized topology, expected/actual behavior and
reproduction steps. Remove passwords, keys, client QR/config content and
subscription URLs/tokens. Security-sensitive reports follow
[SECURITY.md](../SECURITY.md), not public issues. Doctor provides deterministic findings; automatic support bundles are not
required for reporting.

## Beta.2 launcher startup

Check `duck-agent`, `duck-api`, `duck-frr-launcher` and `duck-wg-launcher`
with systemctl and bounded journal output. `/run/duckos-agent` must be
`root:duckos 0750`; native `frr.service` and `wg-quick@.service` are masked.
Do not unmask competing daemons to work around launcher startup. Agent effective
capabilities should be zero and the WireGuard launcher CAP_NET_ADMIN only.
FRR refusal with missing zebra/bgpd is not proof of functioning BGP routing.

Beta.2 installation is fresh-install-only; existing-state refusal is not a
supported upgrade path. Preserve evidence and consult [installation](install.md).
