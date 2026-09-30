# Privacy and network egress

DuckOS is a **local-first** network operating platform. This page
documents, from source and runtime evidence, the categories of
network traffic a V1 beta (`v0.1.0-beta.1`) installation actually
generates — and what it deliberately does not generate.

The classification is evidence-backed. Every category below links
either to a documented behaviour in the supported installer, the public
API, or the product source.

## At a glance

| Traffic category | Present in `v0.1.0-beta.1`? |
|---|---|
| Product telemetry / analytics | **No.** DuckOS V1 beta does not collect, transmit, or store telemetry or analytics from the installed device. No remote telemetry endpoint is configured. |
| Crash reporting | **No.** No automated crash reporting to a remote service. |
| Update channel (remote) | **No.** There is no public OTA / update server. Updates are signed `.duck` payloads applied manually. |
| Outbound metrics exporters (Prometheus, OpenTelemetry, vendor SDKs) | **No.** No metrics export endpoint is configured. |
| Phantom phone-home | **No.** The Engine and Agent use a local Unix socket, not a routable listener. |
| Browser-side telemetry from the SPA | **No.** The SPA talks only to `/api/v1/...` on `127.0.0.1:8080`. |
| Management access from the LAN | **No, by default.** The management UI/API bind only `127.0.0.1:8080`. |
| Install-time package-manager traffic | **Yes, opportunistic and documented.** The supported installer can run `apt-get install` for `frr` when the host has `apt` available and `frr` is not already installed. No other package manager is invoked. |
| User-configured traffic (WireGuard, Xray, DNS forwarders, subscriptions) | **Yes, by configuration.** Whatever the administrator configures and confirms through Safe Apply. |
| First-install prerequisite install | **Possibly.** Before running the DuckOS installer, an administrator on a bare Debian host may run `apt-get install` to provide build / runtime prerequisites (`sutil`, `iproute2`, `nftables`, `curl`, `ca-certificates`, `passwd`, `coreutils`, `findutils`). This is an administrator action, not DuckOS-initiated. |

## What V1 beta traffic actually looks like

### Runtime DuckOS traffic

The runtime traffic is the network traffic the installed DuckOS
service generates by itself, without any user-configured tunnel or
forwarder.

**In V1 beta, runtime DuckOS traffic is limited to loopback IPC:**

- The Engine talks to the privileged Agent over a Unix-domain socket
  (`/run/duckos-agent/agent.sock`). The Engine's HTTP client URL is
  `http://agent/...`, a host alias that resolves to that socket.
- The privileged Agent does not bind a routable listener. It has no
  network socket other than the local IPC socket.
- The API binds `127.0.0.1:8080` only. It validates the management Host
  header and rejects other ports.
- The bootstrap API token (used once, on the first-run enrollment
  screen) is read by the administrator from
  `/etc/duckos/api-token`; no remote token issuer is contacted.

No code path in the Engine or the Agent contacts a remote
endpoint to retrieve product telemetry, crash data, or model /
analytics data.

### Installation / package-manager traffic

The supported installer (`install.sh`) is the only place V1 beta
talks to `apt-get`. It does so **once**, **opportunistically**, and
**only for `frr`**:

```text
apt-get install -y --no-install-recommends frr
```

This runs only when `apt-get` is available on the host AND the `frr`
binaries are not already installed. The output is written to
`/var/log/duckos-install-frr.log`. If `apt-get install frr` fails,
the installer records the honest "not installed" state in
`install-status` and continues — routing-protocol features simply
report not-installed until the administrator resolves it.

The installer does not run `apt update`, `apt upgrade`, or any other
package operation. It does not invoke `apt` for any package other
than `frr`.

### Update-related traffic

There is **no automatic update check**. The update supervisor
operates on signed `.duck` payloads delivered by the administrator.
The GitHub release page is the supported delivery channel for this
beta; there is no public OTA / update server, and no remote endpoint
is contacted to look for new releases.

If you want to update, download the signed `.duck` payload from the
GitHub release, validate the signature with the project's
published key, and apply it through the documented update flow.

### User-configured traffic

DuckOS will route whatever the administrator configures and confirms
through Safe Apply. A non-exhaustive list:

- WireGuard peers, Xray servers, NetBird peers, and subscription
  provider clients, once those subsystems are activated.
- DNS upstream resolvers and DHCP relay targets.
- Site reverse-proxy / published services.
- Routing protocol peers (BGP, OSPF, IS-IS, BFD) once activated. They
  carry their own protocol traffic (TCP 179 for BGP, OSPF on
  protocol 89, etc.).
- Mirror / SPAN, capture, and diagnostic probes, where activated.

None of these is enabled by default. They appear on the network only
after the administrator has prepared a candidate, reviewed the diff,
applied it through Safe Apply, and confirmed it.

### Telemetry / analytics

DuckOS V1 beta does **not** transmit product telemetry, usage
analytics, crash reports, or model-collected data from the installed
device.

There is no remote telemetry endpoint configured in source. There is
no SDK, vendor SDK, OpenTelemetry exporter, Prometheus push gateway,
Datadog agent, Sentry client, Amplitude tracker, Mixpanel tracker,
PostHog, Segment, or similar component wired into the installed
service.

If you operate DuckOS behind a network egress filter or a corporate
proxy, DuckOS V1 beta should produce no logs of "telemetry" attempts,
because it does not make any.

## Browser-side behaviour

The SPA bundled with the installed DuckOS service is served from the
device itself. It calls only `/api/v1/...` on `127.0.0.1:8080`. There
is no third-party CDN, no analytics script, no telemetry SDK, no
remote font loader, and no remote font/icon endpoint.

If you view the SPA's network activity in your browser developer
tools (for example through the SSH tunnel), every documented request
targets `127.0.0.1:8080`.

## Static review notes

This section records what was inspected to write the categories above.
It is a snapshot, not a permanent certification.

### Outbound-endpoint strings in the product source

A direct grep across `cmd/` and `internal/` for `https://` / `http://`
URL strings, excluding loopback (`127.0.0.1`, `localhost`, `::1`),
GitHub, Go stdlib, and synthetic test URLs (`example.test`,
`acme.test`, `vpn.example.test`), finds only:

- Local Unix-socket URL aliases: `http://agent/...`,
  `http://runtime/...`, `http://restore/...`. These resolve to
  `/run/duckos-agent/agent.sock` and similar sockets, not the
  network.
- A `cmd/duck` CLI argument validator that requires `--endpoint` to
  be `http://` / `https://` when supplied. No default value points
  anywhere off-loopback.

### Telemetry / analytics / exporters

A direct grep across `cmd/`, `internal/`, and `web/src/` for
telemetry / analytics / metrics-export / crash-reporting components
finds only:

- `internal/hardware/hardware.go` — references to **local** SFP / SFP+
  module telemetry (hardware-side information read from the
  device).
- `internal/storagewatch/storagewatch.go` — local retention priority
  classes (`PriorityTelemetry`, `PriorityLogging`, etc.) for bounded
  cleanup of on-device data, **not** remote export.
- `internal/measure/measure.go`, `internal/canary/canary.go` — local
  observation primitives (passive capture, canary retentions), with
  no remote endpoint configured.
- Test files in `tests/`, `cmd/*_test.go`, `internal/**/*_test.go` —
  synthetic fixture URLs against `example.test` / `acme.test` /
  `vpn.example.test`. These never reach the network.

No SDK, vendor SDK, or remote endpoint is wired into the installed
runtime.

## What you can do to verify

You can independently reproduce the claims above:

- Run the installed service behind an outbound-egress firewall with no
  rules for DuckOS. There should be **no** outbound attempts to
  vendor / telemetry endpoints. The only outbound connections
  should be those of the user-configured subsystems (WireGuard,
  Xray, DNS, routing protocols, …) once activated.
- Read the SPA's network activity in browser developer tools
  through the SSH tunnel. Every request targets `127.0.0.1:8080`.
- Grep the installed binaries' strings for outbound hostnames. The
  only loopback hostnames (`agent`, `runtime`, `restore`) refer to
  local Unix-socket aliases.

## Caveats and what is NOT proven by this page

- This page describes the **current** V1 beta. A later release may
  add an explicit opt-in telemetry path, an auto-update channel, or
  a metrics-export feature. When that happens, this page will be
  updated.
- A user-configured subsystem (WireGuard / Xray / DNS / routing
  protocols / published services) **does** produce egress traffic.
  This page does not enumerate those.
- This page does not enumerate `apt-get` traffic from the host
  package manager outside of DuckOS. A bare Debian host may have its
  own automatic updates; that is host policy, not DuckOS policy.
- The static review captured a snapshot of the source. If you fork
  the source or build a custom image, the resulting behaviour is yours
  to care for.

## Related documentation

- [`../beta-limitations.md`](../beta-limitations.md) — what V1 beta is
  and is not.
- [`../update-system.md`](../update-system.md) — update model and
  signature verification.
- [`../security-model.md`](../security-model.md) — local identity,
  management binding, privileged Agent isolation.