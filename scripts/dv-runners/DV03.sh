#!/usr/bin/env bash
# DV03 — QoS apply, partial failure and rollback.
#
# All four L subcases (tc_oracle_apply_rollback,
# partial_apply_does_not_commit,
# failed_rollback_recovery_required,
# deny_zero_runtime_calls) require a disposable Linux
# veth fixture with real tc. The single S subcase
# (cancel_does_not_abort_recovery) is covered by the
# qos.Controller unit test.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV03"
start_dv

if run_go_test "./internal/qos/..."; then
    record_s "cancel_does_not_abort_recovery" "PASS" "qos package tests green; final-verification cancellation-and-truth assertions held"
else
    record_s "cancel_does_not_abort_recovery" "FAIL" "qos package test failed; see test-__internal_qos____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV03"; then
        record_l "tc_oracle_apply_rollback" "PASS" "tests/layer_l dv03_l_test.go green: tc qdisc observable via independent oracle; rollback restores before-state"
        record_l "partial_apply_does_not_commit" "PASS" "tests/layer_l dv03_l_test.go green: runner fault after first tc step leaves committed desired state untouched"
        record_l "failed_rollback_recovery_required" "PASS" "tests/layer_l dv03_l_test.go green: rollback failure surfaces recovery-required, never false reverted"
        record_l "deny_zero_runtime_calls" "PASS" "tests/layer_l dv03_l_test.go green: denied/stale apply produces zero tc mutations"
    else
        for s in tc_oracle_apply_rollback partial_apply_does_not_commit failed_rollback_recovery_required deny_zero_runtime_calls; do
            record_l "$s" "FAIL" "tests/layer_l dv03_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV03_.log"
        done
    fi
else
    for s in tc_oracle_apply_rollback partial_apply_does_not_commit failed_rollback_recovery_required deny_zero_runtime_calls; do
        record_l "$s" "NOT_VERIFIED" "disposable Linux veth fixture required; runner is $OSTYPE"
    done
fi

end_dv
