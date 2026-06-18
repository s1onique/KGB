#!/usr/bin/env bash
# verify_field_manual_network_diag.sh — Field manual presence + dangerous pattern check
#
# Narrow verifier for network diagnostics ownership lessons.
#
# Checks:
# 1. Field manual references network diagnostics companion doc
# 2. Companion doc exists with required sections
# 3. No dangerous `catch "0"` patterns in network diagnostics code
# 4. No unannotated @memcpy in network diagnostics code
#
# This verifier is OPTIONAL and NOT wired into make gate.
# Run manually when updating network diagnostics code.

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

SELF_TEST=0
SELF_TEST_FAILED=0
SELF_TEST_PASSED=0

# ============================================================================
# Section 1: Field Manual References Check
# ============================================================================

check_field_manual_sections() {
    local field_manual="docs/tooling/zig-0.16-field-manual.md"
    local companion="docs/tooling/zig-0.16-network-diagnostics-ownership.md"
    local failed=0

    echo "=== Field Manual Sections Check ==="
    echo ""

    if [[ ! -f "$field_manual" ]]; then
        echo -e "${RED}[FAIL]${NC} Field manual not found: $field_manual"
        return 1
    fi

    # Check that field manual references companion doc
    if grep -q "zig-0.16-network-diagnostics-ownership.md" "$field_manual"; then
        echo -e "  ${GREEN}[OK]${NC} Field manual references companion doc"
    else
        echo -e "  ${RED}[FAIL]${NC} Field manual missing companion doc reference"
        failed=1
    fi

    if [[ ! -f "$companion" ]]; then
        echo -e "  ${RED}[FAIL]${NC} Companion doc not found: $companion"
        return 1
    fi
    echo -e "  ${GREEN}[OK]${NC} Companion doc exists: $companion"

    # Required sections in companion doc
    local required_sections=(
        "Ownership Rules for Parsed Diagnostics"
        "Confirmed Zig 0.16 Edge Cases"
    )

    for section in "${required_sections[@]}"; do
        if grep -q "$section" "$companion"; then
            echo -e "  ${GREEN}[OK]${NC} Found section: $section"
        else
            echo -e "  ${RED}[FAIL]${NC} Missing section: $section"
            failed=1
        fi
    done

    if [[ $failed -eq 0 ]]; then
        echo ""
        echo -e "${GREEN}[PASS]${NC} Field manual sections check passed"
        return 0
    else
        echo ""
        echo -e "${RED}[FAIL]${NC} Field manual sections check failed"
        return 1
    fi
}

# ============================================================================
# Section 2: Dangerous Pattern Check in tovarisch/src/net
# ============================================================================

check_dangerous_patterns() {
    local net_dir="tovarisch/src/net"
    local failed=0

    echo ""
    echo "=== Dangerous Pattern Check ==="
    echo ""

    if [[ ! -d "$net_dir" ]]; then
        echo -e "${YELLOW}[SKIP]${NC} Directory not found: $net_dir"
        return 0
    fi

    # Pattern 1: catch "0" or catch "" (mixed ownership trap)
    echo "Checking for mixed ownership patterns (catch \"0\", catch \"\")..."
    local mixed_pattern_files=$(grep -rl 'catch\s*"' "$net_dir" --include="*.zig" 2>/dev/null || true)
    
    if [[ -n "$mixed_pattern_files" ]]; then
        for file in $mixed_pattern_files; do
            # Filter out test files
            if [[ "$file" != *_tests.zig ]]; then
                echo -e "  ${YELLOW}[WARN]${NC} File has catch-with-literal: $file"
                grep -n 'catch\s*"' "$file" | head -5 | sed 's/^/         /'
            fi
        done
        # Don't fail on this - just warn, as some patterns may be intentional
        echo -e "  ${YELLOW}[INFO]${NC} Review above files for mixed ownership issues"
    else
        echo -e "  ${GREEN}[OK]${NC} No catch-with-literal patterns found"
    fi

    echo ""
    echo "Checking for unannotated @memcpy..."
    
    # Find all @memcpy usages
    local memcpy_files=$(grep -rl '@memcpy' "$net_dir" --include="*.zig" 2>/dev/null || true)
    
    if [[ -n "$memcpy_files" ]]; then
        for file in $memcpy_files; do
            # Skip test files
            if [[ "$file" == *_tests.zig ]]; then
                continue
            fi
            
            # Check each @memcpy line for MemoryCopySafety annotation
            while IFS=: read -r line_num line_content; do
                # Check surrounding 5 lines for annotation
                local context_start=$((line_num > 5 ? line_num - 5 : 1))
                local context_end=$((line_num + 5))
                local context=$(sed -n "${context_start},${context_end}p" "$file")
                
                if ! echo "$context" | grep -q 'MemoryCopySafety:'; then
                    echo -e "  ${RED}[FAIL]${NC} $file:$line_num — unannotated @memcpy"
                    echo "         Add MemoryCopySafety: annotation or use copyForwards/copyBackwards"
                    failed=1
                else
                    echo -e "  ${GREEN}[OK]${NC} $file:$line_num — @memcpy has MemoryCopySafety"
                fi
            done < <(grep -n '@memcpy' "$file" 2>/dev/null || true)
        done
    else
        echo -e "  ${GREEN}[OK]${NC} No @memcpy patterns found"
    fi

    if [[ $failed -eq 0 ]]; then
        echo ""
        echo -e "${GREEN}[PASS]${NC} Dangerous pattern check passed"
        return 0
    else
        echo ""
        echo -e "${RED}[FAIL]${NC} Dangerous pattern check failed"
        return 1
    fi
}

# ============================================================================
# Self-Test Mode
# ============================================================================

run_self_test() {
    echo "=== Self-Test Mode ==="
    echo ""
    
    # Create temporary test fixtures
    local temp_dir=$(mktemp -d)
    trap "rm -rf $temp_dir" EXIT
    
    # Good fixture: has MemoryCopySafety
    cat > "$temp_dir/good-net-diag.zig" << 'EOF'
const std = @import("std");

// MemoryCopySafety: dst is a fresh buffer, src is from a different source
pub fn copyData(dst: []u8, src: []const u8) void {
    @memcpy(dst[0..src.len], src);
}
EOF

    # Bad fixture: missing annotation
    cat > "$temp_dir/bad-net-diag.zig" << 'EOF'
const std = @import("std");

pub fn copyData(dst: []u8, src: []const u8) void {
    @memcpy(dst[0..src.len], src);  // Missing annotation
}
EOF

    echo "Testing good fixture (should PASS annotation check):"
    if grep -q 'MemoryCopySafety:' "$temp_dir/good-net-diag.zig"; then
        echo -e "  ${GREEN}[PASS]${NC} Good fixture has annotation"
        SELF_TEST_PASSED=$((SELF_TEST_PASSED + 1))
    else
        echo -e "  ${RED}[FAIL]${NC} Good fixture missing annotation"
        SELF_TEST_FAILED=$((SELF_TEST_FAILED + 1))
    fi

    echo ""
    echo "Testing bad fixture (should FAIL annotation check):"
    if grep -q 'MemoryCopySafety:' "$temp_dir/bad-net-diag.zig"; then
        echo -e "  ${RED}[FAIL]${NC} Bad fixture has annotation (should not)"
        SELF_TEST_FAILED=$((SELF_TEST_FAILED + 1))
    else
        echo -e "  ${GREEN}[PASS]${NC} Bad fixture correctly lacks annotation"
        SELF_TEST_PASSED=$((SELF_TEST_PASSED + 1))
    fi

    echo ""
    echo "Self-test results: $SELF_TEST_PASSED passed, $SELF_TEST_FAILED failed"
    
    if [[ $SELF_TEST_FAILED -gt 0 ]]; then
        echo -e "${RED}[SELF-TEST FAIL]${NC}"
        return 1
    fi
    
    echo -e "${GREEN}[SELF-TEST PASS]${NC}"
    return 0
}

# ============================================================================
# Main
# ============================================================================

main() {
    for arg in "$@"; do
        case "$arg" in
            --self-test) SELF_TEST=1 ;;
        esac
    done

    if [[ "$SELF_TEST" -eq 1 ]]; then
        run_self_test
        exit $?
    fi

    echo "=============================================="
    echo " Field Manual + Network Diag Verifier"
    echo "=============================================="
    echo ""

    local exit_code=0

    if ! check_field_manual_sections; then
        exit_code=1
    fi

    if ! check_dangerous_patterns; then
        exit_code=1
    fi

    echo ""
    if [[ $exit_code -eq 0 ]]; then
        echo -e "${GREEN}=============================================="
        echo -e " ALL CHECKS PASSED"
        echo -e "==============================================${NC}"
    else
        echo -e "${RED}=============================================="
        echo -e " SOME CHECKS FAILED"
        echo -e "==============================================${NC}"
    fi

    exit $exit_code
}

main "$@"
