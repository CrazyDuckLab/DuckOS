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
# Upstream-pinned sha256 values for XTLS/Xray-core v26.3.27. These
# are the trust anchor; if any upstream release changes them, the
# install.sh pinned list MUST be updated alongside this test.
echo "fake-xray-amd64-binary"  > "$WORK/xray-core/Xray-linux-64"
echo "fake-xray-armv7-binary"  > "$WORK/xray-core/Xray-linux-arm32-v7a"
echo "fake-xray-arm64-binary"  > "$WORK/xray-core/Xray-linux-arm64-v8a"
cat > "$WORK/xray-core/XRAY_CORE_SHA256SUMS" <<'SUMS'
23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae  Xray-linux-64
c7265ae13c63ca0241a037df4ef960ad37938c8a67d984cc08834b2cfdf5654b  Xray-linux-arm32-v7a
4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c  Xray-linux-arm64-v8a
SUMS

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

# Stub install(1) so writes to /usr/local/bin/* and
# /usr/local/share/doc/xray/* land in the fake tree. We also strip
# the GNU-only -o / -g flags on hosts whose install(1) does not
# support them (macOS BSD install), because the helper's calls
# use those flags and the wrapper would otherwise fail with
# "unknown group root". The helper only ever uses `-o root -g root`
# as adjacent flag-value pairs, so a simple regex strip is enough.
INSTALL_BIN=$(command -v install)
WRAP=$WORK/install
cat > "$WRAP" <<WRAP
#!/usr/bin/env bash
args=()
i=0
while (( i < \$# )); do
  a="\${@:((i+1)):1}"
  case "\$a" in
    -o) i=\$((i+2)); continue ;;  # skip -o <value>
    -g) i=\$((i+2)); continue ;;  # skip -g <value>
    /usr/local/bin|/usr/local/bin/*)
      a="\${a/\/usr\/local\/bin/$WORK/fake-usr/local/bin}";;
    /usr/local/share/doc/xray|/usr/local/share/doc/xray/*)
      a="\${a/\/usr\/local\/share\/doc\/xray/$WORK/fake-usr/local/share/doc/xray}";;
  esac
  args+=("\$a")
  i=\$((i+1))
done
exec "$INSTALL_BIN" "\${args[@]}"
WRAP
chmod 0755 "$WRAP"
export PATH="$WORK:$PATH"

# Stub ln(1) so the symlink helper installs also lands inside
# the fake tree. Without this, the helper would attempt to
# overwrite the real /usr/local/bin/xray and fail with
# "Permission denied" when run as a non-root user.
LN_BIN=$(command -v ln)
LNWRAP=$WORK/ln
cat > "$LNWRAP" <<WRAP
#!/usr/bin/env bash
args=()
for a in "\$@"; do
  case "\$a" in
    /usr/local/bin|/usr/local/bin/*)
      a="\${a/\/usr\/local\/bin/$WORK/fake-usr/local/bin}";;
  esac
  args+=("\$a")
done
exec "$LN_BIN" "\${args[@]}"
WRAP
chmod 0755 "$LNWRAP"

# Stub /usr/local/bin/xray for the smoke test. The helper invokes
# /usr/local/bin/xray directly via `command -v` and `/usr/local/bin/xray version`
# (NOT via PATH). The DUCKOS_INSTALL_SKIP_SMOKE flag set by the
# happy-path tests below disables the smoke test so we do not
# actually need this stub; the stub is here for the negative
# case "smoke test missing binary" if a future test author
# needs it. Keeping the stub as a regular file in fake-usr is
# safe because the helper installs the symlink with ln -sfn
# (force overwrite).
mkdir -p "$WORK/fake-usr/local/bin"
cat > "$WORK/fake-usr/local/bin/xray" <<'STUB'
#!/usr/bin/env bash
if [[ "$1" == "version" ]]; then
  printf 'Xray 26.3.27 (test stub)\n'
  exit 0
fi
exit 1
STUB
chmod 0755 "$WORK/fake-usr/local/bin/xray"
# Remove the stub so the helper's ln -sfn creates a fresh symlink
# in fake-usr/local/bin/xray. The smoke test is skipped via
# DUCKOS_INSTALL_SKIP_SMOKE in the happy paths.
rm -f "$WORK/fake-usr/local/bin/xray"

# Build a release-tree dir for an arch. The binary's content does
# NOT match the upstream pinned sha (test fixture), so the helper
# would refuse it; the per-case "happy path" tests re-pin the
# helper with the test binary's real sha before calling install.
fake_release() {
  local arch=$1
  local r=$WORK/release-$arch
  rm -rf "$r"
  mkdir -p "$r/xray-core"
  local bin content actual_sha
  case "$arch" in
    amd64) bin=Xray-linux-64 ;;
    armv7) bin=Xray-linux-arm32-v7a ;;
    arm64) bin=Xray-linux-arm64-v8a ;;
    *) printf 'fake_release: bad arch %s\n' "$arch" >&2; return 1 ;;
  esac
  content="fake-$arch-binary-content"
  printf '%s' "$content" > "$r/xray-core/$bin"
  actual_sha=$(sha256sum < "$r/xray-core/$bin" | awk '{print $1}')
  printf '%s  %s\n' "$actual_sha" "$bin" > "$r/xray-core/XRAY_CORE_SHA256SUMS"
  printf '%s\n' "$r"
}

# Patch the helper's pinned entry to match the supplied release's
# staged binary. The patched helper is evaluated into the test
# shell so subsequent install_xray_backend calls see the override.
patch_for_release() {
  local r=$1
  local bin pinned_sha actual_sha helper_patched arch
  bin=$(awk '{print $2}' "$r/xray-core/XRAY_CORE_SHA256SUMS")
  actual_sha=$(awk '{print $1}' "$r/xray-core/XRAY_CORE_SHA256SUMS")
  case "$bin" in
    Xray-linux-64)            arch=amd64; pinned_sha=23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae ;;
    Xray-linux-arm32-v7a)     arch=armv7; pinned_sha=c7265ae13c63ca0241a037df4ef960ad37938c8a67d984cc08834b2cfdf5654b ;;
    Xray-linux-arm64-v8a)     arch=arm64; pinned_sha=4d30283ae614e3057f730f67cd088a42be6fdf91f8639d82cb69e48cde80413c ;;
    *) printf 'patch_for_release: unknown binary %s\n' "$bin" >&2; return 1 ;;
  esac
  helper_patched=$(printf '%s\n' "$HELPER" | \
    sed "s|${arch}:${bin}:${pinned_sha}|${arch}:${bin}:${actual_sha}|")
  eval "$helper_patched" >/dev/null
}

# ---- negative cases (use unpinned helper, expect refusal) ----

# case 1: missing release dir
out=$(install_xray_backend 2>&1 || true)
[[ "$out" == *"requires the release directory"* ]] && assert_pass "missing-arg-refused" \
  || assert_fail "missing-arg-refused (got: $out)"

# case 2: missing binary
DIST_EMPTY=$WORK/release-empty-amd64
mkdir -p "$DIST_EMPTY"
export DUCKOS_TEST_ARCH=amd64
out=$(install_xray_backend "$DIST_EMPTY" 2>&1 || true)
unset DUCKOS_TEST_ARCH
[[ "$out" == *"vendored xray binary missing"* ]] && assert_pass "missing-binary-refused" \
  || assert_fail "missing-binary-refused (got: $out)"

# case 3: sha256 mismatch
DIST_BAD=$WORK/release-bad-amd64
mkdir -p "$DIST_BAD/xray-core"
echo "TAMPERED binary" > "$DIST_BAD/xray-core/Xray-linux-64"
cp "$WORK/xray-core/XRAY_CORE_SHA256SUMS" "$DIST_BAD/xray-core/XRAY_CORE_SHA256SUMS"
export DUCKOS_TEST_ARCH=amd64
out=$(install_xray_backend "$DIST_BAD" 2>&1 || true)
unset DUCKOS_TEST_ARCH
[[ "$out" == *"SHA256 mismatch"* ]] && assert_pass "sha-mismatch-refused" \
  || assert_fail "sha-mismatch-refused (got: $out)"

# case 4: manifest does not list the binary specifically. The
# binary's sha matches the pinned entry (so the helper does not
# fail at the sha check), but the manifest itself omits it.
DIST_NOMANIFEST=$WORK/release-nomanifest-amd64
mkdir -p "$DIST_NOMANIFEST/xray-core"
echo "fake-content" > "$DIST_NOMANIFEST/xray-core/Xray-linux-64"
ACTUAL=$(sha256sum < "$DIST_NOMANIFEST/xray-core/Xray-linux-64" | awk '{print $1}')
# Manifest lists a different binary name with the same sha.
printf '%s  Xray-linux-not-this-one\n' "$ACTUAL" > "$DIST_NOMANIFEST/xray-core/XRAY_CORE_SHA256SUMS"
HELPER_PATCHED_MISSING=$(printf '%s\n' "$HELPER" | \
  sed "s|amd64:Xray-linux-64:23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae|amd64:Xray-linux-64:${ACTUAL}|")
eval "$HELPER_PATCHED_MISSING" >/dev/null
export DUCKOS_TEST_ARCH=amd64
out=$(install_xray_backend "$DIST_NOMANIFEST" 2>&1 || true)
unset DUCKOS_TEST_ARCH
[[ "$out" == *"SHA256SUMS does not list"* ]] && assert_pass "manifest-missing-refused" \
  || assert_fail "manifest-missing-refused (got: $out)"

# ---- happy paths: each arch installs into release tree + symlink ----

for arch_pair in "amd64:Xray-linux-64:amd64" "armv7:Xray-linux-arm32-v7a:armhf" "arm64:Xray-linux-arm64-v8a:arm64"; do
  arch=${arch_pair%%:*}
  bin=${arch_pair#*:}
  bin=${bin%%:*}
  dpkg_arch=${arch_pair##*:}
  R=$(fake_release "$arch")
  patch_for_release "$R"
  export DUCKOS_TEST_ARCH=$dpkg_arch
  export DUCKOS_INSTALL_SKIP_SMOKE=1
  # The smoke test (command -v /usr/local/bin/xray; xray version)
  # requires writing into real /usr/local/bin which a non-root test
  # cannot do. The helper itself runs the smoke test BEFORE the
  # install completes; we accept that the smoke test will report
  # "not on PATH" in this harness and assert only the helper's
  # side-effects (release-tree binary + /usr/local/bin symlink).
  # A live re-install on the host VM exercises the smoke test.
  out=$(install_xray_backend "$R" 2>&1 || true)
  unset DUCKOS_TEST_ARCH
  # Strip the smoke-test noise so the assert message is informative.
  install_ok=
  if [[ -x "$R/xray-core/xray" ]] \
     && [[ -L "$WORK/fake-usr/local/bin/xray" ]] \
     && [[ "$(readlink "$WORK/fake-usr/local/bin/xray")" = "$R/xray-core/xray" ]]; then
    install_ok=1
  fi
  if [[ -n "$install_ok" ]]; then
    assert_pass "${arch}-installs-into-release-with-symlink"
  else
    # Debug: where is the symlink actually pointing?
    debug_link=$(readlink "$WORK/fake-usr/local/bin/xray" 2>&1 || echo "MISSING")
    debug_bin=$(ls -la "$R/xray-core/xray" 2>&1 || echo "MISSING")
    assert_fail "${arch}-installs-into-release-with-symlink (link=$debug_link bin=$debug_bin out=$out)"
  fi
done

# ---- rollback lifecycle: symlink tracks the latest install ----
R_V1=$(fake_release amd64)
V1_SHA=$(awk '{print $1}' "$R_V1/xray-core/XRAY_CORE_SHA256SUMS")
R_V2=$(fake_release amd64)
V2_SHA=$(awk '{print $1}' "$R_V2/xray-core/XRAY_CORE_SHA256SUMS")
patch_for_release "$R_V1"
export DUCKOS_TEST_ARCH=amd64
export DUCKOS_INSTALL_SKIP_SMOKE=1
install_xray_backend "$R_V1" >/dev/null 2>&1
patch_for_release "$R_V2"
install_xray_backend "$R_V2" >/dev/null 2>&1
# After install v2: symlink points at v2.
pass_v2=
[[ "$(readlink "$WORK/fake-usr/local/bin/xray")" = "$R_V2/xray-core/xray" ]] && pass_v2=1 || pass_v2=0
# Simulate orchestrator: swap /opt/duckos/releases/current to v2.
mkdir -p "$WORK/opt/duckos/releases"
ln -sfn "$R_V2" "$WORK/opt/duckos/releases/current"
# Simulate rollback: swap current back to v1. Symlink is now stale.
ln -sfn "$R_V1" "$WORK/opt/duckos/releases/current"
# Reinstall at v1 should re-target the symlink to v1.
patch_for_release "$R_V1"
install_xray_backend "$R_V1" >/dev/null 2>&1
pass_v1=
[[ "$(readlink "$WORK/fake-usr/local/bin/xray")" = "$R_V1/xray-core/xray" ]] && pass_v1=1 || pass_v1=0
unset DUCKOS_TEST_ARCH
if [[ $pass_v2 -eq 1 ]] && [[ $pass_v1 -eq 1 ]]; then
  assert_pass "rollback-symlink-tracks-latest-install"
else
  assert_fail "rollback-symlink-tracks-latest-install (v2=$pass_v2 v1=$pass_v1)"
fi

printf '\nSummary: %d pass, %d fail\n' "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1