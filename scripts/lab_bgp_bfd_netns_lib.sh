#!/bin/bash
# lab_bgp_bfd_netns_lib.sh — Shared library functions for netns lab
#
# Constants and helper functions for the BGP/BFD netns lab harness.
# This is sourced by lab_bgp_bfd_netns.sh.

# Source constants
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_bgp_bfd_netns_consts.sh
source "${SCRIPT_DIR}/lab_bgp_bfd_netns_consts.sh"

# Global variables (set by main script)
declare -g LAB_DIR=""
declare -g BIRD_CONFIG=""
declare -g TOVARISCH_CONFIG=""
declare -g PREFIX_FILE=""
declare -g BIRD_LOG=""
declare -g TOVARISCH_LOG=""
declare -g STATUS_OUTPUT=""
declare -g BGP_ROUTES_OUTPUT=""
declare -g BIRD_SOCKET=""
declare -g BIRD_VERSION="2"

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# Helper to call birdc targeting the lab BIRD instance
birdc_lab() {
    birdc -s "$BIRD_SOCKET" "$@"
}

detect_bird_version() {
    local bird_version_output
    bird_version_output=$(birdc show version 2>/dev/null || bird --version 2>/dev/null || echo "unknown")

    if echo "$bird_version_output" | grep -qE "BIRD 2\."; then
        BIRD_VERSION=2
        log_info "Detected BIRD 2.x"
    elif echo "$bird_version_output" | grep -qE "BIRD 1\."; then
        BIRD_VERSION=1
        log_info "Detected BIRD 1.x"
    else
        BIRD_VERSION=2
        log_info "BIRD version detection ambiguous, assuming 2.x"
    fi
}

setup_temp_dir() {
    LAB_DIR=$(mktemp -d "/tmp/${LAB_NAME}-XXXXXX")
    if [[ ! -d "$LAB_DIR" ]]; then
        log_error "Failed to create temp directory"
        return 1
    fi
    log_info "Lab directory: $LAB_DIR"

    BIRD_CONFIG="$LAB_DIR/bird.conf"
    TOVARISCH_CONFIG="$LAB_DIR/tovarisch.conf"
    PREFIX_FILE="$LAB_DIR/prefixes.txt"
    BIRD_LOG="$LAB_DIR/bird.log"
    TOVARISCH_LOG="$LAB_DIR/tovarisch.log"
    STATUS_OUTPUT="$LAB_DIR/status.json"
    BGP_ROUTES_OUTPUT="$LAB_DIR/bird-routes.txt"
    BIRD_SOCKET="$LAB_DIR/bird.ctl"
}

cleanup() {
    log_info "Cleaning up..."

    # Use lab-specific socket for cleanup
    if [[ -S "$BIRD_SOCKET" ]]; then
        birdc_lab disable all 2>/dev/null || true
        birdc_lab down 2>/dev/null || true
    fi
    pkill -f "bird.*$BIRD_CONFIG" 2>/dev/null || true
    pkill -f "tovarisch.*$TOVARISCH_CONFIG" 2>/dev/null || true

    # Delete network namespaces (veth pairs go with them)
    ip netns del "$NS_TOVARISCH" 2>/dev/null || true
    ip netns del "$NS_BIRD" 2>/dev/null || true

    log_info "Cleanup complete"
}

setup_trap() { trap cleanup EXIT; }

create_namespaces() {
    log_info "Creating network namespaces..."

    ip netns add "$NS_TOVARISCH" 2>/dev/null || log_warn "Namespace $NS_TOVARISCH may already exist"
    ip netns add "$NS_BIRD" 2>/dev/null || log_warn "Namespace $NS_BIRD may already exist"

    # Create veth pair
    ip link add "$VETH_TOVARISCH" type veth peer name "$VETH_BIRD"

    # Move BOTH endpoints into their intended namespaces
    ip link set "$VETH_TOVARISCH" netns "$NS_TOVARISCH"
    ip link set "$VETH_BIRD" netns "$NS_BIRD"

    log_info "Network namespaces and veth pairs created"
}

configure_interfaces() {
    log_info "Configuring network interfaces inside namespaces..."

    # Configure tovarisch side (inside its namespace)
    ip netns exec "$NS_TOVARISCH" ip addr add "10.77.0.2/30" dev "$VETH_TOVARISCH"
    ip netns exec "$NS_TOVARISCH" ip link set "$VETH_TOVARISCH" up
    ip netns exec "$NS_TOVARISCH" ip link set lo up

    # Configure BIRD side (inside its namespace)
    ip netns exec "$NS_BIRD" ip addr add "10.77.0.1/30" dev "$VETH_BIRD"
    ip netns exec "$NS_BIRD" ip link set "$VETH_BIRD" up
    ip netns exec "$NS_BIRD" ip link set lo up

    log_info "Interfaces configured"
}

generate_bird_config() {
    log_info "Generating BIRD config..."

    cat > "$BIRD_CONFIG" << EOF
# BIRD configuration for KGB netns lab
# Generated automatically - do not edit

log "$BIRD_LOG" all;
debug protocols all;

# Router ID
router id $BIRD_ROUTER_ID;

# Control socket for lab-specific targeting
protocol kernel {
    ipv4 {
        import none;
        export all;
    };
    learn;
    scan time 10;
}

# Device protocol
protocol device {
    scan time 10;
}

# Static routes (for lab routes)
protocol static {
    ipv4;
}

EOF

    # Add BFD protocol
    cat >> "$BIRD_CONFIG" << EOF
# BFD multihop protocol
protocol bfd {
    interface "$VETH_BIRD" { interval $BFD_INTERVAL_MS ms; multiplier $BFD_MULTIPLIER; };
}

EOF

    # Add BGP protocol
    cat >> "$BIRD_CONFIG" << EOF
# BGP protocol - tovarisch peer
protocol bgp tovarisch {
    local $IP_BIRD as $BIRD_AS;
    neighbor $IP_TOVARISCH as $TOVARISCH_AS;
    passive;
    enable extended messages;

    ipv4 {
        import all;
        export where source = RTS_STATIC;
    };
}

EOF

    log_info "BIRD config written to $BIRD_CONFIG"
}

generate_tovarisch_config() {
    log_info "Generating tovarisch config..."

    cat > "$TOVARISCH_CONFIG" << EOF
# tovarisch configuration for KGB netns lab
# Generated automatically - do not edit

[bfd]
enabled = true
local_addr = $IP_TOVARISCH
peer_addr = $IP_BIRD
interval_ms = $BFD_INTERVAL_MS
multiplier = $BFD_MULTIPLIER

[bgp]
enabled = true
local_address = $IP_TOVARISCH
router_id = $TOVARISCH_ROUTER_ID
local_as = $TOVARISCH_AS
peer_address = $IP_BIRD
peer_as = $BIRD_AS
advertised_prefix_files = $PREFIX_FILE
EOF

    log_info "tovarisch config written to $TOVARISCH_CONFIG"
}

generate_prefix_file() {
    log_info "Generating prefix file..."

    echo "# Lab prefix file - add/remove prefixes to test BGP export" > "$PREFIX_FILE"
    echo "# Format: one CIDR prefix per line" >> "$PREFIX_FILE"
    echo "10.77.77.0/24" >> "$PREFIX_FILE"

    log_info "Prefix file written to $PREFIX_FILE"
}

start_bird() {
    log_info "Starting BIRD in namespace $NS_BIRD..."

    ip netns exec "$NS_BIRD" pkill bird 2>/dev/null || true
    sleep 1

    # Start BIRD with lab-specific control socket
    ip netns exec "$NS_BIRD" bird -s "$BIRD_SOCKET" -f -c "$BIRD_CONFIG" &

    # Wait for BIRD to initialize and socket to appear
    sleep "$WAIT_BIRD_START"

    if ! ip netns exec "$NS_BIRD" pgrep -x bird &> /dev/null; then
        log_error "BIRD failed to start"
        cat "$BIRD_LOG" 2>/dev/null || true
        return 1
    fi

    log_info "BIRD started successfully with control socket $BIRD_SOCKET"
}

start_tovarisch() {
    log_info "Starting tovarisch in namespace $NS_TOVARISCH..."

    local binary="${TOVARISCH_BINARY:-./tovarisch/zig-out/bin/tovarisch}"

    if [[ ! -x "$binary" ]]; then
        log_error "tovarisch binary not found: $binary"
        return 1
    fi

    ip netns exec "$NS_TOVARISCH" pkill tovarisch 2>/dev/null || true
    sleep 1

    ip netns exec "$NS_TOVARISCH" "$binary" serve \
        --config "$TOVARISCH_CONFIG" \
        > "$TOVARISCH_LOG" 2>&1 &

    sleep 3

    if ! ip netns exec "$NS_TOVARISCH" pgrep -x tovarisch &> /dev/null; then
        log_error "tovarisch failed to start"
        cat "$TOVARISCH_LOG" 2>/dev/null || true
        return 1
    fi

    log_info "tovarisch started successfully"
}

collect_status() {
    log_info "Collecting tovarisch status..."

    local binary="${TOVARISCH_BINARY:-./tovarisch/zig-out/bin/tovarisch}"

    if ip netns exec "$NS_TOVARISCH" "$binary" status --json > "$STATUS_OUTPUT" 2>&1; then
        log_info "Status collected:"
        cat "$STATUS_OUTPUT"
        return 0
    else
        log_error "Failed to collect status"
        cat "$STATUS_OUTPUT" 2>/dev/null || true
        return 1
    fi
}

wait_bfd_convergence() {
    log_info "Waiting for BFD convergence (${WAIT_BFD_CONVERGE}s)..."

    local elapsed=0
    local interval=2

    while [[ $elapsed -lt $WAIT_BFD_CONVERGE ]]; do
        local bfd_status
        bfd_status=$(birdc_lab show bfd sessions 2>/dev/null || echo "")

        if echo "$bfd_status" | grep -qE "Up"; then
            log_info "BFD session is UP"
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "."
    done

    echo ""
    log_warn "BFD convergence timeout - session may not be established"
    return 1
}

wait_bgp_convergence() {
    log_info "Waiting for BGP convergence (${WAIT_BGP_CONVERGE}s)..."

    local elapsed=0
    local interval=2

    while [[ $elapsed -lt $WAIT_BGP_CONVERGE ]]; do
        local bgp_status
        bgp_status=$(birdc_lab show protocols bgp tovarisch 2>/dev/null || echo "")

        if echo "$bgp_status" | grep -qE "Established"; then
            log_info "BGP session is Established"
            return 0
        fi

        sleep $interval
        elapsed=$((elapsed + interval))
        echo -n "."
    done

    echo ""
    log_warn "BGP convergence timeout - session may not be established"
    return 1
}

collect_bgp_routes() {
    log_info "Collecting BGP routes from BIRD..."

    if birdc_lab show routes 2>/dev/null > "$BGP_ROUTES_OUTPUT"; then
        log_info "Routes collected:"
        cat "$BGP_ROUTES_OUTPUT"
        return 0
    else
        log_warn "Failed to collect BGP routes"
        return 1
    fi
}

verify_topology() {
    log_info "Verifying namespace topology..."

    local ok=true

    # Check namespaces exist
    if ! ip netns list | grep -q "$NS_TOVARISCH"; then
        log_error "Namespace $NS_TOVARISCH not found"
        ok=false
    fi

    if ! ip netns list | grep -q "$NS_BIRD"; then
        log_error "Namespace $NS_BIRD not found"
        ok=false
    fi

    # Check both veth endpoints exist inside their namespaces
    if ! ip netns exec "$NS_TOVARISCH" ip link show "$VETH_TOVARISCH" &> /dev/null; then
        log_error "VETH $VETH_TOVARISCH not found in $NS_TOVARISCH"
        ok=false
    fi

    if ! ip netns exec "$NS_BIRD" ip link show "$VETH_BIRD" &> /dev/null; then
        log_error "VETH $VETH_BIRD not found in $NS_BIRD"
        ok=false
    fi

    # Verify expected IPs are assigned
    if ! ip netns exec "$NS_TOVARISCH" ip addr show "$VETH_TOVARISCH" | grep -q "10.77.0.2"; then
        log_error "Expected IP 10.77.0.2 not found in $NS_TOVARISCH"
        ok=false
    fi

    if ! ip netns exec "$NS_BIRD" ip addr show "$VETH_BIRD" | grep -q "10.77.0.1"; then
        log_error "Expected IP 10.77.0.1 not found in $NS_BIRD"
        ok=false
    fi

    # Verify namespace-to-namespace connectivity
    if ip netns exec "$NS_TOVARISCH" ping -c1 -W1 "$IP_BIRD" &> /dev/null; then
        log_info "Connectivity: tovarisch -> BIRD (OK)"
    else
        log_error "Connectivity: tovarisch -> BIRD (FAILED)"
        ok=false
    fi

    if ip netns exec "$NS_BIRD" ping -c1 -W1 "$IP_TOVARISCH" &> /dev/null; then
        log_info "Connectivity: BIRD -> tovarisch (OK)"
    else
        log_error "Connectivity: BIRD -> tovarisch (FAILED)"
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
