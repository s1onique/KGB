#!/usr/bin/env bash
# check_memory_ownership.sh — Memory ownership hygiene gate for status/request paths
#
# Scans Zig source files for risky allocation/ownership patterns that can cause
# RSS leaks and unsafe memory management in status/request rendering paths.
#
# RISKY PATTERNS (caught by this gate):
#   - std.heap.page_allocator       — leaks page-backed memory per call
#   - std.fmt.allocPrint            — allocation without explicit ownership
#   - ArenaAllocator.init           — unbounded growth potential
#   - toOwnedSlice                  — ownership transfer patterns
#   - .dupe(                        — heap allocation patterns
#   - ArrayList.init(               — dynamic collection initialization
#
# ALLOWED PATTERNS:
#   - Only when MemoryOwnership annotation is present nearby
#   - Test files (*_tests.zig) are exempt
#   - Fixtures (*/fixtures/*.zig) are exempt EXCEPT in --self-test mode
#
# MemoryOwnership annotation rules:
#   - Must contain "MemoryOwnership:" prefix
#   - Must NOT justify leaks as "bounded by requests", "per emit cycle", "leaked but acceptable"
#   - Must describe WHY this pattern is safe for the specific use case
#
# This gate prevents reintroducing the page_allocator RSS leak from bfd/status.zig.

set -euo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Configuration
# Scan both files and directories:
# - tovarisch/src/status.zig - main status rendering file
# - tovarisch/src/status/ - status module directory
# - tovarisch/src/http/ - HTTP request/status paths
SCAN_PATHS=(
    "tovarisch/src/status.zig"
    "tovarisch/src/status"
    "tovarisch/src/http"
)

# NOTE: Patterns use ERE syntax (grep -E). Escape dots with [...] and parens with [(]).
RISKY_PATTERNS=(
    "std[.]heap[.]page_allocator"
    "std[.]fmt[.]allocPrint"
    "ArenaAllocator[.]init"
    "[.]toOwnedSlice[(]"
    "[.]dupe[(]"
    "ArrayList[.]init[(]"
)

# Patterns that indicate UNSAFE MemoryOwnership annotations
# These phrases indicate the annotation is justifying unbounded leaks
UNSAFE_ANNOTATION_PATTERNS=(
    "leaked per emit"
    "bounded by request"
    "bounded by request count"
    "leaked but acceptable"
    "leaked but bounded"
    "daemon-lifetime"
    "per emit cycle"
    "per request"
)

# Annotations that allow intentional risky patterns
ALLOWED_ANNOTATION="MemoryOwnership:"

# Counters
total_files=0
failed_files=0
passed=0

# Self-test mode flag
SELF_TEST=0
SELF_TEST_FAILED=0
SELF_TEST_PASSED=0

echo "=== Memory Ownership Hygiene Gate ==="
echo ""

# Check if there are any risky patterns
scan_file() {
    local file="$1"
    local filename
    filename=$(basename "$file")
    
    # In normal mode: skip test files and fixtures
    # In self-test mode: only scan fixtures (bad/good sentinels)
    if [[ "$SELF_TEST" -eq 0 ]]; then
        if [[ "$filename" == *_tests.zig ]]; then
            echo "  [SKIP] $file (test file)"
            return 0
        fi
        
        if [[ "$file" == */fixtures/* ]]; then
            echo "  [SKIP] $file (fixture)"
            return 0
        fi
    else
        # Self-test mode: only scan fixtures
        if [[ "$file" != */fixtures/* ]]; then
            echo "  [SKIP] $file (not a fixture)"
            return 0
        fi
    fi
    
    total_files=$((total_files + 1))
    local found_issues=0
    
    for pattern in "${RISKY_PATTERNS[@]}"; do
        # Check for pattern without MemoryOwnership annotation nearby
        if grep -En "$pattern" "$file" >/dev/null 2>&1; then
            # Get line numbers with the pattern
            local lines
            lines=$(grep -En "$pattern" "$file" | cut -d: -f1)
            
            for line_num in $lines; do
                # Skip lines that are deallocations (free calls)
                local line_content
                line_content=$(sed -n "${line_num}p" "$file")
                if echo "$line_content" | grep -q 'page_allocator\.free'; then
                    continue
                fi
                
                # Check surrounding context (5 lines before, 5 lines after)
                local context_start=$((line_num > 5 ? line_num - 5 : 1))
                local context_end=$((line_num + 5))
                
                local context
                context=$(sed -n "${context_start},${context_end}p" "$file")
                
                # Check if MemoryOwnership annotation exists in context
                if echo "$context" | grep -q "$ALLOWED_ANNOTATION"; then
                    # Verify annotation does NOT contain unsafe patterns
                    local unsafe_found=0
                    for unsafe_pattern in "${UNSAFE_ANNOTATION_PATTERNS[@]}"; do
                        if echo "$context" | grep -qi "$unsafe_pattern"; then
                            echo -e "  ${RED}[REJECT]${NC} $file:$line_num — $pattern"
                            echo "         MemoryOwnership annotation contains UNSAFE justification:"
                            echo "         ($unsafe_pattern)"
                            echo "         Annotations must not justify daemon-lifetime/request-cycle leaks."
                            echo "         Refactor to use caller-owned buffers or add explicit deinit/free."
                            unsafe_found=1
                            found_issues=1
                        fi
                    done
                    
                    if [[ $unsafe_found -eq 0 ]]; then
                        echo "  [ALLOWED] $file:$line_num — $pattern (has safe MemoryOwnership)"
                    fi
                else
                    echo -e "  ${RED}[FAIL]${NC} $file:$line_num — $pattern"
                    echo "         (add MemoryOwnership annotation or fix allocation pattern)"
                    found_issues=1
                fi
            done
        fi
    done
    
    if [[ $found_issues -eq 1 ]]; then
        failed_files=$((failed_files + 1))
        return 1
    fi
    
    return 0
}

# Find files to scan (handles both files and directories)
find_files_to_scan() {
    for path in "${SCAN_PATHS[@]}"; do
        if [[ -f "$path" ]]; then
            # It's a file, scan it directly — use NUL terminator for consistent delimiting
            printf '%s\0' "$path"
        elif [[ -d "$path" ]]; then
            # It's a directory, find all .zig files
            find "$path" -name "*.zig" -type f -print0 2>/dev/null
        fi
    done
}

# Run self-test on fixtures
run_self_test() {
    echo "=== Self-Test Mode ==="
    echo "Testing sentinel fixtures..."
    echo ""
    
    local fixtures_dir="tovarisch/fixtures"
    
    # Test bad fixture - should FAIL
    local bad_fixture="$fixtures_dir/bad-memory-ownership-pattern.zig"
    if [[ -f "$bad_fixture" ]]; then
        echo "Testing bad fixture (should FAIL):"
        if scan_file "$bad_fixture"; then
            echo -e "  ${RED}[EXPECTED FAIL]${NC} bad-memory-ownership-pattern.zig passed but should fail"
            SELF_TEST_FAILED=$((SELF_TEST_FAILED + 1))
        else
            echo -e "  ${GREEN}[PASS]${NC} bad-memory-ownership-pattern.zig correctly failed"
            SELF_TEST_PASSED=$((SELF_TEST_PASSED + 1))
        fi
    fi
    
    # Test good fixture - should PASS
    local good_fixture="$fixtures_dir/good-memory-ownership-pattern.zig"
    if [[ -f "$good_fixture" ]]; then
        echo ""
        echo "Testing good fixture (should PASS):"
        if scan_file "$good_fixture"; then
            echo -e "  ${GREEN}[PASS]${NC} good-memory-ownership-pattern.zig correctly passed"
            SELF_TEST_PASSED=$((SELF_TEST_PASSED + 1))
        else
            echo -e "  ${RED}[FAIL]${NC} good-memory-ownership-pattern.zig failed but should pass"
            SELF_TEST_FAILED=$((SELF_TEST_FAILED + 1))
        fi
    fi
    
    echo ""
    echo "Self-test results: $SELF_TEST_PASSED passed, $SELF_TEST_FAILED failed"
    
    if [[ $SELF_TEST_FAILED -gt 0 ]]; then
        echo -e "${RED}[SELF-TEST FAIL]${NC} Sentinel fixture self-test failed"
        return 1
    fi
    
    echo -e "${GREEN}[SELF-TEST PASS]${NC} All sentinel fixtures tested correctly"
    return 0
}

# Parse arguments
for arg in "$@"; do
    case "$arg" in
        --self-test) SELF_TEST=1 ;;
    esac
done

if [[ "$SELF_TEST" -eq 1 ]]; then
    run_self_test
    exit $?
fi

# Normal scan mode
echo "Scanning status/request paths..."
for path in "${SCAN_PATHS[@]}"; do
    if [[ -f "$path" ]]; then
        echo "  Scanning file: $path"
    elif [[ -d "$path" ]]; then
        echo "  Scanning directory: $path"
    fi
done
echo ""

scan_failed=0

# Scan all Zig files
while IFS= read -r -d '' file; do
    if ! scan_file "$file"; then
        scan_failed=1
    else
        echo -e "  ${GREEN}[OK]${NC} $file"
    fi
done < <(find_files_to_scan | sort -uz)

echo ""
echo "Scanned $total_files source files."

if [[ $scan_failed -eq 1 ]]; then
    echo ""
    echo -e "${RED}[FAIL]${NC} Memory ownership hygiene gate failed."
    echo ""
    echo "To fix: Add MemoryOwnership annotation above the allocation, or refactor"
    echo "to use caller-owned scratch buffers (e.g., std.fmt.bufPrint with fixed buffer)."
    echo ""
    echo "See: docs/tooling/zig-0.16-field-manual-rss-leak.md"
    exit 1
fi

echo ""
echo -e "${GREEN}[PASS]${NC} Memory ownership hygiene gate passed."
exit 0
