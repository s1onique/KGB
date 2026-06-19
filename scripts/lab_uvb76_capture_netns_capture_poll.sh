#!/bin/bash
# lab_uvb76_capture_netns_capture_poll.sh — Capture-specific polling helpers
#
# Functions for polling spike captures and normalizing capture status.
# Sourced by lab_uvb76_capture_netns_lib.sh.
#
# NOTE: fetch_capture_packet moved to lab_uvb76_capture_netns_fetch_packet.sh

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
_write_capture_failure_artifacts() {
    local phase="${1:-unknown}"
    local last_response="${2:-}"
    local last_raw_row="${3:-}"
    local reason="${4:-unknown}"
    
    if [[ -n "$last_response" && -n "${LAB_DIR:-}" ]]; then
        echo "$last_response" | jq '.' > "${LAB_DIR}/${phase}-capture-fail-response.json" 2>/dev/null || true
    fi
    if [[ -n "$last_raw_row" && -n "${LAB_DIR:-}" ]]; then
        echo "$last_raw_row" | jq '.' > "${LAB_DIR}/${phase}-capture-fail-row.json" 2>/dev/null || true
    fi
    if [[ -n "${LAB_DIR:-}" ]]; then
        jq -n --arg phase "$phase" --arg reason "$reason" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{phase: $phase, status: "failure", reason: $reason, timestamp: $timestamp}' \
            > "${LAB_DIR}/${phase}-capture-fail-summary.json" 2>/dev/null || true
    fi
}

# Wait for capture for a specific spike event.
# Polls every 2s until success or timeout.
# On success: exports MATCHED_CAPTURE_JSON, saves LAB_DIR/${phase}-capture-metadata.json
# Returns 0 only if capture entry exists with lifecycle_status=captured.
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

    local interval=2 elapsed=0 success=false
    local last_response="" last_raw_row=""

    while [[ $elapsed -lt $timeout ]]; do
        local spikes_response
        spikes_response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
            "${UVB76_API_URL}/api/v1/latency/spikes?target_id=lab-tovarisch&include_captures=true&limit=20" 2>/dev/null)
        last_response="$spikes_response"

        if [[ -z "$spikes_response" ]] || ! echo "$spikes_response" | jq -e . >/dev/null 2>&1; then
            log_warn "Empty or invalid spikes API response at ${elapsed}s"
        else
            local spike
            spike=$(echo "$spikes_response" | jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' 2>/dev/null)

            if [[ "$spike" == "null" || "$spike" == "" ]]; then
                log_info "Event $event_id not found in spikes list (${elapsed}s elapsed)"
            else
                last_raw_row="$spike"
                local captures_json capture_count
                captures_json=$(echo "$spike" | jq '.captures // []' 2>/dev/null)
                capture_count=$(echo "$captures_json" | jq 'length' 2>/dev/null || echo "0")

                if [[ "$capture_count" -gt 0 ]]; then
                    local cap
                    cap=$(echo "$captures_json" | jq '.[0]' 2>/dev/null)
                    
                    # EXTRACT: lifecycle_status vs diagnostic status
                    # lifecycle_status: use .capture_status if present and non-empty, else .status
                    # Self-test: capture_status="" + status="timeout" -> failed
                    # Self-test: capture_status="skipped_cooldown" + status="ok" -> skipped_cooldown
                    local lifecycle_status
                    local has_capture_status
                    has_capture_status=$(echo "$cap" | jq -r '.capture_status // empty' 2>/dev/null)
                    if [[ -n "$has_capture_status" ]]; then
                        lifecycle_status=$(echo "$cap" | jq -r '.capture_status' 2>/dev/null)
                    else
                        lifecycle_status=$(echo "$cap" | jq -r '.status // "unknown"' 2>/dev/null)
                    fi
                    local diag_status cap_started
                    diag_status=$(echo "$cap" | jq -r '.status // "unknown"' 2>/dev/null)
                    cap_started=$(echo "$cap" | jq -r '.capture_started_at // empty' 2>/dev/null)
                    
                    local norm_lifecycle
                    norm_lifecycle=$(normalize_capture_status "$lifecycle_status")
                    
                    log_info "Capture found: lifecycle_status=$lifecycle_status (normalized: $norm_lifecycle) diag_status=$diag_status"

                    if [[ -n "${LAB_DIR:-}" ]]; then
                        echo "$cap" | jq '.' > "${LAB_DIR}/${phase}-capture-metadata.json" 2>/dev/null || true
                    fi
                    
                    if [[ "$phase" == "phase1" || "$phase" == "phase3" ]]; then
                        if [[ "$norm_lifecycle" != "captured" ]]; then
                            log_error "[FAIL] Phase $phase: lifecycle_status is '$lifecycle_status' (normalized: $norm_lifecycle), expected 'captured'"
                            _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "non_captured_status:${lifecycle_status}"
                            [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
                            return 1
                        fi
                        log_info "[PASS] Phase $phase: capture metadata verified with lifecycle_status=$lifecycle_status"
                        success=true
                        export MATCHED_CAPTURE_JSON="$cap"
                    else
                        log_info "[PASS] Capture found for event $event_id"
                        success=true
                        export MATCHED_CAPTURE_JSON="$cap"
                    fi
                else
                    log_info "Event $event_id found but no captures yet (${elapsed}s elapsed)"
                fi
            fi
        fi

        if [[ "$success" == "true" ]]; then
            [[ -n "$artifact_file" ]] && echo "$spikes_response" | jq '.' > "$artifact_file" 2>/dev/null || true
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    log_error "Timeout waiting for capture for event $event_id after ${timeout}s"
    _write_capture_failure_artifacts "$phase" "$last_response" "$last_raw_row" "timeout"
    [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
    export MATCHED_CAPTURE_JSON=""
    return 1
}
