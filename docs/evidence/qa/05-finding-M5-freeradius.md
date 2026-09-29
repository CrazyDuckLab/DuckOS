# QA §6: FreeRADIUS real runtime — infrastructure ready, config not yet passing

**Severity:** N/A (qualification) + M-5 (medium,
internal/radiusd Render is a partial config; the QA
harness hand-rolls the rest of the tree)
**Section:** §6 (Real FreeRADIUS)
**Status:** PARTIAL on duck-dev-01

## 1. What was exercised

- `which freeradius` / `which radtest` confirmed that
  FreeRADIUS 3.2.7 and radtest are installed at
  `/usr/sbin/freeradius` and `/usr/bin/radtest` on
  duck-dev-01.
- A QA test (`TestFreeRADIUSRealRuntimeAuth`) creates a
  netns `qa-radius` with a veth pair to the host
  (10.254.0.0/30), builds a complete FreeRADIUS config
  tree under `t.TempDir()`, starts the daemon in
  foreground mode inside the netns, and exercises
  radtest against the configured test user with four
  subtests (correct password, wrong password, unknown
  user, wrong shared secret).
- The config tree currently includes `clients.conf`,
  `users`, `mods-available/{pap,files,...}`,
  `mods-enabled/...`, `sites-available/default`,
  `radiusd.conf` with `modules {}` block, and the
  `prefix / logdir / run_dir / db_dir / libdir /
  pidfile` directives.

## 2. What was learned (the hard way)

1. FreeRADIUS 3.2.7 refuses `user = root` in
   radiusd.conf ("deprecated"). The lab runs radiusd
   as root, so we just drop the directive.
2. The `modules {}` block is required in radiusd.conf
   for FreeRADIUS 3.2.x to consider the server
   "configured".
3. The `pap { auto_header = yes }` sub-block inside
   `modules {}` is rejected with "Parse error after
   "pap": unexpected token"}" in the FR 3.2.7 parser.
   The correct shape appears to be `modules { pap }` (no
   sub-block), or to keep the pap config under
   `mods-available/pap` and `ln -s` it from
   `mods-enabled/`.
4. The path `/usr/share/freeradius` is searched by
   default; `-D /usr/share/freeradius` is the safer
   override.
5. `user` and `group` directives in radiusd.conf are
   deprecated and must be set via env (`USER`, `GROUP`)
   or omitted (radiusd runs as the calling uid).

## 3. M-5 finding: render path is too small to boot a
real daemon

`internal/radiusd/radiusd.go::Render()` only emits the
`listen` and `clients` blocks. A real `freeradius -X`
daemon needs at minimum:
- `radiusd.conf` with `prefix / logdir / run_dir / db_dir
  / libdir / pidfile` and a `modules {}` block
- `mods-available/{pap,files,detail,eap,expr,logintime,
  always,preprocess,chap,mschap,suffix,attr_filter}`
- `mods-enabled/...` symlinks
- `clients.conf` (Render covers this)
- `users` (file backend)
- `sites-available/default` and `sites-enabled/default`

The QA test would benefit from the package owning the
whole tree. The fix proposal:
- split `Render` into `RenderServer` (current) and a
  new `RenderSite` (full default site) and
  `RenderModules` (the modules block)
- have a single `RenderAll(dir string, s Server)`
  helper that writes the full tree

## 4. Outstanding sub-areas (deferred)

- **Final pass the FreeRADIUS config parser** — the
  test should boot radiusd with `pap { auto_header =
  yes }` working, then run all four radtest subtests.
- **CoA / Disconnect** — needs a `listen { type = coa
  }` block and a CoA client; out of scope for the
  basic auth-reject success criterion.
- **TLS / radsec** — FreeRADIUS supports EAP-TLS and
  RADIUS/TLS; not exercised in this commit.

## 5. Artifacts

- `internal/qa/radiusd_linux_test.go` — the test
  + config builder
- `docs/evidence/qa/05-finding-M5-freeradius.md` —
  this document
