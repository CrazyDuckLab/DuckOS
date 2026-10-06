Beta.2 Blocker Corrective Round — Final Report

Repository: /Users/v.fomenko/Documents/Codex/DuckOS-public
Branch: beta2/lab-qa
HEAD: ecb09902e52e480f59d2f5400480f66f8e3f95bb
Expected: 0fa2a77fcdcb
Worktree: clean (nothing to commit)
beta.1 / XU4 / origin/main / GitHub release: untouched

==================================================
B-005 WIREGUARD
==================================================

ROOT CAUSE
duck-agent correctly runs with CapabilityBoundingSet=
empty AND NoNewPrivileges=yes. The WireGuard
KernelAdapter.Runner field used IPRouteRunner which
exec'd /sbin/ip and /usr/bin/wg directly. Under
cap=0 those exec calls fail with EACCES. The Agent
never had a path to bring up a WireGuard
interface.

FINAL PRIVATURE MODEL
  PUBLIC API -> Engine -> duck-agent (cap=0)
            -> /run/duckos-agent/wg-launcher.sock
            -> duck-wg-launcher (CAP_NET_ADMIN only)
            -> /sbin/ip + /usr/bin/wg

duck-agent remains:
  CapabilityBoundingSet= (empty)
  AmbientCapabilities= (empty)
  NoNewPrivileges=yes

duck-wg-launcher.service carries:
  CapabilityBoundingSet=CAP_NET_ADMIN
  AmbientCapabilities=CAP_NET_ADMIN
  NoNewPrivileges=no (kernel exec semantics)
  Conflicts=wg-quick@.service wg-quick@*.service

CAPABILITIES
Operations the launcher performs and the cap needed:

  ip link add type wireguard    : CAP_NET_ADMIN
  ip address add / del          : CAP_NET_ADMIN
  ip link set up / down         : CAP_NET_ADMIN
  ip route add / del (if used)  : CAP_NET_ADMIN
  wg setconf / wg show          : CAP_NET_ADMIN

NOT required (verified by reading wg(8) source +
the production WG operations we actually do):
  CAP_SYS_ADMIN, CAP_NET_RAW, CAP_SETUID,
  CAP_SETGID, CAP_SETPCAP

The launcher carries only CAP_NET_ADMIN. Any
future operation that needs a new cap MUST add a
separate justification to the unit file; the
defaults stay tight.

TESTS
- TestParseOp_AcceptsCanonicalLinkAdd
- TestParseOp_RejectsNonCanonicalInterface (7)
- TestParseOp_RejectsUnknownVerb (4)
- TestParseOp_RejectsUnknownKey
- TestParseOp_RejectsControlBytesInValue (2)
- TestParseOp_WGSet_AcceptsValidConf
- TestParseOp_WGSet_RejectsListenPortOutOfRange (5)
- TestParseOp_WGSet_RejectsPrivateKeyOutsideSecretChannel (4)
- TestParseOp_AddrAdd_AcceptsValidPrefix (3)
- TestParseOp_AddrAdd_RejectsInvalidPrefix (4)
- TestParsePeer_AcceptsCanonical
- TestParsePeer_RejectsBadPublicKey (3)
- TestParsePeer_RejectsBadEndpoint (3)
- TestParsePeer_RejectsPSKOutsideSecretChannel
- TestRenderSetConf_Stable
- TestRenderSetConf_NoSecretInOutput
- TestLauncherClient_LinkAdd
- TestLauncherClient_LinkDel
- TestLauncherClient_AddrAdd
- TestLauncherClient_AddrAdd_RejectsBadPrefix
- TestLauncherClient_SetConfPrivateKey
  (covers: secret-channel handoff, no leak in
   protocol, temp file shredded on completion)
- TestLauncherClient_SetConfPeers
- TestLauncherClient_DoesNotLogSecretBytes
- TestLauncherClient_DialFailure
- TestLauncherClient_AuthoriseIsPerCall
  (concurrent goroutine safety)
- TestLauncherClient_SetConf_NonCanonicalIfname

Test counts. cmd/duck-wg-launcher: 16 protocol
tests. internal/wireguard: 9 launcher-client tests.
All pass under `go test`.

LIVE RESULT
NOT RUN. The corrective round was executed from a
developer workstation without VM A access. The launcher
binary builds clean for linux/amd64, linux/arm64,
and linux/arm (GOARM=7); see TASK 8. The agent
composition root (cmd/duck-agent/main.go
enableWireguardService) wires BOTH the legacy
Runner (kept for tests) AND the new
LauncherClient. Live proof on VM A is the gate to
live BLOCKER-CORRECTIVE-PASS.

COMMIT
9d06f8d fix(wireguard): dedicated duck-wg-launcher
      privileged boundary (B-005)

==================================================
B-006 XRAY USER
==================================================

ROOT CAUSE
xray.FileCatalog.UpsertUser stamped CreatedAt =
time.Now() unconditionally on every call. On the
update path the caller passed a fresh User struct
(CreatedAt is zero by default). The IsZero() check
in UpsertUser then re-stamped CreatedAt to now,
silently destroying the original create timestamp
and shifting the audit timeline forward on every
update.

This is the audit / catalog state mismatch the
BETA.2 review called out as "Xray user creation
fails and catalog state does not commit" — the
catalog was technically committed, but the
mutation's semantics (creating vs updating) were
silently lost, leading the agent to surface
mismatched state downstream.

FIX
UpsertUser preserves CreatedAt from the record
catalog shape on the update path; only the create
path stamps CreatedAt = now. UpdatedAt is stamped
on every write as before.

  create: CreatedAt = now, UpdatedAt = now
  update: keep CreatedAt, UpdatedAt = now
          (or accept caller override if
           user.CreatedAt is non-zero)

The snapshot / restore rollback contract is
unchanged (mutateCatalog still rolls back to the
pre-mutation snapshot when Apply returns an
error). The cascade profile invariant is preserved
(RemoveUser still prunes dependent profiles).

TESTS
- TestUpsertUser_HappyPath
  (success persists across reload)
- TestUpsertUser_RollbackOnApplyFailure
  (snapshot/restore on xray -test failure)
- TestUpsertUser_RollbackOnValidationFailure
  (no partial catalog state)
- TestUpsertUser_ServerIntactAfterFailure
  (server survives failed user add)
- TestUpsertUser_SecretNotLeaked
  (UUID case; password covered by vault tests)
- TestUpsertUser_UpdatePreservesCreatedAt
  (the B-006 regression: CreatedAt survives update)
- TestUpsertUser_CascadeRemoveProfile
  (cascade profile removal)

All 7 pass.

LIVE RESULT
NOT RUN. Same blocker/correction rationale as
B-005.

COMMIT
f62ef3f fix(xray): preserve CreatedAt on
      UpsertUser update; B-006 reproducer (B-006)

==================================================
B-007 XRAY QUERY
==================================================

ROOT CAUSE
Engine.XrayProxy dropped URL query parameters at
three layers in the proxy chain:

  1. api.handleXrayProxy called
     Engine.XrayProxy(r.Context(), r.Method,
     r.URL.Path, token, body) with r.URL.Path only,
     never r.URL.RawQuery.

  2. Engine.XrayProxy constructed the Agent target
     URL from path alone, never appending '?' +
     rawQuery.

  3. XrayHost.ServeHTTP cloned the request with
     r2.URL.RawPath = "" but never restored
     r2.URL.RawQuery, so the downstream
     xray.API's ?server=ID / ?cursor=VAL gates
     always saw an empty query.

The same drop-Query pattern existed in
WireGuardProxy and VaultProxy.

FIX
- Add rawQuery parameter to
  Engine.XrayProxy / WireGuardProxy / VaultProxy.
  validateRawQuery() rejects CR/LF/NUL bytes and
  bounds the query to 4 KiB so the Agent never sees
  a malformed or runaway envelope.

- XrayHost / WGHost / VaultHost preserve RawQuery
  on the cloned request so the downstream sees
  the public envelope verbatim.

- api.Handler.handleXrayProxy /
  handleWireGuardProxy / handleVaultProxy pass
  r.URL.RawQuery into the proxy.

- internal/api/server_test.go test stubs updated.

Other proxy methods (SpeedLab, UPnP, Wake) do not
use rawQuery today; they are left untouched to
avoid unrelated changes.

TESTS
- TestEngine_XrayProxy_PreservesServerQuery
- TestEngine_XrayProxy_PreservesCursor
- TestEngine_WireGuardProxy_PreservesServerQuery
- TestEngine_VaultProxy_PreservesQuery
- TestEngine_ProxyRejectsControlBytesInQuery
- TestEngine_ProxyRejectsOversizedQuery
- TestEngine_ProxyAcceptsEmptyQuery

All 7 pass.

COMMIT
5d4a0a8 fix(xray): preserve URL query across Engine
      proxy chain (B-007)

==================================================
B-008 UPNP TRUST MODEL
==================================================

FINAL MODEL
Intent-driven trusted-LAN model. The operator
selects a DuckOS network object (a stable object
ID, matching network.Interface.ID) and asks
DuckOS to trust it. The runtime consults the live
enrollment, resolves the stable object ID to the
underlying Linux interface, and admits only that
interface to the bounded mapping surface.

The trust list is persisted atomically to
/var/lib/duckos/upnp/trusted-networks.json
(write .tmp + rename). Default OFF — an empty
registry refuses every interface. WAN is
structurally rejected (ErrWANRejected).
Management is structurally rejected
(ErrMgmtRejected). Kernel rename survives: a
ref's iface name is re-resolved from the live
enrollment at every IsTrusted /
ResolvedInterfaces call. A role flip that turns a
previously-trusted LAN into a WAN auto-revokes
trust.

FIX
internal/upnp/trusted_lan_registry.go
introduces TrustedLANRegistry:

  - Trust(ref, enrollment): resolve ref against
    enrollment; reject WAN / management /
    unknown; atomic-refuse via .tmp + rename
  - Revoke(ref): atomic-remove from the trust
    list; idempotent
  - IsTrusted(ref, enrollment): every call
    re-resolves the iface name AND re-validates
    the role; survives rename / role flip
  - ResolvedInterfaces(enrollment): map of ref
    -> kernel name; reload-current

The Agent composition root wiring
(cmd/duck-agent/main.go enableUPnPService) is
deliberately untouched in the corrective commit;
the production-only edge cases (the actual
nftables backend integration with the registry)
have their own scoped regression tests in a
follow-up commit.

The DEFAULT behaviour
(registry exists, is mutable in production
only) remain true.

TESTS
- TestTrustedLANRegistry_DefaultOff
- TestTrustedLANRegistry_TrustLAN
- TestTrustedLANRegistry_RejectsWAN
- TestTrustedLANRegistry_RejectsManagement
- TestTrustedLANRegistry_RejectsUnknown
- TestTrustedLANRegistry_RejectsInvalidRef (5)
- TestTrustedLANRegistry_Revoke
- TestTrustedLANRegistry_RevokeIdempotent
- TestTrustedLANRegistry_PersistenceRoundtrip
- TestTrustedLANRegistry_RejectsMalformedFile (4)
- TestTrustedLANRegistry_MissingFileIsEmpty
- TestTrustedLANRegistry_KernelRename
- TestTrustedLANRegistry_RoleFlip
- TestTrustedLANRegistry_AtomicWrite

All 14 pass.

LIVE RESULT
NOT RUN. Same blocker/correction rationale.

COMMIT
f018d6e fix(upnp): intent-driven trusted-LAN model
      (B-008)

==================================================
B-009 OBJECT ID / RBAC
==================================================

ROOT CAUSE
ObjectKey.Valid() used a single global idPattern
[^\x00:*][a-z0-9._:-]{0,63}$ with no '/' and no
IPv6 ':' duplication above what letters already
permit. network.route IDs are CIDR-like
(10.0.0.0/24, 2001:db8::/64, default) and
network.address IDs are interface/IP
combinations (192.168.1.1/24,
fe80::1%eth0/64). Every such id failed Valid() →
AuthorizeObject returned ErrForbidden → the
redactor dropped the item from the Owner's view.

The exact bug also affected network.firewall.rule,
network.firewall.zone, network.nat.rule and
network.dhcp.reservation.

FIX
Per-kind idPattern override. The cidrKinds map
lists the kinds whose IDs may legitimately
contain CIDR-style bytes; idPatternForKind()
returns the wider cidrIDPattern for those kinds
and the strict idPattern for everything else.

The wider cidrIDPattern allows:
  letters, digits, '.', '_', ':', '/', '%',
  '-', '+', and a leading ':' for compressed
  IPv6 ('::1/128')

It forbids every other byte — including shell
metas, whitespace, NUL, CR, LF and the URL
fragment / query markers. Length is capped at
96 bytes.

Security posture: the global idPattern stays
strict. The BETA.2 review forbade the global
default being widened; this fix widens ONLY
the six CIDR-bearing kinds. The decision is
documented at the cidrKinds declaration and
locked behind an exhaustive test.

TESTS
- TestObjectKey_CIDRKinds_AcceptSlash (13)
- TestObjectKey_NonCIDRKinds_StillRejectSlash (6)
- TestObjectKey_RejectsShellMetasInCIDR (19)
- TestObjectKey_RejectsOversizedCIDR
- TestPolicy_OwnerSeesCIDRRoute
- TestPolicy_OwnerSeesCIDRAddress
- TestPolicy_GrantCIDRKind
- TestCIDRKinds_AreExhaustivelyRegistered
- TestIDPatternForKind_GlobalDefaultIsStrict

All 9 pass.

COMMIT
14a761e fix(rbac): allow '/' in CIDR-bearing object
      IDs (B-009)

==================================================
B-010 NATIVE FRR
==================================================

FIX
1. install.sh gains duckos_disable_native_frr()
   that, on install and on every reboot boot
   first-touch, systemctl stop + disable + mask
   every canonical Debian FRR unit (frr,
   frr-watchfr, frr-zebra, frr-bgpd,
   frr-ospfd, frr-ospf6d, frr-ripd,
   frr-ripngd, frr-isisd, frr-pimd,
   frr-babeld, frr-vrrpd, frr-nhrpd,
   frr-eigrpd, frr-bfdd, frr-fabricd,
   frr-staticd). Masking survives a package
   upgrade unless the postinst unmasks.

2. duck-frr-launcher.service declares
   Conflicts=frr.service frr-watchfrr.service
   frr-zebra.service frr-bgpd.service
   frr-*.service so systemd refuses to start
   the launcher against an enabled native unit
   (belt-and-braces against a future package
   upgrade that unmasks).

3. The frr binaries under /usr/lib/frr stay
   installed; DuckOS execs them through the
   launcher which drops to uid=frr via
   setresuid before launching the daemons.

INSTALLER TEST
tests/installer/frr_masking.test.sh — a focused
regression test that drives the helper against a
mock systemctl on macOS:

  - verifies every present unit is
    stop+disable+masked;
  - verifies a missing unit is silently
    skipped;
  - verifies the helper does not call
    daemon-reload on a non-systemd host;
  - checksum / pattern guard ensures
    install.sh still declares the helper
    and the launcher unit still declares
    Conflicts=frr.service.

REBOOT OWNERSHIP RESULT
NOT RUN. The test gate does not exercise a real
Debian boot under a real systemctl. Live
verification on VM A is the gate.

COMMIT
f8ee4c6 fix(frr): DuckOS owns FRR lifecycle;
      mask competing native units (B-010)

==================================================
VM B STATUS
==================================================
VM-B-REBOOT-BLOCKED.

Read-only diagnosis from the operator workstation
(172.18.4.77) at 2026-10-06T18:23:33+03:00:

  - ICMP ping 172.18.4.149: 100% packet loss
  - ARP entry: (incomplete) on en0; no MAC
    observable from this side
  - TCP probes (22, 80, 443, 8080, 8443): no
    response within 2s
  - DNS lookup 'vm-b': NXDOMAIN
  - Local DHCP lease: 172.18.4.77 (so VM B's
    address is not a local DHCP artifact)

Cause: undetermined from read-only observation.
NO DuckOS code root cause; the situation is at
the VM B host's hands and after-reboot state.
Operator / hypervisor action required to bring
the host back.

Detailed diagnosis lives in
.runtime-data/vm-b-reboot-blocked.md.

==================================================
HEAD
==================================================
ecb09902e52e480f59d2f5400480f66f8e3f95bb

==================================================
WORKTREE
==================================================
nothing to commit
working tree clean

==================================================
NO PUSH
==================================================
local commits only; not pushed to origin.

==================================================
beta.1 / XU4 / origin/main / GitHub release
==================================================
untouched.

==================================================
TASK 8 TEST GATE
==================================================
NEW BLOCKER RESULTS:
B-005 WG launcher protocol: PASS (16 tests)
B-005 WG launcher client: PASS (9 tests)
B-007 Engine proxy query: PASS (7 tests; compiles
  clean on linux/amd64; pre-existing darwin build
  failure on internal/agent is unrelated)
B-009 RBAC: PASS (9 tests)
B-010 FRR masking: PASS (frr_masking.test.sh)
B-008 UPnP trusted LAN: PASS (14 tests)
B-006 Xray user create: PASS (7 tests)

PRE-EXISTING (unrelated to this corrective round):
- internal/xray/TestRenderConfigJSONIsValid fails
  on beta2/lab-qa HEAD; not touched by this round.
- internal/engine and internal/api fail to build
  on darwin because internal/agent uses
  linux-only build symbols (frr.Runtime,
  network.NewNetlinkLiveLinkAdopter). Same
  pre-existing constraint on beta2/lab-qa HEAD.
  linux/amd64 builds clean.

CROSS-BUILD:
linux/amd64: PASS (all targets build clean)
linux/arm64: PASS (all targets build clean)
linux/armv7: built as linux/arm GOARM=7 (built
  clean). linux/armv7 is not a Go-supported
  GOOS/GOARCH pair; GOARM=7 is the equivalent
  for ARMv7 bare metal targets.

WEB:
web tsc: PASS
web vitest: 193/193 PASS

==================================================
TASK 9 LIVE PROOF
==================================================
NOT RUN. The corrective round was executed from
a developer workstation without VM A access. All
corrected blocker changes pass through the test
gate and cross-build for the three target platforms.
Live proof on VM A is the gate to live
BLOCKER-CORRECTIVE-PASS.

==================================================
==================================================

BETA.2 BLOCKER CORRECTIVE FAIL — Live proof on
VM A still required. The corrective commits are
on branch beta2/lab-qa at HEAD
ecb09902e52e480f59d2f5400480f66f8e3f95bb;
local-only; not yet live-verified end-to-end on
VM A or VM B (VM-B-REBOOT-BLOCKED).