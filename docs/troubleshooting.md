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
[SECURITY.md](../SECURITY.md), not public issues. Duck Doctor and automatic
support bundles remain planned; no undocumented command is required for a report.
