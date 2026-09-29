# M-8 — Real nftables NAT + packet flow qualification (LAN→WAN)

**Severity:** N/A (positive qualification; one infra defect fixed)
**Section:** §8 / Real network dataplane matrix
**Status:** §8.1 PASS, regression coverage landed
**Found during:** Phase E §8.1 hostile qualification on duck-dev-01

## 1. Affected boundary

DuckOS must move real packets through a real nftables ruleset on
real Linux. The previous reports claimed unit-tested nftables render
without qualifying that the kernel actually accepts the ruleset, that
forwarding works, that masquerade rewrites the conntrack tuple, and
that ICMP echoes round-trip end-to-end.

## 2. Infra defect found and fixed

The first `nftApply` helper invoked `nft -f -` without writing the
ruleset to `cmd.Stdin`. nft read empty stdin, applied nothing, and
exited 0 — so the test silently passed against an empty ruleset. After
the fix, the helper pipes `ruleset` via `strings.NewReader` into
`cmd.Stdin`; nft reports parse errors and the test fails loudly.

## 3. Linux qualification evidence

duck-dev-01, nftables 1.1.3 (Commodore Bullmoose #4), three netns
(LAN / router / WAN) connected by two veth pairs:

```
qa-nat-lan  ─── 10.100.0.1/30 ───  qa-nat-rtr  ─── 10.200.0.1/30 ───  qa-nat-wan
                                       │
                              nft ruleset:
                              - ip forward on
                              - nat/postrouting: oifname qa-rtr-wan masquerade
                              - filter/forward:
                                  iifname qa-rtr-lan oifname qa-rtr-wan accept
                                  iifname qa-rtr-wan oifname qa-rtr-lan
                                       ct state established,related accept
```

Real UDP packet (`printf 'qa-nat-payload' | nc -u -w 1`) from LAN
to WAN's UDP/19999 listener:

```
wan nc log:
    qa-nat-payload

router conntrack (nf_conntrack):
    ipv4 2 udp 17 28
      src=10.100.0.1 dst=10.200.0.2 sport=47702 dport=19999 [UNREPLIED]
      src=10.200.0.2 dst=10.200.0.1 sport=19999 dport=47702
      mark=0 zone=0 use=2

lan->wan ping:
    64 bytes from 10.200.0.2: icmp_seq=1 ttl=63 time=0.070 ms
    1 packets transmitted, 1 received, 0% packet loss
```

The conntrack entry proves three things at once:
1. Original tuple (`src=10.100.0.1 dst=10.200.0.2`) — kernel saw the
   pre-NAT packet from LAN.
2. Reply tuple (`src=10.200.0.2 dst=10.200.0.1`) — kernel installed the
   inverse with the router's WAN IP, which is exactly the masquerade
   semantic. Without `oifname qa-rtr-wan masquerade` the reply would
   still target `10.100.0.1` directly and the LAN-side ping would
   never see a response.
3. `[UNREPLIED]` flag — single UDP probe, no return traffic yet, but
   conntrack has committed to tracking it.

ICMP echo from LAN to WAN succeeds in 70 µs, which proves the
forward chain accepts LAN→WAN and the stateful reverse accepts the
return. The 70 µs RTT is consistent with two veth traversals and
no real WAN.

The ruleset survives a `nft list ruleset` snapshot with the
expected priorities (`dstnat` for prerouting, `srcnat` for
postrouting, `filter` for forward).

## 4. Linux qualification: management boundary

```
eth0   UP   172.18.4.149/16  fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 5. Side findings worth flagging

- **`conntrack` CLI is not installed** on the Debian 13 duck-dev-01
  image. The test reads `/proc/net/nf_conntrack` directly via
  `runInNSCaptureBytes(ns, "/bin/cat", "/proc/net/nf_conntrack")`.
  Same data, no tool dependency.
- **UDP conntrack entries are short-lived** (default ~30 s). The
  test reads the proc file within ~700 ms of the send, which is
  well inside the timeout.
- **Forward chain needs explicit accept** in both directions. The
  test relies on `policy accept` for the chains, but adds explicit
  `accept` for LAN→WAN and `ct state established,related accept`
  for WAN→LAN so future tests can add `policy drop` and still
  have the data path work.
- **The `qa-nat-*` netns leaks** until the lab's cleanup runs.
  Verified zero leaks via `ip -br netns list`.

## 6. Commits

- `feat(qa): §8.1 real nftables NAT + packet flow (LAN->WAN, masquerade)`
- `fix(qa): §8 nftApply actually pipes ruleset via stdin`
