# ADR 0016: Context-bound encrypted secret records

Status: Accepted for encrypted records and bounded storage; Vault activation remains gated.

## Decision

Secret bytes are separate from canonical configuration. Each encrypted record is
bound to its opaque resource ID, credential revision, purpose and master-key ID.
Decryption requires the expected reference from canonical state, not merely the
reference supplied by an untrusted encrypted record.

The portable encryption component uses a 32-byte random master key. Each write
generates a fresh 32-byte random salt and derives a distinct AES-256 key with
HKDF-SHA256 using a versioned DuckOS domain and the complete reference as context.
AES-GCM with standard-library generated random nonce authenticates the same
context. The nonce is carried with ciphertext; the salt is public. A derived key
is used for one encryption, avoiding a shared per-master-key GCM nonce counter.
No password is accepted as a master key and no home-grown cipher is introduced.

Records are bounded, versioned and strictly decoded. Unknown versions, malformed
references, altered metadata, wrong keys, truncation or authentication failure
return no plaintext. Plaintext wrappers redact routine logging and reject implicit
JSON serialization. Explicit reveal operations belong only at authorized use
boundaries. Clearing temporary byte slices reduces retention but is not a claim
that Go can erase all compiler/runtime copies from memory.

## Trust and activation requirements

Encryption is not a substitute for ownership and mode checks. The intended Linux
owner is the privileged Agent, with root-only state/key directories and no master
key in the unprivileged API process. A file-backed unattended-boot key does not
protect against root compromise or theft of a disk containing both key and data.
Hardware sealing may add protection where available, but is not an ARM/amd64
baseline dependency.

The file-backed key custodian provides explicit provisioning into an empty 0700
directory, owned by the serving identity. It creates a 0600 exclusive file and
syncs both file and directory. Loading requires a regular, single-link 32-byte
file with matching ownership; symlink directories/files and permissive modes
are rejected. Missing or partial keys are not regenerated. A failed provisioning
write leaves its evidence for recovery instead of silently replacing it.

The custodian passes the raw key only into the encrypted store and redacts its
ordinary diagnostics. Linux service/installer binding, missing-key recovery and
key-generation migration remain required before activation. No root key has been
provisioned on a deployment host by these library tests.
Master-key rotation must retain the old key until all live and rollback references
are safely migrated. Client credential rotation is a separate transaction and
does not become effective until the serving network service is verified.

Ordinary backups exclude raw keys. Secret-inclusive backups require an explicit
encrypted recovery envelope and tested restore; copying ciphertext alone is not
a recoverable backup without its key. F-004 remains open for this integration.

## Durable secret revisions

The portable store uses the bounded private snapshot mechanism from ADR 0006.
Initialization is explicit and requires an empty directory. Normal open requires
an existing database and checks the supplied key against a domain-separated key
fingerprint, including for an empty store. Wrong keys and corruption never reset
the database. The directory belongs to the serving identity; deployment must bind
that identity to root in the privileged Agent.

Each Put adds an immutable random revision and returns a reference only after
durable commit. Existing revisions remain available for last-known-good recovery;
network activation and revocation are separate transactions. Pending creation is
rolled back on restart. A persistence failure closes the store to further reads
as well as writes until reopened. The generic journal contains ciphertext only,
including its bounded history and pending data.

The store allows at most 128 immutable records and 256 KiB of current encoded
configuration. Capacity exhaustion fails without deleting records. Reference-aware
garbage collection is deliberately not enabled until current, pending, rollback
and backup dependencies can all be accounted for. It must never prune a key still
needed to restore management access. This limit is an implementation bound, not
a target product client-count limit.

## Verification

Tests must cover byte-for-byte round trips, randomized ciphertext, independent
record/reference substitution, wrong key, metadata/ciphertext tampering, strict
size limits and secret-safe diagnostics. Native key-file/backup lifecycle tests
are separate qualification gates.

## References

- [Go authenticated encryption](https://pkg.go.dev/crypto/cipher#NewGCMWithRandomNonce)
- [Go HKDF](https://pkg.go.dev/crypto/hkdf)
