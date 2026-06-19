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
    if ! check_linux; then
        exit 1
    fi
    if ! check_dependencies; then
        exit 1
    fi

    # Setup
    setup_temp_dir
    setup_trap

    # Build binaries
    log_info "Building tovarisch..."
    if ! make tovarisch-build 2>&1; then
        log_error "Failed to build tovarisch"
        exit 1
    fi

    log_info "Building UVB-76..."
    if ! make uvb76-build 2>&1; then
        log_error "Failed to build UVB-76"
        exit 1
    fi

    # Create topology
    create_namespaces
    configure_interfaces

    # Generate configs
    generate_tovarisch_config
    generate_uvb76_config

    # Verify topology came up
    if ! verify_topology; then
        log_error "Topology verification failed"
        exit 1
    fi

    # Collect topology info
    collect_topology_info

    # Start tovarisch
    if ! start_tovarisch; then
        log_error "Failed to start tovarisch"
        exit 1
    fi

    # Wait for tovarisch HTTP to be ready
    if ! wait_for_tovarisch_http; then
        log_error "tovarisch HTTP endpoint not ready"
        exit 1
    fi

    # Collect listen sockets diagnostic (verifies binding address)
    collect_tovarisch_listen_sockets

    # Verify tovarisch status endpoints
    if ! verify_tovarisch_status; then
        log_error "tovarisch status verification failed"
        exit 1
    fi

    # Start UVB-76
    if ! start_uvb76; then
        log_error "Failed to start UVB-76"
        exit 1
    fi

    # Verify baseline connectivity
    if ! verify_baseline_connectivity; then
        log_error "Baseline connectivity verification failed"
        exit 1
    fi

    # Authenticate to UVB-76 API
    if ! uvb76_authenticate; then
        log_error "Failed to authenticate to UVB-76 API"
        exit 1
    fi

    # ========================================
    # PHASE 0: Baseline probe readiness gate + status artifact
    # ========================================
    log_info ""
    log_info "=== PHASE 0: Baseline Probe Readiness Gate + Status Artifact ==="

    # Save UVB-76 status as Phase 0 artifact
    save_phase0_status

    # Poll for probe samples to prove HTTP probe loop is running
    if wait_for_probe_samples_after_cursor "lab-tovarisch" "http" "" "true" 20 "$BASELINE_PROBE_READY_FILE"; then
        log_info "[PASS] Baseline probe readiness verified - HTTP probe loop is running"
        PROBE_READY=true
    else
        log_error "[FAIL] Baseline probe readiness FAILED - no probe samples after 20s"
        log_error "This means the HTTP probe loop is not running or not reaching tovarisch"
        PROBE_READY=false
    fi

    # Copy probe readiness artifact to Phase 0 naming
    copy_phase0_probe_ready

    # Set baseline cursor for phase isolation
    set_phase_cursor "baseline"

    # ========================================
    # PHASE 1: First eligible spike captured
    # ========================================
    log_info ""
    log_info "=== PHASE 1: First Eligible Spike Captured ==="

    PHASE1_CAPTURED=false
    PHASE1_EVENT_ID=""
    PHASE1_REASONS=""

    # Set phase cursor BEFORE inducing spike
    set_phase_cursor "phase1"

    # Inject 100% loss defect to trigger first spike
    inject_netem_defect
    DEFECT_MODE="100pct-loss"

    # Verify defect is in place
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_error "[FAIL] Defect not working - ping succeeded when it should fail"
    else
        log_info "[PASS] Defect verified - ping fails as expected"

        # STEP 1: Wait for HTTP failure spike EVENT
        log_info "Phase 1 Step 1: Waiting for failure spike event..."
        if wait_for_spike_event_after_cursor "phase1" "$PHASE_PHASE1_CURSOR" "http_probe_timeout|http_probe_failure|http_probe_connection_refused" 30 "$LAB_DIR/spikes-phase1-poll.json"; then
            PHASE1_EVENT_ID="$MATCHED_EVENT_ID"
            PHASE1_REASONS="$MATCHED_REASONS"
            log_info "[PASS] Phase 1 spike event found: event_id=$PHASE1_EVENT_ID reasons=$PHASE1_REASONS"

            # STEP 2: Wait for the CAPTURE for this event FIRST
            log_info "Phase 1 Step 2: Waiting for capture for event $PHASE1_EVENT_ID..."
            if wait_for_spike_capture_after_event "phase1" "$PHASE1_EVENT_ID" 15 "$LAB_DIR/spikes-phase1-capture-poll.json"; then
                log_info "[PASS] Phase 1 capture found for event $PHASE1_EVENT_ID"
                PHASE1_CAPTURED=true

                # NOW query spikes API for full response (with capture populated)
                query_spikes_api "$LAB_DIR/spikes-phase1.json" "lab-tovarisch" "true"

                # Create raw row file for packet extraction
                local phase1_raw_row_file="$LAB_DIR/phase1-spike-row-raw.json"
                echo "$(extract_spike_row_for_event "$LAB_DIR/spikes-phase1.json" "$PHASE1_EVENT_ID")" | jq '.' > "$phase1_raw_row_file"

                # Normalize raw row into contract row
                if normalize_spike_row_capture_contract "$phase1_raw_row_file" "$PHASE1_SPIKE_ROW_FILE"; then
                    # Save spike event (full raw)
                    save_phase_spike_event 1 "$PHASE1_SPIKE_EVENT_FILE" "$(cat "$LAB_DIR/spikes-phase1.json")" "$PHASE1_EVENT_ID" "$PHASE1_REASONS"

                    # Save capture packet from RAW row (has captures[] with network_diag)
                    if save_phase_capture_packet 1 "$PHASE1_CAPTURE_PACKET_FILE" "$phase1_raw_row_file"; then
                        # Save contract summary
                        save_phase_contract_summary 1 "$PHASE1_CAPTURE_CONTRACT_FILE" "$PHASE1_SPIKE_ROW_FILE" "$PHASE1_CAPTURE_PACKET_FILE"

                        # Assert Phase 1 contract
                        if assert_captured_row_contract 1 "$PHASE1_SPIKE_ROW_FILE" "$PHASE1_CAPTURE_PACKET_FILE"; then
                            CONTRACT_PHASE1_CAPTURE_OK=true
                            CONTRACT_PHASE1_PACKET_OK=true
                            log_info "[PASS] Phase 1 contract assertions passed"
                        fi
                    fi
                fi
            else
                log_error "[FAIL] Phase 1 capture missing for event_id=$PHASE1_EVENT_ID"
            fi
        else
            log_error "[FAIL] Phase 1 no failure spike event found after 30s"
        fi
    fi

    # ========================================
    # PHASE 2: Inside-cooldown spike skipped
    # ========================================
    log_info ""
    log_info "=== PHASE 2: Inside-Cooldown Spike Skipped ==="

    PHASE2_SKIPPED=false
    PHASE2_EVENT_ID=""
    PHASE2_REASONS=""

    # Phase 2 is only valid if Phase 1 successfully captured.
    # If Phase 1 failed (timeout or no network_diag), the cooldown was not armed,
    # so we cannot expect skipped_cooldown behavior.
    if [[ "$CONTRACT_PHASE1_CAPTURE_OK" != "true" || "$CONTRACT_PHASE1_PACKET_OK" != "true" ]]; then
        log_error "[SKIP] Phase 2: skipped because Phase 1 capture failed (cooldown not armed)"
        log_error "  phase1_capture_required_for_cooldown=true"
        log_error "  phase1_capture_ok=$CONTRACT_PHASE1_CAPTURE_OK"
        log_error "  phase1_packet_ok=$CONTRACT_PHASE1_PACKET_OK"
        log_error "  phase2_cooldown_contract_ok=false (skipped dependency)"
        
        # Save a skipped-phase artifact for contract completeness
        jq -n \
            --arg phase "phase2" \
            --arg reason "phase1_capture_failed" \
            --argjson phase1_capture_ok "$CONTRACT_PHASE1_CAPTURE_OK" \
            --argjson phase1_packet_ok "$CONTRACT_PHASE1_PACKET_OK" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{
                phase: $phase,
                status: "skipped",
                reason: $reason,
                phase1_capture_ok: $phase1_capture_ok,
                phase1_packet_ok: $phase1_packet_ok,
                timestamp: $timestamp
            }' > "$PHASE2_SPIKE_ROW_FILE" 2>/dev/null || true
        
        # Save contract summary for skipped phase
        save_phase_contract_summary 2 "$PHASE2_CAPTURE_CONTRACT_FILE" "$PHASE2_SPIKE_ROW_FILE" ""
        
        # Skip Phase 2 entirely - go to Phase 3
        log_info "Proceeding to Phase 3..."
        CONTRACT_PHASE2_COOLDOWN_OK=false
    fi
    # End Phase 2 conditional

    # Only run Phase 2 actual test if Phase 1 succeeded
    if [[ "$CONTRACT_PHASE1_CAPTURE_OK" == "true" && "$CONTRACT_PHASE1_PACKET_OK" == "true" ]]; then
        # Set phase cursor BEFORE inducing spike
        set_phase_cursor "phase2"

        # Reinject defect BEFORE cooldown expires
        # Lab cooldown is 5 seconds, so reinject immediately
        log_info "Reinjecting defect before cooldown expires..."
        inject_netem_defect

        # Verify defect is in place again
        if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
            log_error "[FAIL] Defect not working - ping succeeded when it should fail"
        else
            log_info "[PASS] Defect verified - ping fails as expected"

            # STEP 1: Wait for HTTP failure spike EVENT
            log_info "Phase 2 Step 1: Waiting for failure spike event..."
            if wait_for_spike_event_after_cursor "phase2" "$PHASE_PHASE2_CURSOR" "http_probe_timeout|http_probe_failure|http_probe_connection_refused" 15 "$LAB_DIR/spikes-phase2-poll.json"; then
                PHASE2_EVENT_ID="$MATCHED_EVENT_ID"
                PHASE2_REASONS="$MATCHED_REASONS"
                log_info "[PASS] Phase 2 spike event found: event_id=$PHASE2_EVENT_ID reasons=$PHASE2_REASONS"

                # Query spikes API for full response
                query_spikes_api "$LAB_DIR/spikes-phase2.json" "lab-tovarisch" "true"

                # Save spike event (full raw)
                save_phase_spike_event 2 "$PHASE2_SPIKE_EVENT_FILE" "$(cat "$LAB_DIR/spikes-phase2.json")" "$PHASE2_EVENT_ID" "$PHASE2_REASONS"

                # STEP 2: Wait for skipped_cooldown row (NOT a capture)
                # The polling helper will normalize and write the row when it finds skipped_cooldown
                log_info "Phase 2 Step 2: Waiting for skipped_cooldown row for event $PHASE2_EVENT_ID..."
                if wait_for_skipped_cooldown_spike_row_after_event 2 "$PHASE2_EVENT_ID" 15 "$PHASE2_SPIKE_ROW_FILE"; then
                    log_info "[PASS] Phase 2 skipped_cooldown row found for event $PHASE2_EVENT_ID"
                    PHASE2_SKIPPED=true

                    # Save contract summary (no packet file for skipped cooldown)
                    save_phase_contract_summary 2 "$PHASE2_CAPTURE_CONTRACT_FILE" "$PHASE2_SPIKE_ROW_FILE" ""

                    # Assert Phase 2 contract using normalized row
                    if assert_skipped_cooldown_row_contract 2 "$PHASE2_SPIKE_ROW_FILE"; then
                        CONTRACT_PHASE2_COOLDOWN_OK=true
                        log_info "[PASS] Phase 2 contract assertions passed"
                    fi
                else
                    log_error "[FAIL] Phase 2 skipped_cooldown row not found for event_id=$PHASE2_EVENT_ID"
                fi
            else
                log_error "[FAIL] Phase 2 no failure spike event found after 15s"
            fi
        fi
    fi
    # End Phase 2 actual test

    # ========================================
    # PHASE 3: Post-cooldown spike captured again
    # ========================================
    log_info ""
    log_info "=== PHASE 3: Post-Cooldown Spike Captured Again ==="

    # Wait for cooldown to expire (lab cooldown is 5 seconds, add buffer)
    log_info "Waiting for cooldown to expire (8 seconds + buffer)..."
    sleep 10

    PHASE3_CAPTURED=false
    PHASE3_EVENT_ID=""
    PHASE3_REASONS=""

    # Clear previous defect first
    clear_defect
    sleep 2

    # Re-inject defect after cooldown
    log_info "Reinjecting defect after cooldown expires..."
    inject_netem_defect

    # Verify defect is in place
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_error "[FAIL] Defect not working - ping succeeded when it should fail"
    else
        log_info "[PASS] Defect verified - ping fails as expected"

        # Set phase cursor BEFORE inducing spike
        set_phase_cursor "phase3"

        # STEP 1: Wait for HTTP failure spike EVENT
        log_info "Phase 3 Step 1: Waiting for failure spike event..."
        if wait_for_spike_event_after_cursor "phase3" "$PHASE_PHASE3_CURSOR" "http_probe_timeout|http_probe_failure|http_probe_connection_refused" 30 "$LAB_DIR/spikes-phase3-poll.json"; then
            PHASE3_EVENT_ID="$MATCHED_EVENT_ID"
            PHASE3_REASONS="$MATCHED_REASONS"
            log_info "[PASS] Phase 3 spike event found: event_id=$PHASE3_EVENT_ID reasons=$PHASE3_REASONS"

            # STEP 2: Wait for the CAPTURE for this event FIRST
            log_info "Phase 3 Step 2: Waiting for capture for event $PHASE3_EVENT_ID..."
            if wait_for_spike_capture_after_event "phase3" "$PHASE3_EVENT_ID" 15 "$LAB_DIR/spikes-phase3-capture-poll.json"; then
                log_info "[PASS] Phase 3 capture found for event $PHASE3_EVENT_ID"
                PHASE3_CAPTURED=true

                # NOW query spikes API for full response (with capture populated)
                query_spikes_api "$LAB_DIR/spikes-phase3.json" "lab-tovarisch" "true"

                # Create raw row file for packet extraction
                local phase3_raw_row_file="$LAB_DIR/phase3-spike-row-raw.json"
                echo "$(extract_spike_row_for_event "$LAB_DIR/spikes-phase3.json" "$PHASE3_EVENT_ID")" | jq '.' > "$phase3_raw_row_file"

                # Normalize raw row into contract row
                if normalize_spike_row_capture_contract "$phase3_raw_row_file" "$PHASE3_SPIKE_ROW_FILE"; then
                    # Save spike event (full raw)
                    save_phase_spike_event 3 "$PHASE3_SPIKE_EVENT_FILE" "$(cat "$LAB_DIR/spikes-phase3.json")" "$PHASE3_EVENT_ID" "$PHASE3_REASONS"

                    # Save capture packet from RAW row (has captures[] with network_diag)
                    if save_phase_capture_packet 3 "$PHASE3_CAPTURE_PACKET_FILE" "$phase3_raw_row_file"; then
                        # Save contract summary
                        save_phase_contract_summary 3 "$PHASE3_CAPTURE_CONTRACT_FILE" "$PHASE3_SPIKE_ROW_FILE" "$PHASE3_CAPTURE_PACKET_FILE"

                        # Assert Phase 3 contract
                        if assert_captured_row_contract 3 "$PHASE3_SPIKE_ROW_FILE" "$PHASE3_CAPTURE_PACKET_FILE"; then
                            CONTRACT_PHASE3_CAPTURE_OK=true
                            CONTRACT_PHASE3_PACKET_OK=true
                            log_info "[PASS] Phase 3 contract assertions passed"
                        fi
                    fi
                fi
            else
                log_error "[FAIL] Phase 3 capture missing for event_id=$PHASE3_EVENT_ID"
            fi
        else
            log_error "[FAIL] Phase 3 no failure spike event found after 30s"
        fi
    fi

    # Clear defect for cleanup
    clear_defect

    # ========================================
    # Run contract verification
    # ========================================
    log_info ""
    log_info "=== Running Contract Verification ==="

    if run_contract_verification; then
        CONTRACT_DIR_OK=true
    else
        log_error "[FAIL] Contract verification failed"
    fi

    # Verify distinct event IDs across phases
    verify_distinct_event_ids

    # ========================================
    # Write final result
    # ========================================
    write_result

    # Print summary
    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact directory: $LAB_DIR"
    log_info ""
    log_info "Phase artifacts:"
    log_info "  Phase 0: $([ -f "$PHASE0_STATUS_FILE" ] && echo "status saved" || echo "MISSING")"
    log_info "  Phase 1: $([ -f "$PHASE1_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING") / $([ -f "$PHASE1_CAPTURE_PACKET_FILE" ] && echo "packet saved" || echo "MISSING")"
    log_info "  Phase 2: $([ -f "$PHASE2_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING")"
    log_info "  Phase 3: $([ -f "$PHASE3_SPIKE_ROW_FILE" ] && echo "row saved" || echo "MISSING") / $([ -f "$PHASE3_CAPTURE_PACKET_FILE" ] && echo "packet saved" || echo "MISSING")"
    log_info ""
    print_lab_result_summary

    compute_lab_exit_code
}

# Run lab when executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_lab "$@"
fi
