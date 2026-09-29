# DuckOS development checkpoint

Checkpoint date: 2026-09-10. DuckOS V1 is not feature-complete or production-ready.
Development is local on the Mac; Linux targets are for later build/test evidence.

## Reading order

1. [Project state](docs/handoff/PROJECT_STATE.md): implemented behavior, limitations and tests.
2. [Current roadmap](docs/handoff/CURRENT_ROADMAP.md): exact next task and dependency order.
3. [Frozen V1 scope](docs/handoff/V1_SCOPE.md): complete requirements and Alpha classification.
4. [Architecture](docs/handoff/ARCHITECTURE.md): implemented boundaries and invariants.
5. [Engineering rules](docs/handoff/AGENT_RULES.md).
6. Read only the ADRs and implementation files needed for the selected roadmap item.

Continue from `CURRENT_ROADMAP.md`, not a new project-wide planning or redesign
exercise. The handoff files describe this checkpoint; newer verified commits may
supersede implementation status but must not silently change frozen requirements.

Use `make check`, `make build-all` and `make check-cross` for the local verification
baseline. Do not treat cross-compilation as Linux runtime qualification. Do not
operate `duck-dev-01`, configure Remote SSH, force-push, or publish an intermediate
release without new explicit authorization. An internal Alpha is a development
gate, not permission for a public release before 1.0.0.
