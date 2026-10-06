duck-agent
=========

User: root
Group: duckos
SupplementaryGroups: frr, frrvty
NoNewPrivileges: yes
CapabilityBoundingSet: (empty)
AmbientCapabilities: (empty)

Effective / Permitted / Inheritable caps at runtime:
  CapBnd = 0
  CapPrm = 0
  CapEff = 0
  CapAmb = 0

The agent does MkdirAll, gid-only chown, chmod, and
write of frr.conf / wg.conf / xray catalog files.
None of those operations require caps. UID 0 owner-
rwx on the parent paths is sufficient when
ReadWritePaths is set (which it is in the unit
file).

The agent then dials the duck-wg-launcher daemon
over /run/duckos-agent/wg-launcher.sock and asks it
to do the kernel-side operations. The launcher
(running in its own systemd unit with the
CAP_NET_ADMIN-only model documented below) is the
canonical privilege holder.

The agent NEVER exec's /sbin/ip or /usr/bin/wg
directly. It talks to the launcher over a Unix
socket. The launcher validates SO_PEERCRED on
every connection and only accepts UID 0
(matching the agent's UID).

CAP_SYS_ADMIN is therefore NEVER granted to the
agent. CAP_NET_ADMIN is therefore NEVER granted to
the agent. CAP_SETUID, CAP_SETGID, CAP_SETPCAP,
CAP_NET_RAW — also never granted.

duck-wg-launcher
================

User: root
Group: root
NoNewPrivileges: no
CapabilityBoundingSet: CAP_NET_ADMIN
AmbientCapabilities: CAP_NET_ADMIN

Effective caps at runtime:
  CapBnd = CAP_NET_ADMIN
  CapPrm = CAP_NET_ADMIN
  CapEff = CAP_NET_ADMIN  (set via AmbientCapabilities)
  CapAmb = CAP_NET_ADMIN

WHY CAP_NET_ADMIN IS NEEDED (and why it is
sufficient)

Every DuckOS WireGuard operation we actually
perform maps to a single capability, CAP_NET_ADMIN:

  ip link add type wireguard    : CAP_NET_ADMIN
  ip address add / del          : CAP_NET_ADMIN
  ip link set up / down         : CAP_NET_ADMIN
  ip link del                   : CAP_NET_ADMIN
  ip route add / del (if used)  : CAP_NET_ADMIN
  wg setconf                    : CAP_NET_ADMIN
  wg show                       : CAP_NET_ADMIN

Verified by reading the iproute2 source (ip / ip
link / ip / route) and the wireguard-tools source
(wg / wg setconf). Every privileged operation in
both code bases checks CAP_NET_ADMIN only.

WHY OTHER CAPS ARE NOT NEEDED (and must not be
added)

CAP_SYS_ADMIN: not required for any WireGuard
operation. Adding it would shadow the privilege
boundary; the BETA.2 review forbade adding it
unless an independent operation needs it.

CAP_NET_RAW: WireGuard operates on the netlink
generic socket family which is governed by
CAP_NET_ADMIN. CAP_NET_RAW is for raw / packet
sockets which DuckOS WG does not use.

CAP_SETUID / CAP_SETGID: the launcher does not
change its uid. It runs as root throughout. The
agent is the only caller and it is also root;
SO_PEERCRED gates the connection. We do NOT need
to drop to another uid for WG operations; the
kernel does not require it.

CAP_SETPCAP: the launcher does not call
cap_set_proc. It runs as root with the bound
capability set; /sbin/ip and /usr/bin/wg inherit
the parent's Effective set. No narrowing is
needed at exec time; the launcher's Bnd is the
canonical gate.

CAP_NET_BIND_SERVICE: WireGuard binds to a UDP
port via `wg setconf ListenPort`. The bind(2)
syscall on a privileged port (< 1024) requires
CAP_NET_BIND_SERVICE. DuckOS WireGuard uses
non-privileged ports (51820 by default, range
> 1024) so this cap is NOT required. The launcher
never binds to port < 1024; the protocol parser
rejects listen_port=0 outright.

CAP_KILL: the launcher does not signal / terminate
processes. /sbin/ip and /usr/bin/wg do not signal
processes either. CAP_KILL is therefore not
required.

WHY NoNewPrivileges=no

The launcher is the privilege holder. It does not
exec setuid binaries (none of /sbin/ip, /usr/bin/wg
have file caps that the launcher would consume);
it does not need file-cap inheritance. The setting
mirrors the FRR launcher's convention (set in
commit ec97736 / dd8c4ea).

The agent, in contrast, KEEPS NoNewPrivileges=yes
because the agent must not gain privileges through
file caps on binaries it exec's. The launcher is
the only thing that talks to /sbin/ip and
/usr/bin/wg.

WHY User=root AND Group=root

The launcher executes /sbin/ip and /usr/bin/wg.
Both binaries inspect the caller's EUID for the
legacy "is the caller root" check (a few older
utilities bypass the capability check on EUID==0).
Running as UID 0 + GID 0 is the simplest model.
The launcher does not need to drop privileges; it
has no other role.

WHY LockPersonality / RestrictRealtime /
RestrictSUIDSGID / ProtectKernelTunables /
ProtectKernelModules / ProtectControlGroups /
ProtectSystem=strict / ProtectHome /
SystemCallArchitectures=native

These are the standard launcher hardening knobs:

  LockPersonality
    Prevents the launcher from changing its
    personality(2) domain. Blocks a family of
    escaping-the-sandbox exploits.

  RestrictRealtime
    Prevents the launcher from acquiring SCHED_FIFO
    / SCHED_RR priorities. No reason for a WG
    launcher to need realtime scheduling.

  RestrictSUIDSGID
    Prevents the launcher from creating setuid /
    setgid files. /sbin/ip and /usr/bin/wg do not
    create files; the launcher writes only to its
    own owned state dir.

  ProtectKernelTunables
    Read-only on /proc/sys, /sys, etc.

  ProtectKernelModules
    Cannot load / unload kernel modules.

  ProtectControlGroups
    Read-only cgroup hierarchy.

  ProtectSystem=strict
    Read-only / (with the ReadWritePaths escape
    hatch — but the WG launcher does not need
    ReadWritePaths because it only writes to the
    launcher-owned state / secret dirs which are
    already covered by RuntimeDirectory).

  ProtectHome
    Read-only /home, /root, /run/user.

  SystemCallArchitectures=native
    Refuses syscall emulation (ia32 / x32).

WHY NOT CapabilityBoundingSet=CAP_NET_ADMIN,
AmbientCapabilities=CAP_NET_ADMIN ALONE without
Execve post-processing

Setting CapabilityBoundingSet + AmbientCapabilities
on the launcher is sufficient because /sbin/ip and
/usr/bin/wg do NOT need CAP_SETPCAP to operate. The
launcher's child inherits the parent's Effective
set (CAP_NET_ADMIN via AmbientCapabilities) at
exec. /sbin/ip performs its privileged operation
under that cap and exits. The child never needs to
narrow the cap set further.

This is fundamentally different from the FRR
launcher, which DOES need to drop to uid=frr via
setresuid (FRR docs / install instructions
explicitly require this) and therefore needs
CAP_SETUID + CAP_SETGID + CAP_SETPCAP at exec
time. The FRR launcher also drops SETUID +
SETGID from its bounding set BEFORE setresuid.

The WG launcher runs as root throughout; it does
not drop uid. Therefore it does NOT need SETUID,
SETGID or SETPCAP. Adding them would be a
gratuitous privilege expansion.

WHY NOT install setcap on /sbin/ip / /usr/bin/wg

Because the launcher inherits the privilege from
its Bnd + AmbientCapabilities, the binaries do not
need file caps. install.sh does NOT setcap /sbin/ip
or /usr/bin/wg; this is by design. Any future
caps the launcher needs are granted via the
service unit, not via file caps.

The shared abstraction between FRR launcher and WG
launcher is the launcher pattern itself (Unix
socket + line protocol + privileged service).
The capability sets differ because the two
problems are different. The BETA.2 review asked
specifically that the WG launcher NOT inherit the
FRR launcher's caps; the privilege model above is
the minimal-cap version of that pattern."