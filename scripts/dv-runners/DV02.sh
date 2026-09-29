#!/usr/bin/env bash
# DV02 — real config transaction and Safe Apply.
#
# S subcases (engine-level idempotency + admission
# gates): idempotent_request_id,
# duplicate_request_id_different_payload,
# stale_revision_refused, confirm_crossing_deadline.
#
# L subcases (need disposable Linux veth + nft/route
# mutation + cross-process restart): preview_apply_confirm,
# kill_after_admission.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV02"
start_dv

# Drive every S subcase through the dedicated engine
# regression tests.
if run_go_test "./internal/engine/..."; then
    record_s "idempotent_request_id" "PASS" "engine idempotency test green; same canonical request returns same admission"
    record_s "duplicate_request_id_different_payload" "PASS" "engine duplicate/payload-conflict test green"
    record_s "stale_revision_refused" "PASS" "engine stale-revision refusal test green"
    record_s "confirm_crossing_deadline" "PASS" "engine confirmation-deadline guard test green"
else
    for s in idempotent_request_id duplicate_request_id_different_payload stale_revision_refused confirm_crossing_deadline; do
        record_s "$s" "FAIL" "engine test failed; see test-__internal_engine____.log"
    done
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV02"; then
        record_l "preview_apply_confirm" "PASS" "tests/layer_l dv02_l_test.go green: preview/apply/confirm on disposable netns with real route/dummy mutation"
        record_l "kill_after_admission" "PASS" "tests/layer_l dv02_l_test.go green: Engine+Agent killed after admission; restart reconciles durable record, no double execution"
    else
        record_l "preview_apply_confirm" "FAIL" "tests/layer_l dv02_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV02_.log"
        record_l "kill_after_admission" "FAIL" "tests/layer_l dv02_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV02_.log"
    fi
else
    record_l "preview_apply_confirm" "NOT_VERIFIED" "disposable veth+nft/route fixture required; cannot run on $OSTYPE"
    record_l "kill_after_admission" "NOT_VERIFIED" "Engine+Agent cross-process kill/restart requires Linux"
fi

end_dv
