VM-B-REBOOT-BLOCKED

Read-only reachability diagnosis from the lab
operator workstation (172.18.4.77). Captured at
2026-10-06T18:23:33+03:00.

TARGET: 172.18.4.149/16 (management expected on
eth0 per the BETA.2 wiring). The BETA.2 live
qualification reported VM B unreachable after a
scheduled reboot.

== L3 ==

  $ ping -c 3 -W 2 172.18.4.149
  PING 172.18.4.149 (172.18.4.149): 56 data bytes
  Request timeout for icmp_seq 0
  Request timeout for icmp_seq 1
  --- 172.18.4.149 ping statistics ---
  3 packets transmitted, 0 packets received, 100% packet loss

  100% packet loss. ICMP echo either filtered or the
  host is offline at L3.

== L2 ==

  $ arp -an | grep -i 172.18.4.149
  ? (172.18.4.149) at (incomplete) on en0 ifscope [ethernet]

  ARP entry exists with "incomplete" status. The
  operator workstation sent an ARP request but never
  received a reply. This is consistent with one of:
    - host powered off (no NIC to reply);
    - host NIC link down;
    - L2 path broken (cable / VLAN / hypervisor port);
    - host on a different network segment.
  No MAC is observable, so the L2 path cannot be
  attributed to a specific cause from the operator
  side.

== L4 ==

  $ nc -z -G 2 172.18.4.149 22
  (no response)
  $ nc -z -G 2 172.18.4.149 80
  (no response)
  $ nc -z -G 2 172.18.4.149 443
  (no response)
  $ nc -z -G 2 172.18.4.149 8080
  (no response)
  $ nc -z -G 2 172.18.4.149 8443
  (no response)

  No TCP service responds on the canonical ports.
  Either the host is offline or a firewall drops
  traffic before the listening daemon.

== DNS ==

  $ nslookup vm-b
  *** Can't find vm-b: No answer

  $ dscacheutil -q host -a name vm-b
  (no result)

  The hostname "vm-b" is not resolvable from the
  operator workstation. A previous lease with a
  different hostname cannot be ruled out.

== DHCP / lease shape ==

  The operator workstation's own lease is for
  172.18.4.77 (yiaddr=172.18.4.77 in the DHCP ACK).
  172.18.4.149 is therefore NOT a DHCP artifact on
  this workstation. The address is statically
  managed on the VM B host, or managed by a separate
  DHCP scope not observed here.

== Diagnosis conclusion ==

  STATUS: VM-B-REBOOT-BLOCKED
  CAUSE: undetermined from read-only observation
  SUGGESTED OPERATOR ACTIONS:
    - check VM B host power state at the hypervisor
      console;
    - verify VM B NIC link at the hypervisor port;
    - check the VM B host's BIOS / IPMI for boot
      progress;
    - if VM B's management address changed (e.g. a
      DHCP re-lease on a fresh boot), the operator
      workstation has no DNS record to follow;
    - once VM B is reachable, capture:
        journalctl -b -u duck-agent.service
        systemctl status duck-frr-launcher
        systemctl status frr.service (must be masked)
        ip link show

  NO DUCKOS CODE ROOT CAUSE: this is a lab-monitor
  unreachable situation. The corrective round MUST
  NOT assume a DuckOS regression on VM B; the
  blocker is at the hands and after-reboot state of
  the VM B host and requires operator / hypervisor
  action."