#!/usr/bin/env bash
# tests/integration/fw_live_netns.sh: LIVE packet-level qualification of the
# owned inet duckos_fw firewall inside dedicated network namespaces.
#
# Run as root on a Linux host: bash tests/integration/fw_live_netns.sh
#
# Isolation contract:
#   - All state lives in a dedicated namespace `fwq-qual` (plus two nested
#     endpoints `fwq-cli`/`fwq-srv`); the host ruleset and the management
#     interface (eth0) are never touched.
#   - Only tables this script creates (inet duckos_fw, inet duckos_upnp
#     inside the test namespace) are cleaned up; nothing else.
#   - Fail-closed: pre-existing resources abort the run.
set -uo pipefail

QUAL=fwq-qual
CLI=fwq-cli
SRV=fwq-srv
NFT=nft
fail=0
say() { printf '%s\n' "$*"; }
check() { # check <name> <expected:pass|fail> <cmd...>
	local name=$1 want=$2; shift 2
	if "$@" >/dev/null 2>&1; then got=pass; else got=fail; fi
	if [[ "$got" == "$want" ]]; then say "PASS $name ($got as expected)"; else say "FAIL $name: got $got, want $want"; fail=$((fail+1)); fi
}

[[ $(id -u) -eq 0 ]] || { say 'BLOCKED: root required'; exit 1; }
command -v "$NFT" >/dev/null || { say 'BLOCKED: nft not found'; exit 1; }
command -v ip >/dev/null || { say 'BLOCKED: ip not found'; exit 1; }
for R in "$QUAL" "$CLI" "$SRV"; do
	ip netns list | grep -q "^$R " && { say "BLOCKED: netns $R already exists"; exit 1; }
done
[[ -x "${FWQ_RENDER_HELPER:-}" ]] || FWQ_RENDER_HELPER=""

# render <file> <rule specs...>: one batch per fwrules.Render contract
# (add/delete/add table idiom + accept-policy forward chain).
render() {
	local out=$1; shift
	{
		printf '# duckos.duckos_fw atomic table replacement\n'
		if [[ $# -eq 0 ]]; then
			printf 'add table inet duckos_fw\ndelete table inet duckos_fw\n'
		else
			printf 'add table inet duckos_fw\ndelete table inet duckos_fw\n'
			printf 'add table inet duckos_fw\n'
			printf 'add chain inet duckos_fw forward { type filter hook forward priority filter ; policy accept ; }\n'
			local i=1
			for spec in "$@"; do
				printf 'add rule inet duckos_fw forward %s comment "duckos_fw:rule-%d"\n' "$spec" "$i"
				i=$((i+1))
			done
		fi
	} > "$out"
}

trap 'ip netns del "$QUAL" >/dev/null 2>&1' EXIT
ip netns add "$QUAL" || { say 'BLOCKED: cannot create test namespace'; exit 1; }

q() { ip netns exec "$QUAL" "$@"; }

# --- topology inside the test namespace ---
q ip link add v-cli type veth peer name v-cli-p || { say 'BLOCKED: veth'; exit 1; }
q ip link add v-srv type veth peer name v-srv-p
q ip link set v-cli-p netns "$CLI"
q ip link set v-srv-p netns "$SRV"
q ip addr add 10.250.0.1/30 dev v-cli
q ip addr add 10.250.0.5/30 dev v-srv
q ip link set v-cli up
q ip link set v-srv up
q sysctl -w net.ipv4.ip_forward=1 >/dev/null
ip netns exec "$CLI" ip addr add 10.250.0.2/30 dev v-cli-p
ip netns exec "$CLI" ip link set v-cli-p up
ip netns exec "$CLI" ip route add default via 10.250.0.1
ip netns exec "$SRV" ip addr add 10.250.0.6/30 dev v-srv-p
ip netns exec "$SRV" ip link set v-srv-p up
ip netns exec "$SRV" ip route add default via 10.250.0.5
q ip link set lo up
ip netns exec "$CLI" ip link set lo up
ip netns exec "$SRV" ip link set lo up

BATCH=$(mktemp)
trap 'rm -f "$BATCH"; ip netns del "$QUAL" >/dev/null 2>&1' EXIT

say "== baseline (no owned table, kernel forward enabled) =="
check 'ping cli->srv passes without firewall' pass ip netns exec "$CLI" ping -c2 -W2 10.250.0.6

say "== firewall: accept rule for 10.250.0.0/24 tcp/443 (ICMP unaffected, policy accept) =="
render "$BATCH" 'ip saddr 10.250.0.0/24 tcp dport 443 accept'
q "$NFT" -f "$BATCH" && say 'PASS apply accept-ruleset' || { say 'FAIL apply accept-ruleset'; fail=$((fail+1)); }
check 'ping still passes (policy accept)' pass ip netns exec "$CLI" ping -c2 -W2 10.250.0.6

say "== firewall: explicit drop of the client source =="
render "$BATCH" 'ip saddr 10.250.0.2 drop'
q "$NFT" -f "$BATCH" && say 'PASS apply drop-ruleset' || { say 'FAIL apply drop-ruleset'; fail=$((fail+1)); }
check 'ping blocked by drop rule' fail ip netns exec "$CLI" ping -c2 -W2 10.250.0.6

say "== coexistence: duckos_upnp mapping survives firewall rollback =="
q "$NFT" add table inet duckos_upnp
q "$NFT" add chain inet duckos_upnp prerouting ' { type nat hook prerouting priority dstnat ; }'
q "$NFT" add rule inet duckos_upnp prerouting 'tcp dport 8080 dnat to 10.250.0.6:80'
render "$BATCH" 'ip saddr 10.250.0.2 drop'   # re-apply same firewall state
q "$NFT" -f "$BATCH"
q "$NFT" list table inet duckos_upnp | grep -q 'dnat to 10.250.0.6:80' \
	&& say 'PASS upnp mapping survives fw rollback' || { say 'FAIL upnp mapping lost'; fail=$((fail+1)); }
q "$NFT" list table inet duckos_fw | grep -q 'drop' \
	&& say 'PASS fw rules survive upnp presence' || { say 'FAIL fw rules lost'; fail=$((fail+1)); }

say "== deleting the upnp mapping preserves firewall rules =="
q "$NFT" delete rule inet duckos_upnp prerouting handle \
	"$(q "$NFT" -a list chain inet duckos_upnp prerouting | awk '/dnat/{gsub("handle ","");print $NF}')"
q "$NFT" list chain inet duckos_fw forward | grep -q 'drop' \
	&& say 'PASS fw rules survive upnp mapping deletion' || { say 'FAIL fw rules lost on upnp delete'; fail=$((fail+1)); }

say "== last rule removal / empty ruleset removes the table =="
render "$BATCH"
q "$NFT" -f "$BATCH"
q "$NFT" list table inet duckos_fw 2>/dev/null && { say 'FAIL table still present after empty apply'; fail=$((fail+1)); } || say 'PASS owned table absent after empty apply'
check 'ping passes again (canonical empty state)' pass ip netns exec "$CLI" ping -c2 -W2 10.250.0.6

say "== first rule insertion on a clean table =="
render "$BATCH" 'ip saddr 10.250.0.2 drop'
q "$NFT" -f "$BATCH" && say 'PASS first-rule batch applies to clean table' || { say 'FAIL first-rule batch'; fail=$((fail+1)); }
check 'drop active again' fail ip netns exec "$CLI" ping -c2 -W2 10.250.0.6

q "$NFT" delete table inet duckos_fw 2>/dev/null
q "$NFT" delete table inet duckos_upnp 2>/dev/null

if [[ "$fail" -ne 0 ]]; then say "RESULT: $fail packet test(s) FAILED"; exit 1; fi
say 'RESULT: all packet-level qualification checks passed'
