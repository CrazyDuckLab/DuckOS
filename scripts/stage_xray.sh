#!/usr/bin/env bash
# stage_xray.sh — build-time helper that vendors an integrity-pinned
# xray-core binary into the DuckOS build artifact.
#
# This script runs ONLY at build time (not on the target host). It
# downloads the upstream zip once, verifies its digest against a
# pinned manifest, and stages the inner `xray` ELF into the
# per-architecture build directory.
#
# Invariants:
#   - The upstream URL and SHA256 list are pinned and reviewed.
#   - The artifact's own SHA256SUMS manifest catches any
#     post-build tampering.
#   - The installer (scripts/install.sh install_xray_backend) re-
#     verifies the staged binary against both an embedded list and
#     the artifact's own manifest before installing.
#   - xray-core is MPL-2.0; the upstream LICENSE is staged next to
#     the binary so /usr/local/share/doc/xray/LICENSE is available
#     after install.
#
# Usage:
#   bash scripts/stage_xray.sh <dist-arch-dir>
# e.g. bash scripts/stage_xray.sh dist/duckos-linux-amd64
#
# Environment overrides (for offline air-gapped builds):
#   XRAY_CORE_STAGE_DIR   pre-staged xray-core root (must already
#                         contain the per-arch binary, LICENSE,
#                         XRAY_CORE_VERSION, XRAY_CORE_SHA256SUMS)
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
DIST=${1:-}
[[ -n "$DIST" ]] || { printf 'Usage: bash scripts/stage_xray.sh <dist-arch-dir>\n' >&2; exit 2; }
[[ "$DIST" = /* ]] || DIST="$ROOT/$DIST"
[[ -d "$DIST" ]] || { printf 'stage_xray: %s is not a directory\n' "$DIST" >&2; exit 2; }
case "$DIST" in
  *duckos-linux-amd64|*build-amd64*) ARCH=amd64;  BINNAME=Xray-linux-64 ;;
  *duckos-linux-arm64|*build-arm64*)  ARCH=arm64;  BINNAME=Xray-linux-arm64-v8a ;;
  *duckos-linux-armv7|*build-armv7*)  ARCH=armv7;  BINNAME=Xray-linux-arm32-v7a ;;
  *) printf 'stage_xray: cannot infer architecture from %s\n' "$DIST" >&2; exit 2 ;;
esac

VERSION=${XRAY_CORE_VERSION:-26.3.27}
UPSTREAM_BASE="https://github.com/XTLS/Xray-core/releases/download/v$VERSION"

# Per-arch sha256 of the upstream zip itself. The pinned list is
# the build-time trust anchor; any mismatch aborts the stage.
# We use a function instead of an associative array because
# bash 3.2 (macOS /usr/bin/bash) mis-parses associative-array
# keys with hyphens under `set -u`.
zip_sha256() {
  case "$1" in
    "Xray-linux-64")          printf '%s' "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae" ;;
    "Xray-linux-arm32-v7a")   printf '%s' "c7265ae13c63ca0241a037df4ef960ad37938c8a67d984cc08834b2cfdf5654b" ;;
    "Xray-linux-arm64-v8a")   printf '%s' "4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c" ;;
    *) printf 'stage_xray: unknown zip name %s\n' "$1" >&2; return 1 ;;
  esac
}

if [[ "$ARCH" == "arm64" ]]; then
  printf 'stage_xray: arm64 binary present in upstream v%s; downloading.\n' "$VERSION" >&2
fi

STAGE=$DIST/xray-core
mkdir -p "$STAGE"

if [[ -n "${XRAY_CORE_STAGE_DIR:-}" ]]; then
  # Offline build: caller pre-staged the xray-core directory.
  [[ -f "$XRAY_CORE_STAGE_DIR/$BINNAME" ]] || { printf 'offline stage missing %s\n' "$XRAY_CORE_STAGE_DIR/$BINNAME" >&2; exit 1; }
  install -m 0755 "$XRAY_CORE_STAGE_DIR/$BINNAME" "$STAGE/$BINNAME"
  install -m 0644 "$XRAY_CORE_STAGE_DIR/LICENSE"     "$STAGE/XRAY_CORE_LICENSE"
  printf '%s\n' "$VERSION" > "$STAGE/XRAY_CORE_VERSION"
  ( cd "$STAGE" && sha256sum "$BINNAME" LICENSE > XRAY_CORE_SHA256SUMS )
  printf 'stage_xray: copied from %s\n' "$XRAY_CORE_STAGE_DIR"
  exit 0
fi

# Online build: download, verify, extract.
WORK=$(mktemp -d "${TMPDIR:-/tmp}/.stage_xray.XXXXXX")
trap 'rm -rf -- "$WORK"' EXIT

ZIP="$WORK/$BINNAME.zip"
URL="$UPSTREAM_BASE/$BINNAME.zip"
EXPECTED=$(zip_sha256 "$BINNAME")
[[ -n "$EXPECTED" ]] || { printf 'stage_xray: no pinned sha256 for %s\n' "$BINNAME" >&2; exit 1; }

# Download with curl-or-wget fallback. We refuse to fall back to
# silent `curl | sh` patterns; the artifact is bytes on disk that
# we then hash-verify.
if command -v curl >/dev/null 2>&1; then
  curl --fail --location --silent --show-error --output "$ZIP" "$URL"
elif command -v wget >/dev/null 2>&1; then
  wget --quiet -O "$ZIP" "$URL"
else
  printf 'stage_xray: neither curl nor wget is available.\n' >&2
  exit 1
fi

ACTUAL=$(sha256sum < "$ZIP" | awk '{print $1}')
[[ "$ACTUAL" == "$EXPECTED" ]] || {
  printf 'stage_xray: upstream zip sha256 mismatch.\n  expected: %s\n  actual:   %s\n' "$EXPECTED" "$ACTUAL" >&2
  exit 1
}

# Unzip. The zip contains a single `xray` ELF and a `geoip.dat`
# / `geosite.dat`; we extract only the binary.
if ! command -v unzip >/dev/null 2>&1; then
  printf 'stage_xray: unzip is required.\n' >&2
  exit 1
fi
unzip -p "$ZIP" xray > "$STAGE/$BINNAME"
chmod 0755 "$STAGE/$BINNAME"

# Fetch the LICENSE file. Xray-core does not attach a LICENSE asset
# to its GitHub releases (that URL is a permanent 404); the
# authoritative copy lives in the source tree at the release tag.
# The license is mandatory in every artifact, so a failed fetch
# fails the build instead of producing an incomplete archive.
LICENSE="$WORK/LICENSE"
curl --fail --location --silent --show-error --output "$LICENSE" \
  "https://raw.githubusercontent.com/XTLS/Xray-core/v$VERSION/LICENSE"
[[ -s "$LICENSE" ]] || { printf 'stage_xray: mandatory XRAY_CORE_LICENSE could not be staged; refusing to produce an incomplete artifact.\n' >&2; exit 1; }
install -m 0644 "$LICENSE" "$STAGE/XRAY_CORE_LICENSE"

printf '%s\n' "$VERSION" > "$STAGE/XRAY_CORE_VERSION"
( cd "$STAGE" && sha256sum "$BINNAME" > XRAY_CORE_SHA256SUMS )

# The installer (scripts/install.sh) re-verifies the staged ELF
# against a pin embedded at build time. Re-derive that pin here
# from the freshly extracted binary so a drift between the two
# lists fails the build instead of failing the install on a
# user host. The zip digest that authenticated the download is
# deliberately NOT an acceptable pin value: the installer hashes
# the extracted ELF, not the archive.
ELF_SHA=$(sha256sum < "$STAGE/$BINNAME" | awk '{print $1}')
PINNED=$(sed -n "s/^.*'${ARCH}:${BINNAME}:\([0-9a-f]\{64\}\)'.*$/\1/p" "$ROOT/scripts/install.sh" | head -n 1)
[[ -n "$PINNED" ]] || { printf 'stage_xray: no installer pin found for %s:%s in scripts/install.sh\n' "$ARCH" "$BINNAME" >&2; exit 1; }
[[ "$ELF_SHA" == "$PINNED" ]] || {
  printf 'stage_xray: installer pin mismatch for %s.\n  extracted ELF sha256: %s\n  installer pin:        %s\nUpdate XRAY_CORE_VERSIONS in scripts/install.sh (and the test trust anchor) to the extracted-ELF digest.\n' \
    "$BINNAME" "$ELF_SHA" "$PINNED" >&2
  exit 1
}

printf 'stage_xray: vendored %s (sha256 %s) and LICENSE under %s\n' \
  "$BINNAME" "$ACTUAL" "$STAGE"