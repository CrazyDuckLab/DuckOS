# Fit and migration planning

DuckOS is not currently a production replacement or a drop-in configuration
importer for another gateway. No performance or security superiority is claimed.

For administrators coming from proprietary appliances, Kerio-style gateways,
general-purpose firewall distributions or OpenWrt, evaluate concrete needs:
hardware/driver support, deployment model, identity, VPN protocols, filtering,
portal workflows, updates, rollback and operational support. Current DuckOS uses
supported Debian plus separate unprivileged control plane and privileged Agent;
it does not supply its own kernel, bootloader or appliance image.

For VPS administrators assembling networking manually, the intended benefit is
one canonical policy and recoverable lifecycle, not unrestricted import of every
existing iproute2/nftables command. Existing managers and firewall ownership must
be identified. Unsupported semantics must be reported, not silently approximated.

Inventory current functions and export a protected backup before planning any
cutover. Map requirements to the [scope matrix](../product-spec-1.0.md), mark
unimplemented/untested functions as blockers, and build an isolated acceptance
lab. Retain the old gateway and out-of-band recovery until actual client traffic,
security and rollback tests pass. There is no current automatic migration command.

Product-specific comparisons should be added only with dated primary documentation
and matched tested versions. See [installation](../install.md),
[security](../security-model.md) and [hardware](../hardware-support.md).
