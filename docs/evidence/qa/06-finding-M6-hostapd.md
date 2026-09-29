# M-6 — `internal/hostapd.Render` emits trailing `!` rejected by hostapd 2.10

**Severity:** MEDIUM
**Section:** §7 / hostapd real-binary qualification
**Status:** FIXED, regression test landed
**Found during:** Phase E §7.1 hostile pass on duck-dev-01 with real hostapd 2.10

## 1. Attack precondition / affected boundary

A DuckOS-generated `hostapd.conf` is loaded by real hostapd 2.10 in
production. The renderer must produce config that real hostapd accepts,
not just configs that pass our own validator.

## 2. Impact

Without the fix, every BSS configuration DuckOS renders for hostapd 2.10
fails at startup with `Line N: invalid line '!'` regardless of whether
the security / channel / band directives are correct. The radio never
comes up. The bug affected every multi-BSS **and** single-BSS render.

## 3. Reproduction (before fix)

`internal/hostapd.Render` ended every BSS block, including the last,
with the `!` BSS separator. Real hostapd 2.10 emits:

```
Configuration file: .../hostapd.conf
Line 12: invalid line '!'
1 errors found in configuration file '...'
Failed to set up interface with .../hostapd.conf
```

The first `internal/qa/hostapd_linux_test.go` assertion passed despite
the rejection because it used substring matching on the word
"interface" — which also appears in the error message. So the test was
green while production was broken. M-6 includes hardening of §7.1's
assertion to use exit status + ` OK` substring.

## 4. Root cause

`internal/hostapd/hostapd.go:Render` unconditionally appended `!\n`
after every BSS iteration. hostapd's grammar treats `!` as a BSS
separator that must NOT appear after the final BSS block; hostapd 2.10
explicitly rejects it there.

## 5. Fix

`internal/hostapd/hostapd.go`:

```go
// '!' is the hostapd BSS separator. hostapd 2.10 rejects
// a trailing '!' after the last BSS block ("invalid line '!'"),
// so emit it only between blocks, never after the final one.
if i+1 < len(bsses) {
    sb.WriteString("!\n")
}
```

The loop now uses `for i, b := range bsses` so the index is available.

## 6. Regression coverage

`internal/qa/hostapd_linux_test.go`:

- `TestHostapdRenderRealBinaryValidation` — runs real hostapd 2.10
  with `driver=none`, 4-second context timeout. Asserts the daemon
  reaches `AP-ENABLED` with no "errors found" line. Now uses
  classification (`accepted`/`rejected`/`hanged`) instead of
  substring matching.
- `TestHostapdRefusesInvalidWPA` — runs real hostapd 2.10 against a
  raw `hostapd.conf` with WPA-PSK but no passphrase, asserts the
  daemon rejects with a diagnostic mentioning `passphrase`/`password`/
  `PSK`. Proves the **daemon**, not just our model, gates the
  invalid config.

## 7. Linux qualification evidence

duck-dev-01, hostapd 2.10:

```
=== RUN   TestHostapdRenderRealBinaryValidation
    hostapd output:
        Configuration file: .../hostapd.conf
        Completing interface initialization
        hostapd_setup_bss(hapd=... (wlan0), first=1)
        Deriving WPA PSK based on passphrase
        SSID - hexdump_ascii(len=7): qa-test
        random: Got 20/20 random bytes
        wlan0: interface state UNINITIALIZED->ENABLED
        wlan0: AP-ENABLED
        wlan0: Setup of interface done.
--- PASS: TestHostapdRenderRealBinaryValidation (4.00s)

=== RUN   TestHostapdRefusesInvalidWPA
    hostapd output:
        WPA-PSK enabled, but PSK or passphrase is not configured.
        1 errors found in configuration file '.../hostapd-raw.conf'
        Failed to set up interface with .../hostapd-raw.conf
--- PASS: TestHostapdRefusesInvalidWPA (0.00s)
PASS
ok  github.com/CrazyDuckLab/DuckOS/internal/qa  4.010s
```

Management boundary preserved: `eth0 UP 172.18.4.149/16`,
`default via 172.18.1.254 dev eth0`, `sshd pid 758 alive`.

## 8. Side findings worth flagging

- **hostapd `-t` is not a test mode.** In hostapd 2.10, `-t` only
  enables timestamps in debug output. There is no built-in
  parse-and-exit. Tests must use `driver=none` + context timeout,
  or `-S` synchronous mode in a real environment.
- **Substring assertions are a trap.** The original §7.1 assertion
  was passing on rejection because the word "interface" appeared in
  the error. Replaced with status + classification-based check.
- **`driver=none` is the right validation harness for CI.** It
  exercises every directive (security mode, BSSes, channels, PMF,
  VLAN, cipher, etc.) without requiring a Wi-Fi NIC.

## 9. Commits

- `fix(qa+hostapd): §7.1 stricter assert + drop trailing '!' BSS separator`
- `fix(qa): §7 hostapd uses driver=none (no radio NIC)`
- `fix(qa): §7 hostapd -t is timestamps only; use context timeout`
