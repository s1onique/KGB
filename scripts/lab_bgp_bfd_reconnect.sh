#!/bin/bash
# lab_bgp_bfd_reconnect.sh — BGP/BFD reconnect proof for tovarisch
#
# Proves: BGP loses TCP and returns to Established WITHOUT restarting tovarisch.
#
# Failure injection: BIRD restart (deterministic)
#
# Primary execution: GitHub Actions (workflow_dispatch)
# Local execution: optional for debugging only
#
# NOT part of make gate.

set -euo pipefail

# Source shared library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_bgp_bfd_netns_lib.sh
source "${SCRIPT_DIR}/lab_bgp_bfd_netns_lib.sh"

# Reconnect lab constants
declare -g LAB_NAME="kgb-bgp-bfd-reconnect"
declare -g ARTIFACT_DIR=""

# ============================================================================
# Reconnect Lab Functions
# ============================================================================

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
    for cmd in ip ss jq curl pgrep pkill bird; do
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
collect_baseline() {
    log_info "=== Collecting baseline artifacts ==="

    collect_status
    collect_status_http
    collect_bgp_protocols
    collect_bfd_sessions
    collect_socket_state
}

# Collect during-failure artifacts (best-effort: BIRD intentionally stopped)
collect_during_failure() {
    log_info "=== Collecting during-failure artifacts ==="

    # HTTP status (best-effort)
    local http_out="$ARTIFACT_DIR/during-failure-status-http.json"
    if command -v curl >/dev/null 2>&1; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" \
            > "$http_out" 2>&1 || echo "FAILED_TO_COLLECT_STATUS_HTTP" > "$http_out"
    fi

    # BIRD intentionally stopped - best-effort BIRD artifact collection
    birdc_lab show protocols all > "$ARTIFACT_DIR/during-failure-bird-protocols.txt" 2>&1 \
        || echo "BIRD_STOPPED_DURING_FAILURE_INJECTION" > "$ARTIFACT_DIR/during-failure-bird-protocols.txt"
    birdc_lab show bfd sessions > "$ARTIFACT_DIR/during-failure-bird-bfd-sessions.txt" 2>&1 \
        || echo "BIRD_STOPPED_DURING_FAILURE_INJECTION" > "$ARTIFACT_DIR/during-failure-bird-bfd-sessions.txt"

    collect_socket_state_for_phase "during-failure" || true
}

# Collect after-recovery artifacts
collect_after_recovery() {
    log_info "=== Collecting after-recovery artifacts ==="

    local output="$ARTIFACT_DIR/after-recovery-status-http.json"
    if command -v curl &> /dev/null; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" > "$output" 2>&1 || true
    fi

    collect_bgp_protocols "after-recovery"
    collect_bfd_sessions
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

# Stop BIRD
stop_bird() {
    log_info "Stopping BIRD..."
    birdc_lab disable all 2>/dev/null || true
    birdc_lab down 2>/dev/null || true
    ip netns exec "$NS_BIRD" pkill bird 2>/dev/null || true
    sleep 2
    log_info "BIRD stopped"
}

# Start BIRD
start_bird() {
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

# ============================================================================
# Main Lab Execution
# ============================================================================

run_reconnect_lab() {
    log_info "=== BGP/BFD Reconnect Lab ==="
    log_info "Proof: BGP recovers WITHOUT restarting tovarisch"
    log_info ""

    # Preflight checks
    require_linux
    require_reconnect_dependencies

    # Setup
    setup_temp_dir
    setup_trap

    # Artifact directory within LAB_DIR
    ARTIFACT_DIR="$LAB_DIR/artifacts"
    mkdir -p "$ARTIFACT_DIR"

    # Create topology
    create_namespaces
    configure_interfaces

    # Generate configs
    generate_bird_config
    generate_tovarisch_config
    generate_prefix_file

    # Verify topology
    if ! verify_topology; then
        log_error "Topology verification failed"
        print_diagnostics
        exit 1
    fi

    # Start services
    start_bird
    start_tovarisch

    # Wait for baseline BFD Up
    log_info ""
    log_info "=== Phase 1: Baseline ==="
    if ! wait_bfd_up; then
        log_error "[FAIL] BFD did not reach Up"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] Baseline BFD Up"

    # Wait for baseline BGP Established
    if ! wait_bgp_established; then
        log_error "[FAIL] BGP did not reach Established"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] Baseline BGP Established"

    # Collect baseline artifacts
    collect_baseline

    # Capture tovarisch PID before failure
    capture_tovarisch_pid "$ARTIFACT_DIR/tovarisch-pid-before.txt"
    local pid_before
    pid_before=$(cat "$ARTIFACT_DIR/tovarisch-pid-before.txt")
    log_info "tovarisch PID before: $pid_before"

    # =========================================================================
    # Phase 2: Inject failure via BIRD restart
    # =========================================================================
    log_info ""
    log_info "=== Phase 2: Inject BGP failure (BIRD restart) ==="

    stop_bird
    log_info "BIRD stopped - BGP should fail"

    # Brief wait, then collect during-failure state
    sleep 2
    collect_during_failure

    # Verify tovarisch is STILL RUNNING during failure
    if ! assert_tovarisch_running; then
        log_error "[FAIL] tovarisch died during BGP failure"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] tovarisch still running during failure"

    # Capture PID during failure (should be same)
    capture_tovarisch_pid "$ARTIFACT_DIR/tovarisch-pid-during-failure.txt"

    # =========================================================================
    # Phase 3: Recover BGP
    # =========================================================================
    log_info ""
    log_info "=== Phase 3: Recover BGP ==="

    start_bird

    # Wait for BFD Up
    if ! wait_bfd_up; then
        log_error "[FAIL] BFD did not recover"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] BFD recovered"

    # Wait for BGP Established
    if ! wait_bgp_established; then
        log_error "[FAIL] BGP did not recover"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] BGP recovered"

    # Collect after-recovery artifacts
    collect_after_recovery

    # Capture tovarisch PID after recovery
    capture_tovarisch_pid "$ARTIFACT_DIR/tovarisch-pid-after.txt"
    local pid_after
    pid_after=$(cat "$ARTIFACT_DIR/tovarisch-pid-after.txt")
    log_info "tovarisch PID after: $pid_after"

    # =========================================================================
    # Phase 4: Verify proof
    # =========================================================================
    log_info ""
    log_info "=== Phase 4: Verify reconnection proof ==="

    local exit_code=0

    # 1. PID unchanged proof
    log_info ""
    if [[ "$pid_before" == "$pid_after" ]] && [[ -n "$pid_before" ]]; then
        log_info "[PASS] tovarisch PID unchanged: $pid_before == $pid_after"
    else
        log_error "[FAIL] tovarisch PID changed: $pid_before != $pid_after"
        exit_code=1
    fi

    # 2. After-recovery BGP is Established
    log_info ""
    if grep -qE "Established" "$ARTIFACT_DIR/after-recovery-bird-protocols.txt" 2>/dev/null; then
        log_info "[PASS] After-recovery BGP is Established"
    else
        log_error "[FAIL] After-recovery BGP not Established"
        exit_code=1
    fi

    # 3. After-recovery status shows BGP OK
    log_info ""
    if verify_after_recovery_status_ok; then
        log_info "[PASS] After-recovery HTTP status shows BGP OK"
    else
        log_error "[FAIL] After-recovery HTTP status does not show BGP OK"
        exit_code=1
    fi

    # 4. Artifact files exist
    log_info ""
    log_info "=== Artifact verification ==="
    local required_artifacts=(
        "baseline-status-http.json"
        "during-failure-status-http.json"
        "after-recovery-status-http.json"
        "baseline-bird-protocols.txt"
        "during-failure-bird-protocols.txt"
        "after-recovery-bird-protocols.txt"
        "tovarisch-pid-before.txt"
        "tovarisch-pid-after.txt"
    )

    for artifact in "${required_artifacts[@]}"; do
        if [[ -f "$ARTIFACT_DIR/$artifact" ]]; then
            log_info "[PASS] Artifact exists: $artifact"
        else
            log_error "[FAIL] Artifact missing: $artifact"
            exit_code=1
        fi
    done

    # Copy tovarisch log to artifact dir
    if [[ -f "$TOVARISCH_LOG" ]]; then
        cp "$TOVARISCH_LOG" "$ARTIFACT_DIR/tovarisch.log"
        log_info "tovarisch.log copied to artifact dir"
    fi

    # Make artifacts readable
    chmod -R a+rX "$LAB_DIR" 2>/dev/null || true

    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact dir: $ARTIFACT_DIR"

    if [[ $exit_code -eq 0 ]]; then
        log_info "Result: PASS"
        log_info ""
        log_info "=== PROOF ACHIEVED ==="
        log_info "BGP lost TCP and returned to Established"
        log_info "WITHOUT restarting tovarisch"
        log_info "tovarisch PID: $pid_before"
    else
        log_error "Result: FAIL"
    fi

    return $exit_code
}

# Run lab when executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_reconnect_lab "$@"
fi
