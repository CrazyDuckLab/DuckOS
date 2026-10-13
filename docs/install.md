# Install DuckOS v0.1.0-beta.2

**Beta software. FRESH-INSTALL-ONLY. No qualified beta.1 → beta.2 in-place upgrade.**
Use a fresh Debian 13 host with systemd, an isolated test network, and an
out-of-band console. Do not remove an existing installation to bypass preflight.
Keep your management interface outside network changes.

## Select and verify the download

On the target, inspect both kernel and Debian architecture:

```bash
uname -m
dpkg --print-architecture
```

| Kernel architecture | Debian architecture | Artifact suffix |
|---|---|---|
| `x86_64` | `amd64` | `amd64` |
| `aarch64` / `arm64` | `arm64` | `arm64` |
| `armv7l` | `armhf` | `armv7` |

ARMv7 is a 32-bit ARM build using `GOARCH=arm GOARM=7`.
See [platform qualification](compatibility.md); ARM builds did not receive
the complete AMD64 two-node final live matrix.

Choose the suffix above. The example defaults to AMD64; set `ARCH=arm64` or
`ARCH=armv7` for the corresponding target:

```bash
ARCH=amd64
ARCHIVE="duckos-v0.1.0-beta.2-linux-${ARCH}.tar.gz"
BASE=https://github.com/CrazyDuckLab/DuckOS/releases/download/v0.1.0-beta.2
curl --fail --location --output "$ARCHIVE" "$BASE/$ARCHIVE"
curl --fail --location --output SHA256SUMS "$BASE/SHA256SUMS"
sha256sum --check --ignore-missing SHA256SUMS
```

Require an `OK` line for the selected archive. Checksums detect corruption;
they do not independently authenticate the publisher. Use the trusted GitHub
release channel. All three archives extract to `duckos-v0.1.0-beta.2/`;
extract only the selected architecture into a new directory.

## Prerequisites and root-owned staging

Prepare Debian tools explicitly before installation:

```bash
sudo apt-get update
sudo apt-get install iproute2 nftables curl ca-certificates passwd coreutils findutils python3
```

The installer checks prerequisites. For WireGuard operations, install
`wireguard-tools`; FRR routing requires `frr`. Install `libcap2-bin` so the
installer can set the constrained launcher file capabilities:

```bash
sudo apt-get install wireguard-tools frr libcap2-bin
```

FRR handling and native daemon masking are
part of its actual install flow; inspect its output and preserve any failure.
`duckos` and `duck-dnsmasq` are service identities: use a separate administrator
account. The current installer tolerates existing names; that does not make
reuse of an interactive account a recommended deployment practice.

Verify the extracted payload locally, then stage it with root-owned ancestors:

```bash
mkdir duckos-beta2-download
tar -xzf "$ARCHIVE" -C duckos-beta2-download
bash duckos-beta2-download/duckos-v0.1.0-beta.2/install.sh \
  --artifact "$(pwd -P)/duckos-beta2-download/duckos-v0.1.0-beta.2" --verify-artifact
sudo mkdir -m 0700 /root/duckos-install
sudo cp -R duckos-beta2-download/duckos-v0.1.0-beta.2 /root/duckos-install/
sudo chown -hR root:root /root/duckos-install/duckos-v0.1.0-beta.2
sudo chmod -R go-w /root/duckos-install/duckos-v0.1.0-beta.2
```

Stop if the staging directory already exists; use a new operator-reviewed
staging location rather than merging payloads. Installation directly from a
user home or world-writable `/tmp` is refused. The verifier checks the closed
inventory, hashes, build metadata and architecture-specific ELF headers.

## Preflight and install

```bash
sudo bash /root/duckos-install/duckos-v0.1.0-beta.2/install.sh \
  --artifact /root/duckos-install/duckos-v0.1.0-beta.2 --check
sudo bash /root/duckos-install/duckos-v0.1.0-beta.2/install.sh \
  --artifact /root/duckos-install/duckos-v0.1.0-beta.2
sudo systemctl status duck-agent duck-api duck-frr-launcher duck-wg-launcher --no-pager
curl --fail http://127.0.0.1:8080/api/v1/health
cat /opt/duckos/current/VERSION
```

The installer waits for authenticated Agent-backed readiness. `/api/v1/health`
proves API liveness only; authenticated `/api/v1/ready` must succeed as well.
Use an enrolled session for API checks after Owner enrollment. Do not expose
credentials in command arguments or logs.

`BUILD-INFO.json` records the source commit and target; runtime identity remains
`1.0.0-dev.<12-character-commit>`, separate from the public beta name.
`/run/duckos-agent` must be `root:duckos 0750`. The Agent has zero effective
Linux capabilities; the WireGuard launcher is limited to `CAP_NET_ADMIN`.
Native `frr.service` and `wg-quick@.service` are masked to avoid competing owners.

## Dashboard and enrollment

From an administrator workstation:

```bash
ssh -N -L 8080:127.0.0.1:8080 <user>@<duckos-ip>
```

Open `http://127.0.0.1:8080`. Read `/etc/duckos/api-token` privately on the host
when the first-run enrollment screen requires it. Create an Owner with a strong
unique passphrase; do not paste the credential into issues, chats or logs.
Follow [getting started](user/getting-started.md).
There is no automatic mDNS discovery or browser-trusted LAN TLS onboarding.

## CLI availability

The archive contains six service binaries and the Web UI; it does not install
the standalone `duck` CLI. Use the UI or authenticated API for routine access.
See [CLI/API](cli-api.md) before following a guide that uses `duck`.

## Failure and recovery

Keep the artifact and build identity. Inspect bounded status and journal output:

```bash
sudo cat /var/lib/duckos/install-status
sudo journalctl -u duck-agent -u duck-api -u duck-frr-launcher -u duck-wg-launcher --no-pager -n 100
```

Do not delete installation paths or identities to make preflight pass.
An interrupted install requires an explicit recovery decision.
See [troubleshooting](troubleshooting.md) and [recovery](user/recovery.md).
The release tarball is an installer payload, not a signed `.duck` upgrade package.

## Build from source

Use a clean checkout of the intended commit, then `make deps`, `make check` and
`make build-all`. The official builder produces `dist/duckos-linux-amd64`,
`dist/duckos-linux-arm64` and `dist/duckos-linux-armv7`. Verify each with its
supplied `install.sh --artifact <absolute-directory> --verify-artifact`.
Dirty builds are refused for target installation.
