# DNS/DHCP daemon preflight evidence

Date: 2026-09-09. Internal development; not network acceptance.

`CheckDNSMasq` renders independently validated intent and passes only bounded
generated content to the real Linux dnsmasq syntax checker. The command has
fixed arguments, no shell, no default host configuration, discarded output and
a bounded lifetime. Portable tests cover generated-content routing, empty
configuration, injection rejection before execution and propagation of failures.

The opt-in isolated Linux namespace workflow now installs `dnsmasq-base` alongside
nftables, then tests generated DNS/DHCP configuration and reservations with the
real binary. Deliberately invalid syntax must fail. The native test compiles on
Mac for Linux amd64; its execution is not claimed as locally passed.

Not yet proved: DNS service installation/lifecycle, real queries and lease
delivery, pool changes with existing leases, external resolver behavior, or
transactional DNS/DHCP recovery on Debian 13. No access to duck-dev-01 occurred.

Reference: [dnsmasq manual](https://thekelleys.org.uk/dnsmasq/docs/dnsmasq-man.html),
`--test` and stdin `--conf-file=-` semantics.
