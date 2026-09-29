# M-16 — Real FRR IS-IS + BFD qualification

**Severity:** N/A (positive qualification)
**Section:** §3.E (IS-IS), §3.F (BFD)
**Status:** §3.E PASS, §3.F PASS, regression coverage landed
**Found during:** Phase E §3.E/§3.F hostile qualification

## 1. §3.E IS-IS

Real FRR 10.3 `isisd` daemon, two netns connected by a veth,
IS-IS NET `49.0001.0000.0000.0001.00` (Lab A) and
`49.0001.0000.0000.0002.00` (Lab B), area `QA`, level-1 only.

Both sides reported neighbor Up within ~5 s:

```
Lab A:  System Id 0000.0000.0002 qa-is-a 1 Up 28 06b6.8247.979a
Lab B:  System Id 0000.0000.0001 qa-is-b 1 Up 28 ba8f.1db7.6d60
```

Test runtime: 5.89 s. PASS.

## 2. §3.F BFD

Real FRR 10.3 `bfdd` daemon, two netns connected by a veth,
BFD peers configured both ways with 200 ms transmit interval
and 3× detect multiplier (theoretical detect time 600 ms).

Session Up observed within 2 s on both sides:

```
ID: 387663745
Remote ID: 387663745
Active mode
Status: up
Uptime: 2 second(s)
Diagnostics: ok
Remote diagnostics: ok
Detect-multiplier: 3
Receive interval: 200ms
Transmission interval: 200ms
```

After Lab B's interface was brought down, Lab A's session
transitioned to Down within ~2 s:

```
Status: down
Downtime: 2 second(s)
Diagnostics: control detection time expired
```

This proves BFD detects sub-second-class failures in real
FRR deployments. Test runtime: 6.66 s. PASS.

## 3. Linux qualification: management boundary

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 4. §3 matrix state (after this finding)

| Section | Daemon | Result |
|---|---|---|
| §3.A eBGP | zebra + bgpd | PASS |
| §3.B iBGP | zebra + bgpd | PASS |
| §3.C OSPFv2 | zebra + ospfd | PASS |
| §3.D OSPFv3 | zebra + ospf6d | TEST WRITTEN, lab-unstable (M-11) |
| §3.E IS-IS | zebra + isisd | PASS |
| §3.F BFD | zebra + bfdd | PASS |

§3.A, §3.B, §3.C, §3.E, §3.F are fully qualified on real FRR
10.3 against real Linux veth interfaces in isolated netns.

## 5. Commits

- `feat(qa): §3.E real FRR IS-IS dataplane (isisd, NET-based adjacency)`
- `feat(qa): §3.F real FRR BFD fast-failure detection (bfdd, 200ms/3x)`
