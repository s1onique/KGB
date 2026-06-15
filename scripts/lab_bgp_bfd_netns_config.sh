#!/bin/bash
# lab_bgp_bfd_netns_config.sh — Config generation for netns lab
#
# Configuration generation functions for the BGP/BFD netns lab.
# Sourced by lab_bgp_bfd_netns_lib.sh.

# Generate BIRD configuration with multihop BFD
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

    # Add BFD multihop protocol (matching tovarisch UDP 4784 per RFC 5883)
    # NOTE: tovarisch uses multihop BFD on UDP port 4784
    # BIRD interface{} syntax is single-hop (UDP 3784) - WRONG for tovarisch
    # BFD sessions are not created automatically; must declare explicit neighbor.
    # BIRD docs: BFD sessions are created on demand by protocols like BGP,
    # or explicitly via neighbor {} declarations.
    cat >> "$BIRD_CONFIG" << EOF
# BFD multihop protocol - UDP 4784 (RFC 5883)
# Must match tovarisch multihop mode on UDP port 4784
protocol bfd {
    multihop {
        interval $BFD_INTERVAL_MS ms;
        multiplier $BFD_MULTIPLIER;
    };

    # ACT 2.1: Explicit multihop neighbor required to create BFD session.
    # Without this, BIRD's bfd protocol is up but no session is created.
    neighbor $IP_TOVARISCH local $IP_BIRD multihop;
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

# Generate tovarisch configuration
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

# Generate prefix file
generate_prefix_file() {
    log_info "Generating prefix file..."

    echo "# Lab prefix file - add/remove prefixes to test BGP export" > "$PREFIX_FILE"
    echo "# Format: one CIDR prefix per line" >> "$PREFIX_FILE"
    echo "10.77.77.0/24" >> "$PREFIX_FILE"

    log_info "Prefix file written to $PREFIX_FILE"
}
