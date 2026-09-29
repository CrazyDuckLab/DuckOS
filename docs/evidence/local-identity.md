# Local identity integration evidence

Date: 2026-09-08. Internal development toward DuckOS 1.0, not a public release.

Implemented: private persistent identity store; one-time first Owner enrollment;
PBKDF2-SHA256 password verification; built-in roles; last-Owner protection;
bounded opaque sessions with idle/absolute expiry, enumeration/revocation;
password change with old-password verification and session revocation; bounded
identity-change audit; API operations; first-run/login UI; logout; installer
identity directory and narrowly scoped systemd write permission.

Evidence on Mac arm64:

- `make check` passed: Go/vet/race tests, 17 frontend tests, 22 installer tests,
  TypeScript/format/schema/shell checks. Linux-only runtime tests remain skipped.
- API tests exercise enrollment, bootstrap rejection after enrollment, login,
  authenticated Overview, user projection redaction, revocation, origin/method/
  body-size/unknown-field boundaries. Identity tests exercise durable reopen,
  session loss on restart, RBAC, last Owner, disabled users, password change,
  salt uniqueness, plaintext exclusion, rate and expiry bounds.
- `make build-all` passed for production frontend and Linux amd64/arm64/ARMv7.
- Actual Mac `duck-api` served the production UI and private identity store on
  127.0.0.1:8380. The first-run form was visually inspected at desktop and 375px;
  field spacing corrected. No user credential was entered. Browser and temporary
  server were stopped; ignored synthetic test state remains in `.cache`.

Limits: loopback HTTP via authenticated administrative tunnel remains the only
supported exposure. No TLS broad bind, MFA/passkeys, account recovery workflow,
custom roles, final user-administration/Reception UI, full authentication-event
audit, or real Linux installer/sandbox validation is claimed. Password KDF cost
requires measurement on ARMv7. Sessions expire on API restart. No network
mutations were enabled; no Linux target was accessed.
