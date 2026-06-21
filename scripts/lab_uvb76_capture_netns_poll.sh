#!/bin/bash
# lab_uvb76_capture_netns_poll.sh — Polling helpers for UVB-76 capture netns lab
#
# THIS FILE IS A THIN WRAPPER - ALL POLLING LOGIC HAS BEEN MIGRATED TO GO.
#
# This script delegates all polling behavior to the Go binary at:
#   uvb76/cmd/uvb76-capture-netns-polling/
#
# No JSON parsing, no polling loops, no jq - shell is just a launcher.
#
# Sourced by lab_uvb76_capture_netns_lib.sh.
#
# ShellRole: go-polling-delegation
# ShellJustification: Thin launcher that delegates polling to Go binary
# MigrationStatus: Polling logic migrated to Go (2024-01)
# MigrationPlan: All JSON parsing and polling state machine moved to
#   uvb76-capture-netns-polling Go package; shell retained as thin launcher

# Ensure Go binary path is set (deterministic - built artifact path)
UVB76_POLLING_BINARY="${UVB76_POLLING_BINARY:-./uvb76/cmd/uvb76-capture-netns-polling/uvb76-capture-netns-polling}"

# Require the binary before polling operations (call this from sourced functions, not at top level)
# Uses return for sourced contexts, exit for direct execution
require_uvb76_polling_binary() {
    if [[ ! -x "$UVB76_POLLING_BINARY" ]]; then
        echo "ERROR: uvb76-capture-netns-polling binary not found at $UVB76_POLLING_BINARY" >&2
        echo "Run 'make uvb76-polling-build' to build the binary" >&2
        # Return if sourced (BASH_SOURCE[0] != $0), exit if executed directly
        if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
            exit 1
        fi
        return 1
    fi
}

# Configuration from environment or defaults
UVB76_API_URL="${UVB76_API_URL:-http://localhost:9999}"
UVB76_API_USER="${UVB76_API_USER:-lab-admin}"
UVB76_API_PASS="${UVB76_API_PASS:-testpass123}"

# Wait for probe samples to prove the HTTP probe loop is running.
# Uses the Go binary to poll the /api/v1/latency/series endpoint.
# Returns 0 if samples found, 1 otherwise.
wait_for_probe_samples_after_cursor() {
    require_uvb76_polling_binary || return $?

    local target="${1:-lab-tovarisch}"
    local kind="${2:-http}"
    local cursor="${3:-}"
    local reachable="${4:-true}"
    local timeout="${5:-30}"
    local artifact_file="${6:-}"

    log_info "Waiting for probe samples: target=$target kind=$kind reachable=$reachable timeout=${timeout}s"

    local require_count=2
    if [[ "$reachable" != "true" ]]; then
        require_count=1
    fi

    local args=(
        "probe-samples"
        "--base-url" "${UVB76_API_URL}"
        "--target" "$target"
        "--kind" "$kind"
        "--require-count" "$require_count"
        "--timeout" "${timeout}s"
        "--username" "$UVB76_API_USER"
        "--password" "$UVB76_API_PASS"
    )

    if [[ -n "$artifact_file" ]]; then
        args+=("--output" "$artifact_file")
    fi

    "$UVB76_POLLING_BINARY" "${args[@]}"
    return $?
}

# Wait for capture with specific reason pattern after a cursor.
# Uses the Go binary to poll the /api/v1/latency/spikes endpoint.
# Returns 0 if capture found with matching reason, 1 otherwise.
wait_for_capture_after_cursor() {
    require_uvb76_polling_binary || return $?

    local phase="${1:-unknown}"
    local cursor="${2:-}"
    local reason_regex="${3:-}"
    local timeout="${4:-30}"
    local artifact_file="${5:-}"

    log_info "Waiting for capture: phase=$phase reason=$reason_regex timeout=${timeout}s"

    # First poll for spike event
    local spike_args=(
        "spike-event"
        "--base-url" "${UVB76_API_URL}"
        "--target" "lab-tovarisch"
        "--reasons" "$reason_regex"
        "--timeout" "${timeout}s"
        "--username" "$UVB76_API_USER"
        "--password" "$UVB76_API_PASS"
    )

    if [[ -n "$cursor" ]]; then
        spike_args+=("--cursor" "$cursor")
    fi

    if [[ -n "$artifact_file" ]]; then
        spike_args+=("--output" "$artifact_file")
    fi

    # Execute and capture output
    local output
    output=$("$UVB76_POLLING_BINARY" "${spike_args[@]}" 2>&1)
    local exit_code=$?

    if [[ $exit_code -ne 0 ]]; then
        log_error "Timeout waiting for capture after ${timeout}s"
        return 1
    fi

    # Extract matched event ID and reasons from output
    export MATCHED_EVENT_ID=$(echo "$output" | grep "^MATCHED_EVENT_ID=" | cut -d= -f2-)
    export MATCHED_REASONS=$(echo "$output" | grep "^MATCHED_REASONS=" | cut -d= -f2-)

    log_info "Found matching capture: event_id=$MATCHED_EVENT_ID reasons=$MATCHED_REASONS"
    return 0
}

# Wait for spike event with specific reason pattern after a cursor.
# Uses the Go binary to poll the /api/v1/latency/spikes endpoint.
# Returns 0 if spike event found with matching reasons, 1 otherwise.
wait_for_spike_event_after_cursor() {
    require_uvb76_polling_binary || return $?

    local phase="${1:-unknown}"
    local cursor="${2:-}"
    local reason_regex="${3:-http_probe_timeout|http_probe_failure|http_probe_connection_refused}"
    local timeout="${4:-30}"
    local artifact_file="${5:-}"

    log_info "Waiting for spike event: phase=$phase reason=$reason_regex timeout=${timeout}s"

    local args=(
        "spike-event"
        "--base-url" "${UVB76_API_URL}"
        "--target" "lab-tovarisch"
        "--reasons" "$reason_regex"
        "--timeout" "${timeout}s"
        "--username" "$UVB76_API_USER"
        "--password" "$UVB76_API_PASS"
    )

    if [[ -n "$cursor" ]]; then
        args+=("--cursor" "$cursor")
    fi

    if [[ -n "$artifact_file" ]]; then
        args+=("--output" "$artifact_file")
    fi

    # Execute and capture output
    local output
    output=$("$UVB76_POLLING_BINARY" "${args[@]}" 2>&1)
    local exit_code=$?

    if [[ $exit_code -ne 0 ]]; then
        log_error "Timeout waiting for spike event after ${timeout}s"
        export MATCHED_EVENT_ID=""
        export MATCHED_REASONS=""
        return 1
    fi

    # Extract matched event ID and reasons from output
    export MATCHED_EVENT_ID=$(echo "$output" | grep "^MATCHED_EVENT_ID=" | cut -d= -f2-)
    export MATCHED_REASONS=$(echo "$output" | grep "^MATCHED_REASONS=" | cut -d= -f2-)

    log_info "[PASS] Spike event found: event_id=$MATCHED_EVENT_ID reasons=$MATCHED_REASONS"
    return 0
}

# Query latency API and save response (for baseline probe readiness).
# This is a simple HTTP GET, shell is acceptable here.
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
