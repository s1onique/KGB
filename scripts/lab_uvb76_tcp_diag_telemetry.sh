#!/bin/bash
# Thin launcher for the TCP Diagnostic Telemetry Lab.
# Bash is restricted to: dependency check, artifact dir creation, binary invocation.
# All telemetry parsing, phase orchestration, and pass/fail derivation is handled by the Go lab.

set -e

# Check for Go toolchain
if ! command -v go &> /dev/null; then
    echo "Error: Go toolchain is required"
    echo "Install from: https://go.dev/doc/install"
    exit 1
fi

# Set artifact directory (can be overridden)
ARTIFACT_DIR="${ARTIFACT_DIR:-}"

# Build and run the Go lab
echo "=== Building TCP Diagnostic Telemetry Lab ==="
cd "$(dirname "$0")/.."

# Build the lab binary
LAB_BINARY="./uvb76-tcp-diag-telemetry-lab"
if [ ! -f "$LAB_BINARY" ] || [ "$(find "$LAB_BINARY" -newer uvb76/cmd/uvb76-tcp-diag-telemetry-lab/main.go 2>/dev/null | wc -l)" -eq 0 ]; then
    echo "Building lab binary..."
    make -C uvb76 lab-tcp-diag-telemetry-build
fi

# Run the lab
echo ""
echo "=== Running TCP Diagnostic Telemetry Lab ==="
if [ -n "$ARTIFACT_DIR" ]; then
    exec "$LAB_BINARY" --artifact-dir "$ARTIFACT_DIR"
else
    exec "$LAB_BINARY"
fi
