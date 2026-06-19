#!/bin/bash
# verify_uvb76_row_normalization_tests.sh — Self-test for normalize_spike_row_capture_contract
#
# Tests the row normalization function to ensure it uses the same extraction
# logic as wait_for_spike_capture_after_event.
#
# Usage:
#   ./verify_uvb76_row_normalization_tests.sh [--verbose]
#   ./verify_uvb76_row_normalization_tests.sh --self-test

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

VERBOSE="${VERBOSE:-false}"
ERRORS=0
PASSES=0

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; PASSES=$((PASSES + 1)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $*" >&2; ERRORS=$((ERRORS + 1)); }
log_info() { [[ "$VERBOSE" == "true" ]] && echo "[INFO] $*" || true; }

# Source the normalizers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract_normalizers.sh"

# Mock log functions for self-test
log_info() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[INFO] $*" || true; }
log_warn() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[WARN] $*" || true; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# =============================================================================
# Test: normalize_spike_row_capture_contract
# =============================================================================

run_row_normalization_tests() {
    echo ""
    echo "=== Testing normalize_spike_row_capture_contract ==="
    
    local test_dir
    test_dir=$(mktemp -d "/tmp/uvb76-row-norm-test-XXXXXX")
    
    # Test 1: raw capture_status="" + status="ok" -> normalized capture_status="captured"
    echo "Test 1: capture_status=\"\" + status=\"ok\" -> captured"
    cat > "$test_dir/raw-status-ok.json" <<'EOF'
{
  "event_id": "evt-001",
  "captures": [
    {
      "capture_status": "",
      "status": "ok",
      "capture_exists": true,
      "is_protected": true
    }
  ]
}
EOF
    local output="$test_dir/norm-status-ok.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-status-ok.json" "$output" 2>/dev/null; then
        local result
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "captured" ]]; then
            log_pass "capture_status=\"\" + status=\"ok\" -> normalized=\"$result\" (expected: captured)"
        else
            log_fail "capture_status=\"\" + status=\"ok\" -> normalized=\"$result\" (expected: captured)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 1"
    fi
    
    # Test 2: raw capture_status="captured" + status="ok" -> captured
    echo "Test 2: capture_status=\"captured\" + status=\"ok\" -> captured"
    cat > "$test_dir/raw-captured.json" <<'EOF'
{
  "event_id": "evt-002",
  "captures": [
    {
      "capture_status": "captured",
      "status": "ok",
      "capture_exists": true,
      "is_protected": true
    }
  ]
}
EOF
    output="$test_dir/norm-captured.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-captured.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "captured" ]]; then
            log_pass "capture_status=\"captured\" -> normalized=\"$result\" (expected: captured)"
        else
            log_fail "capture_status=\"captured\" -> normalized=\"$result\" (expected: captured)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 2"
    fi
    
    # Test 3: raw capture_status="" + status="timeout" -> failed
    echo "Test 3: capture_status=\"\" + status=\"timeout\" -> failed"
    cat > "$test_dir/raw-timeout.json" <<'EOF'
{
  "event_id": "evt-003",
  "captures": [
    {
      "capture_status": "",
      "status": "timeout",
      "capture_exists": false,
      "is_protected": false
    }
  ]
}
EOF
    output="$test_dir/norm-timeout.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-timeout.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "failed" ]]; then
            log_pass "capture_status=\"\" + status=\"timeout\" -> normalized=\"$result\" (expected: failed)"
        else
            log_fail "capture_status=\"\" + status=\"timeout\" -> normalized=\"$result\" (expected: failed)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 3"
    fi
    
    # Test 4: raw capture_status="skipped_cooldown" + status="ok" -> skipped_cooldown
    echo "Test 4: capture_status=\"skipped_cooldown\" + status=\"ok\" -> skipped_cooldown"
    cat > "$test_dir/raw-skipped.json" <<'EOF'
{
  "event_id": "evt-004",
  "captures": [
    {
      "capture_status": "skipped_cooldown",
      "status": "ok",
      "capture_exists": false,
      "is_protected": false,
      "cooldown_info": {
        "last_successful_capture_at": "2026-01-01T00:00:00Z",
        "next_capture_eligible_at": "2026-01-01T00:10:00Z"
      }
    }
  ]
}
EOF
    output="$test_dir/norm-skipped.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-skipped.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "skipped_cooldown" ]]; then
            log_pass "capture_status=\"skipped_cooldown\" -> normalized=\"$result\" (expected: skipped_cooldown)"
        else
            log_fail "capture_status=\"skipped_cooldown\" -> normalized=\"$result\" (expected: skipped_cooldown)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 4"
    fi
    
    # Test 5: missing captures[] -> not_attempted
    echo "Test 5: missing captures[] -> not_attempted"
    cat > "$test_dir/raw-no-captures.json" <<'EOF'
{
  "event_id": "evt-005"
}
EOF
    output="$test_dir/norm-no-captures.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-no-captures.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        local cap_exists
        cap_exists=$(jq -r '.capture_exists' "$output" 2>/dev/null)
        if [[ "$result" == "not_attempted" && "$cap_exists" == "false" ]]; then
            log_pass "missing captures -> normalized=\"$result\", capture_exists=$cap_exists (expected: not_attempted, false)"
        else
            log_fail "missing captures -> normalized=\"$result\", capture_exists=$cap_exists (expected: not_attempted, false)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 5"
    fi
    
    # Test 6: empty captures[] -> not_attempted
    echo "Test 6: empty captures[] -> not_attempted"
    cat > "$test_dir/raw-empty-captures.json" <<'EOF'
{
  "event_id": "evt-006",
  "captures": []
}
EOF
    output="$test_dir/norm-empty-captures.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-empty-captures.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "not_attempted" ]]; then
            log_pass "empty captures[] -> normalized=\"$result\" (expected: not_attempted)"
        else
            log_fail "empty captures[] -> normalized=\"$result\" (expected: not_attempted)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 6"
    fi
    
    # Test 7: capture_status="disabled" -> disabled
    echo "Test 7: capture_status=\"disabled\" -> disabled"
    cat > "$test_dir/raw-disabled.json" <<'EOF'
{
  "event_id": "evt-007",
  "captures": [
    {
      "capture_status": "disabled",
      "status": "ok"
    }
  ]
}
EOF
    output="$test_dir/norm-disabled.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-disabled.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "disabled" ]]; then
            log_pass "capture_status=\"disabled\" -> normalized=\"$result\" (expected: disabled)"
        else
            log_fail "capture_status=\"disabled\" -> normalized=\"$result\" (expected: disabled)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 7"
    fi
    
    # Test 8: status="error" -> failed
    echo "Test 8: capture_status=\"\" + status=\"error\" -> failed"
    cat > "$test_dir/raw-error.json" <<'EOF'
{
  "event_id": "evt-008",
  "captures": [
    {
      "capture_status": "",
      "status": "error"
    }
  ]
}
EOF
    output="$test_dir/norm-error.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-error.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        if [[ "$result" == "failed" ]]; then
            log_pass "capture_status=\"\" + status=\"error\" -> normalized=\"$result\" (expected: failed)"
        else
            log_fail "capture_status=\"\" + status=\"error\" -> normalized=\"$result\" (expected: failed)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 8"
    fi
    
    # Test 9: captured row without capture_exists/is_protected defaults to true
    echo "Test 9: captured row without fields defaults capture_exists=true, is_protected=true"
    cat > "$test_dir/raw-captured-no-fields.json" <<'EOF'
{
  "event_id": "evt-009",
  "captures": [
    {
      "capture_status": "",
      "status": "ok"
    }
  ]
}
EOF
    output="$test_dir/norm-captured-no-fields.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-captured-no-fields.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        local cap_exists=$(jq -r '.capture_exists' "$output" 2>/dev/null)
        local prot=$(jq -r '.is_protected' "$output" 2>/dev/null)
        if [[ "$result" == "captured" && "$cap_exists" == "true" && "$prot" == "true" ]]; then
            log_pass "captured+no-fields -> status=$result, capture_exists=$cap_exists, is_protected=$prot"
        else
            log_fail "captured+no-fields -> status=$result, capture_exists=$cap_exists, is_protected=$prot (expected: captured, true, true)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 9"
    fi
    
    # Test 10: failed row without capture_exists/is_protected defaults to false
    echo "Test 10: failed row without fields defaults capture_exists=false, is_protected=false"
    cat > "$test_dir/raw-failed-no-fields.json" <<'EOF'
{
  "event_id": "evt-010",
  "captures": [
    {
      "capture_status": "",
      "status": "timeout"
    }
  ]
}
EOF
    output="$test_dir/norm-failed-no-fields.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-failed-no-fields.json" "$output" 2>/dev/null; then
        result=$(jq -r '.capture_status' "$output" 2>/dev/null)
        local cap_exists=$(jq -r '.capture_exists' "$output" 2>/dev/null)
        local prot=$(jq -r '.is_protected' "$output" 2>/dev/null)
        if [[ "$result" == "failed" && "$cap_exists" == "false" && "$prot" == "false" ]]; then
            log_pass "failed+no-fields -> status=$result, capture_exists=$cap_exists, is_protected=$prot"
        else
            log_fail "failed+no-fields -> status=$result, capture_exists=$cap_exists, is_protected=$prot (expected: failed, false, false)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 10"
    fi
    
    # Cleanup
    rm -rf "$test_dir"
}

# =============================================================================
# Main
# =============================================================================

main() {
    local do_verbose=false
    local do_self_test=false
    
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --verbose) do_verbose=true; shift ;;
            --self-test) do_self_test=true; shift ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo "Options:"
                echo "  --verbose    Enable verbose output"
                echo "  --self-test  Run self-test"
                echo "  -h, --help  Show this help"
                exit 0
                ;;
            *) echo "Unknown: $1"; exit 1 ;;
        esac
    done
    
    if [[ "$do_verbose" == "true" ]]; then
        VERBOSE=true
    fi
    
    echo "=== UVB-76 Row Normalization Tests ==="
    echo ""
    
    run_row_normalization_tests
    
    echo ""
    echo "=== Self-test Summary ==="
    echo "Passed: $PASSES"
    echo "Failed: $ERRORS"
    
    if [[ $ERRORS -gt 0 ]]; then
        echo "SELF-TEST FAILED"
        return 1
    else
        echo "SELF-TEST PASSED"
        return 0
    fi
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
