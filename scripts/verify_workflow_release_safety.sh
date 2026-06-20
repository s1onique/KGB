#!/usr/bin/env bash
# =============================================================================
# verify_workflow_release_safety.sh
#
# Thin launcher for the Python workflow release safety verifier.
# All complex logic has been moved to verify_workflow_release_safety.py.
#
# Exit codes:
#   0 = PASS
#   1 = FAIL (policy violation found)
#   2 = ERROR (script usage/environment issue)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

exec python3 "${SCRIPT_DIR}/verify_workflow_release_safety.py" "$@"
