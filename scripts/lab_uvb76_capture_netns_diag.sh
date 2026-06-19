#!/bin/bash
# lab_uvb76_capture_netns_diag.sh — Diagnostic capture helper for UVB-76 capture netns lab
#
# Helper functions for querying the UVB-76 API and extracting capture evidence.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Lab configuration (set by constants)
UVB76_API_PORT="${UVB76_PORT:-8316}"
UVB76_API_URL="http://localhost:${UVB76_API_PORT}"
LAB_USER="lab-admin"
LAB_PASS="lab-password"  # This password is only for the lab config

# Track timestamps for phase isolation (captures must be created after these cursors)
declare -g PHASE_BASELINE_CURSOR=""
declare -g PHASE_DEFECT_CURSOR=""
declare -g PHASE_RECOVERY_CURSOR=""

# Wait for diagnostic capture to complete
wait_for_capture() {
    local max_wait="${1:-${WAIT_CAPTURE:-15}}"
    local elapsed=0
    local interval=1

    log_info "Waiting for diagnostic capture (max ${max_wait}s)..."
    while [[ $elapsed -lt $max_wait ]]; do
        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "."
    done
    echo ""
    log_info "Capture wait complete"
}

# Authenticate to UVB-76 and return session cookie
# All API calls MUST run inside $NS_UVB76 namespace
uvb76_authenticate() {
    log_info "Authenticating to UVB-76 API inside $NS_UVB76 namespace..."

    local response
    response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
        -X POST "${UVB76_API_URL}/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"${LAB_USER}\",\"password\":\"${LAB_PASS}\"}" 2>/dev/null)

    if echo "$response" | jq -e '.success == true' >/dev/null 2>&1; then
        log_info "Authentication successful"
        return 0
    else
        log_error "Authentication failed: $response"
        return 1
    fi
}

# Query the spikes API and save response
# Must run inside $NS_UVB76 namespace
query_spikes_api() {
    local output_file="$1"
    local target_id="${2:-lab-tovarisch}"
    local include_captures="${3:-true}"

    log_info "Querying spikes API for target: $target_id (inside $NS_UVB76 namespace)"

    local response
    response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
        "${UVB76_API_URL}/api/v1/latency/spikes?target_id=${target_id}&include_captures=${include_captures}&limit=20" 2>/dev/null)

    if [[ -z "$response" ]]; then
        log_error "Empty response from spikes API"
        echo "{}" > "$output_file"
        return 1
    fi

    # Save raw response
    echo "$response" | jq '.' > "$output_file" 2>/dev/null || {
        log_error "Failed to parse API response as JSON"
        echo "{}" > "$output_file"
        return 1
    }

    log_info "Spikes API response saved to $output_file"
    return 0
}

# Set the timestamp cursor for a phase
# After a phase completes (e.g., defect injection), call this to mark the cutoff
set_phase_cursor() {
    local phase_name="$1"
    local cursor
    cursor=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    
    case "$phase_name" in
        baseline)
            PHASE_BASELINE_CURSOR="$cursor"
            log_info "Set baseline cursor: $PHASE_BASELINE_CURSOR"
            ;;
        defect)
            PHASE_DEFECT_CURSOR="$cursor"
            log_info "Set defect cursor: $PHASE_DEFECT_CURSOR"
            ;;
        recovery)
            PHASE_RECOVERY_CURSOR="$cursor"
            log_info "Set recovery cursor: $PHASE_RECOVERY_CURSOR"
            ;;
        *)
            log_warn "Unknown phase: $phase_name"
            ;;
    esac
}

# Get ISO timestamp in UTC
get_iso_timestamp() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

# Check if a capture timestamp is after the given cursor
is_capture_after_cursor() {
    local capture_timestamp="$1"
    local cursor="$2"
    
    # Empty cursor means no filtering
    [[ -z "$cursor" ]] && return 0
    
    # Compare timestamps (ISO format allows string comparison)
    [[ "$capture_timestamp" > "$cursor" ]]
}

# Extract latest capture from spikes API response, filtering by phase cursor
# This ensures we only select captures created during the current phase
extract_latest_capture() {
    local spikes_file="$1"
    local output_file="$2"
    local phase="${3:-unknown}"
    local cursor_var="PHASE_${phase}_CURSOR"
    local cursor="${!cursor_var:-}"

    log_info "Extracting latest capture for phase: $phase (cursor: ${cursor:-none})"

    # Check if we have any spikes with captures
    local spike_count
    spike_count=$(jq '.count // 0' "$spikes_file" 2>/dev/null || echo "0")

    if [[ "$spike_count" -eq 0 ]]; then
        log_warn "No spikes found in API response"
        # Write empty capture artifact
        jq -n \
            --arg phase "$phase" \
            --arg status "no_spikes" \
            --argjson count 0 \
            --argjson has_network_diag false \
            '{
                phase: $phase,
                timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
                status: $status,
                spike_count: $count,
                captures: [],
                error: null,
                requested_path: null,
                duration_ms: null,
                has_network_diag: $has_network_diag
            }' > "$output_file"
        return 0
    fi

    # Iterate through spikes to find one with captures after the cursor
    local spike_index=0
    local found_capture=false
    local latest_spike=""
    local latest_capture=""
    local max_spikes=20

    while [[ $spike_index -lt $max_spikes ]]; do
        local spike
        spike=$(jq -r --argjson idx "$spike_index" '.spikes[$idx] // null' "$spikes_file" 2>/dev/null)
        
        if [[ "$spike" == "null" || "$spike" == "" ]]; then
            break
        fi

        # Get captures from this spike
        local captures_json
        captures_json=$(jq -r --argjson idx "$spike_index" '.spikes[$idx].captures // []' "$spikes_file" 2>/dev/null)
        local capture_count
        capture_count=$(echo "$captures_json" | jq 'length' 2>/dev/null || echo "0")

        if [[ "$capture_count" -gt 0 ]]; then
            # Check captures in reverse order (newest first)
            local cap_idx=$((capture_count - 1))
            while [[ $cap_idx -ge 0 ]]; do
                local cap
                cap=$(echo "$captures_json" | jq -r --argjson idx "$cap_idx" '.[$idx]')
                
                local cap_timestamp
                cap_timestamp=$(echo "$cap" | jq -r '.created_at // .timestamp // empty')
                
                # Filter by cursor if set
                if is_capture_after_cursor "$cap_timestamp" "$cursor"; then
                    latest_spike="$spike"
                    latest_capture="$cap"
                    found_capture=true
                    break 2
                fi
                
                cap_idx=$((cap_idx - 1))
            done
        fi

        spike_index=$((spike_index + 1))
    done

    if [[ "$found_capture" != "true" ]]; then
        log_warn "No capture found after cursor for phase: $phase"
        jq -n \
            --arg phase "$phase" \
            --arg status "no_capture_for_phase" \
            --argjson spike_count "$spike_count" \
            --arg cursor "${cursor:-null}" \
            '{
                phase: $phase,
                timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
                status: $status,
                spike_count: $spike_count,
                captures: [],
                error: null,
                requested_path: null,
                duration_ms: null,
                has_network_diag: false
            }' > "$output_file"
        return 0
    fi

    # Extract fields from the capture - ALWAYS preserve all fields
    local capture_status
    capture_status=$(echo "$latest_capture" | jq -r '.status // "unknown"')
    
    local has_network_diag="false"
    if echo "$latest_capture" | jq -e '.network_diag != null' >/dev/null 2>&1; then
        has_network_diag="true"
    fi

    # These fields are ALWAYS extracted and preserved
    local duration_ms
    duration_ms=$(echo "$latest_capture" | jq -r '.duration_ms')
    
    local error_msg
    error_msg=$(echo "$latest_capture" | jq -r '.error')
    
    local requested_path
    requested_path=$(echo "$latest_capture" | jq -r '.requested_path')
    
    local source
    source=$(echo "$latest_capture" | jq -r '.source // "unknown"')
    
    local capture_timestamp
    capture_timestamp=$(echo "$latest_capture" | jq -r '.created_at // .timestamp // empty')
    
    local spike_severity
    spike_severity=$(echo "$latest_spike" | jq -r '.severity // "unknown"')
    
    local event_id
    event_id=$(echo "$latest_spike" | jq -r '.event_id // "unknown"')
    
    local latency_ms
    latency_ms=$(echo "$latest_spike" | jq -r '.latency_ms // 0')

    # Build output JSON with ALL fields preserved in all branches
    # duration_ms and requested_path and error are ALWAYS included
    local duration_ms_json="null"
    local error_json="null"
    local requested_path_json="null"
    
    if [[ "$duration_ms" != "null" && "$duration_ms" != "" ]]; then
        duration_ms_json="$duration_ms"
    fi
    
    if [[ "$error_msg" != "null" && "$error_msg" != "" ]]; then
        error_json="\"$error_msg\""
    fi
    
    if [[ "$requested_path" != "null" && "$requested_path" != "" ]]; then
        requested_path_json="\"$requested_path\""
    fi

    jq -n \
        --arg phase "$phase" \
        --arg status "$capture_status" \
        --argjson spike_count "$spike_count" \
        --argjson capture_count 1 \
        --arg source "$source" \
        --arg event_id "$event_id" \
        --arg spike_status "$spike_severity" \
        --argjson latency_ms "$latency_ms" \
        --argjson has_network_diag "$has_network_diag" \
        --arg capture_timestamp "${capture_timestamp:-null}" \
        '{
            phase: $phase,
            timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ")),
            status: $status,
            spike_count: $spike_count,
            capture_count: $capture_count,
            source: $source,
            event_id: $event_id,
            spike_severity: $spike_status,
            latency_ms: $latency_ms,
            duration_ms: '"$duration_ms_json"',
            error: '"$error_json"',
            requested_path: '"$requested_path_json"',
            has_network_diag: $has_network_diag,
            capture_timestamp: $capture_timestamp
        }' > "$output_file"

    log_info "Latest capture extracted: status=$capture_status, has_network_diag=$has_network_diag"
    return 0
}

# Trigger capture through the production path
# This waits for the HTTP probe to naturally trigger spike detection
trigger_capture_via_production_path() {
    log_info "Triggering capture via production path (waiting for probe cycle)..."

    # The HTTP probe runs every 5 seconds (configured in UVB-76 config)
    # We wait for at least 2 probe cycles to ensure samples are recorded
    wait_for_capture 15

    log_info "Production path capture triggered"
}
