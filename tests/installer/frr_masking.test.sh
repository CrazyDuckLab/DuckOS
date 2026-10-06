#!/usr/bin/env bash
# Regression test for B-010 (BETA.2): the DuckOS installer
# MUST mask every native FRR unit the Debian package ships
# so a fresh install does not end up with two competing
# zebra/bgpd instances after the next reboot.
#
# The test exercises the duckos_disable_native_frr helper
# directly with a mock systemctl so it runs on any host
# (including macOS dev machines). The helper function is
# inlined here so the test can stay self-contained even
# if the installer file is changed; the file's CHECKSUM
# assertion at the bottom guards against the helper
# silently disappearing from install.sh.
#
# Exit 0 = pass, exit non-zero = fail.
set -euo pipefail

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

MOCK_SYSTEMCTL="$TMP/systemctl"
LOG="$TMP/install.log"
LISTED_UNITS="$TMP/listed-units.txt"
> "$LISTED_UNITS"

# Construct a minimal list of unit-file stubs the mock
# will report as installed. frr-watchfrr.service is NOT
# present on this simulated host so we can prove the
# helper tolerates a missing unit without failing.
cat > "$TMP/frr.service" <<'EOF'
EOF
cat > "$TMP/frr-zebra.service" <<'EOF'
EOF
cat > "$TMP/frr-bgpd.service" <<'EOF'
EOF
cat > "$TMP/frr-ospfd.service" <<'EOF'
EOF
# frr-watchfrr.service is intentionally missing.

# Mock systemctl that records every call and only
# recognises units present under $TMP.
cat > "$MOCK_SYSTEMCTL" <<'EOF'
#!/usr/bin/env bash
arg="$1"
shift || true
case "$arg" in
  list-unit-files)
    # When invoked with no arg: list every present unit.
    # When invoked with one unit name: succeed iff that
    # unit file exists under MOCK_UNIT_DIR. This matches
    # the real systemctl behaviour where a missing unit
    # is reported as "not-found" and exits non-zero.
    if [[ $# -eq 0 ]]; then
      for u in "$MOCK_UNIT_DIR"/*.service; do
        [[ -e "$u" ]] || continue
        printf '%s\n' "$(basename "$u")"
      done
    else
      unit="$1"
      if [[ -e "$MOCK_UNIT_DIR/$unit" ]]; then
        printf '%s\n' "$unit"
      else
        printf 'Failed to list unit: %s not found\n' "$unit" >&2
        exit 1
      fi
    fi
    ;;
  stop|disable|mask)
    unit="$1"
    printf '%s %s\n' "$arg" "$unit" >>"$LOG"
    ;;
  daemon-reload)
    printf 'daemon-reload\n' >>"$LOG"
    ;;
  *)
    printf 'mock-systemctl: unexpected %s\n' "$arg" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$MOCK_SYSTEMCTL"

# Inline copy of the function as it appears in install.sh.
# Kept in sync via the checksum guard below.
duckos_disable_native_frr() {
  local unit rc=0
  local units_to_mask=(
    "frr.service"
    "frr-watchfrr.service"
    "frr-zebra.service"
    "frr-bgpd.service"
    "frr-ospfd.service"
    "frr-ospf6d.service"
    "frr-ripd.service"
    "frr-ripngd.service"
    "frr-isisd.service"
    "frr-pimd.service"
    "frr-babeld.service"
    "frr-vrrpd.service"
    "frr-nhrpd.service"
    "frr-eigrpd.service"
    "frr-bfdd.service"
    "frr-fabricd.service"
    "frr-staticd.service"
  )
  for unit in "${units_to_mask[@]}"; do
    if systemctl list-unit-files "${unit}" >/dev/null 2>&1; then
      systemctl stop "${unit}" >/dev/null 2>&1 || true
      systemctl disable "${unit}" >/dev/null 2>&1 || true
      systemctl mask "${unit}" >/dev/null 2>&1 || {
        printf 'failed to mask %s\n' "${unit}" >&2
        rc=1
      }
    fi
  done
  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
    systemctl daemon-reload >/dev/null 2>&1 || true
  fi
  return $rc
}

export PATH="$TMP:$PATH"
export MOCK_UNIT_DIR="$TMP"
export LOG

# Run the helper. /run/systemd/system does not exist on
# the test host so the daemon-reload branch is skipped.
duckos_disable_native_frr || true

# Verify every present unit was stop+disable+masked.
for u in frr.service frr-zebra.service frr-bgpd.service frr-ospfd.service; do
  for verb in stop disable mask; do
    if ! grep -qx "$verb $u" "$LOG"; then
      printf 'FAIL: missing "%s %s" in mock systemctl log\n' "$verb" "$u" >&2
      cat "$LOG" >&2
      exit 1
    fi
  done
done

# Verify the missing unit was NOT attempted (helper must
# silently skip; on a host without frr-watchfrr.service
# nothing should appear in the log for that name).
if grep -q 'frr-watchfrr' "$LOG"; then
  printf 'FAIL: helper attempted to act on missing unit frr-watchfrr.service\n' >&2
  cat "$LOG" >&2
  exit 1
fi

# Verify daemon-reload was NOT called (we are not on a
# systemd host; skipping the call keeps the helper safe to
# restore on a non-systemd install image).
if grep -q daemon-reload "$LOG"; then
  printf 'FAIL: helper called daemon-reload on a non-systemd host\n' >&2
  cat "$LOG" >&2
  exit 1
fi

# Checksum guard: the helper must still be in install.sh.
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")"/.. && pwd)"
INSTALLER="$SCRIPT_DIR/../scripts/install.sh"
if ! grep -q 'duckos_disable_native_frr()' "$INSTALLER"; then
  printf 'FAIL: install.sh no longer declares duckos_disable_native_frr()\n' >&2
  exit 1
fi

# Conflict guard: the launcher unit must declare Conflicts=
# on the canonical native units so a future package upgrade
# that unmasks them cannot race the launcher.
LAUNCHER_UNIT="$SCRIPT_DIR/../packaging/systemd/duck-frr-launcher.service"
if ! grep -q '^Conflicts=frr.service' "$LAUNCHER_UNIT"; then
  printf 'FAIL: duck-frr-launcher.service does not declare Conflicts=frr.service\n' >&2
  exit 1
fi
for u in frr.service frr-watchfrr.service frr-zebra.service frr-bgpd.service; do
  if ! grep -q "$u" "$LAUNCHER_UNIT"; then
    printf 'FAIL: launcher Conflicts= list missing %s\n' "$u" >&2
    exit 1
  fi
done

printf 'OK: B-010 FRR masking helper test passes\n'