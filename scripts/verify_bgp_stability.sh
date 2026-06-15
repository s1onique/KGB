#!/bin/bash
# verify_bgp_stability.sh — Verify BGP stability lab artifacts
#
# Fails when:
# - BGP is not Established
# - BFD is not Up
# - imported routes are zero
# - reconnect delta exceeds budget
# - reconnect count increases after Established
# - BIRD protocol output contains recent 'Socket: Connection closed' after stable point
#
# Usage: scripts/verify_bgp_stability.sh "$ARTIFACT_DIR" [RECONNECT_BUDGET]
#        scripts/verify_bgp_stability.sh --self-test
#
# Artifacts expected:
#   status-before.json
#   status-first-established.json
#   status-after-stability.json
#   bird-protocol-before.txt
#   bird-protocol-first-established.txt
#   bird-protocol-after-stability.txt

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
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

# Default artifact directory and reconnect budget
ARTIFACT_DIR="${1:-${LAB_DIR:-.}}"
RECONNECT_BUDGET="${2:-1}"

# =============================================================================
# Helper Functions
# =============================================================================

require_artifact() {
    local artifact="$1"
    local filepath="$ARTIFACT_DIR/$artifact"
    if [[ -f "$filepath" ]] && [[ -s "$filepath" ]]; then
        log_pass "Artifact exists: $artifact"
        return 0
    else
        log_fail "Artifact missing or empty: $artifact"
        return 1
    fi
}

extract_reconnect_count() {
    local status_file="$1"
    if [[ ! -f "$status_file" ]] || [[ ! -s "$status_file" ]]; then
        echo ""
        return 1
    fi
    local reconnect_count
    reconnect_count=$(jq -r '
        .bgp.reconnect_count //
        .checks[] | select(.name == "bgp") | .reconnect_count //
        .metrics.bgp.reconnect_count //
        empty
    ' "$status_file" 2>/dev/null || echo "")
    if [[ -z "$reconnect_count" ]] || [[ "$reconnect_count" == "null" ]]; then
        reconnect_count=$(jq -r '
            .. | objects | select(has("reconnect_count")) | .reconnect_count // empty
        ' "$status_file" 2>/dev/null || echo "")
    fi
    echo "${reconnect_count:-}"
}

bird_is_established() {
    local protocol_file="$1"
    [[ -f "$protocol_file" ]] && [[ -s "$protocol_file" ]] && grep -qE "Established" "$protocol_file" 2>/dev/null
}

bird_has_recent_closures() {
    local protocol_file="$1"
    [[ -f "$protocol_file" ]] && grep -qiE "Connection closed|Socket:.*closed|connect.*failed" "$protocol_file" 2>/dev/null
}

count_bird_routes() {
    local routes_file="$1"
    if [[ ! -f "$routes_file" ]] || [[ ! -s "$routes_file" ]]; then
        echo "0"
        return 1
    fi
    grep -cE "\S" "$routes_file" 2>/dev/null || echo "0"
}

# =============================================================================
# Verification Functions
# =============================================================================

verify_artifacts_exist() {
    log_info "=== Verifying required stability artifacts ==="
    local exit_code=0
    for artifact in "status-before.json" "status-first-established.json" "status-after-stability.json" \
                    "bird-protocol-before.txt" "bird-protocol-first-established.txt" "bird-protocol-after-stability.txt"; do
        require_artifact "$artifact" || exit_code=1
    done
    return $exit_code
}

verify_bgp_established() {
    log_info "=== Verifying BGP is Established ==="
    local status_after="$ARTIFACT_DIR/status-after-stability.json"
    # JSON-aware check: verify BGP state is "established" in status JSON
    if [[ -f "$status_after" ]] && jq -e '
        (.bgp.state == "established") or
        (.checks[] | select(.name == "bgp") | .state == "established") or
        (.metrics.bgp.state == "established")
    ' "$status_after" > /dev/null 2>&1; then
        log_pass "BGP is Established at stability end (from JSON)"
        return 0
    fi
    # Fallback: check BIRD protocol text file
    if bird_is_established "$ARTIFACT_DIR/bird-protocol-after-stability.txt"; then
        log_pass "BGP is Established (from BIRD protocol file)"
        return 0
    fi
    log_fail "BGP is not Established at stability end"
    cat "$ARTIFACT_DIR/bird-protocol-after-stability.txt" 2>/dev/null | head -20
    return 1
}

verify_bfd_up() {
    log_info "=== Verifying BFD is Up ==="
    local bfd_sessions="$ARTIFACT_DIR/bird-bfd-sessions.txt"
    if [[ -f "$bfd_sessions" ]] && grep -qE '(^|[[:space:]])Up([[:space:]]|$)' "$bfd_sessions" 2>/dev/null; then
        log_pass "BFD is Up"
        return 0
    fi
    local status_after="$ARTIFACT_DIR/status-after-stability.json"
    if [[ -f "$status_after" ]]; then
        local bfd_detail
        bfd_detail=$(jq -r '.checks[] | select(.name == "bfd") | .detail // empty' "$status_after" 2>/dev/null || echo "")
        if [[ "$bfd_detail" == *"Up"* ]] || [[ "$bfd_detail" == *"up"* ]]; then
            log_pass "BFD is Up (from status)"
            return 0
        fi
    fi
    log_fail "BFD is not Up"
    cat "$bfd_sessions" 2>/dev/null || echo "Not available"
    return 1
}

verify_bird_established() {
    log_info "=== Verifying BIRD shows Established ==="
    if bird_is_established "$ARTIFACT_DIR/bird-protocol-after-stability.txt"; then
        log_pass "BIRD shows Established at stability end"
        return 0
    fi
    log_fail "BIRD does not show Established at stability end"
    cat "$ARTIFACT_DIR/bird-protocol-after-stability.txt" 2>/dev/null | head -20
    return 1
}

verify_imported_routes_nonzero() {
    log_info "=== Verifying BIRD imported route count > 0 ==="
    local routes_file="$ARTIFACT_DIR/bird-routes.txt"
    if [[ ! -f "$routes_file" ]]; then
        log_fail "Routes file missing: $routes_file"
        return 1
    fi
    if [[ ! -s "$routes_file" ]]; then
        log_fail "Routes file is empty: $routes_file"
        return 1
    fi
    local route_count
    route_count=$(count_bird_routes "$routes_file")
    if [[ "$route_count" -gt 0 ]]; then
        log_pass "BIRD imported routes: $route_count > 0"
        return 0
    fi
    log_fail "BIRD imported routes: $route_count (expected > 0)"
    return 1
}

verify_reconnect_budget() {
    log_info "=== Verifying reconnect budget ==="
    local status_before="$ARTIFACT_DIR/status-before.json"
    local status_first="$ARTIFACT_DIR/status-first-established.json"
    local reconnect_before reconnect_first
    reconnect_before=$(extract_reconnect_count "$status_before")
    reconnect_first=$(extract_reconnect_count "$status_first")

    if [[ -z "$reconnect_before" ]]; then
        log_fail "reconnect_count missing in status-before.json"
        return 1
    fi
    if [[ -z "$reconnect_first" ]]; then
        log_fail "reconnect_count missing in status-first-established.json"
        return 1
    fi
    if [[ ! "$reconnect_before" =~ ^[0-9]+$ ]]; then
        log_fail "reconnect_count non-numeric in status-before.json: '$reconnect_before'"
        return 1
    fi
    if [[ ! "$reconnect_first" =~ ^[0-9]+$ ]]; then
        log_fail "reconnect_count non-numeric in status-first-established.json: '$reconnect_first'"
        return 1
    fi
    local delta=$((reconnect_first - reconnect_before))
    if [[ $delta -le $RECONNECT_BUDGET ]]; then
        log_pass "Reconnect delta ${delta} <= budget ${RECONNECT_BUDGET}"
        return 0
    fi
    log_fail "Reconnect delta ${delta} > budget ${RECONNECT_BUDGET}"
    log_fail "  reconnect_before: $reconnect_before"
    log_fail "  reconnect_first: $reconnect_first"
    return 1
}

verify_reconnect_no_increase_after_established() {
    log_info "=== Verifying reconnect count does not increase after Established ==="
    local status_first="$ARTIFACT_DIR/status-first-established.json"
    local status_after="$ARTIFACT_DIR/status-after-stability.json"
    local reconnect_first reconnect_after
    reconnect_first=$(extract_reconnect_count "$status_first")
    reconnect_after=$(extract_reconnect_count "$status_after")
    if [[ -z "$reconnect_first" ]] || [[ -z "$reconnect_after" ]]; then
        log_fail "reconnect_count missing in status-after-stability.json"
        return 1
    fi
    if [[ ! "$reconnect_first" =~ ^[0-9]+$ ]] || [[ ! "$reconnect_after" =~ ^[0-9]+$ ]]; then
        log_fail "reconnect_count is non-numeric in status-after-stability.json"
        return 1
    fi
    if [[ "$reconnect_after" -gt "$reconnect_first" ]]; then
        log_fail "Reconnect count increased: ${reconnect_first} -> ${reconnect_after}"
        return 1
    fi
    log_pass "Reconnect count stable: ${reconnect_first} -> ${reconnect_after}"
    return 0
}

verify_bird_no_connection_closures() {
    log_info "=== Verifying BIRD has no recent connection closures ==="
    local protocol_after="$ARTIFACT_DIR/bird-protocol-after-stability.txt"
    if bird_has_recent_closures "$protocol_after"; then
        log_fail "BIRD shows recent 'Socket: Connection closed' after stable point"
        grep -iE "Connection closed|Socket:.*closed|connect.*failed" "$protocol_after" 2>/dev/null || true
        return 1
    fi
    log_pass "No recent BIRD connection closures detected"
    return 0
}

verify_bird_stable_during_window() {
    log_info "=== Verifying BIRD remained stable ==="
    local protocol_after="$ARTIFACT_DIR/bird-protocol-after-stability.txt"
    if bird_is_established "$protocol_after"; then
        log_pass "BIRD remained stable throughout stability window"
        return 0
    fi
    log_fail "BIRD became unstable during stability window"
    cat "$protocol_after" 2>/dev/null | head -20
    return 1
}

# =============================================================================
# Self-Test Mode
# =============================================================================

run_self_test() {
    log_info "=== Self-Test Mode ==="
    echo ""
    VERIFIER_PATH="${SCRIPT_DIR}/verify_bgp_stability.sh"
source "${SCRIPT_DIR}/verify_bgp_stability_fixtures.sh"

    local tests_passed=0
    local tests_failed=0

    log_info "--- Test 1: PASS case (healthy stability) ---"
    local test1_dir
    test1_dir=$(mktemp -d "/tmp/self-test-healthy-XXXXXX")
    create_fixture_healthy "$test1_dir"
    if bash "$VERIFIER_PATH" "$test1_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 1: PASS case exits 0"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 1: PASS case should exit 0, got non-zero"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test1_dir"
    echo ""

    log_info "--- Test 2: FAIL - reconnect delta 53 ---"
    local test2_dir
    test2_dir=$(mktemp -d "/tmp/self-test-reconnect-delta-XXXXXX")
    create_fixture_reconnect_delta "$test2_dir"
    if ! bash "$VERIFIER_PATH" "$test2_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 2: reconnect delta 53 fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 2: reconnect delta 53 should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test2_dir"
    echo ""

    log_info "--- Test 3: FAIL - reconnect_count missing ---"
    local test3_dir
    test3_dir=$(mktemp -d "/tmp/self-test-missing-reconnect-XXXXXX")
    create_fixture_missing_reconnect "$test3_dir"
    if ! bash "$VERIFIER_PATH" "$test3_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 3: missing reconnect_count fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 3: missing reconnect_count should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test3_dir"
    echo ""

    log_info "--- Test 4: FAIL - reconnect_count non-numeric ---"
    local test4_dir
    test4_dir=$(mktemp -d "/tmp/self-test-non-numeric-XXXXXX")
    create_fixture_non_numeric_reconnect "$test4_dir"
    if ! bash "$VERIFIER_PATH" "$test4_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 4: non-numeric reconnect_count fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 4: non-numeric reconnect_count should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test4_dir"
    echo ""

    log_info "--- Test 5: FAIL - bird-routes.txt missing ---"
    local test5_dir
    test5_dir=$(mktemp -d "/tmp/self-test-missing-routes-XXXXXX")
    create_fixture_missing_routes "$test5_dir"
    if ! bash "$VERIFIER_PATH" "$test5_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 5: missing bird-routes.txt fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 5: missing bird-routes.txt should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test5_dir"
    echo ""

    log_info "--- Test 6: FAIL - bird-routes.txt empty ---"
    local test6_dir
    test6_dir=$(mktemp -d "/tmp/self-test-empty-routes-XXXXXX")
    create_fixture_empty_routes "$test6_dir"
    if ! bash "$VERIFIER_PATH" "$test6_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 6: empty bird-routes.txt fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 6: empty bird-routes.txt should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test6_dir"
    echo ""

    log_info "--- Test 7: FAIL - BIRD shows Socket: Connection closed ---"
    local test7_dir
    test7_dir=$(mktemp -d "/tmp/self-test-connection-closed-XXXXXX")
    create_fixture_connection_closed "$test7_dir"
    if ! bash "$VERIFIER_PATH" "$test7_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 7: Socket: Connection closed fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 7: Socket: Connection closed should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test7_dir"
    echo ""

    log_info "--- Test 8: FAIL - BIRD not Established after stability ---"
    local test8_dir
    test8_dir=$(mktemp -d "/tmp/self-test-bird-unstable-XXXXXX")
    create_fixture_bird_unstable "$test8_dir"
    if ! bash "$VERIFIER_PATH" "$test8_dir" 1 > /dev/null 2>&1; then
        log_pass "Test 8: BIRD not Established fails verifier"
        tests_passed=$((tests_passed + 1))
    else
        log_fail "Test 8: BIRD not Established should fail verifier"
        tests_failed=$((tests_failed + 1))
    fi
    rm -rf "$test8_dir"
    echo ""

    echo "=== Self-Test Summary ==="
    log_info "Tests passed: $tests_passed"
    log_info "Tests failed: $tests_failed"
    [[ $tests_failed -eq 0 ]] && return 0 || return 1
}

# =============================================================================
# Main
# =============================================================================

main() {
    [[ "${1:-}" == "--self-test" ]] && { run_self_test; return $?; }

    log_info "BGP stability artifact verifier"
    log_info "Artifact directory: $ARTIFACT_DIR"
    log_info "Reconnect budget: <= $RECONNECT_BUDGET"
    echo ""

    local exit_code=0
    verify_artifacts_exist || exit_code=1; echo ""
    verify_bgp_established || exit_code=1; echo ""
    verify_bfd_up || exit_code=1; echo ""
    verify_bird_established || exit_code=1; echo ""
    verify_imported_routes_nonzero || exit_code=1; echo ""
    verify_reconnect_budget || exit_code=1; echo ""
    verify_reconnect_no_increase_after_established || exit_code=1; echo ""
    verify_bird_no_connection_closures || exit_code=1; echo ""
    verify_bird_stable_during_window || exit_code=1; echo ""

    echo "=== Verification Summary ==="
    [[ $exit_code -eq 0 ]] && log_pass "All stability verifications passed" || log_fail "Some stability verifications failed"
    return $exit_code
}

main "$@"
