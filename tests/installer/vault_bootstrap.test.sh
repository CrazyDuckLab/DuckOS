#!/usr/bin/env bash
# Tests for the protected vault master-key bootstrap used by scripts/install.sh.
# We source the function out of install.sh and exercise it in a temp dir under
# a controlled identity. These tests do NOT require root; they assert the
# function's contract: idempotent generation, refuses to fabricate a key when
# records exist, file mode and content shape, and refusal on bogus inputs.
set -euo pipefail

# Extract the bootstrap_vault_master_key function out of install.sh so we can
# invoke it directly. The function is defined inline at the top of the script.
INSTALL_SH="${INSTALL_SH:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/install.sh}"
[[ -r "$INSTALL_SH" ]] || { echo "FAIL: cannot read $INSTALL_SH" >&2; exit 1; }

extract_function() {
  awk '/^bootstrap_vault_master_key\(\) \{$/,/^\}/' "$INSTALL_SH"
}

# Pull the function definition plus a tiny shim so we can source it standalone.
eval "$(extract_function)"

# Provide minimal stand-ins for commands install.sh expects.
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
command -v install >/dev/null 2>&1 || {
  # BusyBox / minimal envs may not have GNU install. Provide a local stub that
  # honours `-o -g -m` and a destination path.
  install() {
    local mode="" owner="" group="" src="" dst=""
    while [[ $# -gt 0 ]]; do
      case "$1" in
        -m) mode="$2"; shift 2 ;;
        -o) owner="$2"; shift 2 ;;
        -g) group="$2"; shift 2 ;;
        -*) shift ;;
        *)  [[ -z "$src" ]] && src="$1" || dst="$1"; shift ;;
      esac
    done
    : "${src:=/dev/null}"
    [[ -n "$mode" ]] && chmod "$mode" "$dst"
    [[ -n "$owner$group" ]] && chown "${owner}:${group}" "$dst" 2>/dev/null || true
    [[ "$src" != "/dev/null" ]] && cp "$src" "$dst"
  }
  export -f install
}

# Use a unique tmp dir for each run.
WORK=$(mktemp -d -t duckos-vault-bootstrap.XXXXXXXX)
trap 'rm -rf "$WORK"' EXIT
chmod 700 "$WORK"

run_test_fresh_install_generates_key() {
  local master="$WORK/vault/master.key"
  local records="$WORK/vault/records"
  mkdir -p "$records"
  bootstrap_vault_master_key "$master" "$records" || fail "fresh install bootstrap failed"
  [[ -f "$master" ]] || fail "master key file not created"
  local mode
  # stat(1) syntax differs across BSD and GNU; use a portable octal decode.
  if stat -c '%a' "$master" >/dev/null 2>&1; then
    mode=$(stat -c '%a' "$master")
  else
    mode=$(stat -f '%Lp' "$master")
  fi
  [[ "$mode" == "600" ]] || fail "master key mode is $mode, expected 600"
  local key
  key=$(cat "$master")
  [[ ${#key} -eq 64 ]] || fail "master key length is ${#key}, expected 64 hex chars"
  [[ "$key" =~ ^[0-9a-f]{64}$ ]] || fail "master key is not lowercase hex"
  # Distinct invocations must yield distinct keys.
  local first=$key
  rm -f "$master"
  bootstrap_vault_master_key "$master" "$records" || fail "second bootstrap failed"
  local second=$(cat "$master")
  [[ "$first" != "$second" ]] || fail "two fresh bootstraps produced identical keys (urandom not working)"
}

run_test_existing_key_preserved() {
  local master="$WORK/vault/master.key"
  local records="$WORK/vault/records"
  mkdir -p "$records"
  bootstrap_vault_master_key "$master" "$records" || fail "first bootstrap failed"
  local first=$(cat "$master")
  # Pretend time passes. Re-running bootstrap must NOT overwrite the existing
  # master (doing so would orphan every secret that was encrypted with it).
  bootstrap_vault_master_key "$master" "$records" || fail "second bootstrap failed"
  local second=$(cat "$master")
  [[ "$first" == "$second" ]] || fail "second bootstrap overwrote existing master key"
}

run_test_records_without_master_refuses() {
  local master="$WORK/vault2/master.key"
  local records="$WORK/vault2/records"
  mkdir -p "$records"
  printf 'pretend-secret-record\n' > "$records/x.json"
  # The function must refuse rather than fabricate a new key, because that
  # would orphan the existing records.
  if bootstrap_vault_master_key "$master" "$records"; then
    fail "bootstrap accepted a missing-master + existing-records state"
  fi
  [[ ! -e "$master" ]] || fail "bootstrap fabricated a master key despite existing records"
}

run_test_rejects_empty_arguments() {
  local master="$WORK/vault3/master.key"
  local records="$WORK/vault3/records"
  mkdir -p "$records"
  if bootstrap_vault_master_key "" "$records"; then fail "empty master path accepted"; fi
  if bootstrap_vault_master_key "$master" ""; then fail "empty records dir accepted"; fi
}

run_test_fresh_install_generates_key
run_test_existing_key_preserved
run_test_records_without_master_refuses
run_test_rejects_empty_arguments

echo "vault bootstrap tests PASS"
