#!/bin/bash
# lab_uvb76_latency_crash.sh — Thin launcher for UVB-76 Latency Crash Lab
#
# Responsibilities:
#   - Find repo root
#   - Run the Go lab command
#   - Capture artifact dir
#   - Call the verifier
#   - Print artifact location
#
# Primary execution: GitHub Actions (workflow_dispatch) or `make lab-uvb76-latency-crash`
# NOT part of make gate

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
UVB76_LAB_CMD="${REPO_ROOT}/uvb76/uvb76-latency-crash-lab"
UVB76_LAB_VERIFY="${REPO_ROOT}/uvb76/uvb76-latency-crash-verify"
ARTIFACT_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# Find the most recent artifact directory by mtime
find_latest_artifact() {
    find /tmp -maxdepth 1 -type d -name 'kgb-uvb76-latency-crash-*' \
        -printf '%T@ %p\n' 2>/dev/null | \
        sort -nr | awk 'NR == 1 { print $2 }'
}

# Main
main() {
    log_info "=== UVB-76 Latency Crash Lab ==="
    log_info "Repo root: ${REPO_ROOT}"
    log_info ""

    # Build Go lab command if not present
    if [[ ! -x "$UVB76_LAB_CMD" ]]; then
        log_info "Building Go lab command..."
        (cd "${REPO_ROOT}/uvb76" && go build -o uvb76-latency-crash-lab ./cmd/uvb76-latency-crash-lab)
    fi

    # Build Go verifier if not present
    if [[ ! -x "$UVB76_LAB_VERIFY" ]]; then
        log_info "Building Go verifier..."
        (cd "${REPO_ROOT}/uvb76" && go build -o uvb76-latency-crash-verify ./cmd/uvb76-latency-crash-verify)
    fi

    # Run the Go lab with explicit binary path
    log_info "Running Go lab..."
    export UVB76_BINARY="${REPO_ROOT}/uvb76/uvb76"
    if ! "$UVB76_LAB_CMD"; then
        log_error "Lab execution failed"
        exit 1
    fi

    # Find artifact directory
    ARTIFACT_DIR=$(find_latest_artifact)
    if [[ -z "$ARTIFACT_DIR" ]]; then
        log_error "Could not find artifact directory"
        exit 1
    fi
    log_info "Artifact directory: ${ARTIFACT_DIR}"

    # Run verifier
    log_info "Running artifact verifier..."
    if ! "$UVB76_LAB_VERIFY" "$ARTIFACT_DIR"; then
        log_error "Artifact verification failed"
        exit 1
    fi

    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact directory: ${ARTIFACT_DIR}"
    log_info "result.json:"
    cat "${ARTIFACT_DIR}/result.json" 2>/dev/null || echo "(not found)"

    exit 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
