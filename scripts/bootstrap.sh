#!/usr/bin/env bash
# Download checksum-pinned build tools into this checkout; never install system-wide.
set -euo pipefail
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) PLATFORM=darwin-arm64; GO_SHA=ee215d57e0ec269c60cc9ceca68e6bda321ba9ee5afe24f4b0988703c2d87d12; NODE_PLATFORM=darwin-arm64; NODE_SHA=40e5607e5ecb3db9192723776da2d75d966260fc74a7a9e731c1bd67dda96bc8 ;;
  Darwin/x86_64) PLATFORM=darwin-amd64; GO_SHA=8f8f52c6649542cf027bbc9b9c68d1ec042f9f34808a40413f0b8b3f66f3caa4; NODE_PLATFORM=darwin-x64; NODE_SHA=9e5b2644cf107befb6aefca676b96d3296bc10138096f022ed378d6233ed81f4 ;;
  Linux/x86_64) PLATFORM=linux-amd64; GO_SHA=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445; NODE_PLATFORM=linux-x64; NODE_SHA=855d581f8a4eb1a8117e3426de25fe02770592febcfb31369aee1ffbfee9e8ec ;;
  Linux/aarch64) PLATFORM=linux-arm64; GO_SHA=3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec; NODE_PLATFORM=linux-arm64; NODE_SHA=3515603e2487879a39bc75716f1a2affd027500c64ba50e845cf72cb33219013 ;;
  *) printf '%s\n' 'Build host must be macOS or Linux amd64/arm64. ARMv7 is cross-built.' >&2; exit 1 ;;
esac
mkdir -p "$ROOT/.tools"
TASK_TMP=$(mktemp -d "$ROOT/.tools/download.XXXXXXXX")
trap 'rm -r -- "$TASK_TMP"' EXIT
verify() {
  local actual
  if command -v sha256sum >/dev/null; then actual=$(sha256sum "$1"); else actual=$(shasum -a 256 "$1"); fi
  [[ ${actual%% *} == "$2" ]] || { printf '%s\n' 'Toolchain checksum mismatch; refusing extraction.' >&2; exit 1; }
}
if [[ ! -x "$ROOT/.tools/go/bin/go" ]]; then
  [[ ! -e "$ROOT/.tools/go" ]] || { printf '%s\n' 'Existing incomplete .tools/go; inspect it before retrying.' >&2; exit 1; }
  curl --proto '=https' --tlsv1.2 --fail --location --retry 2 --connect-timeout 15 --max-time 180 "https://dl.google.com/go/go1.27.1.$PLATFORM.tar.gz" -o "$TASK_TMP/go.tar.gz"
  verify "$TASK_TMP/go.tar.gz" "$GO_SHA"
  tar -xzf "$TASK_TMP/go.tar.gz" -C "$TASK_TMP"
  mv "$TASK_TMP/go" "$ROOT/.tools/go"
fi
if [[ ! -x "$ROOT/.tools/node/bin/node" ]]; then
  [[ ! -e "$ROOT/.tools/node" ]] || { printf '%s\n' 'Existing incomplete .tools/node; inspect it before retrying.' >&2; exit 1; }
  curl --proto '=https' --tlsv1.2 --fail --location --retry 2 --connect-timeout 15 --max-time 180 "https://nodejs.org/dist/v24.20.0/node-v24.20.0-$NODE_PLATFORM.tar.gz" -o "$TASK_TMP/node.tar.gz"
  verify "$TASK_TMP/node.tar.gz" "$NODE_SHA"
  tar -xzf "$TASK_TMP/node.tar.gz" -C "$TASK_TMP"
  mv "$TASK_TMP/node-v24.20.0-$NODE_PLATFORM" "$ROOT/.tools/node"
fi
"$ROOT/.tools/go/bin/go" version
"$ROOT/.tools/node/bin/node" --version
printf '%s\n' 'Toolchains ready. Use make deps, make check, make build.'
