#!/usr/bin/env bash
# verify_split_test_inventory.sh — Detect drift between canonical test inventory and split suites
#
# Canonical inventory: tovarisch/src/test_all.zig
# Split suites: tovarisch/src/test_suite_*.zig
#
# Detects:
# - Modules in canonical inventory but not assigned to any split suite
# - Modules assigned to more than one split suite (duplicate assignment)
# - Modules in split suites but not present in canonical inventory
#
# Exit codes:
#   0 — inventory is consistent
#   1 — drift detected
#   2 — internal error (missing files, etc.)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SRC_DIR="$REPO_ROOT/tovarisch/src"

CANONICAL="$SRC_DIR/test_all.zig"

# =============================================================================
# Helper: extract @import("...") module paths from a Zig source file
# Outputs normalized paths relative to $SRC_DIR (e.g., "net/foo.zig")
# Uses sed instead of grep -P for macOS compatibility.
# =============================================================================
extract_imports() {
    local file="$1"
    sed -n 's/.*@import("\([^"]*\)").*/\1/p' "$file" 2>/dev/null || true
}

# =============================================================================
# Collect canonical inventory from test_all.zig
# =============================================================================
echo "[split-test-inventory] gathering canonical inventory from $CANONICAL"

canonical_modules=$(
    extract_imports "$CANONICAL" \
    | grep -v '^std$' \
    | sort -u
)

if [[ -z "$canonical_modules" ]]; then
    echo "[split-test-inventory] ERROR: no @import found in $CANONICAL" >&2
    exit 2
fi

# =============================================================================
# Collect split-suite inventories
# =============================================================================
split_suites=()
while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    split_suites+=("$f")
done < <(find "$SRC_DIR" -maxdepth 1 -name 'test_suite_*.zig' -type f | sort)

if [[ ${#split_suites[@]} -eq 0 ]]; then
    echo "[split-test-inventory] ERROR: no test_suite_*.zig files found in $SRC_DIR" >&2
    exit 2
fi

echo "[split-test-inventory] found ${#split_suites[@]} split suites"

# Build a map: module -> comma-separated list of suite names
declare -A module_suites

drift=0

for suite in "${split_suites[@]}"; do
    suite_name="$(basename "$suite")"
    
    # Extract imports from const _xxx = @import("...") declarations only
    # This avoids picking up test { refAllDecls(...) } blocks
    # Uses POSIX sed for macOS compatibility
    modules=$(
        sed -n 's/const _[a-zA-Z0-9_]* = @import("\([^"]*\)").*/\1/p' "$suite" 2>/dev/null \
        | sort -u
    )
    
    if [[ -n "$modules" ]]; then
        while IFS= read -r mod; do
            [[ -n "$mod" ]] || continue
            if [[ -v "module_suites[$mod]" ]]; then
                module_suites["$mod"]="${module_suites[$mod]},$suite_name"
            else
                module_suites["$mod"]="$suite_name"
            fi
        done <<< "$modules"
    fi
done

# =============================================================================
# Check 1: modules in canonical but not in any split suite
# =============================================================================
echo ""
echo "[split-test-inventory] === Check 1: canonical modules missing from all split suites ==="

missing=0
while IFS= read -r canonical_mod; do
    [[ -n "$canonical_mod" ]] || continue
    
    if [[ ! -v "module_suites[$canonical_mod]" ]]; then
        echo "ERROR: canonical test module is not assigned to any split suite: $canonical_mod"
        missing=1
    fi
done <<< "$canonical_modules"

# =============================================================================
# Check 2: modules in more than one split suite (duplicate assignment)
# =============================================================================
echo ""
echo "[split-test-inventory] === Check 2: duplicate split-suite assignments ==="

duplicates=0
for mod in "${!module_suites[@]}"; do
    suites="${module_suites[$mod]}"
    # Count commas to determine how many suites this module is in
    comma_count=$(echo "$suites" | tr -cd ',' | wc -c)
    if [[ "$comma_count" -gt 0 ]]; then
        echo "ERROR: test module appears in more than one split suite: $mod"
        IFS=',' read -ra suite_list <<< "$suites"
        for s in "${suite_list[@]}"; do
            echo "  - $s"
        done
        duplicates=1
    fi
done

# =============================================================================
# Check 3: modules in split suites but not in canonical inventory
# =============================================================================
echo ""
echo "[split-test-inventory] === Check 3: split-suite modules not in canonical inventory ==="

unknown=0
for mod in "${!module_suites[@]}"; do
    # Check if this module is in the canonical inventory
    if ! echo "$canonical_modules" | grep -qxF "$mod"; then
        echo "ERROR: split suite references module not present in $CANONICAL: $mod"
        suites="${module_suites[$mod]}"
        IFS=',' read -ra suite_list <<< "$suites"
        for s in "${suite_list[@]}"; do
            echo "  - $s"
        done
        unknown=1
    fi
done

# =============================================================================
# Summary
# =============================================================================
echo ""
if [[ "$missing" -eq 1 || "$duplicates" -eq 1 || "$unknown" -eq 1 ]]; then
    echo "[split-test-inventory] FAIL: drift detected"
    exit 1
fi

echo "[split-test-inventory] PASS: split test inventory is consistent"
exit 0
