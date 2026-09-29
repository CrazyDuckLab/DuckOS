# Identity API v1 contract

All routes use the API's exact Host/Origin boundary, no-store headers, bounded
concurrency and same-origin browser requests. No cookies or query credentials.
JSON mutations require `Content-Type: application/json`, one object, no unknown
fields, and at most 4096 bytes. Unknown routes/methods fail closed. The legacy
observation API remains available when local identity is explicitly disabled.

| Method/path under `/api/v1/auth` | Input/credential | Result |
|---|---|---|
| GET `/status` | None | `{mode:"local-sessions",enrolled:boolean}` |
| POST `/enroll` | Installation bearer; `{username,password}` | 201 User; succeeds once only |
| POST `/login` | `{username,password}` | `{token,session}`; bounded login budget |
| GET `/me` | Session bearer | Session |
| POST `/logout` | Session bearer; `{}` | `{ok:true}`; revokes current token |
| GET `/sessions` | Session bearer | Own sessions; Owner may see all |
| POST `/revoke` | Session bearer; `{id}` | Revokes own session, or any for Owner |
| GET `/users` | Owner session | User projections, never password hashes |
| POST `/users` | Owner; `{username,password,role}` | 201 User |
| PUT `/users` | Owner; `{id,role,disabled}` | `{ok:true}`; target sessions revoked |
| POST `/password` | Session; `{oldPassword,newPassword}` | `{ok:true}`; all own sessions revoked |
| GET `/audit` | Owner session | Up to 256 identity-change audit entries |

User: `{id,username,role,disabled}`. Role values: `owner`, `network-admin`,
`helpdesk`, `reception`, `read-only`. Unknown roles/permissions deny. Session:
`{id,user,createdAt,expiresAt,lastSeen}`; timestamps are RFC3339. Session IDs
are identifiers, not authentication tokens. Audit: `{at,actor,action,target}`;
no password, token or caller-controlled free-form metadata is recorded.

After enrollment Overview/readiness require a session permitted to observe;
the bootstrap credential is no longer accepted. Health remains public liveness.
Reception does not gain administrator Overview access. Its dedicated guest
workspace remains to be integrated.

Errors retain the existing `{error:{code,message}}` envelope: 400 invalid input
or rejected change, 401 invalid/expired credentials, 403 forbidden, 404 unknown,
405 method, 415 content type, 429 concurrency/login budget. Errors never reflect
passwords, private filesystem contents or decoder input. Authentication store
durability failures deny further identity authorization until local recovery.

Contract enforcement tests: `internal/api/identity_test.go` and
`internal/identity/service_test.go`. This document does not claim final MFA,
TLS, recovery, custom roles, authentication-event audit or user management UI.
