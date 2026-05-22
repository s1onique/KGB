#!/usr/bin/env bash
# coverage_gate.sh — Real line coverage gate using kcov
# Fails if kcov is missing unless ALLOW_MISSING_KCOV=1
# NO escape hatches: any failure in the coverage chain fails the gate

set -euo pipefail

# Derive script directory at start (before any cd) for portability
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TOVARISCH_DIR="$REPO_ROOT/tovarisch"

COVERAGE_THRESHOLD="${COVERAGE_THRESHOLD:-60}"

echo "[coverage] using kcov for real line coverage"

# Check for kcov
if ! command -v kcov >/dev/null 2>&1; then
    if [[ "${ALLOW_MISSING_KCOV:-}" == "1" ]]; then
        echo "[INFO] coverage: kcov not found, skipping (ALLOW_MISSING_KCOV=1)"
        exit 0
    fi
    echo "[FAIL] coverage: kcov is required for real coverage gate" >&2
    echo "[INFO] coverage: install kcov or set ALLOW_MISSING_KCOV=1 for local-only bypass" >&2
    exit 1
fi

cd "$TOVARISCH_DIR"

# Build test artifact — MUST succeed; no || true
# Uses test-bin step which installs binary to stable path for kcov
# Must build with debug info for kcov to work
echo "[coverage] building test artifact for kcov"
zig build test-bin -Doptimize=Debug

# Find the stable test binary path
# The test binary is installed to zig-out/bin/ by the test-bin step
TEST_BINARY="zig-out/bin/tovarisch-test"

# Validate test binary exists — must fail if missing
if [[ -z "${TEST_BINARY:-}" || ! -f "$TEST_BINARY" ]]; then
    echo "[FAIL] coverage: test binary not found at $TEST_BINARY" >&2
    exit 1
fi

echo "[coverage] test binary: $TEST_BINARY"

# Zero-test guard: run the test binary once to verify it actually runs tests
# If the binary exits nonzero, fail immediately before wasting time on kcov
echo "[coverage] verifying test binary executes real tests..."
if ! TEST_OUTPUT=$("$TEST_BINARY" 2>&1); then
    echo "[FAIL] coverage: test binary failed before kcov" >&2
    echo "$TEST_OUTPUT" >&2
    exit 1
fi
if echo "$TEST_OUTPUT" | grep -q "All 0 tests passed"; then
    echo "[FAIL] coverage: test binary contains zero tests — cannot measure coverage" >&2
    echo "[INFO] coverage: test binary output:" >&2
    echo "$TEST_OUTPUT" | head -5 >&2
    exit 1
fi
echo "[coverage] test binary confirmed: real tests found"

# =============================================================================
# DWARF DIAGNOSTICS — Determine if binary contains project source paths
# =============================================================================
echo ""
echo "[DWARF-DIAGNOSTIC] === Binary analysis for source mapping ==="

echo "[DWARF-DIAGNOSTIC] file type:"
file "$TEST_BINARY"

echo "[DWARF-DIAGNOSTIC] debug sections:"
if command -v readelf >/dev/null 2>&1; then
    readelf -S "$TEST_BINARY" | grep -E 'debug|symtab' || echo "[DWARF-DIAGNOSTIC] no debug/symtab sections found"
else
    echo "[DWARF-DIAGNOSTIC] readelf not available — skipping section listing"
fi

echo "[DWARF-DIAGNOSTIC] checking for project source paths in DWARF line tables..."
if command -v readelf >/dev/null 2>&1; then
    # Try readelf first (widely available on Linux after binutils install)
    # grep exits 1 when no matches — must be non-fatal inside substitution
    DWARF_LINES=$(
        readelf --debug-dump=decodedline "$TEST_BINARY" 2>/dev/null \
            | grep -E 'tovarisch/src|src/(main|cli|status|http)|commands\.zig|status\.zig' \
            | head -50 \
            || true
    )
    if [[ -n "$DWARF_LINES" ]]; then
        echo "[DWARF-DIAGNOSTIC] FOUND project paths in DWARF line tables:"
        echo "$DWARF_LINES" | head -50 | sed 's/^/[DWARF-DIAGNOSTIC] &/'
        echo "[DWARF-DIAGNOSTIC] project path match count: $(printf '%s\n' "$DWARF_LINES" | sed '/^$/d' | wc -l)"
    else
        echo "[DWARF-DIAGNOSTIC] WARNING: no project source paths found in DWARF line tables"
        echo "[DWARF-DIAGNOSTIC] This suggests Zig compiled the tests without debug-line info"
        
        # Show a sample of what IS in the DWARF to help diagnose
        echo "[DWARF-DIAGNOSTIC] Sample of actual DWARF paths (first 20 lines):"
        readelf --debug-dump=decodedline "$TEST_BINARY" 2>/dev/null | head -20 | sed 's/^/[DWARF-DIAGNOSTIC] &/'
    fi
elif command -v llvm-dwarfdump >/dev/null 2>&1; then
    # Fallback to llvm-dwarfdump if available
    # grep exits 1 when no matches — must be non-fatal inside substitution
    DWARF_LINES=$(
        llvm-dwarfdump --debug-line "$TEST_BINARY" 2>/dev/null \
            | grep -E 'tovarisch/src|src/(main|cli|status|http)|commands\.zig|status\.zig' \
            | head -50 \
            || true
    )
    if [[ -n "$DWARF_LINES" ]]; then
        echo "[DWARF-DIAGNOSTIC] FOUND project paths in DWARF line tables:"
        echo "$DWARF_LINES" | head -50 | sed 's/^/[DWARF-DIAGNOSTIC] &/'
        echo "[DWARF-DIAGNOSTIC] project path match count: $(printf '%s\n' "$DWARF_LINES" | sed '/^$/d' | wc -l)"
    else
        echo "[DWARF-DIAGNOSTIC] WARNING: no project source paths found in DWARF line tables"
        echo "[DWARF-DIAGNOSTIC] llvm-dwarfdump sample (first 20 lines):"
        llvm-dwarfdump --debug-line "$TEST_BINARY" 2>/dev/null | head -20 | sed 's/^/[DWARF-DIAGNOSTIC] &/'
    fi
else
    echo "[DWARF-DIAGNOSTIC] Neither readelf nor llvm-dwarfdump available — cannot inspect DWARF"
fi
echo "[DWARF-DIAGNOSTIC] === End binary analysis ==="
echo ""

# =============================================================================
# KCOV RUN
# =============================================================================

# Create coverage output directory
COVERAGE_DIR="zig-out/coverage"
rm -rf "$COVERAGE_DIR"
mkdir -p "$COVERAGE_DIR"

# Run kcov on test binary with src-only inclusion
# kcov MUST succeed; no || true
echo "[coverage] running kcov with threshold ${COVERAGE_THRESHOLD}%"

echo "[coverage] running kcov (parser filters to tovarisch/src)"
kcov --exclude-path=zig-cache \
     --exclude-path=.git \
     "$COVERAGE_DIR" \
     "$TEST_BINARY"

# Capped kcov tree dump — prevent enormous CI logs while still providing evidence
# Suppress SIGPIPE from head truncation (not a real error)
echo "[coverage-debug] kcov output directory contents (capped at 100 files):"
find "$COVERAGE_DIR" -maxdepth 3 -type f 2>/dev/null | sort | head -100 | sed 's#^#  [coverage-debug] file: #' || true
FILE_COUNT=$(find "$COVERAGE_DIR" -type f 2>/dev/null | wc -l)
if [[ "$FILE_COUNT" -gt 100 ]]; then
    echo "  [coverage-debug] ... and $((FILE_COUNT - 100)) more files (output capped)"
fi
echo "[coverage-debug] end of kcov output tree (total: $FILE_COUNT files)"

# kcov succeeded — parse coverage
# SCRIPT_DIR, REPO_ROOT, TOVARISCH_DIR already set at script start
cd "$TOVARISCH_DIR"

COVERAGE_PCT=$(python3 "$SCRIPT_DIR/extract_kcov_line_coverage.py" "$TOVARISCH_DIR/zig-out/coverage")

echo ""
echo "[INFO] coverage: line coverage ${COVERAGE_PCT}%"
echo "[INFO] coverage: threshold ${COVERAGE_THRESHOLD}.00%"

# Compare against threshold
# COVERAGE_PCT is now a clean number (e.g., "68.42")
COVERAGE_NUM="$COVERAGE_PCT"

if [[ -z "$COVERAGE_NUM" ]]; then
    echo "[FAIL] coverage: could not parse coverage percentage" >&2
    exit 1
fi

# Compare: fail if below threshold
is_below=$(echo "$COVERAGE_NUM < $COVERAGE_THRESHOLD" | bc -l 2>/dev/null || {
    # Fallback: integer comparison
    COV_INT=$(printf "%.0f" "$COVERAGE_NUM" 2>/dev/null || echo 0)
    THR_INT=$COVERAGE_THRESHOLD
    [[ "$COV_INT" -lt "$THR_INT" ]] && echo 1 || echo 0
})

if [[ "$is_below" == "1" ]]; then
    echo "[FAIL] coverage: line coverage ${COVERAGE_PCT}% below threshold ${COVERAGE_THRESHOLD}%" >&2
    exit 1
fi

echo "[PASS] coverage: real line coverage gate passed"
