#!/usr/bin/env bash
# dv-runners/lib.sh is the shared helper every per-DV
# runner sources. The helper writes a deterministic
# results.jsonl (one line per subcase with status +
# detail) and a per-DV .log so the harness can read
# machine-readable evidence.
#
# Usage (in a runner):
#     source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
#     EVIDENCE_DIR="$1"; ON_LINUX="$2"
#     DV="DV01"
#     start_dv
#         record_s "list_all_pages" "PASS" "all 19 kinds returned envelope"
#         record_l "restart_preserves_identity" "NOT_VERIFIED" "Linux fixture unavailable"
#     end_dv

set -uo pipefail

EVIDENCE_DIR="${EVIDENCE_DIR:-}"
DV="${DV:-DVXX}"
RESULTS_FILE=""
LOG_FILE=""
PASS_COUNT=0
FAIL_COUNT=0
NOTVER_COUNT=0
SKIP_COUNT=0
START_TS=""

start_dv() {
    if [[ -z "$EVIDENCE_DIR" ]]; then
        echo "EVIDENCE_DIR is empty" >&2
        exit 2
    fi
    mkdir -p "$EVIDENCE_DIR/$DV"
    RESULTS_FILE="$EVIDENCE_DIR/$DV/results.jsonl"
    LOG_FILE="$EVIDENCE_DIR/$DV/runner.log"
    : > "$RESULTS_FILE"
    : > "$LOG_FILE"
    START_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    log_line "runner start dv=$DV on_linux=${ON_LINUX:-0} ts=$START_TS"
}

end_dv() {
    local end_ts overall
    end_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    overall="PASS"
    if [[ "$FAIL_COUNT" -gt 0 ]]; then
        overall="FAIL"
    fi
    log_line "runner end dv=$DV pass=$PASS_COUNT fail=$FAIL_COUNT not_verified=$NOTVER_COUNT skip=$SKIP_COUNT overall=$overall"
    # machine-readable summary the harness aggregates
    cat > "$EVIDENCE_DIR/$DV/summary.json" <<EOF
{
  "dv": "$DV",
  "started_at": "$START_TS",
  "ended_at": "$end_ts",
  "on_linux": "${ON_LINUX:-0}",
  "pass": $PASS_COUNT,
  "fail": $FAIL_COUNT,
  "not_verified": $NOTVER_COUNT,
  "skip": $SKIP_COUNT,
  "overall": "$overall"
}
EOF
    if [[ "$FAIL_COUNT" -gt 0 ]]; then
        return 1
    fi
    # NOT_VERIFIED is NOT silent skip per contract: we
    # always return non-zero when an L-tier subcase
    # could not be executed, so the harness surfaces the
    # honest gap.
    if [[ "$NOTVER_COUNT" -gt 0 ]]; then
        return 1
    fi
    return 0
}

log_line() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" | tee -a "$LOG_FILE"
}

record_s() {
    # record_s <name> <PASS|FAIL> <detail>
    local name="$1" status="$2" detail="$3"
    local ts
    ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "{\"dv\":\"$DV\",\"subcase\":\"$name\",\"tier\":\"S\",\"status\":\"$status\",\"detail\":\"$detail\",\"ts\":\"$ts\"}" >>"$RESULTS_FILE"
    log_line "S $name $status: $detail"
    case "$status" in
        PASS) PASS_COUNT=$((PASS_COUNT+1)) ;;
        FAIL) FAIL_COUNT=$((FAIL_COUNT+1)) ;;
        NOT_VERIFIED) NOTVER_COUNT=$((NOTVER_COUNT+1)) ;;
        *) SKIP_COUNT=$((SKIP_COUNT+1)) ;;
    esac
}

record_l() {
    # record_l <name> <PASS|FAIL|NOT_VERIFIED> <detail>
    local name="$1" status="$2" detail="$3"
    local ts
    ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "{\"dv\":\"$DV\",\"subcase\":\"$name\",\"tier\":\"L\",\"status\":\"$status\",\"detail\":\"$detail\",\"ts\":\"$ts\"}" >>"$RESULTS_FILE"
    log_line "L $name $status: $detail"
    case "$status" in
        PASS) PASS_COUNT=$((PASS_COUNT+1)) ;;
        FAIL) FAIL_COUNT=$((FAIL_COUNT+1)) ;;
        NOT_VERIFIED) NOTVER_COUNT=$((NOTVER_COUNT+1)) ;;
        *) SKIP_COUNT=$((SKIP_COUNT+1)) ;;
    esac
}

# resolve_go_path picks the right Go toolchain for the
# runner. On Mac the .tools/go bootstrap is used; on
# Linux the system Go (or a user-installed /usr/local/go)
# is used. The result is prepended to PATH so `go test`
# finds the toolchain.
resolve_go_path() {
    local go_path=""
    if [[ -x "$ROOT/.tools/go/bin/go" ]]; then
        go_path="$ROOT/.tools/go/bin"
    elif [[ -x "/usr/local/go/bin/go" ]]; then
        go_path="/usr/local/go/bin"
    elif command -v go >/dev/null 2>&1; then
        go_path="$(command -v go)"
        go_path="${go_path%/*}"
    fi
    if [[ -n "$go_path" ]]; then
        echo "$go_path"
    fi
}

# resolve_go_toolchain returns "local" if a Go >=1.27 is
# already on PATH, otherwise "auto" so Go fetches the
# required toolchain (the project pins go 1.27.1 in
# go.mod; the duck-dev-01 fixture ships go 1.24.4 and
# relies on GOTOOLCHAIN=auto to install the pinned
# version on demand).
resolve_go_toolchain() {
    local gv=""
    if command -v go >/dev/null 2>&1; then
        gv="$(go version 2>/dev/null | awk '{print $3}')"
    fi
    if [[ -n "$gv" ]]; then
        local major minor
        major="${gv%%.*}"
        minor="${gv#*.}"; minor="${minor%%.*}"
        if [[ "$major" -gt 1 || ( "$major" -eq 1 && "$minor" -ge 27 ) ]]; then
            echo "local"
            return
        fi
    fi
    echo "auto"
}

# resolve_go_cache returns the right GOCACHE for the
# runner. Linux disposable fixtures use the home-dir
# cache (~/.cache/go-build) so we don't write to the
# project .cache which may be read-only.
resolve_go_cache() {
    if [[ -n "${GOCACHE:-}" ]]; then
        echo "$GOCACHE"
        return
    fi
    if [[ "$(uname -s)" == "Linux" ]]; then
        echo "$HOME/.cache/go-build"
        return
    fi
    echo "$ROOT/.cache/go-build"
}

# resolve_go_modcache returns the right GOMODCACHE.
resolve_go_modcache() {
    if [[ -n "${GOMODCACHE:-}" ]]; then
        echo "$GOMODCACHE"
        return
    fi
    if [[ "$(uname -s)" == "Linux" ]]; then
        echo "$HOME/go/pkg/mod"
        return
    fi
    echo "$ROOT/.cache/go-mod"
}

run_go_test() {
    # run_go_test <package-pattern>
    # Captures stdout/stderr to a per-test log file.
    local pkg="$1"
    local safe
    safe="$(echo "$pkg" | tr '/. ' '___')"
    local logf="$EVIDENCE_DIR/$DV/test-${safe}.log"
    local go_bin go_toolchain
    go_bin="$(resolve_go_path)"
    go_toolchain="$(resolve_go_toolchain)"
    local go_cache
    go_cache="$(resolve_go_cache)"
    local go_modcache
    go_modcache="$(resolve_go_modcache)"
    log_line "go test -count=1 -timeout 120s $pkg (log=$logf path=$go_bin toolchain=$go_toolchain cache=$go_cache)"
    if (cd "$ROOT" && PATH="$go_bin:$PATH" \
        GOTOOLCHAIN="$go_toolchain" GOCACHE="$go_cache" \
        GOMODCACHE="$go_modcache" \
        go test -count=1 -timeout 120s "$pkg") >"$logf" 2>&1; then
        return 0
    fi
    return 1
}

run_go_test_linux_sudo() {
    # run_go_test_linux_sudo <package-pattern> [-run <regex>]
    # The L-tier tests require root for netns/veth/wg/tc
    # operations. The runner is invoked on the duck-dev-01
    # disposable fixture as a non-root user; we escalate
    # via sudo so the production Go test binary sees a
    # real Linux root context. The user's HOME and PATH
    # are preserved so the local Go toolchain + module
    # cache are picked up.
    local pkg="$1"
    shift
    local safe
    safe="$(echo "$pkg $* " | tr '/. ' '___')"
    local logf="$EVIDENCE_DIR/$DV/test-L-${safe}.log"
    local go_bin go_toolchain
    go_bin="$(resolve_go_path)"
    go_toolchain="$(resolve_go_toolchain)"
    local go_cache
    go_cache="$(resolve_go_cache)"
    local go_modcache
    go_modcache="$(resolve_go_modcache)"
    log_line "sudo go test -count=1 -timeout 180s $* $pkg (log=$logf path=$go_bin toolchain=$go_toolchain cache=$go_cache)"
    if (cd "$ROOT" && sudo -E env "PATH=$go_bin:$PATH" \
        "HOME=$HOME" \
        "GOCACHE=$go_cache" \
        "GOMODCACHE=$go_modcache" \
        "GOTOOLCHAIN=$go_toolchain" \
        go test -count=1 -timeout 180s "$@" "$pkg") >"$logf" 2>&1; then
        return 0
    fi
    return 1
}

require_root() {
    # The contract permits the harness to be invoked on a
    # disposable root fixture; this helper fails closed when
    # the binary cannot run as root.
    if [[ "$(uname -s)" != "Linux" ]]; then
        return 1
    fi
    if [[ "$(id -u)" != "0" ]]; then
        return 1
    fi
    return 0
}

export ROOT="${ROOT:-$(cd "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)}"
