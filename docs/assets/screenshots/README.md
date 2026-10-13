# Public screenshots — index

Real captures against the installed V1 beta (`v0.1.0-beta.1`) RC
backend on a clean Debian 13 amd64 VM (`duck-beta-01`). No concept art,
no mock UI, no static fake JSON.

## v0.1.0-beta.1 launch captures

| File | Page shown | Real installed backend | Sanitized demo data | Build / release |
|---|---|---|---|---|
| [`10-first-run-enroll.png`](10-first-run-enroll.png) | First-run Owner enrollment (pre-Owner state, no users in identity store) | YES | YES | `1.0.0-dev.f698de16b4a9` from commit `f698de16b4a90c52d7bd5b50f287ad01f36da5f3` (public release `v0.1.0-beta.1`). |
| [`01-easy-home.png`](01-easy-home.png) | Easy Mode home dashboard | YES | YES | same |
| [`02-system-overview.png`](02-system-overview.png) | Full Mode System Overview | YES | YES | same |
| [`03-interfaces.png`](03-interfaces.png) | Full Mode Network → Interfaces | YES | YES | same |
| [`04-routing.png`](04-routing.png) | Full Mode Routing → Routes / RIB / FIB | YES | YES | same |
| [`05-firewall.png`](05-firewall.png) | Full Mode Security → Firewall Rules | YES | YES | same |
| [`06-wireguard.png`](06-wireguard.png) | Full Mode Security → WireGuard Servers | YES | YES | same |
| [`07-search.png`](07-search.png) | Global search (`⌘K` / `Ctrl+K`) with query `eth0` open | YES | YES | same |
| [`08-inspector.png`](08-inspector.png) | Page-help Inspector (`?`) on System Overview | YES | YES | same |
| [`09-safe-apply.png`](09-safe-apply.png) | Full Mode System → Candidate and Safe Apply | YES | YES | same |

## Filename convention

`NN-page.png`, where `NN` orders the launch set and `page` is the
public-facing page label.

## Capture method

Each capture used a temporary headless Chromium for Testing
(Playwright-managed) running on the qualification host, connected to
the real installed public RC on `duck-beta-01` over
`ssh -L 8080:127.0.0.1:8080 <duckos-ip>` (the public release docs use this placeholder; the actual capture target was the labeled lab host `duck-beta-01`). The Owner was created on the
real backend via `POST /api/v1/auth/enroll` with the bootstrap token
taken from the installer's `/etc/duckos/api-token` and the session
established via `POST /api/v1/auth/login`. No browser tooling was
committed to the repository.

The browser context was always `1440x900`. Theme was not toggled.
No browser devtools, no credentials in view, no API tokens, no
password fields were rendered. Lab hostname `duck-beta-01` and lab
addresses are the only internal details visible and are demo data.

## Sanitization rules

- No API tokens, session tokens, or passwords in any visible panel.
- No production / customer / partner / personal data.
- No developer private filesystem paths.
- No browser devtools or debug overlays.
- Lab hostname and lab addresses are acceptable demo data.

## Observed runtime state at capture time

Captured against the installed public RC, `GET /api/v1/overview`
returned:

```json
{
  "hostname": "duck-beta-01",
  "kernel": "6.12.107+deb13-amd64",
  "os": "Debian GNU/Linux 13 (trixie)",
  "architecture": "amd64",
  "environment": "test",
  "interfaces": ["lo:up", "eth0:up", "eth1:up", "eth2:up"],
  "default_route": "default via [redacted management address] dev eth0"
}
```

## Navigation behavior (corrected)

The V1 beta SPA uses hash-based routing. The HTML document `title` is
set to `DuckOS · System overview` on initial load and does not change
between pages — that title is the document's static title, not a
page-state title. The **body content** does change between routes:

- `01-easy-home.png` is the Easy Mode landing.
- `02-system-overview.png` through `09-safe-apply.png` are distinct
  Full Mode pages reachable through the navigation rail.

A previous documentation pass claimed the SPA "renders System
Overview on every route." That claim described the document title
only and was imprecise for the live body content. The corrected
description is: same document title, distinct page content per
route, hash-based SPA navigation.

## Future additions

Additional screenshots may be added without renaming any current
file. Suggested next additions:

- A capture of the search results panel after `eth0` was selected
  (currently `07-search.png` shows the open search with the query
  typed but the result row not yet activated).
- A capture of the dashboard in production-flavored theme if a
  theme toggle is exposed in a future beta.
- A capture of Easy Mode Wi-Fi panel after a Wi-Fi scan, if Wi-Fi
  scan becomes observable in the lab VM.

Each future capture must come from a real browser against the
installed public RC, with the same sanitization rules.