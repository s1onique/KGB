#!/bin/bash
# lab_uvb76_capture_netns_result.sh — Result writing for UVB-76 capture netns lab
#
# Writes result.json artifact with lab outcome including contract booleans.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Write result.json with valid JSON output using jq
write_result() {
    log_info "Writing result.json..."

    # Use jq -n for valid JSON output
    local uvb76_pid_json="null"
    local tovarisch_pid_json="null"
    if [[ -n "$UVB76_PID" ]]; then
        uvb76_pid_json="$UVB76_PID"
    fi
    if [[ -n "$TOVARISCH_PID" ]]; then
        tovarisch_pid_json="$TOVARISCH_PID"
    fi

    # Compute ok boolean - must include all contract checks
    local ok_val=false
    if [[ "$PROBE_READY" == true && "$CONTRACT_PHASE1_CAPTURE_OK" == true && \
          "$CONTRACT_PHASE1_PACKET_OK" == true && "$CONTRACT_PHASE2_COOLDOWN_OK" == true && \
          "$CONTRACT_PHASE3_CAPTURE_OK" == true && "$CONTRACT_PHASE3_PACKET_OK" == true && \
          "$CONTRACT_DIR_OK" == true && "$CONTRACT_DISTINCT_EVENT_IDS_OK" == true ]]; then
        ok_val=true
    fi

    jq -n \
        --arg artifact_dir "$LAB_DIR" \
        --arg defect_mode "$DEFECT_MODE" \
        --argjson ok "$ok_val" \
        --argjson probe_ready "$PROBE_READY" \
        --argjson phase1_capture_contract_ok "$CONTRACT_PHASE1_CAPTURE_OK" \
        --argjson phase1_packet_contract_ok "$CONTRACT_PHASE1_PACKET_OK" \
        --argjson phase2_cooldown_contract_ok "$CONTRACT_PHASE2_COOLDOWN_OK" \
        --argjson phase3_capture_contract_ok "$CONTRACT_PHASE3_CAPTURE_OK" \
        --argjson phase3_packet_contract_ok "$CONTRACT_PHASE3_PACKET_OK" \
        --argjson dir_contract_ok "$CONTRACT_DIR_OK" \
        --argjson distinct_event_ids_ok "$CONTRACT_DISTINCT_EVENT_IDS_OK" \
        --argjson uvb76_pid "$uvb76_pid_json" \
        --argjson tovarisch_pid "$tovarisch_pid_json" \
        '{
            ok: $ok,
            probe_ready: $probe_ready,
            artifact_dir: $artifact_dir,
            uvb76_pid: $uvb76_pid,
            tovarisch_pid: $tovarisch_pid,
            defect_mode: $defect_mode,
            contract: {
                phase1_capture_contract_ok: $phase1_capture_contract_ok,
                phase1_packet_contract_ok: $phase1_packet_contract_ok,
                phase2_cooldown_contract_ok: $phase2_cooldown_contract_ok,
                phase3_capture_contract_ok: $phase3_capture_contract_ok,
                phase3_packet_contract_ok: $phase3_packet_contract_ok,
                dir_contract_ok: $dir_contract_ok,
                distinct_event_ids_ok: $distinct_event_ids_ok
            }
        }' > "$RESULT_FILE"

    log_info "Result written to $RESULT_FILE"

    # Validate the output is valid JSON
    if ! jq . "$RESULT_FILE" > /dev/null 2>&1; then
        log_error "result.json is not valid JSON!"
        return 1
    fi
}
