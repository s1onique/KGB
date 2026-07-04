#!/usr/bin/env bash
# check_allocation_patterns.sh — Risky allocation pattern reporter for tovarisch
#
# Scans Zig source files for risky allocation patterns that can cause
# memory leaks, unbounded growth, or RSS inflation in production code.
#
# RISKY PATTERNS DETECTED:
#   - std.heap.page_allocator       — page-backed memory; must have paired free
#   - std.heap.c_allocator         — C heap; must have paired free
#   - GeneralPurposeAllocator       — forbidden in production
#   - allocator.alloc               — raw allocation; must have paired free
#   - allocator.dupe               — string copy; must have paired free
#   - allocator.realloc             — resize; bounded growth required
#   - ArrayList.init                — dynamic collection; must call deinit
#   - ArrayListUnmanaged            — unmanaged; caller must manage lifetime
#
# EXEMPTIONS:
#   - Files ending in _tests.zig (test files)
#   - Files in */fixtures/* (test fixtures)
#   - Lines containing MemoryOwnership: annotation (approved patterns)
#   - Lines containing .free or .deinit (deallocation)
#   - Lines containing defer (deferral patterns)
#
# OUTPUT FORMAT:
#   Reports found patterns with classification (RISKY/ACCEPTED/DEFERRED)
#   First version: reports only, does not fail

set -euo pipefail

# Color codes
RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCAN_ROOT="${1:-tovarisch/src}"

# Risky patterns with risk levels (pattern|RISK pairs)
RISKY_PAIRS=(
    "std\.heap\.page_allocator|HIGH"
    "std\.heap\.c_allocator|HIGH"
    "GeneralPurposeAllocator|MEDIUM"
    "allocator\.alloc|MEDIUM"
    "allocator\.dupe|MEDIUM"
    "allocator\.realloc|HIGH"
    "ArrayList\.init|MEDIUM"
    "ArrayListUnmanaged|LOW"
)

# Counters
HIGH_COUNT=0
MEDIUM_COUNT=0
LOW_COUNT=0
DEFERRED_COUNT=0
ACCEPTED_COUNT=0
EXEMPT_COUNT=0
TOTAL_FILES=0

echo "=== Tovarisch Risky Allocation Pattern Report ==="
echo ""
echo "Scan root: $SCAN_ROOT"
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

# Check if scan root exists
if [[ ! -d "$SCAN_ROOT" ]]; then
    echo -e "${RED}[ERROR]${NC} Scan root does not exist: $SCAN_ROOT"
    exit 1
fi

# Get risk level for a pattern
get_risk_level() {
    local pattern="$1"
    for pair in "${RISKY_PAIRS[@]}"; do
        local p="${pair%%|*}"
        local r="${pair##|*}"
        if [[ "$p" == "$pattern" ]]; then
            echo "$r"
            return
        fi
    done
    echo "UNKNOWN"
}

# Check if a file is exempt
is_exempt() {
    local file="$1"
    local basename
    basename=$(basename "$file")
    
    # Skip test files
    if [[ "$basename" == *_tests.zig ]]; then
        return 0
    fi
    
    # Skip fixture files
    if [[ "$file" == */fixtures/* ]]; then
        return 0
    fi
    
    return 1
}

# Check if a line is exempt
is_line_exempt() {
    local line="$1"
    
    # Check for accepted patterns
    if echo "$line" | grep -qE "MemoryOwnership:"; then
        return 0
    fi
    if echo "$line" | grep -qE "\.deinit\("; then
        return 0
    fi
    if echo "$line" | grep -qE "\.free\("; then
        return 0
    fi
    if echo "$line" | grep -qE "defer "; then
        return 0
    fi
    
    return 1
}

# Classify a finding
classify_finding() {
    local file="$1"
    local line_num="$2"
    local pattern="$3"
    local line_content="$4"
    
    # Check for MemoryOwnership annotation nearby (5 lines before/after)
    local context_start=$((line_num > 5 ? line_num - 5 : 1))
    local context_end=$((line_num + 5))
    local context
    context=$(sed -n "${context_start},${context_end}p" "$file" 2>/dev/null || echo "")
    
    if echo "$context" | grep -q "MemoryOwnership:"; then
        echo "ACCEPTED"
        return
    fi
    
    # Check for defer patterns
    if echo "$context" | grep -qE "(defer|\.deinit\(|\.free\()"; then
        echo "ACCEPTED"
        return
    fi
    
    # Special case: page_allocator with free is ACCEPTED
    if [[ "$pattern" == "std\.heap\.page_allocator" ]] && \
       echo "$context" | grep -qE "page_allocator\.free|defer.*page_allocator"; then
        echo "ACCEPTED"
        return
    fi
    
    # Special case: ArrayList with deinit is ACCEPTED
    if [[ "$pattern" == "ArrayList\.init" ]] && \
       echo "$context" | grep -qE "\.deinit\("; then
        echo "ACCEPTED"
        return
    fi
    
    # Special case: serve_integration uses page_allocator for config parse
    if [[ "$pattern" == "std\.heap\.page_allocator" ]] && \
       [[ "$file" == *"serve_integration"* ]]; then
        echo "DEFERRED"
        return
    fi
    
    # Special case: tunnel_check uses page_allocator for sysfs
    if [[ "$pattern" == "std\.heap\.page_allocator" ]] && \
       [[ "$file" == *"tunnel_check"* ]]; then
        echo "DEFERRED"
        return
    fi
    
    local risk
    risk=$(get_risk_level "$pattern")
    echo "RISKY-$risk"
}

# Process each file
process_file() {
    local file="$1"
    local filename
    filename=$(basename "$file")
    
    TOTAL_FILES=$((TOTAL_FILES + 1))
    
    # Skip exempt files
    if is_exempt "$file"; then
        return
    fi
    
    local file_findings=0
    
    for pair in "${RISKY_PAIRS[@]}"; do
        local pattern="${pair%%|*}"
        
        # Find lines with this pattern
        local matches
        matches=$(grep -En "$pattern" "$file" 2>/dev/null || true)
        
        if [[ -n "$matches" ]]; then
            while IFS=: read -r line_num line_content; do
                # Skip if line is exempt
                if is_line_exempt "$line_content"; then
                    EXEMPT_COUNT=$((EXEMPT_COUNT + 1))
                    continue
                fi
                
                # Classify the finding
                local classification
                classification=$(classify_finding "$file" "$line_num" "$pattern" "$line_content")
                local risk
                risk=$(get_risk_level "$pattern")
                
                case "$classification" in
                    "RISKY-HIGH")
                        echo -e "${RED}[RISKY-HIGH]${NC} $file:$line_num"
                        echo "         Pattern: $pattern"
                        echo "         Line: $(echo "$line_content" | tr -s ' ' | sed 's/^[[:space:]]*//')"
                        HIGH_COUNT=$((HIGH_COUNT + 1))
                        file_findings=$((file_findings + 1))
                        ;;
                    "RISKY-MEDIUM")
                        echo -e "${RED}[RISKY-MEDIUM]${NC} $file:$line_num"
                        echo "         Pattern: $pattern"
                        echo "         Line: $(echo "$line_content" | tr -s ' ' | sed 's/^[[:space:]]*//')"
                        MEDIUM_COUNT=$((MEDIUM_COUNT + 1))
                        file_findings=$((file_findings + 1))
                        ;;
                    "RISKY-LOW")
                        echo -e "${YELLOW}[RISKY-LOW]${NC} $file:$line_num"
                        echo "         Pattern: $pattern"
                        echo "         Line: $(echo "$line_content" | tr -s ' ' | sed 's/^[[:space:]]*//')"
                        LOW_COUNT=$((LOW_COUNT + 1))
                        file_findings=$((file_findings + 1))
                        ;;
                    "ACCEPTED")
                        ACCEPTED_COUNT=$((ACCEPTED_COUNT + 1))
                        ;;
                    "DEFERRED")
                        echo -e "${YELLOW}[DEFERRED]${NC} $file:$line_num"
                        echo "         Pattern: $pattern (legacy surface)"
                        echo "         Line: $(echo "$line_content" | tr -s ' ' | sed 's/^[[:space:]]*//')"
                        DEFERRED_COUNT=$((DEFERRED_COUNT + 1))
                        file_findings=$((file_findings + 1))
                        ;;
                esac
            done <<< "$matches"
        fi
    done
    
    if [[ $file_findings -gt 0 ]]; then
        echo ""
    fi
}

echo -e "${BLUE}[INFO]${NC} Scanning production source files..."
echo ""

# Find all Zig files and process them
while IFS= read -r file; do
    process_file "$file"
done < <(find "$SCAN_ROOT" -name "*.zig" -type f 2>/dev/null | sort)

# Summary
echo "=== Summary ==="
echo ""
echo "Files scanned: $TOTAL_FILES"
echo ""
echo -e "Risky patterns found:"
echo -e "  ${RED}HIGH:${NC}   $HIGH_COUNT"
echo -e "  ${RED}MEDIUM:${NC} $MEDIUM_COUNT"
echo -e "  ${YELLOW}LOW:${NC}    $LOW_COUNT"
echo -e "  ${YELLOW}DEFERRED:${NC} $DEFERRED_COUNT"
echo ""
echo "Accepted patterns: $ACCEPTED_COUNT"
echo "Exempt patterns: $EXEMPT_COUNT"
echo ""

# Exit code logic (report only)
if [[ $HIGH_COUNT -gt 0 ]]; then
    echo -e "${RED}[REPORT]${NC} HIGH risk patterns found. Review and remediate."
    echo ""
    echo "See docs/architecture/tovarisch-allocation-register.md for accepted patterns"
    echo "and deferred legacy surfaces."
    exit 0
elif [[ $MEDIUM_COUNT -gt 0 ]]; then
    echo -e "${YELLOW}[REPORT]${NC} MEDIUM risk patterns found. Consider remediation."
    exit 0
elif [[ $LOW_COUNT -gt 0 ]]; then
    echo -e "${GREEN}[REPORT]${NC} Only LOW risk patterns found."
    exit 0
else
    echo -e "${GREEN}[REPORT]${NC} No risky patterns found in production code."
    exit 0
fi
