# DuckOS GitHub presentation assets

These are repository-owned SVG source assets for the README. Edit SVG directly;
no external fonts, scripts, linked raster content or build tooling are required.
They are product artwork/architecture illustrations, not product screenshots.

- `hero.svg`: original navy/teal/gold composition. The mascot paths and gradients
  are reused verbatim from `web/public/duck.svg`; official DuckOS branding is preserved.
- `platform.svg`: selected control dependencies and privilege boundaries. Xray,
  WireGuard and FRR are separate paths. Other Linux adapters are omitted from this
  overview for clarity; see [expanded diagrams](../../platform-overview.md).
- `roadmap.svg`: provisional direction, with no dates or completed future-release claims.

The SVGs have opaque high-contrast backgrounds, accessible titles/descriptions,
standard system fonts and viewBox dimensions for proportional scaling in GitHub
light/dark themes. Mermaid source lives in [the platform overview](../../platform-overview.md)
and the README Safe Apply section. Changes to diagram claims require source review.

Actual interface captures remain under `../screenshots/` and retain their explicit
historical beta.1 provenance. No screenshot is generated or relabeled here.
