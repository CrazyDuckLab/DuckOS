#!/usr/bin/env bash
# xray_install.test.sh — focused tests for install_xray_backend.
#
# The helper is extracted from scripts/install.sh with awk so the
# test always exercises the live source, not a snapshot. The
# tests run inside a fake release directory so no real /usr/local
# is touched.
set -eo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
INSTALL=$ROOT/scripts/install.sh
[[ -r "$INSTALL" ]] || { printf 'install.sh not readable\n' >&2; exit 1; }

# Extract the helper into a sourced bash file. awk is used so the
# function lives as the install.sh author wrote it; if the source
# changes shape the test will catch a missing helper loudly.
HELPER=$(awk '
  /^install_xray_backend\(\)/ { in_fn=1 }
  in_fn { print }
  in_fn && /^}$/ { exit }
' "$INSTALL")
[[ -n "$HELPER" ]] || { printf 'install_xray_backend helper not found in install.sh\n' >&2; exit 1; }

# shellcheck disable=SC1090
eval "$HELPER" >/dev/null

WORK=$(mktemp -d "${TMPDIR:-/tmp}/.xray_test.XXXXXX")
trap 'rm -rf -- "$WORK"' EXIT
mkdir -p "$WORK/xray-core"
echo "fake-xray-amd64-binary" > "$WORK/xray-core/Xray-linux-64"
echo "fake-xray-armv7-binary" > "$WORK/xray-core/Xray-linux-arm32-v6"
echo "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae  Xray-linux-64" > "$WORK/xray-core/XRAY_CORE_SHA256SUMS"
echo "33bc2686a77f6fe438616982b347f852eb7f9572dbe97adc1459f8fb639aea13  Xray-linux-arm32-v6" >> "$WORK/xray-core/XRAY_CORE_SHA256SUMS"

PASS=0
FAIL=0
assert_pass() { PASS=$((PASS+1)); printf 'PASS: %s\n' "$1"; }
assert_fail() { FAIL=$((FAIL+1)); printf 'FAIL: %s\n' "$1"; }

# Stub dpkg on PATH so we can flip arch per case.
DPDG=$WORK/dpkg
cat > "$DPDG" <<'STUB'
#!/usr/bin/env bash
if [[ "$1" == "--print-architecture" ]]; then
  if [[ -n "${DUCKOS_TEST_ARCH:-}" ]]; then
    printf '%s\n' "$DUCKOS_TEST_ARCH"
    exit 0
  fi
fi
exit 1
STUB
chmod 0755 "$DPDG"
export PATH="$WORK:$PATH"

# case 1: missing release dir
out=$(install_xray_backend 2>&1 || true)
if [[ "$out" == *"requires the release directory"* ]]; then
  assert_pass "missing-arg-refused"
else
  assert_fail "missing-arg-refused (got: $out)"
fi

# case 2: arm64 refuses cleanly
DIST64=$WORK/duckos-linux-arm64
mkdir -p "$DIST64/xray-core"
export DUCKOS_TEST_ARCH=arm64
out=$(install_xray_backend "$DIST64" 2>&1 || true)
if [[ "$out" == *"upstream does not ship an arm64 Linux binary"* ]]; then
  assert_pass "arm64-refused-loudly"
else
  assert_fail "arm64-refused-loudly (got: $out)"
fi
unset DUCKOS_TEST_ARCH

# case 3: missing xray-core dir
DIST_EMPTY=$WORK/duckos-linux-amd64-empty
mkdir -p "$DIST_EMPTY"
export DUCKOS_TEST_ARCH=amd64
out=$(install_xray_backend "$DIST_EMPTY" 2>&1 || true)
if [[ "$out" == *"vendored xray binary missing"* ]]; then
  assert_pass "missing-binary-refused"
else
  assert_fail "missing-binary-refused (got: $out)"
fi

# case 4: sha256 mismatch
DIST_BAD=$WORK/duckos-linux-amd64-bad
mkdir -p "$DIST_BAD/xray-core"
echo "TAMPERED binary" > "$DIST_BAD/xray-core/Xray-linux-64"
cp "$WORK/xray-core/XRAY_CORE_SHA256SUMS" "$DIST_BAD/xray-core/XRAY_CORE_SHA256SUMS"
out=$(install_xray_backend "$DIST_BAD" 2>&1 || true)
if [[ "$out" == *"SHA256 mismatch"* ]]; then
  assert_pass "sha-mismatch-refused"
else
  assert_fail "sha-mismatch-refused (got: $out)"
fi

# case 5: manifest does not list the binary
# To exercise the manifest check, we must (a) have a binary whose
# sha matches the pinned list (otherwise the helper fails at the
# sha check first) AND (b) the manifest itself must not list
# that binary. We extract a copy of the helper with the pinned
# list patched to match the binary we stage here.
BIN_CONTENT=$'a-stub-xray-binary-31-bytes-long'
DIST_NOMANIFEST=$WORK/duckos-linux-amd64-nomanifest
mkdir -p "$DIST_NOMANIFEST/xray-core"
printf '%s' "$BIN_CONTENT" > "$DIST_NOMANIFEST/xray-core/Xray-linux-64"
# Manifest exists but does not list THIS binary.
EXPECTED_SHA=$(sha256sum < "$DIST_NOMANIFEST/xray-core/Xray-linux-64" | awk '{print $1}')
echo "${EXPECTED_SHA}  Xray-linux-not-this-one" > "$DIST_NOMANIFEST/xray-core/XRAY_CORE_SHA256SUMS"

# Re-extract the helper with a single patched pinned entry so
# the sha check passes and we reach the manifest check.
HELPER_PATCHED=$(awk '
  /^install_xray_backend\(\)/ { in_fn=1 }
  in_fn { print }
  in_fn && /^}$/ { exit }
' "$INSTALL")
HELPER_PATCHED=$(printf '%s\n' "$HELPER_PATCHED" | sed "s|amd64:Xray-linux-64:23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae|amd64:Xray-linux-64:${EXPECTED_SHA}|")
eval "$HELPER_PATCHED" >/dev/null
out=$(install_xray_backend "$DIST_NOMANIFEST" 2>&1 || true)
if [[ "$out" == *"SHA256SUMS does not list"* ]]; then
  assert_pass "manifest-missing-refused"
else
  assert_fail "manifest-missing-refused (got: $out)"
fi

unset DUCKOS_TEST_ARCH
unset PATH

printf '\nSummary: %d pass, %d fail\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1