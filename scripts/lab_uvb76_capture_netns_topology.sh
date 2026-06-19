#!/bin/bash
# lab_uvb76_capture_netns_topology.sh — Topology functions for UVB-76 capture netns lab
#
# Network namespace and topology functions.
# Sourced by lab_uvb76_capture_netns_lib.sh.

create_namespaces() {
    log_info "Creating network namespaces..."
    ip netns add "$NS_UVB76" 2>/dev/null || log_warn "Namespace $NS_UVB76 may already exist"
    ip netns add "$NS_TOVARISCH" 2>/dev/null || log_warn "Namespace $NS_TOVARISCH may already exist"

    # Create veth pair
    ip link add "$VETH_UVB76" type veth peer name "$VETH_TOVARISCH"

    # Move each end to its namespace
    ip link set "$VETH_UVB76" netns "$NS_UVB76"
    ip link set "$VETH_TOVARISCH" netns "$NS_TOVARISCH"

    log_info "Network namespaces and veth pairs created"
}

configure_interfaces() {
    log_info "Configuring network interfaces inside namespaces..."

    # Configure uvb76 namespace
    ip netns exec "$NS_UVB76" ip addr add "${IP_UVB76}/${NETMASK}" dev "$VETH_UVB76"
    ip netns exec "$NS_UVB76" ip link set "$VETH_UVB76" up
    ip netns exec "$NS_UVB76" ip link set lo up

    # Configure tovarisch namespace
    ip netns exec "$NS_TOVARISCH" ip addr add "${IP_TOVARISCH}/${NETMASK}" dev "$VETH_TOVARISCH"
    ip netns exec "$NS_TOVARISCH" ip link set "$VETH_TOVARISCH" up
    ip netns exec "$NS_TOVARISCH" ip link set lo up

    log_info "Interfaces configured"
}

verify_topology() {
    log_info "Verifying namespace topology..."
    local ok=true

    if ! ip netns list | grep -q "$NS_UVB76"; then
        log_error "Namespace $NS_UVB76 not found"
        ok=false
    fi
    if ! ip netns list | grep -q "$NS_TOVARISCH"; then
        log_error "Namespace $NS_TOVARISCH not found"
        ok=false
    fi
    if ! ip netns exec "$NS_UVB76" ip link show "$VETH_UVB76" &> /dev/null; then
        log_error "VETH $VETH_UVB76 not found in $NS_UVB76"
        ok=false
    fi
    if ! ip netns exec "$NS_TOVARISCH" ip link show "$VETH_TOVARISCH" &> /dev/null; then
        log_error "VETH $VETH_TOVARISCH not found in $NS_TOVARISCH"
        ok=false
    fi

    if $ok; then
        log_info "Topology verification passed"
        return 0
    else
        log_error "Topology verification failed"
        return 1
    fi
}

collect_topology_info() {
    log_info "Collecting topology information..."

    # Write topology
    cat > "$TOPOLOGY_FILE" <<EOF
UVB-76 Capture Netns Lab Topology
=================================

Namespace: $NS_UVB76
  Interface: $VETH_UVB76
  IP: ${IP_UVB76}/${NETMASK}

Namespace: $NS_TOVARISCH
  Interface: $VETH_TOVARISCH
  IP: ${IP_TOVARISCH}/${NETMASK}

veth pair connects the namespaces

Diagnostic peer URL: $DIAG_PEER_BASE_URL
EOF

    # Collect IP addresses
    ip netns exec "$NS_UVB76" ip addr show > "$NS_UVB76_IP_ADDR_FILE"
    ip netns exec "$NS_TOVARISCH" ip addr show > "$NS_TOVARISCH_IP_ADDR_FILE"

    # Collect routes
    ip netns exec "$NS_UVB76" ip route show > "$NS_UVB76_IP_ROUTE_FILE"
    ip netns exec "$NS_TOVARISCH" ip route show > "$NS_TOVARISCH_IP_ROUTE_FILE"

    log_info "Topology information collected"
}

verify_baseline_connectivity() {
    log_info "Verifying baseline connectivity from uvb76 namespace..."

    # Ping test
    if ip netns exec "$NS_UVB76" ping -c 1 -W 2 "$IP_TOVARISCH" > "$PING_BASELINE_FILE" 2>&1; then
        log_info "Ping from uvb76 to tovarisch: OK"
    else
        log_error "Ping from uvb76 to tovarisch: FAILED"
        return 1
    fi

    # HTTP test
    local http_status
    http_status=$(ip netns exec "$NS_UVB76" curl -s -o /dev/null -w "%{http_code}" "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/status.json?include=network_diag" 2>/dev/null)
    if [[ "$http_status" == "200" ]]; then
        log_info "HTTP from uvb76 to tovarisch: OK (status $http_status)"
    else
        log_error "HTTP from uvb76 to tovarisch: FAILED (status $http_status)"
        return 1
    fi

    log_info "Baseline connectivity verified"
    return 0
}
