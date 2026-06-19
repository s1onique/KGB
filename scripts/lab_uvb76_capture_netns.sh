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
    set_phase_cursor "phase1"

    if wait_and_fetch_capture_with_defect_clear 1 "phase1" "PHASE1_EVENT_ID" "PHASE1_REASONS" "$PHASE_PHASE1_CURSOR" 30 15 "$DEFECT_MODE_CLEAR_BEFORE_FETCH"; then
        CONTRACT_PHASE1_CAPTURE_OK=true; CONTRACT_PHASE1_PACKET_OK=true
    fi

    # PHASE 2: Inside-cooldown spike skipped
    log_info ""; log_info "=== PHASE 2: Inside-Cooldown Spike Skipped ==="
    PHASE2_SKIPPED=false; PHASE2_EVENT_ID=""; PHASE2_REASONS=""

    if [[ "$CONTRACT_PHASE1_CAPTURE_OK" != "true" || "$CONTRACT_PHASE1_PACKET_OK" != "true" ]]; then
        log_error "[SKIP] Phase 2: skipped because Phase 1 capture failed (cooldown not armed)"
        jq -n --arg phase "phase2" --arg reason "phase1_capture_failed" \
            --argjson phase1_capture_ok "$CONTRACT_PHASE1_CAPTURE_OK" \
            --argjson phase1_packet_ok "$CONTRACT_PHASE1_PACKET_OK" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: "skipped", reason: $reason, phase1_capture_ok: $phase1_capture_ok, phase1_packet_ok: $phase1_packet_ok, timestamp: $timestamp}' \
            > "$PHASE2_SPIKE_ROW_FILE" 2>/dev/null || true
        save_phase_contract_summary 2 "$PHASE2_CAPTURE_CONTRACT_FILE" "$PHASE2_SPIKE_ROW_FILE" ""
        log_info "Proceeding to Phase 3..."; CONTRACT_PHASE2_COOLDOWN_OK=false
    elif [[ "$CONTRACT_PHASE1_CAPTURE_OK" == "true" && "$CONTRACT_PHASE1_PACKET_OK" == "true" ]]; then
        set_phase_cursor "phase2"
        log_info "Reinjecting defect before cooldown expires..."
        inject_netem_defect "$DEFECT_MODE_CLEAR_BEFORE_FETCH"

        if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
            log_error "[FAIL] Defect not working"
        else
            log_info "[PASS] Defect verified"
            if wait_for_spike_event_after_cursor "phase2" "$PHASE_PHASE2_CURSOR" "http_probe_timeout|http_probe_failure|http_probe_connection_refused" 15 "$LAB_DIR/spikes-phase2-poll.json"; then
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
    log_info "Waiting for cooldown to expire (8 seconds + buffer)..."; sleep 10
    PHASE3_CAPTURED=false; PHASE3_EVENT_ID=""; PHASE3_REASONS=""
    clear_defect; sleep 2
    
    # Set cursor BEFORE helper injection (helper owns defect injection, verification, capture wait, clear, fetch)
    set_phase_cursor "phase3"
    
    if wait_and_fetch_capture_with_defect_clear 3 "phase3" "PHASE3_EVENT_ID" "PHASE3_REASONS" "$PHASE_PHASE3_CURSOR" 30 15 "$DEFECT_MODE_CLEAR_BEFORE_FETCH"; then
        CONTRACT_PHASE3_CAPTURE_OK=true; CONTRACT_PHASE3_PACKET_OK=true
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
