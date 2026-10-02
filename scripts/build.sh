#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "$0")/env.sh"
TARGET=${1:-amd64}
case "$TARGET" in amd64|arm64|armv7) TARGETS=("$TARGET");; all) TARGETS=(amd64 arm64 armv7);; *) printf '%s\n' 'Usage: bash scripts/build.sh [amd64|arm64|armv7|all]' >&2; exit 2;; esac
npm --prefix web run build
COMMIT=$(git rev-parse HEAD)
DIRTY=false
[[ -z $(git status --porcelain --untracked-files=normal) ]] || DIRTY=true
VERSION="1.0.0-dev.${COMMIT:0:12}"
[[ "$DIRTY" == false ]] || VERSION="$VERSION.dirty"
SOURCE_DATE_EPOCH=$(git show -s --format=%ct HEAD)
export SOURCE_DATE_EPOCH
mkdir -p dist
for ARCH in "${TARGETS[@]}"; do
  TASK_BUILD=$(mktemp -d "$ROOT/dist/.build-$ARCH.XXXXXXXX")
  trap 'if [[ -n ${TASK_BUILD:-} && -d "$TASK_BUILD" ]]; then rm -r -- "$TASK_BUILD"; fi' EXIT
  mkdir -p "$TASK_BUILD/bin" "$TASK_BUILD/web" "$TASK_BUILD/systemd"
  GOARCH=$ARCH
  [[ "$ARCH" != armv7 ]] || GOARCH=arm
  for PROGRAM in duck-api duck-agent duck-updater duck-restore; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" GOARM=7 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -buildid= -X github.com/CrazyDuckLab/DuckOS/pkg/buildinfo.Version=$VERSION -X github.com/CrazyDuckLab/DuckOS/pkg/buildinfo.Commit=$COMMIT" \
      -o "$TASK_BUILD/bin/$PROGRAM" "./cmd/$PROGRAM"
  done
  cp -R web/dist/. "$TASK_BUILD/web/"
  # Strip macOS Finder / AppleDouble metadata from the web bundle so the
  # installer's manifest check does not include them in SHA256SUMS.
  find "$TASK_BUILD/web" -name '.DS_Store' -delete -o -name '._*' -delete
  cp packaging/systemd/*.service "$TASK_BUILD/systemd/"
  cp scripts/install.sh "$TASK_BUILD/install.sh"
  # Strip macOS Finder / AppleDouble metadata from the systemd unit files
  # so a later tar on Darwin does not carry ._* sidecar files that the
  # installer's manifest check would treat as unlisted artifact entries.
  find "$TASK_BUILD/systemd" -name '._*' -delete -o -name '.DS_Store' -delete
  # Vendor an integrity-pinned xray-core binary into the artifact.
  # The script is a no-op on architectures upstream does not
  # provide (currently arm64) so the build does not fail for the
  # unsupported target — the installer will refuse cleanly.
  bash "$ROOT/scripts/stage_xray.sh" "$TASK_BUILD"
  printf '%s\n' "$VERSION" > "$TASK_BUILD/VERSION"
  node scripts/manifest.mjs "$TASK_BUILD" "$VERSION" "$COMMIT" "$DIRTY" "$ARCH" "$SOURCE_DATE_EPOCH"
  # Replace only this repository-generated artifact, preserving prior output for review.
  DEST="$ROOT/dist/duckos-linux-$ARCH"
  if [[ -e "$DEST" ]]; then
    [[ -d "$DEST" && ! -L "$DEST" && -f "$DEST/BUILD-INFO.json" ]] || { printf '%s\n' 'Unexpected dist destination; refusing replacement.' >&2; exit 1; }
    OLD=$(mktemp -d "$ROOT/dist/.previous-$ARCH.XXXXXXXX")
    mv "$DEST" "$OLD/payload"
    printf 'Previous local build preserved: %s\n' "$OLD/payload"
  fi
  mv "$TASK_BUILD" "$DEST"
  TASK_BUILD=
  printf 'Built %s (%s; cross-compiled on %s/%s)\n' "$DEST" "$VERSION" "$(uname -s)" "$(uname -m)"
done
