# Public launch readiness

Preparation only. Repository visibility, metadata changes, announcements,
directories, community posts, releases and public indexing require separate
approval. No publishing or deployment workflow is installed here.

## Factual positioning

DuckOS is a Linux network operating platform for routers, gateways and VPS/edge
nodes, currently in development. Its intended open-source release combines a
web interface with network policy and recoverable configuration on supported
Debian. Public copy must distinguish implemented, qualified and planned features.
No claims of being fastest, most secure or a universal appliance replacement
are supported.

**License**: DuckOS Community source is licensed under
**GPL-3.0-only** ([`LICENSE`](../../LICENSE), [`COPYRIGHT.md`](../../COPYRIGHT.md)).
Trademark use is governed separately by
[`TRADEMARKS.md`](../../TRADEMARKS.md); commercial / dual-licensing
narrative is in [`docs/licensing.md`](../licensing.md). The CLA is a
draft ([`CLA.md`](../../CLA.md)) and is not yet active for accepting
external code contributions.

## Repository metadata

[project.json](project.json) holds the candidate description, architecture list,
route map and topics. There are 20 primary topics; `open-source` is an alternate
only after licensing, replacing rather than adding a twenty-first topic.
Topics are not currently applied to GitHub. Topic names themselves can be public
even on private repositories; this is another reason to defer metadata changes.
See [GitHub topic rules](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/classifying-your-repository-with-topics).

## Future documentation site

The route map covers introduction, quick start, installation, network/firewall,
routing, WireGuard/Xray, subscriptions/clients, edge/multi-WAN, monitoring,
diagnostics, updates/backup, security/API, hardware and contribution guides.
Some routes deliberately share a requirements guide until qualified operational
documentation exists. A route is not evidence that its feature works.

Canonical site URL and release version remain unset. No placeholder domain is
published as an official endpoint. Future rendering must resolve internal links
through this map and supply canonical URLs, Open Graph metadata, structured data,
sitemap and release/news feed from reviewed content. The metadata helper validates
routes and generates only private/noindex metadata; it has no deployment action.
Its pure preview exporter can generate a sitemap and empty Atom feed for an
explicit authenticated staging origin, retaining noindex and without inventing
a public release or canonical project domain. No artifacts are published by tests.
The preview formats follow the [Sitemap protocol](https://www.sitemaps.org/protocol.html)
and [Atom RFC 4287](https://www.rfc-editor.org/rfc/rfc4287).

Use SoftwareSourceCode metadata for the repository and SoftwareApplication for
the product, without fabricated versions, license, ratings or download links.
See [Schema.org SoftwareSourceCode](https://schema.org/SoftwareSourceCode).
The root [llms.txt](../../llms.txt) is a concise document map, not instructions for
an agent and not a substitute for ordinary linked documentation.

Private/staging sites require authentication; robots.txt and noindex are not
access controls. Keep `X-Robots-Tag: noindex, nofollow, noarchive` and a restrictive
robots policy as defense in depth. A robots crawl block can prevent a crawler
from seeing noindex, so neither replaces access restriction. See
[indexing guidance](https://developers.google.com/search/docs/crawling-indexing/block-indexing).

## Launch gate

- Complete the technical 1.0 matrix, security review and native recovery evidence.
- License is selected (GPL-3.0-only); review dependencies and confirm
  support/reporting channels.
- Review README, use cases, compatibility reports and migration claims against evidence.
- Capture only [qualified screenshots](screenshots.md); inspect secret-bearing QR content.
- Approve canonical domain, public content, repository visibility and topic selection.
- Prepare signed artifacts and [release notes](release-notes.md), including known issues.
- Review sitemap/feed entries and indexing behavior on an authenticated staging build.
- Obtain explicit publication approval before any external operation.

Until then, the public launch flags remain false and no release/news entry exists.
