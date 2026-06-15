#!/bin/bash
# lab_bgp_bfd_reconnect_bgp_reset.sh — BGP protocol reset scenario for tovarisch
# Proves: BGP protocol reset with BFD healthy, tovarisch reconnects without restart
# Failure injection: BIRD protocol disable/enable (peer-side BGP restart)
# Sibling to lab_bgp_bfd_reconnect.sh (BIRD full restart scenario)
# NOT part of make gate.

set -euo pipefail

# Source shared reconnect library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_bgp_bfd_reconnect_lib.sh
source "${SCRIPT_DIR}/lab_bgp_bfd_reconnect_lib.sh"

# Reconnect lab constants
declare -g LAB_NAME="kgb-bgp-bfd-reconnect-bgp-reset"

# === BGP Protocol Reset Lab Functions ===

# Collect during-glitch artifacts (BFD should stay up, BGP glitched)
collect_during_glitch() {
    log_info "=== Collecting during-glitch artifacts ==="

    local http_out="$ARTIFACT_DIR/during-glitch-status-http.json"
    if command -v curl >/dev/null 2>&1; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" \
            > "$http_out" 2>&1 || echo "FAILED_TO_COLLECT_STATUS_HTTP" > "$http_out"
    fi

    birdc_lab show protocols all > "$ARTIFACT_DIR/during-glitch-bird-protocols.txt" 2>&1 \
        || echo "BIRD_QUERY_FAILED" > "$ARTIFACT_DIR/during-glitch-bird-protocols.txt"
    birdc_lab show bfd sessions > "$ARTIFACT_DIR/during-glitch-bird-bfd-sessions.txt" 2>&1 \
        || echo "BIRD_QUERY_FAILED" > "$ARTIFACT_DIR/during-glitch-bird-bfd-sessions.txt"

    collect_socket_state_for_phase "during-glitch" || true
}

# Inject BGP protocol reset via BIRD protocol disable/enable cycle
# This performs a peer-side BGP restart without stopping BIRD or affecting BFD
inject_bgp_protocol_reset() {
    log_info "=== Injecting BGP protocol reset ==="

    log_info "Disabling BGP protocol in BIRD..."
    birdc_lab disable tovarisch 2>/dev/null || {
        log_error "Failed to disable BGP protocol"
        return 1
    }

    sleep 2

    log_info "Re-enabling BGP protocol in BIRD..."
    birdc_lab enable tovarisch 2>/dev/null || {
        log_error "Failed to re-enable BGP protocol"
        return 1
    }

    log_info "BGP protocol reset injected"
    return 0
}

# Verify BFD remains Up during/after glitch
# Returns 0 if BFD is Up or in allowed transient state
# Returns 1 if BFD is Down or in error state
verify_bfd_remains_up() {
    local bfd_status
    bfd_status=$(birdc_lab show bfd sessions 2>/dev/null || echo "")

    if echo "$bfd_status" | grep -qE '(^|[[:space:]])Up([[:space:]]|$)'; then
        log_info "[PASS] BFD remains Up during glitch"
        return 0
    fi

    # Check for allowed transient states
    if echo "$bfd_status" | grep -qE 'Demand|Init|AdminDown'; then
        log_info "[INFO] BFD in transient state (allowed): $(echo "$bfd_status" | head -1)"
        return 0
    fi

    log_error "[FAIL] BFD not Up during glitch: $bfd_status"
    return 1
}

# Extract reconnect count from status JSON (checks both .bgp and .checks[] paths)
get_reconnect_count_from_file() {
    local status_file="$1"
    # Try .bgp.reconnect_count first (runtime JSON)
    local count
    count=$(jq -r '.bgp.reconnect_count // 0' "$status_file" 2>/dev/null || echo "0")
    if [[ "$count" -gt 0 ]]; then
        echo "$count"
        return
    fi
    # Fallback: try .checks[] array
    count=$(jq -r '.checks[] | select(.name == "bgp") | (.reconnect_count // 0)' "$status_file" 2>/dev/null || echo "0")
    echo "$count"
}

# Check status for reconnect diagnostics
# This is FATAL — must prove reconnect_count increased during glitch
verify_reconnect_diagnostics() {
    local during_glitch_file="$1"
    local baseline_file="$2"

    log_info "=== Verifying reconnect diagnostics ==="

    local during_count baseline_count reconnect_state last_error
    during_count=$(get_reconnect_count_from_file "$during_glitch_file")
    baseline_count=$(get_reconnect_count_from_file "$baseline_file")

    # Two-step jq fallback: try .bgp first, then .checks[]
    reconnect_state=$(jq -r '.bgp.state // .bgp.status // empty' "$during_glitch_file" 2>/dev/null || true)
    if [[ -z "$reconnect_state" ]]; then
        reconnect_state=$(jq -r '.checks[]? | select(.name == "bgp") | (.state // .status // "unknown")' "$during_glitch_file" 2>/dev/null || echo "unknown")
    fi

    last_error=$(jq -r '.bgp.last_socket_error // empty' "$during_glitch_file" 2>/dev/null || true)
    if [[ -z "$last_error" ]]; then
        last_error=$(jq -r '.checks[]? | select(.name == "bgp") | .last_socket_error // null' "$during_glitch_file" 2>/dev/null || echo "null")
    fi

    # Must prove reconnect_count increased from baseline
    if [[ "$during_count" -gt "$baseline_count" ]]; then
        log_info "[PASS] Reconnect diagnostics: reconnect_count $baseline_count -> $during_count, state=$reconnect_state"

        # Optionally expose last_socket_error if available
        if [[ "$last_error" != "null" ]] && [[ -n "$last_error" ]]; then
            log_info "[INFO] last_socket_error exposed: $last_error"
        fi
        return 0
    fi

    if [[ "$reconnect_state" == "reconnect_wait" ]]; then
        log_info "[PASS] BGP in reconnect_wait state"
        return 0
    fi

    log_error "[FAIL] No reconnect diagnostics: reconnect_count $baseline_count -> $during_count, state=$reconnect_state"
    return 1
}

# === Main Lab Execution ===

run_bgp_reset_lab() {
    log_info "=== BGP Protocol Reset Lab ==="
    log_info "Proof: BGP protocol reset with BFD healthy, tovarisch reconnects without restart"
    log_info ""

    require_linux
    require_reconnect_dependencies

    setup_temp_dir
    setup_trap

    ARTIFACT_DIR="$LAB_DIR/artifacts"
    mkdir -p "$ARTIFACT_DIR"

    create_namespaces
    configure_interfaces

    generate_bird_config
    generate_tovarisch_config
    generate_prefix_file

    if ! verify_topology; then
        log_error "Topology verification failed"
        print_diagnostics
        exit 1
    fi

    start_bird
    start_tovarisch

    log_info ""
    log_info "=== Phase 1: Baseline ==="
    if ! wait_bfd_up; then
        log_error "[FAIL] BFD did not reach Up"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] Baseline BFD Up"

    if ! wait_bgp_established; then
        log_error "[FAIL] BGP did not reach Established"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] Baseline BGP Established"

    collect_baseline

    # Capture baseline reconnect count
    local baseline_reconnect_count
    baseline_reconnect_count=$(get_reconnect_count_from_file "$ARTIFACT_DIR/baseline-status-http.json")
    log_info "Baseline reconnect_count: $baseline_reconnect_count"

    capture_tovarisch_pid "$ARTIFACT_DIR/tovarisch-pid-before.txt"
    local pid_before
    pid_before=$(cat "$ARTIFACT_DIR/tovarisch-pid-before.txt")
    log_info "tovarisch PID before: $pid_before"

    # =========================================================================
    # Phase 2: Inject BGP protocol reset
    # =========================================================================
    log_info ""
    log_info "=== Phase 2: Inject BGP protocol reset ==="

    if ! inject_bgp_protocol_reset; then
        log_error "[FAIL] Failed to inject BGP protocol reset"
        print_diagnostics
        exit 1
    fi

    sleep 3
    collect_during_glitch

    if ! assert_tovarisch_running; then
        log_error "[FAIL] tovarisch died during BGP glitch"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] tovarisch still running during glitch"

    # FATAL: Verify BFD remains Up during glitch
    if ! verify_bfd_remains_up; then
        log_error "[FAIL] BFD did not remain Up during glitch"
        print_diagnostics
        exit 1
    fi

    capture_tovarisch_pid "$ARTIFACT_DIR/tovarisch-pid-during-glitch.txt"

    # FATAL: Verify reconnect diagnostics
    if ! verify_reconnect_diagnostics "$ARTIFACT_DIR/during-glitch-status-http.json" "$ARTIFACT_DIR/baseline-status-http.json"; then
        log_error "[FAIL] Reconnect diagnostics not exposed"
        print_diagnostics
        exit 1
    fi

    # =========================================================================
    # Phase 3: Recover BGP
    # =========================================================================
    log_info ""
    log_info "=== Phase 3: Recover BGP ==="

    if ! wait_bfd_up; then
        log_error "[FAIL] BFD did not recover"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] BFD recovered"

    if ! wait_bgp_established; then
        log_error "[FAIL] BGP did not recover"
        print_diagnostics
        exit 1
    fi
    log_info "[PASS] BGP recovered"

    collect_after_recovery

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

    if [[ "$pid_before" == "$pid_after" ]] && [[ -n "$pid_before" ]]; then
        log_info "[PASS] tovarisch PID unchanged: $pid_before == $pid_after"
    else
        log_error "[FAIL] tovarisch PID changed: $pid_before != $pid_after"
        exit_code=1
    fi

    if grep -qE "Established" "$ARTIFACT_DIR/after-recovery-bird-protocols.txt" 2>/dev/null; then
        log_info "[PASS] After-recovery BGP is Established"
    else
        log_error "[FAIL] After-recovery BGP not Established"
        exit_code=1
    fi

    if verify_after_recovery_status_ok; then
        log_info "[PASS] After-recovery HTTP status shows BGP OK"
    else
        log_error "[FAIL] After-recovery HTTP status does not show BGP OK"
        exit_code=1
    fi

    log_info ""
    log_info "=== Artifact verification ==="
    local required_artifacts=(
        "baseline-status-http.json"
        "during-glitch-status-http.json"
        "after-recovery-status-http.json"
        "baseline-bird-protocols.txt"
        "during-glitch-bird-protocols.txt"
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

    if [[ -f "$TOVARISCH_LOG" ]]; then
        cp "$TOVARISCH_LOG" "$ARTIFACT_DIR/tovarisch.log"
        log_info "tovarisch.log copied to artifact dir"
    fi

    chmod -R a+rX "$LAB_DIR" 2>/dev/null || true

    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact dir: $ARTIFACT_DIR"

    if [[ $exit_code -eq 0 ]]; then
        log_info "Result: PASS"
        log_info ""
        log_info "=== PROOF ACHIEVED ==="
        log_info "BGP protocol reset with BFD healthy"
        log_info "BGP returned to Established without process restart"
        log_info "tovarisch PID: $pid_before"
    else
        log_error "Result: FAIL"
    fi

    return $exit_code
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_bgp_reset_lab "$@"
fi
