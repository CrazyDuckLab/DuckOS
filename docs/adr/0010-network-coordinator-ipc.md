# ADR-0010: Network coordinator identity and uncertain acknowledgements

- Status: Accepted for implementation; installed mutation activation remains gated
- Date: 2026-09-09

Engine owns canonical desired state in its private store. Agent owns a separate
private execution journal and independent Safe Apply controller. A random
128-bit transaction identity is persisted by Engine before one asynchronous
apply submission. Agent uses that same identity in its durable journal after
preflight and private host capture. Configuration digests alone are not commit
receipts. Neither an HTTP timeout nor process restart authorizes replaying apply.

The fixed Unix HTTP protocol exposes status, apply, confirm, rollback and
cancel-admission under `/agent/v1/network/`. Existing socket peer credentials
remain the authentication boundary. Engine verifies bounded status documents,
revision hashes and configuration schemas before accepting receipts. Agent
independently validates configuration, metadata, identity and admission age.
No executable, path, shell fragment or arbitrary Linux command is an operation.

Apply and confirmation execution survive disconnected HTTP clients. Status
reports bounded progress and receipts without private kernel recovery evidence.
Engine adopts a matching Agent outcome only when identity, before/candidate
revisions and Agent current state agree. Confirmation lost in transit can be
reconciled after restart without inventing a new commit or reversing an already
durable decision. An unresolved disagreement blocks further mutation.

Missing Agent pending state is not proof of rollback: an apply may still be in
admission. Engine may instead request idempotent cancellation of admission.
Under the same worker lock used by admission, Agent refuses cancellation while
work is active or pending, otherwise atomically records a rolled-back outcome
without changing host or current configuration. Delayed submissions carrying a
retained receipt identity are refused. Engine waits for that durable receipt;
it never clears intent merely on a successful cancellation HTTP response.
Receipts retain the existing 32-entry bound. Apply freshness is limited to one
minute with five seconds of future clock tolerance; cancellation can resolve
older intent. This is coordinator replay protection, not authorization to trust
an arbitrary client with access to the privileged socket.

This change defines and tests coordination, not complete Linux networking.
The installed Agent does not enable this service yet: explicit resource
ownership enrollment, the composite Linux adapter, service permissions,
management-path verification and native integration evidence are still required.
Portable integration tests use a clearly labelled synthetic host adapter with
real Unix HTTP transport and separate durable stores. They do not prove packet
forwarding, DHCP, interface rollback or kernel behavior.
