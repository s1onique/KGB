#!/bin/bash
# lab_uvb76_capture_netns_contract.sh — Contract verification helpers for UVB-76 capture netns lab
#
# Contract verification functions for diagnostic packet contract validation.
# Sources by lab_uvb76_capture_netns_lib.sh.

# Contract verification helper functions

# Extract spike row from spikes API response for a specific event
# Returns the spike row JSON that includes capture info
extract_spike_row_for_event() {
    local spikes_file="$1"
    local event_id="$2"
    
    local spike_row
    spike_row=$(jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' "$spikes_file" 2>/dev/null || echo "null")
    
    echo "$spike_row"
}

# Save phase spike event artifact
save_phase_spike_event() {
    local phase_num="$1"
    local phase_event_file="$2"
    local spikes_response="$3"
    local event_id="$4"
    local reasons="$5"
    
    log_info "Saving phase $phase_num spike event: event_id=$event_id reasons=$reasons"
    
    # Extract the spike row for this event
    local spike_row
    spike_row=$(echo "$spikes_response" | jq --argjson idx 0 \
        '[.spikes[] | select(.event_id == $eid)] | .[0]' --arg eid "$event_id" 2>/dev/null || echo "{}")
    
    # Write spike event artifact with metadata
    jq -n \
        --arg phase "phase${phase_num}" \
        --argjson event_id "$event_id" \
        --argjson reasons "$reasons" \
        --argjson timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --argjson spike_row "$spike_row" \
        '{
            phase: $phase,
            event_id: $event_id,
            reasons: $reasons,
            timestamp: $timestamp,
            spike_row: $spike_row
        }' > "$phase_event_file"
    
    log_info "Phase $phase_num spike event saved: $phase_event_file"
}

# Save phase spike row artifact (just the row from spikes API)
save_phase_spike_row() {
    local phase_num="$1"
    local phase_row_file="$2"
    local spikes_file="$3"
    local event_id="$4"
    
    log_info "Saving phase $phase_num spike row: event_id=$event_id"
    
    # Extract the spike row for this event
    local spike_row
    spike_row=$(jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' "$spikes_file" 2>/dev/null || echo "{}")
    
    echo "$spike_row" | jq '.' > "$phase_row_file"
    
    log_info "Phase $phase_num spike row saved: $phase_row_file"
}

# Save capture packet artifact from a spike row
# Extracts the network_diag from the capture within the spike row
save_capture_packet() {
    local phase_num="$1"
    local packet_file="$2"
    local spike_row_file="$3"
    
    log_info "Extracting capture packet from spike row for phase $phase_num"
    
    # Extract network_diag from the capture within spike row
    local network_diag
    network_diag=$(jq '[.captures[] | select(.network_diag != null)] | .[0] | .network_diag' "$spike_row_file" 2>/dev/null || echo "null")
    
    # Write as standalone packet artifact
    jq -n \
        --arg phase "phase${phase_num}" \
        --argjson network_diag "$network_diag" \
        --argjson timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            network_diag: $network_diag,
            timestamp: $timestamp
        }' > "$packet_file"
    
    log_info "Phase $phase_num capture packet saved: $packet_file"
}

# Save contract summary for a phase
save_phase_contract_summary() {
    local phase_num="$1"
    local contract_file="$2"
    local spike_row_file="$3"
    local packet_file="$4"
    
    log_info "Generating contract summary for phase $phase_num"
    
    local capture_status="unknown"
    local capture_exists="false"
    local is_protected="false"
    local cooldown_info_present="false"
    local last_successful_capture_at_present="false"
    local next_capture_eligible_at_present="false"
    local network_diag_present="false"
    local packet_contract_ok="false"
    
    if [[ -f "$spike_row_file" ]]; then
        # Get capture status from the spike row's capture info
        # The spike row may have capture_info or captures[]
        local capture_info
        capture_info=$(jq '[.captures[] | select(.capture_status != null)] | .[0] | {capture_status: .capture_status, capture_exists: .capture_exists, is_protected: .is_protected, cooldown_info: .cooldown_info}' "$spike_row_file" 2>/dev/null || echo "{}")
        
        if [[ "$capture_info" != "{}" && "$capture_info" != "null" ]]; then
            capture_status=$(jq -r '.capture_status // "unknown"' <<< "$capture_info" 2>/dev/null || echo "unknown")
            capture_exists=$(jq -r '.capture_exists // false' <<< "$capture_info" 2>/dev/null || echo "false")
            is_protected=$(jq -r '.is_protected // false' <<< "$capture_info" 2>/dev/null || echo "false")
            
            if jq -e '.cooldown_info != null' <<< "$capture_info" >/dev/null 2>&1; then
                cooldown_info_present="true"
                
                if jq -e '.cooldown_info.last_successful_capture_at != null' <<< "$capture_info" >/dev/null 2>&1; then
                    last_successful_capture_at_present="true"
                fi
                
                if jq -e '.cooldown_info.next_capture_eligible_at != null' <<< "$capture_info" >/dev/null 2>&1; then
                    next_capture_eligible_at_present="true"
                fi
            fi
        fi
    fi
    
    if [[ -f "$packet_file" ]]; then
        if jq -e '.network_diag != null' "$packet_file" >/dev/null 2>&1; then
            network_diag_present="true"
        fi
        
        # Run packet shape verification
        if "${SCRIPT_DIR}/verify_uvb76_diag_packet_contract.sh" --capture "$packet_file" --phase "phase${phase_num}" >/dev/null 2>&1; then
            packet_contract_ok="true"
        fi
    fi
    
    jq -n \
        --arg phase "phase${phase_num}" \
        --arg capture_status "$capture_status" \
        --argjson capture_exists "$capture_exists" \
        --argjson is_protected "$is_protected" \
        --argjson cooldown_info_present "$cooldown_info_present" \
        --argjson last_successful_capture_at_present "$last_successful_capture_at_present" \
        --argjson next_capture_eligible_at_present "$next_capture_eligible_at_present" \
        --argjson network_diag_present "$network_diag_present" \
        --argjson packet_contract_ok "$packet_contract_ok" \
        --argjson timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            capture_status: $capture_status,
            capture_exists: $capture_exists,
            is_protected: $is_protected,
            cooldown_info_present: $cooldown_info_present,
            last_successful_capture_at_present: $last_successful_capture_at_present,
            next_capture_eligible_at_present: $next_capture_eligible_at_present,
            network_diag_present: $network_diag_present,
            packet_contract_ok: $packet_contract_ok,
            timestamp: $timestamp
        }' > "$contract_file"
    
    log_info "Phase $phase_num contract summary saved: $contract_file"
}

# Run contract verifier on all phase artifacts
run_contract_verification() {
    local output_file="$1"
    local lab_dir="$2"
    
    log_info "Running contract verification on artifacts: $lab_dir"
    
    # Find the verifier script
    local verifier="${SCRIPT_DIR}/verify_uvb76_diag_packet_contract.sh"
    
    if [[ ! -x "$verifier" ]]; then
        log_warn "Contract verifier not found or not executable: $verifier"
        return 0
    fi
    
    # Run verifier on all phase spike rows
    {
        echo "=== Contract Verification Output ==="
        echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "Lab directory: $lab_dir"
        echo ""
        
        for phase_row in "$lab_dir"/phase*-spike-row.json; do
            [[ -f "$phase_row" ]] || continue
            
            echo "--- Verifying: $(basename "$phase_row") ---"
            if "$verifier" --row "$phase_row" --phase "$(basename "$phase_row" .json)" 2>&1; then
                echo "[PASS] Spike row contract verified"
            else
                echo "[FAIL] Spike row contract FAILED"
            fi
            echo ""
        done
        
        for packet in "$lab_dir"/phase*-capture-packet.json; do
            [[ -f "$packet" ]] || continue
            
            echo "--- Verifying: $(basename "$packet") ---"
            if "$verifier" --capture "$packet" --phase "$(basename "$packet" .json)" 2>&1; then
                echo "[PASS] Packet contract verified"
            else
                echo "[FAIL] Packet contract FAILED"
            fi
            echo ""
        done
        
        echo "=== Contract Verification Complete ==="
    } > "$output_file" 2>&1 || true
    
    log_info "Contract verification output: $output_file"
}

# Assert contract for a captured row
assert_captured_row_contract() {
    local phase="$1"
    local spike_row_file="$2"
    local packet_file="$3"
    
    local ok=true
    
    log_info "Asserting captured row contract for phase $phase"
    
    # Check spike row has capture_status == captured
    local capture_status
    capture_status=$(jq '[.captures[] | select(.capture_status != null)] | .[0] | .capture_status // "unknown"' "$spike_row_file" 2>/dev/null || echo "unknown")
    
    if [[ "$capture_status" != "captured" ]]; then
        log_error "[FAIL] Phase $phase: expected capture_status=captured, got: $capture_status"
        ok=false
    fi
    
    # Check packet exists and has network_diag
    if [[ ! -f "$packet_file" ]]; then
        log_error "[FAIL] Phase $phase: capture packet file not found: $packet_file"
        ok=false
    elif ! jq -e '.network_diag != null' "$packet_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: capture packet missing network_diag"
        ok=false
    fi
    
    [[ "$ok" == "true" ]] && log_info "[PASS] Phase $phase: captured row contract satisfied"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

# Assert contract for a skipped_cooldown row
assert_skipped_cooldown_row_contract() {
    local phase="$1"
    local spike_row_file="$2"
    
    local ok=true
    
    log_info "Asserting skipped_cooldown row contract for phase $phase"
    
    # Extract capture info
    local capture_info
    capture_info=$(jq '[.captures[] | select(.capture_status != null)] | .[0]' "$spike_row_file" 2>/dev/null || echo "{}")
    
    if [[ "$capture_info" == "{}" || "$capture_info" == "null" ]]; then
        log_error "[FAIL] Phase $phase: no capture info found in spike row"
        return 1
    fi
    
    local capture_status
    capture_status=$(jq -r '.capture_status // "unknown"' <<< "$capture_info" 2>/dev/null || echo "unknown")
    
    # capture_status must be skipped_cooldown
    if [[ "$capture_status" != "skipped_cooldown" ]]; then
        log_error "[FAIL] Phase $phase: expected capture_status=skipped_cooldown, got: $capture_status"
        ok=false
    fi
    
    # cooldown_info must be present
    if ! jq -e '.cooldown_info != null' <<< "$capture_info" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info"
        ok=false
    fi
    
    # cooldown_info must have last_successful_capture_at
    if ! jq -e '.cooldown_info.last_successful_capture_at != null' <<< "$capture_info" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info.last_successful_capture_at"
        ok=false
    fi
    
    # cooldown_info must have next_capture_eligible_at
    if ! jq -e '.cooldown_info.next_capture_eligible_at != null' <<< "$capture_info" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info.next_capture_eligible_at"
        ok=false
    fi
    
    [[ "$ok" == "true" ]] && log_info "[PASS] Phase $phase: skipped_cooldown row contract satisfied"
    [[ "$ok" == "true" ]] && return 0 || return 1
}
