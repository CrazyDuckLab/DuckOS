# DuckOS technical mark

The main interface uses a six-column, two-line ASCII duck. Canonical source is
`pkg/brand/mark.txt`, embedded by Go and imported as raw text by the web build.
The standalone installer has the same literal, enforced by a contract test.

Selected:

```text
<(o)__
 (___/
```

Alternatives explored:

```text
 __
<(o )___
 ( ._> /
  `---'
```

This silhouette is recognizable but too tall and detailed for a compact sidebar.

```text
<o)__/
```

The one-line variant is smaller but reads as punctuation rather than a duck at
small sizes. The selected two-line variant keeps a beak, head, back and hull
without cartoon detail, emoji, Unicode-width ambiguity or an image dependency.

Web treatment uses a monospace stack, preserved whitespace, disabled ligatures
and restrained accent color. The mark is decorative beside accessible DuckOS
text; screen readers should not pronounce its punctuation. Sidebar, login,
first-run and loading states share the same component. Both executables offer
`--banner`; `--version` stays machine-friendly and logs do not repeat a banner.
Installer check-only output remains unchanged; actual installation displays it.

The former mascot was inline SVG in the component and the browser favicon.
The main UI drawing is replaced by text; the optional SVG favicon uses the same
ASCII text instead of a mascot silhouette. No unrelated assets were deleted.
