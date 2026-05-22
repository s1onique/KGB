#!/usr/bin/env bash
# verify_status_json.sh — Structural JSON validation for `tovarisch status --json`
#
# This script performs non-grep structural validation of the status JSON contract.
# It fails if:
#   - Output is not valid JSON
#   - Required top-level fields are missing
#   - Field types do not match expectations
#
# Usage:
#   ./scripts/verify_status_json.sh [json_string]
#
# When called with no arguments, reads from stdin.
# Returns exit code 0 on valid contract, non-zero on violation.

set -euo pipefail

# Required top-level fields for the status JSON contract
REQUIRED_FIELDS=("service" "version" "node_id" "status" "checks")

check_field() {
    local json="$1"
    local field="$2"

    # jq returns null and exits non-zero if field missing
    if ! jq -e "$field" <<<"$json" >/dev/null 2>&1; then
        echo "[verify_status-json] FAIL: missing required field: $field" >&2
        return 1
    fi
    return 0
}

check_type() {
    local json="$1"
    local field="$2"
    local expected_type="$3"

    local actual_type
    actual_type=$(jq -r "$field | type" <<<"$json" 2>/dev/null || echo "null")

    if [[ "$actual_type" != "$expected_type" ]]; then
        echo "[verify_status-json] FAIL: field '$field' has type '$actual_type', expected '$expected_type'" >&2
        return 1
    fi
    return 0
}

main() {
    local json

    if [[ $# -eq 0 ]]; then
        # Read from stdin
        if [[ -t 0 ]]; then
            echo "[verify_status-json] ERROR: no input provided; pipe JSON or pass as argument" >&2
            exit 1
        fi
        json=$(cat)
    else
        json="$1"
    fi

    # Step 1: Validate that input is parseable JSON
    if ! jq empty <<<"$json" 2>/dev/null; then
        echo "[verify_status-json] FAIL: not valid JSON" >&2
        exit 1
    fi

    # Step 2: Validate required fields exist
    for field in "${REQUIRED_FIELDS[@]}"; do
        check_field "$json" ".$field" || exit 1
    done

    # Step 3: Validate field types
    check_type "$json" ".service" "string" || exit 1
    check_type "$json" ".version" "string" || exit 1
    check_type "$json" ".node_id" "string" || exit 1
    check_type "$json" ".status" "string" || exit 1
    check_type "$json" ".checks" "array" || exit 1

    # Step 4: Validate service value (must be "tovarisch")
    local service_value
    service_value=$(jq -r '.service' <<<"$json")
    if [[ "$service_value" != "tovarisch" ]]; then
        echo "[verify_status-json] FAIL: service must be 'tovarisch', got '$service_value'" >&2
        exit 1
    fi

    # Step 5: Validate status value is one of: ok, warn, error
    local status_value
    status_value=$(jq -r '.status' <<<"$json")
    case "$status_value" in
        ok|warn|error) ;;
        *)
            echo "[verify_status-json] FAIL: status must be 'ok', 'warn', or 'error', got '$status_value'" >&2
            exit 1
            ;;
    esac

    # Step 6: Validate each check object in the checks array
    local check_count
    check_count=$(jq '.checks | length' <<<"$json")
    if [[ "$check_count" -gt 0 ]]; then
        for i in $(seq 0 $((check_count - 1))); do
            check_type "$json" ".checks[$i].name" "string" || exit 1
            check_type "$json" ".checks[$i].status" "string" || exit 1
            check_type "$json" ".checks[$i].detail" "string" || exit 1

            local check_status
            check_status=$(jq -r ".checks[$i].status" <<<"$json")
            case "$check_status" in
                ok|warn|error) ;;
                *)
                    echo "[verify_status-json] FAIL: checks[$i].status must be 'ok', 'warn', or 'error', got '$check_status'" >&2
                    exit 1
                    ;;
            esac
        done
    fi

    echo "[verify_status-json] PASS: status JSON contract valid"
    exit 0
}

main "$@"
