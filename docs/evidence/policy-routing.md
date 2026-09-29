# IP policy routing compilation evidence

Date: 2026-09-09. Internal development, not native network acceptance.

Portable race tests cover IPv4 and IPv6 selectors, reserved table/priority
bounds, matching route references, duplicate route/rule refusal, canonical CIDR
validation, deterministic connected-network guards, management-LAN anchoring,
explicit root ownership grants and refusal by the firewall-only adapter.
Generated daemon artifact sizes are bounded after policy expansion.

The opt-in Linux namespace test translates only generated allowlisted networkd
rule fields to kernel `ip rule` commands. It creates a test-owned dummy link,
a working main default and a private default. Lookup succeeds through the
private table; after that route is removed, lookup must fail despite a working
main fallback. Removing the unreachable guard must then restore fallback,
showing why the paired guard is necessary. The namespace is disposable; no
test-owned interface is deleted unless this invocation created it successfully.

The test compiles for Linux amd64 on Mac. Native execution and CI results have
not been verified here. This is intended RPDB route-selection evidence, not
networkd file parsing/reload or packet forwarding qualification. Actual routing
mutations still require enrolled kernel inventory, transactional traffic guards,
bounded capture/restore and native lifecycle/traffic tests before activation.
