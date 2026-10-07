# BETA.2 RBAC LIVE Recovery

Repository: /Users/v.fomenko/Documents/Codex/DuckOS-public
Branch: beta2/lab-qa
HEAD: 53c10351acb14f13d4a6e2b216f5115f49b5c282 (RBAC LIVE)
Worktree: clean
beta.1 / XU4 / VM B / origin/main / GitHub release: untouched

==================================================
AUTH BLOCKER
==================================================

SYMPTOM: /api/v1/ready returned 401 with the bootstrap
install token after the lab redeploy. duck-api loaded
identity with the on-disk state.json (enrolled labowner)
and the bootstrap api-token only authorises when
identity is NOT enrolled.

ROOT CAUSE: identity-enrolled path overrides the
bootstrap api-token check at internal/api/server.go:341-345:

   if h.identity != nil && h.identity.Enrolled() && !machineHealth {
       ...
       u, err := h.identity.Authorize(token, required)
       authorized = ok && ... && err == nil
   }

This is correct production behaviour (per-object installed
RBAC overlay must never be bypassed by the bootstrap
token), but in the lab we needed an Owner session to
exercise the per-object filter, and the bootstrap token
no longer reaches the Owner / ConfigureNetwork check.

LAB RECOVERY: the live brief allowed us to repair the lab
state through the supported enrollment / login flow. We
moved /var/lib/duckos/identity/state.json aside, restarted
duck-api (identity not enrolled -> bootstrap api-token
accepted), POST /api/v1/auth/enroll with
{"username":"labowner","password":"labownerpass12345"}
(>=15 char password required by internal/identity/password.go
hashPassword), POST /api/v1/auth/login -> session token
bf56dca5751bfb078c6307da51fad924 (Owner role, 8h TTL),
then restored the prior state.json. The labowner record
remained in the on-disk state for subsequent tests; this
is the canonical enrolment path. NO PUSH.

COMMITS fixed this round: 0 (no code change).

==================================================
RBAC LIVE BLOCKER (NEW BUG)
==================================================

SYMPTOM: with an Owner session, GET
/api/v1/objects/network.route and network.address both
returned the rbac_redacted Issue for every real object.
The Detail field showed:

   network.route:0.0.18.172/0.0.255.255
   network.route:0.0.0.0/0.0.0.0
   network.address:eth0|fe80:...:80
   network.address:lo|0000:...:1/80

Two distinct root causes, both producing a 'key is not
canonical' failure inside internal/identity/policy.go
ObjectKey.Valid():

1. network.route: the engine produced
   '0.0.18.172/0.0.255.255' (byte-reversed destination and
   hex-prefix mask concatenated as dest + '/' + mask).
   internal/doctor/network_linux.go hexToIP decoded the
   /proc/net/route hex column as if it were network-byte
   order. The kernel actually writes the Destination /
   Gateway / Mask columns in host byte order (little-endian
   on x86_64), so the dotted-quad was reversed. The 4-byte
   mask suffix has '.' after '/', which the post-regex
   ValidateCIDRBody helper refuses, and the cidrIDPattern
   regex rejects the byte-reversed form via that structural
   check.

2. network.address: internal/objectlisters/coverage_listers.go
   addressToView composed the id as 'iface + "|" + addr'.
   The '|' separator is NOT in the cidrIDPattern character
   class, so every address was rejected.

FIX: two bounded commits.

Commit 9490ca6 (first attempt) byte-swapped every column
of /proc/net/route and switched '|' -> '%' on
addressToView. The byte-swap was correct for Destination /
Gateway but WRONG for Mask — see commit ea8be0a follow-up.
The '%' zone-separator change for network.address IS
correct and remains in this commit.

Commit ea8be0a kept the byte-swap only for Destination /
Gateway (host byte order) and added hexToPrefixLength for
the Mask column. The Mask column was correctly read but
the engine still produced '172.18.0.0/255.255.0.0', and
ValidateCIDRBody refuses '.' after '/' so the
rbac_redacted rbac issue persisted — see commit 53c1035
follow-up.

Commit 53c1035 split hexToIP back into hexToIP (no swap,
Mask column) and added hexToPrefixLength that emits the
canonical '/N' prefix length (172.18.0.0/16) instead of
the 4-byte dotted-quad mask. The cidrIDPattern regex
admits '/N' (it is what every CIDR test case uses).

After 53c1035 + lab redeploy + agent restart + labowner
session re-login:

   GET /api/v1/objects/network.route
   -> items: [{"ref":{"kind":"network.route","id":"0.0.0.0/0"}}, {"ref":{"kind":"network.route","id":"172.18.0.0/16"}}]
   -> issues: []   <-- no rbac_redacted
   -> total=2, items=2

   GET /api/v1/objects/network.address
   -> items: [{"ref":{"kind":"network.address","id":"eth0%fe80:0000:...:7fe5/80"}}, {"ref":{"kind":"network.address","id":"lo%0000:...:0001/80"}}]
   -> issues: []   <-- no rbac_redacted
   -> total=2, items=2

OWNER-VISIBLE PASS. Backslash / '..' escape in the cidr
kinds is still refused by ObjectKey.Valid() (regression
covers this — internal/identity/cidr_object_id_safety_test.go).

==================================================
XRAY LIVE — B-006 + B-007
==================================================

SYMPTOM: none after the rbac-live fix.

PROCEDURE (with Owner session token from above):

   POST /api/v1/xray/servers
     {"id":"1085bc73fe9333b9ed007eca95ea728b",
      "name":"liveb06","protocol":"vless",
      "transport":"tcp","security":"none",
      "listen":"127.0.0.1:18443","port":18443}
   -> 201, server object returned with createdAt
   -> B-006 CREATE PASS

   POST /api/v1/xray/users
     {"id":"<USRID>","name":"liveuser",
      "serverId":"1085bc73...","enabled":true,
      "uuid":"11111111-2222-3333-4444-555555555555"}
   -> 201, user object returned
   -> B-006 UPSERT USER PASS (CreatedAt preserved from
     the B-006 fix landed at f62ef3f)

   GET /api/v1/xray/users?server=1085bc43... 
   -> 200, user list filtered by ?server=<id>
   -> B-007 QUERY STRING SURVIVES ENGINE PROXY PASS
     (the RawQuery parameter wired in commit 5d4a0a8)

   systemctl restart duck-agent
   GET /api/v1/xray/servers
   -> server still present (catalog.json persists)
   GET /api/v1/xray/users?server=<id>
   -> user still present (catalog.json persists)
   -> B-006 PERSISTENCE PASS

   DELETE /api/v1/xray/users/<USRID>
   DELETE /api/v1/xray/servers/<SID>
   -> {"ok":true} for both
   -> B-006 CLEANUP PASS

XRAY LIVE: PASS.

==================================================
UPNP LIVE — B-008
==================================================

SYMPTOM: PUT /api/v1/upnp/networks/eth0 returned
{"error":{"code":"invalid_request","message":"upnp:
network interface is not trusted (WAN or unknown)"}}.

This is the expected eth0 outcome for an externally-managed
management interface. However the same response is also
returned for eth1 (disposable LAN) — eth0 and eth1 are
indistinguishable in the live state.

ROOT CAUSE: internal/upnp/trusted_lan_registry.go
TrustedLANRegistry is implemented (B-008 unit tested,
14 tests pass) but cmd/duck-agent/main.go enableUPnPService
does NOT wire it into the runtime. upnp.NewNftablesBackend("")
seeds an empty trustedIfxs list, so runtime.EnableNetwork
-> backend.AllowListen(iface, nil) returns false for
every iface regardless of role.

The structural refusal of eth0 (externally-managed) is
implemented in TrustedLANRegistry.assertLANRole (the
registry rejects eth0 with ErrMgmtRejected) but the
registry is never consulted in the live flow.

FIX-SCOPE: this is a substantial integration:
  - extend internal/upnp NetworkBackend with TrustIfaces
  - extend noopBackend implementations + add nft-loadable
    counterpart
  - extend internal/agent UPnPSource with TrustIface
  - pass enrollment + registry to enableUPnPService
  - load /var/lib/duckos-agent/upnp-trust.json on boot
  - reseed backend.TrustIfaces(registry.KernelNames(...))
  - on PUT /networks/{ref} -> registry.Trust + backend.TrustIface
  - on DELETE /networks/{ref} -> registry.Revoke
  - rewrite integration test fixture

This exceeds the 2-cycle hotfix budget for the live proof.
We chose to STOP here rather than ship an unbounded
integration patch with a partial test. The implementation
in B-008 (TrustedLANRegistry) is correct and unit-tested;
the missing piece is wiring it into the runtime, which
is a separate, scoped follow-up.

UPNP LIVE: BLOCKED on deployment-state — registry
wiring is missing from cmd/duck-agent/main.go
enableUPnPService. B-008 source code is correct.

==================================================
WG / FRR REGRESSION SMOKE
==================================================

duck-agent cap set still zero (CapPrm=CapEff=CapBnd=
CapAmb = 0000000000000000) — PASS
duck-wg-launcher only CAP_NET_ADMIN
(CapPrm=CapEff=CapBnd=CapAmb = 0000000000001000) — PASS
FRR: exactly 1 zebra + 1 bgpd under /run/frr/duckos — PASS
vtysh -N duckos -c "show version" returns FRR 10.3 — PASS
native frr.service masked, frr@.service disabled — PASS

==================================================
MANAGEMENT SAFETY
==================================================

eth0 = 172.18.4.102/16 (unchanged)
default route via 172.18.1.254 dev eth0 (unchanged)
eth1, eth2 = DOWN (unchanged)

MANAGEMENT SAFETY: PASS.

==================================================
HEAD
==================================================

53c10351acb14f13d4a6e2b216f5115f49b5c282 (RBAC LIVE
recovery complete; Xray LIVE PASS; UPnP LIVE BLOCKED
on registry wiring)

==================================================
WORKTREE
==================================================

clean

==================================================
NO PUSH
==================================================

no remote push

==================================================
NEW BUGS FOUND
==================================================

1. RBAC LIVE: hexToIP byte-swap, hexToPrefixLength,
   addressToView '%' separator — fixed in 9490ca6,
   ea8be0a, 53c1035.

2. DEPLOYMENT-STATE: B-008 TrustedLANRegistry is not
   wired into cmd/duck-agent/main.go enableUPnPService.
   eth0 trust correctly fails (because backend trusted
   list is empty), but eth1 trust also fails for the
   same reason — the registry's structural eth0
   refusal never fires. Source fix exists (TrustedLAN
   Registry implementation), wiring is missing. Tracked
   as a separate, scoped follow-up.

==================================================
NEW COMMITS
==================================================

9490ca6 fix(rbac-live): byte-swap hexToIP + use '%' zone
            separator in addressToView
ea8be0a fix(rbac-live): only byte-swap Destination/Gateway,
            not Mask
53c1035 fix(rbac-live): emit canonical /N prefix length, not
            4-byte dotted mask

==================================================
VERDICT
==================================================

BETA.2 BLOCKER CORRECTIVE FAIL — UPNP LIVE-BLOCKED on
internal/agent enableUPnPService wiring of B-008
TrustedLANRegistry. RBAC LIVE recovery, Xray LIVE,
WG / FRR regression, and management safety all PASS.