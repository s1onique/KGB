#!/bin/bash
# lab_bgp_bfd_netns_collect.sh — Status collection for netns lab
#
# Collection functions for the BGP/BFD netns lab.
# Sourced by lab_bgp_bfd_netns_lib.sh.

# Collect CLI status
collect_status() {
    log_info "Collecting tovarisch CLI status..."

    local binary="${TOVARISCH_BINARY:-./tovarisch/zig-out/bin/tovarisch}"

    if ip netns exec "$NS_TOVARISCH" "$binary" status --json > "$STATUS_OUTPUT" 2>&1; then
        log_info "CLI status collected:"
        cat "$STATUS_OUTPUT"
        return 0
    else
        log_error "Failed to collect CLI status"
        cat "$STATUS_OUTPUT" 2>/dev/null || true
        return 1
    fi
}

# Collect runtime HTTP status
collect_status_http() {
    log_info "Collecting tovarisch runtime HTTP status from serve process..."

    # tovarisch serve binds to 127.0.0.1:8317 by default
    # Query from inside the namespace using curl.
    # Returns 0 on success, 1 on failure. Caller decides whether to fail or warn.
    if command -v curl &> /dev/null; then
        if ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" > "$STATUS_HTTP_OUTPUT" 2>&1; then
            log_info "Runtime HTTP status collected:"
            cat "$STATUS_HTTP_OUTPUT"
            return 0
        else
            log_error "Failed to collect runtime HTTP status"
            cat "$STATUS_HTTP_OUTPUT" 2>/dev/null || true
            return 1
        fi
    else
        log_warn "curl not available - cannot query HTTP status endpoint"
        echo "{}" > "$STATUS_HTTP_OUTPUT"
        return 1
    fi
}

# Collect BGP routes
collect_bgp_routes() {
    log_info "Collecting BGP routes from BIRD..."

    # BIRD uses "show route" not "show routes"
    if birdc_lab show route 2>/dev/null > "$BGP_ROUTES_OUTPUT"; then
        log_info "Routes collected:"
        cat "$BGP_ROUTES_OUTPUT"
        return 0
    else
        log_warn "Failed to collect BGP routes"
        return 1
    fi
}
