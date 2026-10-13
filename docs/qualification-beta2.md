# Beta.2 qualification and evidence

The current runtime qualification source is
`cdaae71f276a055b1425d8a603931ace33f57906`, build
`1.0.0-dev.cdaae71f276a`. It supersedes the earlier
[historical report](../beta2-final-report.md). The release commit may contain
subsequent documentation changes; each artifact's `BUILD-INFO.json` identifies
its exact source and must match the annotated release tag.

## Evidence boundaries

| Target or gate | Evidence at the qualified runtime source |
|---|---|
| Debian 13 AMD64 | Two-node fresh installation and targeted live qualification passed |
| ARM64 and ARMv7 | Official cross-build and artifact verification passed; equivalent live matrix not completed |
| WireGuard | Two-node tunnels, traffic, peer changes and recovery passed |
| Speed Lab | Bounded probes, finalization, cancellation and SSRF refusal checks passed |
| Boot recovery | AMD64 service/tunnel recovery passed; startup retry regression passed |
| FRR H10 | Live BGP advertise/withdraw/restart passed on AMD64 with Debian FRR 10.3 |
| Web UI | TypeScript and 193 Vitest tests passed in the independent audit |
| Installer | 31 JavaScript tests and four shell regression suites passed in the independent audit |
| Security boundary | Agent effective capabilities zero; WireGuard launcher CAP_NET_ADMIN only |

These results were recorded in the final live qualification and independent
release audit supplied by the maintainer. Release preparation independently
checked source identity, remote ancestry, current artifact metadata and both
VMs' running build, services, API liveness, IPC ownership and capability sets.
It did not repeat the complete destructive installation/reboot matrix.

H08/DV07 passed on the clean qualification environments. They intentionally
refuse on already installed systems; do not wipe an installation to rerun them.
Earlier reports that FRR daemons were absent are historical: the subsequent
independent audit verified FRR availability and the H10 live pass.

## Limits

No physical ODROID-XU4 final qualification, equivalent ARM live matrix,
arbitrary NIC/Wi-Fi compatibility, power-loss matrix or production readiness
is claimed. Hypervisor shutdown incidents occurred during earlier VM testing;
subsequent recovery passed, but host-level root cause remains outside product
qualification. See [limitations](beta-limitations.md) and [compatibility](compatibility.md).

Implemented, configured, observed, verified and qualified are distinct states.
API liveness does not prove network health. Documentation-only release changes
preserve the runtime evidence; runtime changes require affected requalification.
