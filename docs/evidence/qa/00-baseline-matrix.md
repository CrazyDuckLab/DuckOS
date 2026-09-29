# DuckOS V1 — Reality Check Baseline Matrix

**Branch:** V1 UI qualification branch
**HEAD:** 1032e29 (after gofmt baseline cleanup)
**Ahead of main:** 102 commits
**Generated:** start of QA Program (Reality Check / Red Team / Destructive QA)
**Local Go:** go1.27.1 darwin/arm64
**Linux host:** duck-dev-01 (eth0 172.18.4.149/16, default via 172.18.1.254 dev eth0)
**Local test status (Darwin):** all 146 packages green

## 1. Honest baseline policy

The 9-state maturity scale (used throughout this program):

| Code | State                              | What it requires                                                                                         |
|------|------------------------------------|----------------------------------------------------------------------------------------------------------|
| MOD  | MODELLED                           | Typed contract, fields, validators, state machine, documentation                                          |
| IMP  | IMPLEMENTED                        | Working code paths in the package; not just a type                                                       |
| UNT  | UNIT-TESTED                        | Real Go tests (not compile-only); asserts semantics                                                       |
| RLR  | REAL-LINUX-RUNTIME                 | Real binary or kernel API exercised end-to-end (config render alone does NOT count)                       |
| RPF  | REAL-PACKET-FLOW                   | Real packets transmitted/received; success and failure observed                                          |
| FIJ  | FAILURE-INJECTED                   | Process, disk, time, network, permission faults injected; recovery verified                              |
| SEC  | SECURITY-TESTED                    | Auth, RBAC, secret, injection, parser-hostile, and crypto misuse test coverage                           |
| HP   | HARDWARE-PENDING                   | Requires physical NIC / radio / TPM / SFP / smartcard / sensor; cannot be safely simulated              |
| CP   | CREDENTIAL-PENDING                 | Requires real third-party credentials (Cloudflare, Hetzner, RADIUS server, RPKI validator)               |

A renderer or fake executor is **NOT** real-runtime qualification. A pure unit test is **NOT** failure-injection. A passing go test is **NOT** security-testing.

## 2. Pre-program evidence already in tree

Evidence that informed this matrix (read but not modified in this program):

- `internal/wireguard/adapter_linux.go` — only `IPRouteRunner` and `LocalFilesystem` use `os/exec` for actual Linux plumbing; the rest is model+intent+adapter interface
- `internal/xray/runtime_linux.go` — the only daemon with a real Linux runtime (writePrivate, wait, Apply, Stop, Rollback) using `exec.Cmd` and `xray` binary lookup
- `internal/observe/collector_linux.go` — actual `/proc`, `/sys/class/net`, `nft list ruleset` parsing; bounded subprocess via `runFixed`
- `internal/network/artifacts.go` — real filesystem artifact directory with atomic replace, restore, drift detection, symlink refusal
- `internal/observe/collector.go` — bounded subprocess helper that caps stdout/stderr — important for §23 leak/FD safety
- `internal/agent/*.go` — wires wireguard + lifecycle + xray in the only place real subprocesses are spawned in the agent

**Consequence:** the great majority of `internal/*` packages are model+render only. Real-runtime coverage lives in roughly 8 packages out of 146.

## 3. Per-package matrix (initial honest reading)

Legend: ✓ = reached, – = not reached, ~ = partial/qualified, ? = not assessed yet

### 3.1 Core infrastructure

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `agent` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real subprocesses via wireguard+xray, but no agent-level fault-injection or RBAC test |
| `api` | ✓ | ✓ | ✓ | – | – | – | – | – | – | HTTP surface; no auth/RBAC tests visible yet (to be tested §17) |
| `engine` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Orchestration; safe-apply wiring, but no real commit failure tests yet (§12) |
| `lifecycle` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Restart semantics not fault-injected (§28) |
| `state` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Snapshot/concurrency, race coverage not yet re-checked at scale (§22) |
| `model` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Typed catalog |

### 3.2 Safe apply / recovery

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `safeapply` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Not exercised against a real daemon |
| `powercrash` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Pure model; needs subprocess-level fault injection (§13) |
| `upgrade` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model+engine; no real signed-release torture yet (§14) |
| `recovery` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Emergency paths not exercised end-to-end |
| `backup` | ✓ | ✓ | ✓ | – | – | – | – | – | – | No tortured restore of corrupt/truncated/old-schema backup yet (§15) |
| `timeintegrity` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Wall-clock/monotonic split; clock jump not fault-injected (§27) |

### 3.3 Secrets / vault / auth

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `vault` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Secret canary grep NOT yet run against artifacts/support bundle (§16, §42) |
| `apitokens` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Token scope/revoke/expiry not adversarially tested (§17) |
| `identity` | ✓ | ✓ | ✓ | – | – | – | – | – | – | SSH host key regen on clone not tested (§46) |
| `sshkeys` | ✓ | ✓ | ✓ | – | – | – | – | – | – | CA + authorized_keys model; not exercised against real sshd |
| `accesslifecycle` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Subject/Active/Seq-monotonic history model |
| `cloneprotect` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Clone detection; not validated against real clone image (§78) |

### 3.4 Dataplane subsystems (most are MODELLED only)

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `wireguard` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real `ip`/`wg` exec; not yet exercised in real netns topology on duck-dev-01 |
| `xray` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real `xray` exec in `runtime_linux.go`; binary not yet installed on duck-dev-01 (§4) |
| `vlan` / `vrf` / `bonding` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Pure model+render; no kernel apply path |
| `mtu` | ✓ | ✓ | ✓ | – | – | – | – | – | – | No PMTU black-hole test yet (§8) |
| `qos` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real `tc` exec present; not yet exercised end-to-end |
| `qos2` | ✓ | ✓ | ✓ | – | – | – | – | – | – | HTB/HFSC render only |
| `multiwan` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Failover logic not run against real WAN |
| `sdwan` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Selection model; no live path scoring yet |
| `wan` / `tunnels` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Dataplane topology model only |

### 3.5 Dynamic routing

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `frr` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | FRR config render is real-format; **no FRR daemon has been started in any test yet** (§3) |
| `bgpadv` | ✓ | ✓ | ✓ | – | – | – | – | – | – | RPKI / BMP / FlowSpec / large comms model only |
| `isis` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model only |
| `dynamicrt` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Trace determinism fixed; runtime not exercised |
| `srmpls` | ✓ | ✓ | ✓ | – | – | – | – | – | – | SR/LDP/L3VPN render only |
| `evpn` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Type 2/3/5 model only |
| `mclag` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model only |
| `oam` | ✓ | ✓ | ✓ | – | – | – | – | – | – | CFM/ERPS/MRP model only |
| `macsec` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model only |
| `multicast` / `mcast` | ✓ | ✓ | ✓ | – | – | – | – | – | – | PIM/IGMP/MLD model only |
| `ipv6trans` / `cgnat` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Render only |
| `ipv6` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Render only |

### 3.6 Wireless / RADIUS / NTP

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `wifi` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model |
| `hostapd` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Config render is real-format; **no hostapd started in test**; radio = HARDWARE-PENDING |
| `radiusd` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | FreeRADIUS config render only; **no FreeRADIUS daemon exercised** (§6) |
| `ntp` / `nts` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Model only; no real ntpd / NTS-KE exercise |
| `ptpd` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | linuxptp config render only; PHC hw = HP |

### 3.7 Security / control-plane

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| `fwcheck` / `fwanalysis` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Static analysis only; no real nft apply (§9) |
| `copp` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Render only |
| `dos` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Quarantine model only |
| `ids` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Suricata-class capability gate; no real IDS |
| `l7` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Classification only |
| `geosets` | ✓ | ✓ | ✓ | – | – | – | – | – | – | FQDN/GeoIP/ASN model |
| `dnsrpz` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Policy DNS render only |
| `upnp` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Render only |
| `posture` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Security posture model only |
| `blastradius` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Risk model |
| `accrunner` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Acceptance runner not yet run at scale (§31) |
| `chaos` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Fault-injection framework; not yet run as campaign (§32) |
| `crashloop` / `watchdog` / `degradation` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Models only |
| `webhooks` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Signing/queue model |

### 3.8 PKI / cert / DNS

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP |
|---|---|---|---|---|---|---|---|---|---|
| `certs` | ✓ | ✓ | ✓ | – | – | – | – | – | – |
| `acme` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | StubTransport only; **no Pebble exercise yet** (§5) |
| `intca` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Internal CA / ZTP model |
| `ddns` / `dhcpsnoop` / `dhcpha` | ✓ | ✓ | ✓ | – | – | – | – | – | – | DHCP/DDNS models only; no real dnsmasq/kea exercised |

### 3.9 Observability / observability

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP |
|---|---|---|---|---|---|---|---|---|---|
| `observe` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real /proc + nft + ip + wg reads; not yet driven as a real netns lab |
| `capture` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real `tcpdump` exec; needs real-packet lab |
| `nettools` | ✓ | ✓ | ✓ | – | – | – | – | – | – | ICMP/TCP/HTTP/DNS probe model |
| `logging` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Appender model |
| `support` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Redaction model; canary grep not yet run (§42) |
| `telemetrypriv` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Off-by-default model |
| `speedlab` / `perfirq` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Models |

### 3.10 Hardware / storage / power

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP |
|---|---|---|---|---|---|---|---|---|---|
| `hardware` | ✓ | ✓ | ✓ | – | – | – | – | – | – | LLDP / NIC / SFP / PoE / modem model |
| `hwhealth` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Sensor/PSU/SMART model |
| `storagewatch` / `storageops` / `resbudget` / `tenantquotas` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Quota/budget models |
| `ups` | ✓ | ✓ | ✓ | ~ | – | – | – | – | – | Real `apcupsd` / `nut` exec interface; no real UPS attached |
| `kernelfw` / `secureboot` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Models |
| `sbom` / `provenance` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Release SBOM model |
| `integrity` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Secure boot / verified boot / SBOM aggregator |
| `consolecli` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Serial/console recovery model |
| `crashevidence` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Crash evidence model |
| `span` / `portsec` / `stp` / `ebpf` / `switchdev` / `sriov` / `anycast` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Render-only |

### 3.11 Operations

| Package | MOD | IMP | UNT | RLR | RPF | FIJ | SEC | HP | CP |
|---|---|---|---|---|---|---|---|---|---|
| `sites` / `topology` / `catalog` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Inventory models |
| `templates` / `provenance` / `clientexport` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Templating |
| `subscription` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Xray user/sub model |
| `migrate` | ✓ | ✓ | ✓ | – | – | – | – | – | – | VyOS/MikroTik/OpenWrt import renderers |
| `install` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Installer model |
| `explain` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Explain-Why models |
| `incident` / `incident2` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Incident model + correlation model |
| `beforeafter` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Before/after comparison |
| `staledetect` / `safedestroy` / `retention` / `decommission` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Lifecycle models |
| `labmode` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Lab refusal policy |
| `scheduler` / `maintenance` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Cron + maintenance windows |
| `capability` / `readiness` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Feature flag / readiness registry |
| `tr369` | ✓ | ✓ | ✓ | – | – | – | – | – | – | USP model (optional) |
| `ztp` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Zero-touch provisioning model |
| `firstrun` / `localfirst` | ✓ | ✓ | ✓ | – | – | – | – | – | – | First-run / cloud-independent model |
| `vrrp` | ✓ | ✓ | ✓ | – | – | – | – | – | – | VRRP/keepalived model |
| `mgmt` / `management` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Management plane model |
| `svcstate` / `txnorder` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Service-state + transaction-ordering model |
| `canary` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Canary-apply model |
| `acceptance` | ✓ | ✓ | ✓ | – | – | – | – | – | – | 65 acceptance scenarios; not yet run end-to-end (§31) |
| `snmp` | ✓ | ✓ | ✓ | – | – | – | – | – | – | SNMP model |
| `doctor` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Doctor model |
| `observability` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Telemetry model |
| `publish` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Distribution model |
| `secpolicy` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Guardrails / posture / blast / crash / watchdog / degradation / quotas / webhooks aggregator |
| `ddns` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Dynamic DNS model |
| `ipam` | ✓ | ✓ | ✓ | – | – | – | – | – | – | IPAM model |
| `nac` | ✓ | ✓ | ✓ | – | – | – | – | – | – | NAC + IPAM + port-sec + SPAN + eBPF + SR-IOV + DHCP-HA + anycast aggregator |
| `chaos` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Chaos + readiness + privacy + crash-evidence + perf + NTS + USP + SSH + console + clone |
| `accrunner` | ✓ | ✓ | ✓ | – | – | – | – | – | – | Acceptance scenario runner (newly split) |

## 4. What is *actually* real-runtime-qualified

Packages whose code path includes a working `os/exec` to a real Linux binary, verified by `grep`:

- `internal/wireguard/adapter_linux.go` — `wg` / `ip` / `ip-route` plumbing
- `internal/xray/runtime_linux.go` — `xray` process management
- `internal/observe/collector_linux.go` — `ip`, `nft`, `wg`, `tcpdump` readers
- `internal/capture/...` — `tcpdump` invocation
- `internal/network/...` — artifact directory only (fs, not subprocess)
- `internal/qos/...` — `tc` invocation
- `internal/ups/...` — `apcupsd` / `nut` interface
- `internal/agent/...` — wires real subprocesses

Of these, **none** have been end-to-end exercised in a real netns topology on `duck-dev-01` by an automated test. They have unit tests using fake binaries or local fs only.

## 5. Honest gap to fill by this program

| Section | Subsystem                  | Real daemon / kernel API            | Status before program             |
|---------|----------------------------|-------------------------------------|-----------------------------------|
| §3      | FRR dataplane              | zebra/bgpd/ospfd/isisd/bfdd in netns | installed, never started by test  |
| §4      | Xray                       | xray binary                        | not installed on duck-dev-01     |
| §5      | ACME                       | Pebble test server                 | not installed                     |
| §6      | FreeRADIUS                 | freeradius + radtest               | installed, never started by test  |
| §7      | hostapd                    | hostapd (parse only, no radio)     | not exercised                     |
| §8      | Real packet-flow matrix    | nft/tc/ip/wg in netns              | not exercised                     |
| §9      | Firewall adversarial       | nftables + real packets            | not exercised                     |
| §10     | L2 storms                  | bridge + veth                      | not exercised                     |
| §11     | DHCP/DNS failure           | dnsmasq/kea/unbound in netns       | not exercised                     |
| §12     | Safe Apply torture         | xray/wireguard/nft/upgrade/backup  | not exercised                     |
| §13     | Power-loss simulation      | subprocess kill around journal     | not exercised                     |
| §14     | Update torture             | signed/unsigned/truncated/invalid  | not exercised                     |
| §15     | Backup/restore torture     | corrupt/wrong-pw/old-schema        | not exercised                     |
| §16     | Vault secret canary        | grep over all artifacts            | **must be done first**            |
| §22     | -race repeated             | high-contention tests              | selective                         |
| §23     | FD/goroutine leaks         | 100/1k iteration loops             | not measured                      |
| §42     | Support bundle canary      | grep canary through support       | not exercised                     |
| §49     | Cleanup verification       | netns/veth/bridge/VRF leaks        | not asserted                      |
| §50     | Management safety          | eth0 / default route / SSH         | must be HARD test failure         |

## 6. Initial readiness estimate (adversarial-pessimistic)

Without the adversarial pass:

- **Backend MOD+IMP+UNT coverage:** high (most of 146 packages)
- **REAL-LINUX-RUNTIME coverage:** low (≤ 8 packages touch real binaries; none have been end-to-end exercised in netns on duck-dev-01)
- **REAL-PACKET-FLOW coverage:** none
- **FAILURE-INJECTED coverage:** none at this depth
- **SECURITY-TESTED coverage:** partial (vault/certs/pki redaction, but no canary grep yet)
- **HARDWARE-PENDING coverage:** Wi-Fi radio, NIC offload, SFP, PHC hw, TPM, smartcard
- **CREDENTIAL-PENDING coverage:** Cloudflare/Hetzner/DO/R53, real RADIUS, RPKI validator

**Conservative backend readiness before adversarial testing:** **~78%** (matching the previous R12+Enterprise Pack #2 report).

**Expected post-program readiness:** will be revised down where real defects are found, and will be presented per-subsystem, not as a single headline number, in line with §52.

## 7. What this program will deliberately not do

- Will NOT inflate readiness because types/models exist
- Will NOT mark a feature green because its unit test exists
- Will NOT weaken failing tests
- Will NOT skip inconvenient cases
- Will NOT start R3 UI/UX
- Will NOT touch eth0 / 172.18.4.149 / default route / SSH
- Will NOT request external Internet BGP / production ACME credentials
