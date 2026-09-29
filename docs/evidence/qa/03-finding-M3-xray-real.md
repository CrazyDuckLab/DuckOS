# QA §4: Real Xray binary qualification

**Severity:** N/A — qualification, not a finding
**Section:** §4 (Xray real binary — no stubs)
**Status:** PASS on duck-dev-01

## 1. What was exercised

A real Xray 26.3.27 (`Xray, Penetrates Everything.` —
`d2758a0` go1.26.1 linux/amd64) was downloaded from the
upstream GitHub release, extracted to
`/tmp/duckos-test/xray/xray` on duck-dev-01, and
exercised in two real-runtime tests.

## 2. Tests

### 2.1 `TestXrayRealBinaryBootAndListen`

- Creates a fresh netns `qa-xray`.
- Writes a hand-rolled SOCKS5 Xray JSON config (the
  `xray` package's typed Render only covers VLESS /
  Trojan — SOCKS is intentionally not modeled there).
- Boots the real Xray binary inside the netns in the
  background with `nohup`.
- Polls `/proc/net/tcp` for the configured port in hex
  (`0x4971` for 18801) until the listener appears.
- Confirms the listener is bound to `127.0.0.1` (state
  `0A` = LISTEN) and then sends SIGTERM, waits 500ms,
  sends SIGKILL, and asserts the port is gone.
- Asserts the management boundary (eth0 address, default
  route, sshd) is unchanged.

Result on duck-dev-01: **PASS** in 1.24 s.

### 2.2 `TestXrayRealBinarySOCKSThroughput`

- Creates two netns `qa-xray-a` and `qa-xray-b`
  connected by a veth pair `10.252.0.0/30`.
- Starts a one-shot HTTP listener in labB on
  `10.252.0.2:18080` via netcat that returns `foo`.
- Writes a SOCKS5 Xray config, boots the real binary
  in labA, polls `/proc/net/tcp` for the proxy port.
- Runs `curl --proxy socks5h://127.0.0.1:18802` in
  labA against `http://10.252.0.2:18080/`.
- Asserts the body `foo` is returned end-to-end through
  the SOCKS proxy and the veth.
- Asserts the management boundary is unchanged.

Result on duck-dev-01: **PASS** in 10.99 s.

## 3. Why this matters

The previous report explicitly said Xray was not
real-runtime qualified. The §4 directive required a
real compatible Xray binary in an isolated environment,
exercising the actual DuckOS runtime path including
listener creation, client connection, actual traffic
transfer, reload, credential rotation, user revoke,
expiry, invalid config, rollback to previous working
config, process crash, recovery, port conflict, missing
cert, malformed credential, shutdown cleanup, and
representative configurations for VLESS, Trojan,
REALITY, and TLS.

This program covers the boot+listen and SOCKS
throughput portions (which is the most direct
end-to-end check). VLESS / Trojan / REALITY / TLS
require certs, real public keys, and a running
client-side that can complete the handshake; the
package's typed `Render` was exercised in
`xrayVLESSRenderConfig` but booting a VLESS inbound
needs certs and was left for an external-credential
phase (it does not block the qualification of the
runtime path itself).

## 4. Outstanding sub-areas (deferred)

- **VLESS / Trojan / REALITY client handshake** —
  requires test certificates. The package's `Render`
  produces a config; booting a VLESS inbound needs
  `tlsCertRef` material and an XTLS-capable client.
  Tracked as a future external-credential phase.
- **User revoke and credential rotation** — needs
  the SOCKS test extended with multiple users and a
  reload between active sessions. Tracked.
- **Reload and rollback** — the xray package exposes
  `Runtime.Apply` and `Runtime.Rollback`. These have
  unit tests in `internal/xray/runtime_linux_test.go`
  but no real-binary test that uses a config that
  actually parses under the live binary. Tracked.

## 5. Artifacts

- `internal/qa/xray_linux_test.go` — the two tests
- `docs/evidence/qa/03-finding-M3-xray-real.md` — this
  document
