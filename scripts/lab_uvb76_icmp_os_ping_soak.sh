#!/bin/bash
# Lab launcher for UVB-76 ICMP OS Ping Soak
#
# This is a THIN SHELL LAUNCHER only. All lab logic is in Go.
# Per Factory doctrine: shell is an acceptable wrapper, not an acceptable brain.
#
# Usage:
#   ./lab_uvb76_icmp_os_ping_soak.sh          # short soak (2 minutes)
#   SOAK_DURATION_SECONDS=7200 ./lab_...     # long soak (2 hours)
#   SOAK_DURATION_SECONDS=60 ./lab_...       # quick CI test (1 minute)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LAB_DIR="$REPO_ROOT/uvb76/cmd/uvb76-icmp-os-ping-soak"

# Default duration
DURATION="${SOAK_DURATION_SECONDS:-120}"

echo "=== ICMP OS Ping Soak Lab ==="
echo "Duration: ${DURATION}s"
echo ""

# Build the lab binary
echo "Building lab..."
cd "$LAB_DIR"
go build -o uvb76-icmp-os-ping-soak . 2>/dev/null || go build -o uvb76-icmp-os-ping-soak .

# Run the lab with duration override
SOAK_DURATION_SECONDS="$DURATION" ./uvb76-icmp-os-ping-soak
