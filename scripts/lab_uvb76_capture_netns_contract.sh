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
    local phase_num="${1:-unknown}"
    local phase_event_file="${2:-}"
    local spikes_response="${3:-}"
    local event_id="${4:-unknown}"
    local reasons="${5:-unknown}"
    
    # Validate required arguments
    if [[ -z "$phase_event_file" ]]; then
        log_error "[FAIL] save_phase_spike_event: missing phase_event_file argument"
        return 1
    fi
    
    log_info "Saving phase $phase_num spike event: event_id=$event_id reasons=$reasons"
    
    # Extract the spike row for this event
    local spike_row="{}"
    if [[ -n "$spikes_response" ]]; then
        spike_row=$(echo "$spikes_response" | jq --arg eid "$event_id" \
            '[.spikes[] | select(.event_id == $eid)] | .[0] // {}' 2>/dev/null || echo "{}")
    fi
    
    # Write spike event artifact with metadata
    jq -n \
        --arg phase "phase${phase_num}" \
        --arg event_id "$event_id" \
        --arg reasons "$reasons" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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
    local phase_num="${1:-unknown}"
    local phase_row_file="${2:-}"
    local spikes_file="${3:-}"
    local event_id="${4:-unknown}"
    
    # Validate required arguments
    if [[ -z "$phase_row_file" ]]; then
        log_error "[FAIL] save_phase_spike_row: missing phase_row_file argument"
        return 1
    fi
    if [[ -z "$spikes_file" ]]; then
        log_error "[FAIL] save_phase_spike_row: missing spikes_file argument"
        return 1
    fi
    
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
    local phase_num="${1:-unknown}"
    local packet_file="${2:-}"
    local spike_row_file="${3:-}"
    
    # Validate required arguments
    if [[ -z "$packet_file" ]]; then
        log_error "[FAIL] save_capture_packet: missing packet_file argument"
        return 1
    fi
    if [[ -z "$spike_row_file" ]]; then
        log_error "[FAIL] save_capture_packet: missing spike_row_file argument"
        return 1
    fi
    if [[ ! -f "$spike_row_file" ]]; then
        log_error "[FAIL] save_capture_packet: file not found: $spike_row_file"
        return 1
    fi
    
    log_info "Extracting capture packet from spike row for phase $phase_num"
    
    # Extract network_diag from the capture within spike row
    local network_diag="null"
    network_diag=$(jq '[.captures[]? | select(.network_diag != null)] | .[0] | .network_diag // null' "$spike_row_file" 2>/dev/null || echo "null")
    
    if [[ "$network_diag" == "null" || "$network_diag" == "" ]]; then
        log_error "[FAIL] Phase $phase_num: no network_diag found in spike row"
        return 1
    fi
    
    # Write as standalone packet artifact
    jq -n \
        --arg phase "phase${phase_num}" \
        --argjson network_diag "$network_diag" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            network_diag: $network_diag,
            timestamp: $timestamp
        }' > "$packet_file"
    
    log_info "Phase $phase_num capture packet saved: $packet_file"
}

# Save contract summary for a phase
save_phase_contract_summary() {
    local phase_num="${1:-unknown}"
    local contract_file="${2:-}"
    local spike_row_file="${3:-}"
    local packet_file="${4:-}"
    
    # Validate required arguments
    if [[ -z "$contract_file" ]]; then
        log_error "[FAIL] save_phase_contract_summary: missing contract_file argument"
        return 1
    fi
    
    log_info "Generating contract summary for phase $phase_num"
    
    local capture_status="unknown"
    local capture_exists="false"
    local is_protected="false"
    local cooldown_info_present="false"
    local last_successful_capture_at_present="false"
    local next_capture_eligible_at_present="false"
    local network_diag_present="false"
    local packet_contract_ok="false"
    
    if [[ -n "$spike_row_file" && -f "$spike_row_file" ]]; then
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
    
    if [[ -n "$packet_file" && -f "$packet_file" ]]; then
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
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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

# TCP Diagnostics contract tracking (exported for result.sh to use)
declare -g TCP_CONTRACT_PHASE1_OK=false
declare -g TCP_CONTRACT_PHASE3_OK=false

# Run contract verifier on all phase artifacts
# Sets TCP_CONTRACT_PHASE*_OK variables for result.sh to use
run_contract_verification() {
    local output_file="${1:-}"
    local lab_dir="${2:-}"
    
    # Validate required arguments
    if [[ -z "$output_file" ]]; then
        log_error "[FAIL] run_contract_verification: missing output_file argument"
        return 1
    fi
    if [[ -z "$lab_dir" ]]; then
        log_error "[FAIL] run_contract_verification: missing lab_dir argument"
        return 1
    fi
    if [[ ! -d "$lab_dir" ]]; then
        log_error "[FAIL] run_contract_verification: lab directory not found: $lab_dir"
        return 1
    fi
    
    log_info "Running contract verification on artifacts: $lab_dir"
    
    # Find the verifier script
    local verifier="${SCRIPT_DIR}/verify_uvb76_diag_packet_contract.sh"
    
    if [[ ! -x "$verifier" ]]; then
        log_warn "Contract verifier not found or not executable: $verifier"
        return 0
    fi
    
    # Reset TCP contract tracking
    TCP_CONTRACT_PHASE1_OK=false
    TCP_CONTRACT_PHASE3_OK=false
    
    local overall_ok=true
    local packet_verification_failed=false
    
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
                overall_ok=false
            fi
            echo ""
        done
        
        for packet in "$lab_dir"/phase*-capture-packet.json; do
            [[ -f "$packet" ]] || continue
            
            local phase_name
            phase_name=$(basename "$packet" .json)
            
            echo "--- Verifying: $(basename "$packet") ---"
            local packet_ec=0
            if "$verifier" --capture "$packet" --phase "$phase_name" 2>&1; then
                echo "[PASS] Packet contract verified (shape + TCP diagnostics)"
                # Track TCP contract status for phase1 and phase3
                if [[ "$phase_name" == "phase1-capture-packet" ]]; then
                    TCP_CONTRACT_PHASE1_OK=true
                elif [[ "$phase_name" == "phase3-capture-packet" ]]; then
                    TCP_CONTRACT_PHASE3_OK=true
                fi
            else
                echo "[FAIL] Packet contract FAILED (shape and/or TCP diagnostics)"
                overall_ok=false
                packet_verification_failed=true
            fi
            echo ""
        done
        
        echo "=== Contract Verification Complete ==="
    } > "$output_file" 2>&1 || true
    
    # Log TCP contract status for visibility
    log_info "TCP diagnostics contract status:"
    log_info "  Phase 1 capture packet: $TCP_CONTRACT_PHASE1_OK"
    log_info "  Phase 3 capture packet: $TCP_CONTRACT_PHASE3_OK"
    
    log_info "Contract verification output: $output_file"
    
    # Return failure if any row or packet verification failed
    if [[ "$overall_ok" != "true" ]]; then
        return 1
    fi
    return 0
}

# Assert contract for a captured row.
#
# Reads from the NORMALIZED row shape produced by normalize_spike_row_capture_contract().
# The normalized row has root-level fields:
#   - capture_status: must be "captured"
#   - capture_exists: must be true
#   - is_protected: must be true
#   - cooldown_info: must be null (captured rows do NOT have cooldown info)
#
# IMPORTANT: DO NOT read from .captures[0] paths - those are raw API shapes.
# The normalizer extracts and canonicalizes them to root-level for assertions.
assert_captured_row_contract() {
    local phase="$1"
    local spike_row_file="$2"
    local packet_file="$3"
    
    local ok=true
    
    log_info "Asserting captured row contract for phase $phase"
    
    # Check spike row has capture_status == captured (root-level normalized field)
    local capture_status
    capture_status=$(jq -r '.capture_status // "unknown"' "$spike_row_file" 2>/dev/null || echo "unknown")
    
    if [[ "$capture_status" != "captured" ]]; then
        log_error "[FAIL] Phase $phase: expected capture_status=captured, got: $capture_status"
        ok=false
    fi
    
    # Check capture_exists == true (root-level normalized field)
    local capture_exists
    capture_exists=$(jq -r '.capture_exists // false' "$spike_row_file" 2>/dev/null || echo "false")
    if [[ "$capture_exists" != "true" ]]; then
        log_error "[FAIL] Phase $phase: expected capture_exists=true, got: $capture_exists"
        ok=false
    fi
    
    # Check is_protected == true (root-level normalized field)
    local is_protected
    is_protected=$(jq -r '.is_protected // false' "$spike_row_file" 2>/dev/null || echo "false")
    if [[ "$is_protected" != "true" ]]; then
        log_error "[FAIL] Phase $phase: expected is_protected=true, got: $is_protected"
        ok=false
    fi
    
    # Check cooldown_info is null (captured rows do NOT have cooldown info)
    if jq -e '.cooldown_info != null' "$spike_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: captured row must NOT have cooldown_info"
        ok=false
    fi
    
    # Check packet exists and has network_diag (real diagnostic data)
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

# Assert contract for a skipped_cooldown row (validates normalized root-level row)
assert_skipped_cooldown_row_contract() {
    local phase="$1"
    local spike_row_file="$2"
    
    local ok=true
    
    log_info "Asserting skipped_cooldown row contract for phase $phase"
    
    # Validate normalized root-level row
    # capture_status must be skipped_cooldown
    if ! jq -e '.capture_status == "skipped_cooldown"' "$spike_row_file" >/dev/null 2>&1; then
        local actual_status
        actual_status=$(jq -r '.capture_status // "null"' "$spike_row_file" 2>/dev/null)
        log_error "[FAIL] Phase $phase: expected capture_status=skipped_cooldown, got: $actual_status"
        ok=false
    fi
    
    # capture_exists must be false for skipped_cooldown
    if ! jq -e '.capture_exists == false' "$spike_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: expected capture_exists=false for skipped_cooldown"
        ok=false
    fi
    
    # cooldown_info must be present
    if ! jq -e '.cooldown_info != null' "$spike_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info"
        ok=false
    fi
    
    # cooldown_info must have last_successful_capture_at
    if ! jq -e '.cooldown_info.last_successful_capture_at != null' "$spike_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info.last_successful_capture_at"
        ok=false
    fi
    
    # cooldown_info must have next_capture_eligible_at
    if ! jq -e '.cooldown_info.next_capture_eligible_at != null' "$spike_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: skipped_cooldown row missing cooldown_info.next_capture_eligible_at"
        ok=false
    fi
    
    [[ "$ok" == "true" ]] && log_info "[PASS] Phase $phase: skipped_cooldown row contract satisfied"
    [[ "$ok" == "true" ]] && return 0 || return 1
}
