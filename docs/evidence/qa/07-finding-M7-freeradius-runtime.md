# M-7 — Real FreeRADIUS 3.2.7 runtime qualification: 6 separate config defects found and fixed

**Severity:** HIGH (config-render / startup blockers; RADIUS auth broken without fix)
**Section:** §6 / FreeRADIUS real-binary qualification
**Status:** FIXED, regression test PASSING on duck-dev-01
**Found during:** Phase E §6 hostile qualification against real FreeRADIUS 3.2.7

## 1. Affected boundary

Every config that `internal/radiusd.Render` (or hand-rolled config) emits
must:

1. Parse cleanly with FreeRADIUS 3.2.7's strict unlang parser.
2. Reference modules that are actually built into the distro package.
3. Use only configuration items accepted by the running daemon version.
4. Wire `$INCLUDE` correctly (trailing slash for directories).
5. Register modules in a `modules { ... }` block, not at top level.
6. Make modules-enabled symlinks, not empty placeholders.

## 2. Impact (before fix)

`TestFreeRADIUSRealRuntimeAuth` could not start a real FreeRADIUS 3.2.7
daemon against the test config. The daemon either crashed on bind
("Address already in use" from a leftover process), failed config
parse, or registered no modules so site references like `pap` failed
with "Failed to find pap as a module or policy". RADIUS authentication
was untestable end-to-end on real FR.

## 3. Six separate root causes, six separate fixes

### 3.1 `mods-enabled/<name>` must be a symlink, not an empty file

FreeRADIUS's `$INCLUDE mods-enabled/` reads each file. If the file
contains only a comment, the include contributes nothing — the module
never registers. Upstream layout: `mods-enabled/<name>` is a relative
symlink to `../mods-available/<name>`. Fixed by `os.Symlink`.

### 3.2 `$INCLUDE` for directories uses a trailing slash

`$INCLUDE mods-enabled/*` is a glob-style path FR does NOT expand; it
treats `mods-enabled/*` as a literal filename. The correct form is
`$INCLUDE mods-enabled/` (with trailing slash) which is FR's "include
all files in directory" directive.

### 3.3 `$INCLUDE mods-enabled/` must live INSIDE `modules { ... }`

Without a top-level `modules { ... }` wrapper, FR logs "Cannot find a
modules section" and silently registers zero modules. Site references
to module names then fail with "Failed to find pap as a module or
policy". The upstream `/etc/freeradius/3.0/radiusd.conf` uses exactly
this wrapping.

### 3.4 Not every rlm_*.so ships in every distro build

Debian 13's `freeradius 3.2.7+dfsg-1+deb13u2` does NOT include:

- `rlm_suffix.so` — `Failed to link to module 'rlm_suffix'`
- `rlm_policy.so` — `Failed to link to module 'rlm_policy'`
- `rlm_eap.so` (with default config) — `Failed to find 'Auth-Type
  EAP' section. Cannot authenticate users.`

Removed from enabled modules; the auth-only test path doesn't need
them. Documented so the production `internal/radiusd.Render` knows
which modules are portable.

### 3.5 Deprecated config items are rejected

FreeRADIUS 3.2.7's parser actively rejects deprecated keys:

- `attr_filter { attrsfile = "..." }` → "Configuration item 'attrsfile'
  is deprecated. Replace 'attrsfile' with 'filename'."
- `detail { detailperm = 0600 }` → "Configuration item 'detailperm' is
  deprecated. Replace 'detailperm' with 'permissions'."

Both fixed by switching to the new names.

### 3.6 `filter_username` is a built-in unlang filter, not a module

`authorize { filter_username preprocess chap mschap ... files pap }`
fails: "Failed to find 'filter_username' as a module or policy."
`filter_username` is a built-in unlang keyword and must not appear in
the module list. Removed from the authorize{} block.

### 3.7 radtest CLI shape: `nas-port-number` is positional

`radtest user pass radius-server[:port] secret` is **wrong**. The
correct shape is:

```
radtest user pass radius-server[:port] nas-port-number secret
```

With `nas-port-number` missing or `-t 3` mis-used as timeout, radtest
prints Usage and exits with no UDP traffic. Note: `-t` is auth method
(pap/chap/mschap/eap-md5), NOT timeout. Removed `-t` to use PAP
default and added the `0` NAS port.

## 4. Linux qualification evidence

duck-dev-01, FreeRADIUS 3.2.7, test config in isolated netns
(`qa-radius`), 10.254.0.2:1812, real `radtest` from the host:

```
=== RUN   TestFreeRADIUSRealRuntimeAuth
=== RUN   TestFreeRADIUSRealRuntimeAuth/correct_password
    Received Access-Accept Id 166 from 10.254.0.2:1812
--- PASS: TestFreeRADIUSRealRuntimeAuth/correct_password (0.02s)
=== RUN   TestFreeRADIUSRealRuntimeAuth/wrong_password
    Received Access-Reject Id 8 from 10.254.0.2:1812
--- PASS: TestFreeRADIUSRealRuntimeAuth/wrong_password (0.02s)
=== RUN   TestFreeRADIUSRealRuntimeAuth/unknown_user
    Received Access-Reject Id 73 from 10.254.0.2:1812
--- PASS: TestFreeRADIUSRealRuntimeAuth/unknown_user (0.02s)
=== RUN   TestFreeRADIUSRealRuntimeAuth/wrong_shared_secret
    No reply from server for ID 121 socket 3
--- PASS: TestFreeRADIUSRealRuntimeAuth/wrong_shared_secret (15.03s)
--- PASS: TestFreeRADIUSRealRuntimeAuth (15.99s)
PASS
ok  github.com/CrazyDuckLab/DuckOS/internal/qa  15.990s
```

Management boundary preserved: `eth0 UP 172.18.4.149/16`,
`default via 172.18.1.254 dev eth0`, `sshd pid 758 alive`.

## 5. Reproducible harness

The verification was performed with a one-shot harness
`scripts/qa-frtest.sh` that built the proven config in `/tmp/qa-frtest`
and exited 0 on `freeradius -X -C`. The harness has since been removed
from the tracked tree as a pre-public-cleanup step. The reproducible
verification procedure is documented in the test artifacts above.

## 6. Production impact

`internal/radiusd.Render` currently emits only a partial config
(listen + clients). The hand-rolled test config documented here is the
de-facto reference for what `internal/radiusd.Render` must expand to
cover the same scenarios. Recommended follow-up (tracked in M-5):
split `Render` into `RenderServer` / `RenderSite` / `RenderModules`
and apply the rules above.

## 7. Commits

- `fix(qa): §6 FreeRADIUS mods-enabled as symlinks + no modules{} block`
- `fix(qa): §6 FreeRADIUS $INCLUDE uses trailing slash for directory`
- `fix(qa): §6 FreeRADIUS modules{} wrapper + symlinks + working radtest shape`
- `fix(qa): §6 radtest '-t 3' is auth-type not timeout; explicit veth cleanup`
