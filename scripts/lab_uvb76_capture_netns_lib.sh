#!/bin/bash
# lab_uvb76_capture_netns_lib.sh — Shared library for UVB-76 capture netns lab
#
# Core setup and helper functions. Sources modular components.

# Source constants
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lab_uvb76_capture_netns_consts.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_consts.sh"
# shellcheck source=lab_uvb76_capture_netns_topology.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_topology.sh"
# shellcheck source=lab_uvb76_capture_netns_defect.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_defect.sh"
# shellcheck source=lab_uvb76_capture_netns_diag.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_diag.sh"
# shellcheck source=lab_uvb76_capture_netns_poll.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_poll.sh"
# shellcheck source=lab_uvb76_capture_netns_capture_poll.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_capture_poll.sh"
# shellcheck source=lab_uvb76_capture_netns_result.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_result.sh"
# shellcheck source=lab_uvb76_capture_netns_tovarisch.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_tovarisch.sh"
# shellcheck source=lab_uvb76_capture_netns_contract.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract.sh"
# shellcheck source=lab_uvb76_capture_netns_contract_normalizers.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract_normalizers.sh"
# shellcheck source=lab_uvb76_capture_netns_contract_assertions.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_contract_assertions.sh"
# shellcheck source=lab_uvb76_capture_netns_phase_helpers.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_phase_helpers.sh"
# shellcheck source=lab_uvb76_capture_netns_result_helpers.sh
source "${SCRIPT_DIR}/lab_uvb76_capture_netns_result_helpers.sh"

# Global variables (set by main script)
declare -g LAB_DIR=""
declare -g UVB76_CONFIG=""
declare -g TOVARISCH_CONFIG=""
declare -g UVB76_LOG=""
declare -g TOVARISCH_LOG=""
declare -g UVB76_PID=""
declare -g TOVARISCH_PID=""

# Artifact paths
declare -g TOPOLOGY_FILE=""
declare -g NS_UVB76_IP_ADDR_FILE=""
declare -g NS_UVB76_IP_ROUTE_FILE=""
declare -g NS_TOVARISCH_IP_ADDR_FILE=""
declare -g NS_TOVARISCH_IP_ROUTE_FILE=""
declare -g TOVARISCH_LISTEN_SOCKETS_FILE=""
declare -g PING_BASELINE_FILE=""
declare -g CURL_STATUS_BASELINE_FILE=""
declare -g CURL_STATUS_NETWORK_DIAG_BASELINE_FILE=""
declare -g CURL_PEER_STATUS_NETWORK_DIAG_FILE=""
declare -g CURL_PEER_STATUS_NETWORK_DIAG_EXITCODE_FILE=""
declare -g CAPTURE_BASELINE_FILE=""
declare -g DEFECT_BEFORE_FILE=""
declare -g DEFECT_TC_QDISC_FILE=""
declare -g CAPTURE_DURING_DEFECT_FILE=""
declare -g DEFECT_AFTER_CLEAR_FILE=""
declare -g CAPTURE_AFTER_RECOVERY_FILE=""
declare -g LATENCY_DURING_DEFECT_FILE=""
declare -g LATENCY_AFTER_RECOVERY_FILE=""
declare -g UVB76_PROBE_CAPTURE_EVENTS_FILE=""
declare -g RESULT_FILE=""
declare -g BASELINE_PROBE_READY_FILE=""
declare -g SPIKES_DURING_DEFECT_POLL_FILE=""
declare -g SPIKES_AFTER_RECOVERY_POLL_FILE=""

# Phase-separated artifact paths (for diagnostic packet contract verification)
declare -g PHASE0_STATUS_FILE=""
declare -g PHASE0_PROBE_READY_FILE=""
declare -g PHASE1_SPIKE_EVENT_FILE=""
declare -g PHASE1_SPIKE_ROW_FILE=""
declare -g PHASE1_CAPTURE_PACKET_FILE=""
declare -g PHASE1_CAPTURE_CONTRACT_FILE=""
declare -g PHASE2_SPIKE_EVENT_FILE=""
declare -g PHASE2_SPIKE_ROW_FILE=""
declare -g PHASE2_CAPTURE_CONTRACT_FILE=""
declare -g PHASE3_SPIKE_EVENT_FILE=""
declare -g PHASE3_SPIKE_ROW_FILE=""
declare -g PHASE3_CAPTURE_PACKET_FILE=""
declare -g PHASE3_CAPTURE_CONTRACT_FILE=""
declare -g CONTRACT_VERIFIER_OUTPUT_FILE=""

# Lab result tracking
declare -g PROBE_READY=false
declare -g BASELINE_CAPTURE_OK=false
declare -g DEFECT_OBSERVED=false
declare -g RECOVERY_CAPTURE_OK=false
declare -g REQUESTED_PATH_BASELINE=""
declare -g REQUESTED_PATH_DURING_DEFECT=""
declare -g REQUESTED_PATH_AFTER_RECOVERY=""
declare -g DEFECT_MODE="100pct-loss"

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# Check if running on Linux
check_linux() {
    if [[ "$(uname)" != "Linux" ]]; then
        log_error "This lab requires Linux (network namespaces)."
        log_error "On macOS, run manually in GitHub Actions."
        return 1
    fi
    return 0
}

# Check required tools
check_dependencies() {
    local missing=()

    for cmd in ip tc curl jq; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing+=("$cmd")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error "Install: iproute2 jq curl"
        return 1
    fi

    if ! ip netns list >/dev/null 2>&1; then
        log_error "ip netns is unavailable or not permitted"
        log_error "Install iproute2 and run with sufficient privileges"
        return 1
    fi

    return 0
}

setup_temp_dir() {
    LAB_DIR=$(mktemp -d "/tmp/${LAB_NAME}-XXXXXX")
    if [[ ! -d "$LAB_DIR" ]]; then
        log_error "Failed to create temp directory"
        return 1
    fi
    log_info "Lab directory: $LAB_DIR"
    # Config and log files
    UVB76_CONFIG="$LAB_DIR/${ARTIFACT_UVB76_CONFIG}"
    TOVARISCH_CONFIG="$LAB_DIR/${ARTIFACT_TOVARISCH_CONFIG}"
    UVB76_LOG="$LAB_DIR/${ARTIFACT_UVB76_LOG}"
    TOVARISCH_LOG="$LAB_DIR/${ARTIFACT_TOVARISCH_LOG}"
    # Artifact paths
    TOPOLOGY_FILE="$LAB_DIR/${ARTIFACT_TOPOLOGY}"
    NS_UVB76_IP_ADDR_FILE="$LAB_DIR/${ARTIFACT_NS_UVB76_IP_ADDR}"
    NS_UVB76_IP_ROUTE_FILE="$LAB_DIR/${ARTIFACT_NS_UVB76_IP_ROUTE}"
    NS_TOVARISCH_IP_ADDR_FILE="$LAB_DIR/${ARTIFACT_NS_TOVARISCH_IP_ADDR}"
    NS_TOVARISCH_IP_ROUTE_FILE="$LAB_DIR/${ARTIFACT_NS_TOVARISCH_IP_ROUTE}"
    TOVARISCH_LISTEN_SOCKETS_FILE="$LAB_DIR/${ARTIFACT_TOVARISCH_LISTEN_SOCKETS}"
    PING_BASELINE_FILE="$LAB_DIR/${ARTIFACT_PING_BASELINE}"
    CURL_STATUS_BASELINE_FILE="$LAB_DIR/${ARTIFACT_CURL_STATUS_BASELINE}"
    CURL_STATUS_NETWORK_DIAG_BASELINE_FILE="$LAB_DIR/${ARTIFACT_CURL_STATUS_NETWORK_DIAG_BASELINE}"
    CURL_PEER_STATUS_NETWORK_DIAG_FILE="$LAB_DIR/${ARTIFACT_CURL_PEER_STATUS_NETWORK_DIAG}"
    CURL_PEER_STATUS_NETWORK_DIAG_EXITCODE_FILE="$LAB_DIR/${ARTIFACT_CURL_PEER_STATUS_NETWORK_DIAG_EXITCODE}"
    CAPTURE_BASELINE_FILE="$LAB_DIR/${ARTIFACT_CAPTURE_BASELINE}"
    DEFECT_BEFORE_FILE="$LAB_DIR/${ARTIFACT_DEFECT_BEFORE}"
    DEFECT_TC_QDISC_FILE="$LAB_DIR/${ARTIFACT_DEFECT_TC_QDISC}"
    CAPTURE_DURING_DEFECT_FILE="$LAB_DIR/${ARTIFACT_CAPTURE_DURING_DEFECT}"
    DEFECT_AFTER_CLEAR_FILE="$LAB_DIR/${ARTIFACT_DEFECT_AFTER_CLEAR}"
    CAPTURE_AFTER_RECOVERY_FILE="$LAB_DIR/${ARTIFACT_CAPTURE_AFTER_RECOVERY}"
    LATENCY_DURING_DEFECT_FILE="$LAB_DIR/${ARTIFACT_LATENCY_DURING_DEFECT}"
    LATENCY_AFTER_RECOVERY_FILE="$LAB_DIR/${ARTIFACT_LATENCY_AFTER_RECOVERY}"
    UVB76_PROBE_CAPTURE_EVENTS_FILE="$LAB_DIR/${ARTIFACT_UVB76_PROBE_CAPTURE_EVENTS}"
    RESULT_FILE="$LAB_DIR/${ARTIFACT_RESULT}"
    BASELINE_PROBE_READY_FILE="$LAB_DIR/${ARTIFACT_BASELINE_PROBE_READY}"
    SPIKES_DURING_DEFECT_POLL_FILE="$LAB_DIR/${ARTIFACT_SPIKES_DURING_DEFECT_POLL}"
    SPIKES_AFTER_RECOVERY_POLL_FILE="$LAB_DIR/${ARTIFACT_SPIKES_AFTER_RECOVERY_POLL}"
    
    # Phase-separated artifact paths (for diagnostic packet contract verification)
    PHASE0_STATUS_FILE="$LAB_DIR/${ARTIFACT_PHASE0_STATUS}"
    PHASE0_PROBE_READY_FILE="$LAB_DIR/${ARTIFACT_PHASE0_PROBE_READY}"
    PHASE1_SPIKE_EVENT_FILE="$LAB_DIR/${ARTIFACT_PHASE1_SPIKE_EVENT}"
    PHASE1_SPIKE_ROW_FILE="$LAB_DIR/${ARTIFACT_PHASE1_SPIKE_ROW}"
    PHASE1_CAPTURE_PACKET_FILE="$LAB_DIR/${ARTIFACT_PHASE1_CAPTURE_PACKET}"
    PHASE1_CAPTURE_CONTRACT_FILE="$LAB_DIR/${ARTIFACT_PHASE1_CAPTURE_CONTRACT}"
    PHASE2_SPIKE_EVENT_FILE="$LAB_DIR/${ARTIFACT_PHASE2_SPIKE_EVENT}"
    PHASE2_SPIKE_ROW_FILE="$LAB_DIR/${ARTIFACT_PHASE2_SPIKE_ROW}"
    PHASE2_CAPTURE_CONTRACT_FILE="$LAB_DIR/${ARTIFACT_PHASE2_CAPTURE_CONTRACT}"
    PHASE3_SPIKE_EVENT_FILE="$LAB_DIR/${ARTIFACT_PHASE3_SPIKE_EVENT}"
    PHASE3_SPIKE_ROW_FILE="$LAB_DIR/${ARTIFACT_PHASE3_SPIKE_ROW}"
    PHASE3_CAPTURE_PACKET_FILE="$LAB_DIR/${ARTIFACT_PHASE3_CAPTURE_PACKET}"
    PHASE3_CAPTURE_CONTRACT_FILE="$LAB_DIR/${ARTIFACT_PHASE3_CAPTURE_CONTRACT}"
    CONTRACT_VERIFIER_OUTPUT_FILE="$LAB_DIR/${ARTIFACT_CONTRACT_VERIFIER_OUTPUT}"
}

make_artifacts_readable() {
    if [[ -n "${LAB_DIR:-}" && -d "$LAB_DIR" ]]; then
        chmod -R a+rX "$LAB_DIR" 2>/dev/null || true
        log_info "Made lab artifacts readable: $LAB_DIR"
    fi
}

cleanup() {
    log_info "Cleaning up..."
    make_artifacts_readable

    # Stop processes
    if [[ -n "$UVB76_PID" ]]; then
        kill "$UVB76_PID" 2>/dev/null || true
    fi
    if [[ -n "$TOVARISCH_PID" ]]; then
        ip netns exec "$NS_TOVARISCH" pkill tovarisch 2>/dev/null || true
    fi

    # Clean up tc qdiscs inside namespace
    ip netns exec "$NS_TOVARISCH" tc qdisc del dev "$VETH_TOVARISCH" root 2>/dev/null || true

    # Delete network namespaces
    ip netns del "$NS_TOVARISCH" 2>/dev/null || true
    ip netns del "$NS_UVB76" 2>/dev/null || true

    log_info "Cleanup complete"
}

setup_trap() { trap cleanup EXIT; }

generate_tovarisch_config() {
    log_info "Generating tovarisch config..."
    cat > "$TOVARISCH_CONFIG" <<EOF
[server]
listen = "0.0.0.0:${TOVARISCH_PORT}"
EOF
    log_info "tovarisch config: $TOVARISCH_CONFIG"
}

generate_uvb76_config() {
    log_info "Generating UVB-76 config..."
    # Lab: 2s probes, 1s timeout, 5s cooldown
    cat > "$UVB76_CONFIG" <<EOF
{
  "listen": {"addr": ":${UVB76_PORT}", "tls_cert_file": "", "tls_key_file": ""},
  "auth": {"username": "lab-admin", "password_sha256": "sha256:ad31a00094d25f7b5b3fa5ba2a4998db:ae3908b2ae4825fc884248f29385f4497ca9f3ff0c3d1416c6a216f3a400c4e1"},
  "scrape": {"interval_seconds": 60, "timeout_milliseconds": 5000},
  "latency": {
    "http": {"enabled": true, "interval_seconds": 2, "timeout_milliseconds": 1000, "window_seconds": 60, "retained_range_seconds": 120, "histogram_buckets_ms": [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000], "recent_samples_max": 120},
    "icmp": {"enabled": false, "interval_seconds": 1, "timeout_seconds": 3, "window_seconds": 60, "retained_range_seconds": 300, "histogram_buckets_ms": [1, 5, 10, 25, 50, 100, 250, 500, 1000], "recent_samples_max": 300}
  },
  "diagnostics": {"enabled": true, "capture_on_spike": true, "timeout_ms": 2000, "cooldown_seconds": 5, "max_uncaptured_spikes": 50, "peers": [{"name": "tovarisch-lab", "base_url": "${DIAG_PEER_BASE_URL}", "targets": ["lab-tovarisch"]}]},
  "targets": [{"id": "lab-tovarisch", "name": "Lab Tovarisch", "base_url": "${DIAG_PEER_BASE_URL}", "enabled": true}]
}
EOF
    log_info "UVB-76 config: $UVB76_CONFIG"
}

start_tovarisch() {
    log_info "Starting tovarisch in namespace $NS_TOVARISCH..."
    local binary="${TOVARISCH_BINARY:-./tovarisch/zig-out/bin/tovarisch}"

    if [[ ! -x "$binary" ]]; then
        log_error "tovarisch binary not found: $binary"
        return 1
    fi

    # Kill any existing tovarisch
    ip netns exec "$NS_TOVARISCH" pkill tovarisch 2>/dev/null || true
    sleep 1

    # Start tovarisch serve
    # --listen-all-public-dangerous is required because config binds to 0.0.0.0
    ip netns exec "$NS_TOVARISCH" "$binary" serve \
        --config "$TOVARISCH_CONFIG" \
        --listen-all-public-dangerous \
        > "$TOVARISCH_LOG" 2>&1 &

    TOVARISCH_PID=$!

    log_info "tovarisch started with PID $TOVARISCH_PID"
    sleep "$WAIT_TOVARISCH_START"

    # Verify it's running
    if ! kill -0 "$TOVARISCH_PID" 2>/dev/null; then
        log_error "tovarisch failed to start"
        cat "$TOVARISCH_LOG" 2>/dev/null || true
        return 1
    fi

    log_info "tovarisch started successfully"
    return 0
}

start_uvb76() {
    log_info "Starting UVB-76 in namespace $NS_UVB76..."
    local binary="${UVB76_BINARY:-./uvb76/uvb76}"

    if [[ ! -x "$binary" ]]; then
        log_error "UVB-76 binary not found: $binary"
        return 1
    fi

    # Kill any existing uvb76
    ip netns exec "$NS_UVB76" pkill uvb76 2>/dev/null || true
    sleep 1

    # Start uvb76 in dev mode (no TLS required)
    ip netns exec "$NS_UVB76" "$binary" -dev -config "$UVB76_CONFIG" \
        > "$UVB76_LOG" 2>&1 &

    UVB76_PID=$!

    log_info "UVB-76 started with PID $UVB76_PID"
    sleep "$WAIT_UVB76_START"

    # Verify it's running
    if ! kill -0 "$UVB76_PID" 2>/dev/null; then
        log_error "UVB-76 failed to start"
        cat "$UVB76_LOG" 2>/dev/null || true
        return 1
    fi

    log_info "UVB-76 started successfully"
    return 0
}

wait_for_tovarisch_http() {
    log_info "Waiting for tovarisch HTTP endpoint..."
    local max_attempts=10
    local attempt=0

    while [[ $attempt -lt $max_attempts ]]; do
        if ip netns exec "$NS_TOVARISCH" curl -s -o /dev/null -w "%{http_code}" \
            "http://localhost:${TOVARISCH_PORT}/status.json" 2>/dev/null | grep -q "200"; then
            log_info "tovarisch HTTP endpoint is ready"
            return 0
        fi
        sleep 1
        attempt=$((attempt + 1))
    done

    log_error "tovarisch HTTP endpoint did not become ready"
    return 1
}

verify_tovarisch_status() {
    log_info "Verifying tovarisch status endpoints..."

    # Check /status.json from localhost (ns-tovarisch)
    local status_json
    status_json=$(ip netns exec "$NS_TOVARISCH" curl -s \
        "http://localhost:${TOVARISCH_PORT}/status.json" 2>/dev/null)
    if [[ -z "$status_json" ]]; then
        log_error "Failed to get /status.json from tovarisch"
        return 1
    fi
    echo "$status_json" > "$CURL_STATUS_BASELINE_FILE"

    # Check /status.json?include=network_diag from localhost (ns-tovarisch)
    local status_network_diag
    status_network_diag=$(ip netns exec "$NS_TOVARISCH" curl -s \
        "http://localhost:${TOVARISCH_PORT}/status.json?include=network_diag" 2>/dev/null)
    if [[ -z "$status_network_diag" ]]; then
        log_error "Failed to get /status.json?include=network_diag from tovarisch"
        return 1
    fi
    echo "$status_network_diag" > "$CURL_STATUS_NETWORK_DIAG_BASELINE_FILE"

    # Verify network_diag is present
    if ! echo "$status_network_diag" | jq -e '.network_diag' >/dev/null 2>&1; then
        log_error "network_diag not present in tovarisch response"
        return 1
    fi

    # CRITICAL: Verify peer reachability from ns-uvb76 to veth address
    # This is the smoking gun for loopback-only binding issue.
    # curl status 000 means no HTTP response received (connection failure).
    log_info "Verifying peer reachability from ns-uvb76 to veth address..."

    # Capture curl output with verbose mode for diagnostics
    set +e
    ip netns exec "$NS_UVB76" curl -sv --connect-timeout 3 \
        "http://${IP_TOVARISCH}:${TOVARISCH_PORT}/status.json?include=network_diag" \
        > "$CURL_PEER_STATUS_NETWORK_DIAG_FILE" 2>&1
    local curl_exit_code=$?
    echo "$curl_exit_code" > "$CURL_PEER_STATUS_NETWORK_DIAG_EXITCODE_FILE"
    set -e

    # Check for HTTP response code in verbose output
    local http_code
    http_code=$(grep -E '< HTTP/' "$CURL_PEER_STATUS_NETWORK_DIAG_FILE" | tail -1 | awk '{print $3}' || echo "")

    if [[ -z "$http_code" ]]; then
        log_error "Peer reachability FAILED: No HTTP response from ${IP_TOVARISCH}:${TOVARISCH_PORT}"
        log_error "curl exit code: $curl_exit_code"
        log_error "Expected: HTTP 200, Got: connection failure (check tovarisch binds to 0.0.0.0 not 127.0.0.1)"
        log_error "Artifact: $CURL_PEER_STATUS_NETWORK_DIAG_FILE"
        return 1
    fi

    if [[ "$http_code" != "200" ]]; then
        log_error "Peer reachability returned HTTP $http_code (expected 200)"
        return 1
    fi

    log_info "Peer reachability verified: HTTP $http_code from ns-uvb76 to ${IP_TOVARISCH}:${TOVARISCH_PORT}"
    log_info "tovarisch status endpoints verified (localhost + peer)"
    return 0
}
