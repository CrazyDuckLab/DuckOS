# B4-04 — Security decision record: Agent privilege model

Status: **BLOCKED for acceptance** — requires project-owner / independent
release-reviewer approval of exactly one of the two contracts below. No
code change was made in this remediation.

## Observed facts (verified)

- `packaging/systemd/duck-agent.service`: `User=root`,
  `CapabilityBoundingSet=` (empty), `AmbientCapabilities=` (empty),
  `NoNewPrivileges=yes`. The same unit exists in the qualified
  beta.3 Router Core (`2e0c5af`); the merge did not change it.
- On the laboratory VMs the Agent really runs as UID 0 with an empty
  capability bounding set.
- The empty bounding set removes all capabilities — including
  DAC_OVERRIDE — but UID 0 still passes ordinary DAC checks against
  files owned by UID 0 (the install root, agent state dirs, launcher
  sockets are root-owned). The Agent's own design restricts it to a
  read-only view plus bounded delegation to the privileged launchers.

## Option A — genuine non-root Agent

Requires: all agent state directories chowned to a dedicated user;
launcher sockets group-accessible to that user; SO_PEERCRED-based IPC
authorization reworked to match; installer migration for existing
hosts; full re-qualification of Router Core startup (beta.3 GO covers
the current root model). Estimated as a separate architectural
migration; NOT performed here because it cannot be validated without
re-opening the qualified Router Core.

## Option B — accepted UID-0 zero-capability model

Remains as built. Compensating restrictions in place: empty
capability bounding set (no CAP_* — in particular no
DAC_OVERRIDE/CHOWN/SYS_ADMIN), NoNewPrivileges, ProtectSystem-style
hardening where present, launcher separation for all privileged
operations (nft/FRR/WireGuard), SO_PEERCRED socket ownership checks.
Residual DAC risk: UID 0 read/write access to root-owned files
(install tree, agent state, vault master key material stored
root-only). This is strictly narrower than an unconfined root but is
not "non-root".

## Decision

The acceptance gate for the non-root Agent contract is **BLOCKED**
pending explicit owner/reviewer choice between A and B. Neither the
remediation branch nor the qualified Router Core was modified.
