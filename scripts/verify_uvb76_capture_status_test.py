"""Self-test suite for capture status normalization.

Part of verify_uvb76_capture_helpers_test.py split.
Tests normalize_capture_status and extract_raw_status behavior.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from verify_uvb76_capture_helpers import (
    normalize_capture_status,
    extract_raw_status,
)


def run_tests() -> bool:
    """Run status normalization tests."""
    tests = [
        ("ok maps to captured", test_ok_to_captured),
        ("captured maps to captured", test_captured_to_captured),
        ("timeout maps to failed", test_timeout_to_failed),
        ("error maps to failed", test_error_to_failed),
        ("failed maps to failed", test_failed_to_failed),
        ("skipped_cooldown maps to skipped_cooldown", test_skipped_cooldown),
        ("disabled maps to disabled", test_disabled),
        ("not_configured maps to not_configured", test_not_configured),
        ("not_attempted maps to not_attempted", test_not_attempted),
        ("in_progress maps to pending", test_in_progress_to_pending),
        ("pending maps to pending", test_pending_to_pending),
        ("empty string maps to pending", test_empty_to_pending),
        ("unknown value maps to unknown", test_unknown_to_unknown),
        ("random value maps to unknown", test_random_to_unknown),
        ("EXTRACTION: capture_status non-empty uses capture_status", test_extract_capture_status),
        ("EXTRACTION: capture_status empty falls back to status", test_extract_empty_capture_status_fallback),
        ("EXTRACTION: missing capture_status falls back to status", test_extract_missing_capture_status_fallback),
        ("EXTRACTION: capture_status skipped_cooldown preserves value", test_extract_skipped_cooldown),
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
    print(f"\nStatus tests: {passed} passed, {failed} failed")
    return failed == 0


# === normalize_capture_status tests ===

def test_ok_to_captured() -> bool:
    return normalize_capture_status("ok") == "captured"

def test_captured_to_captured() -> bool:
    return normalize_capture_status("captured") == "captured"

def test_timeout_to_failed() -> bool:
    return normalize_capture_status("timeout") == "failed"

def test_error_to_failed() -> bool:
    return normalize_capture_status("error") == "failed"

def test_failed_to_failed() -> bool:
    return normalize_capture_status("failed") == "failed"

def test_skipped_cooldown() -> bool:
    return normalize_capture_status("skipped_cooldown") == "skipped_cooldown"

def test_disabled() -> bool:
    return normalize_capture_status("disabled") == "disabled"

def test_not_configured() -> bool:
    return normalize_capture_status("not_configured") == "not_configured"

def test_not_attempted() -> bool:
    return normalize_capture_status("not_attempted") == "not_attempted"

def test_in_progress_to_pending() -> bool:
    return normalize_capture_status("in_progress") == "pending"

def test_pending_to_pending() -> bool:
    return normalize_capture_status("pending") == "pending"

def test_empty_to_pending() -> bool:
    return normalize_capture_status("") == "pending"

def test_unknown_to_unknown() -> bool:
    return normalize_capture_status("unknown") == "unknown"

def test_random_to_unknown() -> bool:
    return normalize_capture_status("random") == "unknown"

# === extract_raw_status tests ===

def test_extract_capture_status() -> bool:
    capture = {"capture_status": "ok", "status": "timeout"}
    return extract_raw_status(capture) == "ok"

def test_extract_empty_capture_status_fallback() -> bool:
    capture = {"capture_status": "", "status": "timeout"}
    return extract_raw_status(capture) == "timeout"

def test_extract_missing_capture_status_fallback() -> bool:
    capture = {"status": "timeout"}
    return extract_raw_status(capture) == "timeout"

def test_extract_skipped_cooldown() -> bool:
    capture = {"capture_status": "skipped_cooldown", "status": "ok"}
    return extract_raw_status(capture) == "skipped_cooldown"


if __name__ == "__main__":
    print("[capture-status] Running tests")
    if run_tests():
        print("\n[capture-status] PASS")
        sys.exit(0)
    else:
        print("\n[capture-status] FAIL", file=sys.stderr)
        sys.exit(1)
