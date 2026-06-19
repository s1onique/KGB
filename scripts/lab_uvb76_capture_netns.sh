#!/bin/bash
# lab_uvb76_capture_netns.sh — UVB-76 diagnostic capture netns fault lab
#
# Creates isolated Linux network namespaces with UVB-76 and tovarisch
# for testing diagnostic capture under network impairment conditions.
#
# This lab uses the REAL UVB-76 API to extract capture evidence,
# not log-grep synthesis.
#
# Primary execution: GitHub Actions (ubuntu-latest)
# Local execution: optional debugging only
#
# Trigger: workflow_dispatch (manual only)
# This is NOT part of make gate.

set -euo pipefail

# Source shared library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_uvb76_capture_netns_lib.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_lib.sh"

# Main lab execution
run_lab() {
    log_info "=== UVB-76 Diagnostic Capture Netns Lab ==="
    log_info "Using REAL UVB-76 API for capture evidence"
    log_info "Primary execution: GitHub Actions (workflow_dispatch)"
    log_info "Local execution: optional for debugging only"
    log_info ""

    # Pre-flight checks
    if ! check_linux; then
        exit 1
    fi
    if ! check_dependencies; then
        exit 1
    fi

    # Setup
    setup_temp_dir
    setup_trap

    # Build binaries
    log_info "Building tovarisch..."
    if ! make tovarisch-build 2>&1; then
        log_error "Failed to build tovarisch"
        exit 1
    fi

    log_info "Building UVB-76..."
    if ! make uvb76-build 2>&1; then
        log_error "Failed to build UVB-76"
        exit 1
    fi

    # Create topology
    create_namespaces
    configure_interfaces

    # Generate configs
    generate_tovarisch_config
    generate_uvb76_config

    # Verify topology came up
    if ! verify_topology; then
        log_error "Topology verification failed"
        exit 1
    fi

    # Collect topology info
    collect_topology_info

    # Start tovarisch
    if ! start_tovarisch; then
        log_error "Failed to start tovarisch"
        exit 1
    fi

    # Wait for tovarisch HTTP to be ready
    if ! wait_for_tovarisch_http; then
        log_error "tovarisch HTTP endpoint not ready"
        exit 1
    fi

    # Verify tovarisch status endpoints
    if ! verify_tovarisch_status; then
        log_error "tovarisch status verification failed"
        exit 1
    fi

    # Start UVB-76
    if ! start_uvb76; then
        log_error "Failed to start UVB-76"
        exit 1
    fi

    # Verify baseline connectivity
    if ! verify_baseline_connectivity; then
        log_error "Baseline connectivity verification failed"
        exit 1
    fi

    # Authenticate to UVB-76 API
    if ! uvb76_authenticate; then
        log_error "Failed to authenticate to UVB-76 API"
        exit 1
    fi

    # Wait for HTTP probe to collect baseline samples
    log_info "Waiting for HTTP probe to collect baseline samples..."
    sleep 20

    # ========================================
    # PHASE 1: Baseline capture
    # ========================================
    log_info ""
    log_info "=== PHASE 1: Baseline Capture ==="

    # Query the spikes API with captures
    query_spikes_api "$LAB_DIR/spikes-baseline.json" "lab-tovarisch" "true"

    # Extract capture evidence from API response
    # Note: For baseline, we don't expect a natural spike to have occurred yet
    # We accept: ok (good), no_spikes, no_capture_for_phase (acceptable - no natural spike)
    extract_latest_capture "$LAB_DIR/spikes-baseline.json" "$CAPTURE_BASELINE_FILE" "baseline"
    REQUESTED_PATH_BASELINE="/status.json?include=network_diag"

    # Set baseline cursor - captures must be created AFTER this time for subsequent phases
    set_phase_cursor "baseline"

    # Check if baseline capture was successful
    # Acceptable statuses for baseline:
    # - "ok" = capture exists and succeeded (may have occurred during warmup)
    # - "no_capture_for_phase" = no captures created during baseline window (expected - no natural spike)
    # - "no_capture_yet" = spike exists but no capture yet
    # - "no_spikes" = no spikes detected yet
    local baseline_status
    baseline_status=$(jq -r '.status' "$CAPTURE_BASELINE_FILE" 2>/dev/null || echo "unknown")

    if [[ "$baseline_status" == "ok" ]]; then
        BASELINE_CAPTURE_OK=true
        log_info "[PASS] Baseline capture successful (status: $baseline_status)"
    elif [[ "$baseline_status" == "no_capture_for_phase" || "$baseline_status" == "no_spikes" || "$baseline_status" == "no_capture_yet" ]]; then
        # These are acceptable for baseline - we're establishing a pre-defect state
        # We just need connectivity to be working, not necessarily a spike
        BASELINE_CAPTURE_OK=true
        log_info "[PASS] Baseline phase complete (status: $baseline_status - acceptable, no natural spike expected)"
    else
        BASELINE_CAPTURE_OK=false
        log_error "[FAIL] Baseline capture failed: $baseline_status"
    fi

    # ========================================
    # PHASE 2: Inject defect and test
    # ========================================
    log_info ""
    log_info "=== PHASE 2: Defect Injection ==="

    # Set defect cursor BEFORE injecting defect - captures after this are during defect
    set_phase_cursor "defect"

    # Inject 100% loss defect (deterministic)
    inject_netem_defect
    DEFECT_MODE="100pct-loss"

    # Verify defect is in place - ping MUST fail
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_error "[FAIL] Defect not working - ping succeeded when it should fail"
        DEFECT_OBSERVED=false
    else
        log_info "[PASS] Defect verified - ping fails as expected"

        # Wait for probe to observe defect
        sleep 20

        # Query the spikes API during defect
        # Using defect cursor to ensure we only get captures created after defect was injected
        query_spikes_api "$LAB_DIR/spikes-during-defect.json" "lab-tovarisch" "true"
        extract_latest_capture "$LAB_DIR/spikes-during-defect.json" "$CAPTURE_DURING_DEFECT_FILE" "during-defect"
        REQUESTED_PATH_DURING_DEFECT="/status.json?include=network_diag"

        # Check if defect was observed (timeout/error status)
        local during_status
        during_status=$(jq -r '.status' "$CAPTURE_DURING_DEFECT_FILE" 2>/dev/null || echo "unknown")

        if [[ "$during_status" == "timeout" || "$during_status" == "error" ]]; then
            DEFECT_OBSERVED=true
            log_info "[PASS] Defect observed in diagnostic capture (status: $during_status)"
        elif [[ "$during_status" == "ok" ]]; then
            # Capture succeeded despite defect - check latency
            local latency_ms
            latency_ms=$(jq -r '.latency_ms // 0' "$CAPTURE_DURING_DEFECT_FILE" 2>/dev/null || echo "0")
            if [[ "$latency_ms" -gt 1000 ]]; then
                DEFECT_OBSERVED=true
                log_info "[PASS] Defect observed - high latency: ${latency_ms}ms"
            else
                DEFECT_OBSERVED=false
                log_error "[FAIL] Defect not observed - latency: ${latency_ms}ms"
            fi
        else
            DEFECT_OBSERVED=false
            log_error "[FAIL] Defect effect unclear: $during_status"
        fi
    fi

    # ========================================
    # PHASE 3: Recovery
    # ========================================
    log_info ""
    log_info "=== PHASE 3: Recovery ==="

    # Clear the defect FIRST
    clear_defect

    # Set recovery cursor AFTER clearing defect - captures after this are post-recovery
    set_phase_cursor "recovery"

    # Wait for connectivity to restore
    log_info "Waiting for connectivity to restore..."
    sleep 3

    # Verify connectivity restored
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_info "Connectivity restored"
    else
        log_warn "Connectivity may not be fully restored"
    fi

    # Wait for probe to stabilize
    sleep 20

    # Query the spikes API after recovery
    # Using recovery cursor to ensure we only get captures created after recovery
    query_spikes_api "$LAB_DIR/spikes-after-recovery.json" "lab-tovarisch" "true"
    extract_latest_capture "$LAB_DIR/spikes-after-recovery.json" "$CAPTURE_AFTER_RECOVERY_FILE" "after-recovery"
    REQUESTED_PATH_AFTER_RECOVERY="/status.json?include=network_diag"

    # Check if recovery capture was successful
    local recovery_status
    recovery_status=$(jq -r '.status' "$CAPTURE_AFTER_RECOVERY_FILE" 2>/dev/null || echo "unknown")

    if [[ "$recovery_status" == "ok" ]]; then
        RECOVERY_CAPTURE_OK=true
        log_info "[PASS] Recovery capture successful (status: $recovery_status)"
    else
        RECOVERY_CAPTURE_OK=false
        log_error "[FAIL] Recovery capture failed: $recovery_status"
    fi

    # ========================================
    # Write final result
    # ========================================
    write_result

    # Print summary
    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact directory: $LAB_DIR"
    log_info ""
    log_info "Summary:"
    log_info "  Baseline capture: $([ "$BASELINE_CAPTURE_OK" = true ] && echo "OK" || echo "FAIL")"
    log_info "  Defect observed: $([ "$DEFECT_OBSERVED" = true ] && echo "YES" || echo "NO")"
    log_info "  Recovery capture: $([ "$RECOVERY_CAPTURE_OK" = true ] && echo "OK" || echo "FAIL")"
    log_info ""

    # Determine exit code
    local exit_code=0
    if [[ "$BASELINE_CAPTURE_OK" != "true" ]]; then
        log_error "Baseline capture failed"
        exit_code=1
    fi
    if [[ "$DEFECT_OBSERVED" != "true" ]]; then
        log_error "Defect was not observed - lab purpose not proven"
        exit_code=1
    fi
    if [[ "$RECOVERY_CAPTURE_OK" != "true" ]]; then
        log_error "Recovery capture failed"
        exit_code=1
    fi

    if [[ $exit_code -eq 0 ]]; then
        log_info "Result: PASS"
    else
        log_error "Result: FAIL"
    fi

    return $exit_code
}

# Run lab when executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_lab "$@"
fi
