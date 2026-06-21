"""Self-test suite for row normalization.

Part of verify_uvb76_capture_helpers_test.py split.
Tests normalize_spike_row_capture_contract behavior.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from verify_uvb76_capture_helpers import normalize_spike_row_capture_contract


def run_tests() -> bool:
    """Run row normalization tests."""
    tests = [
        ("missing captures[] -> not_attempted", test_normalize_no_captures),
        ("empty captures[] -> not_attempted", test_normalize_empty_captures),
        ("capture_status=ok -> captured", test_normalize_ok_to_captured),
        ("capture_status=captured -> captured", test_normalize_captured_to_captured),
        ("capture_status=timeout -> failed", test_normalize_timeout_to_failed),
        ("capture_status=skipped_cooldown -> skipped_cooldown", test_normalize_skipped_cooldown),
        ("capture_status=disabled -> disabled", test_normalize_disabled),
        ("capture_status=error -> failed", test_normalize_error_to_failed),
        ("captured row defaults capture_exists=true, is_protected=true", test_normalize_captured_defaults),
        ("failed row defaults capture_exists=false, is_protected=false", test_normalize_failed_defaults),
        ("captured row with cooldown_info has suppressed_by_cooldown=true", test_normalize_cooldown_info),
        ("REGRESSION: ok->captured not unknown", test_regression_ok_not_unknown),
        ("REGRESSION: captured->captured not unknown", test_regression_captured_not_unknown),
    ]
    passed = 0
    failed = 0
    for name, test_fn in tests:
        print(f"  Test: {name}")
        try:
            if test_fn():
                print("    OK")
                passed += 1
            else:
                print("    FAIL")
                failed += 1
        except Exception as e:
            print(f"    FAIL: {e}")
            failed += 1
    print(f"\nRow normalization tests: {passed} passed, {failed} failed")
    return failed == 0


def test_normalize_no_captures() -> bool:
    row = {"event_id": "evt-001"}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "not_attempted" and not result.capture_exists

def test_normalize_empty_captures() -> bool:
    row = {"event_id": "evt-001", "captures": []}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "not_attempted"

def test_normalize_ok_to_captured() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "captured"

def test_normalize_captured_to_captured() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "captured", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "captured"

def test_normalize_timeout_to_failed() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "", "status": "timeout"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "failed"

def test_normalize_skipped_cooldown() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "skipped_cooldown", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "skipped_cooldown"

def test_normalize_disabled() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "disabled", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "disabled"

def test_normalize_error_to_failed() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "", "status": "error"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "failed"

def test_normalize_captured_defaults() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_exists is True and result.is_protected is True

def test_normalize_failed_defaults() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "", "status": "timeout"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_exists is False and result.is_protected is False

def test_normalize_cooldown_info() -> bool:
    row = {
        "event_id": "evt-001",
        "captures": [{
            "capture_status": "skipped_cooldown",
            "status": "ok",
            "cooldown_info": {
                "last_successful_capture_at": "2026-01-01T00:00:00Z",
                "next_capture_eligible_at": "2026-01-01T00:10:00Z"
            }
        }]
    }
    result = normalize_spike_row_capture_contract(row)
    return result.suppressed_by_cooldown is True and result.cooldown_info is not None

def test_regression_ok_not_unknown() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "ok", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "captured" and result.capture_status != "unknown"

def test_regression_captured_not_unknown() -> bool:
    row = {"event_id": "evt-001", "captures": [{"capture_status": "captured", "status": "ok"}]}
    result = normalize_spike_row_capture_contract(row)
    return result.capture_status == "captured" and result.capture_status != "unknown"


if __name__ == "__main__":
    print("[capture-row-normalization] Running tests")
    if run_tests():
        print("\n[capture-row-normalization] PASS")
        sys.exit(0)
    else:
        print("\n[capture-row-normalization] FAIL", file=sys.stderr)
        sys.exit(1)
