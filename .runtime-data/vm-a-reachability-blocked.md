VM A REACHABILITY — BLOCKED

VM A: duck-beta-01
Expected management eth0: [redacted management address]

From the operator workstation ([redacted management address]):

  $ ping -c 1 -W 1 [redacted management address]
  --- [redacted management address] ping statistics ---
  1 packets transmitted, 0 packets received, 100% packet loss

  $ nc -z -G 3 [redacted management address] 22
  (no response)

  $ arp -an | grep [redacted management address]
  (no ARP entry)

A scan of /24 showed all 15 probed hosts
([redacted management address], [redacted management address], ..., [redacted management address])
time out. The lab subnet is unreachable from this
workstation. The ICMP echo is filtered or the L2
path between the operator workstation and the lab
switch is down. VM B was already on the same
network and was also unreachable (see TASK 7
previous commit). Together this points to a
workstation-side routing problem, not a
DuckOS-side regression.

CONSEQUENCE FOR THE PRE-LIVE GATE
================================
The pre-live source/test gate is fully green (see
.runtime-data/beta2-blocker-corrective-final.md and
.runtime-data/wg-privilege-model.md). TASK 5 live
proof on VM A cannot be executed from this workstation
because the lab network is unreachable.

The corrective round is therefore not BLOCKER-
CORRECTIVE-PASS — the live gate cannot be crossed
from this side.

SUGGESTED OPERATOR ACTIONS
==========================
- restore lab-network connectivity from the
  operator workstation to the [redacted management address]
  segment;
- re-run TASK 5 (WireGuard, Xray, UPnP, RBAC,
  FRR) on VM A;
- if VM A's management address changed at the
  lab side, update the operator workstation's
  known_hosts / DNS;
- the BETA.2 release is gated on VM A passing
  the live proof; until then the round is
  CORRECTIVE-FAIL with the network reachability
  as the only outstanding blocker."