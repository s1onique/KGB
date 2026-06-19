#!/bin/bash
# lab_uvb76_capture_netns_phase_capture_helpers.sh — Phase capture helpers for netns lab
#
# Extracted helper functions to keep the main lab script under LLM-friendly limits.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Wait for spike event and then capture, then fetch diagnostic packet.
# Returns 0 on success, sets CONTRACT_*_CAPTURE_OK and CONTRACT_*_PACKET_OK globals.
wait_and_fetch_capture_with_defect_clear() {
    local phase_num="$1"
    local phase_prefix="$2"
    local phase_event_id_var="$3"  # e.g., "PHASE1_EVENT_ID"
    local phase_reasons_var="$4"   # e.g., "PHASE1_REASONS"
    local phase_cursor="$5"        # e.g., "$PHASE_PHASE1_CURSOR"
    local spike_timeout="${6:-30}"
    local capture_timeout="${7:-15}"
    local defect_mode="${8:-${DEFECT_MODE_CLEAR_BEFORE_FETCH}}"
    
    # Declare variables to hold outputs
    local local_event_id=""
    local local_reasons=""
    local captured=false
    local packet_ok=false
    
    # Inject defect to trigger spike
    inject_netem_defect "$defect_mode"
    
    # Verify defect is in place
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_error "[FAIL] Defect not working - ping succeeded when it should fail"
        return 1
    fi
    log_info "[PASS] Defect verified - ping fails as expected"
    
    # STEP 1: Wait for HTTP failure spike EVENT
    log_info "Phase $phase_num Step 1: Waiting for failure spike event..."
    if wait_for_spike_event_after_cursor "$phase_prefix" "$phase_cursor" "http_probe_timeout|http_probe_failure|http_probe_connection_refused" "$spike_timeout" "$LAB_DIR/spikes-${phase_prefix}-poll.json"; then
        local_event_id="$MATCHED_EVENT_ID"
        local_reasons="$MATCHED_REASONS"
        log_info "[PASS] Phase $phase_num spike event found: event_id=$local_event_id reasons=$local_reasons"
        
        # Export the event_id and reasons to caller
        export "$phase_event_id_var=$local_event_id"
        export "$phase_reasons_var=$local_reasons"
        
        # STEP 2: Wait for the CAPTURE for this event
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
                
                # CRITICAL: Clear defect BEFORE fetching diagnostic packet
                log_info "Clearing defect before diagnostic packet fetch..."
                clear_defect
                sleep 1
                
                # Fetch diagnostic packet
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
