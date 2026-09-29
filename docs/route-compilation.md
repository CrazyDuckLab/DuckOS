# Static route compilation

Canonical route intent is shared by the networkd renderer, policy simulator and
the Agent's private-table route compiler. The latter produces exact static
kernel expectations only for explicitly enrolled private tables; it neither
imports main/local tables nor substitutes dynamic lease state.

IPv4 metric zero remains zero. IPv6 metric zero means effective metric 1024,
matching Linux's `IP6_RT_PRIO_USER` substitution on route creation. Validation
therefore rejects two same-prefix, same-table IPv6 routes whose metrics are zero
and 1024: they are equal-cost routes, not two distinct preferences. The current
schema requires an explicit future multipath group rather than silently
accepting that ambiguity. Rendering and simulation use the effective metric;
the original canonical input is not rewritten.

Private-table gateway routes use the already validated on-link gateway and
explicit on-link flag. Direct IPv4 routes have link scope; IPv6 route readback
uses global scope. Interface object IDs resolve to validated kernel names.
Snapshots require the effective IPv6 metric because zero cannot be reproduced
as such by Linux. The compiler does not establish interface ownership, link
health, gateway reachability or authorization to mutate the host.

Kernel references:

- [IPv6 route creation and zero-metric substitution](https://github.com/torvalds/linux/blob/master/net/ipv6/route.c)
- [IPv6 route priority constants](https://github.com/torvalds/linux/blob/v6.12/include/uapi/linux/ipv6_route.h)

Native namespace tests remain separate from portable compiler and simulator
tests. Cross-compilation is not evidence of successful kernel application.
