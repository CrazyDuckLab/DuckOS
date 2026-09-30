# Multi-WAN and selective routing

Design guide; current IP policy compilation does not implement live multi-WAN.

Use selective routing when particular clients, networks or destinations need a
specific uplink/tunnel while other traffic follows the normal path. Record match
precedence, route group, health conditions, stickiness and explicit fail-open or
fail-closed behavior. A disappeared route must not silently leak traffic through
an unintended default. Domain/service classification has CDN, DNS and QUIC limits;
it is not perfect application identity and does not justify HTTPS interception.

A multi-WAN gateway needs more than link-up or one successful ping. Test gateway,
upstream/DNS failures, partial loss, recovery hysteresis and established-session
behavior. Account for asymmetric return traffic and management route selection.
Observe actual client packets in both families before accepting a policy.

Current [preview API](../../shared/api/network-preview.md) generates bounded
private-table rules with unreachable guards. It does not install them or select
healthy providers. See [policy decision](../adr/0012-fail-closed-ip-policy-routing.md),
[requirements](../product-spec-1.0.md) and [testing](../testing.md).
