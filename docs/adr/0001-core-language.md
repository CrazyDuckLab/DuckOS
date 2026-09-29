# ADR-0001: Core implementation languages

- Status: Accepted
- Date: 2026-09-07
- Scope: initial DuckOS 1.x implementation

## Context

DuckOS must deliver one product across Linux ARMv7, ARM64, and amd64, including
the resource-constrained ODROID XU4 reference target. The control plane needs
small deployable artifacts, predictable resource use, concurrency appropriate
for system work, and access to Linux networking and service interfaces. The web
interface needs a maintainable typed application stack.

The frontend architecture uses React with TypeScript; Go is the preferred
backend/core language. A component may use another technology when there is
objective evidence that it is a better fit.

## Decision

1. Duck Web will be implemented with React and TypeScript.
2. Go is the default implementation language for Duck API, Duck Engine, Duck
   Agent, the capability/configuration core, and the `duck` CLI.
3. The default is an engineering constraint, not a claim that every repository
   file must use one language. Platform packaging, build definitions, schemas,
   migrations, tests, and generated assets may use the language or format
   appropriate to their role.
4. A runtime product component may deviate from these defaults only when its
   owner documents a concrete interoperability, safety, platform, or resource
   advantage and records the consequential change in an ADR before relying on
   it.
5. Adding a separate long-running runtime, framework, or native-language
   boundary requires evidence that the benefit outweighs its cross-build,
   packaging, update, security, and ARMv7 costs.

## Consequences

- Backend/core code can target the three supported Linux architectures as
  ordinary binaries without making a heavyweight application runtime part of
  the router baseline.
- Shared Go packages may express product semantics, but process and privilege
  boundaries still follow the architecture; sharing a language does not permit
  API code to bypass Engine or Agent contracts.
- TypeScript supplies a typed frontend boundary, while the API remains the
  authority for validation and authorization.
- Cross-compilation is only build evidence. Runtime, reboot, failure, and real
  network-path claims still require the evidence tiers in
  [`../testing.md`](../testing.md).
- Each additional runtime increases dependency, patching, storage, memory,
  packaging, and supply-chain obligations and therefore needs explicit review.

## Deliberately unresolved

This ADR does not choose:

- Go, Node.js, TypeScript, package-manager, or frontend framework versions;
- the React application framework, renderer, design system, or TanStack
  packages;
- API and Engine process topology (F-001);
- Debian/kernel/ABI and build-toolchain baselines (F-005);
- whether a narrowly scoped adapter eventually justifies another language.

Those choices must be resolved by the relevant foundation decision and verified
on ARMv7, ARM64, and amd64 before they become compatibility promises.
