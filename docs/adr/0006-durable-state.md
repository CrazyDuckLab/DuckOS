# ADR-0006: Durable bounded configuration snapshots

- Status: Accepted for implementation under the owner's continuation request
- Date: 2026-09-07

## Decision

The Engine's initial persistence mechanism is a bounded JSON snapshot, not an
external database. Each snapshot contains the committed revision, at most one
pending transaction, and up to 32 previous revisions. A revision is identified by
the SHA-256 digest of its canonical JSON payload. Configuration contains secret
references only. The schema-specific validator is required on writes and reads.

A single process holds an exclusive nonblocking filesystem lock for the store's
lifetime. Updates write a private temporary file, fsync it, rename it over the
snapshot, and fsync the directory. Readers never see a partially written JSON
document. A failed durability operation poisons the open store: reopen and
inspect durable state before attempting another mutation. Checksums detect
accidental corruption, not hostile modification by a process owning the store.

Snapshot format v2 also retains the last 32 transaction outcome receipts. A
receipt binds the transaction ID, before/candidate revision digests, completion
time and `committed` or `rolled-back` result. It is persisted in the same atomic
snapshot write that clears pending state. It records the coordinator's verified
decision, not separate proof of external Linux health. Expired retention means
unknown outcome; matching configuration content must never be treated as a
receipt or justification to replay a dangerous operation.

Readers accept v1 without modifying it. The next normal mutation atomically
upgrades to v2 while preserving revisions and any pending transaction. Binaries
that only support v1 must not be selected against v2 state, even with no pending
transaction. This applies to identity stores as well as network journals. A
future update installer must enforce shared-state format compatibility before
offering Active/Previous rollback; this change does not enable software updates.

The store is a private, pre-created directory, opened through `os.Root`; files
must be regular, private, and not symlinks. State is limited to 16 MiB, individual
configuration to 256 KiB. Temporary data is a single fixed owned file, never an
unbounded collection. Invalid state is an explicit recovery error, never a
reason to initialize empty configuration.

An operation durably records its before/candidate pair before a privileged
change. Preparing, applying, awaiting confirmation, rolling back, and recovery
required are distinct stages. The persistence component does not assert Linux
verification or perform networking itself. An independent Agent rollback
watchdog and reboot reconciliation are required before exposing Safe Apply.
No transaction is implicitly confirmed on restart. Revision restore goes
through candidate preparation, not direct replacement of the committed state.

The private Agent transaction may additionally contain at most 1 MiB of
adapter-validated JSON host recovery evidence, captured read-only before any
mutation and committed in the same journal write as the candidate. This is not
canonical desired state and is stripped from controller status/apply replies.
Adapters must exclude secret material and validate evidence before restoration.
A missing/incompatible recovery adapter fails closed, including on reboot.
Older binaries that do not understand this optional journal field must not be
selected during an unfinished transaction; update compatibility must account
for this before any software rollback is enabled.

This resolves the initial F-003 storage mechanism. It does not resolve Vault,
network ownership, or host-package rollback. Those require separate decisions
before their corresponding adapters are enabled. A future indexed history
database may supplement but must not silently replace this recovery boundary.

## Verification

Tests cover exclusive ownership, invalid/truncated/checksum-mismatched input,
optimistic concurrency, pending transaction restart, rejected transitions,
bounded history, immutable caller data, and failed writes. Mac tests establish
the algorithm only; Linux filesystem power-loss and disk-full qualification
remain mandatory and cannot be inferred from fsync calls.
