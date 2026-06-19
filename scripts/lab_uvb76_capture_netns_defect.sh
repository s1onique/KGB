#!/bin/bash
# lab_uvb76_capture_netns_defect.sh — Network defect injection for UVB-76 capture netns lab
#
# Network impairment (tc netem) functions.
# IMPORTANT: tc commands must run inside the namespace that owns the interface.
# Sourced by lab_uvb76_capture_netns_lib.sh.

# Defect mode constants
# Note: These apply interface-wide loss, NOT just probe-specific traffic.
# Diagnostic fetch is preserved ONLY because the helper clears the defect before fetching.
DEFECT_MODE_CLEAR_BEFORE_FETCH="clear-before-fetch"  # Block probe path, clear before diagnostic fetch
DEFECT_MODE_100PCT_LOSS="100pct-loss"               # Break ALL traffic (for failure-mode tests)

# Current defect mode
declare -g CURRENT_DEFECT_MODE="${DEFECT_MODE_CLEAR_BEFORE_FETCH}"

inject_netem_defect() {
    local mode="${1:-${DEFECT_MODE_CLEAR_BEFORE_FETCH}}"
    CURRENT_DEFECT_MODE="$mode"
    
    case "$mode" in
        "$DEFECT_MODE_CLEAR_BEFORE_FETCH")
            inject_clear_before_fetch_defect
            ;;
        "$DEFECT_MODE_100PCT_LOSS"|*)
            inject_100pct_loss_defect
            ;;
    esac
}

# Clear-before-fetch defect: Break probe path while diagnostic fetch happens after clearing.
# This is achieved by applying netem on the UVB-76 side egress.
# IMPORTANT: This blocks ALL outgoing packets from that interface, not just probe traffic.
# The diagnostic fetch is only preserved because the helper clears the defect before fetching.
inject_clear_before_fetch_defect() {
    log_info "Injecting clear-before-fetch defect (breaks probe, clear before diagnostic fetch)..."
    echo "Injecting clear-before-fetch defect on $VETH_UVB76 in namespace $NS_UVB76" > "$DEFECT_BEFORE_FILE"

    # Apply netem loss on the UVB-76 side egress (outgoing from uvb76 to tovarisch)
    # This blocks ALL traffic from uvb76 to tovarisch while active
    ip netns exec "$NS_UVB76" tc qdisc add dev "$VETH_UVB76" root netem loss 100% 2>/dev/null || \
        ip netns exec "$NS_UVB76" tc qdisc change dev "$VETH_UVB76" root netem loss 100%

    # Show the qdisc config
    ip netns exec "$NS_UVB76" tc qdisc show dev "$VETH_UVB76" > "$DEFECT_TC_QDISC_FILE"

    log_info "Clear-before-fetch defect injected (100% loss on $VETH_UVB76 egress)"
    log_info "NOTE: Diagnostic fetch is only preserved because helper clears defect before fetching"
}

# 100% loss defect: Break ALL traffic to tovarisch.
# Use this for failure-mode tests where we expect capture to timeout.
inject_100pct_loss_defect() {
    log_info "Injecting 100% packet loss defect..."
    echo "Injecting 100% loss on $VETH_TOVARISCH in namespace $NS_TOVARISCH" > "$DEFECT_BEFORE_FILE"

    # Apply netem loss INSIDE the namespace that owns the interface (tovarisch side)
    # This breaks ALL traffic to tovarisch, including diagnostic fetch
    ip netns exec "$NS_TOVARISCH" tc qdisc add dev "$VETH_TOVARISCH" root netem loss 100% 2>/dev/null || \
        ip netns exec "$NS_TOVARISCH" tc qdisc change dev "$VETH_TOVARISCH" root netem loss 100%

    # Show the qdisc config
    ip netns exec "$NS_TOVARISCH" tc qdisc show dev "$VETH_TOVARISCH" > "$DEFECT_TC_QDISC_FILE"

    log_info "100% loss defect injected"
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
