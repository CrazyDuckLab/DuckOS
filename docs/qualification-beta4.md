# Beta.4 scope, ancestry and qualification contract

Release identifier: **`v0.1.0-beta.4`**, prerelease. The final source is the commit
resolved by the release tag and recorded in every artifact's `BUILD-INFO.json`.
Development build identifiers are not official release versions.

## Accepted lineage and focused changes

- Router Core baseline: `2e0c5afa089649a3238c543a82624075a9bc816e`.
- Integrated Xray Center / node security checkpoint:
  `5c1d707cd5b2e9da28eafdb67f10946a682a43de`.
- Published beta.3 source: `59ad46e7097d2d8a913b399a0b507c7dddf46515`.
- Beta.3 installer ELF32/ELF64 correction is retained by a traceable backport.
- Later catalog security fixes are narrowly backported from `1e2fa743` and
  `0b58c67a`: unrelated-object preservation, durable atomic commits and stopping
  the runtime on uncertain persistence. Their original commit IDs are recorded
  by `cherry-pick -x` in the release lineage.
- P050 routing is corrected without importing the later RC9 UI architecture;
  EN/RU/DE/ES regressions cover the intended Center page.
- Linux firewall unit tests install their fake launcher before service startup.
  No production retry or privilege boundary is weakened.
- Complete signed update packages preserve helper executability and normalize
  file/directory modes despite UMask=0077. Bounded gzip tar fits the existing
  incoming cap; signatures, payload digests and expanded-size limits remain
  mandatory. Legacy raw tar is retained. See [update contract](update-system.md).

This is a descendant of the accepted beta.4 checkpoint. It is not RC9 relabelled.
Apple-style UI / beta.5, later metadata IPC and unrelated feature development
are excluded. Existing public history and tags remain unchanged.

## Product and security boundaries

Center server/client/subscription/node operations use the authenticated public
API / Engine. Missing, unreadable or corrupt identity state is fail-closed.
Node credentials bind to canonical authenticated peer scope; revocation and
rotation completion persist across restart. CLI rotation/recovery use protected
stdin and reject literal credential flags. Vault materialization and bounded
private Agent IPC remain the privileged execution path.

The owner explicitly accepted **B4-04 on 2026-10-30**: preserve the existing
UID-0 Agent with zero effective capabilities and constrained launchers.
UID 0 retains residual root-owned-file access; capabilities do not eliminate
that risk. No general shell or routable Agent listener is introduced.

The historical Router Core F1–F8 fixes remain inherited. nft mutations use the
scoped launcher, authenticated peer credentials, owned-table validation and
bounded resource limits. Safe Apply and management anti-lockout remain required.

## Evidence levels and mandatory release gates

| Evidence | Scope / requirement |
|---|---|
| Historical AMD64 Router Core | Native Linux packet tests, installed startup, authenticated readiness, service restart and VM reboot at the qualified Router Core baseline; inherited evidence, not a new beta.4 whole-system run |
| Historical beta.4 software security | Accepted F-01 identity, F-02 trusted peer scope, F-03 durable revocation, F-04 stdin CLI / four-locale guidance at the checkpoint above |
| Focused release regressions | Must be rerun on the release lineage; commands, source IDs, exit codes and failures belong in the release report |
| Installer / systemd | Fresh Debian 13 on the exact final SHA, verified disposable isolation, snapshot restoration and console; effective installed hardening and authenticated readiness |
| Upgrade / rollback | Beta.3 → beta.4 through the authenticated update contract; Active + Previous plus compatible configuration and durable state; no signature bypass |
| REALITY | Actual Linux client → server traffic, distinct from configuration validation |
| Packaging | Fresh AMD64 / ARM64 / ARMv7 builds at one clean tagged commit, genuine ELF verification, pinned Xray, closed manifests and external SHA256SUMS |
| Publication | All mandatory gates pass; no verified active secret or new material finding; public tag, asset identity, download and digest verification |

Native AMD64 software checks, an ARM64 VM and physical ARM boards are different
execution environments. Cross-builds never establish hardware qualification.
Every failed or unavailable mandatory gate keeps publication blocked. Skipping
installed-system witnesses on an existing development VM is a refusal, not PASS.

Historical security acceptance used explicit safe-stdin replacement tests for
older argv-based fixtures. It did not authorize restoring the unsafe credential
interface or ignoring unrelated failures. Final reports must identify the tests
actually executed and cannot call an incomplete whole-suite run green.

## Snapshot hygiene and recovery

Eight tracked `.runtime-data` laboratory reports are removed from the current
snapshot; ignore and export-ignore rules prevent ordinary reintroduction.
Removal does not erase historical blobs or downloaded copies. Known historical
private topology is not proof of a live credential compromise. New active
credentials or material disclosures remain publication blockers.

The installer remains fresh-install-only. Tarballs are not signed `.duck`
updates; an existing installation must not be deleted to defeat preflight.
Official upgrade trust cannot be replaced by an invented signing key.

See [install](install.md), [limitations](beta-limitations.md), [artifact identity](release-identity.md),
[snapshot history](release-source-hygiene.md), [architecture](architecture.md) and
[release notes](release-notes.md).
