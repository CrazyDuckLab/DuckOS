# Independent update supervisor

`duck-updater` is a root-owned service outside the replaceable release:
`/usr/libexec/duckos/duck-updater`. Its unit and executable are installed once;
normal update transactions never replace them. The copy in a build artifact is
installer input, not the executable used through `/opt/duckos/current`.

The Agent retains its existing sandbox. It sends only typed stage, activate,
rollback and status requests to `/run/duckos-updater/updater.sock`. The socket
directory is 0700, the socket is 0600, and Linux peer credentials must identify
UID 0. The updater has no TCP listener, arbitrary command endpoint, package
hooks, caller-selected executable path or caller-selected systemd unit.

The supervisor verifies Ed25519 signatures, bounded archive paths, native ELF
binaries and signed build identity. It selects only its trusted release tree.
Systemd restarts are limited to `duck-agent.service` and `duck-api.service`.
Activation health requires the selected executable digests to match the actual
systemd MainPID executables and an authenticated readiness response identifying
the expected running API version and commit. Offline WAN operation is not a
health failure.

Activation acceptance is asynchronous: `result: accepted` means queued work,
not a successful activation. Poll upgrade status. The independent supervisor
continues if the Agent or requesting client disconnects. Committed Active stays
the rollback before-image until health succeeds. Pending startup transactions
restore that release and verify it, rather than trusting the provisional link.
An unprovable recovery remains blocked.

The supported package path currently requires schema 1 → 1 and no host package
dependencies. Schema or Debian host mutations are rejected; they do not obtain
an implicit rollback guarantee. Release archives are uncompressed tar contents
inside the signed `.duck` envelope and include `bin/duck-agent`, `bin/duck-api`
and `BUILD-INFO.json` with version, 40-character commit and `linux/<arch>` target.

## Trust and installed baseline

The installer installs the supervisor but does not invent signing trust or
enable updates against an unsigned artifact. Provision the public signing keys
as root-owned `/etc/duckos/update-keys`, one Ed25519 public key in hex per line.
Keep signing private keys off the router.

For an already installed release, provide its exact signed package at
`/var/lib/duckos-updater/bootstrap.duck` (root-owned, private directory), then run:

```sh
/usr/libexec/duckos/duck-updater --enroll-installed
systemctl enable --now duck-updater.service
```

Enrollment verifies the installed files and actual running build before
establishing the rollback baseline; it does not extract or replace installed
files. A mismatched or unverified existing `current` link is refused. An older
release without the required readiness/build contract must not be silently
adopted as a verified baseline.

The private `/var/lib/duckos-updater/workflow.json` journal is single-writer and
fsync/rename durable. Do not manually replace or delete it to clear an error.
Preserve it and the signed release artifacts for recovery diagnosis.
