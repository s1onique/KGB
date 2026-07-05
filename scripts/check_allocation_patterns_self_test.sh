#!/usr/bin/env bash
# ShellJustification: Self-test wrapper; no polling loops or risky patterns
# ShellRole: Self-test suite for allocation pattern gate
# MigrationPlan: Synthetic test generation is bash-appropriate
# check_allocation_patterns_self_test.sh — Self-test suite for allocation pattern gate
#
# This file contains the self-test functionality extracted from check_allocation_patterns.sh
# to keep the main scanner under the 450-line LLM-friendliness limit.

set -euo pipefail

# Color codes (must match main script)
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Self-test results ( globals to track results)
SELF_TEST_PASSED=0
SELF_TEST_FAILED=0

# Self-test function
run_self_test() {
    local test_name="$1"
    local expected_exit="$2"
    local test_script="$3"
    
    echo -e "${BLUE}[SELF-TEST]${NC} Running: $test_name"
    
    local actual_exit
    actual_exit=$(eval "$test_script" 2>&1) && actual_exit=0 || actual_exit=$?
    
    if [[ "$actual_exit" -eq "$expected_exit" ]]; then
        echo -e "${GREEN}[PASS]${NC} $test_name (exit $actual_exit)"
        SELF_TEST_PASSED=$((SELF_TEST_PASSED + 1))
    else
        echo -e "${RED}[FAIL]${NC} $test_name (expected $expected_exit, got $actual_exit)"
        echo "Output: $actual_exit"
        SELF_TEST_FAILED=$((SELF_TEST_FAILED + 1))
    fi
    echo ""
}

# Run all self-tests
run_all_self_tests() {
    echo "=== Self-Test Mode ==="
    echo ""
    
    # Discover repo root relative to this script
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
    
    # Test 1: Clean tree should pass (exit 0)
    run_self_test "Clean tree passes" 0 \
        "cd '$REPO_ROOT' && bash scripts/check_allocation_patterns.sh --scan-root tovarisch/src 2>&1 | tail -20"
    
    # Test 2: Synthetic HIGH should fail with exit 1
    # We create a temp file with unregistered page_allocator
    local temp_test_dir
    temp_test_dir=$(mktemp -d)
    mkdir -p "$temp_test_dir/test_module"
    
    cat > "$temp_test_dir/test_module/synthetic.zig" << 'EOF'
// Synthetic test file for allocation pattern gate
const std = @import("std");

pub fn process() void {
    // This is an unregistered page_allocator - should fail gate
    var allocator = std.heap.page_allocator;
    _ = &allocator;
}
EOF

    run_self_test "Synthetic HIGH (unregistered page_allocator) fails with exit 1" 1 \
        "cd '$REPO_ROOT' && bash scripts/check_allocation_patterns.sh --scan-root '$temp_test_dir' 2>&1"
    
    rm -rf "$temp_test_dir"
    
    # Test 3: Accepted registered pattern should pass
    temp_test_dir=$(mktemp -d)
    mkdir -p "$temp_test_dir/test_module"
    
    cat > "$temp_test_dir/test_module/synthetic_accepted.zig" << 'EOF'
// Synthetic test file for allocation pattern gate
const std = @import("std");

// MemoryOwnership: page_allocator for synthetic test
// Rationale: This is an accepted test pattern with paired free
// MaxSize: 1024 bytes
// Deinit: page_allocator.free() called immediately after
// FailureMode: Test fails, no production impact
// TestCoverage: Synthetic self-test
pub fn process() void {
    var allocator = std.heap.page_allocator;
    var buf = allocator.alloc(u8, 64) catch return;
    defer allocator.free(buf);
    _ = &allocator;
}
EOF

    run_self_test "Accepted registered pattern passes (exit 0)" 0 \
        "cd '$REPO_ROOT' && bash scripts/check_allocation_patterns.sh --scan-root '$temp_test_dir' 2>&1"
    
    rm -rf "$temp_test_dir"
    
    # Test 4: DEFERRED pattern should pass (report-only, exit 0)
    temp_test_dir=$(mktemp -d)
    mkdir -p "$temp_test_dir/serve_integration"
    
    cat > "$temp_test_dir/serve_integration/synthetic.zig" << 'EOF'
// Synthetic test file simulating serve_integration deferred pattern
const std = @import("std");

pub fn init() void {
    // Simulating the deferred page_allocator in serve_integration
    var raw = std.heap.page_allocator.alloc(u8, 1024) catch return;
    _ = raw;
}
EOF

    run_self_test "Synthetic DEFERRED pattern passes (exit 0, report-only)" 0 \
        "cd '$REPO_ROOT' && bash scripts/check_allocation_patterns.sh --scan-root '$temp_test_dir' 2>&1"
    
    rm -rf "$temp_test_dir"
    
    # Test 5: --enforce-medium with MEDIUM should fail with exit 2
    temp_test_dir=$(mktemp -d)
    mkdir -p "$temp_test_dir/test_module"
    
    cat > "$temp_test_dir/test_module/synthetic_medium.zig" << 'EOF'
// Synthetic test file for allocation pattern gate
const std = @import("std");

pub fn process() void {
    // This is an unregistered GeneralPurposeAllocator - MEDIUM risk
    var gpa = std.heap.GeneralPurposeAllocator(.{}){};
    _ = &gpa;
}
EOF

    run_self_test "RISKY-MEDIUM with --enforce-medium fails with exit 2" 2 \
        "cd '$REPO_ROOT' && bash scripts/check_allocation_patterns.sh --enforce-medium --scan-root '$temp_test_dir' 2>&1"
    
    rm -rf "$temp_test_dir"
    
    # Summary
    echo "=== Self-Test Summary ==="
    echo "Passed: $SELF_TEST_PASSED"
    echo "Failed: $SELF_TEST_FAILED"
    echo ""
    
    if [[ "$SELF_TEST_FAILED" -gt 0 ]]; then
        echo -e "${RED}[SELF-TEST FAILED]${NC} $SELF_TEST_FAILED test(s) failed"
        exit 1
    else
        echo -e "${GREEN}[SELF-TEST PASSED]${NC} All $SELF_TEST_PASSED test(s) passed"
        exit 0
    fi
}

# Run self-tests when executed directly
run_all_self_tests
