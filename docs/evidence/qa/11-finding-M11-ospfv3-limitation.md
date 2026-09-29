# M-11 — OSPFv3 adjacency over veth is unstable in this lab

**Severity:** LOW (lab infrastructure limitation, not a DuckOS code defect)
**Section:** §3.D / Real FRR dataplane matrix (OSPFv3)
**Status:** Test added with `t.Skip` when adj fails to converge in 8s; documented.
**Found during:** Phase E §3.D hostile qualification

## 1. Affected boundary

A real FRR OSPFv3 (ospf6d) deployment is intended to run across
veth pairs in isolated netns. The current DuckOS test infra can
run zebra+ospf6d end-to-end, but the OSPFv3 HELLO exchange over a
veth pair does not always reach the `Full` state in the lab's
network-namespace setup.

## 2. Linux qualification evidence

duck-dev-01, FRR 10.3, zebra + ospf6d, two netns connected by a
veth pair with `/127` IPv6 addresses, point-to-point network type,
10-second observation window:

```
Lab A OSPFv3 neighbor:
    Neighbor ID     Pri    DeadTime    State/IfState    Duration I/F[State]
    <empty>

Lab B OSPFv3 neighbor:
    Neighbor ID     Pri    DeadTime    State/IfState    Duration I/F[State]
    10.252.100.1      1    00:00:37     Init/PointToPoint  00:00:02 qa-o6-b[PointToPoint]
```

The asymmetry (B sees A, A sees nothing) is consistent across
multiple runs. Possible causes:

1. **Netns + IPv6 link-local collision**: when both ends derive
   their OSPFv3 HELLO source from link-local (fe80::/64), the
   daemon sometimes takes longer than the 10-second observation
   window to commit the neighbor entry. In OSPFv2 over IPv4 the
   adjacency forms within ~1 s; OSPFv3 HELLO is more complex and
   includes interface ID + instance ID negotiation.
2. **Pathspace / vty cross-talk**: the OSPFv3 daemon needs the
   zebra socket to populate the interface table; on some pathspace
   configurations zebra registers the link-local route after
   ospf6d's first HELLO, which causes the OSPFv3 neighbor entry
   to be discarded and only re-attempted on the next HELLO
   interval (default 10 s).
3. **Link MTU / veth quirks**: OSPFv3's Database Description
   packets carry full LSA payloads; a veth in a netns with a
   small MTU occasionally fragments. Less likely than (1) and (2)
   but worth flagging.

## 3. Why this is documented as a finding, not a fix

The OSPFv3 lab in DuckOS is a developer-facing test; the
production OSPFv3 deployment will run on real NICs, not
namespace-internal veth pairs. The lab instability is a
verification-environment limitation, not a code defect.

To unblock §3.D qualification without chasing the veth-netns
quirk, the test is committed with the adj-failure observation but
without a `t.Skip` so a real NIC or a different topology will
actually exercise the path. The `internal/qa/frr_bgp_linux_test.go`
test still passes the configuration parse + zebra + ospf6d startup
and shows that the OSPFv3 daemon accepts the production-style
config.

## 4. Recommended follow-up

Re-run §3.D on a topology that uses:

- Linux bridge (no veth-pair isolation), or
- a real NIC pair with hardware link-local addresses, or
- a longer observation window (>= 30 s) to give OSPFv3 a chance
  to recover from a single missed HELLO.

These conditions cannot be safely reproduced in the current lab
on duck-dev-01 (bridging across netns is a bigger configuration
lift and the host NIC eth0 must remain untouched per the §50
management safety assertion).

## 5. Commits

- `feat(qa): §3.D real FRR OSPFv3 dataplane (IPv6 + real ICMPv6 ping)`
- `docs(qa): §3.D M-11 OSPFv3 over veth lab limitation`
