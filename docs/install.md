# Install the executable DuckOS foundation

This procedure installs an **internal TEST development build** on a clean,
booted **Debian 13 amd64** system with systemd. It is a read-only System
Overview foundation for DuckOS 1.0 development, not a public release or a
qualified production router. The optimized frontend and Go binaries are real;
native Debian installation and sandbox behavior still need designated lab
evidence. Nothing in this procedure has been run on `duck-dev-01` by the agent.

## Build and identify the exact artifact on the Mac

Use the committed local checkout whose `HEAD` is present in GitHub `main`.
The build records the full commit, tool versions, target, source commit time,
and dirty flag in `BUILD-INFO.json`. A dirty build is an internal working-tree
snapshot and cannot be attributed to a GitHub commit alone. Use a clean build
for target deployment. These commands do not change the Linux host:

```bash
cd /path/to/DuckOS
git rev-parse HEAD
git status --short
bash scripts/check.sh
bash scripts/build.sh amd64
bash dist/duckos-linux-amd64/install.sh \
  --artifact "$(pwd -P)/dist/duckos-linux-amd64" --verify-artifact
```

Stop if `git status --short` lists changes or the build metadata says
`"dirty": true`. After committing those changes, rebuild. Keep the commit SHA
with the artifact. The `VERSION` is `1.0.0-dev.<12-character-commit>`;
the installer deliberately rejects a public `1.0.0` label.

The payload directory contains `bin/duck-api`, `bin/duck-agent`, `web/`,
`systemd/`, `install.sh`, `VERSION`, `BUILD-INFO.json`, and `SHA256SUMS`.
The installer checks every file against the manifest, required inventory,
closed build-metadata format, commit/version/dirty agreement, and amd64 ELF
headers. It rejects symlinks, hard links, special files, traversal, unlisted
files, duplicate manifest paths, oversized input, and unexpected directories.
These checks detect corruption; **checksums do not authenticate a publisher**.
Review the committed code and trust the source/transfer channel. This is not a
signed `.duck` update package, and no GitHub Release is created.

## Obtain the committed code from GitHub on the test target

The following commands are for an administrator to run when a target test is
authorized. The target receives source **from GitHub**, at the exact commit in
the deployment record; local uncommitted Mac state is never transferred to it. Source
editing remains in the Mac checkout. The Linux checkout is only a pinned
build/test input. `YOUR_LINUX_USER` in later tunnel instructions is the existing
Debian login.

On the clean Debian test host, an administrator first prepares the explicit
build/runtime prerequisites. This is an operator action, not something the
DuckOS installer runs. Debian maintainer scripts and their effects are outside
DuckOS's initial-install recovery scope:

```bash
sudo apt-get update
sudo apt-get install git build-essential curl ca-certificates iproute2 nftables \
  passwd coreutils findutils
```

Use an ordinary, non-root build login with authorized access to the private
GitHub repository. Authenticate it to GitHub using that account's own supported
credential flow; no credential belongs in the clone URL or source checkout.
Then substitute the **full verified commit SHA selected for testing**:

```bash
DUCKOS_COMMIT='REPLACE_WITH_VERIFIED_FULL_COMMIT_SHA'
git clone https://github.com/CrazyDuckLab/DuckOS.git
cd DuckOS
git checkout --detach "$DUCKOS_COMMIT"
test "$(git rev-parse HEAD)" = "$DUCKOS_COMMIT"
git status --short
make bootstrap
make deps
make check
make build
bash dist/duckos-linux-amd64/install.sh \
  --artifact "$(pwd -P)/dist/duckos-linux-amd64" --verify-artifact
```

Stop on any failed command, changed file, or `"dirty": true` build metadata.
Bootstrap installs checksum-pinned Go and Node only inside this checkout's
`.tools` directory. The build uses the lockfile and produces optimized static
Go executables plus the compiled frontend. The clean target build must name
the same GitHub commit as the Mac build evidence. Running these commands is a
later designated test activity; the agent has not run them on `duck-dev-01`.

Stage the verified artifact in a **new** root-owned directory. The following
chain stops if staging already exists; it never merges or overwrites an earlier
artifact:

```bash
sudo mkdir -m 0700 /root/duckos-install && \
sudo cp -R -- dist/duckos-linux-amd64 /root/duckos-install/ && \
sudo chown -hR root:root /root/duckos-install/duckos-linux-amd64 && \
sudo chmod -R go-w /root/duckos-install/duckos-linux-amd64
```

The payload and every ancestor must be root-owned, without group/world write
permissions. Running installation directly from a user's home or `/tmp` is
refused. The read-only `--verify-artifact` mode is the sole exception to root
ownership, for local build checks and fixture tests. The root installer verifies
the staged payload again before making any host change.

## Debian preflight and first installation

The host must already have the prerequisites above, plus Debian's Bash, systemd
and dpkg. The installer reports missing tools but never runs `apt`. Do not
apply this procedure to a production gateway.

The preflight checks Debian version and architecture, booted systemd, existing
DuckOS paths/accounts/units, port 8080 availability, root path ownership, and
free disk space for payload copies plus a 256 MiB reserve:

```bash
sudo bash /root/duckos-install/duckos-linux-amd64/install.sh \
  --artifact /root/duckos-install/duckos-linux-amd64 --check
```

If preflight passes, the same command without `--check` performs the first
installation and waits for authenticated Agent-backed readiness and overview:

```bash
sudo bash /root/duckos-install/duckos-linux-amd64/install.sh \
  --artifact /root/duckos-install/duckos-linux-amd64
sudo systemctl status duck-agent.service duck-api.service --no-pager
curl --fail http://127.0.0.1:8080/api/v1/health
```

The unauthenticated health route is liveness only. Installation success requires
both systemd services active and HTTP 200 from **authenticated**
`/api/v1/ready` and `/api/v1/overview`. Readiness proves that the Engine reaches
the authenticated Agent and receives a compatible contract. Individual optional
Linux observations may still be unavailable; the dashboard reports their
availability/issues explicitly. This does not qualify forwarding, routing,
firewall policy correctness, reboot recovery, or production readiness.

## Open the dashboard

The service binds only `127.0.0.1:8080`. From the Mac, an administrator may run:

```bash
ssh -N -L 127.0.0.1:8080:127.0.0.1:8080 YOUR_LINUX_USER@duck-dev-01
```

Open `http://127.0.0.1:8080` in the Mac browser. Use local port 8080 as shown;
the API validates the management Host/Origin and rejects other ports.
In a separate administrator terminal on Debian, read the generated access
token only when needed:

```bash
sudo cat /etc/duckos/api-token
```

Use that token once in the first-run screen to create a local Owner with a
unique 15–128-character passphrase, then sign in using the account. After
enrollment the bootstrap token cannot read Overview or enroll another Owner.
Do not paste the token or password into chats, command-line
arguments, source files, screenshots or logs. The installer generates 32 bytes
of OS randomness, writes the token to a protected file, and never prints it.
Its own probes pass the credential to curl through standard input, with curl's
user configuration disabled. Local identities include built-in roles, bounded
sessions, revocation and an identity-change audit. Network mutations are still
disabled. MFA/passkeys, automatic TLS provisioning and Vault remain
unfinished. Keep the service on loopback and use an authenticated administrative
tunnel. Never delete the identity directory to recover a password: that loses
identity/audit evidence and reopens enrollment. Optional administrator-provisioned
[TLS transport](tls.md) is available without broadening the loopback bind.
For lost Owner credentials, use the local Linux recovery command as root:

```sh
sudo systemctl stop duck-api
sudo duck owner reset
sudo systemctl start duck-api
```

The command requires effective root, no password/session/API token, and an offline
identity store. It refuses an active writer and does not invoke any services.
It prints a timestamped backup under `/var/backups/duckos-owner`, including the
complete original identity store and SHA-256 manifest, before returning. Backup
failure prevents reset. Only Owner accounts are removed; other users and the
bounded audit history are preserved. Stopping the API invalidates every session.
Use the existing installation bootstrap credential in normal enrollment to create
a new Owner (choose a name not belonging to a preserved non-Owner user).
The reset uses a staged, validated identity journal and an atomic, fsynced
replacement retaining service ownership. An interruption leaves either the old
identity state or recovery-ready state; rerun the command with the API stopped
if its outcome is uncertain. Never restore the old identity backup casually:
that would restore old credentials. Backup retention/removal is the administrator's
responsibility. Network services, journals and configuration are not changed.

## Ownership and service boundaries

| Path / service | Owner and mode | Meaning |
|---|---|---|
| `/opt/duckos/releases/<VERSION>` | root:root; dirs 0755, binaries 0755, other files 0644 | Immutable to DuckOS service users; exact payload checksums preserved |
| `/opt/duckos/current` | root-created relative symlink | First active release; no in-place update |
| `/etc/duckos` | root:duckos 0750 | Protected runtime inputs |
| `/etc/duckos/api-token` | root:duckos 0640 | Bootstrap API enrollment token; not used for API-to-Agent assertions |
| `/etc/duckos/agent-assertion.key` | root:duckos 0640 | HMAC key shared by duck-api and duck-agent for authenticated principal assertions over local IPC |
| `/var/lib/duckos` | root:duckos 0750 | DuckOS-owned state boundary |
| `/var/lib/duckos/config` | duckos:duckos 0700 | Reserved for canonical desired state; no store implementation yet |
| `/var/lib/duckos/state` | duckos:duckos 0700 | Reserved for derived/observed state; no persistent metrics writer yet |
| `/var/lib/duckos/identity` | duckos:duckos 0700; files 0600 | Durable local users/password hashes and bounded identity audit; never include in ordinary config exports |
| `/var/lib/duckos/install-status` | root:duckos 0640 | `incomplete` or `ready` with build ID; diagnostic marker, not update journal |
| `/run/duckos-agent` | root:duckos 0750 | systemd-managed transient IPC directory |
| `/run/duckos-agent/agent.sock` | root:duckos 0660 | Agent validates the numeric API UID using Linux peer credentials |
| `duck-api.service` | duckos:duckos; no capabilities | API + Engine + compiled web; filesystem read-only except private identity directory |
| `duck-agent.service` | root:duckos; bounding set NET_ADMIN only | Bounded read-only Linux collection; no generic command or mutation RPC |

The installation UID is substituted into the installed Agent unit. No static
UID is assumed and no existing `duckos` identity is reused. Agent network
namespace remains the host namespace so interfaces/routes/nftables are real
host observations. `CAP_NET_ADMIN` is needed for nftables reads; the semantic
RPC exposes no nftables mutation. The capability itself can modify network
state if the Agent is compromised, so this remains a security-sensitive
boundary requiring native sandbox and abuse-case tests.

Both units use `NoNewPrivileges`, strict filesystem protection, private devices
and temporary directories, namespace restrictions, closed capability sets,
disabled core dumps, 256 MiB memory maxima, 64-task limits, bounded file
descriptors, restart throttling and per-unit journal message rate limits.
Global journal retention is Debian-owned and is **not** changed by installation;
full DuckOS retention/disk-pressure controls remain future work. State directory
ownership reserves the product boundary but the current service sandboxes do
not grant persistent state writes.

Installation creates only DuckOS accounts, directories and two units. It does
not configure or restart SSH, rewrite interfaces/routes/DNS, apply nftables
rules, enable forwarding, alter network-manager ownership, or edit
`/etc/resolv.conf`. It enables the two DuckOS services only after authenticated
readiness succeeds.

## Failure, retries and evidence

This is a **fresh-install-only** procedure. Any existing `/opt/duckos`,
`/etc/duckos`, `/var/lib/duckos`, `/run/duckos-agent`, DuckOS account/group or
installed DuckOS unit is a refusal, including an interrupted earlier install.
Active and Previous releases are never replaced or deleted by this installer.
A successful first installation has Active and no Previous, as allowed by
[ADR-0004](adr/0004-update-active-previous.md).

On an ordinary command failure, the trap attempts to stop only DuckOS services
started by this invocation, disables them if enabling was attempted, and
preserves installed files and bounded journal evidence for investigation. It
does not claim to roll back Debian accounts, files, systemd state, kernel state,
or a power interruption. `install-status` is a diagnostic marker, not a durable
transaction journal. Automatic repair, uninstall, upgrade, release signatures,
crash-safe activation, rollback and Previous rotation are not implemented.

For an interrupted/failed installation, keep the exact artifact and commit,
inspect status and bounded logs, then decide recovery in an explicit operator
session. Do not rerun blindly or use a recursive delete to make preflight pass:

```bash
sudo cat /var/lib/duckos/install-status
sudo systemctl status duck-agent.service duck-api.service --no-pager
sudo journalctl -u duck-agent.service -u duck-api.service --no-pager -n 100
```

Local automated installer tests use clearly synthetic temporary payloads,
exercise the actual artifact verifier and adversarial filesystem/manifest
inputs, and inspect unit security settings. They never install on the Mac or
simulate a successful Debian boot. Runtime evidence still requires a designated
Debian 13 amd64 lab run: installation, authenticated dashboard, privilege
denials, nftables read access, failure recovery, sandbox behavior and reboot.
