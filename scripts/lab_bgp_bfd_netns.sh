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

# Check ACT 3 dependencies (fatal for stability assertions)
check_stability_dependencies() {
    local missing=()

    for cmd in curl jq; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "ACT 3 stability assertions require: ${missing[*]}"
        return 1
    fi
    return 0
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
    if ! collect_status_http; then
        log_warn "[FAIL-CANDIDATE] Runtime HTTP status collection failed - will assert below"
    fi

    # Assertions (v1 - core startup + v1.5 runtime config evidence)
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

    # 4. Prove CLI status --json is collectable (v1 - not runtime evidence)
    if [[ -s "$STATUS_OUTPUT" ]] && jq . "$STATUS_OUTPUT" &> /dev/null; then
        log_info "[PASS] CLI status JSON is valid"
    else
        log_error "[FAIL] CLI status JSON not valid"
        exit_code=1
    fi

    # 5. v1.5: Prove runtime HTTP status endpoint responds
    log_info ""
    log_info "=== v1.5 Runtime Config Evidence ==="

    if [[ -s "$STATUS_HTTP_OUTPUT" ]] && jq . "$STATUS_HTTP_OUTPUT" &> /dev/null; then
        log_info "[PASS] Runtime HTTP status JSON is valid"
    else
        log_error "[FAIL] Runtime HTTP status JSON not valid"
        exit_code=1
    fi

    # 6. v1.5: Prove runtime status shows BFD/BGP config was loaded
    # The runtime /status.json uses ServeContext with BFD runtime from config,
    # so it should NOT show "bfd not configured" when --config was provided.
    # It's acceptable for BFD/BGP sessions to not be Up yet, but the config
    # must be acknowledged as loaded (shown by absence of "not configured").
    if [[ -s "$STATUS_HTTP_OUTPUT" ]]; then
        local bfd_detail
        bfd_detail=$(jq -r '.checks[] | select(.name == "bfd") | .detail' "$STATUS_HTTP_OUTPUT" 2>/dev/null || echo "PARSE_ERROR")
        local bgp_detail
        bgp_detail=$(jq -r '.checks[] | select(.name == "bgp") | .detail' "$STATUS_HTTP_OUTPUT" 2>/dev/null || echo "PARSE_ERROR")

        # Runtime BFD check should NOT be "bfd not configured" when config was loaded
        if [[ "$bfd_detail" == "bfd not configured" ]]; then
            log_error "[FAIL] Runtime BFD shows 'not configured' - config may not have been loaded"
            exit_code=1
        else
            log_info "[PASS] Runtime BFD config acknowledged: $bfd_detail"
        fi

        # Runtime BGP check should NOT be "BGP not configured" when config was loaded
        if [[ "$bgp_detail" == "BGP not configured" ]]; then
            log_error "[FAIL] Runtime BGP shows 'not configured' - config may not have been loaded"
            exit_code=1
        else
            log_info "[PASS] Runtime BGP config acknowledged: $bgp_detail"
        fi
    else
        log_error "[FAIL] Cannot verify runtime config - HTTP status not available"
        exit_code=1
    fi

    # 7. ACT 2: Assert BFD session reaches Up (required for ACT 2 completion)
    log_info ""
    log_info "=== ACT 2: BFD Session Up Assertion ==="

    if ! assert_bfd_up; then
        log_error "[FAIL] BFD session did not reach Up"
        exit_code=1
    fi

    # 8. ACT 3: Assert BGP stability with reconnect budget
    log_info ""
    log_info "=== ACT 3: BGP Stability Assertions ==="
    log_info "Scope: Clean-start stability (no failure injection)"
    log_info "Assertions:"
    log_info "  - BGP reaches Established"
    log_info "  - BFD reaches Up (prerequisite)"
    log_info "  - BIRD reaches Established"
    log_info "  - BIRD imported route count > 0"
    log_info "  - tovarisch reconnect_count delta <= budget (<= ${RECONNECT_BUDGET:-1})"
    log_info "  - reconnect_count does not increase during stability window"
    log_info "  - BIRD does not return to Idle/Active/Connect"
    log_info "  - No 'Socket: Connection closed' after stable point"

    # Check ACT 3 dependencies (fatal for stability assertions)
    if ! check_stability_dependencies; then
        log_error "[FAIL] ACT 3 stability assertions require curl and jq"
        exit_code=1
    else
        if ! assert_bgp_stability; then
            log_error "[FAIL] BGP stability assertions failed"
            exit_code=1
        fi

        # Collect routes for route import proof (fatal)
        log_info ""
        log_info "=== Route Import Proof ==="
        if ! collect_stability_routes; then
            log_error "[FAIL] Route import proof failed"
            exit_code=1
        fi
    fi

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
