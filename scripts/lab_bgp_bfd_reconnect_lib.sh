#!/bin/bash
# lab_bgp_bfd_reconnect_lib.sh — Shared library for BGP/BFD reconnect labs
#
# Common functions for BGP/BFD reconnect scenario scripts.
# This is sourced by lab_bgp_bfd_reconnect.sh and lab_bgp_bfd_reconnect_bgp_reset.sh.

set -euo pipefail

# Source shared netns library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_bgp_bfd_netns_lib.sh
source "${SCRIPT_DIR}/lab_bgp_bfd_netns_lib.sh"

# Global variables (set by main script)
declare -g LAB_NAME=""
declare -g ARTIFACT_DIR=""

# === Shared Reconnect Lab Functions ===

# Preflight check: require Linux
require_linux() {
    if [[ "$(uname -s)" != "Linux" ]]; then
        log_error "This lab requires Linux network namespaces"
        return 1
    fi
}

# Preflight check: require reconnect-specific dependencies
require_reconnect_dependencies() {
    local missing=0
    for cmd in ip ss jq curl pgrep pkill bird birdc; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "missing required command: $cmd"
            missing=1
        fi
    done
    return "$missing"
}

# Capture tovarisch PID to a file
capture_tovarisch_pid() {
    local output_file="$1"
    local pid
    pid=$(ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch 2>/dev/null || echo "")
    echo "$pid" > "$output_file"
    log_info "tovarisch PID captured: $pid -> $output_file"
}

# Assert tovarisch is still running
assert_tovarisch_running() {
    if ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch &> /dev/null; then
        return 0
    else
        log_error "tovarisch is not running"
        return 1
    fi
}

# Collect baseline artifacts (before failure injection)
# Includes BIRD route table to verify initial route import.
collect_baseline() {
    log_info "=== Collecting baseline artifacts ==="

    collect_status
    collect_status_http

    # Explicitly write baseline HTTP artifact to expected location
    local baseline_http="$ARTIFACT_DIR/baseline-status-http.json"
    if command -v curl >/dev/null 2>&1; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" \
            > "$baseline_http" 2>&1 || {
                log_warn "Failed to collect baseline HTTP status"
                echo "FAILED_TO_COLLECT_BASELINE_STATUS_HTTP" > "$baseline_http"
            }
    fi

    collect_bgp_protocols
    collect_bfd_sessions
    # NEW: Collect BIRD routes to verify initial route import
    collect_bird_routes "baseline"
    collect_bird_protocol_detail "baseline"
    collect_socket_state
}

# Collect after-recovery artifacts
# Now includes BIRD routes to verify route import after reconnect.
# Critical for catching the false-green: BGP Established but 0 imported routes.
collect_after_recovery() {
    log_info "=== Collecting after-recovery artifacts ==="

    local output="$ARTIFACT_DIR/after-recovery-status-http.json"
    if command -v curl &> /dev/null; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" > "$output" 2>&1 || true
    fi

    collect_bgp_protocols "after-recovery"
    collect_bfd_sessions
    # NEW: Collect BIRD routes to verify route import after reconnect
    collect_bird_routes "after-recovery"
    collect_bird_protocol_detail "after-recovery"
    collect_socket_state
}

# Collect BGP protocols status
collect_bgp_protocols() {
    local suffix="${1:-baseline}"
    local output="$ARTIFACT_DIR/${suffix}-bird-protocols.txt"

    if birdc_lab show protocols all 2>/dev/null > "$output"; then
        log_info "BGP protocols collected: $output"
    else
        log_warn "Failed to collect BGP protocols"
        echo "FAILED_TO_COLLECT" > "$output"
    fi
}

# Collect BFD sessions status
collect_bfd_sessions() {
    local output="$ARTIFACT_DIR/bird-bfd-sessions.txt"
    if birdc_lab show bfd sessions 2>/dev/null > "$output"; then
        log_info "BFD sessions collected"
    else
        echo "BFD_QUERY_FAILED" > "$output"
    fi
}

# Collect BIRD routes (primary: verify route import after reconnect)
# This captures the route table so we can assert the deterministic prefix is present.
# Critical for catching the false-green: BGP Established but 0 imported routes.
collect_bird_routes() {
    local suffix="${1:-baseline}"
    local output="$ARTIFACT_DIR/${suffix}-bird-routes.txt"

    # Collect routes from tovarisch protocol (the BGP peer)
    if birdc_lab show route protocol tovarisch all 2>/dev/null > "$output"; then
        log_info "BIRD routes collected for phase '$suffix': $output"
    else
        log_warn "Failed to collect BIRD routes for phase '$suffix'"
        echo "BIRD_QUERY_FAILED" > "$output"
    fi
}

# Collect BIRD protocol detailed stats (includes import counters)
collect_bird_protocol_detail() {
    local suffix="${1:-baseline}"
    local output="$ARTIFACT_DIR/${suffix}-bird-protocol-detail.txt"

    if birdc_lab "show protocol all tovarisch" 2>/dev/null > "$output"; then
        log_info "BIRD protocol detail collected for phase '$suffix': $output"
    else
        log_warn "Failed to collect BIRD protocol detail for phase '$suffix'"
        echo "BIRD_QUERY_FAILED" > "$output"
    fi
}

# Verify BIRD has imported the deterministic test prefix.
# Returns 0 if the prefix is present in BIRD's route table.
# Returns 1 if the prefix is missing (the false-green condition).
verify_bird_route_import() {
    local routes_file="$1"
    local expected_prefix="${2:-10.77.77.0/24}"

    if [[ ! -f "$routes_file" ]] || [[ ! -s "$routes_file" ]]; then
        log_error "[FAIL] Routes file not available: $routes_file"
        return 1
    fi

    # Use -F for literal matching (prefixes contain dots which are regex wildcards)
    if grep -qF -- "$expected_prefix" "$routes_file" 2>/dev/null; then
        log_info "[PASS] Deterministic prefix '$expected_prefix' found in BIRD route table"
        return 0
    else
        log_error "[FAIL] Deterministic prefix '$expected_prefix' NOT found in BIRD route table"
        echo "--- BIRD routes content ---"
        cat "$routes_file" 2>/dev/null || echo "(empty)"
        return 1
    fi
}

# Verify BIRD protocol shows non-zero import metrics.
# This is secondary to route presence but provides additional proof.
verify_bird_import_counters() {
    local protocol_detail_file="$1"

    if [[ ! -f "$protocol_detail_file" ]] || [[ ! -s "$protocol_detail_file" ]]; then
        log_error "[FAIL] Protocol detail file not available: $protocol_detail_file"
        return 1
    fi

    # Check for "Routes: N imported" where N > 0
    # BIRD format: "Routes: 0 imported, 1 exported, 0 preferred"
    if grep -qE "Routes: [1-9][0-9]* imported" "$protocol_detail_file" 2>/dev/null; then
        local imported_count
        imported_count=$(grep -E "Routes: [0-9]+ imported" "$protocol_detail_file" | head -1)
        log_info "[PASS] BIRD shows non-zero import count: $imported_count"
        return 0
    fi

    if grep -qE "Routes: 0 imported" "$protocol_detail_file" 2>/dev/null; then
        log_error "[FAIL] BIRD shows 0 imported routes"
        echo "--- BIRD protocol detail ---"
        grep -E "(Routes:|Import updates:)" "$protocol_detail_file" 2>/dev/null || echo "(no match)"
        return 1
    fi

    log_warn "[INFO] Could not parse import counters from BIRD protocol detail"
    return 1
}

# Collect socket state (legacy, single file)
collect_socket_state() {
    local output="$ARTIFACT_DIR/tovarisch-socket-state.txt"

    {
        echo "=== tovarisch socket state ==="
        ip netns exec "$NS_TOVARISCH" ss -lunp 2>&1 || echo "ss failed"
    } > "$output" 2>&1
}

# Collect socket state for a specific phase (phase-specific file)
collect_socket_state_for_phase() {
    local suffix="$1"
    local output="$ARTIFACT_DIR/${suffix}-socket-state.txt"

    {
        echo "=== tovarisch socket state: $suffix ==="
        ip netns exec "$NS_TOVARISCH" ss -tanp 2>&1 || true
        ip netns exec "$NS_TOVARISCH" ss -lunp 2>&1 || true
    } > "$output" 2>&1
    log_info "Socket state collected for phase '$suffix': $output"
}

# Wait for BFD Up
wait_bfd_up() {
    log_info "Waiting for BFD Up (${WAIT_BFD_CONVERGE}s)..."
    local elapsed=0
    local interval=2

    while [[ $elapsed -lt $WAIT_BFD_CONVERGE ]]; do
        local bfd_status
        bfd_status=$(birdc_lab show bfd sessions 2>/dev/null || echo "")
        if echo "$bfd_status" | grep -qE '(^|[[:space:]])Up([[:space:]]|$)'; then
            log_info "BFD is Up"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "."
    done
    echo ""
    log_warn "BFD Up timeout"
    return 1
}

# Wait for BGP Established
wait_bgp_established() {
    log_info "Waiting for BGP Established (${WAIT_BGP_CONVERGE}s)..."
    local elapsed=0
    local interval=2

    while [[ $elapsed -lt $WAIT_BGP_CONVERGE ]]; do
        local bgp_status
        bgp_status=$(birdc_lab show protocols tovarisch 2>/dev/null || echo "")
        if echo "$bgp_status" | grep -qE "Established"; then
            log_info "BGP is Established"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "."
    done
    echo ""
    log_warn "BGP Established timeout"
    return 1
}

# Verify after-recovery status shows BGP OK
verify_after_recovery_status_ok() {
    local status_file="$ARTIFACT_DIR/after-recovery-status-http.json"

    if [[ ! -f "$status_file" ]] || [[ ! -s "$status_file" ]]; then
        log_error "After-recovery status JSON not available"
        return 1
    fi

    # Check BGP status (runtime JSON uses .status, not .state)
    local bgp_state
    bgp_state=$(jq -r '.checks[] | select(.name == "bgp") | (.status // .state // "unknown")' "$status_file" 2>/dev/null || echo "unknown")

    if [[ "$bgp_state" == "ok" ]] || [[ "$bgp_state" == "up" ]]; then
        log_info "[PASS] After-recovery BGP status: $bgp_state"
        return 0
    else
        log_error "[FAIL] After-recovery BGP status: $bgp_state (expected ok/up)"
        return 1
    fi
}

# Stop BIRD
stop_bird() {
    log_info "Stopping BIRD..."
    birdc_lab disable all 2>/dev/null || true
    birdc_lab down 2>/dev/null || true
    ip netns exec "$NS_BIRD" pkill bird 2>/dev/null || true
    sleep 2
    log_info "BIRD stopped"
}

# Start BIRD (from lib)
start_bird_reconnect() {
    log_info "Starting BIRD..."
    ip netns exec "$NS_BIRD" bird -s "$BIRD_SOCKET" -f -c "$BIRD_CONFIG" &
    sleep "$WAIT_BIRD_START"
    if ! ip netns exec "$NS_BIRD" pgrep -x bird &> /dev/null; then
        log_error "BIRD failed to start"
        return 1
    fi
    log_info "BIRD started"
    return 0
}

# === Route Import Verification Functions ===
# Moved from lab_bgp_bfd_reconnect.sh to reduce main script size.

# Verify baseline route import (called from main script)
verify_baseline_route_import() {
    log_info "=== Phase 4b: Baseline route import verification ==="
    if verify_bird_route_import "$ARTIFACT_DIR/baseline-bird-routes.txt" "10.77.77.0/24"; then
        log_info "[PASS] Baseline: tovarisch advertised prefix 10.77.77.0/24 to BIRD"
        return 0
    else
        log_error "[FAIL] Baseline: tovarisch did NOT advertise prefix to BIRD"
        return 1
    fi
}

# Verify after-recovery route import (called from main script)
# Critical for catching the false-green: BGP Established but 0 imported routes.
verify_after_recovery_route_import() {
    log_info "=== Phase 4c: After-recovery route import verification ==="
    if verify_bird_route_import "$ARTIFACT_DIR/after-recovery-bird-routes.txt" "10.77.77.0/24"; then
        log_info "[PASS] After-recovery: tovarisch re-advertised prefix 10.77.77.0/24 to BIRD"
        return 0
    else
        log_error "[FAIL] After-recovery: tovarisch did NOT re-advertise prefix to BIRD"
        log_error "This is the false-green condition: BGP Established but 0 imported routes"
        return 1
    fi
}

# Verify after-recovery import counters (secondary proof, non-fatal)
verify_after_recovery_import_counters() {
    log_info "=== Phase 4d: After-recovery import counter verification ==="
    if verify_bird_import_counters "$ARTIFACT_DIR/after-recovery-bird-protocol-detail.txt"; then
        log_info "[PASS] After-recovery: BIRD shows non-zero import counters"
        return 0
    else
        log_warn "[INFO] Could not verify import counters (not fatal if route present)"
        return 0
    fi
}

# Verify required route artifacts exist
verify_route_artifacts() {
    log_info "=== Verifying route artifacts ==="
    local exit_code=0
    local route_artifacts=(
        "baseline-bird-routes.txt"
        "after-recovery-bird-routes.txt"
        "baseline-bird-protocol-detail.txt"
        "after-recovery-bird-protocol-detail.txt"
    )

    for artifact in "${route_artifacts[@]}"; do
        if [[ -f "$ARTIFACT_DIR/$artifact" ]]; then
            log_info "[PASS] Artifact exists: $artifact"
        else
            log_error "[FAIL] Artifact missing: $artifact"
            exit_code=1
        fi
    done
    return $exit_code
}
