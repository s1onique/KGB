#!/bin/bash
# lab_uvb76_capture_netns_tovarisch.sh — Tovarisch-specific helpers for UVB-76 capture netns lab
#
# Functions for tovarisch lifecycle and diagnostics.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Collect tovarisch listen sockets as a diagnostic artifact.
# This helps diagnose binding issues (e.g., 127.0.0.1 vs 0.0.0.0).
collect_tovarisch_listen_sockets() {
    log_info "Collecting tovarisch listen sockets..."
    if ! ip netns exec "$NS_TOVARISCH" ss -ltnp > "$TOVARISCH_LISTEN_SOCKETS_FILE" 2>&1; then
        log_warn "Failed to collect ss output (ss may not be available)"
        echo "# ss failed" > "$TOVARISCH_LISTEN_SOCKETS_FILE"
    fi
    # Also log to stdout for immediate visibility
    log_info "tovarisch listening sockets:"
    cat "$TOVARISCH_LISTEN_SOCKETS_FILE" | head -20
}
