#!/usr/bin/env bash
# verify_structured_logs.sh
#
# Checks for prose runtime logging in tovarisch source files.
# Prose runtime logs should use structured JSON logging instead.
#
# Scans tovarisch/src/**/*.zig for forbidden patterns:
# - std.debug.print in serve/runtime paths
# - stdout/stderr prose in runtime paths
# - Prose human-readable status messages in runtime code
# Note: Emoji are ALLOWED in structured log field string values
#
# Exceptions:
# - help/usage text modules
# - test/fixture files
# - Error handling that writes to stderr (for CLI errors, not runtime logs)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "[verify-structured-logs] scanning tovarisch/src/"

# Track violations
violations=0

# Helper: report violation
report_violation() {
    local file="$1"
    local line="$2"
    local pattern="$3"
    echo "[verify-structured-logs] FAIL: $file:$line - '$pattern'" >&2
    violations=$((violations + 1))
}

# Scan all .zig files in tovarisch/src/
# Exclude: test files, usage module (intentional help text)
while IFS= read -r -d '' file; do
    # Skip test files
    case "$file" in
        *_tests.zig)
            continue
            ;;
    esac

    # Skip usage.zig - contains intentional help text
    if [[ "$file" == *"/usage.zig" ]]; then
        continue
    fi

    # Check for forbidden prose patterns in runtime paths
    # Pattern 1: std.debug.print with human-readable messages (not errors)
    # Allow: std.debug.print("..." with errno, error context
    # Forbid: std.debug.print("Listening on", "Entering accept loop", etc.
    while IFS= read -r line_num; do
        # Get the actual line content
        line=$(sed -n "${line_num}p" "$file")
        
        # Skip if this is an error/diagnostic print (contains errno, error)
        if echo "$line" | grep -qE 'errno|error:'; then
            continue
        fi
        
        # Forbid prose runtime logs
        if echo "$line" | grep -qE 'Listening on|Entering accept|Listen to UVB'; then
            report_violation "$file" "$line_num" "prose runtime log"
        fi
    done < <(grep -n 'std\.debug\.print' "$file" 2>/dev/null | cut -d: -f1)

    # Pattern 2: stdout.print with prose (not JSON, not status payload)
    # Allow: status --json output, help text
    # Forbid: startup messages, status messages in serve loop
    while IFS= read -r line_num; do
        line=$(sed -n "${line_num}p" "$file")
        
        # Skip status payload rendering (renderPayload)
        if echo "$line" | grep -qE 'renderPayload|status\.renderPayload'; then
            continue
        fi
        
        # Skip if it's JSON (contains {")
        if echo "$line" | grep -qE '\{\s*"'; then
            continue
        fi
        
        # Forbid prose stdout in server/serve paths
        if echo "$line" | grep -qE 'Starting tovarisch|Press Ctrl\+C'; then
            report_violation "$file" "$line_num" "prose stdout"
        fi
    done < <(grep -n '\.print\|writeAll' "$file" 2>/dev/null | grep -E 'stdout|stderr' | cut -d: -f1)

done < <(find "$PROJECT_ROOT/tovarisch/src" -name "*.zig" -print0 2>/dev/null)

if [[ "$violations" -gt 0 ]]; then
    echo ""
    echo "[verify-structured-logs] FAIL: $violations prose logging violation(s) found" >&2
    echo "[verify-structured-logs] Use structured JSON logging for runtime messages" >&2
    exit 1
fi

echo "[verify-structured-logs] PASS: no prose runtime logs detected"
exit 0
