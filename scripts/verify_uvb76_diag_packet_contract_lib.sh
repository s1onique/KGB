#!/bin/bash
# verify_uvb76_diag_packet_contract_lib.sh — Packet contract verifier library
#
# Shared library for verify_uvb76_diag_packet_contract.sh
# Contains fixtures for self-testing.

# Colors (defined in parent script)
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Error tracking (defined in parent script)
ERRORS=0
WARNINGS=0

log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*" >&2; ERRORS=$((ERRORS + 1)); }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*" >&2; WARNINGS=$((WARNINGS + 1)); }
log_info() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[INFO] $*" || true; }

# =============================================================================
# Helper: assert_jq FILE EXPR MESSAGE
# =============================================================================
assert_jq() {
    local file="$1"
    local expr="$2"
    local message="$3"
    
    if [[ ! -f "$file" ]]; then
        log_fail "$message (file not found: $file)"
        return 1
    fi
    
    if ! jq -e "$expr" "$file" >/dev/null 2>&1; then
        log_fail "$message"
        log_fail "  file=$file expr=$expr"
        return 1
    fi
    return 0
}

# =============================================================================
# Fixtures for self-test
# =============================================================================

# Good fixtures
FIXTURE_GOOD_CAPTURED_ROW='{"capture_status":"captured","capture_exists":true,"is_protected":true}'
FIXTURE_GOOD_CAPTURED_PACKET='{"network_diag":{"status":"ok","started_at":"2026-01-01T00:00:00Z"}}'
FIXTURE_GOOD_SKIPPED_ROW='{"capture_status":"skipped_cooldown","capture_exists":false,"is_protected":false,"cooldown_info":{"cooldown_scope":"per_target","last_successful_capture_at":"2026-01-01T00:00:00Z","next_capture_eligible_at":"2026-01-01T00:00:05Z","cooldown_seconds":5}}'
FIXTURE_GOOD_NOT_ATTEMPTED_ROW='{"capture_status":"not_attempted","capture_exists":false,"is_protected":false}'
FIXTURE_GOOD_FAILED_ROW='{"capture_status":"failed"}'
FIXTURE_GOOD_DISABLED_ROW='{"capture_status":"disabled","capture_exists":false,"is_protected":false}'

# Bad fixtures
FIXTURE_BAD_SKIPPED_NO_COOLDOWN='{"capture_status":"skipped_cooldown","capture_exists":false,"is_protected":false,"cooldown_info":null}'
FIXTURE_BAD_SKIPPED_NO_LAST='{"capture_status":"skipped_cooldown","capture_exists":false,"is_protected":false,"cooldown_info":{"next_capture_eligible_at":"2026-01-01T00:00:05Z","cooldown_seconds":5}}'
FIXTURE_BAD_SKIPPED_NO_NEXT='{"capture_status":"skipped_cooldown","capture_exists":false,"is_protected":false,"cooldown_info":{"last_successful_capture_at":"2026-01-01T00:00:00Z","cooldown_seconds":5}}'
FIXTURE_BAD_CAPTURED_NO_PACKET='{"network_diag":null}'
FIXTURE_BAD_CAPTURED_NO_EXISTS='{"capture_status":"captured","capture_exists":false,"is_protected":true}'
FIXTURE_BAD_CAPTURED_NOT_PROTECTED='{"capture_status":"captured","capture_exists":true,"is_protected":false}'
FIXTURE_BAD_NOT_ATTEMPTED_SUPPRESSED='{"capture_status":"not_attempted","capture_exists":false,"is_protected":false,"suppressed_by_cooldown":true}'
FIXTURE_BAD_FAILED_WITH_COOLDOWN='{"capture_status":"failed","cooldown_info":{"last_successful_capture_at":"2026-01-01T00:00:00Z","next_capture_eligible_at":"2026-01-01T00:00:05Z","cooldown_seconds":5}}'
