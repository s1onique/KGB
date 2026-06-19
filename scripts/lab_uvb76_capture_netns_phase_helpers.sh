#!/bin/bash
# lab_uvb76_capture_netns_phase_helpers.sh — Phase-specific polling and artifact helpers
#
# Phase 0 helpers and Phase 2 skipped cooldown polling.

# Declare effective probe URL file globally (set in lib)
declare -g EFFECTIVE_PROBE_URL_FILE=""

# Save the effective probe URL as an artifact for verification.
# This fetches from UVB-76 targets API and extracts the effective_probe_url field.
save_effective_probe_url() {
    log_info "Saving effective probe URL artifact..."
    
    # Get targets from UVB-76 API
    local response
    response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
        "${UVB76_API_URL}/api/v1/targets" 2>/dev/null)

    if [[ -z "$response" ]] || ! echo "$response" | jq -e . >/dev/null 2>&1; then
        log_error "[FAIL] Failed to get targets from UVB-76 API"
        echo "{}" > "$EFFECTIVE_PROBE_URL_FILE"
        return 1
    fi

    # Extract effective_probe_url for lab-tovarisch target
    local effective_url
    effective_url=$(echo "$response" | jq -r '.[] | select(.id == "lab-tovarisch") | .effective_probe_url // empty' 2>/dev/null)

    if [[ -z "$effective_url" || "$effective_url" == "empty" ]]; then
        log_error "[FAIL] effective_probe_url not found in targets response"
        echo "$response" | jq '.' > "$EFFECTIVE_PROBE_URL_FILE"
        return 1
    fi

    # Save as artifact
    cat > "$EFFECTIVE_PROBE_URL_FILE" <<EOF
{
  "target_id": "lab-tovarisch",
  "effective_probe_url": "${effective_url}",
  "expected_probe_url": "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/lab/probe"
}
EOF

    log_info "[PASS] Effective probe URL saved: $effective_url"
    
    # Verify it matches expected
    if [[ "$effective_url" == "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/lab/probe" ]]; then
        log_info "[PASS] Effective probe URL matches expected /lab/probe"
        return 0
    else
        log_error "[FAIL] Effective probe URL mismatch: got $effective_url, expected http://${IP_TOVARISCH}:${TOVARISCH_PORT}/lab/probe"
        return 1
    fi
}

# Save UVB-76 status API response as Phase 0 artifact
save_phase0_status() {
    log_info "Saving Phase 0 status artifact..."

    local response
    response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
        "${UVB76_API_URL}/api/v1/status" 2>/dev/null)

    if [[ -z "$response" ]] || ! echo "$response" | jq -e . >/dev/null 2>&1; then
        log_error "[FAIL] Phase 0: failed to get status from UVB-76 API"
        echo "{}" > "$PHASE0_STATUS_FILE"
        return 1
    fi

    echo "$response" | jq '.' > "$PHASE0_STATUS_FILE"
    log_info "[PASS] Phase 0 status saved: $PHASE0_STATUS_FILE"
    return 0
}

# Copy probe readiness artifact to Phase 0 naming
copy_phase0_probe_ready() {
    if [[ -f "$BASELINE_PROBE_READY_FILE" ]]; then
        cp "$BASELINE_PROBE_READY_FILE" "$PHASE0_PROBE_READY_FILE"
        log_info "[PASS] Phase 0 probe ready artifact copied: $PHASE0_PROBE_READY_FILE"
    else
        log_warn "Phase 0 probe ready artifact not found: $BASELINE_PROBE_READY_FILE"
    fi
}

# Extract spike row from spikes API response for a specific event
extract_spike_row_for_event() {
    local spikes_file="$1"
    local event_id="$2"

    local spike_row
    spike_row=$(jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0]' "$spikes_file" 2>/dev/null || echo "null")

    echo "$spike_row"
}

# Wait for skipped_cooldown spike row after an event.
# Polls until capture_status == "skipped_cooldown" or timeout.
wait_for_skipped_cooldown_spike_row_after_event() {
    local phase_num="$1"
    local event_id="$2"
    local timeout="${3:-15}"
    local out_file="$4"

    log_info "Waiting for skipped_cooldown spike row: Phase $phase_num event_id=$event_id timeout=${timeout}s"

    local interval=2
    local elapsed=0
    local success=false
    local last_response=""

    while [[ $elapsed -lt $timeout ]]; do
        local spikes_response
        spikes_response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
            "${UVB76_API_URL}/api/v1/latency/spikes?target_id=lab-tovarisch&include_captures=true&limit=20" 2>/dev/null)
        last_response="$spikes_response"

        if [[ -z "$spikes_response" ]] || ! echo "$spikes_response" | jq -e . >/dev/null 2>&1; then
            log_warn "Empty or invalid spikes API response at ${elapsed}s"
        else
            local spike_row
            spike_row=$(jq --arg eid "$event_id" '[.spikes[] | select(.event_id == $eid)] | .[0] // null' <<< "$spikes_response" 2>/dev/null)

            if [[ "$spike_row" == "null" || "$spike_row" == "" ]]; then
                log_info "Event $event_id not yet in spikes list (${elapsed}s elapsed)"
            else
                local capture_status
                capture_status=$(jq -r '[.captures[] | select(.capture_status != null)] | .[0] | .capture_status // "null"' <<< "$spike_row" 2>/dev/null)

                if [[ "$capture_status" == "null" || "$capture_status" == "" ]]; then
                    log_info "Event $event_id found but no capture_status yet (${elapsed}s elapsed)"
                elif [[ "$capture_status" == "skipped_cooldown" ]]; then
                    log_info "[PASS] Found skipped_cooldown spike for event $event_id at ${elapsed}s"

                    local raw_row_file
                    raw_row_file=$(mktemp "/tmp/phase${phase_num}-raw-row-XXXXXX.json")
                    echo "$spike_row" | jq '.' > "$raw_row_file"

                    if normalize_spike_row_capture_contract "$raw_row_file" "$out_file"; then
                        rm -f "$raw_row_file"
                        log_info "Phase $phase_num skipped_cooldown row saved: $out_file"
                        success=true
                    else
                        rm -f "$raw_row_file"
                        log_error "[FAIL] Phase $phase_num: failed to normalize skipped_cooldown row"
                    fi
                else
                    log_error "[FAIL] Phase $phase_num: spike has unexpected status: $capture_status"
                    echo "$spikes_response" | jq '.' > "/tmp/phase${phase_num}-last-response.json" 2>/dev/null || true
                    return 1
                fi
            fi
        fi

        [[ "$success" == "true" ]] && return 0

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    log_error "[FAIL] Timeout waiting for skipped_cooldown spike row after ${timeout}s"
    if [[ -n "$last_response" ]]; then
        echo "$last_response" | jq '.' > "/tmp/phase${phase_num}-timeout-response.json" 2>/dev/null || true
    fi

    return 1
}
