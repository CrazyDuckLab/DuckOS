#!/usr/bin/env bash
# F6 failure-injection test: the launcher unit bookkeeping must stop,
# disable and roll back ONLY what a failed install created, and must
# restore a pre-existing unit instead of deleting it. systemctl is a
# recording stub; the paths live in a temp root.
set -euo pipefail
cd "$(dirname "$0")/../.."
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
ETC="$TMP/etc/systemd/system"; ROLLBACK="$TMP/rollback"
mkdir -p "$ETC" "$ROLLBACK"
export ETC ROLLBACK

# Stub systemctl records every invocation.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/systemctl" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$SYSTEMCTL_LOG"
exit 0
STUB
chmod 0755 "$TMP/bin/systemctl"
export SYSTEMCTL_LOG="$TMP/systemctl.log"
: > "$SYSTEMCTL_LOG"
export PATH="$TMP/bin:$PATH"
export NFT_UNIT_PATH="$ETC/duck-nft-launcher.service"
export NFT_ROLLBACK_DIR="$ROLLBACK"

# Extract the functions under test from the installer.
eval "$(awk '/^rollback_launcher_unit\(\)/,/^}/' scripts/install.sh)"
nft_rollback_dir=$NFT_ROLLBACK_DIR
nft_unit_path=$NFT_UNIT_PATH

run_cleanup() {
  nft_unit_installed=$1; nft_unit_preexisting=$2
  nft_started=$3; nft_enabled=$4
  rollback_launcher_unit
}

# Case 1: this install created the unit -> remove it, reload daemon.
rm -f "$ETC/duck-nft-launcher.service"
run_cleanup true false true true
grep -q "stop duck-nft-launcher.service" "$SYSTEMCTL_LOG"
grep -q "disable duck-nft-launcher.service" "$SYSTEMCTL_LOG"
grep -q "daemon-reload" "$SYSTEMCTL_LOG"
[[ ! -e "$ETC/duck-nft-launcher.service" ]]

# Case 2: a pre-existing unit was backed up -> restore it, not delete.
: > "$SYSTEMCTL_LOG"
printf '# preexisting\n' > "$ROLLBACK/duck-nft-launcher.service.preinstall"
run_cleanup true true true true
grep -q "# preexisting" "$ETC/duck-nft-launcher.service"
[[ $(grep -c "stop duck-nft-launcher.service" "$SYSTEMCTL_LOG") -eq 1 ]]

# Case 3: nothing was installed -> the cleaner must not touch systemd.
: > "$SYSTEMCTL_LOG"
rm -f "$ETC/duck-nft-launcher.service"
run_cleanup false false true true
[[ ! -s "$SYSTEMCTL_LOG" ]]

echo "nft_launcher_rollback: PASS"
