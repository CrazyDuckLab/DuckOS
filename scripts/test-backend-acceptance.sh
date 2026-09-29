#!/usr/bin/env bash
# test-backend-acceptance.sh is the backend acceptance
# manifest runner. The interface is fixed:
#
#   scripts/test-backend-acceptance.sh --list
#   scripts/test-backend-acceptance.sh --id DV00 --evidence <dir>
#   scripts/test-backend-acceptance.sh --id DV01 --evidence <dir>
#   scripts/test-backend-acceptance.sh --id DV02 --evidence <dir>
#   ... --id DV10 --evidence <dir>
#   scripts/test-backend-acceptance.sh --all  --evidence <dir>
#
# DV04 runs BOTH the WG and Xray branches. Each DV has
# tier-S (subcase mandatory) and tier-L (subcase requiring
# a disposable Linux fixture; marked NOT VERIFIED if the
# fixture is unavailable).
#
# The runner does NOT silently skip L cases: when a
# disposable Linux fixture is unavailable the runner
# exits non-zero with NOT VERIFIED so the next
# independent verification knows exactly which DVs
# were not executed.

set -eo pipefail

ROOT="$(cd "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export GOTOOLCHAIN=local
# PATH / GOCACHE / GOMODCACHE are resolved per-DV by the
# lib.sh helper, which detects the host (Mac .tools/go vs
# Linux /usr/local/go vs PATH-relative go) and the runner
# user (root vs non-root). Do NOT hard-code them here: the
# duck-dev-01 disposable fixture has neither .tools/go nor
# a project .cache directory writable by the test user.

# Canonical subcase catalogue. tier-S cases MUST pass;
# tier-L cases require a disposable Linux fixture.
DV_CATALOGUE='[
  {"id":"DV00","family":"production-composition","subcases":[
    {"name":"production_composition","tier":"S","desc":"installed duck/duck-api/duck-agent/main calls expose the production composition root, not stubs"},
    {"name":"runtime_oracle_mutation","tier":"L","desc":"a representative mutation runs against a disposable Linux fixture and produces an independent runtime effect"}
  ]},
  {"id":"DV01","family":"core-collections","subcases":[
    {"name":"list_all_pages","tier":"S","desc":"every core V1 kind returns stable identity, pagination, snapshotID across all pages"},
    {"name":"detail_denied","tier":"S","desc":"denied IDs/payload/identity never appear in Items/Issues/cursor"},
    {"name":"restart_preserves_identity","tier":"L","desc":"restart API/Agent does not lose canonical identity and does not resurrect stale observed cache"},
    {"name":"disabled_daemon","tier":"L","desc":"missing source daemon produces typed degraded/unavailable, not fake empty complete"}
  ]},
  {"id":"DV02","family":"config-transaction","subcases":[
    {"name":"preview_apply_confirm","tier":"L","desc":"preview/apply/confirm on a disposable managed veth fixture with real nft/route mutation"},
    {"name":"idempotent_request_id","tier":"S","desc":"identical request id + payload returns the same admission without re-applying"},
    {"name":"duplicate_request_id_different_payload","tier":"S","desc":"same request id + different payload refuses to admission"},
    {"name":"stale_revision_refused","tier":"S","desc":"stale expected revision refuses before runtime call"},
    {"name":"confirm_crossing_deadline","tier":"S","desc":"confirm after confirmation deadline does NOT commit"},
    {"name":"kill_after_admission","tier":"L","desc":"Engine and Agent killed independently after admission; restart picks up the durable record without double execution"}
  ]},
  {"id":"DV03","family":"qos-apply","subcases":[
    {"name":"tc_oracle_apply_rollback","tier":"L","desc":"apply class/shaper on disposable veth; tc reads confirm values; rollback restores before state"},
    {"name":"partial_apply_does_not_commit","tier":"L","desc":"runner fault after first tc step leaves the committed desired state untouched"},
    {"name":"failed_rollback_recovery_required","tier":"L","desc":"tc rollback failure returns recovery-required, not false reverted"},
    {"name":"cancel_does_not_abort_recovery","tier":"S","desc":"caller cancel of original request does NOT cancel the recovery path"},
    {"name":"deny_zero_runtime_calls","tier":"L","desc":"denied/stale apply produces zero tc mutations"}
  ]},
  {"id":"DV04-WG","family":"wireguard","subcases":[
    {"name":"crud_kernel_runtime","tier":"L","desc":"create/update/disable/re-enable/delete mutates the kernel via real wg tool and the catalog"},
    {"name":"restart_no_resurrection","tier":"L","desc":"restart does not resurrect deleted peers from old catalog"},
    {"name":"export_redacted_matches_qr","tier":"L","desc":"export redacted .conf equals the decoded QR payload; private key never leaves the protected export"},
    {"name":"denied_export_zero_effect","tier":"S","desc":"export with denied permission produces zero key-resolver invocations"}
  ]},
  {"id":"DV04-XRAY","family":"xray","subcases":[
    {"name":"crud_xray_validation","tier":"L","desc":"create/update/disable/delete goes through native Xray validation and runtime listener"},
    {"name":"subscription_qr_matches_url","tier":"L","desc":"subscription export QR equals the exact issued URL"},
    {"name":"restart_no_resurrection","tier":"L","desc":"restart does not resurrect deleted users/profiles"},
    {"name":"incompatible_protocol_rejected","tier":"S","desc":"unsupported protocol/security combination is refused before runtime"}
  ]},
  {"id":"DV05","family":"rbac-production","subcases":[
    {"name":"wildcard_deny_wins","tier":"S","desc":"wildcard deny beats exact allow"},
    {"name":"exact_deny_beats_wildcard_allow","tier":"S","desc":"exact deny beats wildcard allow"},
    {"name":"policy_load_durability","tier":"S","desc":"policy persists across restart; corrupted policy never becomes permissive empty"},
    {"name":"denied_export_no_resolver","tier":"S","desc":"denied export produces zero vault/resolver invocations"},
    {"name":"production_startup_loads_policy","tier":"S","desc":"cmd/duck-api loads PolicyStore on startup; Policy unit tests are insufficient evidence"}
  ]},
  {"id":"DV06","family":"cli-api-parity","subcases":[
    {"name":"network_cli_uses_public_api","tier":"S","desc":"duck network commands only connect to /api/v1/config/*, never /agent/v1/* directly"},
    {"name":"upgrade_cli_uses_public_api","tier":"S","desc":"duck upgrade activate/status/rollback reach the public API"},
    {"name":"backup_cli_uses_public_api","tier":"S","desc":"duck backup list/plan/restore reach the public API"},
    {"name":"actor_principal_not_spoofed","tier":"S","desc":"--actor flag does not bypass session principal"},
    {"name":"non_tty_does_not_prompt","tier":"S","desc":"non-TTY CLI invocation never prompts for secrets"}
  ]},
  {"id":"DV07","family":"update-workflow","subcases":[
    {"name":"signed_package_activate","tier":"L","desc":"stage/activate a signed fixture package; running build-marker changes"},
    {"name":"corrupt_package_refused","tier":"S","desc":"corrupt/truncated/unsigned package refused before activation"},
    {"name":"health_failure_recovery","tier":"L","desc":"failing health restores known-good or surfaces recovery-required"},
    {"name":"kill_at_phase_boundary","tier":"L","desc":"kill before/after materialize, anchor transition, migration, service start, health receipt"},
    {"name":"rollback_failure_not_healthy","tier":"S","desc":"failed rollback is not reported as healthy"}
  ]},
  {"id":"DV08","family":"backup-restore","subcases":[
    {"name":"portable_selective_restore","tier":"L","desc":"fixture A backup restored into fixture B with different virtual NIC names"},
    {"name":"selected_unselected_comparison","tier":"L","desc":"selected objects restored; unselected objects unchanged"},
    {"name":"cancelled_before_during_rollback","tier":"L","desc":"cancelled apply restores B before-state"},
    {"name":"corrupt_archive_refused","tier":"S","desc":"corrupt archive refused before any runtime call"},
    {"name":"disk_write_failure_not_committed","tier":"S","desc":"ENOSPC/rename failure never returns committed"},
    {"name":"kill_at_phase_boundary","tier":"L","desc":"kill at snapshot/admission/partial apply/verify/commit; restart reconciles"}
  ]},
  {"id":"DV09","family":"anycast","subcases":[
    {"name":"frr_advertise_withdraw","tier":"L","desc":"isolated FRR fixture advertises/withdraws the bound VIP via real zebra/bgp"},
    {"name":"health_failure_unadvertise","tier":"L","desc":"health failure removes the advertisement"},
    {"name":"missing_source_never_ready","tier":"S","desc":"no FRR source means honest degraded/not-advertised, never ready fiction"},
    {"name":"restart_reconciles_actual","tier":"L","desc":"restart after command but before receipt; reconcile actual FRR state, no resurrected withdrawn prefix"}
  ]},
  {"id":"DV10","family":"ztp","subcases":[
    {"name":"production_key_loader","tier":"S","desc":"cmd/duck-agent loads ZTP production key via OpenManager; arbitrary short key rejected"},
    {"name":"authentic_issue_redeem","tier":"S","desc":"IssuedToken minted via Manager.Issue is verified via Manager.Redeem (MAC + TTL + scope + node)"},
    {"name":"replay_refused","tier":"S","desc":"second Redeem of the same IssuedToken returns ErrTokenReplay"},
    {"name":"capacity_256_fail_closed","tier":"S","desc":"MaxRedemptionJournal unexpired records exhaust capacity: ErrCapacityExhausted, no eviction"},
    {"name":"corrupt_journal_lines_skipped","tier":"S","desc":"malformed journal lines skipped on reload; good records preserved"},
    {"name":"second_writer_no_collision","tier":"L","desc":"two concurrent OpenManager writers do not corrupt the durable file"}
  ]}
]'

list_dvs() {
    echo "$DV_CATALOGUE" | python3 -c '
import json,sys
data=json.load(sys.stdin)
for fam in data:
    fid = fam["id"]
    famname = fam["family"]
    s = sum(1 for c in fam["subcases"] if c["tier"]=="S")
    l = sum(1 for c in fam["subcases"] if c["tier"]=="L")
    print("  " + fid + "  tier-S=" + str(s) + " tier-L=" + str(l) + "  " + famname)
    for c in fam["subcases"]:
        print("      [" + c["tier"] + "] " + c["name"] + ": " + c["desc"])
'
}

LIST=0
ALL=0
TARGET_DV=""
EVIDENCE_DIR=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --list) LIST=1 ;;
        --all) ALL=1 ;;
        --id) TARGET_DV="$2"; shift ;;
        --evidence) EVIDENCE_DIR="$2"; shift ;;
        *) echo "unknown arg: $1" >&2; exit 2 ;;
    esac
    shift
done

if [[ "$LIST" == "1" ]]; then
    echo "Backend acceptance manifest:"
    list_dvs
    exit 0
fi

if [[ -z "$EVIDENCE_DIR" ]]; then
    echo "--evidence <dir> is required" >&2
    exit 2
fi
mkdir -p "$EVIDENCE_DIR"
SUMMARY="$EVIDENCE_DIR/summary.json"
LOG="$EVIDENCE_DIR/acceptance.log"
: > "$LOG"
echo "[harness] evidence=$EVIDENCE_DIR log=$LOG" >>"$LOG"

if [[ -z "$TARGET_DV" && "$ALL" == "0" ]]; then
    echo "either --list, --id DVxx, or --all is required" >&2
    exit 2
fi

OS_VERSION="$(uname -srvmo 2>&1 || true)"
GO_VERSION="$(go version 2>&1 || true)"
GIT_BASE="$(git rev-parse HEAD)"
GIT_DIRTY="$(git status --porcelain | wc -l | tr -d ' ')"

cat > "$SUMMARY" <<EOF
{
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "git_head": "$GIT_BASE",
  "git_dirty": "$GIT_DIRTY",
  "os": "$OS_VERSION",
  "go": "$GO_VERSION"
}
EOF

ON_LINUX=0
if [[ "$(uname -s)" == "Linux" ]]; then ON_LINUX=1; fi

DV_FAMILY=""
if [[ "$ALL" == "1" ]]; then
    DV_FAMILY="DV00 DV01 DV02 DV03 DV04-WG DV04-XRAY DV05 DV06 DV07 DV08 DV09 DV10"
else
    case "$TARGET_DV" in
        DV00) DV_FAMILY="DV00" ;;
        DV01) DV_FAMILY="DV01" ;;
        DV02) DV_FAMILY="DV02" ;;
        DV03) DV_FAMILY="DV03" ;;
        DV04) DV_FAMILY="DV04-WG DV04-XRAY" ;;
        DV05) DV_FAMILY="DV05" ;;
        DV06) DV_FAMILY="DV06" ;;
        DV07) DV_FAMILY="DV07" ;;
        DV08) DV_FAMILY="DV08" ;;
        DV09) DV_FAMILY="DV09" ;;
        DV10) DV_FAMILY="DV10" ;;
        "") echo "either --list, --id DVxx, or --all is required" >&2; exit 2 ;;
        *) echo "unknown DV: $TARGET_DV" >&2; exit 2 ;;
    esac
fi

SCRIPT_DIR="$(dirname -- "${BASH_SOURCE[0]}")"

RESULTS_FILE="$EVIDENCE_DIR/results.jsonl"
: > "$RESULTS_FILE"

run_runner() {
    local dv="$1"
    local runner="$SCRIPT_DIR/dv-runners/$dv.sh"
    if [[ ! -x "$runner" ]]; then
        echo "[$dv] runner NOT FOUND: $runner" >>"$LOG"
        return 1
    fi
    echo "[$dv] runner=$runner" >>"$LOG"
    if "$runner" "$EVIDENCE_DIR" "$ON_LINUX" >>"$LOG" 2>&1; then
        rc=0
    else
        echo "[$dv] runner exited non-zero (see $LOG)" >>"$LOG"
        rc=1
    fi
    # Aggregate per-DV results into the top-level
    # results.jsonl the contract requires.
    if [[ -f "$EVIDENCE_DIR/$dv/results.jsonl" ]]; then
        cat "$EVIDENCE_DIR/$dv/results.jsonl" >>"$RESULTS_FILE"
    fi
    return $rc
}

OVERALL=0
for dv in $DV_FAMILY; do
    run_runner "$dv" || OVERALL=1
done

echo "[harness] overall_exit=$OVERALL evidence=$EVIDENCE_DIR" >>"$LOG"
exit $OVERALL
