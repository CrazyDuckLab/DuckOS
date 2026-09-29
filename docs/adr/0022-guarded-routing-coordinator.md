# ADR 0022: Guarded kernel routing coordination

Status: Accepted for implementation; installed mutation path remains gated.

## Decision

The Agent uses a single kernel coordinator for the enrolled firewall table,
private route tables and policy-rule priority ranges. These resources cannot be
applied atomically together. A forwarding barrier must therefore span the whole
change, including uncertain outcomes and recovery.

Capture compiles the candidate, checks root-granted scope, obtains actual kernel
before-state and emits at most 1 MiB of private recovery evidence tied to the
candidate digest. The outer transaction must durably journal this evidence
before invoking mutation. Capture authorizes one attempt; loss of an
acknowledgement never authorizes blind retry. Observation is bounded to 15
seconds and mutation/recovery to 90 seconds each.

Apply verifies unchanged routes and rules, installs the forwarding barrier,
changes private routes, changes policy rules, and applies filtering/NAT while
retaining the barrier. It then verifies all three resources. Success means only
that this kernel stage is complete: it does not release forwarding or confirm
the network transaction.

Recovery works with a newly constructed coordinator and durable evidence. It
first restores the previous input/output firewall behavior with a forwarding
barrier, then restores the routes and policy rules from their journaled
before/candidate unions. Unknown route/rule drift blocks restoration without
flushing or adopting third-party state. Recovery also retains the barrier until
the outer coordinator verifies the complete restored network.

## Remaining activation gates

This component is not an independently exposed API or an enabled startup
mutation path. The outer coordinator must integrate durable canonical state,
networkd/DNS artifacts, daemon lifecycle, physical identity checks, competing
manager detection, absence of bypass/offload paths and management reachability.
It must serialize all owned-resource mutation and release the barrier only after
those checks succeed. The forwarding barrier does not isolate local output,
bridged traffic or external packet-processing paths.

Portable fault tests exercise lost acknowledgements at guard installation,
route mutation, rule mutation and firewall application, recovery after lost
process memory, one-shot admission and unknown drift. They do not replace native
namespace tests, remote-management preservation tests or power-loss testing.
