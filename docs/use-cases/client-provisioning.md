# VPN client provisioning and subscriptions

Requirements guide; provisioning/export endpoints and final UI are not available yet.

The intended flow is client → profile → selected nodes/protocols → renderer.
Profiles may select multiple nodes. Supported HAPP subscription output must share
the same generic model as future Xray, sing-box or Clash output; compatibility
requires a tested client/version, not a label alone.

Subscriptions provide Show QR, Copy URL and Export. Their QR payload is exactly
the generated subscription URL. URLs carrying credentials are bearer secrets.
WireGuard provides Show QR, Download .conf, Copy config and Rotate keys; its
complete standard mobile-client configuration must be identical in all exports.
Changing a token, URL or key invalidates the corresponding export.

Anyone who scans a working WireGuard QR can use that connection. Authenticate
and authorize viewing/export, audit metadata without the secret, and never save
QRs as public assets. Use a revoked disposable client for documentation images.
Do not paste configuration, private keys or subscription URLs into issue reports.

Provisioning does not prove a listener is reachable. Review service Port Exposure,
upstream firewall and actual external tests separately. See
[edge nodes](edge-node.md), [security](../security-model.md) and
[complete requirements](../product-spec-1.0.md).
