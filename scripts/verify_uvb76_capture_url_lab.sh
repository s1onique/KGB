#!/usr/bin/env bash
# verify_uvb76_capture_url_lab.sh
# Hermetic regression test for diagnostic capture URL construction.
# Verifies that diagnostic capture uses canonical /status.json?include=network_diag.

set -euo pipefail

echo "=== UVB-76 Capture URL Lab ==="
echo "Testing: Diagnostic capture uses canonical tovarisch status endpoint"
echo ""

# Ensure web assets are built (required dependency)
if [[ -d "uvb76/web/dist" ]]; then
    echo "[skip] uvb76-web-build (already built)"
else
    echo "[build] uvb76-web-build..."
    (cd uvb76/web && npm ci && npm run build) || {
        echo "ERROR: uvb76-web-build failed"
        exit 1
    }
fi

# Run the capture URL lab tests
echo ""
echo "[test] Running capture URL lab tests..."
cd uvb76

# Run the specific lab tests with verbose output
go test -v -run "TestDiagnosticCapture|TestCaptureURLLab|TestDiagPeerStatusURL" ./server/... ./config/... ./diag/...

# Also run the config URL tests
echo ""
echo "[test] Running config URL helper tests..."
go test -v -run "TestDiagPeerStatusURL" ./config/...

echo ""
echo "=== All capture URL lab tests passed ==="
