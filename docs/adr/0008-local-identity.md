# ADR-0008: Local identity, sessions and role enforcement

- Status: Accepted for implementation under the full-product request
- Date: 2026-09-07

Management identity is local and does not depend on cloud availability. The
installation bootstrap token may enroll the first Owner only; it must not gain
network-write permissions. Once an Owner exists, enrollment is closed. Identity
and bounded audit metadata live in a private independent durable store. Password
hashes are never included in public user projections or configuration exports.

Passwords use standard-library PBKDF2-HMAC-SHA256 with 600,000 iterations,
independent random 128-bit salts and 256-bit output. This avoids an additional
cryptographic runtime dependency on ARMv7; it is not a FIPS certification claim.
Versioned hash metadata permits migration to a future memory-hard scheme.
Passwords allow spaces and Unicode, require at least 15 code points and are
bounded to 128 code points/512 bytes. Real ARMv7 latency must be measured before
qualification; weakening the work factor silently is not permitted.

Opaque 256-bit random session tokens are held by the browser only in memory.
Only token digests are retained server-side, with an eight-hour absolute expiry,
30-minute idle expiry, at most 64 concurrent sessions, explicit revocation and
revocation on identity changes. Sessions deliberately expire on process restart.
Login attempts are globally bounded before the KDF and additionally constrained
by the API's concurrency boundary; unknown users use an equivalent dummy hash.
HTTP integration must enforce exact same-origin and HTTPS before broader binds.

Roles are Owner, Network Admin, Helpdesk, Reception and Read Only. Permissions
are explicit and deny unknown operations. Only Owner manages users, security,
updates and backup secrets. The last active Owner cannot be removed or demoted.
Reception permissions do not imply network or administrator access. Custom
roles, MFA/passkeys and recovery credentials remain required future integration,
not silently implemented by these initial built-in roles.

Identity changes are local atomic commits, not Linux Safe Apply transactions;
an interrupted uncommitted identity candidate is discarded at startup. Any
future network operation must separately require network-write authorization
and traverse the Engine/Agent transaction boundary.

Reference: [OWASP password storage guidance](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).
