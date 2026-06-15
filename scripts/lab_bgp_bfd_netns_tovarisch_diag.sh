#!/bin/bash
# lab_bgp_bfd_netns_tovarisch_diag.sh — tovarisch-side BFD diagnostics
#
# ACT 2.2: Diagnose tovarisch BFD receive/respond path
#
# Key questions to answer:
# 1. Does tovarisch have a UDP/4784 listener/socket?
# 2. Do packets arrive at veth-tovarisch?
# 3. Does tovarisch log BFD packet receive?
# 4. Does tovarisch send any BFD control packets back?
#
# Sourced by lab_bgp_bfd_netns_lib.sh.

# =============================================================================
# ACT 2.2: tovarisch BFD Receive/Respond Path Diagnostics
# =============================================================================

# Collect socket/listener state from tovarisch namespace
# Answers: Is tovarisch listening on UDP/4784?
collect_tovarisch_socket_state() {
    log_info "=== ACT 2.2: Collecting tovarisch socket state ==="

    local output="$LAB_DIR/tovarisch-socket-state.txt"
    local ok=false

    {
        echo "=== ss -lunp in namespace $NS_TOVARISCH ==="
        echo "Purpose: Prove whether tovarisch has a UDP/4784 listener"
        echo ""
        echo "Commands:"
        echo "  ss -lunp   : Listening UDP sockets with process info"
        echo "  ss -unp    : All UDP sockets with process info"
        echo ""

        echo "--- ss -lunp (listening UDP, numeric, process) ---"
        ip netns exec "$NS_TOVARISCH" ss -lunp 2>&1 || echo "ss command failed"
        echo ""

        echo "--- ss -unp (all UDP, numeric, process) ---"
        ip netns exec "$NS_TOVARISCH" ss -unp 2>&1 || echo "ss command failed"
        echo ""

        echo "--- Check for UDP 4784 specifically ---"
        ip netns exec "$NS_TOVARISCH" ss -lunp 2>&1 | grep -E "4784|4785" || echo "No UDP 4784/4785 sockets found"
        echo ""

        echo "--- Check for 127.0.0.1:8317 (tovarisch HTTP) ---"
        ip netns exec "$NS_TOVARISCH" ss -lunp 2>&1 | grep -E "8317|127.0.0.1" || echo "No localhost:8317 sockets found"
        echo ""

        echo "--- Running tovarisch process ---"
        ip netns exec "$NS_TOVARISCH" ps aux 2>&1 | grep -E "tovarisch|PID" || echo "No tovarisch process"
        echo ""

        echo "--- UDP sockets owned by tovarisch PID ---"
        local tovarisch_pid
        tovarisch_pid=$(ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch 2>/dev/null || echo "")
        if [[ -n "$tovarisch_pid" ]]; then
            echo "tovarisch PID: $tovarisch_pid"
            ip netns exec "$NS_TOVARISCH" ss -lunp "sport = :4784" 2>&1 || echo "No UDP 4784"
            ip netns exec "$NS_TOVARISCH" ss -unp "pid $tovarisch_pid" 2>&1 || echo "No sockets for PID"
        else
            echo "tovarisch PID not found"
        fi

    } > "$output" 2>&1

    log_info "Socket state collected to $output"
    cat "$output"
}

# Start tcpdump in tovarisch namespace to capture incoming BFD packets
# Answers: Do packets arrive at veth-tovarisch?
start_tcpdump_tovarisch_bfd() {
    local duration="${1:-30}"

    if ! command -v tcpdump &> /dev/null; then
        log_warn "tcpdump not available - skipping tovarisch packet capture"
        echo "# tcpdump not available" > "$TCPDUMP_BFD_TOVARISCH"
        return 1
    fi

    log_info "Starting BFD tcpdump in tovarisch namespace (${duration}s)..."

    # Capture on veth-tovarisch specifically to see packets arriving at the interface
    ip netns exec "$NS_TOVARISCH" timeout "$duration" tcpdump -c 50 -n -v -i "$VETH_TOVARISCH" \
        "udp port 4784 or udp port 3784" \
        > "$TCPDUMP_BFD_TOVARISCH" 2>&1 &

    local pid=$!
    log_info "tovarisch tcpdump started with PID $pid"
    return 0
}

# Collect tovarisch logs for BFD events
# Answers: Does tovarisch log BFD packet receive?
collect_tovarisch_bfd_logs() {
    log_info "Collecting tovarisch BFD log entries..."

    local output="$LAB_DIR/tovarisch-bfd-logs.txt"
    local tovarisch_pid

    {
        echo "=== tovarisch BFD-related logs ==="
        echo ""

        # Get raw log tail
        echo "--- Last 50 lines of tovarisch log ---"
        tail -n 50 "$TOVARISCH_LOG" 2>/dev/null || echo "Log not available"
        echo ""

        # Look for BFD keywords
        echo "--- BFD-related log entries ---"
        grep -iE "bfd|4784|multihop|bird|control|session|transmit|receive" "$TOVARISCH_LOG" 2>/dev/null | tail -n 30 || echo "No BFD keywords found"
        echo ""

        # Look for socket/network errors
        echo "--- Socket/network error entries ---"
        grep -iE "error|failed|cannot|bind|listen|permission|denied" "$TOVARISCH_LOG" 2>/dev/null | tail -n 20 || echo "No errors found"
        echo ""

        # tovarisch process state
        echo "--- tovarisch process state ---"
        tovarisch_pid=$(ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch 2>/dev/null || echo "")
        if [[ -n "$tovarisch_pid" ]]; then
            echo "PID: $tovarisch_pid"
            ip netns exec "$NS_TOVARISCH" cat "/proc/$tovarisch_pid/status" 2>/dev/null | grep -E "Name|State|Pid" || echo "Cannot read proc"
        else
            echo "tovarisch not running"
        fi

    } > "$output" 2>&1

    log_info "tovarisch BFD logs collected to $output"
}

# Collect BFD tx statistics from tovarisch runtime
collect_tovarisch_bfd_tx_stats() {
    log_info "Collecting tovarisch BFD TX statistics..."

    local output="$LAB_DIR/tovarisch-bfd-tx-stats.txt"

    {
        echo "=== tovarisch BFD TX statistics ==="
        echo ""

        # Check HTTP status for BFD TX/rx counters if available
        if [[ -s "$STATUS_HTTP_OUTPUT" ]]; then
            echo "--- BFD state from HTTP status ---"
            jq -r '.checks[] | select(.name == "bfd")' "$STATUS_HTTP_OUTPUT" 2>/dev/null || echo "Cannot parse BFD state"
            echo ""
        fi

        # Also check ACT 2 HTTP status if available
        if [[ -s "$LAB_DIR/status-http-bfd.json" ]]; then
            echo "--- BFD state from ACT 2 HTTP status ---"
            jq -r '.checks[] | select(.name == "bfd")' "$LAB_DIR/status-http-bfd.json" 2>/dev/null || echo "Cannot parse"
            echo ""
        fi

        # Check if there are any metrics endpoints
        echo "--- Checking for metrics endpoint ---"
        if command -v curl &> /dev/null; then
            ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/metrics" 2>&1 | head -n 20 || echo "Metrics endpoint not available"
        fi

    } > "$output" 2>&1

    log_info "BFD TX stats collected to $output"
}

# Comprehensive ACT 2.2 diagnostic collection
# Run this BEFORE assert_bfd_up to establish baseline, and AFTER for comparison
collect_tovarisch_bfd_diagnostics() {
    log_info "=== ACT 2.2: Collecting tovarisch BFD diagnostics ==="

    # 1. Socket state: Is tovarisch listening on UDP/4784?
    collect_tovarisch_socket_state

    # 2. tovarisch logs: Does tovarisch log BFD packet receive?
    collect_tovarisch_bfd_logs

    # 3. BFD TX stats
    collect_tovarisch_bfd_tx_stats

    # 4. veth interface stats: Were there receive/transmit errors?
    log_info "=== veth interface statistics ==="
    {
        echo "--- veth-tovarisch RX/TX stats ---"
        ip netns exec "$NS_TOVARISCH" ip -s link show "$VETH_TOVARISCH" 2>/dev/null || echo "Cannot show veth"
        echo ""
        echo "--- veth-bird RX/TX stats ---"
        ip netns exec "$NS_BIRD" ip -s link show "$VETH_BIRD" 2>/dev/null || echo "Cannot show veth"
    } | tee "$LAB_DIR/veth-stats.txt"

    log_info "ACT 2.2 diagnostics complete"
}
