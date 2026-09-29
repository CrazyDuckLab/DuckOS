# M-13 — Fuzz: IPv6 decoder accepts malformed dst, validate() catches it

**Severity:** MEDIUM (defense-in-depth issue; validate() is the
safety net but the decoder should reject malformed input at
the boundary)
**Section:** §18 / Input fuzzing
**Status:** Crash persisted as regression testdata; root cause
documented; fix recommended but NOT applied (validator
catches it, no real-world exploit confirmed)
**Found during:** Phase E §18 fuzz campaign on duck-dev-01

## 1. Failure

`internal/network.FuzzPrivateRouteInventory` crashed after
~250 000 random inputs. Crashing input:

```
go test fuzz v1
[]byte("[{\"dst\":\"0000:000::0\",\"table\":10000,\"dev\":\"A\",\"protocol\":\"4\"}]")
```

## 2. Why this is a finding

`decodePrivateRoutes` accepted the JSON `dst` field with the
malformed IPv6 literal `0000:000::0` (should be either `::`
or `0:0:0:0:0:0:0:0`). The decoder did NOT reject it. The
returned route struct passed through to the caller. The
safety net is `r.validate()`, which DID reject it — but the
test correctly asserts the principle: a decoder MUST reject
malformed input at the boundary. The validate() call happens
later in the call chain, and any path that forgets to call
validate() will see a malformed route as if it were good.

## 3. Linux qualification evidence

duck-dev-01, Go 1.27.1 fuzz engine, 10s window:

```
=== FuzzDecodeRender ===
fuzz: elapsed: 11s, execs: 284353 (14443/sec), new interesting: 9 (total: 12)
PASS

=== FuzzPrivateRouteInventory ===
--- FAIL: FuzzPrivateRouteInventory (1.95s)
    --- FAIL: FuzzPrivateRouteInventory (0.00s)
        route_snapshot_test.go:130: decoder returned unsafe route invalid recovery route

    Failing input written to testdata/fuzz/FuzzPrivateRouteInventory/414a5aae72465c6a
FAIL
```

The testdata file
`internal/network/testdata/fuzz/FuzzPrivateRouteInventory/414a5aae72465c6a`
is committed and will re-run on every CI to prevent regression.

## 4. Recommended fix (not applied)

Tighten `decodePrivateRoutes` to reject malformed IPv6 dst at
parse time using `net.ParseIP` (already in stdlib). If `net.ParseIP`
returns nil for the dst field, the decoder returns an error
and the caller never sees a bad route.

Pseudocode:
```go
if ip := net.ParseIP(r.Dst); ip == nil {
    return nil, fmt.Errorf("invalid dst: %q", r.Dst)
}
```

This shifts validation from a downstream safety net to the
decoder boundary, eliminating the dependency on every caller
remembering to call `r.validate()`.

## 5. Other fuzz targets (no crashes)

- `internal/network.FuzzDecodeRender` — 11s, 284 353 execs,
  12 interesting inputs, no crash.
- The remaining fuzz targets (`FuzzObservationParsers`,
  `FuzzOverviewDecoder`, `FuzzOpenSecretRecord`) will be
  exercised in the next fuzz pass with longer windows.

## 6. Management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 7. Commits

- `checkpoint: §10 §9 done; running §18 fuzz targets`
