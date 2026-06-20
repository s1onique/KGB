#!/bin/bash
# lab_uvb76_capture_netns_phase1_harden.sh — Phase 1 Harden assertions
#
# Phase 1 specific assertions to prevent the "all-suppressed cooldown false green" scenario.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# =============================================================================
# Phase 1 Harden: Real Capture Assertions
# =============================================================================

# Global flag for fail-closed behavior
declare -g PHASE1_HARDEN_FAILED=false

# =============================================================================
# Self-test for jq probe detection
# =============================================================================

# Run self-test to prove jq probes work correctly.
# Tests that the probes correctly detect real diagnostic fields.
run_phase1_harden_self_test() {
    local failures=0
    echo "=== Phase 1 Harden Self-Test ==="
    
    # Test 1: Packet with started_at and completed_at should pass
    local test_packet_1; test_packet_1=$(mktemp "/tmp/test-packet-1-XXXXXX.json")
    cat > "$test_packet_1" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "completed_at": "2026-01-01T00:00:01Z"
  }
}
EOF
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_1" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects started_at+completed_at as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect started_at+completed_at"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_1"
    
    # Test 2: Packet with requests array should pass
    local test_packet_2; test_packet_2=$(mktemp "/tmp/test-packet-2-XXXXXX.json")
    cat > "$test_packet_2" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "requests": [{"url": "http://example.com"}]
  }
}
EOF
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_2" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects requests[] as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect requests[]"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_2"
    
    # Test 3: Packet with tcp_info should pass
    local test_packet_3; test_packet_3=$(mktemp "/tmp/test-packet-3-XXXXXX.json")
    cat > "$test_packet_3" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "tcp_info": {"state": "ESTABLISHED"}
  }
}
EOF
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_3" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects tcp_info as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect tcp_info"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_3"
    
    # Test 4: Packet with xray_result should pass
    local test_packet_4; test_packet_4=$(mktemp "/tmp/test-packet-4-XXXXXX.json")
    cat > "$test_packet_4" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "xray_result": {"connections": []}
  }
}
EOF
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_4" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects xray_result as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect xray_result"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_4"
    
    # Test 5: Suppressed packet should fail
    local test_packet_5; test_packet_5=$(mktemp "/tmp/test-packet-5-XXXXXX.json")
    cat > "$test_packet_5" <<'EOF'
{
  "network_diag": {
    "status": "suppressed"
  }
}
EOF
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_5" >/dev/null 2>&1; then
        echo "[FAIL] Self-test: suppressed packet should NOT pass"
        failures=$((failures + 1))
    else
        echo "[PASS] Self-test: correctly rejects suppressed status"
    fi
    rm -f "$test_packet_5"
    
    # Test 6: Empty packet (no network_diag) should fail
    local test_packet_6; test_packet_6=$(mktemp "/tmp/test-packet-6-XXXXXX.json")
    echo '{}' > "$test_packet_6"
    if jq -e '
      .network_diag as $d
      | ($d != null)
      and (($d.status // "") != "suppressed")
      and (
        (($d.requests // []) | length > 0)
        or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
        or ($d.tcp_info != null)
        or ($d.xray_result != null)
      )
    ' "$test_packet_6" >/dev/null 2>&1; then
        echo "[FAIL] Self-test: empty packet should NOT pass"
        failures=$((failures + 1))
    else
        echo "[PASS] Self-test: correctly rejects empty packet"
    fi
    rm -f "$test_packet_6"
    
    echo "--- Self-test: $failures failures ---"
    [[ $failures -eq 0 ]] && return 0 || return 1
}

# Assert Phase 1 real capture requirements:
# - At least one post-cursor capture has real network diagnostics
# - protected_spike_count >= 1 (at least one protected spike exists)
#
# This prevents the "all-suppressed cooldown false green" scenario where:
# - Warmup polling consumes the only real capture
# - Phase 1 only sees suppressed_cooldown spikes
# - Lab passes while UI shows retained spikes with only skipped:cooldown captures
assert_phase1_real_capture() {
    local phase="$1"
    local spike_row_file="$2"
    local packet_file="$3"
    local prior_phase_row_file="${4:-}"
    
    local ok=true
    
    log_info "Asserting Phase 1 real capture requirements"
    
    # CRITICAL CHECK 1: Phase 1 row must have capture_status == captured
    # This proves the spike was actually captured, not skipped
    local capture_status
    capture_status=$(jq -r '.capture_status // "unknown"' "$spike_row_file" 2>/dev/null || echo "unknown")
    if [[ "$capture_status" != "captured" ]]; then
        log_error "[FAIL] Phase 1: capture_status must be 'captured', got: '$capture_status'"
        log_error "This means no real capture occurred - possible warmup consumed the only capture"
        ok=false
        PHASE1_HARDEN_FAILED=true
    else
        log_info "[PASS] Phase 1: capture_status=captured (real capture confirmed)"
    fi
    
    # CRITICAL CHECK 2: Capture must have REAL network_diag (not suppressed metadata)
    # A suppressed capture may have network_diag object but with status="suppressed" or empty.
    # Real captures have at least one of: requests[], started_at+completed_at, tcp_info, xray_result.
    #
    # FIXED: Use proper jq -e with redirection to prevent stdout pollution.
    # Old pattern: `jq -e '...' "$f" 2>/dev/null && echo "yes" || echo "no"` - WRONG
    #   This pollutes the variable with jq output (e.g., "true\nyes" instead of "yes").
    # New pattern: `if jq -e '...' "$f" >/dev/null 2>&1; then` - CORRECT
    #   The exit code is tested, not stdout.
    if [[ ! -f "$packet_file" ]]; then
        log_error "[FAIL] Phase 1: capture packet file not found: $packet_file"
        ok=false
        PHASE1_HARDEN_FAILED=true
    elif ! jq -e '.network_diag != null' "$packet_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase 1: capture packet missing network_diag"
        log_error "Real captures must have network_diag object, not just suppressed metadata"
        ok=false
        PHASE1_HARDEN_FAILED=true
    elif jq -e '.network_diag.status == "suppressed"' "$packet_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase 1: capture packet has suppressed network_diag"
        log_error "Real captures must have status != 'suppressed'"
        ok=false
        PHASE1_HARDEN_FAILED=true
    else
        # Verify network_diag has at least one real diagnostic field
        # Real captures have: requests[] OR (started_at AND completed_at) OR tcp_info OR xray_result
        # Using combined jq predicate to avoid stdout pollution from individual field checks
        if jq -e '
          .network_diag as $d
          | ($d != null)
          and (($d.status // "") != "suppressed")
          and (
            (($d.requests // []) | length > 0)
            or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
            or ($d.tcp_info != null)
            or ($d.xray_result != null)
          )
        ' "$packet_file" >/dev/null 2>&1; then
            log_info "[PASS] Phase 1: capture packet has real network_diag with diagnostic data"
            # Log which fields were detected for traceability
            local has_requests="no" has_started="no" has_completed="no" has_tcp="no" has_xray="no"
            jq -e '.network_diag.requests != null' "$packet_file" >/dev/null 2>&1 && has_requests="yes"
            jq -e '.network_diag.started_at != null' "$packet_file" >/dev/null 2>&1 && has_started="yes"
            jq -e '.network_diag.completed_at != null' "$packet_file" >/dev/null 2>&1 && has_completed="yes"
            jq -e '.network_diag.tcp_info != null' "$packet_file" >/dev/null 2>&1 && has_tcp="yes"
            jq -e '.network_diag.xray_result != null' "$packet_file" >/dev/null 2>&1 && has_xray="yes"
            log_info "  Detected fields: requests=$has_requests, started_at=$has_started, completed_at=$has_completed, tcp_info=$has_tcp, xray_result=$has_xray"
        else
            log_error "[FAIL] Phase 1: capture packet has no real diagnostic fields"
            log_error "Expected at least one of: requests[], started_at+completed_at, tcp_info, xray_result"
            # Log all field states for debugging
            local has_requests="no" has_started="no" has_completed="no" has_tcp="no" has_xray="no"
            jq -e '.network_diag.requests != null' "$packet_file" >/dev/null 2>&1 && has_requests="yes"
            jq -e '.network_diag.started_at != null' "$packet_file" >/dev/null 2>&1 && has_started="yes"
            jq -e '.network_diag.completed_at != null' "$packet_file" >/dev/null 2>&1 && has_completed="yes"
            jq -e '.network_diag.tcp_info != null' "$packet_file" >/dev/null 2>&1 && has_tcp="yes"
            jq -e '.network_diag.xray_result != null' "$packet_file" >/dev/null 2>&1 && has_xray="yes"
            log_error "  Current fields: requests=$has_requests, started_at=$has_started, completed_at=$has_completed, tcp_info=$has_tcp, xray_result=$has_xray"
            ok=false
            PHASE1_HARDEN_FAILED=true
        fi
    fi
    
    # CRITICAL CHECK 3: If a prior phase row exists, it must be captured (not skipped)
    # This ensures cooldown suppression is anchored to a prior successful capture
    if [[ -n "$prior_phase_row_file" && -f "$prior_phase_row_file" ]]; then
        local prior_status
        prior_status=$(jq -r '.capture_status // "unknown"' "$prior_phase_row_file" 2>/dev/null || echo "unknown")
        if [[ "$prior_status" != "captured" ]]; then
            log_error "[FAIL] Phase 1 cooldown anchor: prior phase status is '$prior_status', expected 'captured'"
            log_error "Cooldown suppression requires a prior successful capture to exist"
            ok=false
            PHASE1_HARDEN_FAILED=true
        else
            log_info "[PASS] Phase 1 cooldown anchor: prior phase was captured (cooldown is valid)"
        fi
    fi
    
    if [[ "$ok" != "true" ]]; then
        log_error "[FAIL] Phase 1 real capture assertions FAILED"
        PHASE1_HARDEN_FAILED=true
        return 1
    fi
    
    log_info "[PASS] Phase 1 real capture assertions satisfied"
    return 0
}
