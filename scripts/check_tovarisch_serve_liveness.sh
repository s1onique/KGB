#!/usr/bin/env bash
# Regression test: tovarisch serve must not exit immediately after startup.
# This verifies the daemon loop fix - the process should stay alive until
# interrupted by a signal.

set -euo pipefail

BIN="${1:-./tovarisch/zig-out/bin/tovarisch}"

if [[ ! -x "$BIN" ]]; then
    echo "[FAIL] Binary not found or not executable: $BIN"
    echo "[FAIL] Run 'make tovarisch-build' first"
    exit 1
fi

echo "=== Serve Liveness Regression ==="
echo "Testing: $BIN serve"

# Start serve in background
"$BIN" serve > /tmp/tovarisch-serve.log 2>&1 &
pid="$!"

# Cleanup function
cleanup() {
    if kill -0 "$pid" >/dev/null 2>&1; then
        kill "$pid" >/dev/null 2>&1 || true
        wait "$pid" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

# Wait for startup
sleep 0.5

# Check if process is still alive
if ! kill -0 "$pid" >/dev/null 2>&1; then
    echo "[FAIL] tovarisch serve exited immediately"
    echo "--- Log output ---"
    cat /tmp/tovarisch-serve.log || true
    echo "--- End log ---"
    exit 1
fi

echo "[PASS] tovarisch serve stays alive after startup"

# Verify it responds to HTTP requests
if command -v curl >/dev/null 2>&1; then
    if curl -fsS --max-time 2 http://127.0.0.1:8317/status >/dev/null 2>&1; then
        echo "[PASS] tovarisch serve responds to HTTP requests"
    else
        echo "[WARN] Could not verify HTTP response (curl failed)"
    fi
else
    echo "[SKIP] curl not available for HTTP verification"
fi

echo "=== Serve Liveness Regression: PASSED ==="
