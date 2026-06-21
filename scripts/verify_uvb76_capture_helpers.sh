#!/bin/bash
# ShellRole: gate-delegation
# ShellJustification: Thin wrapper that delegates to standalone shell test scripts.
#   No JSON parsing, no business logic. Just orchestrates existing test scripts.
# MigrationPlan: Tests are now duplicated in Python (spec mirror); when shell helper
#   implementations are fully ported, this wrapper can be removed.
#
# verify_uvb76_capture_helpers.sh — Thin wrapper for shell helper self-tests
#
# This is a THIN WRAPPER that delegates to standalone test scripts.
# The actual helper functions are tested via their standalone test scripts.
#
# Coverage:
#   - Row normalization helpers (via verify_uvb76_row_normalization_tests.sh)
#   - Captured row assertion helpers (via verify_uvb76_captured_row_assertion_tests.sh)
#   - Network diag extraction (via verify_uvb76_extract_network_diag_tests.sh)
#
# This wrapper exists ONLY to preserve the quality gate interface.
# The Python verifier tests standalone function behavior as a specification mirror.
#
# Usage:
#   ./verify_uvb76_capture_helpers.sh --self-test
#   python3 scripts/verify_uvb76_capture_helpers.py --self-test  # spec mirror

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

main() {
    local do_self_test=false
    local arg
    
    for arg in "$@"; do
        case "$arg" in
            --self-test) do_self_test=true ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo "Options:"
                echo "  --self-test   Run delegated shell helper tests"
                echo "  -h, --help   Show this help"
                exit 0
                ;;
            *) echo "Unknown: $arg"; exit 1 ;;
        esac
    done
    
    if [[ "$do_self_test" == "true" ]]; then
        echo "=== UVB-76 Capture Helper Shell Tests ==="
        echo ""
        
        local errors=0
        
        # Test row normalization helpers
        echo "Testing row normalization helpers..."
        if bash "${SCRIPT_DIR}/verify_uvb76_row_normalization_tests.sh" --self-test; then
            echo "[PASS] Row normalization tests"
        else
            echo "[FAIL] Row normalization tests" >&2
            errors=$((errors + 1))
        fi
        
        echo ""
        
        # Test captured row assertion helpers
        echo "Testing captured row assertion helpers..."
        if bash "${SCRIPT_DIR}/verify_uvb76_captured_row_assertion_tests.sh" --self-test; then
            echo "[PASS] Captured row assertion tests"
        else
            echo "[FAIL] Captured row assertion tests" >&2
            errors=$((errors + 1))
        fi
        
        echo ""
        
        # Test network diag extraction helpers
        echo "Testing network diag extraction helpers..."
        if bash "${SCRIPT_DIR}/verify_uvb76_extract_network_diag_tests.sh" --self-test; then
            echo "[PASS] Network diag extraction tests"
        else
            echo "[FAIL] Network diag extraction tests" >&2
            errors=$((errors + 1))
        fi
        
        echo ""
        echo "=== Shell Helper Tests Summary ==="
        
        if [[ $errors -gt 0 ]]; then
            echo "Failed: $errors"
            return 1
        else
            echo "All shell helper tests passed"
            return 0
        fi
    else
        echo "Usage: $0 --self-test"
        echo "Use python3 scripts/verify_uvb76_capture_helpers.py for specification tests"
        return 0
    fi
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
