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
            requested_path_after_recovery: $requested_path_after_recovery
        }' > "$RESULT_FILE"

    log_info "Result written to $RESULT_FILE"

    # Validate the output is valid JSON
    if ! jq . "$RESULT_FILE" > /dev/null 2>&1; then
        log_error "result.json is not valid JSON!"
        return 1
    fi
}
