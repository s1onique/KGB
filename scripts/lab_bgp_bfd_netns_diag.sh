#!/bin/bash
# lab_bgp_bfd_netns_diag.sh — Diagnostic helpers for netns lab
#
# Diagnostic functions for the BGP/BFD netns lab harness.
# This is sourced by lab_bgp_bfd_netns_lib.sh.

print_diagnostics() {
    log_info "=== Lab Diagnostics ==="

    echo "--- Temp directory ---"
    echo "$LAB_DIR"

    echo "--- Generated configs ---"
    echo "BIRD config: $BIRD_CONFIG"
    echo "tovarisch config: $TOVARISCH_CONFIG"

    echo "--- Logs ---"
    echo "BIRD log: $BIRD_LOG"
    echo "tovarisch log: $TOVARISCH_LOG"
    echo "Status output: $STATUS_OUTPUT"
    echo "BGP routes: $BGP_ROUTES_OUTPUT"

    echo "--- BIRD control socket ---"
    echo "Socket: $BIRD_SOCKET"
    ls -la "$BIRD_SOCKET" 2>/dev/null || echo "Socket not ready"

    echo "--- Namespace list ---"
    ip netns list

    echo "--- tovarisch namespace interfaces ---"
    ip netns exec "$NS_TOVARISCH" ip addr show 2>/dev/null || echo "Not accessible"

    echo "--- BIRD namespace interfaces ---"
    ip netns exec "$NS_BIRD" ip addr show 2>/dev/null || echo "Not accessible"

    echo "--- BIRD protocols status (lab instance) ---"
    birdc_lab show protocols 2>/dev/null || echo "BIRD not accessible"

    echo "--- BIRD BFD sessions (lab instance) ---"
    birdc_lab show bfd sessions 2>/dev/null || echo "BFD not accessible"

    echo "--- BIRD BGP status (lab instance) ---"
    birdc_lab show protocols bgp 2>/dev/null || echo "BGP not accessible"

    echo "--- tovarisch log (last 20 lines) ---"
    tail -n 20 "$TOVARISCH_LOG" 2>/dev/null || echo "Log not available"

    echo "--- ACT 2 BFD Artifacts ---"
    echo "BFD sessions: ${BFD_SESSIONS_OUTPUT:-not set}"
    cat "$BFD_SESSIONS_OUTPUT" 2>/dev/null || echo "Not available"

    echo "--- BFD HTTP Status (ACT 2) ---"
    local http_bfd_file="${LAB_DIR:-}/status-http-bfd.json"
    if [[ -s "$http_bfd_file" ]]; then
        echo "File: $http_bfd_file"
        jq '.' "$http_bfd_file" 2>/dev/null || cat "$http_bfd_file"
    else
        echo "Not available"
    fi

    echo "--- tcpdump BFD capture (BIRD namespace) ---"
    if [[ -s "${TCPDUMP_BFD_BIRD:-}" ]]; then
        cat "$TCPDUMP_BFD_BIRD"
    else
        echo "Not available or empty"
    fi

    echo "--- tcpdump BFD capture (tovarisch namespace) ---"
    if [[ -s "${TCPDUMP_BFD_TOVARISCH:-}" ]]; then
        cat "$TCPDUMP_BFD_TOVARISCH"
    else
        echo "Not available or empty"
    fi

    echo "--- All artifacts in lab directory ---"
    if [[ -d "${LAB_DIR:-}" ]]; then
        ls -la "$LAB_DIR" 2>/dev/null || echo "Cannot list"
    else
        echo "Lab directory not available"
    fi
}
