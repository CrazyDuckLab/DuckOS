#!/usr/bin/env bash
# tests/installer/version.test.sh: build version identity contract.
set -euo pipefail
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SOURCE_ROOT=$(CDPATH= cd -- "$HERE/../.." && pwd)
# Never create, force-update or delete tags in the shared release repository.
ROOT=$(mktemp -d)
trap 'rm -rf -- "$ROOT"' EXIT
mkdir -p "$ROOT/scripts"
cp "$SOURCE_ROOT/scripts/version.sh" "$ROOT/scripts/version.sh"
git -C "$ROOT" init -q
git -C "$ROOT" config user.name 'Version fixture'
git -C "$ROOT" config user.email 'fixture@example.invalid'
git -C "$ROOT" add scripts/version.sh
git -C "$ROOT" commit -qm 'version fixture'
git -C "$ROOT" commit --allow-empty -qm 'second fixture revision'
VERSION_SCRIPT="$ROOT/scripts/version.sh"
COMMIT=$(git -C "$ROOT" rev-parse HEAD)
fails=0

expect() { # expect <name> <want-exit:0|1> <output...>
	local name=$1 want=$2; shift 2
	local out rc
	out=$(DUCKOS_RELEASE_VERSION="$RELEASE" bash "$VERSION_SCRIPT" "$COMMIT" "$DIRTY" 2>&1) && rc=0 || rc=$?
	if [[ "$rc" != "$want" ]]; then
		printf 'FAIL %s: exit %s, expected %s (%s)\n' "$name" "$rc" "$want" "$out"
		fails=$((fails+1))
	elif [[ "$want" == 0 && "$out" != "$EXPECT_OK" ]]; then
		printf 'FAIL %s: output %q, expected %q\n' "$name" "$out" "$EXPECT_OK"
		fails=$((fails+1))
	else
		printf 'ok %s\n' "$name"
	fi
}

# Development identity preserved.
RELEASE= DIRTY=false EXPECT_OK="1.0.0-dev.${COMMIT:0:12}" expect 'dev clean' 0
RELEASE= DIRTY=true EXPECT_OK="1.0.0-dev.${COMMIT:0:12}.dirty" expect 'dev dirty' 0

# Release identity accepted (tag exists in this repo? beta tags may not;
# create a throwaway tag pointing at HEAD, then remove it).
TAG=v0.0.0-versionselftest
git -C "$ROOT" tag -f "$TAG" "$COMMIT" >/dev/null
RELEASE=$TAG DIRTY=false EXPECT_OK="$TAG" expect 'release tag at HEAD' 0

# Mismatched tag/source rejected.
git -C "$ROOT" tag -f "$TAG" "$COMMIT^" >/dev/null 2>&1 || true
RELEASE=$TAG DIRTY=false EXPECT_OK= expect 'tag not at HEAD rejected' 1

# Unknown tag rejected.
RELEASE=v9.9.9-nonexistent DIRTY=false EXPECT_OK= expect 'missing tag rejected' 1

# Dirty tree rejected for release builds.
RELEASE=$TAG DIRTY=true EXPECT_OK= expect 'dirty release rejected' 1

# Malformed version rejected.
RELEASE=0.1.0-beta.3 DIRTY=false EXPECT_OK= expect 'missing v prefix rejected' 1
RELEASE='v0.1.0-beta.3; rm -rf /' DIRTY=false EXPECT_OK= expect 'injection rejected' 1
for RELEASE in v01.1.0 v1.01.0 v1.0.01 v1.0.0-beta..3 v1.0.0-beta. v1.0.0-01; do
	DIRTY=false EXPECT_OK= expect "invalid semver $RELEASE" 1
done

if [[ "$fails" -ne 0 ]]; then printf '%s version test(s) failed\n' "$fails" >&2; exit 1; fi
printf 'version identity tests passed\n'
