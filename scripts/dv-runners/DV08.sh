#!/usr/bin/env bash
# DV08 — portable/selective restore with durable
# recovery. S subcases:
#  - corrupt_archive_refused
#  - disk_write_failure_not_committed
# L subcases need real backup fixture + cross-process
# kill/restart.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV08"
start_dv

if run_go_test "./internal/backup/..."; then
    record_s "corrupt_archive_refused" "PASS" "backup package tests assert corrupt archive/path-traversal refused before runtime"
    record_s "disk_write_failure_not_committed" "PASS" "backup journal-store tests assert ENOSPC/rename failure never returns committed"
else
    record_s "corrupt_archive_refused" "FAIL" "backup package test failed; see test-__internal_backup____.log"
    record_s "disk_write_failure_not_committed" "FAIL" "backup package test failed; see test-__internal_backup____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV08"; then
        record_l "portable_selective_restore" "PASS" "tests/layer_l dv08_l_test.go green: fixture A backup restored into fixture B with different NIC names"
        record_l "selected_unselected_comparison" "PASS" "tests/layer_l dv08_l_test.go green: selected objects restored; unselected objects unchanged"
        record_l "cancelled_before_during_rollback" "PASS" "tests/layer_l dv08_l_test.go green: cancelled apply restores B before-state"
        record_l "kill_at_phase_boundary" "PASS" "tests/layer_l dv08_l_test.go green: kill at snapshot/admission/partial apply/verify/commit; restart reconciles"
    else
        for s in portable_selective_restore selected_unselected_comparison cancelled_before_during_rollback kill_at_phase_boundary; do
            record_l "$s" "FAIL" "tests/layer_l dv08_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV08_.log"
        done
    fi
else
    for s in portable_selective_restore selected_unselected_comparison cancelled_before_during_rollback kill_at_phase_boundary; do
        record_l "$s" "NOT_VERIFIED" "real cross-host backup fixture + cross-process kill required; runner is $OSTYPE"
    done
fi

end_dv
