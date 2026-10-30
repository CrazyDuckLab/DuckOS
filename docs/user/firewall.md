# Firewall

Beta.3 uses nftables through the bounded privileged nft launcher. The UID-0
Agent keeps zero effective capabilities. Ownership is split between
`inet duckos` (network Safe Apply), `inet duckos_fw` (typed forward rules) and
`inet duckos_upnp` (NAT mappings). Foreign objects are outside the mutation scope.

Typed rules are forward-only. Their controller keeps a durable journal, applies
an atomic batch, checks semantic readback and requires confirmation; timeout or
failure retains a rollback path. Empty candidates remove the owned table.
Preserve the management interface and keep console access. The network forwarding
guard is owned by the network Safe Apply path; do not release it manually.

Kernel packet and coexistence tests are part of accepted AMD64 Router Core
qualification. This does not qualify every UI workflow, input/output filtering,
arbitrary policy or physical hardware. See [Safe Apply](safe-apply.md),
[recovery](recovery.md) and [qualification](../qualification-beta3-final.md).
