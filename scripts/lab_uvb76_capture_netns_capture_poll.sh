#!/bin/bash
# lab_uvb76_capture_netns_capture_poll.sh — Capture-specific polling helpers
#
# Functions for polling spike captures and normalizing capture status.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Normalize API capture status to contract canonical status.
# Returns the normalized status string.
normalize_capture_status() {
    local api_status="${1:-}"
    
    # Handle empty string (no argument or explicit empty)
    if [[ -z "$api_status" ]]; then
        echo "pending"
        return
    fi
    
    case "$api_status" in
        ok|captured)      echo "captured" ;;
        timeout|error|failed) echo "failed" ;;
        skipped_cooldown)  echo "skipped_cooldown" ;;
        disabled)          echo "disabled" ;;
        not_configured)    echo "not_configured" ;;
        not_attempted)     echo "not_attempted" ;;
        in_progress|pending) echo "pending" ;;
        *)                 echo "unknown" ;;
    esac
}

# Write failure artifacts to lab directory for debugging.
# Always writes the latest API response and raw row on failure.
_write_capture_failure_artifacts() {
    local phase="${1:-unknown}"
    local last_response="${2:-}"
    local last_raw_row="${3:-}"
    local reason="${4:-unknown}"
    
    # Write full API response
    if [[ -n "$last_response" && -n "${LAB_DIR:-}" ]]; then
        local response_file="${LAB_DIR}/${phase}-capture-fail-response.json"
        echo "$last_response" | jq '.' > "$response_file" 2>/dev/null || true
        log_info "Failure artifact: $response_file"
    fi
    
    # Write raw spike row
    if [[ -n "$last_raw_row" && -n "${LAB_DIR:-}" ]]; then
        local row_file="${LAB_DIR}/${phase}-capture-fail-row.json"
        echo "$last_raw_row" | jq '.' > "$row_file" 2>/dev/null || true
        log_info "Failure artifact: $row_file"
    fi
    
    # Write failure summary
    if [[ -n "${LAB_DIR:-}" ]]; then
        local summary_file="${LAB_DIR}/${phase}-capture-fail-summary.json"
        jq -n \
            --arg phase "$phase" \
            --arg reason "$reason" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{
                phase: $phase,
                status: "failure",
                reason: $reason,
                timestamp: $timestamp
            }' > "$summary_file" 2>/dev/null || true
        log_info "Failure artifact: $summary_file"
    fi
}

# Wait for capture for a specific spike event.
# This is the SECOND step - prove the capture exists for the event.
# Uses the event_id to precisely match the capture.
# Polls every 2s until success or timeout.
# On success, exports MATCHED_CAPTURE_JSON with the actual capture object.
# Returns 0 only if:
#   - capture entry exists
#   - capture status maps to canonical captured/success
#   - network_diag exists and is an object
#   - capture packet can be extracted
# Returns 1 if capture status is timeout, failed, error, disabled, not_configured,
#   not_attempted, skipped_cooldown, or missing network_diag.
wait_for_spike_capture_after_event() {
    local phase="${1:-unknown}"
    local event_id="${2:-}"
    local timeout="${3:-30}"
    local artifact_file="${4:-}"

    log_info "Waiting for capture: phase=$phase event_id=$event_id timeout=${timeout}s"

    if [[ -z "$event_id" ]]; then
        log_error "No event_id provided for capture wait"
        return 1
    fi

    local interval=2
    local elapsed=0
    local success=false
    local last_response=""
    local matched_capture_json=""
    local last_raw_row=""

    while [[ $elapsed -lt $timeout ]]; do
        local spikes_response
        spikes_response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
            "${UVB76_API_URL}/api/v1/latency/spikes?target_id=lab-tovarisch&include_captures=true&limit=20" 2>/dev/null)
        last_response="$spikes_response"

        if [[ -z "$spikes_response" ]] || ! echo "$spikes_response" | jq -e . >/dev/null 2>&1; then
            log_warn "Empty or invalid spikes API response at ${elapsed}s"
        else
            # Find the spike with matching event_id
            local spike
            spike=$(echo "$spikes_response" | jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' 2>/dev/null)

            if [[ "$spike" == "null" || "$spike" == "" ]]; then
                log_info "Event $event_id not found in spikes list (${elapsed}s elapsed)"
            else
                # Save raw row for artifact on failure
                last_raw_row="$spike"
                
                # Check for captures
                local captures_json
                captures_json=$(echo "$spike" | jq '.captures // []' 2>/dev/null)
                local capture_count
                capture_count=$(echo "$captures_json" | jq 'length' 2>/dev/null || echo "0")

                if [[ "$capture_count" -gt 0 ]]; then
                    local cap
                    cap=$(echo "$captures_json" | jq '.[0]' 2>/dev/null)
                    local cap_status
                    cap_status=$(echo "$cap" | jq -r '.status // "unknown"' 2>/dev/null)
                    local cap_started
                    cap_started=$(echo "$cap" | jq -r '.capture_started_at // empty' 2>/dev/null)
                    
                    # Normalize the status
                    local norm_status
                    norm_status=$(normalize_capture_status "$cap_status")
                    
                    log_info "Capture found for event $event_id: status=$cap_status (normalized: $norm_status) started=$cap_started"
                    
                    # For Phase 1/3, we require captured status + network_diag
                    if [[ "$phase" == "phase1" || "$phase" == "phase3" ]]; then
                        if [[ "$norm_status" != "captured" ]]; then
                            log_error "[FAIL] Phase $phase: capture status is '$cap_status' (normalized: $norm_status), expected 'captured' or 'ok'"
                            # Log available keys
                            local available_keys
                            available_keys=$(echo "$cap" | jq 'keys | join(", ")' 2>/dev/null || echo "unknown")
                            log_error "  Available capture keys: $available_keys"
                            # Write failure artifacts to lab directory
                            _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "non_captured_status:${cap_status}"
                            # Also write artifact_file if provided
                            [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
                            return 1
                        fi
                        
                        # Check for network_diag in various possible locations
                        local network_diag_json="null"
                        local network_diag_found=false
                        
                        # Try .network_diag (direct)
                        if echo "$cap" | jq -e '.network_diag != null' >/dev/null 2>&1; then
                            network_diag_json=$(echo "$cap" | jq '.network_diag' 2>/dev/null)
                            network_diag_found=true
                        # Try .packet.network_diag
                        elif echo "$cap" | jq -e '.packet.network_diag != null' >/dev/null 2>&1; then
                            network_diag_json=$(echo "$cap" | jq '.packet.network_diag' 2>/dev/null)
                            network_diag_found=true
                        # Try .diagnostics.network_diag
                        elif echo "$cap" | jq -e '.diagnostics.network_diag != null' >/dev/null 2>&1; then
                            network_diag_json=$(echo "$cap" | jq '.diagnostics.network_diag' 2>/dev/null)
                            network_diag_found=true
                        # Try .network_diag_packet.network_diag
                        elif echo "$cap" | jq -e '.network_diag_packet.network_diag != null' >/dev/null 2>&1; then
                            network_diag_json=$(echo "$cap" | jq '.network_diag_packet.network_diag' 2>/dev/null)
                            network_diag_found=true
                        fi
                        
                        if [[ "$network_diag_found" != "true" ]]; then
                            log_error "[FAIL] Phase $phase: no network_diag found in capture entry"
                            # Log available keys
                            local available_keys
                            available_keys=$(echo "$cap" | jq 'keys | join(", ")' 2>/dev/null || echo "unknown")
                            log_error "  Available capture keys: $available_keys"
                            # Write failure artifacts to lab directory
                            _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "missing_network_diag"
                            # Also write artifact_file if provided
                            [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
                            return 1
                        fi
                        
                        # Verify network_diag is an object
                        if ! echo "$network_diag_json" | jq -e 'type == "object"' >/dev/null 2>&1; then
                            log_error "[FAIL] Phase $phase: network_diag is not an object"
                            # Write failure artifacts to lab directory
                            _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "network_diag_not_object"
                            # Also write artifact_file if provided
                            [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
                            return 1
                        fi
                        
                        log_info "[PASS] Phase $phase: capture verified with status=$cap_status and network_diag present"
                        success=true
                        matched_capture_json="$cap"
                        export MATCHED_CAPTURE_JSON="$cap"
                    else
                        # For other phases, just verify capture exists
                        log_info "[PASS] Capture found for event $event_id"
                        success=true
                        matched_capture_json="$cap"
                        export MATCHED_CAPTURE_JSON="$cap"
                    fi
                else
                    log_info "Event $event_id found but no captures yet (${elapsed}s elapsed)"
                fi
            fi
        fi

        if [[ "$success" == "true" ]]; then
            [[ -n "$artifact_file" ]] && echo "$spikes_response" | jq '.' > "$artifact_file" 2>/dev/null || true
            [[ -n "$artifact_file" ]] && log_info "Saved capture artifact: $artifact_file"
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    log_error "Timeout waiting for capture for event $event_id after ${timeout}s"
    # Write failure artifacts to lab directory
    _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "timeout"
    # Also write artifact_file if provided
    [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
    export MATCHED_CAPTURE_JSON=""
    return 1
}
