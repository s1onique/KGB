#!/bin/bash
# lab_uvb76_capture_netns_result.sh — Result writing for UVB-76 capture netns lab
#
# Writes result.json artifact with lab outcome.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Write result.json with valid JSON output using jq
write_result() {
    log_info "Writing result.json..."

    # Compute ok boolean properly - probe_readiness is now required
    local ok_val=false
    if [[ "$PROBE_READY" == true && "$BASELINE_CAPTURE_OK" == true && "$DEFECT_OBSERVED" == true && "$RECOVERY_CAPTURE_OK" == true ]]; then
        ok_val=true
    fi

    # Use jq -n for valid JSON output
    local uvb76_pid_json="null"
    local tovarisch_pid_json="null"
    if [[ -n "$UVB76_PID" ]]; then
        uvb76_pid_json="$UVB76_PID"
    fi
    if [[ -n "$TOVARISCH_PID" ]]; then
        tovarisch_pid_json="$TOVARISCH_PID"
    fi

    # Collect event info from capture files if available
    # Prefer summary files (which contain event_id/reasons) over raw capture objects
    local defect_event_id="null"
    local defect_reasons="null"
    local defect_failure_reason="null"
    local recovery_event_id="null"
    local recovery_reasons="null"
    local recovery_failure_reason="null"

    # Read defect capture info - prefer summary file if it exists
    local defect_info_file="$CAPTURE_DURING_DEFECT_FILE"
    [[ -f "${CAPTURE_DURING_DEFECT_FILE}.summary" ]] && defect_info_file="${CAPTURE_DURING_DEFECT_FILE}.summary"
    if [[ -f "$defect_info_file" ]]; then
        defect_event_id=$(jq -r '.event_id // null' "$defect_info_file" 2>/dev/null || echo "null")
        defect_reasons=$(jq -r '.reasons // null' "$defect_info_file" 2>/dev/null || echo "null")
        defect_failure_reason=$(jq -r '.failure_reason // null' "$defect_info_file" 2>/dev/null || echo "null")
    fi

    # Read recovery capture info - prefer summary file if it exists
    local recovery_info_file="$CAPTURE_AFTER_RECOVERY_FILE"
    [[ -f "${CAPTURE_AFTER_RECOVERY_FILE}.summary" ]] && recovery_info_file="${CAPTURE_AFTER_RECOVERY_FILE}.summary"
    if [[ -f "$recovery_info_file" ]]; then
        recovery_event_id=$(jq -r '.event_id // null' "$recovery_info_file" 2>/dev/null || echo "null")
        recovery_reasons=$(jq -r '.reasons // null' "$recovery_info_file" 2>/dev/null || echo "null")
        recovery_failure_reason=$(jq -r '.failure_reason // null' "$recovery_info_file" 2>/dev/null || echo "null")
    fi

    jq -n \
        --arg artifact_dir "$LAB_DIR" \
        --arg defect_mode "$DEFECT_MODE" \
        --arg requested_path_baseline "$REQUESTED_PATH_BASELINE" \
        --arg requested_path_during_defect "$REQUESTED_PATH_DURING_DEFECT" \
        --arg requested_path_after_recovery "$REQUESTED_PATH_AFTER_RECOVERY" \
        --argjson uvb76_pid "$uvb76_pid_json" \
        --argjson tovarisch_pid "$tovarisch_pid_json" \
        --argjson ok "$ok_val" \
        --argjson probe_ready "$PROBE_READY" \
        --argjson baseline_capture_ok "$BASELINE_CAPTURE_OK" \
        --argjson defect_observed "$DEFECT_OBSERVED" \
        --argjson recovery_capture_ok "$RECOVERY_CAPTURE_OK" \
        --arg defect_event_id "${defect_event_id:-null}" \
        --arg defect_reasons "${defect_reasons:-null}" \
        --arg defect_failure_reason "${defect_failure_reason:-null}" \
        --arg recovery_event_id "${recovery_event_id:-null}" \
        --arg recovery_reasons "${recovery_reasons:-null}" \
        --arg recovery_failure_reason "${recovery_failure_reason:-null}" \
        '{
            ok: $ok,
            probe_ready: $probe_ready,
            baseline_capture_ok: $baseline_capture_ok,
            defect_observed: $defect_observed,
            recovery_capture_ok: $recovery_capture_ok,
            artifact_dir: $artifact_dir,
            uvb76_pid: $uvb76_pid,
            tovarisch_pid: $tovarisch_pid,
            defect_mode: $defect_mode,
            requested_path_baseline: $requested_path_baseline,
            requested_path_during_defect: $requested_path_during_defect,
            requested_path_after_recovery: $requested_path_after_recovery,
            defect_event_id: (if $defect_event_id == "null" then null else $defect_event_id end),
            defect_reasons: (if $defect_reasons == "null" then null else $defect_reasons end),
            defect_failure_reason: (if $defect_failure_reason == "null" then null else $defect_failure_reason end),
            recovery_event_id: (if $recovery_event_id == "null" then null else $recovery_event_id end),
            recovery_reasons: (if $recovery_reasons == "null" then null else $recovery_reasons end),
            recovery_failure_reason: (if $recovery_failure_reason == "null" then null else $recovery_failure_reason end)
        }' > "$RESULT_FILE"

    log_info "Result written to $RESULT_FILE"

    # Validate the output is valid JSON
    if ! jq . "$RESULT_FILE" > /dev/null 2>&1; then
        log_error "result.json is not valid JSON!"
        return 1
    fi
}
