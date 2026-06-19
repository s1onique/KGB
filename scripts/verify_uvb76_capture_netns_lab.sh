#!/usr/bin/env bash
# verify_uvb76_capture_netns_lab.sh
# Verifier for the UVB-76 diagnostic capture netns lab.
#
# Runs the lab and verifies the result.json artifacts.

set -euo pipefail

echo "=== UVB-76 Capture Netns Lab Verifier ==="
echo ""

# Check for required tools
check_tools() {
    local missing=()
    for cmd in make jq; do
        if ! command -v "$cmd" &> /dev/null; then
            missing+=("$cmd")
        fi
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "ERROR: Missing required tools: ${missing[*]}"
        exit 1
    fi
}

# Run the lab
run_lab() {
    echo "[RUN] Executing lab-uvb76-capture-netns..."
    make lab-uvb76-capture-netns
}

# Find the result.json from the latest lab run
find_result() {
    local result_file
    result_file=$(find /tmp/kgb-uvb76-capture-netns-lab-* -name "result.json" -type f 2>/dev/null | sort -r | head -1)
    if [[ -z "$result_file" ]]; then
        echo "ERROR: result.json not found"
        echo "Lab may have failed or not produced artifacts"
        exit 1
    fi
    echo "$result_file"
}

# Verify result.json
verify_result() {
    local result_file="$1"
    echo ""
    echo "[VERIFY] Checking result.json..."
    echo "  File: $result_file"
    echo ""

    # Check if result.json is valid JSON
    if ! jq . "$result_file" > /dev/null 2>&1; then
        echo "ERROR: result.json is not valid JSON"
        exit 1
    fi

    # Extract values
    local ok
    local baseline_capture_ok
    local defect_observed
    local recovery_capture_ok
    local artifact_dir
    local defect_mode
    local requested_path_baseline
    local requested_path_during_defect
    local requested_path_after_recovery

    ok=$(jq -r '.ok' "$result_file")
    baseline_capture_ok=$(jq -r '.baseline_capture_ok' "$result_file")
    defect_observed=$(jq -r '.defect_observed' "$result_file")
    recovery_capture_ok=$(jq -r '.recovery_capture_ok' "$result_file")
    artifact_dir=$(jq -r '.artifact_dir' "$result_file")
    defect_mode=$(jq -r '.defect_mode' "$result_file")
    requested_path_baseline=$(jq -r '.requested_path_baseline' "$result_file")
    requested_path_during_defect=$(jq -r '.requested_path_during_defect' "$result_file")
    requested_path_after_recovery=$(jq -r '.requested_path_after_recovery' "$result_file")

    echo "[RESULT] Parsed values:"
    echo "  ok: $ok"
    echo "  baseline_capture_ok: $baseline_capture_ok"
    echo "  defect_observed: $defect_observed"
    echo "  recovery_capture_ok: $recovery_capture_ok"
    echo "  defect_mode: $defect_mode"
    echo "  artifact_dir: $artifact_dir"
    echo "  requested_path_baseline: $requested_path_baseline"
    echo "  requested_path_during_defect: $requested_path_during_defect"
    echo "  requested_path_after_recovery: $requested_path_after_recovery"
    echo ""

    # Validation checks - track errors
    local errors=0

    # Check baseline capture is OK (REQUIRED)
    if [[ "$baseline_capture_ok" != "true" ]]; then
        echo "[FAIL] Baseline capture did not succeed - REQUIRED"
        errors=$((errors + 1))
    else
        echo "[PASS] Baseline capture succeeded"
    fi

    # Check recovery capture is OK (REQUIRED)
    if [[ "$recovery_capture_ok" != "true" ]]; then
        echo "[FAIL] Recovery capture did not succeed - REQUIRED"
        errors=$((errors + 1))
    else
        echo "[PASS] Recovery capture succeeded"
    fi

    # Defect observed is REQUIRED for this lab
    # This lab's purpose is to prove defect causes capture failure
    if [[ "$defect_observed" != "true" ]]; then
        echo "[FAIL] Defect was NOT observed - REQUIRED for proof"
        echo "       The lab must demonstrate that network impairment"
        echo "       causes diagnostic capture to fail/timeout"
        errors=$((errors + 1))
    else
        echo "[PASS] Defect was observed (expected behavior)"
    fi

    # Check requested paths are correct
    if [[ "$requested_path_baseline" == "/status.json?include=network_diag" ]]; then
        echo "[PASS] Baseline requested path is correct"
    else
        echo "[WARN] Baseline requested path may be incorrect: $requested_path_baseline"
    fi

    if [[ "$requested_path_during_defect" == "/status.json?include=network_diag" ]]; then
        echo "[PASS] During-defect requested path is correct"
    else
        echo "[WARN] During-defect requested path may be incorrect: $requested_path_during_defect"
    fi

    if [[ "$requested_path_after_recovery" == "/status.json?include=network_diag" ]]; then
        echo "[PASS] After-recovery requested path is correct"
    else
        echo "[WARN] After-recovery requested path may be incorrect: $requested_path_after_recovery"
    fi

    # Check artifact directory exists
    if [[ -d "$artifact_dir" ]]; then
        echo "[PASS] Artifact directory exists: $artifact_dir"
        echo ""
        echo "[ARTIFACTS] Key files:"
        ls -la "$artifact_dir"/*.txt "$artifact_dir"/*.json 2>/dev/null | head -20 || true
    else
        echo "[FAIL] Artifact directory not found: $artifact_dir"
        errors=$((errors + 1))
    fi

    echo ""

    # Final verdict
    if [[ $errors -eq 0 ]]; then
        echo "=== VERIFICATION PASSED ==="
        echo "Artifact directory: $artifact_dir"
        return 0
    else
        echo "=== VERIFICATION FAILED ($errors errors) ==="
        return 1
    fi
}

# Main
main() {
    check_tools

    # Run the lab (may be skipped if already ran)
    # Uncomment to always run:
    # run_lab

    # For verification-only mode, find and verify the latest result
    local result_file
    result_file=$(find_result)

    if ! verify_result "$result_file"; then
        exit 1
    fi
}

# If called with --run flag, run the lab first
if [[ "${1:-}" == "--run" ]]; then
    check_tools
    run_lab
    result_file=$(find_result)
    verify_result "$result_file"
else
    main
fi
