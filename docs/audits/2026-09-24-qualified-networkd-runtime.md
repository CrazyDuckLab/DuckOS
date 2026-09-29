# Qualified networkd runtime application

Scope: the remaining profile-cache High after
`c7f578f743155cf8c644ae31f869b9963dc097a4`, on `V1 UI qualification branch`.
This is not a new backend audit. No UI, DV07, deployed binaries, host network
configuration, or systemd service restrictions were changed.

## Mechanism and admission boundary

`ManagedRoutingAdapter` qualifies the host before any forwarding guard or
artifact mutation. It captures link/address/MTU evidence alongside the existing
durable route/rule/firewall evidence and requires the before configuration to
match observed static runtime. Recovery validates this evidence against its
transaction fingerprints; pre-existing journal records remain readable.

Only scoped changed artifacts are atomically persisted. The daemon host then
requalifies the complete networkd input set and issues one `networkctl reload`.
This is a **qualified manager-wide cache refresh**, not a per-interface reload
primitive and not an unrestricted reload. `networkctl reconfigure` was removed
from this path because it does not load modified profile bytes.

Admission requires:

- Root enrollment, unchanged physical-link identities, and matching owned
  profiles. External enrollment entries cannot appear in rendered intent.
- Installed systemd 257; the qualified host must run its installed package.
  System package maintenance and other root configuration writers must not
  overlap a network transaction. This does not attempt to defend against a
  competing privileged administrator replacing daemon/configuration inputs.
- Standalone static physical-link configuration: no DHCP client, RA, VLAN,
  bridge, netdev, or profile drop-in dependencies.
- Every `.network` in `/etc/systemd/network`, `/run/systemd/network`,
  `/usr/local/lib/systemd/network`, and `/usr/lib/systemd/network` must be exact
  owned renderer output in the authoritative `/etc` directory. Foreign files,
  lower-priority profiles, symlinks, unsafe directories, pending files, and
  missing owned profiles reject the transaction.
- Every unowned link, including external eth0, must already be `unmanaged`,
  have no winning networkd profile and have no drop-ins.

Stock vendor profiles or legacy intent containing external eth0 can therefore
cause a safe refusal. They are **not automatically removed or adopted**. This
change does not claim DHCP/VLAN/bridge or arbitrary mixed-manager support.

## Why the refresh is scoped in effect

In systemd 257, [manager_reload](https://raw.githubusercontent.com/systemd/systemd/v257/src/network/networkd-manager.c)
reloads configuration and requests reevaluation of links. In
[link_reconfigure_impl](https://raw.githubusercontent.com/systemd/systemd/v257/src/network/networkd-link.c),
an unmatched link takes the unmanaged path; `link_enter_unmanaged` immediately
returns when it is already unmanaged. No matching profile or netdev is allowed
by the qualification above. Unchanged profiles retain their network objects in
[network_reload](https://raw.githubusercontent.com/systemd/systemd/v257/src/network/networkd-network.c).
Changed owned profiles get a new object and are actually applied.

Thus eth0 is still *observed* by networkd during manager refresh, but is not
configured, claimed, cycled, or mutated. The isolated witness below checks
kernel multicast events, not just that a command returned successfully.

## Verification and rollback

Convergence requires the same admitted link index/kind, exact owned profile,
configured state, observed static addresses/MTU/up state, and unchanged external
link/address/IPv4+IPv6 route state. Artifact bytes are checked again. Existing
adapter stages independently verify private routes, policy rules, firewall,
and management connectivity before a successful transaction result.

Rollback durably restores the before artifacts and uses the same qualified
refresh primitive. A fresh daemon-host instance can perform recovery. A failed
command, observation, qualification, or convergence never becomes success;
there is no fallback to unconditional reload/reconfigure. Legacy reload entry
points also pass through this gate.

## Isolated runtime witness

`TestQualifiedNetworkdIsolatedRuntime` uses private mount/network/PID namespaces,
a private system bus, the real Debian `systemd-networkd` 257.13-1~deb13u1,
real `networkctl`, actual renderer/artifact persistence, and kernel networking.
Its external eth0 is a veth connected to a gateway in another isolated namespace;
eth1 is a disposable dummy link. Neither has access to host interfaces.

The test calls the exact private production `refreshQualified` primitive below
physical enrollment admission. It does **not** mock inventory, commands, files,
or kernel state and does **not** falsify the dummy's kernel kind. It explicitly
asserts that the separate physical-NIC admission gate rejects a dummy. This is
a runtime-mechanism witness, not a claim of public API/physical hardware E2E.

Assertions cover:

1. A foreign eth0-matching profile causes preflight refusal.
2. Changing the eth1 address becomes visible in the kernel.
3. A route-only profile change creates the exact expected private route snapshot.
4. Candidate artifacts equal the persisted intent after each change.
5. After stopping networkd and removing the owned address/route, restarting the
   daemon reconstructs candidate runtime from disk.
6. Restoring before artifacts through a fresh host restores eth1 addresses and
   exact private routes; candidate routes disappear.
7. eth0 has no winning profile, stays unmanaged, and its address plus IPv4/IPv6
   route JSON remains byte-identical through apply, restart and rollback.
8. A synchronously bound rtnetlink socket observes link/address/route events
   throughout the operation. Loss, truncation, unknown route attribution and
   multipath events fail the witness. eth1 events are its positive control.
9. Continuous eth0-to-gateway ICMP has zero packet loss.

The witness uses read-only private sysfs so networkd does not wait for host
udev. Initial IPv6 DAD is explicitly awaited before opening the observation
window. Host networkd and the installed DuckOS services are never restarted.

## Reproduction

Only in an isolated test checkout on Linux with the required tools installed:

```sh
go test -race ./internal/network ./internal/safeapply -count=1 -timeout=180s
sudo env DUCKOS_NETWORKD_WITNESS=1 go test -race ./internal/network \
  -run '^TestQualifiedNetworkdIsolatedRuntime$' -count=3 -v -timeout=150s
go vet ./internal/network ./internal/safeapply ./cmd/duck-agent
go build -o /tmp/duck-agent-review ./cmd/duck-agent
```

The opt-in witness skips without root and its explicit environment flag.
Unrelated opt-in hardware tests are not a substitute for this required witness.
No full backend regression or live deployment is part of this verification.

## Final results

On the final source, 2026-09-24:

| Check | Result |
| --- | --- |
| `internal/network` race tests | PASS |
| `internal/safeapply` race tests | PASS |
| Required isolated networkd witness under race detector | 3 PASS / 0 FAIL |
| eth0 link/address/route events | 0 in every run |
| eth1 positive-control events | 29 in every run |
| Continuous eth0 gateway traffic | 13/13, 13/13, 12/12 replies; 0% loss |
| eth0 address and IPv4/IPv6 route snapshots | Byte-identical |
| Address / route-only / rollback / restart / disk convergence | PASS |
| Foreign matching profile refusal | PASS |
| Targeted vet and Agent build | PASS |

The admission, drift, identity, legacy-entry and durable-evidence tests also
pass. No unqualified reload fallback remains. The service sandbox is unchanged.

NETWORK SAFE APPLY REVIEW: PASS

SAFE TO DEPLOY TO duck-dev-01: YES

This verdict covers the qualified static mechanism and safe refusal outside
its admission boundary, not automatic qualification of the VM's existing
configuration. No deployment was performed. A future deployment must preserve
the external management link and must not bypass a failed admission check.
