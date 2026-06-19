#!/bin/bash
# lab_uvb76_capture_netns_defect.sh — Network defect injection for UVB-76 capture netns lab
#
# Network impairment functions for probe failure testing.
# IMPORTANT: tc commands must run inside the namespace that owns the interface.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Defect mode constants
# Note: DEFECT_MODE_LAB_PROBE is the PREFERRED mode for success/cooldown contract.
# It breaks only the probe signal (/lab/probe returns 503) while keeping /status healthy.
# The old interface-wide netem modes are kept for failure-mode tests only.
DEFECT_MODE_LAB_PROBE="lab-probe"             # Probe-only failure via file toggle (PREFERRED)
DEFECT_MODE_CLEAR_BEFORE_FETCH="clear-before-fetch"  # Block probe path, clear before diagnostic fetch (DEPRECATED)
DEFECT_MODE_100PCT_LOSS="100pct-loss"         # Break ALL traffic (for failure-mode tests)

# Current defect mode
declare -g CURRENT_DEFECT_MODE="${DEFECT_MODE_LAB_PROBE}"

# Path to the lab probe failure file (set in lib after LAB_DIR is created)
declare -g LAB_PROBE_FAILURE_FILE=""

inject_netem_defect() {
    local mode="${1:-${DEFECT_MODE_LAB_PROBE}}"
    CURRENT_DEFECT_MODE="$mode"
    
    case "$mode" in
        "$DEFECT_MODE_LAB_PROBE")
            inject_lab_probe_defect
            ;;
        "$DEFECT_MODE_CLEAR_BEFORE_FETCH")
            inject_clear_before_fetch_defect
            ;;
        "$DEFECT_MODE_100PCT_LOSS"|*)
            inject_100pct_loss_defect
            ;;
    esac
}

# Lab probe defect: Break ONLY the probe signal while keeping /status healthy.
# This is the PREFERRED method for the captured → skipped_cooldown → captured contract.
#
# How it works:
# - tovarisch is configured with lab_mode=true and lab_probe_failure_file=<path>
# - /lab/probe returns 503 when the failure file exists, 200 when it doesn't
# - /status remains healthy at all times
# - UVB-76 probes /lab/probe (not /status), so probe failures are recorded
# - Diagnostic capture fetches /status, which always succeeds
inject_lab_probe_defect() {
    log_info "Injecting lab probe defect (probe fails, /status remains healthy)..."
    
    # Get the failure file path from lab config
    local failure_file="${LAB_DIR}/${ARTIFACT_LAB_PROBE_FAILURE_FILE}"
    LAB_PROBE_FAILURE_FILE="$failure_file"
    
    # Create the failure file to make /lab/probe return 503
    touch "$failure_file"
    
    echo "Lab probe failure file created: $failure_file" > "$DEFECT_BEFORE_FILE"
    log_info "Lab probe defect injected: $failure_file exists"
    log_info "/lab/probe will now return 503, /status remains healthy"
}

# Clear the lab probe defect (remove failure file)
clear_lab_probe_defect() {
    local failure_file="${LAB_DIR}/${ARTIFACT_LAB_PROBE_FAILURE_FILE}"
    
    if [[ -f "$failure_file" ]]; then
        rm -f "$failure_file"
        log_info "Lab probe defect cleared: $failure_file removed"
        echo "Lab probe failure file removed at $(date -Iseconds)" > "$DEFECT_AFTER_CLEAR_FILE"
    fi
}

# Clear-before-fetch defect: Break probe path while diagnostic fetch happens after clearing.
# DEPRECATED: Use DEFECT_MODE_LAB_PROBE instead for deterministic behavior.
# This is achieved by applying netem on the UVB-76 side egress.
# IMPORTANT: This blocks ALL outgoing packets from that interface, not just probe traffic.
inject_clear_before_fetch_defect() {
    log_info "Injecting clear-before-fetch defect (DEPRECATED - use lab-probe mode)..."
    echo "Injecting clear-before-fetch defect on $VETH_UVB76 in namespace $NS_UVB76" > "$DEFECT_BEFORE_FILE"

    # Apply netem loss on the UVB-76 side egress (outgoing from uvb76 to tovarisch)
    ip netns exec "$NS_UVB76" tc qdisc add dev "$VETH_UVB76" root netem loss 100% 2>/dev/null || \
        ip netns exec "$NS_UVB76" tc qdisc change dev "$VETH_UVB76" root netem loss 100%

    ip netns exec "$NS_UVB76" tc qdisc show dev "$VETH_UVB76" > "$DEFECT_TC_QDISC_FILE"
    log_info "Clear-before-fetch defect injected (DEPRECATED mode)"
}

# 100% loss defect: Break ALL traffic to tovarisch.
# Use this for failure-mode tests where we expect capture to timeout.
inject_100pct_loss_defect() {
    log_info "Injecting 100% packet loss defect (failure-mode test only)..."
    echo "Injecting 100% loss on $VETH_TOVARISCH in namespace $NS_TOVARISCH" > "$DEFECT_BEFORE_FILE"

    ip netns exec "$NS_TOVARISCH" tc qdisc add dev "$VETH_TOVARISCH" root netem loss 100% 2>/dev/null || \
        ip netns exec "$NS_TOVARISCH" tc qdisc change dev "$VETH_TOVARISCH" root netem loss 100%

    ip netns exec "$NS_TOVARISCH" tc qdisc show dev "$VETH_TOVARISCH" > "$DEFECT_TC_QDISC_FILE"
    log_info "100% loss defect injected (failure-mode test only)"
}

inject_drop_defect() {
    log_info "Injecting drop network defect..."
    echo "Injecting DROP on all traffic" > "$DEFECT_BEFORE_FILE"

    # Add drop qdisc - drop all packets
    ip netns exec "$NS_TOVARISCH" tc qdisc add dev "$VETH_TOVARISCH" root netem loss 100% 2>/dev/null || \
        ip netns exec "$NS_TOVARISCH" tc qdisc change dev "$VETH_TOVARISCH" root netem loss 100%

    ip netns exec "$NS_TOVARISCH" tc qdisc show dev "$VETH_TOVARISCH" > "$DEFECT_TC_QDISC_FILE"

    log_info "Drop defect injected"
}

clear_defect() {
    log_info "Clearing network defect..."

    # Clear lab probe defect if active
    if [[ "$CURRENT_DEFECT_MODE" == "$DEFECT_MODE_LAB_PROBE" ]]; then
        clear_lab_probe_defect
    fi

    # Clear defect from UVB-76 side if present
    ip netns exec "$NS_UVB76" tc qdisc del dev "$VETH_UVB76" root 2>/dev/null || true
    
    # Clear defect from tovarisch side if present
    ip netns exec "$NS_TOVARISCH" tc qdisc del dev "$VETH_TOVARISCH" root 2>/dev/null || true

    echo "Defect cleared at $(date -Iseconds)" > "$DEFECT_AFTER_CLEAR_FILE"

    # Verify connectivity restored
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > /dev/null 2>&1; then
        log_info "Connectivity restored after defect clear"
    else
        log_warn "Connectivity may not be fully restored"
    fi

    log_info "Network defect cleared"
}
