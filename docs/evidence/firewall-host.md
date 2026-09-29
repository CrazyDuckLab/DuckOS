# Firewall host mechanism evidence

Date: 2026-09-08. Internal development, not DuckOS 1.0 qualification.

Portable tests exercise the actual command-construction boundary with explicit
synthetic command results: preflight before mutation, drift refusal, malformed
inventory refusal, bounded scoped snapshots, counter-insensitive fingerprints,
and restore/readback ordering. The production implementation invokes the real
Linux nft binary; it has no product simulation mode.

The native test is opt-in and refuses the PID 1 network namespace even as root.
It covers real initial apply, rule changes, fingerprint mismatch, restoration
and complete removal of base chains. To run on a disposable Linux environment
with nftables installed, build the test binary and run in a newly created netns:

```sh
go test -c -o /tmp/duckos-network.test ./internal/network
sudo env DUCKOS_ISOLATED_NETNS_TEST=1 unshare --net -- /tmp/duckos-network.test \
  -test.run '^TestNFTNativeAtomicApplyAndRestore$' -test.v
```

The repository verification workflow includes this test on its disposable Ubuntu
runner, not duck-dev-01; its execution result must be inspected separately.
This does not substitute for Debian 13/systemd or hardware integration.

No native test has been run on the development Mac or duck-dev-01. Passing
portable tests/cross-compilation cannot establish native nft JSON compatibility
or packet-path behavior. There is no enabled network mutation endpoint yet.

## Controller integration, 2026-09-09

Portable race tests now connect the production firewall adapter to the real
durable controller with an explicit synthetic kernel boundary. They cover
uncertain mutation without replay, successful before-state restoration,
recovery-required persistence on restore failure, process-death journal reopen,
and refusal of non-firewall changes or apply without captured evidence.
The native namespace test also includes a real adapter/controller apply and
rollback with nft kernel fingerprint verification. It compiles for Linux amd64;
that compilation is not evidence that native execution passed.

Validation regression cases also reject IPv4 network/broadcast port-forward
targets, DHCP reservations colliding with secondary router addresses, and
zero/multicast/broadcast client MACs. Valid locally administered unicast MACs
remain supported. These checks run independently at Engine and Agent decoding.

Root enrollment and networkd ownership guards are now composed with the
firewall adapter. Portable tests cover NIC replacement, boot index changes,
foreign winning profiles/drop-ins, existing VLAN collisions and restoring the
owned table despite a missing NIC while retaining failed health verification.
`make check`, production frontend/three-architecture cross-builds and Linux
amd64 native-test compilation passed locally. Networkd JSON parsing uses the
reviewed systemd 257 source contract; native runtime evidence remains pending.
