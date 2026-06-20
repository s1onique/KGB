#!/bin/bash
# lab_uvb76_capture_netns_phase1_harden.sh — Phase 1 Harden assertions
#
# Phase 1 specific assertions to prevent the "all-suppressed cooldown false green" scenario.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Source self-test from extracted file to stay under LLM-friendly limits
# Use SCRIPTS_DIR if available, otherwise compute relative to this script
if [[ -n "${SCRIPTS_DIR:-}" ]]; then
    source "${SCRIPTS_DIR}/lab_uvb76_capture_netns_phase1_harden_self_test.sh"
else
    # Fallback: compute path relative to this script
    _self_test_source="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)/lab_uvb76_capture_netns_phase1_harden_self_test.sh"
    if [[ -f "$_self_test_source" ]]; then
        source "$_self_test_source"
    fi
fi

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
    local summary_file="${5:-}"  # fetch summary from extract_network_diag_from_spike_row
    
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
        # Real captures have: requests[] OR (started_at+completed_at) OR tcp_info OR xray_result
        # OR tovarisch canonical sections (interfaces, routes, underlay_tcp, tcp_absence_events)
        # Using combined jq predicate to avoid stdout pollution from individual field checks
        if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$packet_file" >/dev/null 2>&1; then
            log_info "[PASS] Phase 1: capture packet has real network_diag with diagnostic data"
            # Log which fields were detected for traceability
            local has_requests="no" has_started="no" has_completed="no" has_tcp="no" has_xray="no"
            local has_interfaces="no" has_routes="no" has_underlay="no" has_absence="no"
            jq -e '.network_diag.requests != null' "$packet_file" >/dev/null 2>&1 && has_requests="yes"
            jq -e '.network_diag.started_at != null' "$packet_file" >/dev/null 2>&1 && has_started="yes"
            jq -e '.network_diag.completed_at != null' "$packet_file" >/dev/null 2>&1 && has_completed="yes"
            jq -e '.network_diag.tcp_info != null' "$packet_file" >/dev/null 2>&1 && has_tcp="yes"
            jq -e '.network_diag.xray_result != null' "$packet_file" >/dev/null 2>&1 && has_xray="yes"
            jq -e '.network_diag.interfaces != null' "$packet_file" >/dev/null 2>&1 && has_interfaces="yes"
            jq -e '.network_diag.routes != null' "$packet_file" >/dev/null 2>&1 && has_routes="yes"
            jq -e '.network_diag.underlay_tcp != null' "$packet_file" >/dev/null 2>&1 && has_underlay="yes"
            jq -e '.network_diag.tcp_absence_events != null' "$packet_file" >/dev/null 2>&1 && has_absence="yes"
            log_info "  Detected fields: requests=$has_requests, started_at=$has_started, completed_at=$has_completed, tcp_info=$has_tcp, xray_result=$has_xray"
            log_info "  Canonical sections: interfaces=$has_interfaces, routes=$has_routes, underlay_tcp=$has_underlay, tcp_absence_events=$has_absence"
        else
            log_error "[FAIL] Phase 1: capture packet has no real diagnostic fields"
            log_error "Expected at least one of: requests[], started_at+completed_at, tcp_info, xray_result, or tovarisch canonical sections"
            # Log all field states for debugging
            local has_requests="no" has_started="no" has_completed="no" has_tcp="no" has_xray="no"
            local has_interfaces="no" has_routes="no" has_underlay="no" has_absence="no"
            jq -e '.network_diag.requests != null' "$packet_file" >/dev/null 2>&1 && has_requests="yes"
            jq -e '.network_diag.started_at != null' "$packet_file" >/dev/null 2>&1 && has_started="yes"
            jq -e '.network_diag.completed_at != null' "$packet_file" >/dev/null 2>&1 && has_completed="yes"
            jq -e '.network_diag.tcp_info != null' "$packet_file" >/dev/null 2>&1 && has_tcp="yes"
            jq -e '.network_diag.xray_result != null' "$packet_file" >/dev/null 2>&1 && has_xray="yes"
            jq -e '.network_diag.interfaces != null' "$packet_file" >/dev/null 2>&1 && has_interfaces="yes"
            jq -e '.network_diag.routes != null' "$packet_file" >/dev/null 2>&1 && has_routes="yes"
            jq -e '.network_diag.underlay_tcp != null' "$packet_file" >/dev/null 2>&1 && has_underlay="yes"
            jq -e '.network_diag.tcp_absence_events != null' "$packet_file" >/dev/null 2>&1 && has_absence="yes"
            log_error "  Current fields: requests=$has_requests, started_at=$has_started, completed_at=$has_completed, tcp_info=$has_tcp, xray_result=$has_xray"
            log_error "  Canonical sections: interfaces=$has_interfaces, routes=$has_routes, underlay_tcp=$has_underlay, tcp_absence_events=$has_absence"
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
    
    # CRITICAL CHECK 4: Verify stored capture was used (not live fallback)
    # The event-specific stored capture should be used, not a live /status fetch.
    # This ensures the diagnostic packet matches the spike event, not current state.
    # 
    # FAIL-CLOSED: Missing or malformed summary is a hard failure in normal mode.
    # A future refactor could silently skip the fallback-source proof if this is just a warning.
    if [[ -z "$summary_file" || ! -f "$summary_file" ]]; then
        log_error "[FAIL] Phase 1: missing fetch summary; cannot prove stored capture source"
        log_error "  File expected: $summary_file"
        
        # Check if degraded mode is enabled (debug-only flag)
        local degraded_mode="${DEGRADED_MODE:-false}"
        if [[ "$degraded_mode" == "true" ]]; then
            log_warn "[DEGRADED MODE] Allowing missing summary - debug mode only"
            log_warn "  This should NEVER happen in production"
        else
            log_error "[HARDEN] Missing summary is NOT allowed in hardened mode"
            log_error "  Expected: fetch summary file with is_fallback field"
            log_error "  Fix: Ensure extract_network_diag_from_spike_row creates the summary file"
            ok=false
            PHASE1_HARDEN_FAILED=true
        fi
    else
        # Diagnostic: log summary file path and key fields
        log_info "  Fetch summary file: $summary_file"
        jq -c '{is_fallback, summary_source, event_id, found_location, capture_source}' "$summary_file" 2>/dev/null || true
        
        local fetch_is_fallback
        fetch_is_fallback=$(jq -r '.is_fallback // "null"' "$summary_file" 2>/dev/null || echo "null")
        
        if [[ "$fetch_is_fallback" == "false" ]]; then
            log_info "[PASS] Phase 1: used stored capture artifact (is_fallback=false)"
            log_info "  Event-specific diagnostic packet confirmed"
        elif [[ "$fetch_is_fallback" == "true" ]]; then
            log_error "[FAIL] Phase 1: used live /status fallback instead of stored capture artifact"
            log_error "  This means the captured spike row did NOT contain embedded network_diag"
            log_error "  The diagnostic packet does NOT match the specific spike event"
            
            # Check if degraded mode is enabled (debug-only flag)
            local degraded_mode="${DEGRADED_MODE:-false}"
            if [[ "$degraded_mode" == "true" ]]; then
                log_warn "[DEGRADED MODE] Allowing fallback fetch - debug mode only"
                log_warn "  This should NEVER happen in production"
            else
                log_error "[HARDEN] Falling back to live /status is NOT allowed in hardened mode"
                log_error "  Expected: spike row .captures[].network_diag should contain stored diagnostic"
                log_error "  Fix: Ensure UVB-76 capture service populates network_diag in spike row"
                ok=false
                PHASE1_HARDEN_FAILED=true
            fi
        else
            log_error "[FAIL] Phase 1: invalid or malformed fetch summary"
            log_error "  is_fallback value: $fetch_is_fallback (expected: true or false)"
            
            # Check if degraded mode is enabled (debug-only flag)
            local degraded_mode="${DEGRADED_MODE:-false}"
            if [[ "$degraded_mode" == "true" ]]; then
                log_warn "[DEGRADED MODE] Allowing malformed summary - debug mode only"
                log_warn "  This should NEVER happen in production"
            else
                log_error "[HARDEN] Malformed summary is NOT allowed in hardened mode"
                ok=false
                PHASE1_HARDEN_FAILED=true
            fi
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
