#!/bin/bash
# verify_uvb76_diag_packet_contract.sh — Diagnostic packet contract verifier
#
# Validates diagnostic packet JSON and spike/capture row semantics for UVB-76.
#
# Usage:
#   ./verify_uvb76_diag_packet_contract.sh --self-test
#   ./verify_uvb76_diag_packet_contract.sh --capture FILE [--row FILE]
#   ./verify_uvb76_diag_packet_contract.sh --dir DIR
#
# Exit codes: 0 = all checks passed, 1 = contract violation

set -euo pipefail

VERBOSE="${VERBOSE:-false}"
ERRORS=0

# Source library with fixtures
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/verify_uvb76_diag_packet_contract_lib.sh"

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*" >&2; ERRORS=$((ERRORS + 1)); }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
log_info() { [[ "$VERBOSE" == "true" ]] && echo "[INFO] $*" || true; }

# =============================================================================
# Row verification functions
# =============================================================================

verify_captured_row() {
    local row_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying captured row: $row_file (phase: $phase)"
    assert_jq "$row_file" '.capture_status == "captured"' "Captured row must have capture_status=captured (phase: $phase)" || ok=false
    assert_jq "$row_file" '.capture_exists == true' "Captured row must have capture_exists=true (phase: $phase)" || ok=false
    assert_jq "$row_file" '.is_protected == true' "Captured row must be protected (phase: $phase)" || ok=false
    local cv; cv=$(jq '.cooldown_info // null' "$row_file" 2>/dev/null || echo "null")
    [[ "$cv" != "null" ]] && log_fail "Captured row must NOT have cooldown_info (phase: $phase)" && ok=false
    [[ "$ok" == "true" ]] && log_pass "Captured row contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

verify_skipped_cooldown_row() {
    local row_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying skipped_cooldown row: $row_file (phase: $phase)"
    assert_jq "$row_file" '.capture_status == "skipped_cooldown"' "Skipped cooldown row must have capture_status=skipped_cooldown (phase: $phase)" || ok=false
    assert_jq "$row_file" '.capture_exists == false' "Skipped cooldown row must NOT advertise capture_exists (phase: $phase)" || ok=false
    assert_jq "$row_file" '.is_protected == false' "Skipped cooldown row must NOT be protected (phase: $phase)" || ok=false
    assert_jq "$row_file" '.cooldown_info != null' "Skipped cooldown row must include cooldown_info (phase: $phase)" || ok=false
    assert_jq "$row_file" '.cooldown_info.last_successful_capture_at != null' "Skipped cooldown row must expose last_successful_capture_at (phase: $phase)" || ok=false
    assert_jq "$row_file" '.cooldown_info.next_capture_eligible_at != null' "Skipped cooldown row must expose next_capture_eligible_at (phase: $phase)" || ok=false
    assert_jq "$row_file" '.cooldown_info.cooldown_seconds > 0' "Skipped cooldown row must have cooldown_seconds > 0 (phase: $phase)" || ok=false
    [[ "$ok" == "true" ]] && log_pass "Skipped cooldown row contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

verify_not_attempted_row() {
    local row_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying not_attempted row: $row_file (phase: $phase)"
    assert_jq "$row_file" '.capture_status == "not_attempted"' "Not attempted row must have capture_status=not_attempted (phase: $phase)" || ok=false
    assert_jq "$row_file" '.capture_exists == false' "Not attempted row must have capture_exists=false (phase: $phase)" || ok=false
    assert_jq "$row_file" '.is_protected == false' "Not attempted row must not be protected (phase: $phase)" || ok=false
    local cv; cv=$(jq '.cooldown_info // null' "$row_file" 2>/dev/null || echo "null")
    [[ "$cv" != "null" ]] && log_fail "Not attempted row must NOT have cooldown_info (phase: $phase)" && ok=false
    local sv; sv=$(jq '.suppressed_by_cooldown // null' "$row_file" 2>/dev/null || echo "null")
    [[ "$sv" == "true" ]] && log_fail "Not attempted row must NOT have suppressed_by_cooldown=true (phase: $phase)" && ok=false
    [[ "$ok" == "true" ]] && log_pass "Not attempted row contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

verify_failed_row() {
    local row_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying failed row: $row_file (phase: $phase)"
    assert_jq "$row_file" '.capture_status == "failed"' "Failed row must have capture_status=failed (phase: $phase)" || ok=false
    local cv; cv=$(jq '.cooldown_info // null' "$row_file" 2>/dev/null || echo "null")
    [[ "$cv" != "null" ]] && log_fail "Failed row must NOT have cooldown_info (phase: $phase)" && ok=false
    [[ "$ok" == "true" ]] && log_pass "Failed row contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

verify_disabled_row() {
    local row_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying disabled row: $row_file (phase: $phase)"
    assert_jq "$row_file" '.capture_status | . == "disabled" or . == "not_configured"' "Disabled row must have capture_status=disabled|not_configured (phase: $phase)" || ok=false
    assert_jq "$row_file" '.capture_exists == false' "Disabled row must have capture_exists=false (phase: $phase)" || ok=false
    assert_jq "$row_file" '.is_protected == false' "Disabled row must not be protected (phase: $phase)" || ok=false
    local cv; cv=$(jq '.cooldown_info // null' "$row_file" 2>/dev/null || echo "null")
    [[ "$cv" != "null" ]] && log_fail "Disabled row must NOT have cooldown_info (phase: $phase)" && ok=false
    [[ "$ok" == "true" ]] && log_pass "Disabled row contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

verify_packet_shape() {
    local packet_file="$1"; local phase="${2:-unknown}"; local ok=true
    log_info "Verifying packet shape: $packet_file (phase: $phase)"
    assert_jq "$packet_file" '.network_diag != null' "Packet must include network_diag (phase: $phase)" || ok=false
    assert_jq "$packet_file" '(.network_diag | type) == "object"' "Packet network_diag must be an object (phase: $phase)" || ok=false
    assert_jq "$packet_file" '(.network_diag.status | type) == "string"' "Packet network_diag.status must be a string (phase: $phase)" || ok=false
    assert_jq "$packet_file" '(.network_diag.started_at | type) == "string"' "Packet network_diag.started_at must be a string (phase: $phase)" || ok=false
    [[ "$ok" == "true" ]] && log_pass "Packet shape contract satisfied (phase: $phase)"
    [[ "$ok" == "true" ]] && return 0 || return 1
}

# =============================================================================
# Contract summary
# =============================================================================

write_contract_summary() {
    local phase="$1"; local row_file="$2"; local packet_file="$3"; local output_file="$4"
    log_info "Writing contract summary: $output_file"
    local cs="unknown"; local ce="false"; local ip="false"; local cip="false"; local np="false"; local no="false"; local pco="false"
    if [[ -f "$row_file" ]]; then
        cs=$(jq -r '.capture_status // "unknown"' "$row_file" 2>/dev/null || echo "unknown")
        ce=$(jq -r '.capture_exists // false' "$row_file" 2>/dev/null || echo "false")
        ip=$(jq -r '.is_protected // false' "$row_file" 2>/dev/null || echo "false")
        jq -e '.cooldown_info != null' "$row_file" >/dev/null 2>&1 && cip="true"
    fi
    if [[ -f "$packet_file" ]]; then
        jq -e '.network_diag != null' "$packet_file" >/dev/null 2>&1 && { np="true"; no="true"; }
        verify_packet_shape "$packet_file" "$phase" >/dev/null 2>&1 && pco="true"
    fi
    jq -n --arg phase "$phase" --arg cs "$cs" --argjson ce "$ce" --argjson ip "$ip" --argjson cip "$cip" --argjson np "$np" --argjson no "$no" --argjson pco "$pco" \
        '{phase: $phase, capture_status: $cs, capture_exists: $ce, is_protected: $ip, cooldown_info_present: $cip, network_diag_present: $np, network_diag_object: $no, packet_contract_ok: $pco}' > "$output_file"
}

# =============================================================================
# Self-test
# =============================================================================

run_self_test() {
    echo "=== Running self-test ==="; local test_dir; test_dir=$(mktemp -d "/tmp/uvb76-contract-test-XXXXXX"); local t_err=0; local t_pass=0
    run_test() {
        local name="$1"; local expected="$2"; shift 2; echo "--- Test: $name ---"
        local ec=0; "$@" 2>&1 || ec=$?
        if [[ "$expected" == "pass" ]]; then [[ $ec -eq 0 ]] && { echo "  [PASS] $name"; t_pass=$((t_pass + 1)); } || { echo "  [FAIL] $name (expected pass, got fail)"; t_err=$((t_err + 1)); }
        else [[ $ec -ne 0 ]] && { echo "  [PASS] $name (expected fail, got fail)"; t_pass=$((t_pass + 1)); } || { echo "  [FAIL] $name (expected fail, got pass)"; t_err=$((t_err + 1)); }; fi; echo ""
    }
    echo "$FIXTURE_GOOD_CAPTURED_ROW" > "$test_dir/good-captured-row.json"; run_test "Good captured row" pass verify_captured_row "$test_dir/good-captured-row.json" "self-test"
    echo "$FIXTURE_GOOD_CAPTURED_PACKET" > "$test_dir/good-packet.json"; run_test "Good captured packet shape" pass verify_packet_shape "$test_dir/good-packet.json" "self-test"
    echo "$FIXTURE_GOOD_SKIPPED_ROW" > "$test_dir/good-skipped-row.json"; run_test "Good skipped cooldown row" pass verify_skipped_cooldown_row "$test_dir/good-skipped-row.json" "self-test"
    echo "$FIXTURE_GOOD_NOT_ATTEMPTED_ROW" > "$test_dir/good-not-attempted.json"; run_test "Good not_attempted row" pass verify_not_attempted_row "$test_dir/good-not-attempted.json" "self-test"
    echo "$FIXTURE_BAD_SKIPPED_NO_COOLDOWN" > "$test_dir/bad-skipped-no-cooldown.json"; run_test "Bad: skipped_cooldown without cooldown_info" fail verify_skipped_cooldown_row "$test_dir/bad-skipped-no-cooldown.json" "self-test"
    echo "$FIXTURE_BAD_SKIPPED_NO_LAST" > "$test_dir/bad-skipped-no-last.json"; run_test "Bad: skipped_cooldown without last_successful_capture_at" fail verify_skipped_cooldown_row "$test_dir/bad-skipped-no-last.json" "self-test"
    echo "$FIXTURE_BAD_CAPTURED_NO_PACKET" > "$test_dir/bad-captured-no-packet.json"; run_test "Bad: captured packet with network_diag null" fail verify_packet_shape "$test_dir/bad-captured-no-packet.json" "self-test"
    echo "$FIXTURE_BAD_CAPTURED_NO_EXISTS" > "$test_dir/bad-captured-no-exists.json"; run_test "Bad: captured with capture_exists=false" fail verify_captured_row "$test_dir/bad-captured-no-exists.json" "self-test"
    rm -rf "$test_dir"; echo "=== Self-test Summary ==="; echo "Passed: $t_pass"; echo "Failed: $t_err"
    [[ $t_err -gt 0 ]] && echo "SELF-TEST FAILED" || echo "SELF-TEST PASSED"
    [[ $t_err -gt 0 ]] && return 1 || return 0
}

# =============================================================================
# Usage & main
# =============================================================================

usage() { cat <<EOF
Usage: $0 [OPTIONS]
Options:
  --self-test       Run self-test with good/bad fixtures
  --capture FILE    Verify capture packet FILE
  --row FILE        Verify spike/capture row FILE
  --phase PHASE     Phase name for reporting
  --dir DIR         Verify all phase artifacts in DIR
  --output FILE     Write contract summary to FILE
  --verbose         Enable verbose output
  -h, --help       Show this help
EOF
}

main() {
    local cap_file=""; local row_file=""; local phase="unknown"; local out_file=""; local verify_dir=""; local do_self_test=false
    while [[ $# -gt 0 ]]; do
        case "$1" in --self-test) do_self_test=true; shift ;; --capture) cap_file="$2"; shift 2 ;; --row) row_file="$2"; shift 2 ;;
            --phase) phase="$2"; shift 2 ;; --output) out_file="$2"; shift 2 ;; --dir) verify_dir="$2"; shift 2 ;;
            --verbose) VERBOSE=true; shift ;; -h|--help) usage; exit 0 ;; *) echo "Unknown: $1"; usage; exit 1 ;; esac
    done
    if [[ "$do_self_test" == "true" ]]; then run_self_test; exit $?; fi
    if [[ -n "$verify_dir" ]]; then
        local ok=true
        # Require ALL expected phase artifacts
        for req in \
            phase1-spike-row.json \
            phase1-capture-packet.json \
            phase2-spike-row.json \
            phase3-spike-row.json \
            phase3-capture-packet.json
        do
            if [[ ! -f "$verify_dir/$req" ]]; then
                echo "[FAIL] missing required artifact: $req" >&2
                ok=false
            fi
        done
        if [[ "$ok" != "true" ]]; then
            echo "[FAIL] Required phase artifacts missing" >&2
            exit 1
        fi
        # Normalize and verify each phase spike row
        for phase_row in "$verify_dir"/phase*-spike-row.json; do
            [[ -f "$phase_row" ]] || continue
            local phase; phase=$(basename "$phase_row" .json)
            echo "Verifying: $phase"
            
            # Normalize: extract capture info to temp file for validators
            local tmp_norm; tmp_norm=$(mktemp "/tmp/norm-XXXXXX.json")
            jq 'if .capture_status != null then . else {capture_status: (.captures[0].capture_status // null), capture_exists: (.captures[0].capture_exists // false), is_protected: (.captures[0].is_protected // false), cooldown_info: (.captures[0].cooldown_info // null)} end' "$phase_row" > "$tmp_norm" 2>/dev/null || true
            
            local cs; cs=$(jq -r '.capture_status // "unknown"' "$tmp_norm" 2>/dev/null || echo "unknown")
            
            case "$cs" in
                captured)
                    verify_captured_row "$tmp_norm" "$phase" || ok=false
                    local packet="${phase_row/spike-row/capture-packet}"
                    [[ -f "$packet" ]] && verify_packet_shape "$packet" "$phase" || ok=false
                    ;;
                skipped_cooldown)
                    verify_skipped_cooldown_row "$tmp_norm" "$phase" || ok=false
                    ;;
                not_attempted) verify_not_attempted_row "$tmp_norm" "$phase" || ok=false ;;
                failed) verify_failed_row "$tmp_norm" "$phase" || ok=false ;;
                disabled|not_configured) verify_disabled_row "$tmp_norm" "$phase" || ok=false ;;
                *) echo "[FAIL] $phase: unrecognized capture_status: $cs" >&2; ok=false ;;
            esac
            rm -f "$tmp_norm"
        done
        # Verify all phase packets
        for packet in "$verify_dir"/phase*-capture-packet.json; do
            [[ -f "$packet" ]] || continue
            local phase; phase=$(basename "$packet" .json)
            verify_packet_shape "$packet" "$phase" || ok=false
        done
        [[ "$ok" == "true" ]] && exit 0 || exit 1
    fi
    local ok=true
    if [[ -n "$row_file" ]]; then
        local cs; cs=$(jq -r '.capture_status // "unknown"' "$row_file" 2>/dev/null || echo "unknown")
        case "$cs" in captured) verify_captured_row "$row_file" "$phase" || ok=false ;; skipped_cooldown) verify_skipped_cooldown_row "$row_file" "$phase" || ok=false ;;
            not_attempted) verify_not_attempted_row "$row_file" "$phase" || ok=false ;; failed) verify_failed_row "$row_file" "$phase" || ok=false ;;
            disabled|not_configured) verify_disabled_row "$row_file" "$phase" || ok=false ;;
            *) echo "[FAIL] Unknown capture_status: $cs" >&2; ok=false ;;
        esac
    fi
    if [[ -n "$cap_file" ]]; then
        verify_packet_shape "$cap_file" "$phase" || ok=false
    fi
    if [[ -n "$out_file" ]]; then
        write_contract_summary "$phase" "$row_file" "$cap_file" "$out_file"
    fi
    [[ "$ok" == "true" ]] && exit 0 || exit 1
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
