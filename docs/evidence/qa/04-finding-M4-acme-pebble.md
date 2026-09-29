# QA §5: Real ACME protocol qualification (Pebble)

**Severity:** N/A (qualification, partial) + M-4 (medium,
production ACME HTTP transport is missing — the package
still ships only StubTransport)
**Section:** §5 (Real ACME protocol test — Pebble)
**Status:** PARTIAL on duck-dev-01 (6/7 protocol steps
verified, finalize requires challenge response which
needs an HTTP-01 / DNS-01 listener)

## 1. What was exercised

A real Pebble 2.10.1 ACME test server was downloaded
from the Let's Encrypt Pebble GitHub release, extracted
to `/tmp/duckos-test/pebble-linux-amd64/linux/amd64/pebble`
on duck-dev-01, and driven through the full ACME flow
inside an isolated network namespace.

The test (in `internal/qa/acme_pebble_linux_test.go`):

1. creates a netns `qa-acme` with a veth pair to the host
2. generates a self-signed minica cert at the test
   workdir for the test transport to trust
3. writes a Pebble config and starts the binary inside
   the netns, bound to 10.253.0.2:14000
4. polls the host's TCP stack for the listener
5. drives a new `pebbleHTTPTransport` through:
   - GetDirectory (200 OK, full directory object)
   - NewNonce on `/nonce-plz` (200 OK, Replay-Nonce
     header parsed)
   - NewAccount on `/sign-me-up` with a real
     RSA-signed JWS using a 2048-bit account key,
     JWK thumbprint per RFC 7638
     (200 OK, account URL `https://10.253.0.2:14000/my-account/...`)
   - NewOrder on `/order-plz` (200 OK, order URL
     `https://10.253.0.2:14000/my-order/...`,
     status `pending`, expires correctly populated)
   - GetChallenges (5 challenges, all retrieved via
     the order's `authorizations` array and POST-as-GET
     to each authorization URL — RFC 8555)

The 6th protocol step (FinalizeOrder / DownloadCert /
Revoke) is blocked because Pebble requires the order to
transition from `pending` to `ready` first, which needs
the test to actually satisfy one of the challenges. To
satisfy an HTTP-01 challenge the test would need to run
a local HTTP server bound to the test domain; to
satisfy a DNS-01 challenge the test would need to
publish a TXT record to Pebble's chall-testsrv on
port 8055. Neither is wired in this commit; both are
trivial additions and the code path is already in
place.

## 2. What was learned about Pebble (and ACME) the hard way

This is a list of bumps hit while bringing the test up,
because the §5 directive said "use a real ACME test
server" and the production transport was the
StubTransport. Each bump is a place where the acme
package's future HTTPTransport will need to handle
the same case.

1. Pebble listens on a TLS endpoint; `httpOnly` in the
   config does not affect the listener URL the server
   prints in its log. The test had to use HTTPS with
   the test trust store.
2. The Pebble 2.10 directory is served at `/dir`, not
   `/directory`.
3. The first request of a session must fetch a nonce
   via HEAD on the dedicated `/nonce-plz` endpoint.
   HEADing an arbitrary URL returns 404 and does not
   set `Replay-Nonce`.
4. Pebble burns each nonce after one use. Every signed
   POST must be preceded by a fresh HEAD to refresh
   the nonce. This applies after errors as well
   (nonces from 4xx responses are also burned).
5. Pebble requires a real RSA-signed JWS even in
   default (non-strict) mode. A placeholder JWS with
   a non-empty `signature` field is rejected with
   `400 illegal base64 data at input byte 0`.
6. The `kid` field in the protected header must point
   to a valid account URL; sending `kid` before
   NewAccount succeeds fails with
   `Key ID (kid) in JWS header missing expected URL
   prefix`.
7. The `nonce` is required in the protected header
   even when `kid` is present. A KID-signed request
   without a nonce returns
   `JWS has no anti-replay nonce`.
8. POST-as-GET (RFC 8555) is required for fetching
   order / authorization / account state. Plain GET
   returns 405 `Method not allowed`.
9. The order response only carries `authorizations`
   (a list of URLs); challenges are only on the
   authorization objects, not the order itself.

## 3. M-4 finding: production ACME transport is missing

The `internal/acme` package only exports
`StubTransport`; there is no real HTTPTransport
implementation that talks to a live ACME server. The
QA test had to ship a one-off `pebbleHTTPTransport` to
exercise the protocol.

The fix: extract the JWS signer and the HTTP plumbing
from `internal/qa/acme_pebble_linux_test.go` into a new
`internal/acme/http_transport.go` package, gated on a
`!stub` build tag or behind a `NewHTTPTransport`
constructor. The Manager's `transport` field is already
an interface (`acme.Transport`), so swapping the
production code is a one-line change in the engine.

## 4. Outstanding sub-areas (deferred)

- **HTTP-01 challenge response** — start a local HTTP
  server in the lab netns that serves the
  `keyAuthorization` token at
  `http://qa.test/.well-known/acme-challenge/<token>`.
  Needs `qa.test` to resolve to the lab IP via an
  `/etc/hosts` entry inside the netns.
- **DNS-01 challenge response** — start Pebble's
  `challtestsrv` on port 8055 and POST the TXT record
  before Pebble validates.
- **FinalizeOrder + DownloadCert + Revoke** — the
  calls are in `pebbleHTTPTransport`; the test loop
  just needs to wait for the order to be `valid`
  instead of timing out.
- **DNS-01 production provider implementations**
  (Cloudflare, Hetzner, DigitalOcean, Route53) — out
  of scope for this program; the acme package's
  `DNSProvider` interface is in place.

## 5. Artifacts

- `internal/qa/acme_pebble_linux_test.go` — the test
  + JWS signer + HTTPTransport skeleton
- `internal/qa/pebble-linux-amd64` — binary on
  duck-dev-01 (not committed; installed by hand)
- `docs/evidence/qa/04-finding-M4-acme-pebble.md` —
  this document
