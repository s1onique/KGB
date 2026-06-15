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
}
