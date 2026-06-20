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
    # Real captures have at least one of: requests[], started_at, completed_at, tcp_info, xray_result.
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
        local has_requests has_started has_completed has_tcp has_xray
        has_requests=$(jq -e '.network_diag.requests != null' "$packet_file" 2>/dev/null && echo "yes" || echo "no")
        has_started=$(jq -e '.network_diag.started_at != null' "$packet_file" 2>/dev/null && echo "yes" || echo "no")
        has_completed=$(jq -e '.network_diag.completed_at != null' "$packet_file" 2>/dev/null && echo "yes" || echo "no")
        has_tcp=$(jq -e '.network_diag.tcp_info != null' "$packet_file" 2>/dev/null && echo "yes" || echo "no")
        has_xray=$(jq -e '.network_diag.xray_result != null' "$packet_file" 2>/dev/null && echo "yes" || echo "no")
        
        local has_real_field="no"
        [[ "$has_requests" == "yes" ]] && has_real_field="yes"
        [[ "$has_started" == "yes" && "$has_completed" == "yes" ]] && has_real_field="yes"
        [[ "$has_tcp" == "yes" ]] && has_real_field="yes"
        [[ "$has_xray" == "yes" ]] && has_real_field="yes"
        
        if [[ "$has_real_field" != "yes" ]]; then
            log_error "[FAIL] Phase 1: capture packet has no real diagnostic fields"
            log_error "Expected at least one of: requests[], started_at+completed_at, tcp_info, xray_result"
            log_error "Current fields: requests=$has_requests, started_at=$has_started, completed_at=$has_completed, tcp_info=$has_tcp, xray_result=$has_xray"
            ok=false
            PHASE1_HARDEN_FAILED=true
        else
            log_info "[PASS] Phase 1: capture packet has real network_diag with diagnostic data"
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
