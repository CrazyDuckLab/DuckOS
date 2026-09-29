# DuckOS V1 — Post-Confirm TCP Loss Trace + Firewall Scope Fix

## ROOT CAUSE

The firewall renderer (`internal/network/render.go`, `Render`) emitted
one `table inet duckos` with an INPUT chain whose policy was
`drop`. The chain added accept rules for:

- loopback (`iifname "lo" accept`)
- established/related (`ct state established,related accept`)
- ICMP (`meta l4proto ipv6-icmp accept`, `ip protocol icmp accept`)
- per-interface management / LAN / DHCP rules for interfaces
  listed in the candidate

Packets whose `iifname` was NOT any of those were blackholed by the
policy drop. Most critically, **TCP to eth0:22 (the host's own
externally-managed management NIC, currently owned by dhcpcd)
matched no accept rule and was therefore dropped**.

This violated an invariant the user made a hard scope rule:

> Confirming an eth1/eth2-only network transaction must never change
> effective INPUT/OUTPUT reachability on externallyManaged eth0.

The renderer was conflating two scopes:

| Scope | Owner | DuckOS authority |
|---|---|---|
| A. host's externally-managed management plane (eth0 under dhcpcd) | dhcpcd / host administrator | none — DuckOS does not enumerate its services |
| B. candidate's DuckOS-managed links (eth1, eth2) | DuckOS | full |

The renderer must filter only scope B. Scope A must pass through.

## EXACT nft/routing verdict that killed TCP

The verdict for `iifname "eth0" tcp dport 22` packets was:

```
chain input {
  type filter hook input priority filter; policy drop;   ← verdict: drop
  iifname "lo" accept
  ct state invalid drop
  ct state established,related accept
  meta l4proto ipv6-icmp accept
  ip protocol icmp accept
  ...per-interface rules for eth1, eth2 only...
}
```

The packet walked the chain from the top. No rule matched
(`iifname "eth0"` is not lo, the conntrack entry is not yet
established on a fresh SYN, eth0 is not eth1 or eth2). It reached
the chain end, and the policy `drop` verdict was returned. The
kernel silently dropped the SYN. No SYN-ACK was ever generated.

ICMP worked because the chain had an explicit
`ip protocol icmp accept` rule. Loopback worked because of the
`iifname "lo" accept` rule.

## COMMIT FIXING IT

`595b2b7 network: scope firewall INPUT chain to DuckOS-managed interfaces only`

The fix prepends one accept rule to the INPUT chain:

```nft
chain input {
  type filter hook input priority filter; policy drop;
  iifname != { "eth1", "eth2" } accept       ← new scope clause
  iifname "lo" accept
  ct state invalid drop
  ct state established,related accept
  meta l4proto ipv6-icmp accept
  ip protocol icmp accept
  ...per-interface rules...
}
```

The clause lists the interfaces DuckOS owns in the candidate. Packets
whose `iifname` is NOT in that set are accepted before any other rule
evaluates, so the policy drop never fires for them.

- No rule by name targets any externally-managed interface.
- No IP is hardcoded.
- The firewall is not disabled.
- The externallyManaged protection is preserved (eth0 stays out of
  DuckOS scope).

The renderer already knew which interfaces DuckOS owns from the
candidate (those are exactly `c.Interfaces`). The clause
`iifname != { <owned> } accept` is the ownership/scope fix the
witness required. The forward chain is left untouched: its policy
drop is documented safe for forwarded traffic (only DuckOS-owned
ingress/egress combos reach the forward hook).

## TARGETED TESTS

| Test | Asserts |
|---|---|
| `TestRenderFirewallINPUTChainScopedToDuckOSManagedInterfaces` | (a) `iifname != { "testlan0", "testwan0" } accept` appears in `p.NFTables`; (b) the clause lives inside `chain input {`; (c) the clause precedes every per-interface rule in the chain; (d) the clause does NOT appear in the `chain forward {` block. |
| `TestRenderFirewallINPUTChainEmptyCandidateIsUnconditionalAccept` | An empty candidate renders `destroy table inet duckos` only; no INPUT chain is installed. |
| `TestJoinQuotedEmitsSafeSetLiteral` | The helper that builds the iifname-set argument emits each name as a quoted token (nftables-safe against dashes and other tokenisers). |

`go test ./internal/network/...` and `go vet ./internal/network/...`
are green on the fix commit.

## LIVE TCP CONTINUITY: CANNOT PROVE until management plane is recovered

The previous witness's confirmed candidate left the
broken-render's INPUT chain installed on `duck-dev-01`. With that
chain still in place, all TCP from the harness to `172.18.4.149:22`
is dropped at the kernel. ICMP continues to work.

I cannot run the live witness because I have no path to clear the
firewall on `duck-dev-01` without root on the host:

- SSH/22 is blocked by the bad render (the bug under fix).
- The duck-api listens on `127.0.0.1:8080` only — no external access.
- No console or out-of-band management is exposed to the harness.
- nftables rules are in kernel memory only; they do not persist
  across reboot, but I have no way to trigger a reboot from the
  harness.

Recovery requires a one-shot out-of-band `nft delete table inet
duckos` on the host (manual intervention that the directive
otherwise forbids, but is the only path to clear the previous
witness's broken render and re-enable the witness). After that,
the new build can be installed and the witness can run end-to-end.

## ETH0 NETLINK EVENTS

0 — eth0 link state never changed in any live witness. The TCP drop
is purely a kernel verdict on inbound packets, not a netlink event.

## SSH FAILURES

- The previous witness's confirmed candidate left a kernel-state
  INPUT chain installed on `duck-dev-01` that drops TCP/22 from any
  iifname that is not `lo` or one of the candidate's interfaces.
  No externally-managed interface is in that set. Recovery requires
  the broken chain to be cleared from the host.

## FINAL HEAD

`595b2b7 network: scope firewall INPUT chain to DuckOS-managed interfaces only`

## WORKING TREE

clean (`git status` reports nothing to commit; HEAD is `595b2b7`)

## Summary of all defects found in this thread

| # | Defect | Root cause | Commit fixing it |
|---|--------|-----------|------------------|
| A.1 | Recovery could not clear networkd's cached `configured` state without `systemctl restart systemd-networkd` | `qualifyRefresh` rejected links whose `.network` was removed but `AdministrativeState` had not caught up | `fdea306` |
| A.2 | `protected network state changed during refresh` on every apply | per-iteration protected comparison; networkd's inotify re-evaluation briefly perturbed eth0 | `ed00408` (move check to once after convergence) |
| A.3 | `protected network state changed during refresh` on rollback | the snapshot included DuckOS-managed links (eth1/eth2) which legitimately change across apply AND rollback | `12fc492` (restrict snapshot to enrollment-externally-managed links) |
| A.4 | `networkd convergence interrupted` at 40 s | inner 40 s `WithTimeout` capped the parent 90 s window prematurely | `7e125a7` (drop inner cap) |
| A.5 | eth1 stuck in `AdministrativeState=configuring` for the parent deadline | redundant `networkctl reconfigure eth1 eth2` raced with networkd's own inotify-driven evaluation | `7e125a7` (suppress per-interface reconfigure) |
| A.6 | WAN physical interfaces rendered without `ConfigureWithoutCarrier=yes` | the renderer emitted the directive only for `role=lan` | `7e125a7` (emit on every physical role) |
| A.7 | Post-Confirm TCP to eth0:22 blackholed for hours | the firewall's INPUT chain had `policy drop` and only accepted packets whose iifname was in the candidate; eth0 (externally-managed) was not in the candidate and had no accept rule | **`595b2b7` (this commit)** — scope the INPUT chain to DuckOS-managed interfaces only |

## Hard scope rule for the next witness run

The next witness on `duck-dev-01` MUST be preceded by an
out-of-band `nft delete table inet duckos` on the host. After that,
install the `595b2b7` build and re-run the chain:
clean install → empty durable Config → adopt eth1+eth2 → diff →
apply → awaiting-confirmation → confirm → second apply → rollback
→ restart/reconciliation. The internal loopback probe and an
external `nc -z 172.18.4.149 22` must both stay UP at every step.
