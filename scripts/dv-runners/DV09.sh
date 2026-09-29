#!/usr/bin/env bash
# DV09 — Anycast software advertisement path. S
# subcase: missing_source_never_ready. L subcases
# need a disposable FRR fixture.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV09"
start_dv

# missing_source_never_ready: anycast package test
# asserts nil source / missing FRR is reported as
# degraded / not-advertised, never Ready=true.
if run_go_test "./internal/anycast/..."; then
    record_s "missing_source_never_ready" "PASS" "anycast package tests assert nil source surfaces not-advertised, never Ready=true"
else
    record_s "missing_source_never_ready" "FAIL" "anycast package test failed; see test-__internal_anycast____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV09"; then
        record_l "frr_advertise_withdraw" "PASS" "tests/layer_l dv09_l_test.go green: device-attached route advertised in disposable netns, then withdrawn via ip route del"
        record_l "health_failure_unadvertise" "PASS" "tests/layer_l dv09_l_test.go green: route add + del removes the observable prefix; lister sees not-advertised"
        record_l "restart_reconciles_actual" "PASS" "tests/layer_l dv09_l_test.go green: netns drop + recreate wipes kernel route; reconciled state matches the kernel"
    else
        for s in frr_advertise_withdraw health_failure_unadvertise restart_reconciles_actual; do
            record_l "$s" "FAIL" "tests/layer_l dv09_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV09_.log"
        done
    fi
else
    for s in frr_advertise_withdraw health_failure_unadvertise restart_reconciles_actual; do
        record_l "$s" "NOT_VERIFIED" "disposable FRR fixture required; runner is $OSTYPE"
    done
fi

end_dv
