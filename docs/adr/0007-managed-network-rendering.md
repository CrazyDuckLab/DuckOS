# ADR-0007: Explicit managed networking plans

- Status: Accepted for offline implementation; host enrollment remains gated
- Date: 2026-09-07
- Management grant semantics refined by [ADR-0013](0013-independent-management-policy.md).

The initial network backend targets Debian 13 systemd-networkd, dnsmasq, and
nftables. Engine and Agent independently validate the same bounded canonical
schema. Physical interface bindings are explicit; VLANs reference stable parent
IDs. Dual-stack addresses and routes are first-class. Unsupported settings are
rejected, never retained while silently ignored.

Renderers are pure functions producing fixed-role artifacts, not arbitrary host
paths or shell commands. They generate networkd link/network definitions,
dnsmasq DNS/DHCP configuration, and one complete nftables transaction affecting
only the dedicated `inet duckos` table. They never flush the host ruleset.
Default forwarding and input are deny; management is limited to explicit LANs,
DHCP/DNS to LANs, and established traffic and essential ICMP are permitted.
LAN-to-WAN forwarding requires explicit rules. Port forwards match a local
destination address so routed traffic is not accidentally redirected.

The empty, unenrolled configuration is an exception to deny-by-default managed
operation: it renders no networkd files, no DNS configuration, and no nft hooks.
Its nft batch destroys only the owned DuckOS table, including all base chains.
Nonempty plans destroy/recreate that table in one atomic nft batch, rather than
leaving obsolete chain hooks or policies behind with a rules-only flush.
The adapter must verify table
ownership before executing it and must stop the owned DNS service when its
configuration is empty; an empty string is not permission to start dnsmasq with
host defaults. DNS intent without managed interfaces is rejected. Removing the
last managed interface remains a dangerous, confirmed transaction, not an
implicit way to bypass enrollment or recovery.

Rendering is not host ownership or Safe Apply. Before activation the Agent must
require explicit enrollment, refuse competing network managers/configuration,
snapshot its owned artifacts, validate daemon candidates, arm a durable
independent rollback watchdog, and verify actual Linux state. Any management
access change needs confirmation. Existing observation-only installations remain
read-only until that complete lifecycle is connected and tested.

No Debian package mutations are part of a network transaction. Missing reviewed
dependencies must be reported during enrollment. Restoring owned network state
does not claim to undo arbitrary operator commands or Debian package upgrades.

## Artifact replacement implementation

The initial Agent-side file primitive verifies exact generated before-state and
rejects collisions for new files. Root-owned unrelated files are not imported
or removed. Replacement uses private exclusive staging, fsync, rename and
directory fsync, with final mode 0644 for the systemd-network service to read.
The enclosing transaction must already be durable because a multi-file set is
not atomically replaced as a whole.

Restore admits only before/candidate values and validated interrupted staging
prefixes for journaled names. Unknown drift is preserved and surfaced as
recovery-required. This does not guarantee rollback of out-of-band root edits.
Local tests exercise replacement, partial staging recovery, drift refusal,
symlink/path injection rejection and preservation of unrelated files. This
primitive is not yet an enabled Linux mutation endpoint.

DNS candidate preflight invokes only `/usr/sbin/dnsmasq --test --conf-file=-`,
with generated configuration on stdin and a five-second deadline. This prevents
loading host defaults and does not start the daemon. Empty intent skips syntax
execution and still means the owned service must stop, never start with defaults.
Syntax success is not evidence of socket binding, DNS resolution or DHCP leases.

Sources: [systemd-networkd configuration](https://www.freedesktop.org/software/systemd/man/257/systemd.network.html),
[dnsmasq manual](https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html),
[nftables reference](https://netfilter.org/projects/nftables/manpage.html).
