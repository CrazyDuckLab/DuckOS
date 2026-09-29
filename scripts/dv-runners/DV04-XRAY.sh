#!/usr/bin/env bash
# DV04-XRAY — Xray runtime lifecycle. S subcase
# (incompatible_protocol_rejected) is exercised by the
# xray package tests. L subcases need a real xray
# daemon on Linux.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV04-XRAY"
start_dv

if run_go_test "./internal/xray/..."; then
    record_s "incompatible_protocol_rejected" "PASS" "xray package tests green; protocol/security validation rejects unsupported combinations before runtime"
else
    record_s "incompatible_protocol_rejected" "FAIL" "xray package test failed; see test-__internal_xray____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV04Xray"; then
        record_l "crud_xray_validation" "PASS" "tests/layer_l dv04xray_l_test.go green: 'xray run -c <cfg> -test' validates VLESS+TCP; malformed config rejected"
        record_l "subscription_qr_matches_url" "PASS" "tests/layer_l dv04xray_l_test.go green: subscription export round-trips with exact issued URL"
        record_l "restart_no_resurrection" "PASS" "tests/layer_l dv04xray_l_test.go green: catalog remove + reopen does not resurrect deleted user"
    else
        for s in crud_xray_validation subscription_qr_matches_url restart_no_resurrection; do
            record_l "$s" "FAIL" "tests/layer_l dv04xray_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV04Xray_.log"
        done
    fi
else
    for s in crud_xray_validation subscription_qr_matches_url restart_no_resurrection; do
        record_l "$s" "NOT_VERIFIED" "real xray daemon fixture required; runner is $OSTYPE"
    done
fi

end_dv
