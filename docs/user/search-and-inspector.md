# Global search and Inspector

DuckOS has **one** global search and **one** Inspector pane. There is
no second command palette, no second omnibox, no second
context-sensitive help.

## Search

Press `⌘K` (macOS) or `Ctrl+K` (anywhere else) to open the search
overlay. Search covers:

- Pages — every page in Full Mode and Easy Mode.
- Help — the contextual help entries.
- Real objects observed in the current Overview — interfaces, IPs,
  MACs, VLANs, hostnames, services. The search index lives in
  `web/src/search/objects.ts`.

Search results are highlighted as you type. Selecting a result jumps
to the relevant page or opens the relevant contextual help.

## Inspector

The Inspector is the question-mark icon in the top bar. It opens a
contextual reference for the page you are currently on. The Inspector
explains:

- What the page is for.
- What each field means.
- Which other pages are related.
- Which configuration concepts the page touches.

The Inspector does **not** make changes. It is read-only.

## What you should NOT do

- Do not create a second search bar. There is one. Use it.
- Do not bypass the Inspector for a "faster" workflow. The Inspector
  is the canonical reference for the current page.