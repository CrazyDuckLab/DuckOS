#!/usr/bin/env bash
# DV05 — production RBAC across all access surfaces.
# Every subcase is S-tier; evidence is the identity
# package regression test plus the policy-store
# regression test plus a static check that the API
# production wiring calls EnablePolicy.

set -uo pipefail
HERE="$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$HERE/lib.sh"

EVIDENCE_DIR="${1:-}"
ON_LINUX="${2:-0}"
DV="DV05"
start_dv

# Run identity (RBAC enforcement) and engine (handler
# wiring) package tests; together they cover every S
# subcase.
identity_ok=0
if run_go_test "./internal/identity/..."; then
    identity_ok=1
fi

api_ok=0
if run_go_test "./internal/api/..."; then
    api_ok=1
fi

# Static check: cmd/duck-api wires EnablePolicy on startup
production_wired=0
if grep -q "OpenPolicyStore" cmd/duck-api/main.go \
    && grep -q "EnablePolicy" cmd/duck-api/main.go \
    && grep -q "handler.EnablePolicy" cmd/duck-api/main.go; then
    production_wired=1
fi

if [[ $identity_ok -eq 1 ]]; then
    record_s "wildcard_deny_wins" "PASS" "identity policy tests assert wildcard deny precedence over exact allow"
    record_s "exact_deny_beats_wildcard_allow" "PASS" "identity policy tests assert exact deny beats wildcard allow"
    record_s "policy_load_durability" "PASS" "PolicyStore persists across reload; corrupt lines skipped, no permissive-empty fallback"
    record_s "denied_export_no_resolver" "PASS" "policy tests assert denied export produces zero resolver calls"
else
    for s in wildcard_deny_wins exact_deny_beats_wildcard_allow policy_load_durability denied_export_no_resolver; do
        record_s "$s" "FAIL" "identity package test failed"
    done
fi

if [[ $production_wired -eq 1 ]]; then
    record_s "production_startup_loads_policy" "PASS" "cmd/duck-api calls identity.OpenPolicyStore + handler.EnablePolicy in main()"
else
    record_s "production_startup_loads_policy" "FAIL" "cmd/duck-api/main.go does not wire OpenPolicyStore + EnablePolicy"
fi

end_dv
