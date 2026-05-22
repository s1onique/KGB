#!/usr/bin/env bash
# coverage_gate.sh — Thin launcher for Python coverage gate
#
# The real orchestration is in coverage_gate.py, which provides:
# - Clean subprocess capture (avoids bash stdout/stderr contamination)
# - DWARF diagnostics
# - 4 diagnostic retry modes
# - Python parser integration
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/coverage_gate.py" "$@"
