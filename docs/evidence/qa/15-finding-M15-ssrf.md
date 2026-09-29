# M-15 — SSRF review: outbound HTTP is internal-only, webhook delivery not implemented

**Severity:** LOW (no exploitable SSRF path exists today; webhook
URL field has minimal validation but no fetcher)
**Section:** §35 / SSRF / outbound request security
**Status:** Documented; webhook delivery must include SSRF
guardrails before merging
**Found during:** Phase E §35 static review

## 1. Inventory of outbound HTTP in DuckOS

Grep of all `http.NewRequest`, `http.Get`, `http.Post`, `http.Do`,
`http.NewRequestWithContext`:

- `internal/engine/engine.go` — all calls hardcode
  `http://agent` + a fixed path (e.g. `http://agent/api/v1/qos`).
  The agent is a Unix-socket-backed HTTP bridge in the same
  process tree; the URL is constant.
- `internal/engine/network_control.go` — same pattern.

**No user-controlled URL reaches `http.NewRequest` today.**

## 2. Dormant SSRF surface: webhook URL field

`internal/secpolicy.Webhook.URL` is a user-configurable string
that currently has only non-empty validation:

```go
if w.ID == "" || w.URL == "" || w.SecretRef == "" {
    return errors.New("secpolicy: webhook id+url+secret required")
}
```

There is no scheme check, host check, or block-list for
loopback / link-local / RFC1918 / metadata addresses.

The webhook delivery code does NOT exist yet — `WebhookStore`
just stores the URL. So the SSRF risk is latent.

When the webhook delivery is implemented (probably as an
`internal/webhook` or `internal/notify` package), it MUST:

1. Require `https://` or `http://` and parse cleanly.
2. Refuse loopback addresses (127.0.0.0/8, ::1).
3. Refuse link-local (169.254.0.0/16, fe80::/10) unless
   explicitly opted in for cloud metadata.
4. Refuse RFC1918 private space (10/8, 172.16/12, 192.168/16)
   unless explicitly opted in.
5. Refuse cloud metadata addresses (169.254.169.254,
   fd00:ec2::254, etc.).
6. Refuse schemes other than http/https (e.g. `file://`,
   `gopher://`, `ldap://`).

## 3. Recommended preventive test

Add a Go-level test in `internal/secpolicy` that rejects
webhook URLs pointing at:

- `http://127.0.0.1/`
- `http://169.254.169.254/` (cloud metadata)
- `http://[::1]/`
- `http://192.168.0.1/` (private)
- `file:///etc/passwd`
- `gopher://example.com/`

This test will fail until the validator is added, so it can
serve as a regression gate when the delivery implementation
lands.

## 4. Management boundary preserved

N/A (no runtime action).

## 5. Commits

- (review document only)
- `docs(qa): §35 M-15 SSRF review`
