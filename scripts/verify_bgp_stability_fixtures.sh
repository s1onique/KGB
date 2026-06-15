#!/bin/bash
# verify_bgp_stability_fixtures.sh — Self-test fixtures for BGP stability verifier
#
# This file contains fixture data for the --self-test mode of verify_bgp_stability.sh.
# It is sourced by the main verifier when running self-tests.

# Colors (must be defined before sourcing)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*"; }
log_info() { echo -e "[INFO] $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }

# Absolute path to the verifier (set by the main script)
# VERIFIER_PATH set by verify_bgp_stability.sh before sourcing

# =============================================================================
# Self-Test Fixtures
# =============================================================================

# Test 1: PASS case - all healthy
create_fixture_healthy() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0, "state": "active"}, "checks": []}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 1, "state": "established"}, "checks": []}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 1, "state": "established"}, "checks": []}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
Name       Proto      State          Info
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
Name       Proto      State          Info
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
Name       Proto      State          Info
tovarisch  BGP       Established
EOF
    echo "10.0.0.0/24 via 10.77.0.2" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

# Test 2: FAIL - reconnect delta 53
create_fixture_reconnect_delta() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 53}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 53}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    echo "10.0.0.0/24" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

# Test 3: FAIL - reconnect_count missing
create_fixture_missing_reconnect() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    echo "10.0.0.0/24" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

# Test 4: FAIL - reconnect_count non-numeric
create_fixture_non_numeric_reconnect() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": "abc"}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": "xyz"}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": "xyz"}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    echo "10.0.0.0/24" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

# Test 5: FAIL - bird-routes.txt missing
create_fixture_missing_routes() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
    # bird-routes.txt is intentionally not created
}

# Test 6: FAIL - bird-routes.txt empty
create_fixture_empty_routes() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
    touch "$test_dir/bird-routes.txt"  # empty file
}

# Test 7: FAIL - BIRD protocol contains Socket: Connection closed
create_fixture_connection_closed() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
Name       Proto      State          Info
tovarisch  BGP       Established    Socket: Connection closed
EOF
    echo "10.0.0.0/24" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

# Test 8: FAIL - BIRD after-stability not Established
create_fixture_bird_unstable() {
    local test_dir="$1"
    cat > "$test_dir/status-before.json" <<'EOF'
{"bgp": {"reconnect_count": 0}}
EOF
    cat > "$test_dir/status-first-established.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/status-after-stability.json" <<'EOF'
{"bgp": {"reconnect_count": 1}}
EOF
    cat > "$test_dir/bird-protocol-before.txt" <<'EOF'
tovarisch  BGP       Active
EOF
    cat > "$test_dir/bird-protocol-first-established.txt" <<'EOF'
tovarisch  BGP       Established
EOF
    cat > "$test_dir/bird-protocol-after-stability.txt" <<'EOF'
Name       Proto      State          Info
tovarisch  BGP       Idle           Socket: Connection closed
EOF
    echo "10.0.0.0/24" > "$test_dir/bird-routes.txt"
    cat > "$test_dir/bird-bfd-sessions.txt" <<'EOF'
tovarisch  BFD       Up
EOF
}

