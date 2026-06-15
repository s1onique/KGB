#!/bin/bash
# lab_bgp_bfd_netns.sh — BGP/BFD netns lab harness for tovarisch
#
# Creates isolated Linux network namespaces with BIRD and tovarisch
# for manual CI verification of BFD/BGP behavior.
#
# Primary execution: GitHub Actions (ubuntu-latest)
# Local execution: optional debugging only
#
# Trigger: workflow_dispatch (manual only)
# This is NOT part of make gate.

set -euo pipefail

# Source shared library
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_bgp_bfd_netns_lib.sh
source "${SCRIPT_DIR}/lab_bgp_bfd_netns_lib.sh"

# Check if running on Linux
check_linux() {
    if [[ "$(uname)" != "Linux" ]]; then
        log_error "This lab requires Linux (network namespaces)."
        log_error "On macOS, run manually in GitHub Actions."
        exit 1
    fi
}

# Check required tools
check_dependencies() {
    local missing=()

    for cmd in ip bird birdc; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error "Install: iproute2 bird2"
        exit 1
    fi

    detect_bird_version
}

# Main lab execution
run_lab() {
    log_info "=== BGP/BFD Netns Lab ==="
    log_info "Primary execution: GitHub Actions (workflow_dispatch)"
    log_info "Local execution: optional for debugging only"
    log_info ""

    # Pre-flight checks
    check_linux
    check_dependencies

    # Setup
    setup_temp_dir
    setup_trap

    # Create topology
    create_namespaces
    configure_interfaces

    # Generate configs
    generate_bird_config
    generate_tovarisch_config
    generate_prefix_file

    # Verify topology came up
    if ! verify_topology; then
        log_error "Topology verification failed"
        print_diagnostics
        exit 1
    fi

    # Start services
    start_bird
    start_tovarisch

    # Collect status (always do this)
    collect_status

    # Assertions (v1 - core startup)
    local exit_code=0

    # 1. Prove namespace topology
    if verify_topology; then
        log_info "[PASS] Namespace topology verified"
    else
        log_error "[FAIL] Namespace topology failed"
        exit_code=1
    fi

    # 2. Prove BIRD started
    if ip netns exec "$NS_BIRD" pgrep -x bird &> /dev/null; then
        log_info "[PASS] BIRD started"
    else
        log_error "[FAIL] BIRD not running"
        exit_code=1
    fi

    # 3. Prove tovarisch started
    if ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch &> /dev/null; then
        log_info "[PASS] tovarisch started"
    else
        log_error "[FAIL] tovarisch not running"
        exit_code=1
    fi

    # 4. Prove status --json is collectable
    if [[ -s "$STATUS_OUTPUT" ]] && jq . "$STATUS_OUTPUT" &> /dev/null; then
        log_info "[PASS] Status JSON is valid"
    else
        log_error "[FAIL] Status JSON not valid"
        exit_code=1
    fi

    # 5. Attempt BFD/BGP convergence (target v1 - bounded wait)
    log_info ""
    log_info "=== Convergence Assertions ==="

    wait_bfd_convergence || log_warn "[DEFERRED] BFD convergence not achieved in v1"
    wait_bgp_convergence || log_warn "[DEFERRED] BGP convergence not achieved in v1"

    # Collect routes (non-fatal in v1 since BGP convergence is deferred)
    collect_bgp_routes || log_warn "[DEFERRED] BGP route collection unavailable in v1"

    # Print diagnostics
    print_diagnostics

    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Temp dir: $LAB_DIR"
    log_info "Artifacts: $LAB_DIR/*"

    if [[ $exit_code -eq 0 ]]; then
        log_info "Result: PASS (v1 assertions met)"
    else
        log_error "Result: FAIL (some assertions failed)"
    fi

    return $exit_code
}

# Run lab when executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    run_lab "$@"
fi
