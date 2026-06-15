#!/bin/bash
# verify_bgp_reconnect_artifact.sh — Verify BGP reconnect lab artifacts
#
# Fails unless:
# - after-recovery BIRD output contains "Established"
# - after-recovery status JSON contains BGP status "ok"
# - PID before equals PID after
# - baseline/during/after artifact files exist
#
# Usage: scripts/verify_bgp_reconnect_artifact.sh "$ARTIFACT_DIR"

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

    # Extract BGP state
    local bgp_state
    bgp_state=$(jq -r '.checks[] | select(.name == "bgp") | .state // "unknown"' "$status_file" 2>/dev/null || echo "unknown")

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

main() {
    log_info "BGP reconnect artifact verifier"
    log_info "Artifact directory: $ARTIFACT_DIR"
    echo ""

    local exit_code=0

    # Run all verifications
    verify_artifacts_exist || exit_code=1
    echo ""

    verify_pid_unchanged || exit_code=1
    echo ""

    verify_after_recovery_bgp_established || exit_code=1
    echo ""

    verify_after_recovery_bgp_status_ok || exit_code=1
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
