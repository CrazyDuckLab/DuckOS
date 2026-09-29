# M-9 — Real nftables hairpin NAT: NAT rule ordering matters

**Severity:** MEDIUM (config-render pitfall; no security boundary breach,
but a hairpinned service silently fails when NAT rules are ordered
incorrectly)
**Section:** §8.2 / Real network dataplane matrix
**Status:** FIXED, regression test PASS on duck-dev-01
**Found during:** Phase E §8.2 hostile qualification

## 1. Attack precondition / affected boundary

When DuckOS exposes a LAN-side service behind a router-published WAN
IP (the classic hairpin / NAT-loopback scenario), the nftables
postrouting chain must apply a hairpin-specific SNAT BEFORE the
generic `masquerade` rule. Otherwise the generic rule rewrites the
source first, and the more specific SNAT never fires because its
`saddr` match is no longer true.

## 2. Impact

A LAN host trying to reach a published LAN service by the WAN IP
(typical for split-horizon DNS, captive-portal logins, or
self-routed traffic when the admin is on VPN) would either:

- never receive a reply (the reply targets the LAN host's original
  IP, which is unreachable from the LAN-side server), or
- receive a reply from an unexpected source (the unmodified WAN
  service IP), which the LAN host's kernel may treat as a martian
  and drop silently.

Both failure modes are invisible to the LAN user; the symptom is a
service that "works from outside, broken from inside".

## 3. Root cause

nftables evaluates postrouting rules in declaration order. The
generic `oifname X masquerade` rule matches first, rewrites the
source to the outgoing interface's primary address (10.210.0.1 in
this lab), and the subsequent `ip saddr 10.110.0.1 ... snat to
10.110.0.2` rule can no longer match because `saddr` is no longer
10.110.0.1.

Empirical verification on duck-dev-01:

```
table ip nat {
  chain postrouting {
    oifname "qa-hp-rtr-wan" masquerade     # counter packets 1 (matched first)
    oifname "qa-hp-rtr-wan" ip saddr 10.110.0.1 ip daddr 10.210.0.2 snat to 10.110.0.2  # counter packets 0 (never matched)
  }
}
```

After swapping the rule order (SNAT first, masquerade second):

```
table ip nat {
  chain postrouting {
    oifname "qa-hp-rtr-wan" ip saddr 10.110.0.1 ip daddr 10.210.0.2 snat to 10.110.0.2  # counter packets 1
    oifname "qa-hp-rtr-wan" masquerade     # counter packets 0 (hairpin matched first, nothing left for masquerade)
  }
}
```

## 4. Fix

The `internal/qa/nftables_nat_linux_test.go` hairpin ruleset now
declares the SNAT first, the masquerade second. The production
`internal/nftables` package (when it grows a hairpin builder)
should emit rules in the same order.

## 5. Linux qualification evidence

duck-dev-01, nftables 1.1.3, three netns (LAN 10.110.0.1/30 /
router / WAN 10.210.0.1/30). LAN dials the router's WAN IP
(10.210.0.1) on UDP/29999, which is hairpin-DNAT'd to the WAN
service at 10.210.0.2.

```
=== RUN   TestNftablesNATHairpin
    wan service log:
        qa-hairpin-payload
    router conntrack (hairpin):
        ipv4 2 udp 17 28
          src=10.110.0.1 dst=10.210.0.1 sport=41670 dport=29999 [UNREPLIED]
          src=10.210.0.2 dst=10.110.0.2 sport=29999 dport=41670
          mark=0 zone=0 use=2
--- PASS: TestNftablesNATHairpin (2.08s)
```

Key facts:

1. **WAN service received `qa-hairpin-payload`** — the DNAT worked
   (10.210.0.1 → 10.210.0.2).
2. **Reply tuple destination is `10.110.0.2`** (router's LAN IP),
   not `10.110.0.1` (LAN host's IP) — proving the hairpin SNAT
   rewrote the source on the forward path.
3. **`[UNREPLIED]`** because nc closed before the WAN service
   could reply (UDP without reply injection). The conntrack entry
   itself is the proof.

## 6. Management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 7. Recommendation for `internal/nftables`

When the production package grows a hairpin builder, it MUST
document and enforce rule ordering: any explicit per-flow SNAT for
hairpin must precede the generic `masquerade` rule in the same
chain. A simple invariant check (postrouting rules containing
`snat to` must appear before any `masquerade`) would catch this at
validation time.

## 8. Commits

- `feat(qa): §8.2 real nftables hairpin NAT (LAN dials router's WAN IP)`
- `fix(qa): §8.2 hairpin SNAT must precede masquerade in postrouting`
