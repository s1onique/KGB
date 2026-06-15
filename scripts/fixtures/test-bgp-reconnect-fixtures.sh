#!/bin/bash
# test-bgp-reconnect-fixtures.sh — Verify BGP reconnect artifact fixtures
#
# Tests the verify_bgp_reconnect_artifact.sh script against known fixtures:
# - passing fixture: BGP Established + routes imported (should PASS)
# - failing fixture: BGP Established + 0 imported routes (should FAIL)
#
# This proves the verifier correctly catches the false-green condition.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY_SCRIPT="$SCRIPT_DIR/../verify_bgp_reconnect_artifact.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0

test_fixture() {
    local fixture_name="$1"
    local fixture_dir="$SCRIPT_DIR/$fixture_name"
    local expected_result="$2"  # "pass" or "fail"

    echo ""
    echo "=== Testing fixture: $fixture_name (expected: $expected_result) ==="

    if [[ ! -d "$fixture_dir" ]]; then
        echo "${RED}[ERROR]${NC} Fixture directory not found: $fixture_dir"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return 1
    fi

    local artifact_dir="$fixture_dir/artifacts"
    if [[ ! -d "$artifact_dir" ]]; then
        echo "${RED}[ERROR]${NC} Artifacts directory not found: $artifact_dir"
        FAIL_COUNT=$((FAIL_COUNT + 1))
        return 1
    fi

    # Run the verifier
    local output
    local exit_code=0
    output=$("$VERIFY_SCRIPT" "$artifact_dir" 2>&1) || exit_code=$?

    echo "$output"
    echo ""

    if [[ "$expected_result" == "pass" ]]; then
        if [[ $exit_code -eq 0 ]]; then
            echo "${GREEN}[PASS]${NC} $fixture_name: verifier passed as expected"
            PASS_COUNT=$((PASS_COUNT + 1))
            return 0
        else
            echo "${RED}[FAIL]${NC} $fixture_name: verifier failed but expected pass"
            FAIL_COUNT=$((FAIL_COUNT + 1))
            return 1
        fi
    else
        if [[ $exit_code -ne 0 ]]; then
            echo "${GREEN}[PASS]${NC} $fixture_name: verifier failed as expected (false-green caught)"
            PASS_COUNT=$((PASS_COUNT + 1))
            return 0
        else
            echo "${RED}[FAIL]${NC} $fixture_name: verifier passed but expected fail (false-green NOT caught)"
            FAIL_COUNT=$((FAIL_COUNT + 1))
            return 1
        fi
    fi
}

main() {
    echo "=== BGP Reconnect Artifact Fixture Tests ==="

    # Test passing fixture (should pass)
    test_fixture "bgp-reconnect-passing-fixture" "pass"

    # Test failing fixture (should fail - catching false-green)
    test_fixture "bgp-reconnect-failing-fixture" "fail"

    echo ""
    echo "=== Fixture Test Summary ==="
    echo "Passed: $PASS_COUNT"
    echo "Failed: $FAIL_COUNT"

    if [[ $FAIL_COUNT -eq 0 ]]; then
        echo "${GREEN}All fixture tests passed!${NC}"
        return 0
    else
        echo "${RED}Some fixture tests failed!${NC}"
        return 1
    fi
}

main "$@"
