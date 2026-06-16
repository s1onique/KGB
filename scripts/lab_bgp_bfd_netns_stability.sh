#!/bin/bash
# lab_bgp_bfd_netns_stability.sh — Stability checkpoint collection for netns lab
#
# ACT: Add BGP session stability/reconnect-budget assertions to netns lab.
# Artifacts: status-before.json, status-first-established.json, status-after-stability.json
# plus bird-protocol-*.txt at each checkpoint.
#
# Assertions:
# - reconnect_count delta <= budget during initial convergence
# - reconnect_count does NOT increase during stability window
# - BIRD does NOT return to Idle/Active/Connect
#
# Sourced by lab_bgp_bfd_netns_lib.sh.

STABILITY_WINDOW_SECONDS="${STABILITY_WINDOW_SECONDS:-15}"
RECONNECT_BUDGET="${RECONNECT_BUDGET:-1}"

collect_status_snapshot() {
    local output="$1"
    if command -v curl &> /dev/null; then
        ip netns exec "$NS_TOVARISCH" curl -s -f "http://127.0.0.1:8317/status.json" > "$output" 2>&1 && return 0
    fi
    echo "{}" > "$output"
    return 1
}

collect_bird_protocol_snapshot() {
    local output="$1"
    local protocol="${2:-}"
    if [[ -n "$protocol" ]]; then
        birdc_lab "show protocol all $protocol" 2>/dev/null > "$output" && return 0
    else
        birdc_lab show protocols all 2>/dev/null > "$output" && return 0
    fi
    echo "BIRD_QUERY_FAILED" > "$output"
    return 1
}

extract_reconnect_count() {
    local status_file="$1"
    [[ -f "$status_file" ]] && [[ -s "$status_file" ]] || { echo ""; return 1; }
    local rc
    rc=$(jq -r '.bgp.reconnect_count // .checks[] | select(.name == "bgp") | .reconnect_count // empty' "$status_file" 2>/dev/null || echo "")
    [[ -z "$rc" ]] || [[ "$rc" == "null" ]] && rc=$(jq -r '.. | objects | select(has("reconnect_count")) | .reconnect_count // empty' "$status_file" 2>/dev/null || echo "")
    echo "${rc:-}"
}

is_bird_state_stable() {
    local protocol_file="$1"
    [[ -f "$protocol_file" ]] && [[ -s "$protocol_file" ]] && grep -qE "Established" "$protocol_file" 2>/dev/null
}

bird_has_recent_closures() {
    local protocol_file="$1"
    [[ -f "$protocol_file" ]] && grep -qiE "Connection closed|Socket:.*closed" "$protocol_file" 2>/dev/null
}

extract_bird_imported_route_count() {
    local routes_file="$1"
    [[ -f "$routes_file" ]] && [[ -s "$routes_file" ]] || { echo "-1"; return 1; }
    grep -cE "\S" "$routes_file" 2>/dev/null || echo "0"
}

assert_bgp_stability() {
    log_info "=== ACT 3: BGP Stability Assertion ==="
    log_info "Stability window: ${STABILITY_WINDOW_SECONDS}s, Reconnect budget: <= ${RECONNECT_BUDGET}"
    local exit_code=0
    local first_established=false

    # Phase 1: Before convergence
    log_info "=== Phase 1: Before Convergence ==="
    collect_status_snapshot "$STATUS_BEFORE_OUTPUT"
    collect_bird_protocol_snapshot "$BIRD_PROTOCOL_BEFORE_OUTPUT" "tovarisch"
    local reconnect_before
    reconnect_before=$(extract_reconnect_count "$STATUS_BEFORE_OUTPUT")
    log_info "Reconnect count (before): ${reconnect_before:-not available}"

    # Phase 2: Wait for BFD Up then BGP Established
    log_info ""
    log_info "=== Phase 2: Wait for First Established ==="
    log_info "Waiting for BFD Up (${WAIT_BFD_CONVERGE}s)..."
    local bfd_elapsed=0
    while [[ $bfd_elapsed -lt $WAIT_BFD_CONVERGE ]]; do
        birdc_lab show bfd sessions 2>/dev/null | grep -qE '(^|[[:space:]])Up([[:space:]]|$)' && break
        sleep 2
        bfd_elapsed=$((bfd_elapsed + 2))
        echo -n "."
    done
    echo ""
    [[ $bfd_elapsed -ge $WAIT_BFD_CONVERGE ]] && { log_error "[FAIL] BFD did not reach Up"; exit_code=1; }

    log_info "Waiting for BGP Established (${WAIT_BGP_CONVERGE}s)..."
    local bgp_elapsed=0
    while [[ $bgp_elapsed -lt $WAIT_BGP_CONVERGE ]]; do
        birdc_lab show protocols tovarisch 2>/dev/null | grep -qE "Established" && {
            first_established=true
            collect_status_snapshot "$STATUS_FIRST_ESTABLISHED_OUTPUT"
            collect_bird_protocol_snapshot "$BIRD_PROTOCOL_FIRST_ESTABLISHED_OUTPUT" "tovarisch"
            break
        }
        sleep 2
        bgp_elapsed=$((bgp_elapsed + 2))
        echo -n "."
    done
    echo ""
    $first_established || { log_error "[FAIL] BGP did not reach Established"; exit_code=1; }

    local reconnect_first
    reconnect_first=$(extract_reconnect_count "$STATUS_FIRST_ESTABLISHED_OUTPUT")
    log_info "Reconnect count (first Established): ${reconnect_first:-not available}"

    # Phase 3: Stability window monitoring
    log_info ""
    log_info "=== Phase 3: Stability Window (${STABILITY_WINDOW_SECONDS}s) ==="
    local stability_elapsed=0
    local reconnect_increased=false
    local bird_became_unstable=false

    while [[ $stability_elapsed -lt $STABILITY_WINDOW_SECONDS ]]; do
        sleep 2
        stability_elapsed=$((stability_elapsed + 2))
        echo -n "."

        local current_status="$LAB_DIR/status-checkpoint-temp.json"
        collect_status_snapshot "$current_status"
        local reconnect_current
        reconnect_current=$(extract_reconnect_count "$current_status")

        if [[ -n "$reconnect_first" ]] && [[ -n "$reconnect_current" ]] &&
           [[ "$reconnect_current" =~ ^[0-9]+$ ]] && [[ "$reconnect_first" =~ ^[0-9]+$ ]]; then
            [[ "$reconnect_current" -gt "$reconnect_first" ]] && {
                log_error "[FAIL] Reconnect increased: ${reconnect_first} -> ${reconnect_current}"
                reconnect_increased=true
            }
        fi

        local current_protocol="$LAB_DIR/bird-protocol-checkpoint-temp.txt"
        collect_bird_protocol_snapshot "$current_protocol" "tovarisch"
        is_bird_state_stable "$current_protocol" || {
            log_error "[FAIL] BIRD became unstable during stability window"
            bird_became_unstable=true
        }
    done
    echo ""

    collect_status_snapshot "$STATUS_AFTER_STABILITY_OUTPUT"
    collect_bird_protocol_snapshot "$BIRD_PROTOCOL_AFTER_STABILITY_OUTPUT" "tovarisch"
    rm -f "$LAB_DIR/status-checkpoint-temp.json" "$LAB_DIR/bird-protocol-checkpoint-temp.txt" 2>/dev/null || true

    # Phase 4: Assertions
    log_info ""
    log_info "=== Phase 4: Stability Assertions ==="

    $first_established && log_info "[PASS] BGP reached Established" || log_error "[FAIL] BGP not Established"
    [[ $bfd_elapsed -lt $WAIT_BFD_CONVERGE ]] && log_info "[PASS] BFD reached Up" || log_error "[FAIL] BFD not Up"

    if [[ -f "$BIRD_PROTOCOL_AFTER_STABILITY_OUTPUT" ]] && grep -qE "Established" "$BIRD_PROTOCOL_AFTER_STABILITY_OUTPUT" 2>/dev/null; then
        log_info "[PASS] BIRD shows Established at stability end"
    else
        log_error "[FAIL] BIRD does not show Established"
        exit_code=1
    fi

    if [[ -n "$reconnect_first" ]] && [[ "$reconnect_first" =~ ^[0-9]+$ ]]; then
        local delta=0
        [[ -n "$reconnect_before" ]] && [[ "$reconnect_before" =~ ^[0-9]+$ ]] && delta=$((reconnect_first - reconnect_before))
        if [[ $delta -le $RECONNECT_BUDGET ]]; then
            log_info "[PASS] Reconnect delta ${delta} <= budget ${RECONNECT_BUDGET}"
        else
            log_error "[FAIL] Reconnect delta ${delta} > budget ${RECONNECT_BUDGET}"
            exit_code=1
        fi
    fi

    $reconnect_increased && { log_error "[FAIL] Reconnect count increased during stability"; exit_code=1; } || log_info "[PASS] Reconnect count stable"
    $bird_became_unstable && { log_error "[FAIL] BIRD became unstable"; exit_code=1; } || log_info "[PASS] BIRD remained stable"

    if bird_has_recent_closures "$BIRD_PROTOCOL_AFTER_STABILITY_OUTPUT"; then
        log_error "[FAIL] BIRD shows recent 'Socket: Connection closed'"
        exit_code=1
    else
        log_info "[PASS] No recent BIRD connection closures"
    fi

    log_info ""
    [[ $exit_code -eq 0 ]] && log_info "=== ACT 3: STABILITY ASSERTION PASSED ===" || log_error "=== ACT 3: STABILITY ASSERTION FAILED ==="
    return $exit_code
}

collect_stability_routes() {
    log_info "Collecting BIRD routes for stability verification..."
    if birdc_lab show route protocol tovarisch all 2>/dev/null > "$BIRD_ROUTES_OUTPUT"; then
        local route_count
        route_count=$(extract_bird_imported_route_count "$BIRD_ROUTES_OUTPUT")
        [[ "$route_count" -gt 0 ]] && log_info "[PASS] BIRD imported routes: ${route_count} > 0" || log_error "[FAIL] BIRD imported routes: ${route_count}"
        [[ "$route_count" -gt 0 ]] && return 0 || return 1
    fi
    log_error "[FAIL] Failed to collect BIRD routes"
    return 1
}
