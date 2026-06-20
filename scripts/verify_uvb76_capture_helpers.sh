#!/bin/bash
# verify_uvb76_capture_helpers.sh — Self-test for capture helper functions
#
# Tests normalize_capture_status and save_phase_capture_packet_from_raw_row
# using the same fixture patterns as verify_uvb76_diag_packet_contract.sh
#
# Usage:
#   ./verify_uvb76_capture_helpers.sh [--verbose]
#   ./verify_uvb76_capture_helpers.sh --self-test

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

# Source the capture poll helpers for normalize_capture_status
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_capture_poll.sh"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract_normalizers.sh"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_cooldown_helpers.sh"

# Mock log functions for self-test
log_info() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[INFO] $*" || true; }
log_warn() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[WARN] $*" || true; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# =============================================================================
# Test: normalize_capture_status
# =============================================================================

test_normalize_capture_status() {
    local input="$1"
    local expected="$2"
    local description="$3"
    
    local result
    result=$(normalize_capture_status "$input")
    
    if [[ "$result" == "$expected" ]]; then
        log_pass "normalize_capture_status($input) = $result (expected: $expected)"
    else
        log_fail "normalize_capture_status($input) = $result (expected: $expected)"
    fi
}

run_normalize_capture_status_tests() {
    echo "=== Testing normalize_capture_status ==="
    
    # ok -> captured
    test_normalize_capture_status "ok" "captured" "ok maps to captured"
    
    # captured -> captured
    test_normalize_capture_status "captured" "captured" "captured maps to captured"
    
    # timeout -> failed
    test_normalize_capture_status "timeout" "failed" "timeout maps to failed"
    
    # error -> failed
    test_normalize_capture_status "error" "failed" "error maps to failed"
    
    # failed -> failed
    test_normalize_capture_status "failed" "failed" "failed maps to failed"
    
    # skipped_cooldown -> skipped_cooldown
    test_normalize_capture_status "skipped_cooldown" "skipped_cooldown" "skipped_cooldown maps to skipped_cooldown"
    
    # disabled -> disabled
    test_normalize_capture_status "disabled" "disabled" "disabled maps to disabled"
    
    # not_configured -> not_configured
    test_normalize_capture_status "not_configured" "not_configured" "not_configured maps to not_configured"
    
    # not_attempted -> not_attempted
    test_normalize_capture_status "not_attempted" "not_attempted" "not_attempted maps to not_attempted"
    
    # in_progress -> pending
    test_normalize_capture_status "in_progress" "pending" "in_progress maps to pending"
    
    # pending -> pending
    test_normalize_capture_status "pending" "pending" "pending maps to pending"
    
    # empty -> pending
    test_normalize_capture_status "" "pending" "empty maps to pending"
    
    # EXTRACTION SELF-TEST: Full JSON extraction rule
    # Test case 1: capture_status="" + status="timeout" -> extraction gives "timeout" -> normalized "failed"
    # Empty .capture_status is treated as absent, falls back to .status
    local cap_json='{"capture_status": "", "status": "timeout"}'
    local extracted
    extracted=$(echo "$cap_json" | jq -r '.capture_status // empty' 2>/dev/null)
    if [[ -z "$extracted" ]]; then
        # No capture_status -> fall back to status
        extracted=$(echo "$cap_json" | jq -r '.status // "unknown"' 2>/dev/null)
    fi
    local expected="failed"
    local actual
    actual=$(normalize_capture_status "$extracted")
    if [[ "$actual" == "$expected" ]]; then
        log_pass "EXTRACTION: capture_status=\"\" + status=\"timeout\" -> normalized=\"$actual\" (expected: $expected)"
    else
        log_fail "EXTRACTION: capture_status=\"\" + status=\"timeout\" -> normalized=\"$actual\" (expected: $expected)"
    fi
    
    # Test case 2: capture_status="skipped_cooldown" + status="ok" -> extraction gives "skipped_cooldown"
    cap_json='{"capture_status": "skipped_cooldown", "status": "ok"}'
    extracted=$(echo "$cap_json" | jq -r '.capture_status // empty' 2>/dev/null)
    if [[ -z "$extracted" ]]; then
        extracted=$(echo "$cap_json" | jq -r '.status // "unknown"' 2>/dev/null)
    fi
    expected="skipped_cooldown"
    actual=$(normalize_capture_status "$extracted")
    if [[ "$actual" == "$expected" ]]; then
        log_pass "EXTRACTION: capture_status=\"skipped_cooldown\" + status=\"ok\" -> normalized=\"$actual\" (expected: $expected)"
    else
        log_fail "EXTRACTION: capture_status=\"skipped_cooldown\" + status=\"ok\" -> normalized=\"$actual\" (expected: $expected)"
    fi
    
    # unknown -> unknown
    test_normalize_capture_status "unknown" "unknown" "unknown maps to unknown"
    
    # random -> unknown
    test_normalize_capture_status "random" "unknown" "random maps to unknown"
}

# =============================================================================
# Test: save_phase_capture_packet_from_raw_row
# =============================================================================

run_packet_extraction_tests() {
    echo ""
    echo "=== Testing save_phase_capture_packet_from_raw_row ==="
    
    local test_dir
    test_dir=$(mktemp -d "/tmp/uvb76-helper-test-XXXXXX")
    
    # Test 1: Direct .captures[].network_diag
    echo "Testing direct .captures[].network_diag..."
    cat > "$test_dir/raw-direct.json" <<'EOF'
{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "ok",
      "network_diag": {
        "status": "ok",
        "started_at": "2026-01-01T00:00:00Z"
      }
    }
  ]
}
EOF
    local output="$test_dir/packet-direct.json"
    if save_phase_capture_packet_from_raw_row "1" "$output" "$test_dir/raw-direct.json" 2>/dev/null; then
        if jq -e '.network_diag != null' "$output" >/dev/null 2>&1; then
            log_pass "Direct .captures[].network_diag extraction passed"
        else
            log_fail "Direct .captures[].network_diag extraction failed"
        fi
    else
        log_fail "Direct .captures[].network_diag extraction failed"
    fi
    
    # Test 2: .captures[].packet.network_diag
    echo "Testing .captures[].packet.network_diag..."
    cat > "$test_dir/raw-packet.json" <<'EOF'
{
  "event_id": "evt-456",
  "captures": [
    {
      "status": "ok",
      "packet": {
        "network_diag": {
          "status": "ok",
          "started_at": "2026-01-01T00:00:00Z"
        }
      }
    }
  ]
}
EOF
    output="$test_dir/packet-packet.json"
    if save_phase_capture_packet_from_raw_row "2" "$output" "$test_dir/raw-packet.json" 2>/dev/null; then
        if jq -e '.network_diag != null' "$output" >/dev/null 2>&1; then
            log_pass ".captures[].packet.network_diag extraction passed"
        else
            log_fail ".captures[].packet.network_diag extraction failed"
        fi
    else
        log_fail ".captures[].packet.network_diag extraction failed"
    fi
    
    # Test 3: .captures[].diagnostics.network_diag
    echo "Testing .captures[].diagnostics.network_diag..."
    cat > "$test_dir/raw-diagnostics.json" <<'EOF'
{
  "event_id": "evt-789",
  "captures": [
    {
      "status": "ok",
      "diagnostics": {
        "network_diag": {
          "status": "ok",
          "started_at": "2026-01-01T00:00:00Z"
        }
      }
    }
  ]
}
EOF
    output="$test_dir/packet-diagnostics.json"
    if save_phase_capture_packet_from_raw_row "3" "$output" "$test_dir/raw-diagnostics.json" 2>/dev/null; then
        if jq -e '.network_diag != null' "$output" >/dev/null 2>&1; then
            log_pass ".captures[].diagnostics.network_diag extraction passed"
        else
            log_fail ".captures[].diagnostics.network_diag extraction failed"
        fi
    else
        log_fail ".captures[].diagnostics.network_diag extraction failed"
    fi
    
    # Test 4: Missing network_diag should fail
    echo "Testing missing network_diag (should fail)..."
    cat > "$test_dir/raw-no-diag.json" <<'EOF'
{
  "event_id": "evt-000",
  "captures": [
    {
      "status": "ok",
      "network_diag": null
    }
  ]
}
EOF
    output="$test_dir/packet-no-diag.json"
    if save_phase_capture_packet_from_raw_row "4" "$output" "$test_dir/raw-no-diag.json" 2>/dev/null; then
        log_fail "Missing network_diag should have failed"
    else
        log_pass "Missing network_diag correctly failed"
    fi
    
    # Test 5: No captures array should fail
    echo "Testing no captures array (should fail)..."
    cat > "$test_dir/raw-no-captures.json" <<'EOF'
{
  "event_id": "evt-111"
}
EOF
    output="$test_dir/packet-no-captures.json"
    if save_phase_capture_packet_from_raw_row "5" "$output" "$test_dir/raw-no-captures.json" 2>/dev/null; then
        log_fail "No captures should have failed"
    else
        log_pass "No captures correctly failed"
    fi
    
    # Test 6: Missing arguments should fail gracefully
    echo "Testing missing arguments (should fail gracefully)..."
    if save_phase_capture_packet_from_raw_row "" "" "" 2>/dev/null; then
        log_fail "Missing arguments should have failed"
    else
        log_pass "Missing arguments correctly failed"
    fi
    
    # Test 7: Root .network_diag
    echo "Testing root .network_diag..."
    cat > "$test_dir/raw-root.json" <<'EOF'
{
  "event_id": "evt-222",
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}
EOF
    output="$test_dir/packet-root.json"
    if save_phase_capture_packet_from_raw_row "7" "$output" "$test_dir/raw-root.json" 2>/dev/null; then
        if jq -e '.network_diag != null' "$output" >/dev/null 2>&1; then
            log_pass "Root .network_diag extraction passed"
        else
            log_fail "Root .network_diag extraction failed"
        fi
    else
        log_fail "Root .network_diag extraction failed"
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
                echo "  --self-test   Run self-test"
                echo "  -h, --help   Show this help"
                exit 0
                ;;
            *) echo "Unknown: $1"; exit 1 ;;
        esac
    done
    
    if [[ "$do_verbose" == "true" ]]; then
        VERBOSE=true
    fi
    
    echo "=== UVB-76 Capture Helpers Self-Test ==="
    echo ""
    
    run_normalize_capture_status_tests
    
    # Run row normalization tests from split file (LLM-friendly: under 450 lines)
    # Use bash to avoid permission issues if executable bit is not set
    echo ""
    if ! bash "${SCRIPT_DIR}/verify_uvb76_row_normalization_tests.sh" --self-test; then
        ERRORS=$((ERRORS + 1))
    fi
    
    # Run captured row assertion tests from split file (LLM-friendly: under 450 lines)
    if ! bash "${SCRIPT_DIR}/verify_uvb76_captured_row_assertion_tests.sh" --self-test; then
        ERRORS=$((ERRORS + 1))
    fi
    
    run_packet_extraction_tests
    
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
