#!/bin/bash
# lab_uvb76_capture_netns.sh — UVB-76 diagnostic capture netns fault lab
#
# Creates isolated Linux network namespaces with UVB-76 and tovarisch
# for testing diagnostic capture under network impairment conditions.
#
# This lab uses the REAL UVB-76 API to extract capture evidence,
# not log-grep synthesis.
#
# Primary execution: GitHub Actions (ubuntu-latest)
# Local execution: optional debugging only
#
# Trigger: workflow_dispatch (manual only)
# This is NOT part of make gate.

set -euo pipefail

# Source shared library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_uvb76_capture_netns_lib.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_lib.sh"

# =============================================================================
# Contract result tracking
# =============================================================================

CONTRACT_PHASE1_CAPTURE_OK=false
CONTRACT_PHASE1_PACKET_OK=false
CONTRACT_PHASE2_COOLDOWN_OK=false
CONTRACT_PHASE3_CAPTURE_OK=false
CONTRACT_PHASE3_PACKET_OK=false
CONTRACT_DIR_OK=false
CONTRACT_DISTINCT_EVENT_IDS_OK=false

# Main lab execution
run_lab() {
    log_info "=== UVB-76 Diagnostic Capture Netns Lab ==="
    log_info "Using REAL UVB-76 API for capture evidence"
    log_info "Primary execution: GitHub Actions (workflow_dispatch)"
    log_info "Local execution: optional for debugging only"
    log_info ""

    # Pre-flight checks
    if ! check_linux; then exit 1; fi
    if ! check_dependencies; then exit 1; fi

    # Setup
    setup_temp_dir
    setup_trap

    # Build binaries
    log_info "Building tovarisch..."
    if ! make tovarisch-build 2>&1; then
        log_error "Failed to build tovarisch"; exit 1; fi

    log_info "Building UVB-76..."
    if ! make uvb76-build 2>&1; then
        log_error "Failed to build UVB-76"; exit 1; fi

    # Create topology and configs
    create_namespaces
    configure_interfaces
    generate_tovarisch_config
    generate_uvb76_config

    # Verify topology came up
    if ! verify_topology; then
        log_error "Topology verification failed"; exit 1; fi
    collect_topology_info

    # Start tovarisch
    if ! start_tovarisch; then
        log_error "Failed to start tovarisch"; exit 1; fi
    if ! wait_for_tovarisch_http; then
        log_error "tovarisch HTTP endpoint not ready"; exit 1; fi
    collect_tovarisch_listen_sockets
    if ! verify_tovarisch_status; then
        log_error "tovarisch status verification failed"; exit 1; fi

    # Start UVB-76
    if ! start_uvb76; then
        log_error "Failed to start UVB-76"; exit 1; fi
    if ! verify_baseline_connectivity; then
        log_error "Baseline connectivity verification failed"; exit 1; fi
    if ! uvb76_authenticate; then
        log_error "Failed to authenticate to UVB-76 API"; exit 1; fi

    # PHASE 0: Baseline probe readiness gate + status artifact
    log_info ""; log_info "=== PHASE 0: Baseline Probe Readiness Gate + Status Artifact ==="
    
    # CRITICAL: Save and verify effective probe URL BEFORE anything else
    # This proves the exact URL UVB-76 will probe. Fails hard - no point continuing if wrong.
    if ! save_effective_probe_url; then
        log_error "[FAIL] Effective probe URL verification failed - lab cannot proceed"
        exit 1
    fi
    
    # CRITICAL: Save and verify effective diagnostic URL BEFORE anything else
    # This proves the exact URL UVB-76 will use for diagnostic capture.
    # Verifies base_url is origin-only (not full path) to avoid double-path issues.
    if ! save_effective_diag_url; then
        log_error "[FAIL] Effective diagnostic URL verification failed - lab cannot proceed"
        exit 1
    fi
    
    save_phase0_status

    if wait_for_probe_samples_after_cursor "lab-tovarisch" "http" "" "true" 20 "$BASELINE_PROBE_READY_FILE"; then
        log_info "[PASS] Baseline probe readiness verified"
        PROBE_READY=true
    else
        log_error "[FAIL] Baseline probe readiness FAILED"
        PROBE_READY=false
    fi
    copy_phase0_probe_ready
    set_phase_cursor "baseline"

    # PHASE 1: First eligible spike captured
    log_info ""; log_info "=== PHASE 1: First Eligible Spike Captured ==="
    PHASE1_CAPTURED=false; PHASE1_EVENT_ID=""; PHASE1_REASONS=""
    PHASE1_ROW_ASSERTION_OK=false
    set_phase_cursor "phase1"

    # Phase 1 result tracking
    PHASE1_HAD_CAPTURE=false
    PHASE1_HAD_PACKET=false
    PHASE1_ASSERTION_FAILED=false

    # Run Phase 1 capture with defect clear (trap/finally pattern)
    if wait_and_fetch_capture_with_defect_clear 1 "phase1" "PHASE1_EVENT_ID" "PHASE1_REASONS" "$PHASE_PHASE1_CURSOR" 30 15 "$DEFECT_MODE_LAB_PROBE"; then
        # SUCCESS: Phase 1 capture succeeded and packet fetch succeeded
        PHASE1_CAPTURED=true
        PHASE1_HAD_CAPTURE=true
        PHASE1_HAD_PACKET=true
        PHASE1_ROW_ASSERTION_OK=true
        
        # =============================================================================
        # PHASE 1 HARDEN: Assert real capture requirements
        # This prevents the "all-suppressed cooldown false green" scenario:
        # - Must have capture_status=captured (not skipped_cooldown)
        # - Must have real network_diag (not just suppressed metadata)
        # - No prior phase should have consumed the only real capture
        # =============================================================================
        log_info ""
        log_info "=== PHASE 1 HARDEN: Verifying Real Capture Requirements ==="
        
        if assert_phase1_real_capture 1 "$PHASE1_SPIKE_ROW_FILE" "$PHASE1_CAPTURE_PACKET_FILE"; then
            log_info "[PASS] Phase 1 real capture verification passed"
            CONTRACT_PHASE1_CAPTURE_OK=true
            CONTRACT_PHASE1_PACKET_OK=true
        else
            log_error "[FAIL] Phase 1 real capture verification FAILED"
            log_error "This indicates the 'all-suppressed cooldown false green' scenario:"
            log_error "  - Phase 1 capture_status is NOT 'captured' (likely 'skipped_cooldown')"
            log_error "  - OR network_diag is missing (only suppressed metadata present)"
            log_error "  - Possible cause: warmup polling consumed the only real capture"
            CONTRACT_PHASE1_CAPTURE_OK=false
            CONTRACT_PHASE1_PACKET_OK=false
            PHASE1_CAPTURED=false
            
            # Write failure artifact
            jq -n \
                --arg phase "phase1" \
                --arg status "failure" \
                --arg reason "hardened_real_capture_required" \
                --arg event_id "$PHASE1_EVENT_ID" \
                --arg reasons "$PHASE1_REASONS" \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{phase: $phase, status: $status, reason: $reason, event_id: $event_id, reasons: $reasons, timestamp: $timestamp}' \
                > "$LAB_DIR/phase1-failure.json" 2>/dev/null || true
        fi
        
        # CRITICAL: Clear defect immediately after Phase 1 success
        # This prevents the defect from persisting and causing skipped_cooldown spikes
        # during Phase 2 or Phase 3 waits
        log_info "Phase 1 completion: clearing defect before proceeding..."
        clear_defect
        
        if [[ "$CONTRACT_PHASE1_CAPTURE_OK" == "true" ]]; then
            # Save Phase 1 success indicator
            jq -n \
                --arg phase "phase1" \
                --arg event_id "$PHASE1_EVENT_ID" \
                --arg reasons "$PHASE1_REASONS" \
                --arg status "success" \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{phase: $phase, event_id: $event_id, reasons: $reasons, status: $status, timestamp: $timestamp}' \
                > "$LAB_DIR/phase1-success.json" 2>/dev/null || true
        fi
        
        # =============================================================================
        # FAIL-CLOSED: Exit if Phase 1 harden failed
        # This ensures the lab fails hard when real capture requirements are not met,
        # preventing the "all-suppressed cooldown false green" scenario.
        # =============================================================================
        if [[ "$PHASE1_HARDEN_FAILED" == "true" ]]; then
            log_error "[FATAL] Phase 1 harden failed - exiting with failure"
            log_error "This prevents the 'all-suppressed cooldown false green' scenario"
            write_result
            print_lab_result_summary
            compute_lab_exit_code
            exit 1
        fi
    else
        # FAILURE: Phase 1 capture or packet fetch failed
        PHASE1_CAPTURED=false
        
        # CRITICAL: Clear defect immediately after Phase 1 failure
        # This is the "finally" part of the trap - we must not leave the lab
        # probe defect active while skipping phases or waiting
        log_info "Phase 1 failure: clearing defect (trap/finally pattern)..."
        clear_defect
        
        # Write failure artifact
        jq -n \
            --arg phase "phase1" \
            --arg status "failure" \
            --arg event_id "$PHASE1_EVENT_ID" \
            --arg reasons "$PHASE1_REASONS" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: $status, event_id: $event_id, reasons: $reasons, timestamp: $timestamp}' \
            > "$LAB_DIR/phase1-failure.json" 2>/dev/null || true
        
        # Check if assertion failed vs. capture/packet failure
        # The phase_capture_helpers writes phaseN-row-assertion-failed marker file
        if [[ -f "$LAB_DIR/phase1-row-assertion-failed" ]]; then
            PHASE1_ASSERTION_FAILED=true
            log_error "[FAIL] Phase 1 row assertion FAILED - stopping lab (contract bug, not product)"
            # Write contract failure artifact for debugging
            if [[ -f "$PHASE1_SPIKE_ROW_FILE" ]]; then
                cp "$PHASE1_SPIKE_ROW_FILE" "$LAB_DIR/phase1-spike-row-debug.json" 2>/dev/null || true
            fi
            jq -n \
                --arg phase "phase1" \
                --arg type "assertion_failure" \
                --arg message "Row assertion failed - contract bug, not product behavior" \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{phase: $phase, type: $type, message: $message, timestamp: $timestamp}' \
                > "$LAB_DIR/phase1-assertion-failure-summary.json" 2>/dev/null || true
        fi
        
        # If Phase 1 failed (capture or packet), we cannot proceed to Phase 2/3
        # Write skip artifacts for downstream phases
        jq -n --arg phase "phase2" --arg reason "phase1_failed" \
            --argjson phase1_captured "$PHASE1_CAPTURED" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: "skipped", reason: $reason, phase1_captured: $phase1_captured, timestamp: $timestamp}' \
            > "$PHASE2_SPIKE_ROW_FILE" 2>/dev/null || true
        jq -n --arg phase "phase3" --arg reason "phase1_failed" \
            --argjson phase1_captured "$PHASE1_CAPTURED" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: "skipped", reason: $reason, phase1_captured: $phase1_captured, timestamp: $timestamp}' \
            > "$PHASE3_SPIKE_ROW_FILE" 2>/dev/null || true
        
        # CONTRACT FAILURE: Stop the lab if Phase 1 failed
        # We cannot validate Phase 2 (cooldown) or Phase 3 (re-capture) without Phase 1 success
        if [[ "$PHASE1_ASSERTION_FAILED" == "true" ]]; then
            log_error "[FATAL] Phase 1 assertion failed - stopping lab for contract fix"
            log_error "This is a contract bug, not a product failure"
            log_error "Artifacts saved for debugging:"
            log_error "  - phase1-spike-row-debug.json"
            log_error "  - phase1-assertion-failure-summary.json"
            CONTRACT_PHASE1_CAPTURE_OK=false
            CONTRACT_PHASE1_PACKET_OK=false
            write_result
            print_lab_result_summary
            compute_lab_exit_code
            exit 1
        else
            log_error "[FAIL] Phase 1 capture/packet failed - proceeding to Phase 3 for recovery test"
            # Allow Phase 3 to run even if Phase 1 failed (for recovery testing)
            # but mark Phase 2 as skipped
            CONTRACT_PHASE1_CAPTURE_OK=false
            CONTRACT_PHASE1_PACKET_OK=false
        fi
    fi

    # PHASE 2: Inside-cooldown spike skipped
    # Phase 2 gating: Run only if Phase 1 ACTUAL capture succeeded (not just row assertion)
    # We require: CONTRACT_PHASE1_CAPTURE_OK == true AND CONTRACT_PHASE1_PACKET_OK == true
    # This means Phase 1 capture metadata was captured AND packet fetch succeeded.
    # We do NOT require PHASE1_ROW_ASSERTION_OK - that's a contract detail, not product behavior.
    log_info ""; log_info "=== PHASE 2: Inside-Cooldown Spike Skipped ==="
    PHASE2_SKIPPED=false; PHASE2_EVENT_ID=""; PHASE2_REASONS=""

    if [[ "$CONTRACT_PHASE1_CAPTURE_OK" != "true" || "$CONTRACT_PHASE1_PACKET_OK" != "true" ]]; then
        log_error "[SKIP] Phase 2: skipped because Phase 1 capture/packet failed (cooldown not armed)"
        jq -n --arg phase "phase2" --arg reason "phase1_capture_or_packet_failed" \
            --argjson phase1_capture_ok "$CONTRACT_PHASE1_CAPTURE_OK" \
            --argjson phase1_packet_ok "$CONTRACT_PHASE1_PACKET_OK" \
            --argjson phase1_row_assertion_ok "$PHASE1_ROW_ASSERTION_OK" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: "skipped", reason: $reason, phase1_capture_ok: $phase1_capture_ok, phase1_packet_ok: $phase1_packet_ok, phase1_row_assertion_ok: $phase1_row_assertion_ok, timestamp: $timestamp}' \
            > "$PHASE2_SPIKE_ROW_FILE" 2>/dev/null || true
        save_phase_contract_summary 2 "$PHASE2_CAPTURE_CONTRACT_FILE" "$PHASE2_SPIKE_ROW_FILE" ""
        log_info "Proceeding to Phase 3..."; CONTRACT_PHASE2_COOLDOWN_OK=false
    elif [[ "$CONTRACT_PHASE1_CAPTURE_OK" == "true" && "$CONTRACT_PHASE1_PACKET_OK" == "true" ]]; then
        set_phase_cursor "phase2"
        log_info "Reinjecting lab probe defect before cooldown expires..."
        inject_netem_defect "$DEFECT_MODE_LAB_PROBE"

        if ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" \
            "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/lab/probe" 2>/dev/null | grep -q "503"; then
            log_info "[PASS] Lab probe defect verified - /lab/probe returns 503"
            
            # Verify /status remains healthy during defect (same as Phase 1/3)
            local status_status
            status_status=$(ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" \
                "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/status" 2>/dev/null)
            if [[ "$status_status" != "200" ]]; then
                log_error "[FAIL] /status should remain healthy - returned $status_status, expected 200"
                CONTRACT_PHASE2_COOLDOWN_OK=false
            else
                log_info "[PASS] /status remains healthy during Phase 2 defect - returns 200"
            fi
            # Only proceed with spike detection if /status is healthy
            if [[ "$status_status" == "200" ]] && wait_for_spike_event_after_cursor "phase2" "$PHASE_PHASE2_CURSOR" "http_probe_timeout|http_probe_failure|http_probe_connection_refused|http_probe_503" 15 "$LAB_DIR/spikes-phase2-poll.json"; then
                PHASE2_EVENT_ID="$MATCHED_EVENT_ID"; PHASE2_REASONS="$MATCHED_REASONS"
                log_info "[PASS] Phase 2 spike event found: event_id=$PHASE2_EVENT_ID"
                query_spikes_api "$LAB_DIR/spikes-phase2.json" "lab-tovarisch" "true"
                save_phase_spike_event 2 "$PHASE2_SPIKE_EVENT_FILE" "$(cat "$LAB_DIR/spikes-phase2.json")" "$PHASE2_EVENT_ID" "$PHASE2_REASONS"

                if wait_for_skipped_cooldown_spike_row_after_event 2 "$PHASE2_EVENT_ID" 15 "$PHASE2_SPIKE_ROW_FILE"; then
                    log_info "[PASS] Phase 2 skipped_cooldown row found"
                    PHASE2_SKIPPED=true; clear_defect
                    save_phase_contract_summary 2 "$PHASE2_CAPTURE_CONTRACT_FILE" "$PHASE2_SPIKE_ROW_FILE" ""
                    if assert_skipped_cooldown_row_contract 2 "$PHASE2_SPIKE_ROW_FILE"; then
                        CONTRACT_PHASE2_COOLDOWN_OK=true
                        log_info "[PASS] Phase 2 contract assertions passed"
                    fi
                fi
            fi
        fi
    fi

    # PHASE 3: Post-cooldown spike captured again
    log_info ""; log_info "=== PHASE 3: Post-Cooldown Spike Captured Again ==="
    PHASE3_CAPTURED=false; PHASE3_EVENT_ID=""; PHASE3_REASONS=""
    
    # CRITICAL: Wait for actual cooldown expiration using Phase 2 cooldown_info
    # The hardcoded sleep 10 was a race condition - Phase 3 could start before cooldown expired.
    # Now we read next_capture_eligible_at from Phase 2 and wait until that time + safety margin.
    # FAIL-HARD policy: If Phase 2 row is missing or cooldown_info is invalid, return non-zero.
    # We do NOT fall back to hardcoded sleep - that reintroduces the race condition.
    if [[ ! -f "$PHASE2_SPIKE_ROW_FILE" ]]; then
        log_error "[FAIL] Phase 2 row file not found; cannot prove cooldown eligibility"
        CONTRACT_PHASE3_CAPTURE_OK=false
        CONTRACT_PHASE3_PACKET_OK=false
        return 1
    fi

    log_info "Phase 2 row file exists, waiting for actual cooldown expiration..."
    COOLDOWN_WAIT_SUMMARY_FILE="$PHASE3_COOLDOWN_WAIT_SUMMARY_FILE"
    if ! wait_until_cooldown_eligible "$PHASE2_SPIKE_ROW_FILE" 2; then
        log_error "[FAIL] Failed to wait for cooldown expiration - cooldown_info missing or invalid"
        CONTRACT_PHASE3_CAPTURE_OK=false
        CONTRACT_PHASE3_PACKET_OK=false
        return 1
    fi

    log_info "[PASS] Cooldown wait complete - Phase 3 eligible to capture"
    
    clear_defect; sleep 2
    
    # Set cursor AFTER cooldown wait to ensure Phase 3 events are distinct
    set_phase_cursor "phase3"
    
    if wait_and_fetch_capture_with_defect_clear 3 "phase3" "PHASE3_EVENT_ID" "PHASE3_REASONS" "$PHASE_PHASE3_CURSOR" 30 15 "$DEFECT_MODE_LAB_PROBE"; then
        CONTRACT_PHASE3_CAPTURE_OK=true; CONTRACT_PHASE3_PACKET_OK=true
    else
        # Phase 3 failed - save decision metadata for debugging
        log_error "[FAIL] Phase 3 capture failed - saving decision metadata for debugging"
        if [[ -f "$PHASE3_SPIKE_ROW_FILE" ]]; then
            # Save cooldown decision metadata
            save_cooldown_decision_metadata 3 "$PHASE3_SPIKE_ROW_FILE" "$LAB_DIR/phase3-cooldown-decision.json" || true
            
            # Also save the raw spike row for debugging
            cp "$PHASE3_SPIKE_ROW_FILE" "$LAB_DIR/phase3-spike-row-debug.json" 2>/dev/null || true
            
            # Log the capture status and cooldown info
            local phase3_capture_status
            phase3_capture_status=$(jq -r '.capture_status // "unknown"' "$PHASE3_SPIKE_ROW_FILE" 2>/dev/null || echo "unknown")
            log_error "Phase 3 capture_status: $phase3_capture_status"
            
            if jq -e '.cooldown_info != null' "$PHASE3_SPIKE_ROW_FILE" >/dev/null 2>&1; then
                log_error "Phase 3 cooldown_info present (showing cooldown decision at capture time)"
                jq '.cooldown_info' "$PHASE3_SPIKE_ROW_FILE" 2>/dev/null || true
            fi
        fi
    fi

    # Run contract verification
    log_info ""; log_info "=== Running Contract Verification ==="
    if run_contract_verification "$CONTRACT_VERIFIER_OUTPUT_FILE" "$LAB_DIR"; then
        CONTRACT_DIR_OK=true
    else
        log_error "[FAIL] Contract verification failed"
    fi
    verify_distinct_event_ids
    write_result

    # Print summary
    log_info ""; log_info "=== Lab Complete ==="
    log_info "Artifact directory: $LAB_DIR"
    log_info "  Phase 0: $([ -f "$PHASE0_STATUS_FILE" ] && echo "status saved" || echo "MISSING")"
    log_info "  Phase 1: $([ -f "$PHASE1_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING") / $([ -f "$PHASE1_CAPTURE_PACKET_FILE" ] && echo "packet saved" || echo "MISSING")"
    log_info "  Phase 2: $([ -f "$PHASE2_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING")"
    log_info "  Phase 3: $([ -f "$PHASE3_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING") / $([ -f "$PHASE3_CAPTURE_PACKET_FILE" ] && echo "packet saved" || echo "MISSING")"
    log_info ""; print_lab_result_summary
    compute_lab_exit_code
}

# Run lab when executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_lab "$@"
fi
