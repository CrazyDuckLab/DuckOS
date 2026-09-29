#!/usr/bin/env bash
# DV07 — verified executable update and restart
# recovery. S subcases:
#  - corrupt_package_refused
#  - rollback_failure_not_healthy
# L subcases need real signed fixture + service restart.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV07"
start_dv

if run_go_test "./internal/upgrade/..."; then
    record_s "corrupt_package_refused" "PASS" "upgrade package tests reject corrupt/short/unsigned/wrong-arch before activation"
    record_s "rollback_failure_not_healthy" "PASS" "upgrade orchestrator tests assert rollback failures surface recovery-required, never healthy"
else
    record_s "corrupt_package_refused" "FAIL" "upgrade package test failed; see test-__internal_upgrade____.log"
    record_s "rollback_failure_not_healthy" "FAIL" "upgrade package test failed; see test-__internal_upgrade____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV07"; then
        record_l "signed_package_activate" "PASS" "tests/layer_l dv07_l_test.go green: stage/activate signed fixture; running build-marker changes"
        record_l "health_failure_recovery" "PASS" "tests/layer_l dv07_l_test.go green: failing health restores known-good or surfaces recovery-required"
        record_l "kill_at_phase_boundary" "PASS" "tests/layer_l dv07_l_test.go green: kill before/after materialize, anchor transition, migration, service start, health receipt"
    else
        for s in signed_package_activate health_failure_recovery kill_at_phase_boundary; do
            record_l "$s" "FAIL" "tests/layer_l dv07_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV07_.log"
        done
    fi
else
    for s in signed_package_activate health_failure_recovery kill_at_phase_boundary; do
        record_l "$s" "NOT_VERIFIED" "real signed fixture + service build-marker required; runner is $OSTYPE"
    done
fi

end_dv
