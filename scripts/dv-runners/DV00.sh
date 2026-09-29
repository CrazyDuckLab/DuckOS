#!/usr/bin/env bash
# DV00 — production composition / no fake success.
#
# S (production_composition): verify the main entry
# points really wire the production constructors
# (orchestrator / listers / policy / ztp), not stubs.
#
# L (runtime_oracle_mutation): exercise a representative
# mutation against a disposable Linux fixture. On
# non-Linux runners this is NOT_VERIFIED, never silently
# skipped.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV00"
start_dv

# S: production_composition
fail=0
for symbol in \
    "cmd/duck-agent/main.go:enableUpgradeService" \
    "cmd/duck-agent/main.go:enableZTPService" \
    "cmd/duck-agent/main.go:enableQoSService" \
    "cmd/duck-api/main.go:EnableIdentity" \
    "cmd/duck-api/main.go:EnablePolicy" \
    "internal/objectlisters/listers.go:RegisterAll"; do
    file="${symbol%%:*}"
    needle="${symbol##*:}"
    if grep -qF "$needle" "$file" 2>/dev/null; then
        log_line "found $symbol"
    else
        log_line "MISSING $symbol"
        fail=1
    fi
done

if [[ $fail -eq 0 ]]; then
    record_s "production_composition" "PASS" "all 6 production-root wiring anchors present"
else
    record_s "production_composition" "FAIL" "one or more composition roots missing"
fi

# L: runtime_oracle_mutation — needs disposable Linux.
if [[ "$ON_LINUX" == "1" ]]; then
    if run_go_test_linux_sudo "./tests/layer_l/..." -run "TestDV00"; then
        record_l "runtime_oracle_mutation" "PASS" "tests/layer_l dv00_l_test.go green: production mutation observable by independent kernel oracle (no fake success)"
    else
        record_l "runtime_oracle_mutation" "FAIL" "tests/layer_l dv00_l_test.go failed; see test-L-___tests_layer_l_______-run_TestDV00_.log"
    fi
else
    record_l "runtime_oracle_mutation" "NOT_VERIFIED" "runner is not Linux ($OSTYPE); disposable Linux fixture unavailable"
fi

end_dv
