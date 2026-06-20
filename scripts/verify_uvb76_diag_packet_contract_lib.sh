# verify_uvb76_diag_packet_contract_lib.sh — Packet contract verifier library
# Shared library for verify_uvb76_diag_packet_contract.sh

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


# Good: TCP with sockets
FIXTURE_GOOD_TCP_WITH_SOCKETS='{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [
      {
        "name": "xray",
        "state": "ESTAB",
        "local": "127.0.0.1:8080",
        "remote": "127.0.0.1:9090",
        "rtt_ms": 0.5,
        "status": "ok"
      }
    ],
    "events": []
  }
}'

# Good: TCP absence with event
FIXTURE_GOOD_TCP_ABSENCE_WITH_EVENT='{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "no matching socket found for filter",
        "fields": "{\"reason\":\"no_matching_socket\",\"filter_port\":8080}"
      }
    ]
  }
}'

# Good: socket_closed_before_capture
FIXTURE_GOOD_TCP_ABSENCE_SOCKET_CLOSED='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "socket closed before capture completed",
        "fields": "{\"reason\":\"socket_closed_before_capture\"}"
      }
    ]
  }
}'

# Good: command_failed
FIXTURE_GOOD_TCP_ABSENCE_COMMAND_FAILED='{
  "network_diag": {
    "status": "unavailable",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "error",
        "source": "underlay_tcp",
        "message": "ss -tin command failed",
        "fields": "{\"reason\":\"command_failed\",\"exit_code\":127}"
      }
    ]
  }
}'

# Good: not_configured
FIXTURE_GOOD_TCP_ABSENCE_NOT_CONFIGURED='{
  "network_diag": {
    "status": "disabled",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "info",
        "source": "underlay_tcp",
        "message": "underlay TCP diagnostics disabled by config",
        "fields": "{\"reason\":\"not_configured\"}"
      }
    ]
  }
}'

# Good: parse_failed
FIXTURE_GOOD_TCP_ABSENCE_PARSE_FAILED='{
  "network_diag": {
    "status": "unavailable",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "error",
        "source": "underlay_tcp",
        "message": "failed to parse ss output",
        "fields": "{\"reason\":\"parse_failed\",\"detail\":\"UnexpectedToken\"}"
      }
    ]
  }
}'

# Bad: no event
FIXTURE_BAD_TCP_ABSENCE_NO_EVENT='{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": []
  }
}'

# Bad: warning-only event
FIXTURE_BAD_TCP_WARNING_ONLY='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "No TCP diagnostics captured"
      }
    ]
  }
}'

# Bad: event without fields
FIXTURE_BAD_TCP_NO_FIELDS_IN_EVENT='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "TCP diag unavailable"
      }
    ]
  }
}'

# Good: fields as object
FIXTURE_GOOD_TCP_FIELDS_AS_OBJECT='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "no matching socket found",
        "fields": {"reason": "no_matching_socket", "filter_port": 8080}
      }
    ]
  }
}'

# Bad: underlay_tcp is object
FIXTURE_BAD_TCP_UNDERLAY_IS_OBJECT='{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": {"count": 1},
    "events": []
  }
}'

# Bad: unknown reason
FIXTURE_BAD_TCP_UNKNOWN_REASON='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "TCP diag unavailable",
        "fields": "{\"reason\":\"lol\"}"
      }
    ]
  }
}'

# Bad: malformed JSON fields
FIXTURE_BAD_TCP_MALFORMED_FIELDS='{
  "network_diag": {
    "status": "warning",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": [],
    "events": [
      {
        "ts": "2026-01-01T00:00:00Z",
        "severity": "warning",
        "source": "underlay_tcp",
        "message": "TCP diag unavailable",
        "fields": "{bad json"
      }
    ]
  }
}'


# Good: capture with diag
FIXTURE_CAPTURE_OK_WITH_DIAG='{
  "capture_status": "ok",
  "capture_exists": true,
  "is_protected": true,
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}'

# Good: captured with diag
FIXTURE_CAPTURE_CAPTURED_WITH_DIAG='{
  "capture_status": "captured",
  "capture_exists": true,
  "is_protected": true,
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}'

# Bad: capture timeout
FIXTURE_CAPTURE_TIMEOUT_NO_DIAG='{
  "capture_status": "timeout",
  "capture_exists": true,
  "is_protected": false,
  "network_diag": null
}'

# Bad: ok without diag
FIXTURE_CAPTURE_OK_NO_DIAG='{
  "capture_status": "ok",
  "capture_exists": true,
  "is_protected": true,
  "network_diag": null
}'

# Bad: capture failed
FIXTURE_CAPTURE_FAILED='{
  "capture_status": "failed",
  "capture_exists": true,
  "is_protected": false,
  "network_diag": null
}'

# Bad: capture error
FIXTURE_CAPTURE_ERROR='{
  "capture_status": "error",
  "capture_exists": true,
  "is_protected": false,
  "network_diag": null
}'

# Bad: not_attempted
FIXTURE_CAPTURE_NOT_ATTEMPTED='{
  "capture_status": "not_attempted",
  "capture_exists": false,
  "is_protected": false,
  "network_diag": null
}'

# Good: skipped_cooldown
FIXTURE_SKIPPED_COOLDOWN_VALID='{
  "capture_status": "skipped_cooldown",
  "capture_exists": false,
  "is_protected": false,
  "cooldown_info": {
    "cooldown_scope": "per_target",
    "last_successful_capture_at": "2026-01-01T00:00:00Z",
    "next_capture_eligible_at": "2026-01-01T00:00:05Z",
    "cooldown_seconds": 5
  }
}'

# Bad: skipped_cooldown without prior capture
FIXTURE_SKIPPED_COOLDOWN_NO_PRIOR_CAPTURE='{
  "capture_status": "skipped_cooldown",
  "capture_exists": false,
  "is_protected": false,
  "cooldown_info": {
    "cooldown_scope": "per_target",
    "last_successful_capture_at": null,
    "next_capture_eligible_at": "2026-01-01T00:00:05Z",
    "cooldown_seconds": 5
  }
}'

# Bad: raw spike row with captures[0].status=ok but no network_diag
FIXTURE_RAW_CAPTURE_OK_NO_DIAG='{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "ok",
      "capture_status": "ok",
      "capture_exists": true,
      "is_protected": true,
      "network_diag": null
    }
  ]
}'

# Good: raw spike row with captures[0].status=ok and network_diag
FIXTURE_RAW_CAPTURE_OK_WITH_DIAG='{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "ok",
      "capture_status": "ok",
      "capture_exists": true,
      "is_protected": true,
      "network_diag": {
        "status": "ok",
        "started_at": "2026-01-01T00:00:00Z"
      }
    }
  ]
}'

# Good: raw spike row with captures[0].status=timeout (should fail Phase 1/3)
FIXTURE_RAW_CAPTURE_TIMEOUT='{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "timeout",
      "capture_status": "timeout",
      "capture_exists": true,
      "is_protected": false,
      "network_diag": null
    }
  ]
}'

# Good: raw spike row with captures[0].packet.network_diag (alternative shape)
FIXTURE_RAW_CAPTURE_PACKET_NESTED='{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "ok",
      "capture_status": "ok",
      "capture_exists": true,
      "is_protected": true,
      "packet": {
        "network_diag": {
          "status": "ok",
          "started_at": "2026-01-01T00:00:00Z"
        }
      }
    }
  ]
}'

# Good: raw spike row with captures[0].diagnostics.network_diag (alternative shape)
FIXTURE_RAW_CAPTURE_DIAGNOSTICS_NESTED='{
  "event_id": "evt-123",
  "captures": [
    {
      "status": "ok",
      "capture_status": "ok",
      "capture_exists": true,
      "is_protected": true,
      "diagnostics": {
        "network_diag": {
          "status": "ok",
          "started_at": "2026-01-01T00:00:00Z"
        }
      }
    }
  ]
}'
