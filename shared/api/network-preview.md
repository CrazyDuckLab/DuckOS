# Network preview API v1

`POST /api/v1/network/preview` accepts the canonical v1 network configuration
defined by `internal/network.Config` and its strict decoder/validator. This is
an offline planning operation, not a mutation or a host enrollment endpoint.

The companion [flow simulation endpoint](network-simulation.md) evaluates the
same canonical candidate through shared forwarding/NAT and policy instructions.

Required: enrolled local identity, bearer session with `network.write`
permission (Owner or NetworkAdmin), uncompressed `application/json`, no query
parameters. Request body is bounded to 256 KiB. Installation bootstrap tokens
cannot authorize planning. Normal exact Host/Origin and TLS boundaries apply.

Success is HTTP 200 with `schemaVersion: "1"`, `hostValidated: false`,
`applyAvailable: false`, and `plan` containing generated `networkd` basename-to-
content mapping, `dnsmasq` text, and `nftables` batch text. Response is bounded
to 2 MiB. An empty baseline has no networkd files or DNS configuration, and its
nft batch has no hooks. Generated artifacts are previews, never commands for
the browser to execute or files for the browser to write.

HTTP 401: invalid/expired session; 403: role denied or enrollment required;
400: invalid transport parameters or oversized/unreadable body; 405: wrong
method; 415: unsupported content type/encoding; 422: invalid or unsupported
network intent; 503: capacity or planner unavailable. Errors do not echo
configuration contents. POST does not persist state, access Agent IPC, validate
installed daemon syntax, prove ownership, or change Linux. There is no enabled
`/api/v1/network/apply` endpoint yet.

Native Linux daemon validation, ownership, Safe Apply IPC, and a configuration
editor remain separate required work; a successful preview is not acceptance
of an installable routing configuration.

Optional `policyRules` accepts bounded IP-level selectors (`id`, `family`,
`priority`, `table`, optional `from`/`to` canonical CIDRs). Private route tables
are 10000–10999; even priorities 20000–21998 reserve the following priority for
an unreachable guard. Matching table/family routes must exist in the same
candidate. Generated connected-network and fail-closed guards are inspectable
in the networkd preview. This is not the full named Traffic Policy/Route Group
API, and it does not activate rules or establish root resource enrollment.

Optional `managementPolicy.rules` accepts up to 64 explicit grants with `id`,
`interface`, `service` (`ssh`, `web`, `api`), TCP `port`, and up to 16 canonical
trusted source CIDRs. Host addresses use /32 or /128. Grants are independent
of WAN/LAN role; /0 public sources and mixing legacy interface `management`
flags with explicit policy are rejected. Without the field, legacy behavior is
preserved. An explicit policy permits one-NIC plans, but does not start listeners,
prove reachability or enable mutations. No LAN means no implicit DNS service.
See [ADR-0013](../../docs/adr/0013-independent-management-policy.md).
