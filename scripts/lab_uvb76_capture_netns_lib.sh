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
declare -g PING_BASELINE_FILE=""
declare -g CURL_STATUS_BASELINE_FILE=""
declare -g CURL_STATUS_NETWORK_DIAG_BASELINE_FILE=""
declare -g CAPTURE_BASELINE_FILE=""
declare -g DEFECT_BEFORE_FILE=""
declare -g DEFECT_TC_QDISC_FILE=""
declare -g CAPTURE_DURING_DEFECT_FILE=""
declare -g DEFECT_AFTER_CLEAR_FILE=""
declare -g CAPTURE_AFTER_RECOVERY_FILE=""
declare -g RESULT_FILE=""

# Lab result tracking
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
    for cmd in ip netns tc curl jq; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_error "Install: iproute2 jq curl"
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

    # Config files
    UVB76_CONFIG="$LAB_DIR/uvb76.json"
    TOVARISCH_CONFIG="$LAB_DIR/tovarisch.conf"

    # Log files
    UVB76_LOG="$LAB_DIR/uvb76.log"
    TOVARISCH_LOG="$LAB_DIR/tovarisch.log"

    # Artifact paths
    TOPOLOGY_FILE="$LAB_DIR/topology.txt"
    NS_UVB76_IP_ADDR_FILE="$LAB_DIR/ns-uvb76-ip-addr.txt"
    NS_UVB76_IP_ROUTE_FILE="$LAB_DIR/ns-uvb76-ip-route.txt"
    NS_TOVARISCH_IP_ADDR_FILE="$LAB_DIR/ns-tovarisch-ip-addr.txt"
    NS_TOVARISCH_IP_ROUTE_FILE="$LAB_DIR/ns-tovarisch-ip-route.txt"
    PING_BASELINE_FILE="$LAB_DIR/ping-baseline.txt"
    CURL_STATUS_BASELINE_FILE="$LAB_DIR/curl-status-baseline.json"
    CURL_STATUS_NETWORK_DIAG_BASELINE_FILE="$LAB_DIR/curl-status-network-diag-baseline.json"
    CAPTURE_BASELINE_FILE="$LAB_DIR/capture-baseline.json"
    DEFECT_BEFORE_FILE="$LAB_DIR/defect-before.txt"
    DEFECT_TC_QDISC_FILE="$LAB_DIR/defect-tc-qdisc.txt"
    CAPTURE_DURING_DEFECT_FILE="$LAB_DIR/capture-during-defect.json"
    DEFECT_AFTER_CLEAR_FILE="$LAB_DIR/defect-after-clear.txt"
    CAPTURE_AFTER_RECOVERY_FILE="$LAB_DIR/capture-after-recovery.json"
    RESULT_FILE="$LAB_DIR/result.json"
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
{
  "serve": {
    "http_port": ${TOVARISCH_PORT}
  }
}
EOF
    log_info "tovarisch config: $TOVARISCH_CONFIG"
}

generate_uvb76_config() {
    log_info "Generating UVB-76 config..."
    # Real password hash for "lab-password" computed as sha256(salt + password)
    # Format: sha256:<16-byte-salt-hex>:<sha256-hash-hex>
    # Salt: ad31a00094d25f7b5b3fa5ba2a4998db
    # Hash: ae3908b2ae4825fc884248f29385f4497ca9f3ff0c3d1416c6a216f3a400c4e1
    cat > "$UVB76_CONFIG" <<EOF
{
  "listen": {
    "addr": ":${UVB76_PORT}",
    "tls_cert_file": "",
    "tls_key_file": ""
  },
  "auth": {
    "username": "lab-admin",
    "password_sha256": "sha256:ad31a00094d25f7b5b3fa5ba2a4998db:ae3908b2ae4825fc884248f29385f4497ca9f3ff0c3d1416c6a216f3a400c4e1"
  },
  "scrape": {
    "interval_seconds": 60,
    "timeout_milliseconds": 5000
  },
  "latency": {
    "http": {
      "enabled": true,
      "interval_seconds": 5,
      "timeout_milliseconds": 3000,
      "window_seconds": 60,
      "retained_range_seconds": 300,
      "histogram_buckets_ms": [5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000],
      "recent_samples_max": 300
    },
    "icmp": {
      "enabled": false,
      "interval_seconds": 1,
      "timeout_seconds": 3,
      "window_seconds": 60,
      "retained_range_seconds": 300,
      "histogram_buckets_ms": [1, 5, 10, 25, 50, 100, 250, 500, 1000],
      "recent_samples_max": 300
    }
  },
  "diagnostics": {
    "enabled": true,
    "capture_on_spike": true,
    "timeout_ms": 2000,
    "cooldown_seconds": 5,
    "max_uncaptured_spikes": 50,
    "peers": [
      {
        "name": "tovarisch-lab",
        "base_url": "${DIAG_PEER_BASE_URL}",
        "targets": ["lab-tovarisch"]
      }
    ]
  },
  "targets": [
    {
      "id": "lab-tovarisch",
      "name": "Lab Tovarisch",
      "base_url": "${DIAG_PEER_BASE_URL}",
      "enabled": true
    }
  ]
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
    ip netns exec "$NS_TOVARISCH" "$binary" serve \
        --config "$TOVARISCH_CONFIG" \
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

    # Check /status.json
    local status_json
    status_json=$(ip netns exec "$NS_TOVARISCH" curl -s \
        "http://localhost:${TOVARISCH_PORT}/status.json" 2>/dev/null)
    if [[ -z "$status_json" ]]; then
        log_error "Failed to get /status.json from tovarisch"
        return 1
    fi
    echo "$status_json" > "$CURL_STATUS_BASELINE_FILE"

    # Check /status.json?include=network_diag
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

    log_info "tovarisch status endpoints verified"
    return 0
}

# Write result.json with valid JSON output using jq
write_result() {
    log_info "Writing result.json..."

    # Compute ok boolean properly
    local ok_val=false
    if [[ "$BASELINE_CAPTURE_OK" == true && "$DEFECT_OBSERVED" == true && "$RECOVERY_CAPTURE_OK" == true ]]; then
        ok_val=true
    fi

    # Use jq -n for valid JSON output
    local uvb76_pid_json="null"
    local tovarisch_pid_json="null"
    if [[ -n "$UVB76_PID" ]]; then
        uvb76_pid_json="$UVB76_PID"
    fi
    if [[ -n "$TOVARISCH_PID" ]]; then
        tovarisch_pid_json="$TOVARISCH_PID"
    fi

    jq -n \
        --arg artifact_dir "$LAB_DIR" \
        --arg defect_mode "$DEFECT_MODE" \
        --arg requested_path_baseline "$REQUESTED_PATH_BASELINE" \
        --arg requested_path_during_defect "$REQUESTED_PATH_DURING_DEFECT" \
        --arg requested_path_after_recovery "$REQUESTED_PATH_AFTER_RECOVERY" \
        --argjson uvb76_pid "$uvb76_pid_json" \
        --argjson tovarisch_pid "$tovarisch_pid_json" \
        --argjson ok "$ok_val" \
        --argjson baseline_capture_ok "$BASELINE_CAPTURE_OK" \
        --argjson defect_observed "$DEFECT_OBSERVED" \
        --argjson recovery_capture_ok "$RECOVERY_CAPTURE_OK" \
        '{
            ok: $ok,
            baseline_capture_ok: $baseline_capture_ok,
            defect_observed: $defect_observed,
            recovery_capture_ok: $recovery_capture_ok,
            artifact_dir: $artifact_dir,
            uvb76_pid: $uvb76_pid,
            tovarisch_pid: $tovarisch_pid,
            defect_mode: $defect_mode,
            requested_path_baseline: $requested_path_baseline,
            requested_path_during_defect: $requested_path_during_defect,
            requested_path_after_recovery: $requested_path_after_recovery
        }' > "$RESULT_FILE"

    log_info "Result written to $RESULT_FILE"

    # Validate the output is valid JSON
    if ! jq . "$RESULT_FILE" > /dev/null 2>&1; then
        log_error "result.json is not valid JSON!"
        return 1
    fi
}
