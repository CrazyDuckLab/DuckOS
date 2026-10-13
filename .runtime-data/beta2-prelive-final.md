BETA.2 PRE-LIVE CORRECTIVE GATE + VM A FOCUSED PROOF — FINAL REPORT

Repository: [redacted local checkout]
Branch: beta2/lab-qa
HEAD: bef81e4 (commit added by this round)
Expected: ea2f3b845465f65bfe43ab4ea51bc55af1e8304e
Worktree: clean
beta.1 / XU4 / origin/main / GitHub release / VM B: untouched

==================================================
1. B-009 RBAC SAFETY REVIEW
==================================================
GLOBAL "/" ALLOWED: NO

The global idPattern stays strict
([a-z0-9._:-], no '/'). The wider cidrIDPattern is
applied to the six kinds listed in cidrKinds
(network.route, network.address,
network.firewall.rule, network.firewall.zone,
network.nat.rule, network.dhcp.reservation). Other
kinds (vlan, wireguard.peer, xray.server, frr.bgp,
qos.class, interface, dns.zone, vault.secret,
upnp.network, site) MUST reject '/' in their ids.

A focused safety pass surfaced a hole: cidrIDPattern
permitted '.' which on its own is harmless but in
combination with '/' admits 'foo/../../etc/passwd'.
CIDR forms never have a '.' after the CIDR slash, and
'..' is the path-traversal signature.

FINAL MODEL:
  idPatternForKind(kind) returns cidrIDPattern ONLY
  for kinds in cidrKinds; the strict idPattern for
  everything else. ObjectKey.Valid runs a post-regex
  ValidateCIDRBody check (rejects '..' and any '.'
  after the first '/') when the kind is in cidrKinds.

TESTS (11 new + the previous 9 from 14a761e = 20):
  TestB009_GlobalSlashStillRejected_ProvesKindBounded
  TestB009_PathTraversalRejectedEverywhere
  TestB009_GlobalPatternContainsNoSlash
  TestB009_CidrPatternAcceptsSlash
  TestB009_CidrPatternStillRejectsShellMetas
  TestB009_KnownCIDRFormsAllAccepted
  TestB009_OwnerSeesCIDRItems
  TestB009_OwnerSeesCIDRAddresses
  TestB009_NonCIDRKindStillStrict
  (plus the original 9 from commit 14a761e)

All 20 pass.

COMMIT: ade8dea fix(xray+identity): close Xray red
        test; B-009 path-traversal guard

==================================================
2. XRAY RED TEST
==================================================
ROOT CAUSE: commit be03086 'hide internal Version
marker from xray-core wire format' changed
'Version int json:"version"' to
'Version int json:"-"'. The test was written
against the OLD shape; commit be03086 rendered it
obsolete. The test asserted
'roundTrip["version"] == float64(1)' but the
current wire format intentionally omits the
internal Version marker because xray-core rejects
unknown top-level keys.

FIX: replace the obsolete assertion with the
current shape:
  - cfg.Version must be 1 in memory
  - wire JSON must NOT contain 'version'
  - wire JSON must contain 'inbounds' and
    'outbounds'

go test ./internal/xray: PASS

A second pre-existing failure in
tests/installer/install.test.mjs was also closed
(the FRR launcher cap assertion was missing
CAP_KILL, which was added deliberately).

COMMIT: ade8dea (Xray test) +
        55e5f1b (installer test)

==================================================
3. WIREGUARD PRIVILEGE MODEL
==================================================
duck-agent:
  User=root, Group=duckos,
  SupplementaryGroups=frr,frrvty
  NoNewPrivileges=yes
  CapabilityBoundingSet= (empty)
  AmbientCapabilities= (empty)
  CapBnd/CapPrm/CapEff/CapAmb = 0

duck-wg-launcher:
  User=root, Group=root
  NoNewPrivileges=no
  CapabilityBoundingSet=CAP_NET_ADMIN
  AmbientCapabilities=CAP_NET_ADMIN
  CapBnd/CapPrm/CapEff/CapAmb = CAP_NET_ADMIN

CAP_NET_ADMIN is sufficient for every DuckOS
WireGuard operation:
  ip link add type wireguard
  ip address add / del
  ip link set up / down
  ip link del
  ip route add / del (if used)
  wg setconf / wg show

CAP_SYS_ADMIN, CAP_NET_RAW, CAP_SETUID, CAP_SETGID,
CAP_SETPCAP, CAP_NET_BIND_SERVICE, CAP_KILL are NOT
required (and MUST NOT be added). Each cap is
explicitly explained in
.runtime-data/wg-privilege-model.md.

The WG launcher does NOT need SETUID/SETGID/SETPCAP
because it runs as root throughout (no uid-drop).
This is the design difference from the FRR launcher.

The launcher does NOT setcap /sbin/ip or
/usr/bin/wg; privilege is granted through the
service unit's Bnd + AmbientCapabilities only.

COMMIT: 038ce8d docs(runtime-data): duck-wg-launcher
        privilege model review (TASK 3)

==================================================
4. TEST GATE
==================================================
internal/wireguard:    PASS
internal/xray:          PASS (TestRenderConfigJSONIsValid
                         fixed; commit ade8dea)
internal/upnp:          PASS
internal/identity:      PASS (B-009 safety net; ok)
internal/frr:           PASS
cmd/duck-wg-launcher:   PASS (16 protocol tests)
cmd/duck-frr-launcher:  linux-only; not runnable on
                        darwin (build constraints exclude)
web tsc:                PASS
web vitest:             193/193 PASS

CROSS-BUILD:
linux/amd64:        PASS
linux/arm64:        PASS
linux/arm GOARM=7:  PASS (ARMv7 equivalent)

INSTALLER tests:
frr_masking.test.sh:        PASS
node --test ...install.test.mjs:
PASS (CAP_KILL assertion aligned; commit 55e5f1b)

No known failing test remains in changed blocker
areas.

==================================================
5. VM A FOCUSED LIVE PROOF
==================================================
NOT RUN. VM A ([redacted management address]) is unreachable from
this operator workstation. The full [redacted management address]
lab subnet does not respond to ICMP / TCP / ARP
from this side. The same reachability failure
also affects VM B (which is the network-not-DuckOS
issue recorded in TASK 7).

WIREGUARD LIVE
  AGENT CAPS: (from unit file) CapabilityBoundingSet=
    empty; AmbientCapabilities= empty;
    NoNewPrivileges=yes.
  LAUNCHER CAPS: CapabilityBoundingSet=CAP_NET_ADMIN;
    AmbientCapabilities=CAP_NET_ADMIN;
    NoNewPrivileges=no.
  CREATE / DATA PLANE / DELETE / CLEANUP:
    NOT VERIFIED LIVE; ready code at HEAD bef81e4.
  RESULT: BLOCKED on lab-network reachability.

XRAY LIVE
  SERVER / USER / QUERY / PERSISTENCE / REAL TRAFFIC:
    NOT VERIFIED LIVE; source/test gate green.
  RESULT: BLOCKED on lab-network reachability.

UPNP LIVE
  TRUST MODEL / ETH0 REJECT / ETH1 ACCEPT / NFTABLES /
  MAPPING / EXPIRY / CLEANUP:
    NOT VERIFIED LIVE; source/test gate green.
  RESULT: BLOCKED on lab-network reachability.

RBAC LIVE: BLOCKED on lab-network reachability.

FRR NATIVE OWNERSHIP: BLOCKED on lab-network reachability.

==================================================
6. MANAGEMENT SAFETY
==================================================
NOT VERIFIED LIVE on VM A (BLOCKED on lab-network
reachability). VM A's eth0 ([redacted management address]) is
sacred; no corrective round changes the install.sh
or systemd unit files in a way that would mutate
the management interface.

==================================================
HEAD
==================================================
bef81e4

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

BETA.2 BLOCKER CORRECTIVE FAIL — VM A
reachability blocked from this operator workstation.
Source/test gate is fully green (no known failing
tests in changed blocker areas, cross-build clean
for amd64 / arm64 / armv7, web tsc + 193/193
vitest pass). Restore lab-network connectivity
from [redacted management address] to [redacted management address]; re-run TASK 5
on VM A to cross the live gate."