#!/usr/bin/env bash
# linux_smoke_test.sh — Linux-only runtime path smoke tests
#
# Exercises Linux-specific runtime paths that cannot be verified on macOS:
#   - /proc/self/status RSS read (runtime.rss_kib in status --json)
#   - shell-level sysfs readability probe; does not exercise Zig sysfs reader
#
# This script is Linux-only and will exit early on non-Linux platforms.
# Sysfs checks are skipped with an explicit warning when counters are not readable.
#
# Usage:
#   ./scripts/linux_smoke_test.sh [tovarisch_binary_path]
#
# Exit codes:
#   0 = all exercised paths passed or gracefully skipped
#   1 = assertion failure (rss_kib null when expected non-null, etc.)

set -euo pipefail

# Detect if running on Linux
is_linux() {
    [[ "$(uname -s)" == "Linux" ]]
}

# Check if we have the tovarisch binary
BINARY_PATH="${1:-}"
if [[ -z "$BINARY_PATH" ]]; then
    # Default to the Zig build output
    BINARY_PATH="./tovarisch/zig-out/bin/tovarisch"
fi

# Verify binary exists
if [[ ! -x "$BINARY_PATH" ]]; then
    echo "[linux-smoke] ERROR: binary not found or not executable: $BINARY_PATH" >&2
    exit 1
fi

echo "[linux-smoke] Starting Linux runtime path smoke tests..."

# Track exit status
EXIT_CODE=0
TESTS_RUN=0
TESTS_SKIPPED=0
TESTS_PASSED=0

# ============================================================================
# Test 1: /proc/self/status RSS read
# ============================================================================
test_rss_read() {
    echo ""
    echo "=== Test: /proc/self/status RSS read ==="
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    # Run tovarisch status --json and extract runtime.rss_kib
    local status_json
    status_json=$("$BINARY_PATH" status --json 2>&1) || true
    
    # Check if output is valid JSON
    if ! echo "$status_json" | jq empty >/dev/null 2>&1; then
        echo "[linux-smoke] FAIL: status --json output is not valid JSON" >&2
        echo "Output: $status_json" >&2
        EXIT_CODE=1
        return
    fi
    
    # Extract runtime.rss_kib value
    local rss_kib
    rss_kib=$(echo "$status_json" | jq -r '.runtime.rss_kib')
    
    # Check if rss_kib is null
    if [[ "$rss_kib" == "null" ]]; then
        echo "[linux-smoke] FAIL: runtime.rss_kib is null (expected non-null on Linux)" >&2
        EXIT_CODE=1
        return
    fi
    
    # Verify it's a valid non-negative integer
    if ! [[ "$rss_kib" =~ ^[0-9]+$ ]]; then
        echo "[linux-smoke] FAIL: runtime.rss_kib is not a valid integer: '$rss_kib'" >&2
        EXIT_CODE=1
        return
    fi
    
    echo "[linux-smoke] PASS: runtime.rss_kib = $rss_kib KiB"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

# ============================================================================
# Probe 2: sysfs interface statistics readability
# ============================================================================
test_sysfs_stats() {
    echo ""
    echo "=== Probe: sysfs interface statistics readability ==="
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    # Check if /sys/class/net exists and is readable
    if [[ ! -d "/sys/class/net" ]]; then
        echo "[linux-smoke] SKIP: /sys/class/net does not exist (not a network namespace container?)"
        echo "[linux-smoke] WARN: sysfs smoke test skipped — not a false pass"
        TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
        return
    fi
    
    # Try to read counters from the first available interface
    # We don't care which interface, just that we can read the counters
    
    # Find an interface with statistics
    local interfaces=()
    for iface_dir in /sys/class/net/*; do
        if [[ -d "$iface_dir/statistics" ]]; then
            interfaces+=("$iface_dir")
        fi
    done
    
    if [[ ${#interfaces[@]} -eq 0 ]]; then
        echo "[linux-smoke] SKIP: no interfaces with /statistics directory found"
        echo "[linux-smoke] WARN: sysfs smoke test skipped — not a false pass"
        TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
        return
    fi
    
    # Test reading counters from the first interface
    local test_iface
    test_iface=$(basename "${interfaces[0]}")
    local stats_dir="${interfaces[0]}/statistics"
    
    echo "[linux-smoke] Testing interface: $test_iface"
    
    # Try to read each counter file
    local rx_bytes="" tx_bytes="" rx_packets="" tx_packets=""
    local FAILED=0
    
    for counter in rx_bytes tx_bytes rx_packets tx_packets; do
        local counter_path="${stats_dir}/${counter}"
        if [[ -r "$counter_path" ]]; then
            local value
            value=$(cat "$counter_path" 2>/dev/null || echo "UNREADABLE")
            if [[ "$value" == "UNREADABLE" ]]; then
                echo "[linux-smoke] SKIP: cannot read $counter_path (permission denied in container?)"
                echo "[linux-smoke] WARN: sysfs smoke test skipped — not a false pass"
                TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
                return
            fi
            # Verify it's a valid integer
            if ! [[ "$value" =~ ^[0-9]+$ ]]; then
                echo "[linux-smoke] FAIL: $counter contains non-integer value: '$value'" >&2
                FAILED=1
            fi
        else
            echo "[linux-smoke] SKIP: $counter_path not readable"
            echo "[linux-smoke] WARN: sysfs smoke test skipped — not a false pass"
            TESTS_SKIPPED=$((TESTS_SKIPPED + 1))
            return
        fi
    done
    
    if [[ $FAILED -eq 1 ]]; then
        EXIT_CODE=1
        return
    fi
    
    echo "[linux-smoke] PASS: all sysfs counters readable for $test_iface"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

# ============================================================================
# Main
# ============================================================================
main() {
    if ! is_linux; then
        echo "[linux-smoke] SKIP: this script is Linux-only (running on $(uname -s))"
        exit 0
    fi
    
    echo "[linux-smoke] Running on Linux $(uname -r)"
    echo "[linux-smoke] Binary: $BINARY_PATH"
    
    # Run RSS test (always runs on Linux)
    test_rss_read
    
    # Run sysfs test (conditional)
    test_sysfs_stats
    
    echo ""
    echo "=== Linux Smoke Test Summary ==="
    echo "Tests run:     $TESTS_RUN"
    echo "Tests passed:  $TESTS_PASSED"
    echo "Tests skipped: $TESTS_SKIPPED"
    
    if [[ $TESTS_SKIPPED -gt 0 ]]; then
        echo "[linux-smoke] WARN: $TESTS_SKIPPED test(s) skipped with explicit warning"
    fi
    
    if [[ $EXIT_CODE -eq 0 ]]; then
        echo "[linux-smoke] All tests passed"
    else
        echo "[linux-smoke] FAIL: some tests failed"
    fi
    
    exit $EXIT_CODE
}

main "$@"
