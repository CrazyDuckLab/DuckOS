# ADR 0015: Shared secret client exports

Status: Accepted; renderer and access-control components, lifecycle integration pending.

## Context

Subscription URLs can contain bearer credentials. A WireGuard client configuration
contains its private key. QR images, clipboard text and downloaded files therefore
cross the same security boundary and must not be generated independently.

## Decision

The client export service resolves a current canonical client through a provider
on every request. The provider must enforce expiry, disablement and credential
revision; an old export is never used as a credential source. Resolution is bounded
and cancellable. Only Owner and Network Admin have `clients.export` permission.
Session authorization precedes resolution and is checked again by the durable
audit operation before delivery. Audit failure denies export.

A document has one exact UTF-8 payload plus resource ID, credential revision,
kind and a constrained filename. QR, copy and file download use these same bytes.
Ordinary formatting and structured logging redact the document; generic JSON
serialization is refused. An authenticated transport must explicitly reveal it.
Audit records contain only actor/resource identifiers, kind and action, never
keys, URLs or configurations. An audit event records an authorized export request,
not proof that a person saved or scanned the result.

WireGuard rendering uses standard wg-quick client sections, X25519 keys, validated
addresses, endpoints and peer settings. Arbitrary shell hooks are not accepted.
Subscription rendering preserves the exact HTTPS URL. Nothing is sent to a remote
QR service or written as a public/static image.

The browser uses byte-mode QR with medium error correction and a four-module quiet
zone. The maximum payload is 2,331 bytes (version 40); oversized payloads fail rather
than truncate or silently substitute a URL. ASCII client configurations and escaped
subscription URLs are the interoperability baseline. Phone-client qualification is
still required, especially for non-ASCII URL content and dense symbols.

The modal warns that possession grants use of the connection. Each copy/download
resolves and audits fresh content; a resource revision change discards the prior
modal session and aborts its pending requests. Temporary download object URLs are
revoked. Closing does not erase a user's clipboard or downloaded file.

## Dependencies and verification

`qrcode` 1.5.4 (MIT) generates matrices locally. `jsqr` 1.4.0 (Apache-2.0) is a
development-only independent decoder used to verify exact payload bytes. Versions
and package integrity are pinned in the frontend lockfile. Automated tests cover
configuration generation, access denial, audit redaction, fresh resolution after
rotation, QR decoding, modal cancellation and download equality.

## Remaining boundaries

`POST /api/v1/clients/export` accepts only resource ID, kind and export action in
its JSON body, uses the Engine export service and returns a private no-store JSON
document. It is denied without local session authorization and canonical providers.
The browser loader rejects redirects and bounds responses to 16 KiB. No secret is
placed in a request URL. The API is not a public subscription distribution endpoint.

These components do not yet provision Linux WireGuard peers, persist encrypted
private credentials, implement the client lifecycle API or expose a production
client-management page. Those integrations must preserve this contract; a rendered
configuration alone is not evidence of an active tunnel. Live mobile scans and
traffic tests remain integration gates.

## References

- [WireGuard quick start](https://www.wireguard.com/quickstart/)
- [wg-quick configuration](https://git.zx2c4.com/wireguard-tools/about/src/man/wg-quick.8)
- [QR encoder and capacity](https://github.com/soldair/node-qrcode)
