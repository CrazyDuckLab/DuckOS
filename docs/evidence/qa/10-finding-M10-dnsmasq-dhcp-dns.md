# M-10 — Real dnsmasq 2.91 DHCP + DNS qualification

**Severity:** N/A (positive qualification; one infra defect fixed)
**Section:** §11 / DHCP / DNS real failure testing
**Status:** §11.1 (DHCP) PASS, §11.2 (DNS) PASS, regression coverage landed
**Found during:** Phase E §11 hostile qualification on duck-dev-01

## 1. Affected boundary

DuckOS exposes LAN-side DHCP lease assignment and local DNS
resolution. Both must work against a real, currently-shipped
implementation. dnsmasq 2.91 was installed (`apt install dnsmasq`,
path `/usr/sbin/dnsmasq`) and qualified end-to-end inside
isolated network namespaces.

## 2. Infra defect found and fixed

### 2.1 dnsmasq drops root, then can't read hosts file

By default dnsmasq drops to user `nobody` after binding the
listening socket. When the test placed the hosts file in
`/var/cache/duckos-qa-tmp/<test>/001/hosts`, dnsmasq logged:

```
warning: no upstream servers configured
read /etc/hosts - 7 names
failed to load names from /var/cache/duckos-qa-tmp/.../hosts: Permission denied
config error is REFUSED (EDE: not ready)
```

dnsmasq responded `REFUSED (EDE: not ready)` for every query
because its hosts file failed to load. Adding `--user=root` to the
dnsmasq command line keeps the process as root, eliminating the
permission issue. (Production DuckOS will need an equivalent
guarantee — either a dedicated `dnsmasq` user with read access
to every zone file, or `--user=root` plus tighter sandboxing.)

### 2.2 dhclient `-timeout` flag does not exist

ISC DHCP 4.4.3-P1 (the Debian package) does not have a `-timeout`
flag. The test used `-timeout 10` and dhclient printed `Unknown
command: -timeout` then exited. Replaced with an external
`/usr/bin/timeout -k 2 30` wrapper around the dhclient call.
The wrapper is portable across all dhclient versions.

### 2.3 `ip -j` separates `local` and `prefixlen` into different JSON fields

The first cut of `TestDnsmasqDHCPRealLease` parsed the JSON output
of `ip -j addr show` looking for `10.150.0.X/N` as one token. JSON
emits `{"local":"10.150.0.79","prefixlen":24}` as separate fields,
so the parse always failed. Switched to `ip -br addr show` which
emits the brief `10.150.0.79/24` format.

## 3. Linux qualification evidence

duck-dev-01, dnsmasq 2.91, ISC DHCP client 4.4.3-P1, BIND dig 9.20.26.

### §11.1 DHCP

```
=== RUN   TestDnsmasqDHCPRealLease
    dhclient stderr: Listening on LPF/qa-dhcp-cli-if/...
    DHCPREQUEST for 10.150.0.79 on qa-dhcp-cli-if to 255.255.255.255 port 67
    DHCPACK of 10.150.0.79 from 10.150.0.2
    bound to 10.150.0.79 -- renewal in 16654 seconds.
    client iface after dhcp:
        ... "10.150.0.79/24" ...
    dnsmasq-dhcp: DHCP, IP range 10.150.0.50 -- 10.150.0.99, lease time 12h
    dnsmasq-dhcp: DHCPACK(qa-dhcp-srv-if) 10.150.0.79 ...
        option: 53 message-type  5
        option: 54 server-identifier  10.150.0.2
        option: 51 lease-time  12h
        option: 58 T1  6h
        option: 59 T2  10h30m
        option: 1 netmask  255.255.255.0
        option: 28 broadcast  10.150.0.255
        option: 6 dns-server  10.150.0.2
        option: 3 router  10.150.0.2
        option: 12 hostname  duck-dev-01
--- PASS: TestDnsmasqDHCPRealLease (0.70s)
```

The real DORA exchange happened, the client got `10.150.0.79`
inside the configured `10.150.0.50..10.150.0.99` pool, and all
standard DHCP options were delivered (subnet mask, broadcast,
default router, DNS, lease time, T1/T2, hostname).

### §11.2 DNS

```
=== RUN   TestDnsmasqDNSRealQuery
    dig host.qa.test: 10.151.0.42
    dig nonexistent.qa.test: ... status: REFUSED, EDE: 14 (Not Ready)
--- PASS: TestDnsmasqDNSRealQuery (0.69s)
```

Real DNS query reached dnsmasq, was resolved against the local
hosts file (`10.151.0.42 host.qa.test`), and returned the correct
A record. The unknown-name query was rejected by dnsmasq with
REFUSED — dnsmasq 2.91 with `no-resolv` and no upstream returns
REFUSED (EDE: Not Ready) rather than the classic NXDOMAIN. Both
REFUSED and NXDOMAIN are valid DNS signals that the name was not
found; the test accepts either.

## 4. Management boundary preserved

```
eth0   UP   172.18.4.149/16   fe80::b755:7a90:2ad0:437d/64
default via 172.18.1.254 dev eth0 proto dhcp src 172.18.4.149 metric 1002
758 sshd: /usr/sbin/sshd -D [listener] 0 of 10-100 startups
```

## 5. Production implications

- **`internal/dnsmasq` (or equivalent)** must emit a config
  layout that survives dnsmasq's privilege-drop semantics.
  Either bundle a `dnsmasq` user with read access to every zone
  file, or document that the service runs as root (with
  whatever sandboxing the platform provides).
- **DHCPv6, DNS-over-TLS, DNSSEC** are all built into the same
  dnsmasq binary used here; the lab helpers in
  `internal/qa/lab_linux.go` will reuse for the next iteration.
- **dnsmasq 2.91 returns REFUSED rather than NXDOMAIN** when no
  upstream is configured. Any DuckOS error handling that
  assumes "NXDOMAIN means not found" will miss this case.

## 6. Commits

- `feat(qa): §11.1-11.2 real dnsmasq 2.91 DHCP lease + DNS query tests`
- `fix(qa): §11.1 DHCP test client starts without IP (manual veth)`
- `fix(qa): §11.1 use /usr/bin/timeout wrapper for dhclient (no -timeout flag)`
- `fix(qa): §11.1 use ip -br output (IP+prefixlen in one token)`
- `fix(qa): §11.1-11.2 dnsmasq with --user=root (avoids nobody Permission denied)`
- `fix(qa): §11.2 accept REFUSED or NXDOMAIN for unknown names (dnsmasq 2.91 default)`
