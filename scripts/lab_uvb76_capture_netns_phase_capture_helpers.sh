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
    
    # For lab-probe mode, verify effective probe URL returns 503 and /status returns 200
    if [[ "$defect_mode" == "$DEFECT_MODE_LAB_PROBE" ]]; then
        # Extract effective probe URL from saved artifact
        local effective_probe_url
        effective_probe_url=$(jq -r '.effective_probe_url // empty' "$EFFECTIVE_PROBE_URL_FILE" 2>/dev/null)
        
        if [[ -z "$effective_probe_url" || "$effective_probe_url" == "empty" ]]; then
            log_error "[FAIL] Effective probe URL not found in artifact"
            return 1
        fi
        
        log_info "Using effective probe URL from artifact: $effective_probe_url"
        
        # Verify effective probe URL returns 503 during defect
        local probe_status
        probe_status=$(ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" \
            "$effective_probe_url" 2>/dev/null)
        if [[ "$probe_status" != "503" ]]; then
            log_error "[FAIL] Lab probe defect not working - effective probe URL returned $probe_status, expected 503"
            return 1
        fi
        log_info "[PASS] Lab probe defect verified - effective probe URL returns 503"
        
        # Verify /status remains healthy during defect (for diagnostics)
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
                
                # CRITICAL: Extract network_diag from spike row FIRST (event-specific stored capture)
                # This is the canonical stored capture artifact, NOT a live /status fetch
                # The spike row .captures[].network_diag is the event-specific evidence
                local phase_fetch_summary="$LAB_DIR/${phase_prefix}-capture-fetch-summary.json"
                if extract_network_diag_from_spike_row "$phase_num" "$raw_row_file" "$packet_file" "$phase_fetch_summary"; then
                    log_info "[PASS] Phase $phase_num: extracted network_diag from stored spike row (event-specific capture)"
                    # is_fallback=false by design - we used the stored artifact
                    
                    # Save contract summary
                    local contract_file_var="PHASE${phase_num}_CAPTURE_CONTRACT_FILE"
                    local contract_file="${!contract_file_var}"
                    save_phase_contract_summary "$phase_num" "$contract_file" "$phase_row_file" "$packet_file"
                    
                    # Assert contract
                    if assert_captured_row_contract "$phase_num" "$phase_row_file" "$packet_file"; then
                        log_info "[PASS] Phase $phase_num contract assertions passed"
                        # Write success artifact (for caller to detect success vs failure)
                        touch "$LAB_DIR/phase${phase_num}-row-assertion-ok"
                        return 0
                    else
                        log_error "[FAIL] Phase $phase_num: row assertion FAILED"
                        # Write failure artifact (for caller to detect assertion failure vs capture failure)
                        touch "$LAB_DIR/phase${phase_num}-row-assertion-failed"
                        # Write debug artifact with field paths read by assertion
                        {
                            echo "=== Phase $phase_num Row Assertion Debug ==="
                            echo "timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
                            echo "row_file: $phase_row_file"
                            echo "packet_file: $packet_file"
                            echo "--- Fields read by assert_captured_row_contract ---"
                            echo "capture_status: $(jq -r '.capture_status // "unknown"' "$phase_row_file" 2>/dev/null)"
                            echo "capture_exists: $(jq -r '.capture_exists // "unknown"' "$phase_row_file" 2>/dev/null)"
                            echo "is_protected: $(jq -r '.is_protected // "unknown"' "$phase_row_file" 2>/dev/null)"
                            echo "cooldown_info: $(jq -r '.cooldown_info // "null"' "$phase_row_file" 2>/dev/null)"
                            echo "network_diag present: $(jq -r '.network_diag != null' "$packet_file" 2>/dev/null)"
                        } > "$LAB_DIR/phase${phase_num}-row-assertion-debug.txt" 2>/dev/null || true
                        return 1
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
