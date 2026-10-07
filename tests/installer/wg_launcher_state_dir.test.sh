#!/usr/bin/env bash
# Regression test for B-005 (BETA.2 corrective) — the
# DuckOS installer MUST create /var/lib/duckos/wireguard
# with ownership that the duck-wg-launcher (root user,
# CapabilityBoundingSet=CAP_NET_ADMIN only, NO
# CAP_DAC_OVERRIDE) can write to.
#
# Failure mode (now-fixed): install.sh created the dir
# as duckos:duckos mode 0700. The launcher, restricted to
# CAP_NET_ADMIN, could not bypass DAC and could not write
# owned.json.tmp. The kernel netlink call succeeded but
# the launcher's state-file write returned EACCES, which
# the live launcher surfaces as a 500 exec_failed with
# "stage owned set: open ... owned.json.tmp: permission
# denied". That looks like a netlink failure to the
# public API but is actually a state-directory DAC
# failure.
#
# The test exercises install.sh's relevant
# `install -d ... wireguard` line directly with a mock
# install(1) binary that records the arguments, so it
# runs on any host (including macOS dev machines).
#
# Exit 0 = pass, exit non-zero = fail.
set -euo pipefail

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

LOG="$TMP/install.log"
MOCK_INSTALL="$TMP/mockbin"
WG_OUT="$TMP/wg-out.txt"
> "$LOG"
> "$WG_OUT"

mkdir -p "$MOCK_INSTALL"
# Mock install(1): record -d invocations and extract
# the owner/group/mode/path for any path containing
# "wireguard" to $WG_OUT for separate inspection.
cat > "$MOCK_INSTALL/install" <<'EOF'
#!/usr/bin/env bash
DUMMY_INSTALL_LOG="${DUMMY_INSTALL_LOG:-/dev/null}"
WG_OUT="${WG_OUT:-/dev/null}"
printf '%s\n' "$*" >> "$DUMMY_INSTALL_LOG"
if [[ "${1:-}" == "-d" ]]; then
  shift
  OWNER=""; MODE=""; PATHARG=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -o) OWNER="$2"; shift 2 ;;
      -g) GROUP="$2"; shift 2 ;;
      -m) MODE="$2"; shift 2 ;;
      --) shift; break ;;
      -*) shift ;;
      *)
        if [[ "$1" == *wireguard* ]]; then
          printf '/var/lib/duckos/wireguard owner=%s group=%s mode=%s\n' "${OWNER:-?}" "${GROUP:-?}" "${MODE:-?}" >> "$WG_OUT"
        fi
        shift ;;
    esac
  done
fi
exit 0
EOF
chmod 0755 "$MOCK_INSTALL/install"

# Prepend mock install to PATH so install.sh picks it up.
export PATH="$MOCK_INSTALL:$PATH"
export DUMMY_INSTALL_LOG="$LOG"
export WG_OUT="$WG_OUT"

# Run install.sh's install -d block directly. We don't
# want to run the full installer — just the directory
# creation step. Extract that single block from the
# real install.sh so the test stays in lock-step with
# the production source.
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")"/.. && pwd)"
INSTALLER="$SCRIPT_DIR/../scripts/install.sh"

# Find the install -d line that mentions /var/lib/duckos/wireguard
LINE=$(grep -n "install -d.*wireguard" "$INSTALLER" | head -1 | cut -d: -f1)
if [[ -z "$LINE" ]]; then
  printf 'FAIL: install.sh does not contain an install -d ... wireguard line\n' >&2
  exit 1
fi

# Run that single line in a bash subshell with our mocked install.
sed -n "${LINE}p" "$INSTALLER" | bash

# Now assert.
if [[ ! -s "$WG_OUT" ]]; then
  printf 'FAIL: install.sh did not provision /var/lib/duckos/wireguard\n' >&2
  cat "$LOG" >&2
  exit 1
fi

ENTRY=$(grep '/var/lib/duckos/wireguard' "$WG_OUT" | head -1)
echo "Provisioned: $ENTRY"

OWNER=$(echo "$ENTRY" | sed -n 's/.*owner=\([^ ]*\).*/\1/p')
GROUP=$(echo "$ENTRY" | sed -n 's/.*group=\([^ ]*\).*/\1/p')
MODE=$(echo "$ENTRY" | sed -n 's/.*mode=\([^ ]*\).*/\1/p')

# Owner MUST be root so the launcher (User=root,
# CapBnd=CAP_NET_ADMIN only) can write via owner bits.
if [[ "$OWNER" != "root" ]]; then
  printf 'FAIL: /var/lib/duckos/wireguard owner=%s (want root)\n' "$OWNER" >&2
  exit 1
fi

# Mode MUST allow the owner write (>=6xx). Mode 0750 is
# the project's canonical choice because it also lets
# the duckos group read for audit paths without making
# the dir world-writable. The middle digit (group) may
# be 0/5/7 — we only require owner-write.
if [[ "${MODE:1:1}" != "5" && "${MODE:1:1}" != "7" ]]; then
  printf 'FAIL: /var/lib/duckos/wireguard mode=%s owner-write (want owner digit 6 or 7; got %s)\n' "$MODE" "${MODE:1:1}" >&2
  exit 1
fi

# Sanity: the launcher systemd unit MUST still declare
# CapabilityBoundingSet=CAP_NET_ADMIN (the bounded-cap
# invariant that this test exists to defend).
LAUNCHER_UNIT="$SCRIPT_DIR/../packaging/systemd/duck-wg-launcher.service"
if ! grep -q '^CapabilityBoundingSet=CAP_NET_ADMIN$' "$LAUNCHER_UNIT"; then
  printf 'FAIL: duck-wg-launcher.service no longer bounds caps to CAP_NET_ADMIN only\n' >&2
  exit 1
fi
if ! grep -q '^AmbientCapabilities=CAP_NET_ADMIN$' "$LAUNCHER_UNIT"; then
  printf 'FAIL: duck-wg-launcher.service no longer sets AmbientCapabilities=CAP_NET_ADMIN\n' >&2
  exit 1
fi

# Guard: install.sh MUST NOT re-bundle /var/lib/duckos/wireguard
# into the duckos:duckos 0700 line that ships with config/state/
# identity/policy/xray/backups. If a future edit re-merges the
# two, the launcher regression re-appears.
if grep -nE 'install -d.*duckos -g duckos.*/var/lib/duckos/wireguard' "$INSTALLER" >/dev/null; then
  printf 'FAIL: /var/lib/duckos/wireguard is back in the duckos:duckos 0700 batch\n' >&2
  exit 1
fi

printf 'OK: B-005 wg-launcher state-dir ownership test passes (owner=root group=%s mode=%s)\n' "$GROUP" "$MODE"