# ADR 0021: Isolated DNS/DHCP service lifecycle

Status: Accepted for implementation; native deployment qualification pending.

## Decision

DuckOS packages a dedicated `duck-dnsmasq.service`, separate from any host
`dnsmasq.service`. It runs as the non-login `duck-dnsmasq` identity. Its
configuration directory is root-owned and group-readable; its private lease
state belongs to the service identity. The DNS service is not a member of the
API service group and cannot read the API authentication token through group
membership.

Installation creates the identity, directories and unit, but does not install
DNS configuration, enable or start the unit. Existing host DNS services are not
stopped or modified. The unit requires both its explicit configuration file and
a volatile Agent readiness marker. These conditions are admission gates, not
connectivity checks.

The privileged lifecycle primitive accepts only the fixed DuckOS unit. It checks
root ownership, the exact packaged unit bytes, the loaded fragment, absence of
drop-ins and pending daemon reload, and the service identity before operations.
Only stable active/running and inactive/dead observations are accepted. Unknown,
failed and transitional states require recovery rather than being interpreted
as absence. Start/stop uses exact before-state comparison and verifies the
result. A successful systemctl exit with an unsatisfied unit condition is not a
successful start. No enable, disable, reload or reset-failed operation is exposed.

Recovery has a separate stop-only operation. With durable transaction ownership,
it can stop the owned unit even after a process-free failed start, while checking
unchanged boot enablement. Transitional states, a failed unit with a remaining
main process, foreign ownership and uncertain stop acknowledgements remain
errors. The operation verifies a process-free inactive or failed state and does
not reset failure evidence or bypass systemd start-rate limits. A failed state
still cannot become the stable baseline of a new transaction.

## Integration boundary

The lifecycle primitive is not yet activated by the installed Agent. The
transaction coordinator must publish the readiness marker only after restoring
and verifying durable network state, remove it before recovery, and coordinate
configuration files, routing, the forwarding barrier and service lifecycle.
Failures require bounded recovery while preserving management access. Process
activity alone does not prove that DNS answers or DHCP leases work.

The unit uses a bounded capability set for binding DNS/DHCP sockets and packet
operations, a read-only system filesystem, private state, and bounded resources
and journal output. Query logging and debug foreground mode are not enabled.
Native Debian testing must qualify capability handling, socket binding,
interface changes, lease persistence, startup gating and recovery before this
service can be considered production-ready.
