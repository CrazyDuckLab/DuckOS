# ADR-0002: Agent privilege boundary

- Status: Accepted
- Date: 2026-09-07
- Scope: all privileged DuckOS host operations

## Context

DuckOS manages firewall, interfaces, routes, services, updates, and other host
state that can require elevated Linux privileges. Exposing those privileges to
the browser, general API process, or a command-shaped RPC would turn ordinary
input-validation failures into arbitrary root execution and make policy,
rollback, and audit behavior impossible to reason about.

The project architecture separates untrusted clients, unprivileged product
semantics, and privileged host mutation. A compromised API/Engine caller is
part of the threat model, so validation performed before IPC is not sufficient.

## Decision

1. Browser/UI and Duck API remain unprivileged and never edit managed Linux
   configuration or invoke privileged commands directly.
2. Duck Engine owns product policy, canonical-state mutation, validation,
   dependency/conflict analysis, transaction planning, verification,
   confirmation, and rollback orchestration.
3. A separate privileged Duck Agent is the only ordinary DuckOS component that
   performs the approved set of privileged host mutations.
4. Engine-to-Agent communication uses an authenticated, versioned, local,
   semantic IPC contract. Operations describe bounded intent such as applying a
   validated firewall plan, configuring an owned interface, restarting a
   managed service, or reading bounded hardware state.
5. The Agent independently validates message shape, protocol compatibility,
   caller identity, operation bounds, target ownership, and applicable
   preconditions. It returns typed, size-bounded, secret-sanitized results.
6. The contract never accepts arbitrary shell text, a caller-selected
   executable, unrestricted file paths, or a caller-controlled process
   environment. There is no production `RunShell` or generic `execute`
   operation.
7. When a native API is impractical, an Agent adapter may invoke a reviewed,
   fixed executable using a constructed argument vector, minimal environment,
   bounded time/output, and adapter-specific validation and rollback. It does
   not use shell interpolation or place secrets in process arguments.
8. A future full Web Terminal, if approved, is a separate optional security
   boundary with its own ADR and review. It must not widen the Agent IPC into an
   arbitrary execution channel.

## Consequences

- Compromise of the web/API process does not automatically grant a general root
  shell, though misuse of allowed operations remains a threat that the Agent
  contract and OS sandbox must minimize.
- Every new privileged operation needs an owner, input limits, ownership rules,
  idempotency/failure behavior, verification, rollback semantics, audit policy,
  secret review, and adversarial tests.
- The Agent stays small and mechanism-focused; it does not become a second
  policy engine or an alternative canonical-state writer.
- UI, external API, CLI, restore, migration, and automation all retain the same
  Engine semantics rather than gaining privileged shortcuts.
- Structured plans and results make failures, recovery, and audit more
  comprehensible than shell scripts and parsed terminal output.

## Deliberately unresolved

This ADR accepts the boundary and contract properties but does not close F-002.
The following remain open before Agent implementation:

- exact IPC transport and encoding (a Unix-domain socket is preferred, not yet
  a finalized wire contract);
- peer-credential and request-authentication mechanism;
- protocol negotiation, compatibility window, replay/staleness behavior, and
  message size limits;
- service users/groups, Linux capabilities, filesystem ownership, systemd
  sandboxing, seccomp, and namespace policy;
- operation catalogue and per-operation idempotency/rollback design;
- API/Engine deployment topology (F-001) and the host ownership contract
  (F-006/F-007).

These details require their own threat analysis and runtime tests on supported
Debian targets; this ADR alone is not security acceptance evidence.
