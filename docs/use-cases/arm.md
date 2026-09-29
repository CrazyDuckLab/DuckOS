# ARM and ODROID deployment

DuckOS targets arm64 and armv7 as well as amd64. ODROID XU4 is the minimum resource
reference, not an automatic certification of every driver or peripheral. There
is no board-specific product fork. Native ARM deployment qualification remains
incomplete; the current fresh TEST installer targets Debian 13 amd64.

Before an ARM trial, inventory architecture/ABI, supported Debian userspace,
kernel, NIC/USB drivers, storage, power stability and a recovery console. A static
Go cross-build does not verify boot, systemd sandboxing, nftables, Wi-Fi or modem
support. ARMv7 is currently a runtime cross-build target, not a promise that the
pinned frontend toolchain runs natively there.

Measure idle/load memory, CPU, temperature, storage writes and recovery time.
Optional heavy diagnostics must respect the resource guard rather than exhaust
RAM or fill flash. Report negative results and exact tested peripherals using
the [compatibility model](../hardware-compatibility.md). See
[hardware support](../hardware-support.md) and [test tiers](../testing.md).
