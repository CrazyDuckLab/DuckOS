# ADR-0009: Scoped firewall kernel recovery

- Status: Accepted for Agent implementation; activation requires enrollment
- Date: 2026-09-08

The privileged firewall mechanism uses only `/usr/sbin/nft`, constructed fixed
argument vectors and generated stdin. It never invokes a shell, accepts a
caller-selected executable, or exposes a general command RPC. Each invocation
has a ten-second deadline, bounded output and private/discarded stderr. Syntax
checking precedes kernel writes; inability to read kernel inventory is not an
empty firewall.

DuckOS exclusively owns `inet duckos` after explicit enrollment. Candidate
installation destroys/recreates this table in one atomic nft batch. Empty
configuration destroys it, including base chains and their policies. No host
ruleset flush is allowed and unrelated tables are never replayed or removed.

The Agent captures bounded JSON kernel before-state in its private journal.
Only table, chain and rule objects in the owned family/table are recoverable;
unsupported object kinds require explicit reconciliation rather than silently
losing them. Fingerprints preserve rule ordering and expressions while ignoring
kernel handles and packet-counter changes. Apply refuses a changed baseline.
Recovery replays the captured owned table and compares fresh kernel readback.
An uncertain apply acknowledgement is a recovery condition, not permission to
reissue the candidate.

Enrollment grants the recovery coordinator authority to restore this table if
an apply fails or loses confirmation. Other root administrators must not edit
the enrolled table during a transaction. This does not grant authority over
other tables or establish ownership of interfaces, routing, DNS or daemon files.
Kernel acknowledgement/readback does not prove DHCP, routing, management access
or end-to-end filtering: those remain separate network verification gates.

The mechanism is not yet connected to a mutation RPC or enabled by the installer.
The existing Agent service permissions are not broadened by this implementation.

Implementation extension, 2026-09-09: `FirewallAdapter` connects the real nft
mechanism to the independent Safe Apply controller. It captures versioned
kernel evidence bound to both configurations, consumes each capture for one
apply attempt, verifies kernel readback before confirmation and restores from
the private journal after uncertain apply or process death. Missing/mismatched
evidence refuses recovery. This scoped adapter rejects every non-firewall
configuration change; it cannot silently accept link, DHCP, DNS or route edits.
It is a component for the eventual composite network adapter, not a substitute
for network-wide verification or ownership enrollment. Installed activation
remains disabled pending those integration boundaries.

Sources: [nft reference](https://netfilter.org/projects/nftables/manpage.html),
[atomic batch scripting](https://wiki.netfilter.org/wiki-nftables/index.php/Scripting).
