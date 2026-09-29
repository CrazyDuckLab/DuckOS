# Pull request

## Summary

One-paragraph summary of what this PR does and why.

## Subsystem

Mark the primary subsystem:

- [ ] UI (React / TypeScript)
- [ ] API / Engine
- [ ] Agent
- [ ] Linux adapters (network / dns / firewall / wg / xray / etc.)
- [ ] Installer / packaging
- [ ] Documentation
- [ ] Tests / fixtures
- [ ] Tooling / build
- [ ] Other (describe)

## Scope discipline

- [ ] Scope is focused; no unrelated changes mixed in.
- [ ] No V1 scope expansion. New features follow [`docs/architecture.md`](../docs/architecture.md).
- [ ] Frozen V1 UI is not redesigned.
- [ ] API contracts are not changed without an ADR.

## Safety

- [ ] No network / auth / Safe Apply invariant broken (see [`AGENTS.md`](../AGENTS.md)).
- [ ] Privileged Agent IPC unchanged, or the change was reviewed against the API/Agent privilege boundary.
- [ ] Anti-lockout guard preserved for the management anchor.
- [ ] No secrets, tokens, real keys, or production addresses included.

## Tests

- [ ] Affected Go packages: `go test ./...` passes locally.
- [ ] Affected frontend packages: `tsc --noEmit` and `vitest run` pass.
- [ ] Linux/amd64 cross-build: `GOOS=linux GOARCH=amd64 go build ./...` passes.
- [ ] Async tests wait for state via `findBy` / `waitFor` / controlled promises.
- [ ] New / changed public docs are linked from `docs/README.md` and reviewed for broken links.

## Documentation

- [ ] Updated docs under `docs/` (architecture / adr / user / launch as applicable).
- [ ] Added entry to [`docs/README.md`](../docs/README.md) if a new doc was created.
- [ ] Updated `AGENTS.md` invariants if any changed.

## Reviewer notes

Anything you want the reviewer to focus on.