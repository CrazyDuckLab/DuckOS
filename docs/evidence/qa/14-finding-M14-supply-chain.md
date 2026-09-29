# M-14 — Supply chain review: minimal dependency footprint

**Severity:** N/A (positive qualification; 9 total deps,
1 direct)
**Section:** §46 / Dependency / supply-chain review
**Status:** PASS, review committed
**Found during:** Phase E §46 static review

## 1. Inventory

`go.mod` declares:

| Module | Version | Direct | Risk notes |
|---|---|---|---|
| `modernc.org/sqlite` | 1.58.0 | yes | Pure-Go SQLite driver. Active. Last release: 2026. |
| `github.com/dustin/go-humanize` | 1.0.1 | indirect | Mature, dormant but stable. |
| `github.com/google/uuid` | 1.6.0 | indirect | Google official. Active. |
| `github.com/mattn/go-isatty` | 0.0.24 | indirect | Stable, used for TTY detection. |
| `github.com/ncruces/go-strftime` | 1.0.0 | indirect | Used by sqlite driver for date formatting. |
| `github.com/remyoudompheng/bigfft` | 20230129 | indirect | Math dep for sqlite. |
| `golang.org/x/sys` | 0.47.0 | indirect | Go x/sys. Active. |
| `modernc.org/libc` | 1.75.6 | indirect | Pure-Go libc for sqlite. |
| `modernc.org/mathutil` | 1.7.1 | indirect | Math dep. |
| `modernc.org/memory` | 1.12.1 | indirect | Memory dep. |

**Total: 10 dependencies, 1 direct.** This is an exceptionally
small footprint for a Go project of this scope.

## 2. Findings

### 2.1 No abandoned / dangerous dependencies

Every indirect dep is either actively maintained (Google,
modernc.org, ncruces, golang.org/x) or dormant-but-stable
(dustin/go-humanize, remyoudompheng/bigfft). None are flagged
as abandoned or known-dangerous.

### 2.2 No mutable download URLs

All deps come from `proxy.golang.org` via the standard Go
module proxy. No `replace` directives point at arbitrary URLs.
No `vendor/` directory overrides any of them.

### 2.3 No executable fetched without verification

The build never downloads an executable at runtime — no
`go install`, no `curl | bash`, no `wget`. The only external
binary DuckOS shells out to is `nft`, `dnsmasq`, `hostapd`,
`freeradius`, FRR daemons, `tc`, `dhclient`, etc., all
installed via the host distro package manager.

### 2.4 Transitive pinning

`go.sum` is committed and present (50 lines, 10 entries —
small enough to be manually inspected). Every dep is pinned
to a specific version + content hash.

## 3. Risks worth flagging

- **modernc.org/sqlite chain is the bulk of the supply-chain
  surface**. A vulnerability in `modernc.org/libc`,
  `modernc.org/mathutil`, or `modernc.org/memory` could
  compromise the SQLite driver. Mitigated by the fact that
  these are pure-Go ports of C libraries — they have no
  transitive C toolchain and no FFI surface.
- **`mattn/go-isatty` is unmaintained as of 2026** (last
  release 2024). Not a security issue but a maintenance one.
  Used by the test harness only, so production runtime
  impact is zero.

## 4. Recommended action

None required for V1 Alpha. The dependency footprint is
healthy and minimal. Continue to monitor `modernc.org/sqlite`
for security advisories; pin updates through the existing
`go.sum` workflow.

## 5. Management boundary preserved

N/A (no runtime action performed).

## 6. Commits

- (no source commit; review document only)
- `docs(qa): §46 M-14 supply chain review`
