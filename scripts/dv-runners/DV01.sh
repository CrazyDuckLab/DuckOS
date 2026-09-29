#!/usr/bin/env bash
# DV01 — core collections, identities and pagination.
#
# S subcases (must pass via the real DV01 coverage
# tests): list_all_pages, detail_denied.
#
# L subcases (need disposable Linux): restart_preserves_identity,
# disabled_daemon.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV01"
start_dv

# DV01 list_all_pages: every required core V1 kind must
# have a real source-backed lister.
if run_go_test "./internal/objectlisters/..."; then
    record_s "list_all_pages" "PASS" "TestDV01EveryKindRegisteredWithRealLister + TestDV01WireguardCatalogRealLister + TestDV01RuntimeUnavailableSubcase all green: 19 kinds source-aware, runtime-unavailable subcase honest, no placeholders"
else
    record_s "list_all_pages" "FAIL" "DV01 coverage test failed (placeholder lister for some core kind, or missing source-backed code); see test-__internal_objectlisters____.log"
fi

# DV01 detail_denied: per-row RBAC enforcement on the
# list endpoint. internal/api TestObjects covers it.
if run_go_test "./internal/api/..."; then
    record_s "detail_denied" "PASS" "internal/api objects tests pass: 405/400/401/200 envelope, per-row RBAC, no per-kind ID leak"
else
    record_s "detail_denied" "FAIL" "api objects test failed; see test-__internal_api____.log"
fi

if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV01"; then
        record_l "restart_preserves_identity" "PASS" "tests/layer_l dv01_l_test.go green: restart keeps canonical identity; disabled daemon produces typed unavailable, never fake complete"
        record_l "disabled_daemon" "PASS" "tests/layer_l dv01_l_test.go green: missing-source daemon surfaces degraded/unavailable, no empty-complete fiction"
    else
        record_l "restart_preserves_identity" "FAIL" "tests/layer_l dv01_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV01_.log"
        record_l "disabled_daemon" "FAIL" "tests/layer_l dv01_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV01_.log"
    fi
else
    record_l "restart_preserves_identity" "NOT_VERIFIED" "runner is not Linux; restart-of-Engine+Agent path unavailable"
    record_l "disabled_daemon" "NOT_VERIFIED" "runner is not Linux; daemon-disable fixture unavailable"
fi

end_dv
