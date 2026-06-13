#!/usr/bin/env bash
# verify_plaintext_logs.sh
#
# Scans tovarisch/src/**/*.zig for forbidden plain-text logging patterns.
# Production code must use structured JSON logging via logging.emit().
#
# Forbidden patterns:
# - std.log.info("plain text", ...)
# - std.log.warn("plain text", ...)
# - std.log.err("plain text", ...)
# - std.log.debug("plain text", ...)
#
# Allowed patterns:
# - StructuredLogException annotation adjacent to the exact std.log call
#   (must include reason and expiry/removal ACT comment)
# - Test files (*_tests.zig) for test diagnostics
# - Comments and documentation

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "[verify-plaintext-logs] scanning tovarisch/src/"

# Track violations
violations=0

# Helper: report violation
report_violation() {
    local file="$1"
    local line="$2"
    local pattern="$3"
    echo "[verify-plaintext-logs] FAIL: $file:$line - '$pattern'" >&2
    violations=$((violations + 1))
}

# Helper: check if a specific line has a StructuredLogException annotation nearby
# The annotation must be on the line itself or within 2 lines before
has_exception_for_line() {
    local file="$1"
    local line_num="$2"
    
    # Check the line itself and 2 lines before for the annotation
    local start=$((line_num - 2))
    if [[ $start -lt 1 ]]; then start=1; fi
    
    local context=$(sed -n "${start},${line_num}p" "$file")
    echo "$context" | grep -q 'StructuredLogException'
}

# Helper: validate exception has reason and expiry
validate_exception() {
    local file="$1"
    local line_num="$2"
    
    local start=$((line_num - 2))
    if [[ $start -lt 1 ]]; then start=1; fi
    
    local context=$(sed -n "${start},$((line_num + 5))p" "$file")
    
    if ! echo "$context" | grep -qE 'reason:'; then
        echo "[verify-plaintext-logs] FAIL: $file:$line_num - StructuredLogException missing reason" >&2
        return 1
    fi
    if ! echo "$context" | grep -qE 'expiry:|removal ACT'; then
        echo "[verify-plaintext-logs] FAIL: $file:$line_num - StructuredLogException missing expiry/removal ACT" >&2
        return 1
    fi
    return 0
}

# Scan all .zig files in tovarisch/src/
# Exclude: test files, files with StructuredLogException annotation
while IFS= read -r -d '' file; do
    # Skip test files - they may use std.log for test diagnostics
    case "$file" in
        *_tests.zig)
            continue
            ;;
    esac

    # Skip usage.zig - contains intentional help text
    if [[ "$file" == *"/usage.zig" ]]; then
        continue
    fi

    # Strict check: any std.log.info/warn/err/debug in non-test source files is forbidden
    # unless explicitly annotated with StructuredLogException adjacent to the call
    while IFS= read -r line_num; do
        line=$(sed -n "${line_num}p" "$file")
        if echo "$line" | grep -qE 'std\.log\.(info|warn|err|debug)\('; then
            # Check if this specific line has an exception annotation
            if has_exception_for_line "$file" "$line_num"; then
                if ! validate_exception "$file" "$line_num"; then
                    violations=$((violations + 1))
                else
                    echo "[verify-plaintext-logs] INFO: $file:$line_num has StructuredLogException (allowed)"
                fi
            else
                report_violation "$file" "$line_num" "std.log.* call in production code (use logging.emit instead)"
            fi
        fi
    done < <(grep -n 'std\.log\.\(info\|warn\|err\|debug\)(' "$file" 2>/dev/null | cut -d: -f1)

done < <(find "$PROJECT_ROOT/tovarisch/src" -name "*.zig" -print0 2>/dev/null)

# === Sentinel Tests ===
echo "[verify-plaintext-logs] testing sentinels..."

# Bad sentinel must be detected as failing
bad_sentinel="$PROJECT_ROOT/tovarisch/fixtures/bad-sentinel-plaintext-log.zig"
if [[ -f "$bad_sentinel" ]]; then
    bad_found=0
    while IFS= read -r line_num; do
        line=$(sed -n "${line_num}p" "$bad_sentinel")
        if echo "$line" | grep -qE 'std\.log\.(info|warn|err)\('; then
            bad_found=1
            echo "[verify-plaintext-logs] INFO: bad-sentinel detected std.log at line $line_num (expected)"
        fi
    done < <(grep -n 'std\.log\.\(info\|warn\|err\)(' "$bad_sentinel" 2>/dev/null | cut -d: -f1)
    
    if [[ "$bad_found" -eq 1 ]]; then
        echo "[verify-plaintext-logs] INFO: bad-sentinel correctly contains prohibited std.log patterns"
    else
        echo "[verify-plaintext-logs] WARN: bad-sentinel has no std.log patterns to detect"
    fi
else
    echo "[verify-plaintext-logs] FAIL: bad-sentinel not found" >&2
    violations=$((violations + 1))
fi

# Good sentinel must pass (no std.log calls)
good_sentinel="$PROJECT_ROOT/tovarisch/fixtures/good-sentinel-structured-log.zig"
if [[ -f "$good_sentinel" ]]; then
    good_has_stdlog=0
    while IFS= read -r line_num; do
        line=$(sed -n "${line_num}p" "$good_sentinel")
        if echo "$line" | grep -qE 'std\.log\.(info|warn|err|debug)\('; then
            good_has_stdlog=1
            echo "[verify-plaintext-logs] FAIL: good-sentinel contains std.log at line $line_num" >&2
            violations=$((violations + 1))
        fi
    done < <(grep -n 'std\.log\.\(info\|warn\|err\|debug\)(' "$good_sentinel" 2>/dev/null | cut -d: -f1)
    
    if [[ "$good_has_stdlog" -eq 0 ]]; then
        echo "[verify-plaintext-logs] INFO: good-sentinel correctly uses logging.emit()"
    fi
else
    echo "[verify-plaintext-logs] FAIL: good-sentinel not found" >&2
    violations=$((violations + 1))
fi

if [[ $violations -gt 0 ]]; then
    echo ""
    echo "[verify-plaintext-logs] FAIL: $violations plaintext logging violation(s) found" >&2
    echo "[verify-plaintext-logs] Use logging.emit() for structured JSON logging" >&2
    echo "[verify-plaintext-logs] Temporary exceptions require StructuredLogException: reason + expiry" >&2
    exit 1
fi

echo "[verify-plaintext-logs] PASS: no plaintext logging detected"
exit 0
