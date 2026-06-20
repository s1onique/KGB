#!/bin/bash
# verify_uvb76_extract_network_diag_tests.sh — Tests for extract_network_diag_from_spike_row
#
# Tests that captured spike rows expose retrievable stored packet evidence.
# Usage: bash ./scripts/verify_uvb76_extract_network_diag_tests.sh

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

ERRORS=0
PASSES=0

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; PASSES=$((PASSES + 1)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $*" >&2; ERRORS=$((ERRORS + 1)); }
log_info() { echo "[INFO] $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_fetch_helpers.sh"

run_extract_network_diag_tests() {
    echo ""
    echo "=== Testing extract_network_diag_from_spike_row regression ==="
    
    local test_dir
    test_dir=$(mktemp -d "/tmp/uvb76-extract-network-diag-test-XXXXXX")
    
    # Test 1: Spike row with network_diag in captures[0].network_diag should extract
    echo "Test 1: captures[0].network_diag extracts successfully"
    cat > "$test_dir/spike-row-with-network-diag.json" <<'EOF'
{
  "event_id": "evt-test-stored-capture",
  "captures": [{
    "capture_status": "captured", "status": "ok", "capture_exists": true,
    "is_protected": true, "source": "tovarisch-lab",
    "requested_path": "/status.json?include=network_diag",
    "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z",
      "completed_at": "2026-01-01T00:00:01Z", "interfaces": [], "routes": [], "underlay_tcp": []}
  }]
}
EOF
    local packet_output="$test_dir/packet-output.json"
    local summary_output="$test_dir/summary-output.json"
    
    if extract_network_diag_from_spike_row "1" "$test_dir/spike-row-with-network-diag.json" \
        "$packet_output" "$summary_output" 2>/dev/null; then
        if jq -e '.network_diag != null' "$packet_output" >/dev/null 2>&1; then
            log_pass "Extracted packet contains network_diag"
        else
            log_fail "Extracted packet missing network_diag"
        fi
        
        # Note: jq -r outputs booleans as literal "false" or "true"
        local is_fallback
        is_fallback=$(jq -r '.is_fallback' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$is_fallback" == "false" ]]; then
            log_pass "Summary shows is_fallback=false (stored artifact used)"
        else
            log_fail "Summary shows is_fallback=$is_fallback (expected: false)"
        fi
        
        local summary_source
        summary_source=$(jq -r '.summary_source // "null"' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$summary_source" == "stored_spike_row_capture" ]]; then
            log_pass "Summary summary_source is 'stored_spike_row_capture'"
        else
            log_fail "Summary summary_source is '$summary_source'"
        fi
        
        local capture_source
        capture_source=$(jq -r '.capture_source // "null"' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$capture_source" == "tovarisch-lab" ]]; then
            log_pass "Summary capture_source is 'tovarisch-lab'"
        else
            log_fail "Summary capture_source is '$capture_source'"
        fi
    else
        log_fail "extract_network_diag_from_spike_row failed"
    fi
    
    # Test 2: captures[0].packet.network_diag (alternative location)
    echo "Test 2: captures[0].packet.network_diag extracts successfully"
    cat > "$test_dir/spike-row-packet-location.json" <<'EOF'
{
  "event_id": "evt-test-packet-location",
  "captures": [{
    "capture_status": "captured", "status": "ok", "source": "tovarisch-lab",
    "packet": {"network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z",
      "interfaces": [], "routes": []}}
  }]
}
EOF
    packet_output="$test_dir/packet-packet-location.json"
    summary_output="$test_dir/summary-packet-location.json"
    
    if extract_network_diag_from_spike_row "1" "$test_dir/spike-row-packet-location.json" \
        "$packet_output" "$summary_output" 2>/dev/null; then
        if jq -e '.network_diag != null' "$packet_output" >/dev/null 2>&1; then
            log_pass "Extracted packet from .captures[0].packet.network_diag"
        else
            log_fail "Failed to extract from .captures[0].packet.network_diag"
        fi
    else
        log_fail "extract_network_diag_from_spike_row failed for packet location"
    fi
    
    # Test 3: Spike row without network_diag should fail
    echo "Test 3: spike row without network_diag fails with clear error"
    cat > "$test_dir/spike-row-no-network-diag.json" <<'EOF'
{
  "event_id": "evt-test-no-network-diag",
  "captures": [{"capture_status": "captured", "status": "ok", "source": "tovarisch-lab"}]
}
EOF
    packet_output="$test_dir/packet-no-network-diag.json"
    summary_output="$test_dir/summary-no-network-diag.json"
    
    if ! extract_network_diag_from_spike_row "1" "$test_dir/spike-row-no-network-diag.json" \
        "$packet_output" "$summary_output" 2>/dev/null; then
        log_pass "Correctly fails when no network_diag in spike row"
        local status
        status=$(jq -r '.status // "null"' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$status" == "no_network_diag_in_spike_row" ]]; then
            log_pass "Summary status is 'no_network_diag_in_spike_row'"
        else
            log_fail "Summary status is '$status'"
        fi
    else
        log_fail "Should have failed when no network_diag in spike row"
    fi
    
    # Test 4: Full chain - spike row exposes retrievable stored packet evidence
    echo "Test 4: Full chain - spike row exposes retrievable stored packet evidence"
    cat > "$test_dir/spike-row-full-chain.json" <<'EOF'
{
  "event_id": "evt-full-chain-test",
  "captures": [{
    "capture_status": "captured", "status": "ok", "capture_exists": true,
    "is_protected": true, "source": "tovarisch-lab",
    "requested_path": "/status.json?include=network_diag",
    "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z",
      "completed_at": "2026-01-01T00:00:01Z",
      "interfaces": [{"name": "eth0", "operstate": "up"}],
      "routes": [], "underlay_tcp": []}
  }]
}
EOF
    packet_output="$test_dir/packet-full-chain.json"
    summary_output="$test_dir/summary-full-chain.json"
    
    if extract_network_diag_from_spike_row "1" "$test_dir/spike-row-full-chain.json" \
        "$packet_output" "$summary_output" 2>/dev/null; then
        local has_interfaces
        has_interfaces=$(jq -r '.network_diag.interfaces | length' "$packet_output" 2>/dev/null || echo "0")
        if [[ "$has_interfaces" -gt 0 ]]; then
            log_pass "Full chain: packet contains real diagnostic data (interfaces)"
        else
            log_fail "Full chain: packet missing real diagnostic data"
        fi
        
        local event_id
        event_id=$(jq -r '.event_id // "null"' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$event_id" == "evt-full-chain-test" ]]; then
            log_pass "Full chain: event_id preserved in summary"
        else
            log_fail "Full chain: event_id is '$event_id'"
        fi
        
        local source
        source=$(jq -r '.capture_source // "null"' "$summary_output" 2>/dev/null || echo "null")
        if [[ "$source" == "tovarisch-lab" ]]; then
            log_pass "Full chain: capture_source preserved (tovarisch-lab)"
        else
            log_fail "Full chain: capture_source is '$source'"
        fi
    else
        log_fail "Full chain: extract_network_diag_from_spike_row failed"
    fi
    
    rm -rf "$test_dir"
}

main() {
    run_extract_network_diag_tests
    
    echo ""
    echo "=== Extract Network Diag Tests Summary ==="
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
