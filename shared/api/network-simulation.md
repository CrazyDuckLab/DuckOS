# Network simulation API v1

`POST /api/v1/network/simulate` evaluates a candidate without sending traffic,
resolving DNS, contacting the Agent, writing state or confirming Safe Apply.
The same enrolled Owner/NetworkAdmin session, transport restrictions, 256 KiB
request bound and error conventions as [network preview](network-preview.md)
apply. Responses are private/no-store; errors never echo submitted configuration.

The request contains `candidate` (canonical `network.Config`) and `question`:

```json
{
  "ingress": "inside",
  "source": "198.51.100.20",
  "destination": "203.0.113.80",
  "protocol": "tcp",
  "port": 443,
  "connectionState": "new"
}
```

Ingress is an interface object ID, not a guessed network derived from source IP.
Both addresses must be unicast literals of one family. Protocol is `tcp`, `udp`,
`icmp` for IPv4 or `ipv6-icmp` for IPv6. Port is optional for TCP/UDP; a missing
port becomes unknown when a relevant port-specific rule could change the result.
ICMP has no port. Existing/related/invalid connection questions return unsupported
because the actual conntrack/NAT binding and packet validity are unavailable.

Success contains `schemaVersion: "1"`, `hostValidated: false`,
`applyAvailable: false` and `explanation`:

- `basis: "candidate-only"`, `reachability: "not-tested"`;
- `configDigest`: SHA-256 of the validated candidate's canonical Go JSON,
  matching the configuration fingerprint used by network recovery adapters;
- `status`: `conditional`, `unknown` or `unsupported`;
- `managedDecision`: `allow`, `drop` or `unknown`;
- `summary`, explicit runtime `assumptions`, and ordered `steps` with stable
  object provenance, stage/code/detail and applicable policy priority/table;
- selected `egress`/`gateway` if known, and `destination`/`port` after DNAT.

A conditional managed allow is not an Internet or service reachability result.
Unknowns must not be rendered as allows, zeroes or healthy connections. Local
input/output, bridge switching, proxy identity, DNS/category resolution, dynamic
leases/RA, existing conntrack and physical/tunnel health are not inferred.
This interface currently analyzes a single new flow, not hostname answers,
network-wide ranges or a live packet trace. Before/after comparison can evaluate
two candidates with the identical question; their digests must remain visible.

The renderer and evaluator share ordered forward/NAT instructions and policy
lookup/unreachable pairs. DNAT changes the destination before route selection
and filtering. Earlier explicit firewall drops still precede generated DNAT
accept rules. A selected private table with no route stops at its unreachable
guard instead of silently falling through to ordinary WAN routing.

This contract exposes a working offline Engine use case. A beginner-oriented
configuration editor and unified client/network identity selection remain
required UI integration work; no JSON-upload workflow is presented as that UI.
