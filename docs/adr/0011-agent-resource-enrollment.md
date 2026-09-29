# ADR-0011: Local Agent resource enrollment

- Status: Accepted as an ownership prerequisite, not complete host takeover
- Date: 2026-09-09

Canonical API configuration cannot grant itself root authority over a NIC or
kernel namespace. A separate bounded `enrollment.json`, provisioned locally by
root in the private Agent directory, declares physical ID/name/MAC bindings,
reserved VLAN interface names and the exact `inet duckos` firewall scope.
The runtime reader never creates, imports, edits or upgrades this document.
Directory mode is 0700 and file mode 0600, owned by the Agent service identity
(root in deployment); symlinks, hard links, changed inodes and oversized or
unknown document fields are rejected.

Before mutation, both previous and candidate physical bindings must match this
allowlist and a fresh Linux host snapshot. A changed interface index after boot
is permitted; a changed enrolled name, logical ID or MAC requires local review.
VLAN names must be explicitly reserved. This protects against accidental NIC
replacement and arbitrary API-selected target names, not malicious root access
or intentional MAC spoofing by another privileged process.

Enrollment is necessary but insufficient for activation. Competing network
managers, networkd winning files/drop-ins, current kernel VLAN ownership,
artifact drift, management reachability and available recovery space still
require independent checks. This does not resolve arbitrary existing Debian
network takeover or authorize overwriting foreign configuration. There is no
automatic enrollment endpoint or installer takeover in this change.

Portable tests verify binding/replacement/name checks, private file permissions,
read-only behavior, symlink/hard-link rejection and bounded strict decoding.
They do not establish native exclusive ownership or usable Linux networking.

The networkd ownership reader obtains bounded manager JSON through fixed
`networkctl --no-pager --no-ask-password --json=short list` arguments. Existing
intent must have the exact generated winning `.network` file, no drop-ins and
configured state. New physical bindings must be unmanaged by networkd; new
VLAN names must be absent. In-place kind/parent/VLAN identity replacement is
refused until an explicit kernel replacement plan exists. This is read-only
preflight and still does not detect all competing configuration tools.

`EnrolledFirewallAdapter` applies these live checks before preflight, capture,
apply and confirmation verification. Recovery still attempts restoration of
the exclusively owned firewall table when a NIC disappears or networkd fails;
subsequent verification retains recovery-required status instead of claiming
the network is healthy. Neither constructor is enabled by installed services.

Protocol reference: systemd 257 [networkctl description implementation](https://raw.githubusercontent.com/systemd/systemd/v257/src/network/networkctl-description.c)
and [networkd JSON fields](https://raw.githubusercontent.com/systemd/systemd/v257/src/network/networkd-json.c).
