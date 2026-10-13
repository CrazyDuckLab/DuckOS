# CLI and API access

## Beta.2 availability

The official installer payload contains `duck-api`, `duck-agent`, `duck-updater`,
`duck-restore`, `duck-frr-launcher` and `duck-wg-launcher`. It does **not** contain
or install the standalone `duck` CLI. The installed Web UI and authenticated
public API are the normal operator entry points. Commands using `duck` elsewhere
in the guides require a separately built CLI; do not assume it is on PATH.

## Build the CLI from source

On a supported developer build host, use a clean checkout of the release tag
and the [pinned toolchain](development.md). Cross-builds must embed the release
source identity explicitly:


```bash
bash scripts/bootstrap.sh
make deps
bash -c 'source scripts/env.sh; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X github.com/CrazyDuckLab/DuckOS/pkg/buildinfo.Version=1.0.0-dev.$(git rev-parse --short=12 HEAD) -X github.com/CrazyDuckLab/DuckOS/pkg/buildinfo.Commit=$(git rev-parse HEAD)" -o .cache/duck ./cmd/duck'
```

For ARM64 use `GOARCH=arm64`; for ARMv7 use `GOARCH=arm GOARM=7`.
Transfer the resulting executable to the matching Linux host through your
administrator-controlled channel. Invoke it by its explicit path and inspect
`--help` and subcommand help before use. This is a source-build workflow, not an
additional official beta.2 download asset. Offline `owner reset` requires root
and a stopped API; it deletes the Owner and preserves a backup.

## Public API

The API binds to loopback at `127.0.0.1:8080`. Access it through the documented
[SSH forward](install.md). `/api/v1/health` is unauthenticated liveness;
`/api/v1/ready` and `/api/v1/overview` require bearer authentication. Use an
enrolled session and protect tokens from command arguments, logs and issues.

[Shared API contracts](../shared/api/README.md) describe observation, identity,
network preview and simulation. CLI help is defined in `cmd/duck/main.go`.
Private `/agent/v1/*` IPC is not a public HTTP API. Network changes retain
Safe Apply validation, confirmation and rollback semantics.
