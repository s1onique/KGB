#!/bin/bash
# lab_uvb76_capture_netns_cooldown_helpers.sh — Cooldown-aware wait helpers
#
# Provides wait_until_cooldown_eligible() to wait for actual cooldown expiration
# using cooldown_info from Phase 2 row, rather than hardcoded sleep values.
#
# This ensures Phase 3 starts only after UVB-76 is truly eligible to capture,
# fixing the race condition where Phase 3 could start before cooldown expires.

# Global artifact path for cooldown wait summary (set by caller)
declare -g COOLDOWN_WAIT_SUMMARY_FILE=""

# Parse an ISO8601 timestamp to Unix epoch seconds.
# Returns 0 and outputs epoch on success, returns 1 on failure.
parse_iso8601_to_epoch() {
    local timestamp="$1"
    
    if [[ -z "$timestamp" || "$timestamp" == "null" || "$timestamp" == "empty" ]]; then
        return 1
    fi
    
    # Try GNU date first (Linux/GitHub Actions)
    if date -d "$timestamp" +%s >/dev/null 2>&1; then
        date -d "$timestamp" +%s
        return 0
    fi
    
    # Try BSD date (macOS) as fallback
    if date -j -f "%Y-%m-%dT%H:%M:%SZ" "$timestamp" +%s >/dev/null 2>&1; then
        date -j -f "%Y-%m-%dT%H:%M:%SZ" "$timestamp" +%s
        return 0
    fi
    
    return 1
}

# Get current UTC time as Unix epoch.
get_current_epoch() {
    date -u +%s
}

# Wait until cooldown is eligible based on Phase 2 cooldown_info.
#
# Arguments:
#   PHASE2_ROW_FILE   - Path to Phase 2 spike row JSON (contains cooldown_info)
#   SAFETY_SECONDS    - Additional seconds to wait beyond next_capture_eligible_at
#
# Behavior:
#   - Reads .cooldown_info.next_capture_eligible_at from phase2-spike-row.json
#   - Fails hard if missing, null, non-string, or unparsable
#   - Compares with current UTC time
#   - Sleeps until next_capture_eligible_at + safety margin
#   - If already eligible, continues immediately after logging
#   - Saves artifact: phase3-cooldown-wait-summary.json
#
# Artifact fields:
#   - last_successful_capture_at
#   - next_capture_eligible_at
#   - cooldown_seconds
#   - now_at_start
#   - computed_sleep_seconds
#   - safety_seconds
#   - now_after_wait
#
# Returns 0 on success, non-zero if:
#   - PHASE2_ROW_FILE is missing or invalid
#   - cooldown_info is missing or null
#   - next_capture_eligible_at is missing, null, or unparsable
wait_until_cooldown_eligible() {
    local phase2_row_file="$1"
    local safety_seconds="${2:-2}"
    
    log_info "=== Waiting for Cooldown to Expire ==="
    
    # Validate phase2 row file
    if [[ ! -f "$phase2_row_file" ]]; then
        log_error "[FAIL] wait_until_cooldown_eligible: phase2 row file not found: $phase2_row_file"
        return 1
    fi
    
    # Check cooldown_info exists
    if ! jq -e '.cooldown_info != null' "$phase2_row_file" >/dev/null 2>&1; then
        log_error "[FAIL] wait_until_cooldown_eligible: cooldown_info missing or null in phase2 row"
        log_error "This means Phase 2 did not properly capture cooldown information"
        return 1
    fi
    
    # Extract cooldown fields
    local next_capture_eligible_at last_successful_capture_at cooldown_seconds
    
    next_capture_eligible_at=$(jq -r '.cooldown_info.next_capture_eligible_at // "null"' "$phase2_row_file" 2>/dev/null)
    last_successful_capture_at=$(jq -r '.cooldown_info.last_successful_capture_at // "null"' "$phase2_row_file" 2>/dev/null)
    cooldown_seconds=$(jq -r '.cooldown_info.cooldown_seconds // "null"' "$phase2_row_file" 2>/dev/null)
    
    # Validate next_capture_eligible_at is present and valid
    if [[ -z "$next_capture_eligible_at" || "$next_capture_eligible_at" == "null" || "$next_capture_eligible_at" == "empty" ]]; then
        log_error "[FAIL] wait_until_cooldown_eligible: next_capture_eligible_at missing or null"
        log_error "Full cooldown_info:"
        jq '.cooldown_info' "$phase2_row_file" 2>/dev/null || echo "(failed to extract)"
        return 1
    fi
    
    # Parse next_capture_eligible_at to epoch
    local eligible_epoch
    if ! eligible_epoch=$(parse_iso8601_to_epoch "$next_capture_eligible_at"); then
        log_error "[FAIL] wait_until_cooldown_eligible: cannot parse next_capture_eligible_at: $next_capture_eligible_at"
        return 1
    fi
    
    local now_epoch
    now_epoch=$(get_current_epoch)
    
    # Calculate required sleep
    local target_epoch=$((eligible_epoch + safety_seconds))
    local computed_sleep=0
    
    if [[ "$now_epoch" -lt "$target_epoch" ]]; then
        computed_sleep=$((target_epoch - now_epoch))
    fi
    
    log_info "Cooldown info from Phase 2:"
    log_info "  last_successful_capture_at: $last_successful_capture_at"
    log_info "  next_capture_eligible_at:  $next_capture_eligible_at (epoch: $eligible_epoch)"
    log_info "  cooldown_seconds: $cooldown_seconds"
    log_info "  now (epoch): $now_epoch"
    log_info "  safety_seconds: $safety_seconds"
    log_info "  target epoch: $target_epoch"
    log_info "  computed sleep: ${computed_sleep}s"
    
    if [[ "$computed_sleep" -gt 0 ]]; then
        log_info "Waiting ${computed_sleep}s for cooldown to expire..."
        sleep "$computed_sleep"
    else
        log_info "Cooldown already expired - proceeding immediately"
    fi
    
    local now_after_wait
    now_after_wait=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    local now_after_epoch
    now_after_epoch=$(get_current_epoch)
    
    # Save cooldown wait summary artifact
    local summary_file="${COOLDOWN_WAIT_SUMMARY_FILE:-${LAB_DIR:-/tmp}/phase3-cooldown-wait-summary.json}"
    
    jq -n \
        --arg last_successful_capture_at "$last_successful_capture_at" \
        --arg next_capture_eligible_at "$next_capture_eligible_at" \
        --argjson cooldown_seconds "$([ "$cooldown_seconds" == "null" ] && echo "null" || echo "$cooldown_seconds")" \
        --argjson now_at_start "$now_epoch" \
        --argjson computed_sleep_seconds "$computed_sleep" \
        --argjson safety_seconds "$safety_seconds" \
        --arg now_after_wait "$now_after_wait" \
        --argjson now_after_epoch "$now_after_epoch" \
        '{
            last_successful_capture_at: $last_successful_capture_at,
            next_capture_eligible_at: $next_capture_eligible_at,
            cooldown_seconds: $cooldown_seconds,
            now_at_start: $now_at_start,
            computed_sleep_seconds: $computed_sleep_seconds,
            safety_seconds: $safety_seconds,
            now_after_wait: $now_after_wait,
            now_after_epoch: $now_after_epoch
        }' > "$summary_file"
    
    log_info "[PASS] Cooldown wait complete. Summary saved: $summary_file"
    return 0
}

# =============================================================================
# Self-test for cooldown helpers
# =============================================================================

run_cooldown_helpers_selftest() {
    local test_dir
    test_dir=$(mktemp -d "/tmp/cooldown-helper-selftest-XXXXXX")
    local errors=0
    local passes=0
    
    echo "=== Cooldown Helpers Self-Test ==="
    
    # Test parse_iso8601_to_epoch
    echo ""
    echo "Testing parse_iso8601_to_epoch..."
    
    # Test 1: Valid timestamp
    local epoch
    if epoch=$(parse_iso8601_to_epoch "2026-01-15T10:30:00Z"); then
        if [[ -n "$epoch" && "$epoch" =~ ^[0-9]+$ ]]; then
            echo "[PASS] Valid ISO8601 timestamp parsed to epoch: $epoch"
            passes=$((passes + 1))
        else
            echo "[FAIL] Valid timestamp returned invalid epoch: $epoch"
            errors=$((errors + 1))
        fi
    else
        echo "[FAIL] Failed to parse valid ISO8601 timestamp"
        errors=$((errors + 1))
    fi
    
    # Test 2: Null/empty input should fail
    if parse_iso8601_to_epoch "null" 2>/dev/null; then
        echo "[FAIL] null input should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] null input correctly fails"
        passes=$((passes + 1))
    fi
    
    if parse_iso8601_to_epoch "" 2>/dev/null; then
        echo "[FAIL] empty input should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] empty input correctly fails"
        passes=$((passes + 1))
    fi
    
    # Test 3: Invalid timestamp should fail
    if parse_iso8601_to_epoch "not-a-timestamp" 2>/dev/null; then
        echo "[FAIL] invalid timestamp should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] invalid timestamp correctly fails"
        passes=$((passes + 1))
    fi
    
    # Test wait_until_cooldown_eligible with fixtures
    echo ""
    echo "Testing wait_until_cooldown_eligible..."
    
    # Mock log functions
    log_info() { echo "[INFO] $*"; }
    log_warn() { echo "[WARN] $*" >&2; }
    log_error() { echo "[ERROR] $*" >&2; }
    
    # Test 4: Missing file should fail
    COOLDOWN_WAIT_SUMMARY_FILE="$test_dir/missing-summary.json"
    if wait_until_cooldown_eligible "/nonexistent/path.json" 2 2>/dev/null; then
        echo "[FAIL] Missing file should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] Missing file correctly fails"
        passes=$((passes + 1))
    fi
    
    # Test 5: Missing cooldown_info should fail
    echo '{"capture_status": "captured"}' > "$test_dir/no-cooldown-info.json"
    if wait_until_cooldown_eligible "$test_dir/no-cooldown-info.json" 2 2>/dev/null; then
        echo "[FAIL] Missing cooldown_info should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] Missing cooldown_info correctly fails"
        passes=$((passes + 1))
    fi
    
    # Test 6: Missing next_capture_eligible_at should fail
    echo '{"cooldown_info": {"last_successful_capture_at": "2026-01-01T00:00:00Z"}}' > "$test_dir/missing-eligible.json"
    if wait_until_cooldown_eligible "$test_dir/missing-eligible.json" 2 2>/dev/null; then
        echo "[FAIL] Missing next_capture_eligible_at should fail"
        errors=$((errors + 1))
    else
        echo "[PASS] Missing next_capture_eligible_at correctly fails"
        passes=$((passes + 1))
    fi
    
    # Test 7: Past eligible time => computed sleep 0
    echo '{"cooldown_info": {"last_successful_capture_at": "2020-01-01T00:00:00Z", "next_capture_eligible_at": "2020-01-01T00:00:00Z", "cooldown_seconds": 5}}' > "$test_dir/past-eligible.json"
    COOLDOWN_WAIT_SUMMARY_FILE="$test_dir/past-summary.json"
    
    # Initialize mock tracking variables before testing
    SLEEP_CALLED=false
    SLEEP_VALUE=""
    
    # Mock sleep to capture what sleep value would be used
    sleep() {
        SLEEP_CALLED=true
        SLEEP_VALUE=$1
    }
    
    if wait_until_cooldown_eligible "$test_dir/past-eligible.json" 2 2>/dev/null; then
        if [[ "$SLEEP_CALLED" != "true" ]]; then
            echo "[PASS] Past eligible time: no sleep needed (computed sleep 0)"
            passes=$((passes + 1))
        else
            echo "[FAIL] Past eligible time should not sleep, but slept for $SLEEP_VALUE"
            errors=$((errors + 1))
        fi
    else
        echo "[FAIL] Past eligible time should succeed"
        errors=$((errors + 1))
    fi
    
    # Restore sleep (just unset the function)
    unset -f sleep 2>/dev/null || true
    
    # Test 8: Verify summary artifact has correct fields
    if [[ -f "$test_dir/past-summary.json" ]]; then
        local summary_computed_sleep
        summary_computed_sleep=$(jq -r '.computed_sleep_seconds' "$test_dir/past-summary.json" 2>/dev/null || echo "null")
        if [[ "$summary_computed_sleep" == "0" ]]; then
            echo "[PASS] Summary artifact has correct computed_sleep_seconds: 0"
            passes=$((passes + 1))
        else
            echo "[FAIL] Summary artifact has wrong computed_sleep_seconds: $summary_computed_sleep (expected 0)"
            errors=$((errors + 1))
        fi
        
        # Verify all required fields are present
        local has_all_fields=true
        for field in last_successful_capture_at next_capture_eligible_at cooldown_seconds now_at_start computed_sleep_seconds safety_seconds now_after_wait; do
            local field_value
            field_value=$(jq -r ".$field" "$test_dir/past-summary.json" 2>/dev/null || echo "MISSING")
            if [[ "$field_value" == "MISSING" || "$field_value" == "null" ]]; then
                echo "[FAIL] Summary missing field: $field"
                has_all_fields=false
                errors=$((errors + 1))
            fi
        done
        if [[ "$has_all_fields" == "true" ]]; then
            echo "[PASS] Summary artifact has all required fields"
            passes=$((passes + 1))
        fi
    else
        echo "[FAIL] Summary artifact not created"
        errors=$((errors + 1))
    fi
    
    # Cleanup
    rm -rf "$test_dir"
    
    echo ""
    echo "=== Self-Test Summary ==="
    echo "Passed: $passes"
    echo "Failed: $errors"
    
    if [[ $errors -gt 0 ]]; then
        echo "SELF-TEST FAILED"
        return 1
    else
        echo "SELF-TEST PASSED"
        return 0
    fi
}

# Run self-test if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    # Source minimal dependencies for self-test
    log_info() { echo "[INFO] $*"; }
    log_warn() { echo "[WARN] $*" >&2; }
    log_error() { echo "[ERROR] $*" >&2; }
    
    run_cooldown_helpers_selftest
fi
