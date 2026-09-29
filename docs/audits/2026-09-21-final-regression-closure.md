# Final regression closure

Scope: known baseline regression failures only; no new audit, UI work or
production implementation changes. H01–H10 implementation remediation remains
closed. H09 was not reopened.

Verified implementation/test HEAD: `57ca259b7eb18688be20d299e264496077ae9a4f`.

## Commits

- `842073e4d406a5887324088a921b5646b288cf6b` — qualify DV07 activation and
  Previous rollback through the real public/systemd supervisor witness.
- `57ca259b7eb18688be20d299e264496077ae9a4f` — align known regression fixtures
  with current production contracts; remove erroneous timeout skip and WG
  self-assignments. Test-only changes.
- This handoff is a separate documentation-only commit.

## Classification

| Baseline case | Classification | Correction and retained oracle |
| --- | --- | --- |
| DV07 dirty fixture | STALE FIXTURE | Replace executor-less metadata assertions with existing real public/systemd supervisor acceptance, including Previous rollback. |
| QoS apply/rollback | STALE FIXTURE | Use durable OwnedRunner and namespace-scoped tc observation instead of LinuxRunner rollback, which correctly refuses unowned deletion. Independent kernel qdisc checks remain. |
| DV00/DV03 rollback | STALE FIXTURE | Same production OwnedRunner wiring; retain mutation, rollback, invalid-plan and failed-runtime checks. |
| FRR OSPFv3 | STALE FIXTURE | Correct /127 endpoint pairing; await actual convergence/withdrawal within a deadline; inspect OSPF link-local next hop rather than requiring a global next hop. Real route and packet oracles pass. |
| FRR/Anycast DV09 | STALE FIXTURE | Use current public FRR production-composition witness with independent BGP oracle and durable restart reconciliation. Old fixture inspected kernel RIB and re-applied state using a retained Manager. Negative fixture now uses canonical `bgp`, not ignored `bgpSessions`. |
| IPv6 firewall | STALE FIXTURE | Correct /127 pairs, explicit routed next hops and DAD readiness. Real ICMPv6 and nftables checks remain. |
| Xray boot | STALE FIXTURE | Enable the server and supply canonical IDs, user/server association, security and host:port. Honor the supported binary override in the H05 fixture. Real daemon listener is checked. |
| Layer-L WG vet | STALE FIXTURE | Remove three no-op self-assignments; assertions unchanged. |
| Timeout SKIP exposed by full run | STALE FIXTURE | Compare actual time.Duration units and fail on mismatch instead of skipping. |

No REAL DEFECT requiring production changes was found in these cases.

## Results

- Full backend/Layer-L: **117 packages PASS, 0 FAIL**. JSON test/subtest
  events: 2257 PASS, 0 FAIL, 21 SKIP in the completed full run.
- Targeted race, entire affected unit packages (`internal/qos`, `internal/frr`,
  `internal/xray`, `internal/agent`): **4 packages, 311 PASS, 0 FAIL, 0 SKIP**.
- Targeted race, real runtime fixtures (`internal/qa`, `tests/integration`,
  `tests/layer_l`): **3 packages, 32 PASS, 0 FAIL, 0 SKIP**.
- `go vet ./...`: PASS. `go build ./...`: PASS.
- The full-run timeout SKIP was subsequently corrected and passed both its
  focused regression and the affected-package race run. It is not an outstanding
  skip. There are **20 remaining documented helper/environment skips**.

Counts are Go JSON test/subtest events, not unique product requirements.
The completed full run preceded the timeout assertion correction. A proposed
second full run was blocked before execution by tool approval quota; it was not
needed to validate that assertion, and was not repeated during final handoff.
Final-session actions only read existing race evidence and checked the management
boundary. No test result is inferred from an unexecuted command.

## Remaining SKIP inventory

- Seven subprocess-helper entries: H03 crash, H09 unauthorized peer, H08 updater
  unauthorized peer, H09 crash, H08 supervisor crash, H08 workflow crash and DV10
  cross-process helper. Parent tests execute the helper scenarios.
- Five capture tests: `/usr/sbin/tcpdump` is absent on the test host.
- Seven native network tests: explicit `DUCKOS_ISOLATED_NETNS_TEST` gate is not
  enabled in the host-level full suite. The management namespace must not be used
  to bypass this safeguard.
- One ACME protocol test: Pebble binary is absent.

These are documented environment/helper exclusions, not claims that unavailable
external-component coverage was executed. Required closure cases all passed.

## Evidence and boundary

Linux evidence retained on duck-dev-01:

- `/tmp/duckos-final-regression-closure.jsonl`
- `/tmp/duckos-final-closure-race-unit.jsonl`
- `/tmp/duckos-final-closure-race-runtime.jsonl`

Existing Xray binary selected through `DUCKOS_XRAY=/tmp/duckos-test/xray/xray`.
No system component was installed for this closure.

Final read-only boundary check, 2026-09-21: eth0 `172.18.4.149/16`, default
gateway `172.18.1.254`, SSH listening on IPv4/IPv6 port 22 — unchanged from the
recorded baseline. Agent and updater sandbox policies were not changed.

FINAL REGRESSION CLOSURE: PASS
