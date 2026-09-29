# WireGuard tunnels

DuckOS exposes a WireGuard subsystem in Full Mode under **VPN** →
**WireGuard**. The V1 beta renders the observed state and the
canonical model. WireGuard activation through Safe Apply is a
qualified subsystem; check [`docs/beta-limitations.md`](../beta-limitations.md)
before relying on it.

## Concepts

- **Server** — a DuckOS-hosted WireGuard server that clients connect
  to.
- **Peer** — a client or upstream peer with its own public key.
- **Endpoint** — the remote address and port the peer uses.
- **Allowed IPs** — the IPv4/IPv6 prefixes routed into the tunnel
  for that peer.
- **Private key** — kept in the encrypted vault. Never exported in
  clear.

## Workflow

1. Open **WireGuard** in Full Mode.
2. Confirm the server's address range, listen port, and DNS.
3. Add or rotate peers as needed.
4. When the activation gate opens, run Safe Apply.

## Xray

DuckOS also exposes Xray. See [`xray.md`](xray.md) for that subsystem.

## Secrets

WireGuard private keys are managed by the encrypted vault. They are
not displayed in the UI, not logged, not part of any QR code that
embeds the private key. Public keys and pre-shared keys are exported
through the secret-export flow, with explicit audit logging.

See [`docs/security-model.md`](../security-model.md).