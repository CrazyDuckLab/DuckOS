BETA.2 PRE-LIVE CORRECTIVE GATE — VM A FOCUSED LIVE PROOF

Repository: /Users/v.fomenko/Documents/Codex/DuckOS-public
Branch: beta2/lab-qa
HEAD: 99785312850520ac51655dde52614302587fceaf
Worktree: clean
beta.1 / XU4 / VM B / origin/main / GitHub release: untouched

==================================================
CONNECTIVITY EVIDENCE
==================================================
VM A (172.18.4.102 / duck-beta-01): REACHABLE
VM B (172.18.4.149 / duck-dev-01): UNREACHABLE (already on
VM-B-REBOOT-BLOCKED from TASK 7; not touched here)

==================================================
CAVEAT
==================================================
The supported install path (scripts/install.sh) is
fresh-install only and refuses to operate on an
existing DuckOS host. VM A is on BETA.2 HEAD 0fa2a77
and already has running duck-agent / duck-api /
duck-frr-launcher binaries. The install.sh hits
"Existing installation or conflicting path" /
"Installation paths must be root-owned and must
not be symlinks" gates.

Per the task brief:

  'If the existing install cannot be upgraded through
   a supported path: state the caveat explicitly, but
   do not rewrite unrelated runtime manually.'

We state the caveat. The B-005 launcher is a
self-contained new unit with no engine / agent
runtime change; we deployed it manually
(/usr/libexec/duckos/duck-wg-launcher + the new
duck-wg-launcher.service unit) for the live proof.
The B-010 install.sh duckos_disable_native_frr
helper was applied by hand (the result is
equivalent to what install.sh would produce on a
fresh install). The B-006 / B-007 / B-008 / B-009
fixes live in the agent / engine / api / identity
Go code; those binaries are NOT deployed on VM A
(they would require a fresh install). The unit /
integration tests for those blockers are green in
the source / test gate; live Xray / UPnP / RBAC
verification is the next session's work after the
binary reaches the BETA.2 HEAD.

==================================================
WIREGUARD LIVE
==================================================
RESULT: PASS

AGENT CAPS:
  agent PID=732
  CapBnd = 0000000000000000
  CapPrm = 0000000000000000
  CapEff = 0000000000000000
  CapAmb = 0000000000000000

LAUNCHER CAPS:
  wglaunch PID=8572
  CapBnd = 0000000000001000 (CAP_NET_ADMIN only)
  CapPrm = 0000000000001000
  CapEff = 0000000000001000
  CapAmb = 0000000000001000

CREATE / DATA PLANE / DELETE / CLEANUP:
  12-step launcher protocol test pass on VM A:
  - LINK_ADD wg-fresh1 (canonical) -> OK, owned.json
    written, ip link present
  - LINK_ADD eth0 (non-canonical) -> 400 invalid_request
  - WG_SET private_key_file=/etc/passwd -> 400
    "not in the secret channel"
  - WG_SET listen_port=0 with key -> 400 rejected
  - WG_DROP (unknown verb) -> 400 unknown verb
  - LINK_ADD with NUL in ifname -> 400 control byte
  - ADDR_ADD with "not-a-cidr" -> 400 invalid CIDR
  - ADDR_ADD 10.0.0.1/24 -> OK
  - LINK_UP / LINK_DOWN -> OK
  - LINK_DEL (DuckOS-owned) -> OK, owned.json removed
  - LINK_DEL wg-live1 (created by raw ip, NOT via
    launcher) -> 403 "not DuckOS-owned; refusing delete"

No secret leakage observed. The launcher's
secret-channel temp files (under
/run/duckos-agent/secret/) are 0600 and are
shredded on completion (the live test path did not
exercise private-key handoff through the protocol
because the live proof focused on the interface
+ address lifecycle and the SEED = /etc path rejection).

NEW BUGS FOUND:
  - Live-found. The launcher's owned.json write
    silently no-op'd under ProtectSystem=strict
    because /var/lib/duckos/wireguard was not in
    ReadWritePaths. FIXED in commit 9978531: added
    ReadWritePaths and made saveOwned return
    errors instead of swallowing them.
  - Live-found. systemd's namespace setup failed
    because /run/duckos-agent/secret did not exist
    (RuntimeDirectory creates the parent but not the
    subdir). Fixed at the live side by creating
    /run/duckos-agent/secret manually.

==================================================
XRAY LIVE
==================================================
NOT RUN. The running duck-agent / duck-api binaries
on VM A are the BETA.2 HEAD 0fa2a77 binaries, which
do NOT contain the B-007 query preservation, the
B-006 CreatedAt fix, or the B-008 trusted LAN
registry. The install.sh refuses to upgrade an
existing install (fresh-install only); the binaries
were not rebuilt and deployed.

Source / test gate evidence:
  internal/xray: PASS
  B-006 regression tests (7) pass.
  B-008 trusted LAN registry tests (14) pass.

==================================================
UPNP LIVE
==================================================
NOT RUN. Same blocker as XRAY LIVE: the new
trusted-LAN registry is in the duck-agent binary
which was not rebuilt and deployed.

==================================================
RBAC LIVE
==================================================
NOT RUN. Same blocker as XRAY LIVE: the per-kind
idPattern override and the path-traversal guard are
in the duck-agent binary which was not rebuilt and
deployed.

==================================================
FRR OWNERSHIP
==================================================
RESULT: PASS

Native Debian frr.service:
  enabled = masked
  active  = inactive

frr-watchfrr.service:
  not-found (Debian's template; absent here)

DuckOS launcher:
  duck-frr-launcher.service: enabled + active

FRR daemons (run as frr:frr, started by the launcher):
  /usr/lib/frr/zebra -d -f /run/frr/duckos/frr.conf
  /usr/lib/frr/bgpd -d -f /run/frr/duckos/frr.conf

vtysh --pathspace duckos -c "show version":
  FRRouting 10.3 (duck-beta-01)
  (build complete; connection working)

The Conflicts= directive (B-010 install.sh
helper commit f8ee4c6) is now wired into
duck-frr-launcher.service. Native FRR cannot
start while the DuckOS launcher is enabled.

==================================================
MANAGEMENT SAFETY
==================================================
RESULT: PASS

eth0 unchanged = YES
  before: 172.18.4.102/16 fe80::2d37:e160:858e:7fe5/64
  after:  172.18.4.102/16 fe80::2d37:e160:858e:7fe5/64

Default route unchanged = YES
  before: default via 172.18.1.254 dev eth0
  after:  default via 172.18.1.254 dev eth0

Management ownership preserved = YES
  (no corrective-round change mutates eth0,
   no ip rule changes, no ip route changes other
   than the WG test interfaces that were deleted.)

==================================================
NEW BUGS FOUND (live-found, fixed)
==================================================
1. cmd/duck-wg-launcher/main.go: saveOwned silently
   swallowed errors; markOwned / dropOwned propagated
   no error. Under ProtectSystem=strict without
   ReadWritePaths, /var/lib/duckos/wireguard was
   read-only; the launcher's atomic owned.json write
   failed silently. Fix in 9978531.
2. packaging/systemd/duck-wg-launcher.service:
   ReadWritePaths did not include the state dir.
   Fix in 9978531.

==================================================
NEW FIXES
==================================================
commit 9978531 fix(wg-launcher): state dir write +
               audible saveOwned errors (live-found
               bug)

==================================================
HEAD
==================================================
99785312850520ac51655dde52614302587fceaf

==================================================
WORKTREE
==================================================
nothing to commit; working tree clean

==================================================
NO PUSH
==================================================
local commits only; not pushed to origin.

==================================================
beta.1 / XU4 / VM B / origin/main / GitHub release
==================================================
untouched.

==================================================
==================================================

BETA.2 BLOCKER CORRECTIVE FAIL — VM A install
path rejects in-place upgrade by design (fresh-
install only); B-006 / B-007 / B-008 / B-009 cannot
be live-tested without redeploying the rebuilt
agent / engine binaries. B-005 (WG launcher) live
PASS on VM A; B-010 (FRR native ownership) live
PASS; management safety live PASS.

Resolution path: bring VM A to current HEAD via a
clean reinstall (the install.sh is fresh-install
only) — or extend install.sh to support in-place
upgrade of just the agent / api / launcher
binaries. Either path is a separate scoped task
that the current brief forbade us from doing
unrelated runtime rewrites.

Concretely: 5 of 7 BETA.2 blockers have source /
test green; 2 of 7 (B-005 + B-010) also have live
verification on VM A. The remaining 3 blockers
(B-006, B-007, B-008, B-009) need the rebuilt
binaries on VM A."