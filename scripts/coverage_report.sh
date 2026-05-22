#!/usr/bin/env bash
# coverage_report.sh — Human-readable coverage report
# Combines real kcov line coverage with behavior coverage ledger

set -euo pipefail

echo "=========================================="
echo "  Tovarisch Coverage Report"
echo "=========================================="
echo ""

# Real line coverage section
echo "--- Real Line Coverage (kcov) ---"

if ! command -v kcov >/dev/null 2>&1; then
    echo "kcov not installed. Install via: brew install kcov"
    echo "Or bypass locally: ALLOW_MISSING_KCOV=1 make coverage"
    echo ""
    echo "See docs/doctrine/day-0-code-coverage.md for rationale."
else
    # Check if coverage dir exists from previous run
    if [[ -d "tovarisch/zig-out/coverage" ]]; then
        echo "NOTE: Found previous kcov output (may be stale)."
        echo "      Run 'make coverage' for fresh enforced coverage."
        echo ""
        echo "Previous kcov output:"
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        COVERAGE_PCT=$("$SCRIPT_DIR/extract_kcov_line_coverage.py" "tovarisch/zig-out/coverage")
        echo "  Line coverage: ${COVERAGE_PCT}%"
        echo "  Threshold: ${COVERAGE_THRESHOLD:-60}%"
        if [[ -f "tovarisch/zig-out/coverage/coverage.json" ]]; then
            echo "  Source files: $(ls tovarisch/zig-out/coverage/src-*/ 2>/dev/null | wc -l || echo 0) covered"
        fi
    else
        echo "No previous kcov output found."
        echo "Run 'make coverage' for fresh enforced coverage."
    fi
fi

echo ""
echo "--- Behavior Coverage Ledger ---"

LEDGER_PATH="docs/coverage/tovarisch-coverage.md"
if [[ -s "$LEDGER_PATH" ]]; then
    # Extract key sections for quick overview
    echo ""
    echo "Covered behaviors (from ledger):"
    grep -A 50 "### Covered Behaviors" "$LEDGER_PATH" | head -40
    
    echo ""
    echo "Accepted uncovered future behaviors:"
    grep -A 30 "### Accepted Uncovered Future Behaviors" "$LEDGER_PATH" | head -25
else
    echo "Coverage ledger not found: $LEDGER_PATH"
fi

echo ""
echo "=========================================="
echo "  Coverage Report Complete"
echo "=========================================="
echo ""
echo "For detailed ledger, see: docs/coverage/tovarisch-coverage.md"
echo "For threshold configuration, set: COVERAGE_THRESHOLD=<n>"
echo "For real coverage enforcement, run: make coverage"