# Xray

DuckOS exposes Xray in Full Mode under **VPN** → **Xray**.
Beta.2 bundles xray-core **26.3.27** and qualified real VLESS/TLS and
VLESS/REALITY client data-plane traffic on AMD64. Credentials and keys use
Vault-backed materialization. Empty inbound renders stop the daemon rather
than reporting an active listener. See [beta limitations](../beta-limitations.md).

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
4. Use the supported apply flow and verify observed runtime state.

## Secrets

Subscription URLs and credentials live in the encrypted vault. They
are never displayed in plain text. QR codes that embed subscription
URLs are redacted.

See [`docs/security-model.md`](../security-model.md).