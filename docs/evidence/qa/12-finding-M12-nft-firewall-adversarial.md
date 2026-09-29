# M-12 — Real nftables rejects 12 categories of malformed config

**Severity:** N/A (positive qualification; baseline for the
DuckOS-side firewall validator that does not yet exist)
**Section:** §9 / Firewall adversarial testing
**Status:** PASS, regression coverage landed
**Found during:** Phase E §9 hostile qualification on duck-dev-01

## 1. Affected boundary

DuckOS will eventually expose a firewall validator (the
`internal/nftables` package does not exist yet — tracked in
the §52 gap matrix). The validator must NOT silently accept
a config that the kernel nft would reject. This test pins the
kernel-side contract that any DuckOS-side validator must at
minimum enforce.

## 2. Infra defects found and fixed in the test harness

### 2.1 Trailing `; true` masked nft's exit code

`nftTryApply` originally ran `nft -f path 2>&1; true` so we
could always capture output. The trailing `; true` meant the
shell exit code was always 0, so the test reported "nft
accepted" for every malformed config. Replaced with a direct
`runInNSCaptureBytes(ns, nftBin, "-f", path)` call so the
real nft exit code propagates.

### 2.2 Stderr was discarded

`runInNSCapture` (used by §3 FRR tests) returns only stdout.
nft reports parse errors to stderr. Switched §9 to
`runInNSCaptureBytes` which returns (stdout, stderr, error)
and concatenates both for the assertion.

### 2.3 Test case "no_hook_specified" was actually valid

A chain without a `type ... hook ...` block is valid in nft
1.1.x — it can still be a target of `jump`. Replaced with
a chain whose `hook` name is invalid (`hook invalid_hook_name`)
which nft must reject.

### 2.4 Test case "nat_hook_on_filter_chain" had wrong ruleset

The ruleset had `type nat` (not `type filter`). Replaced with
a real `snat` statement inside a `type filter hook input`
chain, which is a type/operation mismatch nft must reject.

## 3. Linux qualification evidence

duck-dev-01, nftables 1.1.3. All 12 cases rejected with a
recognizable error string:

```
ok  github.com/CrazyDuckLab/DuckOS/internal/qa  0.047s
```

Categories tested (each rejects with a clear error message,
non-zero exit code):

1. **empty_groups** — chain with no body
2. **duplicate_chain_name** — same chain name twice in one table
3. **invalid_prefix** — `999.999.999.0/24`
4. **inverted_port_range** — `tcp dport 1000-100`
5. **impossible_port_zero** — `tcp dport 0`
6. **impossible_port_65536** — `tcp dport 65536`
7. **tcp_with_udp_only_field** — `tcp dport 22 udp length 100`
8. **jump_to_nonexistent_chain** — `jump z` where `z` is undefined
9. **nat_hook_on_filter_chain** — `snat` in `type filter hook input`
10. **no_hook_specified** — replaced with `hook invalid_hook_name`
11. **negative_prefix_len** — `10.0.0.0/-1`
12. **huge_priority_overflow** — `priority 999999999999999999999999`

## 4. Management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 5. Implications for the future DuckOS validator

When `internal/nftables.Render` ships, every emit path should
re-run this §9 test against the rendered config. A regression
in the renderer that produces any of these 12 patterns MUST
be caught before the rendered config reaches the kernel.

## 6. Commits

- `feat(qa): §9 firewall adversarial - nft rejects 12 malformed configs`
- `fix(qa): §9 nftTryApply capture real exit code (no '; true' mask)`
- `fix(qa): §9 nftTryApply captures stderr (parse errors go there)`
- `fix(qa): §9 fix two ruleset cases (nat/filter mismatch + invalid hook)`
- `fix(qa): §9 use 999999... priority (definitely overflows int parser)`
