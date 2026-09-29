#!/usr/bin/env bash
# DV04-WG — WireGuard runtime lifecycle and protected
# artifacts. S subcase (denied_export_zero_effect) is
# exercised by the wireguard package tests. The three L
# subcases need real kernel wg on Linux.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV04-WG"
start_dv

if run_go_test "./internal/wireguard/..."; then
    record_s "denied_export_zero_effect" "PASS" "wireguard package tests green; export permission test asserts zero key-resolver calls when denied"
else
    record_s "denied_export_zero_effect" "FAIL" "wireguard package test failed; see test-__internal_wireguard____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV04WG"; then
        record_l "crud_kernel_runtime" "PASS" "tests/layer_l dv04wg_l_test.go green: real kernel WireGuard interface CRUD via wg(8); no resurrection after restart"
        record_l "restart_no_resurrection" "PASS" "tests/layer_l dv04wg_l_test.go green: catalog remove + reopen does not resurrect deleted peer"
        record_l "export_redacted_matches_qr" "PASS" "tests/layer_l dv04wg_l_test.go green: redacted peer export contains only public fields; no PrivateKey line, no REDACTED sentinel"
    else
        for s in crud_kernel_runtime restart_no_resurrection export_redacted_matches_qr; do
            record_l "$s" "FAIL" "tests/layer_l dv04wg_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV04WG_.log"
        done
    fi
else
    for s in crud_kernel_runtime restart_no_resurrection export_redacted_matches_qr; do
        record_l "$s" "NOT_VERIFIED" "kernel wg fixture required; runner is $OSTYPE"
    done
fi

end_dv
