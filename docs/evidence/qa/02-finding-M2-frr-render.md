# QA Finding M-2: FRR render path does not produce a real-runtime-valid config

**Severity:** High (the existing `frr.Render()` cannot drive a real FRR
daemon without operator hand-fixing the generated config)
**Section:** §3 (Real FRR dataplane — no more render-only claims) +
§48 (negative tests for routing config)
**Status:** Real dataplane test suite added; the `frr.Render()` path
itself is documented as still model-only, with a concrete patch
proposal.

## 1. Symptom

`internal/frr/frr.go::Render()` produces a syntactically valid FRR
configuration, but the output cannot be loaded by a real FRR 10.3
daemon without manual repair. Two specific defects were discovered
while running the §3 dataplane qualification on duck-dev-01:

### 1.1 Hard-coded neighbor name in `maximum-prefix`

```frr
router bgp 65001
 neighbor X maximum-prefix 100
```

The `Neighbor` is referenced as the literal string `X` regardless of
the actual neighbor address. FRR rejects this with "neighbor
configuration error".

### 1.2 No implicit permit route-maps

In FRR 10.3 the BGP / OSPF daemons silently drop both inbound and
outbound updates with the message:

```
Outbound updates discarded due to missing policy
Inbound  updates discarded due to missing policy
```

unless a route-map is applied to the neighbor (BGP) or the
redistribute command (OSPF). The `frr.Render()` path does not emit
any `route-map … permit …` block, so a config produced by
`Render()` opens the BGP session and reaches `Established` but
never carries prefixes. This is a silent no-op at runtime and a
false green at unit-test time.

## 2. Minimal reproduction (real binary)

The dataplane test in `internal/qa/frr_bgp_linux_test.go` was
written against a hand-crafted config; the very first attempt to
substitute it with `frr.Render()` output failed because of the
two defects above. The hand-crafted config is what is checked into
the lab, NOT `frr.Render()`. The `frr` package itself is still
useful as a typed model but should be advertised as such.

## 3. Root cause

- `renderBGP` (internal/frr/frr.go) hard-codes the neighbor
  identifier in the `maximum-prefix` line. This was likely a
  copy-paste from a single-peer example.
- The render contract does not include route-map generation. The
  model has no place to declare a per-neighbor route-map and no
  default emit at render time.

## 4. Impact

A naïve DuckOS deployment that ships the rendered config to FRR
will:

- fail to load (defect 1.1)
- or load but never advertise any prefix (defect 1.2)
- while the unit tests on `frr.Render()` will pass, because they
  only check the rendered string against expected substrings.

This is a textbook "renders, does not run" failure that the §3
section of this program was written to catch.

## 5. Fix proposal (deferred from this commit)

1. Track the per-neighbor address in `renderBGP` and emit
   `neighbor <addr> maximum-prefix <N>` instead of `X`.
2. Introduce an explicit `RouteMap` field in the BGP / OSPF model
   types and a default `qa-permit-all` block in the rendered
   config, so operators do not have to hand-write one.
3. Extend the unit test on `frr.Render()` to assert that the
   output, when written to disk, is acceptable to
   `vtysh -C -f <file>` (dry-run) and to a real bgpd / ospfd
   (daemonized) in the lab.

## 6. Evidence captured

- `internal/qa/frr_bgp_linux_test.go::TestFRRRealEBGPDataplane` —
  PASS on duck-dev-01. Two isolated netns, real zebra + bgpd
  daemons (FRR 10.3), eBGP session, 10.10.10.0/24 advertised from
  Lab A and installed in Lab B's kernel route table. Withdrawal
  via interface-down + `clear ip bgp * soft out` removes the
  route.
- `internal/qa/frr_bgp_linux_test.go::TestFRRRealOSPFv2Dataplane` —
  PASS on duck-dev-01. Two isolated netns, real zebra + ospfd
  daemons, point-to-point OSPFv2 over the veth, 10.251.100.0/24
  advertised by Lab A installed in Lab B's kernel route table.
- `internal/qa/lab_linux_test.go::TestLabNetnsCreateAndTeardown` —
  PASS. Netns + veth + management-boundary assertion.

## 7. Artifacts

- `internal/qa/lab_linux.go` + `lab_other.go` + `lab_types.go`
- `internal/qa/lab_linux_test.go`
- `internal/qa/frr_bgp_linux_test.go`
- `internal/qa/secretcanary.go` + `secretcanary_test.go`
- `internal/qa/support_canary_test.go`
- this document
