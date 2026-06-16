#!/usr/bin/env bash
# verify_www_authenticate.sh — Verify Basic Auth WWW-Authenticate challenge header
#
# This script validates that a 401 response includes the required WWW-Authenticate
# header per RFC 9110. It fails if:
#   - HTTP status is not 401
#   - WWW-Authenticate header is missing
#   - WWW-Authenticate header does not contain expected Basic auth challenge
#
# Usage:
#   ./scripts/verify_www_authenticate.sh <url> [username:password]
#
# Arguments:
#   url           The URL to test (e.g., https://127.0.0.1:8443/)
#   username:password  Optional credentials for authenticated test
#
# Examples:
#   # Test unauthenticated request (expects 401 + WWW-Authenticate)
#   ./scripts/verify_www_authenticate.sh https://127.0.0.1:8443/
#
#   # Test with wrong credentials (expects 401 + WWW-Authenticate)
#   ./scripts/verify_www_authenticate.sh https://127.0.0.1:8443/ wrong:creds
#
#   # Test with correct credentials (expects non-401)
#   ./scripts/verify_www_authenticate.sh https://127.0.0.1:8443/ admin:secret
#
# Returns exit code 0 on valid response, non-zero on violation.

set -euo pipefail

EXPECTED_WWW_AUTH='Basic realm="uvb76", charset="UTF-8"'

usage() {
    echo "Usage: $0 <url> [username:password]" >&2
    echo "" >&2
    echo "Verifies that 401 responses include WWW-Authenticate header." >&2
    exit 1
}

if [[ $# -lt 1 ]]; then
    usage
fi

URL="$1"
CREDS="${2:-}"

# Build curl command
# Use -D - to dump headers to stdout (more reliable than -i with -o /dev/null)
CURL_CMD=(curl -sS -k -D - -o /dev/null)

if [[ -n "$CREDS" ]]; then
    CURL_CMD+=(-u "$CREDS")
fi

CURL_CMD+=("$URL")

# Execute curl and capture headers
response=$("${CURL_CMD[@]}" 2>&1) || true

# Extract HTTP status line (safe awk that won't fail on empty input)
status_line=$(
    printf '%s\n' "$response" |
    awk 'toupper($0) ~ /^HTTP\// { line=$0 } END { print line }'
)

http_status=$(
    printf '%s\n' "$status_line" |
    awk '{ print $2 }'
)

if [[ -z "$http_status" ]]; then
    echo "[verify-www-authenticate] FAIL: Could not parse HTTP status from response" >&2
    echo "Response was:" >&2
    echo "$response" | head -20 >&2
    exit 1
fi

# Extract WWW-Authenticate header (case-insensitive awk, no grep)
www_auth=$(
    printf '%s\n' "$response" |
    awk 'BEGIN { IGNORECASE=1 }
         /^WWW-Authenticate:/ {
           sub(/^[^:]+:[[:space:]]*/, "")
           sub(/\r$/, "")
           print
           exit
         }'
)

# Check for valid 401 with WWW-Authenticate
if [[ "$http_status" == "401" ]]; then
    if [[ -z "$www_auth" ]]; then
        echo "[verify-www-authenticate] FAIL: HTTP 401 without WWW-Authenticate header" >&2
        echo "Per RFC 9110, a 401 response MUST include WWW-Authenticate." >&2
        exit 1
    fi

    if [[ "$www_auth" != "$EXPECTED_WWW_AUTH" ]]; then
        echo "[verify-www-authenticate] FAIL: WWW-Authenticate header mismatch" >&2
        echo "Expected: $EXPECTED_WWW_AUTH" >&2
        echo "Got:      $www_auth" >&2
        exit 1
    fi

    echo "[verify-www-authenticate] PASS: HTTP $http_status with correct WWW-Authenticate: $www_auth"
    exit 0
fi

# Non-401 response (expected when correct credentials provided)
# Should NOT have WWW-Authenticate header
if [[ -n "$www_auth" ]]; then
    echo "[verify-www-authenticate] FAIL: Non-401 response should not have WWW-Authenticate" >&2
    echo "Got WWW-Authenticate: $www_auth" >&2
    exit 1
fi

echo "[verify-www-authenticate] PASS: HTTP $http_status (no WWW-Authenticate expected for authenticated requests)"
exit 0
