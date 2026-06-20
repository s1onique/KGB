#!/bin/bash
# verify_uvb76_captured_row_assertion_tests.sh — Self-test for captured row assertion
#
# Tests the captured row assertion regression.
# Sourced by verify_uvb76_capture_helpers.sh.
#
# Usage:
#   ./verify_uvb76_captured_row_assertion_tests.sh [--verbose]
#   ./verify_uvb76_captured_row_assertion_tests.sh --self-test

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

# Source the normalizers and fetch helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_capture_poll.sh"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract_normalizers.sh"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_fetch_helpers.sh"

# Mock log functions for self-test
log_info() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[INFO] $*" || true; }
log_warn() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[WARN] $*" || true; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# =============================================================================
# Test: assert_captured_row_contract (regression test for normalized row shape)
# =============================================================================

run_captured_row_assertion_tests() {
    echo ""
    echo "=== Testing assert_captured_row_contract regression ==="
    
    # Source contract helpers (need log functions)
    source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract.sh" 2>/dev/null || true
    
    local test_dir
    test_dir=$(mktemp -d "/tmp/uvb76-captured-assert-test-XXXXXX")
    
    # Test 1: Raw row with capture_status="captured" should normalize to captured
    # This is the shape that comes from the API (spike row with captures[0].capture_status)
    echo "Test 1: raw capture_status=\"captured\" normalizes to captured"
    cat > "$test_dir/raw-captured.json" <<'EOF'
{
  "event_id": "evt-test",
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
    # Create a mock packet file
    cat > "$test_dir/packet.json" <<'EOF'
{
  "phase": "phase1",
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}
EOF
    # Test the normalization produces the right shape
    local norm_output="$test_dir/norm-output.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-captured.json" "$norm_output" 2>/dev/null; then
        local cs ce ip ci
        cs=$(jq -r '.capture_status' "$norm_output" 2>/dev/null)
        ce=$(jq -r '.capture_exists' "$norm_output" 2>/dev/null)
        ip=$(jq -r '.is_protected' "$norm_output" 2>/dev/null)
        ci=$(jq -r '.cooldown_info' "$norm_output" 2>/dev/null)
        if [[ "$cs" == "captured" && "$ce" == "true" && "$ip" == "true" && "$ci" == "null" ]]; then
            log_pass "Normalized row: capture_status=$cs, capture_exists=$ce, is_protected=$ip, cooldown_info=$ci"
        else
            log_fail "Normalized row: expected captured/true/true/null, got $cs/$ce/$ip/$ci"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 1"
    fi
    
    # Test 2: raw capture_status="ok" -> normalized captured (regression: ok->captured)
    echo "Test 2: raw capture_status=\"ok\" should normalize to \"captured\""
    cat > "$test_dir/raw-ok.json" <<'EOF'
{
  "event_id": "evt-test",
  "captures": [
    {
      "capture_status": "ok",
      "status": "ok",
      "capture_exists": true,
      "is_protected": true
    }
  ]
}
EOF
    norm_output="$test_dir/norm-ok.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-ok.json" "$norm_output" 2>/dev/null; then
        cs=$(jq -r '.capture_status' "$norm_output" 2>/dev/null)
        if [[ "$cs" == "captured" ]]; then
            log_pass "raw capture_status=\"ok\" -> normalized=\"$cs\" (expected: captured)"
        else
            log_fail "raw capture_status=\"ok\" -> normalized=\"$cs\" (expected: captured)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 2"
    fi
    
    # Test 3: Verify NO "unknown" result for captured rows (the reported bug)
    echo "Test 3: captured rows should NEVER produce capture_status=unknown"
    cat > "$test_dir/raw-captured2.json" <<'EOF'
{
  "event_id": "evt-test",
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
    norm_output="$test_dir/norm-captured2.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-captured2.json" "$norm_output" 2>/dev/null; then
        cs=$(jq -r '.capture_status' "$norm_output" 2>/dev/null)
        if [[ "$cs" != "unknown" ]]; then
            log_pass "captured row capture_status=\"$cs\" (NOT unknown - regression fixed)"
        else
            log_fail "captured row produced capture_status=\"unknown\" (BUG: should be \"captured\")"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 3"
    fi
    
    # Test 4: Full chain - assert_captured_row_contract accepts normalized captured row
    # This is the KEY test that proves the full contract chain works end-to-end:
    # raw API row -> normalize_spike_row_capture_contract -> assert_captured_row_contract PASS
    echo "Test 4: assert_captured_row_contract accepts normalized captured row (full chain)"
    cat > "$test_dir/raw-fullchain.json" <<'EOF'
{
  "event_id": "evt-fullchain",
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
    norm_output="$test_dir/norm-fullchain.json"
    if normalize_spike_row_capture_contract "$test_dir/raw-fullchain.json" "$norm_output" 2>/dev/null; then
        # Create a valid packet file for the assertion
        cat > "$test_dir/packet-fullchain.json" <<'EOF'
{
  "phase": "phase1",
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}
EOF
        if assert_captured_row_contract 1 "$norm_output" "$test_dir/packet-fullchain.json" 2>/dev/null; then
            log_pass "assert_captured_row_contract accepts normalized captured row (full chain verified)"
        else
            log_fail "assert_captured_row_contract rejected normalized captured row (BUG in assertion)"
        fi
    else
        log_fail "normalize_spike_row_capture_contract failed for test 4"
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
                echo "  -h, --help   Show this help"
                exit 0
                ;;
            *) echo "Unknown: $1"; exit 1 ;;
        esac
    done
    
    if [[ "$do_verbose" == "true" ]]; then
        VERBOSE=true
    fi
    
    run_captured_row_assertion_tests
    
    echo ""
    echo "=== Captured Row Assertion Tests Summary ==="
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
