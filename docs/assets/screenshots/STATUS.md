# Public screenshots — current status

## V1 beta qualification — observed state from a clean installed artifact

The V1 beta artifact was qualified on a clean Debian 13 / amd64 VM
(`duck-beta-01`) from the final release-candidate HEAD. The installed
real backend returned the following observed state via
`GET /api/v1/overview`, which the SPA renders on every SPA route for
this V1 beta build:

```json
{
  "hostname": "duck-beta-01",
  "kernel": "6.12.107+deb13-amd64",
  "os": "Debian GNU/Linux 13 (trixie)",
  "architecture": "amd64",
  "environment": "test",
  "interfaces": ["lo:up", "eth0:up", "eth1:up", "eth2:up"],
  "default_route": "default via 172.18.1.254 dev eth0"
}
```

## Captured screenshots

| File                          | Source                                                     |
|-------------------------------|------------------------------------------------------------|
| `system-overview.png`         | Real browser (Playwright headless Chromium) via SSH tunnel to `127.0.0.1:8080` on the installed backend, captured after Owner enrollment against the live V1 beta artifact. |

The screenshot shows the SPA's System Overview pane rendered against
the live installed backend, displaying real CPU/memory/identity data
and the lab hostname `duck-beta-01`. Lab hostname and lab addresses
are acceptable as demo data per `docs/launch/screenshots.md`.

For this V1 beta build, the SPA renders System Overview for every
route the browser requests; route-specific captures are gated on the
V1 SPA wiring more of the public catalog. Additional captures per
the filename convention below will be added as the SPA exposes them.

## Filename convention (full launch set)

Public screenshots, when added, use the following names so the README
can reference them directly without changing links:

| File                       | Screen                              |
|----------------------------|-------------------------------------|
| `easy-home.png`            | Easy Mode home                      |
| `system-overview.png`      | Full Mode System Overview           |
| `interfaces.png`           | Interfaces list                     |
| `routing.png`              | Routing                             |
| `firewall.png`             | Firewall / NAT                      |
| `wireguard.png`            | WireGuard tunnels                   |
| `search.png`               | Global search                       |
| `safe-apply.png`           | Safe Apply candidate                |
| `theme-dark.png`           | Dark theme example                  |
| `theme-light.png`          | Light theme example                 |

Sanitization rules from `docs/launch/screenshots.md` apply. Each image
must be captured against an isolated test environment with labeled
fixtures.

## V1 beta qualification — capture before live capture

Browser-driven capture is now recorded in the qualification report.
Future launches must re-capture by a human with a browser against the
same backend as part of the launch capture checklist.