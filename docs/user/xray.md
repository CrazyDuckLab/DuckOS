# Xray

DuckOS exposes an Xray subsystem in Full Mode under **VPN** →
**Xray**. Like WireGuard, Xray activation through Safe Apply is a
qualified subsystem; the V1 beta renders the observed state.

## Concepts

- **Inbound** — the inbound listener.
- **Outbound** — the upstream proxy configuration.
- **Routing rules** — per-domain routing rules that decide which
  outbound to use.
- **Subscription** — a subscription provider URL. The subscription
  URL is a credential and is treated as a secret.

## Workflow

1. Open **Xray** in Full Mode.
2. Confirm the inbound and outbound configuration.
3. Add or update routing rules.
4. When the activation gate opens, run Safe Apply.

## Secrets

Subscription URLs and credentials live in the encrypted vault. They
are never displayed in plain text. QR codes that embed subscription
URLs are redacted.

See [`docs/security-model.md`](../security-model.md).