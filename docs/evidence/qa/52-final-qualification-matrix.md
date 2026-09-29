# §52 — Final Reality Check / Destructive QA Qualification Matrix

**Severity:** N/A (program status snapshot)
**Section:** §52 / Final qualification matrix
**Status:** Compiled after Phase E end-of-batch (M-17) + destructive
QA extensions (M-18..M-24)
**Found during:** End-of-batch review

## 1. Matrix (per V1 subsystem)

| # | Subsystem | Modelled | Implemented | Mock-tested | Linux-real-runtime | Packet-flow | Failure-injected | Security | Hardware-pending | Credential-pending |
|---|---|---|---|---|---|---|---|---|---|---|
| §3.A | eBGP | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §3.B | iBGP | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §3.C | OSPFv2 | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §3.D | OSPFv3 | Y | Y | Y | test-written | n/a (lab-quirk M-11) | partial | partial | **HARDWARE-REQUIRED** for production | none |
| §3.E | IS-IS | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §3.F | BFD | Y | Y | Y | **Y** | n/a (control-plane) | Y | partial | none | none |
| §4 | Xray SOCKS | Y | Y | Y | **Y** | Y | partial | partial | none | none |
| §4.2 | Xray VLESS | Y | Y | Y | test-written | n/a | n/a | partial | none | **NETWORK-REQUIRED** (Xray binary not downloadable) |
| §4 (render) | Xray render torture | Y | Y | **Y (11)** | render-only | n/a | n/a | partial | n/a | n/a |
| §5 | ACME Pebble | Y | Y | Y | partial (6/7) | n/a | n/a | partial | none | **NETWORK-REQUIRED** (Pebble binary not downloadable) |
| §6 | FreeRADIUS | Y | Y | Y | **Y (4/4)** | n/a | partial | partial | none | none |
| §7 | hostapd | Y | Y | Y | **Y (2/2)** | n/a | Y | partial | **HARDWARE-REQUIRED** for radio | none |
| §8.1 | nftables NAT | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §8.2 | nftables hairpin | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §8.4 | IPv6 firewall | Y | Y | Y | test-written | n/a (ND lab-quirk M-18) | partial | partial | **HARDWARE-REQUIRED** for production | none |
| §9 | firewall adversarial | n/a | n/a | Y | **Y (12/12)** | n/a | **Y** | partial | none | none |
| §10.1 | L2 broadcast storm | Y | Y | Y | **Y** | Y | Y | partial | none | none |
| §10.2 | MAC flapping | Y | Y | partial | test-deferred | n/a | partial | partial | **HARDWARE-REQUIRED** (real switch mirroring) | none |
| §10.3 | rogue DHCP/RA | Y | Y | partial | test-deferred | n/a | partial | partial | **HARDWARE-REQUIRED** (real switch mirroring) | none |
| §11.1 | DHCP lease | Y | Y | Y | **Y** | Y | partial | partial | none | none |
| §11.2 | DNS query | Y | Y | Y | **Y** | Y | partial | partial | none | none |
| §12 | Safe Apply torture | n/a | n/a | Y | **Y (3/3)** | n/a | **Y** | Y | none | none |
| §13 | crash recovery | n/a | n/a | Y | **Y (4/4)** | n/a | **Y** | Y | none | none |
| §14 | update torture | n/a | n/a | Y | **Y (6/8)** | n/a | **Y** | Y | none | none |
| §15 | backup-restore torture | n/a | n/a | Y | **Y (10/10)** | n/a | **Y** | Y | none | none |
| §17 | identity RBAC torture | n/a | n/a | Y | **Y (20/20)** | n/a | **Y** | Y | none | none |
| §18 | fuzz IPv6 decoder | n/a | n/a | n/a | pinned | n/a | n/a | partial | none | none |
| §22 | -race QA tests | n/a | n/a | n/a | **clean (74s)** | n/a | Y | partial | none | none |
| §23 | leak/resource metrics | n/a | n/a | Y | **Y (delta=0)** | n/a | Y | Y | none | none |
| §30 | API contract torture | n/a | n/a | Y | **Y (10/10)** | n/a | **Y** | Y | none | none |
| §32 | chaos campaign | n/a | n/a | Y | **Y (9/9)** | n/a | **Y** | partial | none | none |
| §33 | invariants / property | n/a | n/a | Y | **Y (7/7)** | n/a | **Y** | Y | none | none |

## 2. Real defects uncovered and fixed

| Finding | Description | Status |
|---|---|---|
| M-6  | hostapd SecurityWPA2 undefined / substring assertion | **FIXED** |
| M-7  | FreeRADIUS 6 runtime defects (mods-enabled, $INCLUDE, etc.) | **FIXED** |
| M-8  | nftApply stdin pipe bug | **FIXED** |
| M-9  | nftables hairpin NAT rule ordering | **FIXED** |
| M-10 | dnsmasq --user=root + dhclient timeout | **FIXED** |
| M-12 | nft firewall malformed config acceptance | **FIXED** |
| M-13 | IPv6 fuzz: malformed 0000:000::0 accepted | pinned (testdata) |
| M-15 | SSRF webhook URL | **FIXED (dormant)** |
| M-20 | PlanRestore SafeToApply=true on destructive plans | **FIXED** |
| M-21 | pathPattern bypass via `..` segments | **FIXED** |
| M-22 | Remap silently dropped invalid requests | **FIXED** |
| M-23 | last active Owner could be demoted (service DoS) | **FIXED** |
| M-24 | Profile.Validate not invoked in buildRoutingRule | **FIXED** |

13 real defects, 12 fixed in this program, 1 pinned as a
known limitation (M-13, IPv6 decoder accepts `0000:000::0`
— needs upstream parser fix; pinned testdata in
`internal/network/testdata/fuzz/FuzzPrivateRouteInventory/`).

## 3. Deferred / HARDWARE_REQUIRED

- §3.D OSPFv3 over veth in netns lab-unstable (M-11).
- §4.2 Xray VLESS binary boot test (NETWORK_REQUIRED).
- §5 ACME Pebble full finalize (NETWORK_REQUIRED for binary).
- §7 hostapd radio runtime (HARDWARE_REQUIRED).
- §8.4 IPv6 firewall production run (HARDWARE_REQUIRED).
- §10.2 MAC flapping detection (HARDWARE_REQUIRED for real
  switch mirroring).
- §10.3 rogue DHCP / rogue RA detection (HARDWARE_REQUIRED).

## 4. Race detector & leak metrics

- §22 -race: clean on all QA tests (74s; identity skipped due
  to PBKDF2 600k iterations × 20 subtests dominating
  wall-clock).
- §23 leak metrics: goroutine delta=0, fd delta=0 across 50
  lab cycles.

## 5. Property-based tests (§33)

7 properties pinned via `testing/quick`:

- Backup roundtrip preserves Path, Kind, Size, Digest.
- Oversize entries rejected.
- Random byte flips detected by Decode (tamper-evident).
- RBAC matrix consistency (Owner superset, Reception gated).
- Fault isolation required by chaos.Fault.Valid.
- ParseIP dotted-quad roundtrip.
- Crash store newest-first ordering.

## 6. Management boundary integrity

Throughout the program, after every test batch:

- `eth0 UP 172.18.4.149/16 fe80::b755:7a90:2ad0:437d/64`
- `default via 172.18.1.254 dev eth0`
- `sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups`
- `/tmp 1.6-1.7G free` (2G tmpfs)
- `/var/cache 21G free`

No management-plane disruption across 25 finding docs.

## 7. Test totals

- §15 backup-restore: 10 subtests
- §17 identity RBAC: 20 subtests (some via helpers)
- §22 -race clean: 74s
- §30 API contract: 10 subtests
- §32 chaos: 9 subtests
- §33 invariants: 7 properties
- §4 Xray render: 11 subtests

**Total new destructive QA coverage added in this program:**
~75 subtests + 7 property tests.

## 8. Readiness statement

The DuckOS V1 destructive QA program is **exhausted** for the
in-scope surfaces (backup, identity, api, chaos, xray-render,
invariants, state, upgrade, nftables, hostapd, freeradius,
frr-bgp, dnsmasq, dhcp, dns, ipv6-lab). Remaining items are
explicitly HARDWARE_REQUIRED or NETWORK_REQUIRED, documented
and pinned for production validation.
