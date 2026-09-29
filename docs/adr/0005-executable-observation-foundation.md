# ADR-0005: Executable system observation foundation

- Status: Accepted for the read-only observation runtime
- Date: 2026-09-07
- Extends: ADR-0001, ADR-0002; preserves ADR-0003 and ADR-0004

## Decision and scope

The next internal development commit implements System Overview end to end:
React UI → Go API → Engine → privileged Agent → actual Linux observations.
The owner explicitly requested this implementation, a Debian 13 amd64 fresh
installer, automated checks, and production-mode builds. This advances the
read-only parts of stages 2–4 and first-install tooling; it does not claim the
remaining stages passed or publish an intermediate product release.

API and Engine are separate Go packages in one unprivileged `duck-api` process.
The independently executable `duck-agent` runs as root with only
`CAP_NET_ADMIN` retained for nftables inspection. The Engine obtains all Linux
facts from the Agent. A small bounded memory cache amortizes observations;
observed state never becomes canonical desired state and is not persisted.

The supported initial install target is clean Debian 13 amd64 with systemd,
iproute2 and nftables available. Stock Debian 13 provides the kernel 6.12 and
systemd 257 baseline. Both Go programs also cross-build for Linux arm64 and
ARMv7 (`GOARM=7`, `CGO_ENABLED=0`); these artifacts are build evidence only.
There is no claim of a Debian XU4 kernel or native ARM runtime qualification.

Build tools are Go 1.27.1 and Node 24.20.0 LTS/npm 11.19.0, installed in an
ignored checkout-local `.tools` directory from checksum-pinned official
archives. React/TypeScript/Vite and test dependencies use exact versions and
`web/package-lock.json`. Node is a build dependency, not installed on a router
when installing a prebuilt payload. Production frontend assets are static.

## Agent contract and threats

HTTP/1.1 JSON over a filesystem Unix socket is the versioned IPC transport.
The only operation is `GET /agent/v1/overview`, with no query or request body.
The peer UID from Linux `SO_PEERCRED` must exactly match the installed `duckos`
UID. Socket directory is root:duckos 0750; socket is root:duckos 0660.
Authentication does not rely on a caller-supplied UID header. Unknown routes,
methods, versions and payloads fail closed. Replies are capped at 2 MiB;
timeouts, concurrency limits and fixed adapter output bounds prevent unbounded
work. A captured read may be repeated; no replayable mutation exists.

The public v1 schema defines observations, collection time, and typed issues.
CPU utilization is a sampled counter delta, initially null; load average is a
different metric. Missing/denied facilities yield issues and unavailable UI
state, never invented success. Resolver observation reads the host's resolver
configuration; it does not assert upstream DNS reachability. nftables table,
chain and rule counts do not establish firewall effectiveness. Service state
is not an application health guarantee.

Adapters may read bounded `/proc`, `/sys`, resolver and OS files, call native
Go networking/statfs APIs, and invoke fixed absolute `ip`, `systemctl`, and
`nft` paths with reviewed argument vectors, deadlines, minimal environment and
bounded output. No shell text, caller-selected command, path, environment,
service, or network mutation crosses IPC. Agent remains in the host network
namespace; a private namespace would observe the wrong machine state.

## Management access

API listens exclusively on a literal loopback address, initially
`127.0.0.1:8080`. It enforces the listener authority and same-origin requests
to mitigate DNS rebinding. `/api/v1/health` reveals liveness only. Overview and
Agent readiness require a cryptographically random installer-generated bearer
token read from root:duckos 0640 `/etc/duckos/api-token`. UI keeps that token in
memory only. Responses are no-store with restrictive browser security headers;
tokens are never placed in URLs or logs. Administrative SSH local forwarding
can carry loopback HTTP securely; this does not change the local-development
workflow. Full enrollment, TLS, sessions, RBAC and Vault remain future work;
the current token grants read-only observation access, not a permanent user
identity model or an encryption-at-rest claim.

## Host ownership and fresh install

Existing network managers remain authoritative for host networking. DuckOS
only observes their output. This closes F-006 for read-only observation; any
future network writer still requires the complete ownership and Safe Apply
decisions. F-003 and F-004 do not block this stateless, non-mutating use case.

The installer owns only new DuckOS users/groups, directories and two units.
It checks Debian/architecture/dependencies, rejects existing installations,
verifies a bounded complete payload inventory and hashes, and creates an
immutable side-by-side release directory with an initial `current` link.
It does not run an OS upgrade or alter host networking. Missing Debian packages
are installed separately by the operator using the documented commands.
Failure leaves stopped DuckOS services and retained install evidence for
inspection. First installation is not the ADR-0004 Upgrade Engine: re-running
it cannot replace Active, delete Previous, or bypass a future update journal.
Checksums bind payload integrity to a reviewed trusted checkout; they are not
release signatures or authentication of an untrusted publisher.

## Verification and remaining work

Required now: Go parser/API/Engine tests, invalid and unauthorized IPC tests,
browser component tests, schema compatibility checks, installer adversarial
preflight tests, frontend production build and all three Linux cross-builds.
Native Linux tests exercise Unix peer credentials and real host collectors;
Linux-only tests must be explicitly reported when skipped on macOS.
Fresh Debian installation, systemd sandbox behavior and real privileged nft
inspection require Linux execution evidence. No target is touched by this
Mac implementation task. See the implementation evidence record for results.

F-001 is resolved for this slice. F-002, F-005 and F-007 have the bounded
contracts above; mutation protocols, broader platform qualification and
upgrades remain open. Configuration transactions, managed networking,
full 1.0 scope qualification and public release approval remain separate gates.

## Sources

- [Go release archives](https://go.dev/dl/)
- [Node release metadata](https://nodejs.org/dist/index.json)
- [Vite build requirements](https://vite.dev/guide/)
- [Debian 13 baseline](https://www.debian.org/releases/trixie/release-notes/whats-new.en.html)
- [Linux Unix sockets and SO_PEERCRED](https://man7.org/linux/man-pages/man7/unix.7.html)
