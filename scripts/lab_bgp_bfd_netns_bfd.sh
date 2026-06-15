#!/bin/bash
# lab_bgp_bfd_netns_bfd.sh — BFD convergence functions for netns lab
#
# BFD-specific functions for asserting BFD session reaches Up.
# Sourced by lab_bgp_bfd_netns_lib.sh.

# =============================================================================
# BFD Convergence and Diagnostic Functions (ACT 2)
# =============================================================================

# Start tcpdump for BFD packets in the specified namespace
# Usage: start_tcpdump_bfd <namespace> <output_file> <duration_seconds>
start_tcpdump_bfd() {
    local ns="$1"
    local output="$2"
    local duration="${3:-30}"

    if ! command -v tcpdump &> /dev/null; then
        log_warn "tcpdump not available - skipping packet capture"
        echo "# tcpdump not available" > "$output"
        return 1
    fi

    log_info "Starting BFD packet capture in namespace $ns (${duration}s)..."

    # Capture multihop BFD (UDP 4784) and single-hop BFD (UDP 3784)
    # -c 50 limits to 50 packets to avoid huge captures
    # -n avoids DNS resolution
    # -v gives verbose output showing ports
    ip netns exec "$ns" timeout "$duration" tcpdump -c 50 -n -v \
        "udp port 4784 or udp port 3784" \
        > "$output" 2>&1 &
    local pid=$!

    log_info "tcpdump started with PID $pid, capturing to $output"
    return 0
}

# Collect BIRD BFD session state to artifact
collect_bfd_sessions() {
    log_info "Collecting BIRD BFD sessions..."

    if birdc_lab show bfd sessions 2>/dev/null > "$BFD_SESSIONS_OUTPUT"; then
        log_info "BFD sessions collected to $BFD_SESSIONS_OUTPUT"
        cat "$BFD_SESSIONS_OUTPUT"
        return 0
    else
        log_warn "Failed to collect BFD sessions"
        echo "FAILED_TO_COLLECT" > "$BFD_SESSIONS_OUTPUT"
        return 1
    fi
}

# Collect runtime HTTP status at specific time for BFD evidence
collect_status_http_bfd() {
    log_info "Collecting runtime HTTP status for BFD evidence..."

    local output="$LAB_DIR/status-http-bfd.json"

    if command -v curl &> /dev/null; then
        if ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" > "$output" 2>&1; then
            log_info "HTTP status collected to $output"
            return 0
        else
            log_error "Failed to collect HTTP status for BFD evidence"
            return 1
        fi
    else
        log_warn "curl not available - cannot collect HTTP BFD status"
        return 1
    fi
}

# Extract BFD session state from HTTP status JSON
# Returns: up, down, init, or unknown
extract_bfd_state_from_http() {
    local status_file="$1"

    if [[ ! -f "$status_file" ]] || [[ ! -s "$status_file" ]]; then
        echo "unknown"
        return 1
    fi

    # Try to find BFD state in HTTP status
    # Look for "bfd": "up" or "bfd": {"state": "up"} patterns
    local bfd_json
    bfd_json=$(jq -r '.bfd // .checks[] | select(.name == "bfd") // empty' "$status_file" 2>/dev/null || echo "")

    if [[ -n "$bfd_json" ]]; then
        # Check for state field
        local state
        state=$(echo "$bfd_json" | jq -r '.state // .detail // empty' 2>/dev/null || echo "")

        if [[ "$state" == "up" ]] || [[ "$bfd_json" == *"up"* ]]; then
            echo "up"
            return 0
        elif [[ "$state" == "down" ]] || [[ "$bfd_json" == *"down"* ]]; then
            echo "down"
            return 0
        elif [[ "$state" == "init" ]] || [[ "$bfd_json" == *"init"* ]]; then
            echo "init"
            return 0
        fi
    fi

    # Fallback: check if bfd_detail contains peer count indicating active sessions
    local bfd_detail
    bfd_detail=$(jq -r '.checks[] | select(.name == "bfd") | .detail // "unknown"' "$status_file" 2>/dev/null || echo "unknown")

    if [[ "$bfd_detail" != "not configured" ]] && [[ "$bfd_detail" != "unknown" ]]; then
        # If config is loaded but state isn't explicitly up, check for session count
        if echo "$bfd_detail" | grep -qE "[0-9]+.*peer"; then
            echo "configured"
            return 0
        fi
    fi

    echo "unknown"
    return 1
}

# Main BFD assertion function - proves BFD session reaches Up
# Returns: 0 on success (BFD Up), 1 on failure
assert_bfd_up() {
    log_info "=== ACT 2: Assert BFD Session Up ==="
    log_info "BFD mode: multihop (UDP 4784 per RFC 5883)"
    log_info "Waiting up to ${WAIT_BFD_CONVERGE}s for BFD Up..."

    local bfd_up=false
    local elapsed=0
    local interval=2
    local last_bird_sessions=""
    local last_http_status=""

    # Start tcpdump for packet diagnostics (non-fatal if unavailable)
    start_tcpdump_bfd "$NS_BIRD" "$TCPDUMP_BFD_BIRD" "$((WAIT_BFD_CONVERGE + 5))" || true

    # Poll for BFD Up
    while [[ $elapsed -lt $WAIT_BFD_CONVERGE ]]; do
        echo -n "."

        # Check BIRD BFD sessions
        last_bird_sessions=$(birdc_lab show bfd sessions 2>/dev/null || echo "")

        if echo "$last_bird_sessions" | grep -qE '(^|[[:space:]])Up([[:space:]]|$)'; then
            log_info ""
            log_info "BIRD reports BFD session Up!"
            bfd_up=true
            break
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
    done

    echo ""

    # Collect final BFD evidence
    collect_bfd_sessions

    # Collect HTTP status for tovarisch BFD state
    local http_status_file="$LAB_DIR/status-http-bfd.json"
    collect_status_http_bfd

    # Stop tcpdump and collect
    pkill -f "tcpdump.*udp port 4784" 2>/dev/null || true
    pkill -f "tcpdump.*udp port 3784" 2>/dev/null || true
    sleep 1

    # Collect tcpdump outputs if they exist
    for f in "$TCPDUMP_BFD_TOVARISCH" "$TCPDUMP_BFD_BIRD" "$TCPDUMP_BFD_TOVERVIEW"; do
        if [[ -s "$f" ]]; then
            log_info "tcpdump capture available: $f"
        fi
    done

    # Evaluate result
    if $bfd_up; then
        log_info "[PASS] BFD session reached Up"

        # Log evidence
        log_info "=== BFD Up Evidence ==="
        log_info "BIRD BFD sessions:"
        cat "$BFD_SESSIONS_OUTPUT" | head -20

        if [[ -s "$http_status_file" ]]; then
            log_info "tovarisch runtime BFD state:"
            jq '.bfd // .checks[] | select(.name == "bfd")' "$http_status_file" 2>/dev/null || echo "Parse failed"
        fi

        return 0
    else
        log_error "[FAIL] BFD session did not reach Up within ${WAIT_BFD_CONVERGE}s"
        log_error "=== BFD Failure Diagnostics ==="

        log_error "BIRD BFD sessions (final check):"
        cat "$BFD_SESSIONS_OUTPUT" 2>/dev/null || echo "No BFD sessions output"

        log_error "BIRD protocols:"
        birdc_lab show protocols all 2>/dev/null || echo "Cannot query BIRD protocols"

        if [[ -s "$http_status_file" ]]; then
            log_error "tovarisch runtime BFD status:"
            jq '.' "$http_status_file" 2>/dev/null || cat "$http_status_file"
        fi

        log_error "tovarisch log tail:"
        tail -n 30 "$TOVARISCH_LOG" 2>/dev/null || echo "Log not available"

        log_error "BIRD log tail:"
        tail -n 30 "$BIRD_LOG" 2>/dev/null || echo "Log not available"

        if [[ -s "$TCPDUMP_BFD_BIRD" ]]; then
            log_error "tcpdump capture (BIRD namespace):"
            cat "$TCPDUMP_BFD_BIRD" 2>/dev/null || echo "Empty"
        fi

        return 1
    fi
}
