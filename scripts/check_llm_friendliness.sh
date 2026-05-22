#!/usr/bin/env bash
# check_llm_friendliness.sh — Enforce file size and line count limits
# Part of KGB quality gate

set -euo pipefail

# =============================================================================
# Configuration: Line count limits
# =============================================================================
# Format: EXTENSION:LIMIT (0 = ignore)
SOFT_LINE_LIMITS='
go:300
zig:300
py:300
sh:300
md:250
yml:250
yaml:250
toml:250
json:250
txt:200
'

HARD_LINE_LIMITS='
go:450
zig:450
py:450
sh:450
md:400
yml:500
yaml:500
toml:500
json:500
txt:300
'

# =============================================================================
# Configuration: Byte size limits (in KB)
# =============================================================================
HARD_BYTE_LIMITS='
go:32
zig:32
py:32
sh:32
md:32
yml:64
yaml:64
toml:64
json:64
txt:32
'

# =============================================================================
# Patterns to ignore entirely (generated/vendor/build artifacts)
# =============================================================================
DEFAULT_IGNORES='
.git/
.zig-cache/
.ziggy/
vendor/
node_modules/
build/
dist/
target/
*.pb.go
*_pb2.py
*.proto
'

# =============================================================================
# Helpers
# =============================================================================

usage() {
    cat <<EOF
Usage: check_llm_friendliness.sh [OPTIONS]

Check files for LLM-friendliness (line count and byte size limits).

OPTIONS:
    --ignore FILE     Read additional ignore patterns from FILE
    --verbose        Print ignored files (for debugging)
    -h, --help       Show this help

Exit codes:
    0  All files pass limits
    1  One or more files exceed limits

EOF
}

log_fail() {
    echo "[FAIL] $*" >&2
}

log_pass() {
    echo "[PASS] $*"
}

log_info() {
    echo "[INFO] $*"
}

# Get file extension (lowercase, no leading dot)
get_ext() {
    local file="$1"
    local ext="${file##*.}"
    ext="${ext##*/}"
    echo "${ext}" | tr '[:upper:]' '[:lower:]'
}

# Check if file matches any ignore pattern
is_ignored() {
    local file="$1"
    for pattern in "${IGNORES[@]}"; do
        # Support glob patterns
        case "$file" in
            ${pattern}) return 0 ;;
            */${pattern}) return 0 ;;
            */${pattern}/*) return 0 ;;
        esac
    done
    return 1
}

# Get limit value from config string
get_limit() {
    local config="$1"
    local ext="$2"
    local limit
    limit=$(echo "$config" | grep "^${ext}:" | head -1 | cut -d: -f2)
    if [[ -z "$limit" ]]; then
        limit=0
    fi
    echo "$limit"
}

# Convert KB to bytes
kb_to_bytes() {
    local kb="$1"
    echo $((kb * 1024))
}

# Read ignore file into IGNORES array
read_ignore_file() {
    local file="$1"
    while IFS= read -r line; do
        # Skip empty lines and comments
        line=$(echo "$line" | sed 's/#.*//' | tr -d '[:space:]')
        if [[ -n "$line" ]]; then
            IGNORES+=("$line")
        fi
    done < "$file"
}

# =============================================================================
# Main logic
# =============================================================================

VERBOSE=false
IGNORE_FILE=""
IGNORES=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --ignore)
            IGNORE_FILE="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage
            exit 1
            ;;
    esac
done

# Build ignore list
for pattern in $DEFAULT_IGNORES; do
    IGNORES+=("$pattern")
done

# Auto-load .llm-friendly-ignore if present
AUTO_IGNORE=".llm-friendly-ignore"
if [[ -f "$AUTO_IGNORE" ]]; then
    read_ignore_file "$AUTO_IGNORE"
fi

# Add custom ignore file patterns (if different from auto-loaded)
if [[ -n "$IGNORE_FILE" && -f "$IGNORE_FILE" && "$IGNORE_FILE" != "$AUTO_IGNORE" ]]; then
    read_ignore_file "$IGNORE_FILE"
fi

# Track failures
FAIL_COUNT=0
CHECK_COUNT=0

# Get list of tracked files AND untracked files (catch before staging)
# Tracked files
mapfile -t TRACKED < <(git ls-files)
# Untracked files (not in .gitignore)
mapfile -t UNTRACKED < <(git ls-files --others --exclude-standard)

# Combine and deduplicate
mapfile -t FILES < <(printf '%s\n' "${TRACKED[@]}" "${UNTRACKED[@]}" | sort -u)

for file in "${FILES[@]}"; do
    # Skip if file doesn't exist (e.g., deleted but still tracked)
    [[ -f "$file" ]] || continue

    # Check ignore patterns
    if is_ignored "$file"; then
        $VERBOSE && log_info "Ignored: $file"
        continue
    fi

    # Get file info
    ext=$(get_ext "$file")
    lines=$(wc -l < "$file" 2>/dev/null || echo 0)
    bytes=$(stat -f%z -- "$file" 2>/dev/null || stat -c%s -- "$file" 2>/dev/null || echo 0)

    CHECK_COUNT=$((CHECK_COUNT + 1))

    # Check line count
    hard_line=$(get_limit "$HARD_LINE_LIMITS" "$ext")
    if [[ "$hard_line" -gt 0 && "$lines" -gt "$hard_line" ]]; then
        log_fail "${file} has ${lines} lines; hard limit is ${hard_line}."
        echo "  Split by responsibility before continuing." >&2
        FAIL_COUNT=$((FAIL_COUNT + 1))
        continue
    fi

    # Check byte size
    hard_byte_kb=$(get_limit "$HARD_BYTE_LIMITS" "$ext")
    if [[ "$hard_byte_kb" -gt 0 ]]; then
        hard_byte=$(kb_to_bytes "$hard_byte_kb")
        if [[ "$bytes" -gt "$hard_byte" ]]; then
            log_fail "${file} is $((bytes / 1024)) KiB; hard limit is ${hard_byte_kb} KiB."
            echo "  Compress, trim, or decompose before continuing." >&2
            FAIL_COUNT=$((FAIL_COUNT + 1))
            continue
        fi
    fi

    # Soft warning for approaching limits (only for source files)
    soft_line=$(get_limit "$SOFT_LINE_LIMITS" "$ext")
    if [[ "$soft_line" -gt 0 && "$lines" -gt "$soft_line" && "$lines" -le "$hard_line" ]]; then
        echo "[WARN] ${file} has ${lines} lines; soft limit is ${soft_line}." >&2
        echo "  Consider decomposing before file grows larger." >&2
    fi

    $VERBOSE && log_pass "OK: $file (${lines} lines, $((bytes / 1024)) KiB)"
done

# Summary
echo ""
echo "[gate] LLM-friendliness: checked ${CHECK_COUNT} files"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    echo "[gate] LLM-friendliness: ${FAIL_COUNT} files exceeded limits" >&2
    exit 1
fi

echo "[gate] LLM-friendliness: PASS"
exit 0
