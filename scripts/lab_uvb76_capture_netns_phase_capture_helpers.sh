#!/bin/bash
# lab_uvb76_capture_netns_phase_capture_helpers.sh — Phase capture helpers for netns lab
#
# Extracted helper functions to keep the main lab script under LLM-friendly limits.
# Sourced by lab_uvb76_capture_netns_lib.sh.
#
# CRITICAL: For captured/skipped_cooldown/captured contract:
#   - Use DEFECT_MODE_LAB_PROBE: only /lab/probe fails, /status remains healthy
#   - Inject defect (creates failure file) to trigger spike detection
#   - DO NOT clear defect before capture - /status is always reachable
#   - Wait for capture (succeeds because /status is healthy)
#   - Fetch diagnostic packet (works because /status is healthy)
#
# This ensures Phase 1/3 capture_status=captured deterministically.

# Wait for spike event and then capture, then fetch diagnostic packet.
# Uses lab probe mode where only /lab/probe fails while /status remains healthy.
# Returns 0 on success, sets CONTRACT_*_CAPTURE_OK and CONTRACT_*_PACKET_OK globals.
wait_and_fetch_capture_with_defect_clear() {
    local phase_num="$1"
    local phase_prefix="$2"
    local phase_event_id_var="$3"  # e.g., "PHASE1_EVENT_ID"
    local phase_reasons_var="$4"   # e.g., "PHASE1_REASONS"
    local phase_cursor="$5"        # e.g., "$PHASE_PHASE1_CURSOR"
    local spike_timeout="${6:-30}"
    local capture_timeout="${7:-15}"
    local defect_mode="${8:-${DEFECT_MODE_LAB_PROBE}}"
    
    # Declare variables to hold outputs
    local local_event_id=""
    local local_reasons=""
    local captured=false
    local packet_ok=false
    
    # STEP 0: Inject lab probe defect to trigger spike detection
    # With lab-probe mode: /lab/probe returns 503, /status remains 200
    # This is the key difference - no need to clear defect for capture to succeed.
    log_info "Phase $phase_num Step 0: Injecting lab probe defect..."
    inject_netem_defect "$defect_mode"
    
    # For lab-probe mode, verify /lab/probe returns 503 and /status returns 200
    if [[ "$defect_mode" == "$DEFECT_MODE_LAB_PROBE" ]]; then
        local probe_status
        probe_status=$(ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" \
            "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/lab/probe" 2>/dev/null)
        if [[ "$probe_status" != "503" ]]; then
            log_error "[FAIL] Lab probe defect not working - /lab/probe returned $probe_status, expected 503"
            return 1
        fi
        log_info "[PASS] Lab probe defect verified - /lab/probe returns 503"
        
        local status_status
        status_status=$(ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" \
            "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/status" 2>/dev/null)
        if [[ "$status_status" != "200" ]]; then
            log_error "[FAIL] /status should remain healthy - returned $status_status, expected 200"
            return 1
        fi
        log_info "[PASS] /status remains healthy during defect - returns 200"
    fi
    
    # STEP 1: Wait for HTTP failure spike EVENT
    # The spike event is created when the probe detects the failure (503 on /lab/probe).
    log_info "Phase $phase_num Step 1: Waiting for failure spike event..."
    if wait_for_spike_event_after_cursor "$phase_prefix" "$phase_cursor" "http_probe_timeout|http_probe_failure|http_probe_connection_refused|http_probe_503" "$spike_timeout" "$LAB_DIR/spikes-${phase_prefix}-poll.json"; then
        local_event_id="$MATCHED_EVENT_ID"
        local_reasons="$MATCHED_REASONS"
        log_info "[PASS] Phase $phase_num spike event found: event_id=$local_event_id reasons=$local_reasons"
        
        # Export the event_id and reasons to caller
        export "$phase_event_id_var=$local_event_id"
        export "$phase_reasons_var=$local_reasons"
        
        # IMPORTANT: With lab-probe mode, we DO NOT clear the defect here.
        # The capture will succeed because /status is always reachable.
        # The defect stays active to detect the spike event.
        
        # STEP 2: Wait for the CAPTURE for this event
        # /status is reachable, so capture should succeed (status=ok → captured).
        log_info "Phase $phase_num Step 2: Waiting for capture for event $local_event_id..."
        if wait_for_spike_capture_after_event "$phase_prefix" "$local_event_id" "$capture_timeout" "$LAB_DIR/spikes-${phase_prefix}-capture-poll.json"; then
            log_info "[PASS] Phase $phase_num capture found for event $local_event_id"
            captured=true
            
            # NOW query spikes API for full response (with capture populated)
            query_spikes_api "$LAB_DIR/spikes-${phase_prefix}.json" "lab-tovarisch" "true"
            
            # Create raw row file
            local raw_row_file="$LAB_DIR/${phase_prefix}-spike-row-raw.json"
            echo "$(extract_spike_row_for_event "$LAB_DIR/spikes-${phase_prefix}.json" "$local_event_id")" | jq '.' > "$raw_row_file"
            
            # Normalize raw row into contract row
            local phase_row_file_var="PHASE${phase_num}_SPIKE_ROW_FILE"
            local phase_row_file="${!phase_row_file_var}"
            if normalize_spike_row_capture_contract "$raw_row_file" "$phase_row_file"; then
                # Save spike event (full raw)
                local phase_event_file_var="PHASE${phase_num}_SPIKE_EVENT_FILE"
                local phase_event_file="${!phase_event_file_var}"
                save_phase_spike_event "$phase_num" "$phase_event_file" "$(cat "$LAB_DIR/spikes-${phase_prefix}.json")" "$local_event_id" "$local_reasons"
                
                # Fetch diagnostic packet (no need to clear defect - /status is healthy)
                local metadata_file="$LAB_DIR/${phase_prefix}-capture-metadata.json"
                local fetch_response="$LAB_DIR/${phase_prefix}-capture-fetch-response.json"
                local fetch_summary="$LAB_DIR/${phase_prefix}-capture-fetch-summary.json"
                local packet_file_var="PHASE${phase_num}_CAPTURE_PACKET_FILE"
                local packet_file="${!packet_file_var}"
                
                if fetch_capture_packet "$phase_num" "$metadata_file" "$packet_file" "$fetch_response" "$fetch_summary"; then
                    # Save contract summary
                    local contract_file_var="PHASE${phase_num}_CAPTURE_CONTRACT_FILE"
                    local contract_file="${!contract_file_var}"
                    save_phase_contract_summary "$phase_num" "$contract_file" "$phase_row_file" "$packet_file"
                    
                    # Assert contract
                    if assert_captured_row_contract "$phase_num" "$phase_row_file" "$packet_file"; then
                        log_info "[PASS] Phase $phase_num contract assertions passed"
                        return 0
                    fi
                else
                    log_error "[FAIL] Phase $phase_num: failed to fetch diagnostic packet"
                fi
            fi
        else
            log_error "[FAIL] Phase $phase_num capture missing for event_id=$local_event_id"
        fi
    else
        log_error "[FAIL] Phase $phase_num no failure spike event found"
    fi
    
    return 1
}
