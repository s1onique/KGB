#!/bin/bash
# lab_uvb76_capture_netns_defect.sh — Network defect injection for UVB-76 capture netns lab
#
# Network impairment (tc netem) functions.
# IMPORTANT: tc commands must run inside the namespace that owns the interface.
# Sourced by lab_uvb76_capture_netns_lib.sh.

inject_netem_defect() {
    log_info "Injecting 100% packet loss defect..."
    # Use 100% loss for deterministic defect - capture MUST fail
    echo "Injecting 100% loss on $VETH_TOVARISCH in namespace $NS_TOVARISCH" > "$DEFECT_BEFORE_FILE"

    # Apply netem loss INSIDE the namespace that owns the interface
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

    # Remove the netem qdisc inside the namespace
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
