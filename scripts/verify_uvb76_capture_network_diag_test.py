"""Self-test suite for network_diag extraction.

Part of verify_uvb76_capture_helpers_test.py split.
Tests get_network_diag_from_row behavior.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from verify_uvb76_capture_helpers import get_network_diag_from_row


def run_tests() -> bool:
    """Run network_diag extraction tests."""
    tests = [
        ("network_diag direct extraction", test_network_diag_direct),
        ("network_diag from packet.network_diag", test_network_diag_packet),
        ("network_diag from diagnostics.network_diag", test_network_diag_diagnostics),
        ("network_diag from root", test_network_diag_root),
        ("missing network_diag returns None", test_network_diag_missing),
        ("null network_diag returns None", test_network_diag_null),
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
    print(f"\nNetwork diag tests: {passed} passed, {failed} failed")
    return failed == 0


def test_network_diag_direct() -> bool:
    row = {
        "event_id": "evt-001",
        "captures": [{
            "status": "ok",
            "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
        }]
    }
    diag, path = get_network_diag_from_row(row)
    return diag is not None and path == "captures[0].network_diag"

def test_network_diag_packet() -> bool:
    row = {
        "event_id": "evt-001",
        "captures": [{
            "status": "ok",
            "packet": {
                "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
            }
        }]
    }
    diag, path = get_network_diag_from_row(row)
    return diag is not None and path == "captures[0].packet.network_diag"

def test_network_diag_diagnostics() -> bool:
    row = {
        "event_id": "evt-001",
        "captures": [{
            "status": "ok",
            "diagnostics": {
                "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
            }
        }]
    }
    diag, path = get_network_diag_from_row(row)
    return diag is not None and path == "captures[0].diagnostics.network_diag"

def test_network_diag_root() -> bool:
    row = {
        "event_id": "evt-001",
        "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
    }
    diag, path = get_network_diag_from_row(row)
    return diag is not None and path == "network_diag"

def test_network_diag_missing() -> bool:
    row = {"event_id": "evt-001", "captures": [{"status": "ok"}]}
    diag, path = get_network_diag_from_row(row)
    return diag is None and path is None

def test_network_diag_null() -> bool:
    row = {"event_id": "evt-001", "captures": [{"status": "ok", "network_diag": None}]}
    diag, path = get_network_diag_from_row(row)
    return diag is None and path is None


if __name__ == "__main__":
    print("[capture-network-diag] Running tests")
    if run_tests():
        print("\n[capture-network-diag] PASS")
        sys.exit(0)
    else:
        print("\n[capture-network-diag] FAIL", file=sys.stderr)
        sys.exit(1)
