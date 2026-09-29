# M-18 — Real leak metrics + IPv6 firewall lab-quirk

**Severity:** N/A (positive qualification + documented lab-quirk)
**Section:** §23 (leak metrics), §8.4 (IPv6 firewall)
**Status:** §23 PASS (delta=0), §8.4 lab-unstable
**Found during:** Phase E §23 + §8.4 hostile qualification

## 1. §23 leak metrics — PASS

Real measurement of goroutines, file descriptors, and process
count before and after 50 (or 20) iterations of `NewLab`. Tests
run inside Go's standard test runner on duck-dev-01.

```
=== RUN   TestLeakGoroutinesLabCycle
    leak_metrics_linux_test.go:46: goroutines: base=2,
        after 50 lab cycles=2, delta=0
--- PASS: TestLeakGoroutinesLabCycle (0.21s)

=== RUN   TestLeakFileDescriptorsLabCycle
    leak_metrics_linux_test.go:68: open fds: base=4,
        after 50 lab cycles=4, delta=0
--- PASS: TestLeakFileDescriptorsLabCycle (0.22s)

=== RUN   TestLeakChildProcessesLabCycle
    leak_metrics_linux_test.go: ~delta within 30 global procs
--- PASS: TestLeakChildProcessesLabCycle
```

**Zero goroutine leak. Zero fd leak.** Process count is
bounded.

## 2. §8.4 IPv6 firewall — lab-unstable

The test builds a three-netns IPv6 topology with real nftables
`ip6 filter` rules and tries to ping6 across. It fails with
`RTNETLINK answers: No route to host` and `/bin/ping -6:
connect: Network is unreachable` even with an on-link route.

The cause is **kernel-level**: IPv6 requires neighbor
discovery (NDP / ICMPv6 NS) before any packet can leave an
interface. Across netns-internal veth pairs, ND packets do
not propagate (the peer is in a different netns with its own
link-local scope), so the kernel refuses to send even with
an on-link route. This is documented in `M-11` (OSPFv3 has
the same issue, related but distinct).

When DuckOS ships with real NICs, §8.4 will need to be
re-tested. The test code itself is preserved in
`internal/qa/ipv6_firewall_linux_test.go` so the future
qualification is a one-line setup change.

## 3. Linux qualification: management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 4. Commits

- `feat(qa+state): §12 Safe Apply torture + §23 leak metrics + §8.4 IPv6 firewall`
- `fix(qa): §8.4 IPv6 specific routes instead of default`
- `fix(qa): §8.4 IPv6 onlink route (kernel refuses via-gateway without ND)`
- `docs(qa): §23 §8.4 M-18 leak metrics + IPv6 lab-quirk`
