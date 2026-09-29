# Transaction and network core development evidence

Date: 2026-09-07. Base: 02582931c9d37ac9d4125858bdc48eb0413cf9b6.

This records configuration and network-control development evidence, not release
qualification or Linux networking acceptance.

Implemented as library packages:

- bounded private JSON revision store, integrity envelope, flock, atomic rename
  and fsync, one candidate, 32 previous revisions, strict recovery errors;
- privileged-controller logic with preflight/apply/verify/confirm lifecycle,
  rollback after partial failure, independent timer, monotonic in-process
  deadline, startup rollback and explicit recovery-required state;
- typed IPv4/IPv6 interface/WAN/LAN/VLAN, DHCP pools/reservations, routes,
  local DNS/upstream, filter and IPv4 NAT/forward intent;
- pure deterministic networkd/dnsmasq/nftables rendering with bounded input and
  conflict/injection validation. No arbitrary command or path input.

Local evidence:

- `make check`: Go race tests/vet, frontend type/format checks and 12 tests,
  22 installer tests, existing contracts, shell checks passed on Mac arm64.
- Focused state/network/controller race tests passed after monotonic deadline
  and ownership validation changes.
- Network decoder/renderer fuzz run: 176,462 executions over approximately six
  seconds; no crash found. This short run is not parser security qualification.
- `make build-all`: production frontend and amd64/arm64/ARMv7 binaries built.
  Separate `CGO_ENABLED=0 GOOS=linux GOARCH=... go build ./...` passed for
  amd64, arm64, and arm/GOARM=7, including the new library packages, which are
  not yet linked into executables. No Linux daemon syntax test was run on Mac.

Not yet integrated: production mutation IPC, exclusive Linux enrollment,
managed artifact adapter and independent Agent startup recovery, canonical
Engine/Agent reconciliation, UI configuration flow. Tests use an explicitly
synthetic adapter to inject failures; no product fake Linux state was added.

Before enabling changes on a lab target: implement those integration boundaries,
then test real DNS/DHCP/forwarded traffic, nft syntax and isolation, daemon
failure, API/Agent kills, reboot, clock changes, disk pressure and rollback on
an isolated Linux network. No access to duck-dev-01 has occurred.

## Recovery evidence extension, 2026-09-08

Added an owned networkd artifact replacement/recovery primitive and bounded
typed Linux link/forwarding capture. Before-state capture is journaled together
with the pending candidate; public controller replies strip this private data.
Tests cover capture failure before mutation, restart restoration using captured
evidence, redaction, multi-file partial staging, external drift refusal and
unknown-file preservation. On Mac these are algorithm/filesystem tests; the
actual Linux capture test is build-tagged and remains native-Linux evidence only.
The actual privileged network mutation adapter and IPC are still not enabled.

## Asynchronous admission and inactive baseline, 2026-09-08

The bounded Safe Apply worker admits at most one operation, continues independently
of the initiating HTTP connection, and exposes journal progress without waiting
for a Linux adapter call. Shutdown stops admission and waits for the active job
before the caller closes the controller and journal. Private recovery evidence
and arbitrary adapter/store error strings are excluded from worker replies.
Admission IDs are process-local, not durable idempotency keys. Outcome receipts
are now retained atomically with commit/rollback in snapshot format v2 (32-entry
bound). Worker progress uses the exact transaction receipt, never a matching
configuration digest; if a receipt is unavailable it reports `resolved` with
unknown outcome. Engine integration still needs durable reconciliation. Tests
cover receipt retention/restart, invalid receipt rejection and read-only v1
loading followed by atomic upgrade on the next journal mutation. Older binaries
cannot safely roll back onto v2 state; update compatibility is not yet enabled.

The empty network plan now has no firewall hooks or DNS listener configuration;
it does not accidentally impose managed-router default-deny policy on an
unenrolled host. Tests cover asynchronous admission, rollback failure, evidence
redaction, later commits with identical candidate content, and this inactive
baseline. These remain local library tests, not Linux mutation acceptance.

## Authenticated offline preview, 2026-09-08

The production API now connects the actual Engine validator/renderers through
`POST /api/v1/network/preview`. Tests exercise real persistent local identities,
all five role boundaries, bootstrap-token refusal, Host/Origin handling,
oversized/unknown/trailing input rejection, and real plan generation without
an Agent socket. Owner and NetworkAdmin may plan; other roles may not. Both
`hostValidated` and `applyAvailable` are false. No mock Linux observations or
mutation endpoints were introduced. UI editing, Agent validation and activation
remain unfinished; this endpoint is not evidence of usable Linux routing.

## Cross-process coordinator protocol, 2026-09-09

ADR-0010 supersedes the process-local admission identity described above.
Engine intent, Agent pending state and durable outcomes now share one ID.
The Unix protocol and Engine coordinator are implemented but not activated by
the installed executables. A portable integration test uses real Unix HTTP,
separate durable stores and an explicitly synthetic adapter: it discards the
apply acknowledgement, observes pending state, confirms, reconciles the exact
receipt and verifies only one apply submission occurred. Additional tests cover
malformed status/revision hashes, private recovery redaction, admission
cancellation tombstones, restart retention and equivalent receipt timestamps.
Cancellation is serialized with admission and cannot cancel active host work;
unknown acknowledgements are never treated as successful mutation or rollback.

Local `make check` passed (Go race/vet, 18 frontend and 22 installer tests).
Additional focused race tests passed after the receipt validation tests were
added. Production frontend and target cross-builds remain distinct from native
network integration. Enrollment, composite Linux adapter, mutation API/UI wiring
and native failure/reboot/traffic evidence remain required before activation.
