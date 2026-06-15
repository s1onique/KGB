#!/bin/bash
# verify_bgp_reconnect_artifact.sh — Verify BGP reconnect lab artifacts
#
# Fails unless:
# - after-recovery BIRD output contains "Established"
# - after-recovery status JSON contains BGP status "ok"
# - PID before equals PID after
# - baseline/during/after artifact files exist
# - baseline BIRD route table contains deterministic prefix 10.77.77.0/24
# - after-recovery BIRD route table contains deterministic prefix (catches false-green)
# - BIRD import counters show non-zero routes imported
#
# This verifier catches the production false-green condition:
#   BGP Established + tovarisch says "configured prefixes" + BIRD 0 imported routes
#
# Usage: scripts/verify_bgp_reconnect_artifact.sh "$ARTIFACT_DIR" [TEST_PREFIX]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*"; }
log_info() { echo -e "[INFO] $*"; }

# Default artifact directory
ARTIFACT_DIR="${1:-${LAB_DIR:-.}}"

# Required artifact files
REQUIRED_ARTIFACTS=(
    "baseline-status-http.json"
    "during-failure-status-http.json"
    "after-recovery-status-http.json"
    "baseline-bird-protocols.txt"
    "during-failure-bird-protocols.txt"
    "after-recovery-bird-protocols.txt"
    "tovarisch-pid-before.txt"
    "tovarisch-pid-after.txt"
)

verify_artifacts_exist() {
    log_info "=== Verifying required artifacts exist ==="
    local exit_code=0

    for artifact in "${REQUIRED_ARTIFACTS[@]}"; do
        local filepath="$ARTIFACT_DIR/$artifact"
        if [[ -f "$filepath" ]]; then
            log_pass "Artifact exists: $artifact"
        else
            log_fail "Artifact missing: $artifact"
            exit_code=1
        fi
    done

    return $exit_code
}

verify_pid_unchanged() {
    log_info "=== Verifying tovarisch PID unchanged ==="

    local pid_before_file="$ARTIFACT_DIR/tovarisch-pid-before.txt"
    local pid_after_file="$ARTIFACT_DIR/tovarisch-pid-after.txt"

    if [[ ! -f "$pid_before_file" ]] || [[ ! -f "$pid_after_file" ]]; then
        log_fail "PID files not available"
        return 1
    fi

    local pid_before pid_after
    pid_before=$(cat "$pid_before_file")
    pid_after=$(cat "$pid_after_file")

    if [[ -z "$pid_before" ]] || [[ -z "$pid_after" ]]; then
        log_fail "PID is empty (before='$pid_before', after='$pid_after')"
        return 1
    fi

    if [[ "$pid_before" == "$pid_after" ]]; then
        log_pass "PID unchanged: $pid_before"
        return 0
    else
        log_fail "PID changed: $pid_before -> $pid_after"
        return 1
    fi
}

verify_after_recovery_bgp_established() {
    log_info "=== Verifying after-recovery BGP is Established ==="

    local protocols_file="$ARTIFACT_DIR/after-recovery-bird-protocols.txt"

    if [[ ! -f "$protocols_file" ]]; then
        log_fail "After-recovery protocols file not available"
        return 1
    fi

    if grep -qE "Established" "$protocols_file" 2>/dev/null; then
        log_pass "After-recovery BGP is Established"
        return 0
    else
        log_fail "After-recovery BGP not Established"
        echo "--- BIRD protocols content ---"
        cat "$protocols_file" | head -30
        return 1
    fi
}

verify_after_recovery_bgp_status_ok() {
    log_info "=== Verifying after-recovery HTTP status shows BGP OK ==="

    local status_file="$ARTIFACT_DIR/after-recovery-status-http.json"

    if [[ ! -f "$status_file" ]] || [[ ! -s "$status_file" ]]; then
        log_fail "After-recovery status JSON not available"
        return 1
    fi

    # Check if it's valid JSON first
    if ! jq . "$status_file" &> /dev/null; then
        log_fail "After-recovery status JSON is invalid"
        return 1
    fi

    # Extract BGP state (runtime JSON uses .status, not .state)
    local bgp_state
    bgp_state=$(jq -r '.checks[] | select(.name == "bgp") | (.status // .state // "unknown")' "$status_file" 2>/dev/null || echo "unknown")

    if [[ "$bgp_state" == "ok" ]] || [[ "$bgp_state" == "up" ]]; then
        log_pass "After-recovery BGP status: $bgp_state"
        return 0
    else
        log_fail "After-recovery BGP status: $bgp_state (expected ok/up)"
        echo "--- BGP check detail ---"
        jq -r '.checks[] | select(.name == "bgp")' "$status_file" 2>/dev/null || echo "Cannot parse"
        return 1
    fi
}

# NEW: Verify baseline BIRD has imported the deterministic prefix
# This proves tovarisch actually announced the route before failure.
verify_baseline_route_import() {
    log_info "=== Verifying baseline route import ==="

    local routes_file="$ARTIFACT_DIR/baseline-bird-routes.txt"
    local expected_prefix="${TEST_PREFIX:-10.77.77.0/24}"

    if [[ ! -f "$routes_file" ]] || [[ ! -s "$routes_file" ]]; then
        log_fail "Baseline routes file not available"
        return 1
    fi

    # Use -F for literal matching (prefixes contain dots which are regex wildcards)
    if grep -qF -- "$expected_prefix" "$routes_file" 2>/dev/null; then
        log_pass "Baseline: deterministic prefix '$expected_prefix' present in BIRD route table"
        return 0
    else
        log_fail "Baseline: deterministic prefix '$expected_prefix' NOT found in BIRD route table"
        echo "--- BIRD routes content ---"
        cat "$routes_file" 2>/dev/null | head -20
        return 1
    fi
}

# NEW: Verify after-recovery BIRD has imported the deterministic prefix
# This catches the false-green condition: BGP Established but 0 imported routes.
verify_after_recovery_route_import() {
    log_info "=== Verifying after-recovery route import ==="

    local routes_file="$ARTIFACT_DIR/after-recovery-bird-routes.txt"
    local expected_prefix="${TEST_PREFIX:-10.77.77.0/24}"

    if [[ ! -f "$routes_file" ]] || [[ ! -s "$routes_file" ]]; then
        log_fail "After-recovery routes file not available"
        return 1
    fi

    # Use -F for literal matching (prefixes contain dots which are regex wildcards)
    if grep -qF -- "$expected_prefix" "$routes_file" 2>/dev/null; then
        log_pass "After-recovery: deterministic prefix '$expected_prefix' present in BIRD route table"
        return 0
    else
        log_fail "After-recovery: deterministic prefix '$expected_prefix' NOT found in BIRD route table"
        log_fail "FALSE-GREEN CONDITION: BGP Established but 0 imported routes"
        echo "--- BIRD routes content ---"
        cat "$routes_file" 2>/dev/null | head -20
        return 1
    fi
}

# NEW: Verify BIRD import counters show non-zero routes imported
# Secondary signal: catches cases where routes appear but counters show 0.
verify_bird_import_counters_nonzero() {
    log_info "=== Verifying BIRD import counters ==="

    local protocol_detail_file="$ARTIFACT_DIR/after-recovery-bird-protocol-detail.txt"

    if [[ ! -f "$protocol_detail_file" ]] || [[ ! -s "$protocol_detail_file" ]]; then
        log_fail "After-recovery protocol detail file not available"
        return 1
    fi

    # Check for "Routes: N imported" where N > 0
    if grep -qE "Routes: [1-9][0-9]* imported" "$protocol_detail_file" 2>/dev/null; then
        local imported_count
        imported_count=$(grep -E "Routes: [0-9]+ imported" "$protocol_detail_file" | head -1)
        log_pass "BIRD shows non-zero import count: $imported_count"
        return 0
    fi

    if grep -qE "Routes: 0 imported" "$protocol_detail_file" 2>/dev/null; then
        log_fail "BIRD shows 0 imported routes (false-green condition)"
        echo "--- BIRD protocol detail counters ---"
        grep -E "(Routes:|Import updates:)" "$protocol_detail_file" 2>/dev/null || echo "(no match)"
        return 1
    fi

    log_info "Could not parse import counters (not fatal if route is present)"
    return 0
}

# NEW: Verify routes artifact files exist
verify_routes_artifacts() {
    log_info "=== Verifying routes artifacts ==="
    local exit_code=0

    local routes_artifacts=(
        "baseline-bird-routes.txt"
        "after-recovery-bird-routes.txt"
        "baseline-bird-protocol-detail.txt"
        "after-recovery-bird-protocol-detail.txt"
    )

    for artifact in "${routes_artifacts[@]}"; do
        local filepath="$ARTIFACT_DIR/$artifact"
        if [[ -f "$filepath" ]]; then
            log_pass "Routes artifact exists: $artifact"
        else
            log_fail "Routes artifact missing: $artifact"
            exit_code=1
        fi
    done

    return $exit_code
}

main() {
    log_info "BGP reconnect artifact verifier"
    log_info "Artifact directory: $ARTIFACT_DIR"
    echo ""

    local exit_code=0

    # Run all verifications
    verify_artifacts_exist || exit_code=1
    echo ""

    verify_routes_artifacts || exit_code=1
    echo ""

    verify_pid_unchanged || exit_code=1
    echo ""

    verify_after_recovery_bgp_established || exit_code=1
    echo ""

    verify_after_recovery_bgp_status_ok || exit_code=1
    echo ""

    # NEW: Route import verifications (catches false-green condition)
    verify_baseline_route_import || exit_code=1
    echo ""

    verify_after_recovery_route_import || exit_code=1
    echo ""

    verify_bird_import_counters_nonzero || exit_code=1
    echo ""

    # Final result
    echo "=== Verification Summary ==="
    if [[ $exit_code -eq 0 ]]; then
        log_pass "All verifications passed"
    else
        log_fail "Some verifications failed"
    fi

    return $exit_code
}

main "$@"
