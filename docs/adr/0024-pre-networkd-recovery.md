# ADR 0024: Pre-networkd artifact recovery gate

Status: Implemented component; integrated cold-boot qualification pending.

The root `duck-agent --recover-network-artifacts` command opens the existing
private Agent journal and restores pending before-state networkd artifacts
before networkd loads them. Missing state is an error, never a new empty journal.
Unknown managed files and foreign bytes block recovery. This phase never invokes
network commands, confirms a candidate or clears the pending record; runtime
recovery and verification remain mandatory.

The packaged `duck-network-recover.service` is a network-isolated root oneshot
ordered before networkd, with no capabilities and bounded resources. Its
persistent required dependency, exact unit, successful loaded state and ordering
are verified before managed routing activation. Installation alone does not
enable it, provision enrollment/journal or modify networkd dependencies.

Tests cover all pending phases, restart/file recovery, idempotence, missing state,
unknown artifacts, service/dependency drift and installation non-activation.
Actual systemd ordering, kernel-state reconstruction and management preservation
are not qualified. The [current roadmap](../handoff/CURRENT_ROADMAP.md) names the
remaining activation/security gates, including the no-pending enrollment check.
