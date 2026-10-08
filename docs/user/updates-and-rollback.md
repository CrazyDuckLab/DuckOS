# Updates and rollback

**v0.1.0-beta.2 is fresh-install-only. No beta.1 → beta.2 in-place upgrade
has been qualified.** Do not pass the release tarball to an update command or
replace installed binaries manually as a supported public upgrade procedure.

The source includes signed `.duck` update foundations, a supervisor and the
Active + Previous release-store invariant. These mechanisms do not establish a
public beta.2 update channel. No public OTA or beta.2 signed upgrade package is
supplied. A successful first install may have Active without Previous.

The broader update design and its evidence requirements are documented in
[update-system.md](../update-system.md) and
[updater-supervisor.md](../updater-supervisor.md).
For installation failures use [recovery](recovery.md) with console access;
do not delete installation or identity state to bypass preflight.
