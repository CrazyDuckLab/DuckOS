# DuckOS BETA.2 — Live Qualification Script (P065 + P090)

Historical laboratory procedure, not public installation instructions. Beta.2
ships the Port Mapping backend without UPnP IGD, SSDP, NAT-PMP or PCP frontends.

This is the bounded script to run on the two Debian 13 lab VMs
once they become reachable from the operator workstation.

**Do NOT execute against inaccessible VMs.** Run only when both
nodes answer SSH and the host's `nft` binary is present.

## Environment

- two VMs reachable as `node-a` ([redacted management address]) and `node-b` ([redacted management address])
- each VM runs the beta.2 image with `nft` (nftables) available
- each VM has a trusted LAN interface (eth1) and a WAN interface (eth0)
- management interface eth0 stays reachable throughout the run
- the host can SSH into each VM with an operator-controlled SSH identity

## P065 — Port Mapping backend and Wake qualification

```bash
set -euo pipefail
NODES=("node-a" "node-b")
VM_LAN=eth1
VM_WAN=eth0
```

### 1. Default OFF, no listeners, no rules

```bash
for n in "${NODES[@]}"; do
  ssh "$n" -- 'sudo systemctl status duckos-agent'
  # Default state must be OFF.
  curl -fsS -H "Authorization: Bearer $TOKEN" \
    http://"$n":8080/api/v1/upnp/state \
    | jq -e '.stats.networks == 0 and (.mappings | length == 0) and .backend.kind == "nftables"'
  # No nftables rules under ducknet duckos_upnp yet.
  ssh "$n" -- 'sudo nft list table inet duckos_upnp' && { echo FAIL; exit 1; } || true
done
```

### 2. Enable UPnP IGD on the trusted LAN

```bash
curl -fsS -X PUT \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"network":{"networkRef":"lan.home","protocols":["upnp"],"ttlSec":600},"iface":"eth1"}' \
  http://node-a:8080/api/v1/upnp/networks/lan.home
# /api/v1/upnp/state now lists lan.home with upnp enabled.
```

### 3. WAN enable is structurally refused

```bash
set +e
curl -sS -X PUT \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"network":{"networkRef":"wan","protocols":["upnp"],"ttlSec":600},"iface":"eth0"}' \
  http://node-a:8080/api/v1/upnp/networks/wan \
  -w '\n%{http_code}\n'
#  Expected: 400 (backend refuses eth0)
```

### 4. Create one mapping per protocol

```bash
for proto in upnp nat-pmp pcp; do
  curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"id\":\"qual-$proto\",\"networkRef\":\"lan.home\",\"protocol\":\"$proto\",\"clientIp\":\"10.0.0.10\",\"externalPort\":40000,\"internalPort\":4080,\"internalHost\":\"10.0.0.10\"}" \
    http://node-a:8080/api/v1/upnp/mappings
done
# Three rules must now live under inet duckos_upnp on node-a.
ssh node-a -- 'sudo nft list table inet duckos_upnp' | tee /tmp/rules.before
```

### 5. Real TCP traffic flows through the mapping

```bash
# Local listener on node-a
ssh node-a -- 'nc -lk 4080 < /dev/zero &'
# External client on node-b connects to node-a:40000 through the DNAT
ssh node-b -- "timeout 5 curl -fsS http://node-a:40000/ | wc -c"
# Expected: bytes received.
```

### 6. TTL expiry removes the rule

```bash
# Mutate TTL to 5 s for the upnp mapping, wait 7 s, then check.
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/upnp/mappings/qual-upnp
# Mapping removed; rule gone.
ssh node-a -- 'sudo nft list table inet duckos_upnp' | grep qual-upnp && { echo FAIL; exit 1; }
```

### 7. Revoke removes the rule

```bash
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/upnp/mappings/qual-nat-pmp
ssh node-a -- 'sudo nft list table inet duckos_upnp' | grep qual-nat-pmp && { echo FAIL; exit 1; }
```

### 8. Disable removes ALL owned rules

```bash
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/upnp/networks/lan.home
ssh node-a -- 'sudo nft list table inet duckos_upnp' && { echo FAIL; exit 1; } || true
```

### 9. Unrelated nftables tables unaffected

```bash
# snapshot BEFORE
ssh node-a -- 'sudo nft list ruleset' | sha256sum > /tmp/ruleset.before
# create + delete many mappings
for i in $(seq 1 50); do
  curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
    -d "{\"id\":\"qual-burst-$i\",\"networkRef\":\"lan.home\",\"protocol\":\"upnp\",\"clientIp\":\"10.0.0.$i\",\"externalPort\":$((40000+i)),\"internalPort\":4080,\"internalHost\":\"10.0.0.10\"}" \
    http://node-a:8080/api/v1/upnp/mappings
done
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/upnp/networks/lan.home
ssh node-a -- 'sudo nft list ruleset' | sha256sum > /tmp/ruleset.after
diff /tmp/ruleset.before /tmp/ruleset.after
# Expected: identical apart from the duckos_upnp table (which is gone).
```

### 10. duck-agent restart recovers state

```bash
ssh node-a -- 'sudo systemctl restart duckos-agent'
sleep 5
curl -fsS -H "Authorization: Bearer $TOKEN" http://node-a:8080/api/v1/upnp/state | jq .
# Expected: stats.networks == 0, backend.kind == "nftables".
```

### 11. Host reboot — no orphan rules

```bash
ssh node-a -- 'sudo reboot'
sleep 60
ssh node-a -- 'sudo nft list table inet duckos_upnp' && { echo FAIL; exit 1; } || true
# No orphan DuckOS-owned rules after a clean reboot.
```

### 12. Wake packet reaches a known host

```bash
# Send WoL to node-b's MAC from node-a's eth1 broadcast.
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"target\":\"$(ssh node-b -- cat /sys/class/net/eth0/address)\",\"port\":9,\"packets\":3}" \
  http://node-a:8080/api/v1/wake/send
# Expected: 202 with status=sent, observation=PACKET_SENT.
# UI must NOT claim the device woke.
```

### 13. Invalid MAC / WAN listener / invalid networkRef all refused

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" -d '{"target":"not-a-mac"}' \
  http://node-a:8080/api/v1/wake/send -w '\n%{http_code}\n'
# 400

curl -sS -X PUT -H "Authorization: Bearer $TOKEN" \
  -d '{"network":{"networkRef":"!!","protocols":["upnp"],"ttlSec":600}}' \
  http://node-a:8080/api/v1/upnp/networks/!! -w '\n%{http_code}\n'
# 400
```

## P090 — Duck Speed Lab qualification

```bash
ALLOW="lab.duckos.example,internal-cache.duckos.example"
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes
# Manager exists; no probes.
```

### 1. Latency probe

```bash
ID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"qual-lat","kind":"latency","target":{"host":"lab.duckos.example","port":443},"duration":3000,"maxSamples":8}' \
  http://node-a:8080/api/v1/speedlab/probes | jq -r .id)
sleep 4
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/"$ID" | jq .
# Expected: state=done, summary.kind=latency, summary.latency.count > 0.
```

### 2. HTTP probe

```bash
ID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"qual-http","kind":"http","target":{"url":"http://lab.duckos.example/"},"duration":3000,"maxSamples":8}' \
  http://node-a:8080/api/v1/speedlab/probes | jq -r .id)
sleep 4
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/"$ID" | jq .
# Expected: summary.latency.count > 0.
```

### 3. Throughput probe

```bash
ID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"qual-tp","kind":"throughput","target":{"url":"http://internal-cache.duckos.example/1GiB.bin"},"duration":10000}' \
  http://node-a:8080/api/v1/speedlab/probes | jq -r .id)
sleep 11
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/"$ID" | jq .
# Expected: summary.throughput.bytes > 0, Mbps > 0.
# UI text must say "throughput to configured target", not "Internet bandwidth".
```

### 4. Cancel

```bash
ID=$(curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"qual-cancel","kind":"latency","target":{"host":"lab.duckos.example","port":443},"duration":30000,"maxSamples":1024}' \
  http://node-a:8080/api/v1/speedlab/probes | jq -r .id)
sleep 1
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/"$ID"
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/"$ID" | jq .
# Expected: state=cancelled or done within seconds.
```

### 5. Failure (unreachable host)

```bash
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"qual-fail","kind":"latency","target":{"host":"blackhole-127.invalid","port":9},"duration":3000,"maxSamples":4}' \
  http://node-a:8080/api/v1/speedlab/probes
sleep 4
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/speedlab/probes/qual-fail | jq .
# Expected: state=failed or done with summary.error.
```

### 6. Concurrent probes — concurrency cap is respected

```bash
# Issue 6 probes in parallel.
for i in $(seq 1 6); do
  curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"id\":\"qual-par-$i\",\"kind\":\"latency\",\"target\":{\"host\":\"lab.duckos.example\",\"port\":443},\"duration\":2000,\"maxSamples\":2}" \
    http://node-a:8080/api/v1/speedlab/probes &
done
wait
# At least one should be refused with speedlab_busy.
```

### 7. UI state tracking

```bash
# Open the browser at https://node-a and walk through Speed Lab page
# manually. The pending probe shows "Running"; completed probe shows
# "Completed" with summary table. Failed probe shows "Failed" with the
# error text. No raw English leaks in RU/DE/ES.
```

## Regression smokes (run on every beta image)

```bash
# 1. WireGuard availability via vault
curl -fsS -H "Authorization: Bearer $TOKEN" \
  http://node-a:8080/api/v1/wireguard/status | jq '.status'

# 2. Xray backend/version
/usr/local/bin/xray version | head -1
# Must match the pinned XRAY_CORE_VERSION in scripts/install.sh.

# 3. Management eth0 preservation
ssh node-a -- 'ip -4 addr show dev eth0'
# Must remain reachable on the management subnet throughout the run.
```

## Pass / fail criteria

- All P065 checks pass against real `nft` on a real Linux host.
- All P090 probes return typed summaries with no raw samples exposed.
- UI shows "backend unavailable" when the fixture is wired (do NOT
  trigger this in production; the production wiring now uses
  UnavailableBackend, not the fixture).
- No orphan `inet duckos_upnp` rules after disable / revoke / reboot.
- No new regressions in WireGuard / Xray / management eth0.

## Notes

- This script is read-only on the source tree.
- It is NOT executed against inaccessible VMs in this round.
- If a step fails, capture the failure mode and the nft / curl / ssh
  output verbatim into the live-qualification report.