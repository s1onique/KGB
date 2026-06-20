#!/bin/bash
# lab_uvb76_targets_crash.sh — Thin launcher for UVB-76 Targets Crash Lab
#
# Responsibilities:
#   - Find repo root
#   - Build Go lab command
#   - Run the Go lab command
#   - Capture artifact dir
#   - Call the verifier
#   - Print artifact location
#
# Primary execution: GitHub Actions (workflow_dispatch) or `make lab-uvb76-targets-crash`
# NOT part of make gate

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
UVB76_LAB_CMD="${REPO_ROOT}/uvb76/uvb76-targets-crash-lab"
UVB76_LAB_VERIFY="${REPO_ROOT}/uvb76/uvb76-targets-crash-verify"
ARTIFACT_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

# Main
main() {
    log_info "=== UVB-76 Targets Crash Lab ==="
    log_info "Repo root: ${REPO_ROOT}"
    log_info ""

    # Build Go lab command if not present
    if [[ ! -x "$UVB76_LAB_CMD" ]]; then
        log_info "Building Go lab command..."
        (cd "${REPO_ROOT}/uvb76" && go build -o uvb76-targets-crash-lab ./cmd/uvb76-targets-crash-lab)
    fi

    # Build Go verifier if not present
    if [[ ! -x "$UVB76_LAB_VERIFY" ]]; then
        log_info "Building Go verifier..."
        (cd "${REPO_ROOT}/uvb76" && go build -o uvb76-targets-crash-verify ./cmd/uvb76-targets-crash-verify)
    fi

    # Run the Go lab with explicit binary path
    log_info "Running Go lab..."
    export UVB76_BINARY="${REPO_ROOT}/uvb76/uvb76"
    
    # Optional: run with shorter duration for quick test
    if [[ "${1:-}" == "--short" ]]; then
        log_info "Running in SHORT mode (10 seconds, 4 workers)..."
        export UVB76_TARGETS_CRASH_LAB_DURATION=10
        export UVB76_TARGETS_CRASH_LAB_WORKERS=4
    fi
    
    # The lab prints the artifact dir to stdout at the end
    LAB_OUTPUT=$("$UVB76_LAB_CMD" 2>&1) || true
    echo "$LAB_OUTPUT"
    
    # Extract artifact dir from lab output using exact ARTIFACT_DIR= prefix
    ARTIFACT_DIR=$(echo "$LAB_OUTPUT" | sed -n 's/.*ARTIFACT_DIR=\(\/tmp\/kgb-uvb76-targets-crash-[^ ]*\).*/\1/p' | head -1)
    
    if [[ -z "$ARTIFACT_DIR" ]]; then
        log_error "Could not determine artifact directory from lab output"
        log_error "Lab output:"
        echo "$LAB_OUTPUT"
        exit 1
    fi
    log_info "Artifact directory: ${ARTIFACT_DIR}"

    # Verify the artifact dir exists and has summary.json
    if [[ ! -d "$ARTIFACT_DIR" ]]; then
        log_error "Artifact directory does not exist: ${ARTIFACT_DIR}"
        exit 1
    fi
    if [[ ! -f "${ARTIFACT_DIR}/summary.json" ]]; then
        log_error "summary.json not found in artifact directory"
        exit 1
    fi

    # Run verifier
    log_info "Running artifact verifier..."
    if ! "$UVB76_LAB_VERIFY" "$ARTIFACT_DIR"; then
        log_error "Artifact verification failed"
        exit 1
    fi

    log_info ""
    log_info "=== Lab Complete ==="
    log_info "Artifact directory: ${ARTIFACT_DIR}"
    log_info "summary.json:"
    cat "${ARTIFACT_DIR}/summary.json" 2>/dev/null || echo "(not found)"

    exit 0
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
