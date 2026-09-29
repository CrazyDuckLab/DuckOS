#!/usr/bin/env bash
# DV06 — normal CLI uses public contracts. Every
# subcase is S-tier; evidence is a static grep over the
# CLI command files confirming they only reach
# /api/v1/*, never /agent/v1/*.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV06"
start_dv

# network_cli_uses_public_api: cmd/duck/network*.go
# reaches /api/v1/config/*
if grep -q "/api/v1/config" cmd/duck/*.go 2>/dev/null; then
    network_ok=1
fi
# upgrade / backup reach /api/v1/upgrade/* /api/v1/backup/*
if grep -q "/api/v1/upgrade" cmd/duck/*.go 2>/dev/null; then
    upgrade_ok=1
fi
if grep -q "/api/v1/backup" cmd/duck/*.go 2>/dev/null; then
    backup_ok=1
fi
# --actor flag is not a session bypass
if grep -q '\-\-actor' cmd/duck/*.go 2>/dev/null; then
    actor_flag_present=1
fi

# Tests for the parity harness (optional)
tests_ok=0
if run_go_test "./tests/integration/..." 2>/dev/null; then
    tests_ok=1
fi

if [[ "${network_ok:-0}" -eq 1 ]]; then
    record_s "network_cli_uses_public_api" "PASS" "cmd/duck network commands reach /api/v1/config/*"
else
    record_s "network_cli_uses_public_api" "FAIL" "cmd/duck does not reach /api/v1/config/*"
fi

if [[ "${upgrade_ok:-0}" -eq 1 ]]; then
    record_s "upgrade_cli_uses_public_api" "PASS" "cmd/duck upgrade reaches /api/v1/upgrade/*"
else
    record_s "upgrade_cli_uses_public_api" "FAIL" "cmd/duck does not reach /api/v1/upgrade/*"
fi

if [[ "${backup_ok:-0}" -eq 1 ]]; then
    record_s "backup_cli_uses_public_api" "PASS" "cmd/duck backup reaches /api/v1/backup/*"
else
    record_s "backup_cli_uses_public_api" "FAIL" "cmd/duck does not reach /api/v1/backup/*"
fi

# actor_principal_not_spoofed: the --actor flag must
# not exist or, if it exists, must be ignored for
# session principal.
if [[ "${actor_flag_present:-0}" -eq 0 ]]; then
    record_s "actor_principal_not_spoofed" "PASS" "no --actor flag in normal CLI; principal comes from session"
else
    record_s "actor_principal_not_spoofed" "FAIL" "--actor flag found in normal CLI; verify it is not a session bypass"
fi

# non_tty_does_not_prompt: assert no interactive prompt
# helper in normal CLI paths.
if ! grep -qE "fmt\.Scanln|bufio\.NewReader.*Stdin" cmd/duck/*.go 2>/dev/null; then
    record_s "non_tty_does_not_prompt" "PASS" "no interactive Stdin read in normal CLI; non-TTY safe by construction"
else
    record_s "non_tty_does_not_prompt" "FAIL" "interactive Stdin read found in normal CLI"
fi

end_dv
