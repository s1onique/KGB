#!/usr/bin/env bash
# verify_opkg_package.sh - Wrapper for Python opkg verifier
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "${SCRIPT_DIR}/verify_opkg_package.py" "$@"
