#!/bin/bash
# verify_uvb76_diag_packet_contract_selftest.sh — Self-test harness for packet contract verifier
# Sourced by verify_uvb76_diag_packet_contract.sh --self-test

# =============================================================================
# Self-test helpers
# =============================================================================

# expect_accept TEST_NAME COMMAND...
# Passes only if command exits 0; replays captured output on failure.
expect_accept() {
    local name="$1"; shift
    local tmp; tmp=$(mktemp "/tmp/st-accept-XXXXXX.log")
    set +e
    (set -e; "$@" >"$tmp" 2>&1)
    local ec=$?
    set -e
    if [[ $ec -eq 0 ]]; then
        rm -f "$tmp"
        echo "[PASS] $name"
        return 0
    else
        echo "[FAIL] $name"
        sed 's/^/  /' "$tmp"
        rm -f "$tmp"
        return 1
    fi
}

# expect_reject TEST_NAME COMMAND...
# Passes only if command exits non-zero; replays captured output on failure.
expect_reject() {
    local name="$1"; shift
    local tmp; tmp=$(mktemp "/tmp/st-reject-XXXXXX.log")
    set +e
    (set -e; "$@" >"$tmp" 2>&1)
    local ec=$?
    set -e
    if [[ $ec -ne 0 ]]; then
        rm -f "$tmp"
        echo "[PASS] $name"
        return 0
    else
        echo "[FAIL] $name expected rejection but verifier accepted fixture"
        sed 's/^/  /' "$tmp"
        rm -f "$tmp"
        return 1
    fi
}

# =============================================================================
# Meta self-test: proves harness fails hard on wrong expectations
# =============================================================================

run_self_test_harness_checks() {
    local failures=0
    local harness_output=""
    local tmp
    
    tmp=$(mktemp "/tmp/harness-good-XXXXXX.json")
    echo '{"capture_status":"captured","capture_exists":true,"is_protected":true}' > "$tmp"
    if expect_reject "Harness: expect_reject good fixture must fail" verify_captured_row "$tmp" "harness-test" >/dev/null 2>&1; then
        harness_output="${harness_output}[FAIL] Harness: expect_reject incorrectly passed on good fixture\n"
        failures=$((failures + 1))
    fi
    rm -f "$tmp"
    
    tmp=$(mktemp "/tmp/harness-bad-XXXXXX.json")
    echo '{"capture_status":"skipped_cooldown","capture_exists":false,"is_protected":false,"cooldown_info":null}' > "$tmp"
    if expect_accept "Harness: expect_accept bad fixture must fail" verify_skipped_cooldown_row "$tmp" "harness-test" >/dev/null 2>&1; then
        harness_output="${harness_output}[FAIL] Harness: expect_accept incorrectly passed on bad fixture\n"
        failures=$((failures + 1))
    fi
    rm -f "$tmp"
    
    if [[ $failures -gt 0 ]]; then
        echo "=== Harness self-test (proves harness fails hard) ===" >&2
        echo -e "$harness_output" >&2
        echo "--- Harness checks: $failures internal failures ---" >&2
        return 1
    fi
    return 0
}

# =============================================================================
# Self-test body
# =============================================================================

run_self_test_body() {
    echo "=== Running self-test ==="
    local test_dir; test_dir=$(mktemp -d "/tmp/uvb76-contract-test-XXXXXX")
    local failures=0
    
    echo "$FIXTURE_GOOD_CAPTURED_ROW" > "$test_dir/good-captured-row.json"
    expect_accept "Accepts: captured row with all required fields" verify_captured_row "$test_dir/good-captured-row.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_CAPTURED_PACKET" > "$test_dir/good-packet.json"
    expect_accept "Accepts: packet with network_diag object" verify_packet_shape "$test_dir/good-packet.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_SKIPPED_ROW" > "$test_dir/good-skipped-row.json"
    expect_accept "Accepts: skipped_cooldown row with cooldown_info" verify_skipped_cooldown_row "$test_dir/good-skipped-row.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_NOT_ATTEMPTED_ROW" > "$test_dir/good-not-attempted.json"
    expect_accept "Accepts: not_attempted row" verify_not_attempted_row "$test_dir/good-not-attempted.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_SKIPPED_NO_COOLDOWN" > "$test_dir/bad-skipped-no-cooldown.json"
    expect_reject "Rejects: skipped_cooldown without cooldown_info" verify_skipped_cooldown_row "$test_dir/bad-skipped-no-cooldown.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_SKIPPED_NO_LAST" > "$test_dir/bad-skipped-no-last.json"
    expect_reject "Rejects: skipped_cooldown without last_successful_capture_at" verify_skipped_cooldown_row "$test_dir/bad-skipped-no-last.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_CAPTURED_NO_PACKET" > "$test_dir/bad-captured-no-packet.json"
    expect_reject "Rejects: captured packet with network_diag null" verify_packet_shape "$test_dir/bad-captured-no-packet.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_CAPTURED_NO_EXISTS" > "$test_dir/bad-captured-no-exists.json"
    expect_reject "Rejects: captured with capture_exists=false" verify_captured_row "$test_dir/bad-captured-no-exists.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_WITH_SOCKETS" > "$test_dir/good-tcp-with-sockets.json"
    expect_accept "Accepts: TCP with non-empty underlay_tcp array" verify_tcp_diagnostics_contract "$test_dir/good-tcp-with-sockets.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_ABSENCE_WITH_EVENT" > "$test_dir/good-tcp-absence-event.json"
    expect_accept "Accepts: TCP absence with structured event" verify_tcp_diagnostics_contract "$test_dir/good-tcp-absence-event.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_ABSENCE_SOCKET_CLOSED" > "$test_dir/good-tcp-socket-closed.json"
    expect_accept "Accepts: TCP socket_closed_before_capture reason" verify_tcp_diagnostics_contract "$test_dir/good-tcp-socket-closed.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_ABSENCE_COMMAND_FAILED" > "$test_dir/good-tcp-command-failed.json"
    expect_accept "Accepts: TCP command_failed reason" verify_tcp_diagnostics_contract "$test_dir/good-tcp-command-failed.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_ABSENCE_NOT_CONFIGURED" > "$test_dir/good-tcp-not-configured.json"
    expect_accept "Accepts: TCP not_configured reason" verify_tcp_diagnostics_contract "$test_dir/good-tcp-not-configured.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_ABSENCE_PARSE_FAILED" > "$test_dir/good-tcp-parse-failed.json"
    expect_accept "Accepts: TCP parse_failed reason" verify_tcp_diagnostics_contract "$test_dir/good-tcp-parse-failed.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_GOOD_TCP_FIELDS_AS_OBJECT" > "$test_dir/good-tcp-fields-as-object.json"
    expect_accept "Accepts: TCP fields as object with reason" verify_tcp_diagnostics_contract "$test_dir/good-tcp-fields-as-object.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_ABSENCE_NO_EVENT" > "$test_dir/bad-tcp-no-event.json"
    expect_reject "Rejects: TCP absence with no event" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-no-event.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_WARNING_ONLY" > "$test_dir/bad-tcp-warning-only.json"
    expect_reject "Rejects: TCP warning-only without structured reason" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-warning-only.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_NO_FIELDS_IN_EVENT" > "$test_dir/bad-tcp-no-fields.json"
    expect_reject "Rejects: TCP event without fields" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-no-fields.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_UNDERLAY_IS_OBJECT" > "$test_dir/bad-tcp-underlay-is-object.json"
    expect_reject "Rejects: TCP underlay_tcp is object, not array" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-underlay-is-object.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_UNKNOWN_REASON" > "$test_dir/bad-tcp-unknown-reason.json"
    expect_reject "Rejects: TCP unknown reason" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-unknown-reason.json" "self-test" || failures=$((failures + 1))
    
    echo "$FIXTURE_BAD_TCP_MALFORMED_FIELDS" > "$test_dir/bad-tcp-malformed-fields.json"
    expect_reject "Rejects: TCP malformed fields JSON" verify_tcp_diagnostics_contract "$test_dir/bad-tcp-malformed-fields.json" "self-test" || failures=$((failures + 1))
    
    rm -rf "$test_dir"
    echo "--- Self-test: $failures failures ---"
    [[ $failures -eq 0 ]] && return 0 || return 1
}
