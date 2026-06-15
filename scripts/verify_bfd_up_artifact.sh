#!/bin/bash
set -euo pipefail

LAB_ROOT="${1:-/tmp}"

latest_lab_dir="$(find "$LAB_ROOT" -maxdepth 1 -type d -name 'kgb-bgp-bfd-lab-*' -printf '%T@ %p\n' 2>/dev/null \
    | sort -nr \
    | awk 'NR == 1 { print $2 }')"

if [[ -z "$latest_lab_dir" ]]; then
    echo "[verify-bfd-up] FAIL: no /tmp/kgb-bgp-bfd-lab-* directory found" >&2
    exit 1
fi

bfd_sessions="$latest_lab_dir/bird-bfd-sessions.txt"

echo "[verify-bfd-up] Lab dir: $latest_lab_dir"
echo "[verify-bfd-up] BFD sessions artifact: $bfd_sessions"

if [[ ! -s "$bfd_sessions" ]]; then
    echo "[verify-bfd-up] FAIL: bird-bfd-sessions.txt missing or empty" >&2
    find "$latest_lab_dir" -maxdepth 1 -type f -printf '  %f\n' 2>/dev/null || true
    exit 1
fi

echo "[verify-bfd-up] bird-bfd-sessions.txt:"
cat "$bfd_sessions"

if grep -qE '(^|[[:space:]])Up([[:space:]]|$)' "$bfd_sessions"; then
    echo "[verify-bfd-up] PASS: BFD session artifact contains Up"
    exit 0
fi

echo "[verify-bfd-up] FAIL: BFD session artifact does not contain Up" >&2
exit 1
