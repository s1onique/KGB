#!/bin/bash
# lab_uvb76_capture_netns_poll.sh — Polling helpers for UVB-76 capture netns lab
#
# Helper functions for polling probe samples and capture events.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Wait for probe samples with reachability after a cursor
# Polls every 2s until success or timeout
# Saves final API response as artifact
# Returns 0 if reachable samples found, 1 otherwise
wait_for_probe_samples_after_cursor() {
    local target="${1:-lab-tovarisch}"
    local kind="${2:-http}"
    local cursor="${3:-}"
    local reachable="${4:-true}"
    local timeout="${5:-30}"
    local artifact_file="${6:-}"

    log_info "Waiting for probe samples: target=$target kind=$kind reachable=$reachable timeout=${timeout}s"

    local interval=2
    local elapsed=0
    local success=false

    while [[ $elapsed -lt $timeout ]]; do
        local query_url="${UVB76_API_URL}/api/v1/latency?target_id=${target}&kind=${kind}&range_seconds=60"
        [[ -n "$cursor" ]] && query_url="${query_url}&after=${cursor}"

        local response
        response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
            "$query_url" 2>/dev/null)

        if [[ -z "$response" ]] || ! echo "$response" | jq -e . >/dev/null 2>&1; then
            log_warn "Empty or invalid latency API response at ${elapsed}s"
        else
            local sample_count
            sample_count=$(echo "$response" | jq '.sample_count // 0' 2>/dev/null || echo "0")

            if [[ "$sample_count" -gt 0 ]]; then
                local reachable_count
                reachable_count=$(echo "$response" | jq '[.samples[] | select(.reachable == true)] | length' 2>/dev/null || echo "0")

                if [[ "$reachable" == "true" ]]; then
                    if [[ "$reachable_count" -ge 2 ]]; then
                        log_info "Found $reachable_count reachable samples (${elapsed}s elapsed)"
                        success=true
                    else
                        log_info "Found $sample_count samples but only $reachable_count reachable (need 2)"
                    fi
                else
                    log_info "Found $sample_count samples (${elapsed}s elapsed)"
                    success=true
                fi
            else
                log_info "No samples yet (${elapsed}s elapsed)"
            fi
        fi

        if [[ "$success" == "true" ]]; then
            [[ -n "$artifact_file" ]] && echo "$response" | jq '.' > "$artifact_file" 2>/dev/null || true
            [[ -n "$artifact_file" ]] && log_info "Saved probe-ready artifact: $artifact_file"
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    log_error "Timeout waiting for probe samples after ${timeout}s"
    [[ -n "$artifact_file" ]] && echo "$response" | jq '.' > "$artifact_file" 2>/dev/null || true
    return 1
}

# Wait for capture with specific reason pattern after a cursor
# Polls every 2s until success or timeout
# Returns 0 if capture found with matching reason, 1 otherwise
wait_for_capture_after_cursor() {
    local phase="${1:-unknown}"
    local cursor="${2:-}"
    local reason_regex="${3:-}"
    local timeout="${4:-30}"
    local artifact_file="${5:-}"

    log_info "Waiting for capture: phase=$phase reason=$reason_regex timeout=${timeout}s"

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
            local spike_count
            spike_count=$(echo "$spikes_response" | jq '.count // 0' 2>/dev/null || echo "0")

            if [[ "$spike_count" -gt 0 ]]; then
                local spike_index=0
                local found_match=false

                while [[ $spike_index -lt 20 ]]; do
                    local spike
                    spike=$(echo "$spikes_response" | jq --argjson idx "$spike_index" '.spikes[$idx] // null' 2>/dev/null)
                    [[ "$spike" == "null" || "$spike" == "" ]] && break

                    local captures_json
                    captures_json=$(echo "$spikes_response" | jq --argjson idx "$spike_index" '.spikes[$idx].captures // []' 2>/dev/null)
                    local capture_count
                    capture_count=$(echo "$captures_json" | jq 'length' 2>/dev/null || echo "0")

                    if [[ "$capture_count" -gt 0 ]]; then
                        local cap_idx=0
                        while [[ $cap_idx -lt "$capture_count" ]]; do
                            local cap
                            cap=$(echo "$captures_json" | jq --argjson idx "$cap_idx" '.[$idx]' 2>/dev/null)
                            local cap_timestamp
                            cap_timestamp=$(echo "$cap" | jq -r '.created_at // .timestamp // empty' 2>/dev/null)

                            local after_cursor=true
                            [[ -n "$cursor" ]] && after_cursor=$(is_capture_after_cursor "$cap_timestamp" "$cursor" && echo "true" || echo "false")

                            if [[ "$after_cursor" == "true" ]]; then
                                local cap_status
                                cap_status=$(echo "$cap" | jq -r '.status // "unknown"' 2>/dev/null)
                                local spike_reasons
                                spike_reasons=$(echo "$spike" | jq -r '.reasons // [] | join("|")' 2>/dev/null)
                                log_info "Checking capture at ${cap_timestamp}: status=$cap_status reasons=$spike_reasons"

                                if [[ -n "$reason_regex" ]]; then
                                    case "$cap_status" in
                                        timeout|error)
                                            if echo "$spike_reasons" | grep -qE "$reason_regex"; then
                                                found_match=true
                                                break 3
                                            fi
                                            ;;
                                        ok)
                                            if echo "$spike_reasons" | grep -qE "$reason_regex"; then
                                                found_match=true
                                                break 3
                                            fi
                                            ;;
                                    esac
                                else
                                    found_match=true
                                    break 3
                                fi
                            fi
                            cap_idx=$((cap_idx + 1))
                        done
                    fi
                    spike_index=$((spike_index + 1))
                done

                [[ "$found_match" == "true" ]] && log_info "Found matching capture after ${elapsed}s" && success=true
                [[ "$found_match" != "true" ]] && log_info "Found $spike_count spikes but no matching capture after cursor"
            else
                log_info "No spikes yet (${elapsed}s elapsed)"
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

    log_error "Timeout waiting for capture after ${timeout}s"
    [[ -n "$artifact_file" ]] && echo "$last_response" | jq '.' > "$artifact_file" 2>/dev/null || true
    return 1
}

# Query latency API and save response (for baseline probe readiness)
query_latency_api() {
    local output_file="$1"
    local target_id="${2:-lab-tovarisch}"
    local kind="${3:-http}"

    log_info "Querying latency API for target: $target_id (kind: $kind)"

    local response
    response=$(ip netns exec "$NS_UVB76" curl -s -c /tmp/uvb76-cookies.txt -b /tmp/uvb76-cookies.txt \
        "${UVB76_API_URL}/api/v1/latency?target_id=${target_id}&kind=${kind}&range_seconds=120" 2>/dev/null)

    if [[ -z "$response" ]]; then
        log_error "Empty response from latency API"
        echo "{}" > "$output_file"
        return 1
    fi

    echo "$response" | jq '.' > "$output_file" 2>/dev/null || {
        log_error "Failed to parse latency API response as JSON"
        echo "{}" > "$output_file"
        return 1
    }

    log_info "Latency API response saved to $output_file"
    return 0
}
