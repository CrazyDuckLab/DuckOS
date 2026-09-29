# ADR 0017: SQLite client catalog

Status: Accepted for canonical client metadata; runtime activation remains separate.

## Decision

Use SQLite for indexed canonical client metadata and its bounded revision/audit
history. This is an additional Engine-owned store, not a replacement for the
privileged Safe Apply recovery journal or the encrypted Vault. The catalog contains
secret references, never WireGuard private keys or credential-bearing URLs.

The pure-Go `modernc.org/sqlite` driver is pinned at v1.58.0 with its required
`modernc.org/libc` v1.75.6. Upstream lists Darwin arm64 and Linux amd64/arm/arm64;
DuckOS still requires its own build and runtime qualification. No host SQLite
package, C compiler or cgo runtime is required.

One process owns an exclusive private directory and one database connection.
Initialization is explicit; ordinary open refuses missing, foreign, corrupt or
unsupported schema versions. SQL statements are fixed and values are bound
parameters. Application ID and user version identify the format. Unknown schema
objects are rejected rather than executed as part of normal writes.

The initial catalog uses rollback journaling in DELETE mode and synchronous EXTRA,
not WAL. This avoids an unbounded WAL/checkpoint lifecycle for the single-connection
control-plane workload. Page size is 4096, database capacity is 32 MiB, cache target
is 2 MiB, memory mapping is disabled, and trusted schema is disabled. These settings
are verified after opening. A SQLite transaction is not proof of filesystem or
hardware power-loss behavior; native fault qualification remains necessary.

Client changes use optimistic revisions and atomically record the prior document
and a secret-free audit event. The catalog retains 32 prior documents per client
and 1024 audit events, with at most 1024 clients and 100 entries per list page.
Semantic comparisons distinguish name, desired enablement, profile association,
expiry and credential-reference changes without copying their values into events.
These are configuration changes, not claims that traffic or connectivity changed.
Desired enablement and expiry
do not establish runtime activation. A service coordinator must reconcile canonical
intent with verified Agent outcomes before offering a working connection/export.
Profile resolution and resource ownership remain Engine responsibilities.

The catalog is private even though it excludes raw secrets: names, identifiers,
profile associations and configuration history are operationally sensitive.
Backup must use a consistent database snapshot, not copy an open database file.
Schema migration and software rollback compatibility must be explicit; neither
an unknown user version nor a corrupt file is a reason to create an empty catalog.

## References

- [Driver architecture support and libc version requirement](https://pkg.go.dev/modernc.org/sqlite)
- [SQLite durability and resource settings](https://sqlite.org/pragma.html)
