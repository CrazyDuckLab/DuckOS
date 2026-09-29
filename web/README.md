# Duck Web

DuckOS's React/TypeScript workspace reads the versioned System Overview API.
It has no browser-to-Linux operation, configuration writer, runtime fixture, or
third-party network request. This is an internal development build toward
DuckOS 1.0, not a published 1.0 release.

## Development

Use the repository's pinned Node toolchain, then run from this directory:

```sh
npm ci
npm run dev
```

Vite binds to `127.0.0.1`. Its development-only `/api` proxy targets the local
API at `http://127.0.0.1:8080` and rewrites Host/Origin for the API's strict
same-origin validation. Production does not use Vite or this proxy: the API
serves the built static files and API routes from the same origin.

The administrator token is entered through the connection screen and retained
only in this tab's memory. It is sent in the Authorization header, never in a
URL, browser storage, cookie, or analytics event. Disconnecting or reloading
ends the UI session.

## Verification

```sh
npm run typecheck
npm test
npm run build
```

Production assets are written to `dist/` for the root build tooling. Tests use
explicit synthetic TEST observations in `src/test/fixture.ts`; the production
dependency graph does not import this file.

## Interaction and visual foundations

- One real workspace: Overview, Interfaces, Routing, DNS, and Services.
- Polling every five seconds, no overlapping requests, a fifteen-second
  deadline, manual refresh, last-observation timestamps, explicit stale data,
  and component-specific observation issues.
- Measured zero is distinct from unavailable CPU, memory, storage, and system
  observations. Ruleset visibility does not claim firewall protection;
  interface administrative state does not claim carrier or reachability.
- Interface search includes addresses and hardware addresses. Routing has
  destination/device/gateway search and independent IPv4/IPv6 filtering.
- A local-first dark visual system: navy surfaces, restrained teal accents,
  shallow glass depth, native system fonts, and repository-owned SVG graphics.
- Semantic sections, keyboard navigation, visible focus, live status messages,
  reduced-motion support, horizontally scrollable route tables, and mobile
  navigation. Glass degrades to opaque surfaces when blur is unsupported.

There are no mutation controls, reachability checks, traffic charts, resolver
upstream discovery, or security conclusions in this observation slice.
