#!/bin/bash
# lab_uvb76_capture_netns_fetch_packet.sh — Fetch capture packet helper
#
# Fetches diagnostic packet from tovarisch using capture metadata.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Fetch the actual diagnostic packet from capture metadata.
# Uses the packet/detail endpoint (not spikes API which only has metadata).
# Writes:
#   - ${phase}-capture-packet.json = fetched diagnostic packet with network_diag
#   - ${phase}-capture-fetch-response.json = raw API response
#   - ${phase}-capture-fetch-summary.json = fetch summary with metadata (includes final fetch_url)
# Fails if fetched packet has no object .network_diag.
# Required inputs from capture metadata:
#   - base_url (tovarisch base URL) - REQUIRED
#   - requested_path - FALLBACK to /status.json if missing
#   - event_id, source - optional, logged in summary
fetch_capture_packet() {
    local phase="${1:-unknown}"
    local capture_metadata_file="${2:-}"
    local packet_file="${3:-}"
    local response_file="${4:-}"
    local summary_file="${5:-}"

    log_info "Fetching diagnostic packet for phase $phase"

    # Validate required arguments
    if [[ -z "$capture_metadata_file" ]]; then
        log_error "[FAIL] fetch_capture_packet: missing capture_metadata_file argument"
        return 1
    fi
    if [[ ! -f "$capture_metadata_file" ]]; then
        log_error "[FAIL] fetch_capture_packet: file not found: $capture_metadata_file"
        return 1
    fi
    if [[ -z "$packet_file" ]]; then
        log_error "[FAIL] fetch_capture_packet: missing packet_file argument"
        return 1
    fi

    # Extract fields from capture metadata
    local event_id source base_url requested_path capture_id
    event_id=$(jq -r '.event_id // empty' "$capture_metadata_file" 2>/dev/null || echo "")
    source=$(jq -r '.source // empty' "$capture_metadata_file" 2>/dev/null || echo "")
    base_url=$(jq -r '.base_url // empty' "$capture_metadata_file" 2>/dev/null || echo "")
    requested_path=$(jq -r '.requested_path // empty' "$capture_metadata_file" 2>/dev/null || echo "")
    capture_id=$(jq -r '.capture_id // .referenced_capture_id // empty' "$capture_metadata_file" 2>/dev/null || echo "")

    log_info "  event_id=$event_id source=$source base_url=$base_url requested_path=${requested_path:-<fallback>}"

    # Must have base_url - that's the tovarisch endpoint
    if [[ -z "$base_url" ]]; then
        log_error "[FAIL] Phase $phase: missing base_url in capture metadata"
        log_error "  Cannot fetch diagnostic packet without tovarisch endpoint"
        if [[ -n "$summary_file" ]]; then
            jq -n \
                --arg phase "phase${phase}" \
                --arg status "error" \
                --arg reason "missing_base_url" \
                --arg event_id "$event_id" \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{
                    phase: $phase,
                    status: $status,
                    reason: $reason,
                    event_id: $event_id,
                    fetch_url: null,
                    timestamp: $timestamp
                }' > "$summary_file" 2>/dev/null || true
        fi
        return 1
    fi

    # Build list of paths to try (in order of preference)
    local -a paths_to_try=()
    if [[ -n "$requested_path" ]]; then
        paths_to_try+=("$requested_path")
    fi
    # Fallback paths if requested_path is missing or failed
    paths_to_try+=("/status" "/status.json")
    
    # Track all attempts with structured data for summary
    local -a attempt_json_lines=()
    local fetch_success=false
    local final_fetch_url=""
    local final_fetch_status=""
    local final_fetch_response=""
    
    # Try each path until we get a valid network_diag
    for try_path in "${paths_to_try[@]}"; do
        # Skip if already tried this exact path
        local already_tried=false
        for prev_line in "${attempt_json_lines[@]}"; do
            local prev_path
            prev_path=$(echo "$prev_line" | jq -r '.path // empty' 2>/dev/null || echo "")
            [[ "$prev_path" == "$try_path" ]] && already_tried=true && break
        done
        [[ "$already_tried" == "true" ]] && continue
        
        # Construct URL
        local try_url="${base_url}${try_path}"
        if [[ "$try_path" != *"include=network_diag"* ]]; then
            if [[ "$try_path" == *"?"* ]]; then
                try_url="${base_url}${try_path}&include=network_diag"
            else
                try_url="${base_url}${try_path}?include=network_diag"
            fi
        fi
        
        log_info "  Trying fetch URL: $try_url"
        
        local try_response
        local try_status
        try_status=$(ip netns exec "$NS_UVB76" curl -s -w "%{http_code}" -o /tmp/uvb76-packet-fetch-try.json \
            "$try_url" 2>/dev/null || echo "000")
        try_response=$(cat /tmp/uvb76-packet-fetch-try.json 2>/dev/null || echo "{}")
        
        log_info "    HTTP status: $try_status"
        
        # Check if we got a valid response with network_diag
        local has_network_diag="false"
        if [[ "$try_status" == "200" ]] && echo "$try_response" | jq -e '.network_diag != null' >/dev/null 2>&1; then
            has_network_diag="true"
        fi
        
        # Record this attempt with structured data
        attempt_json_lines+=("$(jq -n \
            --arg path "$try_path" \
            --arg url "$try_url" \
            --arg status "$try_status" \
            --argjson has_network_diag "$has_network_diag" \
            '{path: $path, url: $url, http_status: $status, has_network_diag: $has_network_diag}')")
        
        # Always update final values (even on failure)
        final_fetch_url="$try_url"
        final_fetch_status="$try_status"
        final_fetch_response="$try_response"
        
        if [[ "$has_network_diag" == "true" ]]; then
            log_info "  [PASS] Valid network_diag found with path: $try_path"
            fetch_success=true
            # Save this successful response
            cp /tmp/uvb76-packet-fetch-try.json /tmp/uvb76-packet-fetch.json
            break
        fi
        
        log_info "    No network_diag or non-200 response, trying next path..."
    done
    
    # Build attempts JSON array
    local attempts_json
    attempts_json=$(printf '%s\n' "${attempt_json_lines[@]}" | jq -s . 2>/dev/null || echo "[]")
    
    # Check if any attempt succeeded
    if [[ "$fetch_success" != "true" ]]; then
        log_error "[FAIL] Phase $phase: no valid network_diag from any fallback path"
        log_error "  Attempted paths: $attempts_json"
        
        # Save last response
        if [[ -n "$response_file" && -n "$final_fetch_response" ]]; then
            echo "$final_fetch_response" | jq '.' > "$response_file" 2>/dev/null || true
        fi
        
        # Write failure summary with all attempts
        if [[ -n "$summary_file" ]]; then
            jq -n \
                --arg phase "phase${phase}" \
                --arg status "fetch_failed" \
                --arg reason "no_network_diag_from_fallbacks" \
                --arg http_status "$final_fetch_status" \
                --arg last_url "$final_fetch_url" \
                --argjson attempts "$attempts_json" \
                --arg event_id "$event_id" \
                --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
                '{
                    phase: $phase,
                    status: $status,
                    reason: $reason,
                    http_status: $http_status,
                    last_fetch_url: $last_url,
                    attempts: $attempts,
                    event_id: $event_id,
                    timestamp: $timestamp
                }' > "$summary_file" 2>/dev/null || true
        fi
        return 1
    fi
    
    # Extract network_diag from successful response
    local network_diag
    network_diag=$(echo "$final_fetch_response" | jq '.network_diag' 2>/dev/null || echo "null")
    
    # Verify network_diag is an object
    if ! echo "$network_diag" | jq -e 'type == "object"' >/dev/null 2>&1; then
        log_error "[FAIL] Phase $phase: network_diag is not an object"
        return 1
    fi
    
    # Save raw response
    if [[ -n "$response_file" ]]; then
        echo "$final_fetch_response" | jq '.' > "$response_file" 2>/dev/null || true
        log_info "  Saved fetch response: $response_file"
    fi
    
    # Write packet file with network_diag
    jq -n \
        --arg phase "phase${phase}" \
        --argjson network_diag "$network_diag" \
        --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{
            phase: $phase,
            network_diag: $network_diag,
            timestamp: $timestamp
        }' > "$packet_file"
    
    log_info "[PASS] Phase $phase capture packet saved: $packet_file"
    
    # Write success summary with all attempts for traceability
    if [[ -n "$summary_file" ]]; then
        jq -n \
            --arg phase "phase${phase}" \
            --arg status "success" \
            --arg http_status "$final_fetch_status" \
            --arg fetch_url "$final_fetch_url" \
            --argjson attempts "$attempts_json" \
            --arg event_id "$event_id" \
            --arg source "$source" \
            --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
            '{
                phase: $phase,
                status: $status,
                http_status: $http_status,
                fetch_url: $fetch_url,
                attempts: $attempts,
                event_id: $event_id,
                source: $source,
                timestamp: $timestamp
            }' > "$summary_file" 2>/dev/null || true
        log_info "  Saved fetch summary: $summary_file"
    fi
    
    return 0
}
