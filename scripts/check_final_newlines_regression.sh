#!/usr/bin/env bash
# Regression test: verify gate catches untracked files without final newlines
# This ensures the final-newline check includes new/untracked files.

set -euo pipefail

echo "[regression] testing final-newline gate on untracked files"

tmp="tmp-final-newline-regression.py"
printf 'print("no newline")' > "$tmp"
trap 'rm -f "$tmp"' EXIT

# Gate should fail when an untracked file is missing a final newline
# Use || true to handle expected gate failure
if (./scripts/quality_gate.sh 2>&1 || true) | grep -F "tmp-final-newline-regression.py" > /dev/null 2>&1; then
    echo "[regression] PASS: gate caught untracked file without final newline"
    exit 0
else
    echo "[regression] FAIL: gate did NOT catch untracked file without final newline"
    exit 1
fi
