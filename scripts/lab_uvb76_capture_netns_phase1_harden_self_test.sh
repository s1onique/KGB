#!/bin/bash
# lab_uvb76_capture_netns_phase1_harden_self_test.sh — Self-test for jq probe detection
#
# Extracted from lab_uvb76_capture_netns_phase1_harden.sh to stay under LLM-friendly limits.

# =============================================================================
# Self-test for jq probe detection
# =============================================================================

# The tovarisch-compatible predicate used in hardener assertions.
# Accepts: requests[], started_at+completed_at, tcp_info, xray_result,
# OR started_at + canonical sections (interfaces, routes, underlay_tcp, tcp_absence_events)
NETWORK_DIAG_REAL_PREDICATE='
.network_diag as $d
| ($d != null)
and (($d.status // "") != "suppressed")
and (
  (($d.requests // []) | length > 0)
  or (($d.started_at // "") != "" and ($d.completed_at // "") != "")
  or ($d.tcp_info != null)
  or ($d.xray_result != null)
  # tovarisch network_diag canonical sections
  or (($d.started_at // "") != "" and (
    ($d.interfaces != null)
    or ($d.routes != null)
    or ($d.underlay_tcp != null)
    or ($d.tcp_absence_events != null)
  ))
)
'

# Run self-test to prove jq probes work correctly.
# Tests that the probes correctly detect real diagnostic fields.
run_phase1_harden_self_test() {
    local failures=0
    echo "=== Phase 1 Harden Self-Test ==="
    
    # Test 1: Packet with started_at and completed_at should pass
    local test_packet_1; test_packet_1=$(mktemp "/tmp/test-packet-1-XXXXXX.json")
    cat > "$test_packet_1" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "completed_at": "2026-01-01T00:00:01Z"
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_1" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects started_at+completed_at as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect started_at+completed_at"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_1"
    
    # Test 2: Packet with requests array should pass
    local test_packet_2; test_packet_2=$(mktemp "/tmp/test-packet-2-XXXXXX.json")
    cat > "$test_packet_2" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "requests": [{"url": "http://example.com"}]
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_2" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects requests[] as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect requests[]"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_2"
    
    # Test 3: Packet with tcp_info should pass
    local test_packet_3; test_packet_3=$(mktemp "/tmp/test-packet-3-XXXXXX.json")
    cat > "$test_packet_3" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "tcp_info": {"state": "ESTABLISHED"}
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_3" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects tcp_info as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect tcp_info"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_3"
    
    # Test 4: Packet with xray_result should pass
    local test_packet_4; test_packet_4=$(mktemp "/tmp/test-packet-4-XXXXXX.json")
    cat > "$test_packet_4" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "xray_result": {"connections": []}
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_4" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects xray_result as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect xray_result"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_4"
    
    # Test 5: Suppressed packet should fail
    local test_packet_5; test_packet_5=$(mktemp "/tmp/test-packet-5-XXXXXX.json")
    cat > "$test_packet_5" <<'EOF'
{
  "network_diag": {
    "status": "suppressed"
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_5" >/dev/null 2>&1; then
        echo "[FAIL] Self-test: suppressed packet should NOT pass"
        failures=$((failures + 1))
    else
        echo "[PASS] Self-test: correctly rejects suppressed status"
    fi
    rm -f "$test_packet_5"
    
    # Test 6: Empty packet (no network_diag) should fail
    local test_packet_6; test_packet_6=$(mktemp "/tmp/test-packet-6-XXXXXX.json")
    echo '{}' > "$test_packet_6"
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_6" >/dev/null 2>&1; then
        echo "[FAIL] Self-test: empty packet should NOT pass"
        failures=$((failures + 1))
    else
        echo "[PASS] Self-test: correctly rejects empty packet"
    fi
    rm -f "$test_packet_6"
    
    # Test 7: started_at ONLY (no completed_at, no canonical sections) should FAIL
    local test_packet_7; test_packet_7=$(mktemp "/tmp/test-packet-7-XXXXXX.json")
    cat > "$test_packet_7" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z"
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_7" >/dev/null 2>&1; then
        echo "[FAIL] Self-test: started_at only should NOT pass"
        failures=$((failures + 1))
    else
        echo "[PASS] Self-test: correctly rejects started_at-only payload"
    fi
    rm -f "$test_packet_7"
    
    # Test 8: started_at + interfaces should PASS (tovarisch canonical section)
    local test_packet_8; test_packet_8=$(mktemp "/tmp/test-packet-8-XXXXXX.json")
    cat > "$test_packet_8" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "interfaces": [{"name": "eth0", "operstate": "up"}]
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_8" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects started_at+interfaces as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect started_at+interfaces"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_8"
    
    # Test 9: started_at + routes should PASS (tovarisch canonical section)
    local test_packet_9; test_packet_9=$(mktemp "/tmp/test-packet-9-XXXXXX.json")
    cat > "$test_packet_9" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "routes": []
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_9" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects started_at+routes as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect started_at+routes"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_9"
    
    # Test 10: started_at + underlay_tcp should PASS (tovarisch canonical section)
    local test_packet_10; test_packet_10=$(mktemp "/tmp/test-packet-10-XXXXXX.json")
    cat > "$test_packet_10" <<'EOF'
{
  "network_diag": {
    "status": "ok",
    "started_at": "2026-01-01T00:00:00Z",
    "underlay_tcp": []
  }
}
EOF
    if jq -e "$NETWORK_DIAG_REAL_PREDICATE" "$test_packet_10" >/dev/null 2>&1; then
        echo "[PASS] Self-test: detects started_at+underlay_tcp as real diagnostic"
    else
        echo "[FAIL] Self-test: failed to detect started_at+underlay_tcp"
        failures=$((failures + 1))
    fi
    rm -f "$test_packet_10"
    
    echo "--- Self-test: $failures failures ---"
    [[ $failures -eq 0 ]] && return 0 || return 1
}
