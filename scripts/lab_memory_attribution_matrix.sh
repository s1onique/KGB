#!/usr/bin/env bash
# Thin compatibility wrapper - delegates to Python matrix runner
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "${SCRIPT_DIR}/lab_memory_attribution_matrix.py" "$@"
