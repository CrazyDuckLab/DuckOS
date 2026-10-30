# Release artifact identity contract

The release is a closed artifact, identified by `VERSION`, canonical
`BUILD-INFO.json` (full source commit, target, toolchains, clean state and kind),
and internal `SHA256SUMS` covering every payload file, including each executable.
The release's external `SHA256SUMS` binds the complete architecture archives.
Hashes detect corruption; they do not independently authenticate the publisher.

This is the existing contract in `scripts/build.sh`, `scripts/manifest.mjs`,
`scripts/install.sh` and the metadata section of [development](development.md).
The product specification does not require every helper to expose its own
version command or carry independent buildinfo strings. Update-package identity
and signature requirements in [update-system.md](update-system.md) are separate
from these fresh-install archives.

`duck-api` and `duck-agent` import `pkg/buildinfo`, expose build metadata, and
must embed the official release version and full final source SHA. The updater,
restore executable and FRR/WireGuard/nft launchers are bound to the same source
and version by the verified artifact manifest; they are not required to import
an otherwise unused package. Passing linker `-X` does not retain an unused
package and is not evidence of a helper version interface.

T-02 was a release-preparation assumption broader than that existing contract,
not a demonstrated runtime defect. It is resolved by documenting and testing
the actual contract, without modifying the five helpers. The genuine-artifact
regressions in `tests/installer/release-payload.test.mjs` require all seven
binaries to match manifest hashes and architecture, verify clean final source
metadata, check API/Agent embedded identity, and run each artifact's own
production installer verifier. They must be explicitly enabled after build-all
with `DUCKOS_REQUIRE_RELEASE_PAYLOADS=1`; missing architectures then fail.

T-01 is an installer-verifier correction after Router Core qualification. It
uses the actual ELF class, little-endian layout and validated bounds for headers,
sections and file-backed segments. Integer calculations do not wrap at 64 bits.
Unsupported extended numbering is refused. NOBITS sections are correctly
excluded from file-byte extents. `python3`, already an installer prerequisite,
is also required for read-only ELF verification on the build host.
