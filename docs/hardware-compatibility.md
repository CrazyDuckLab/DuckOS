# Hardware compatibility reports

Build architecture support and device qualification differ. The
[hardware matrix](hardware-support.md) defines platform requirements; no device
is certified because a binary cross-compiles.

| Field | Information |
|---|---|
| Device | Manufacturer/model and revision; omit serial numbers |
| Architecture | amd64/arm64/armv7; native, virtualized or emulated execution |
| Host | Debian, kernel, firmware and driver versions |
| NICs | Chipsets, driver, link type, tested functions; redact real MACs |
| Wi-Fi/modem | Chipset, firmware, mode, regulatory region or not tested |
| Storage | Media, capacity, filesystem; no serial numbers |
| DuckOS version | Exact commit, artifact digest and installation method |
| Topology | Isolated test diagram using reserved example addresses |
| Status | Supported, Community tested, Experimental or Unsupported |
| Notes | Reproduction, failures, resource use and recovery evidence |

**Supported** requires maintainer-reviewed native release qualification.
**Community tested** identifies reproducible contributor evidence, not
certification. **Experimental** means partial or unqualified behavior.
**Unsupported** names a concrete incompatibility or excluded baseline. Untested
functions remain not tested; silence does not imply support. Negative results
are valuable. Wi-Fi/offload/modem behavior cannot be generalized by CPU alone.
