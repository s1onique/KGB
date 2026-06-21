"""Self-test suite for packet save functionality.

Part of verify_uvb76_capture_helpers_test.py split.
Tests save_phase_capture_packet_from_raw_row behavior.
"""

import json
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from verify_uvb76_capture_helpers import save_phase_capture_packet_from_raw_row


def run_tests() -> bool:
    """Run packet save tests."""
    tests = [
        ("save_phase: direct network_diag extraction", test_save_direct),
        ("save_phase: packet.network_diag extraction", test_save_packet),
        ("save_phase: root network_diag extraction", test_save_root),
        ("save_phase: missing network_diag fails", test_save_missing_fails),
        ("save_phase: no captures fails", test_save_no_captures_fails),
        ("save_phase: missing arguments fails", test_save_missing_args_fails),
        ("save_phase: invalid JSON fails", test_save_invalid_json_fails),
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
    print(f"\nPacket save tests: {passed} passed, {failed} failed")
    return failed == 0


def test_save_direct() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text(json.dumps({
            "event_id": "evt-001",
            "captures": [{
                "status": "ok",
                "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
            }]
        }))
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("1", str(output_file), str(input_file))
        if not result:
            return False
        output = json.loads(output_file.read_text())
        return (output.get("phase") == "phase1" and 
                output.get("network_diag", {}).get("status") == "ok")

def test_save_packet() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text(json.dumps({
            "event_id": "evt-001",
            "captures": [{
                "status": "ok",
                "packet": {
                    "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
                }
            }]
        }))
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("2", str(output_file), str(input_file))
        if not result:
            return False
        output = json.loads(output_file.read_text())
        return output.get("network_diag", {}).get("status") == "ok"

def test_save_root() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text(json.dumps({
            "event_id": "evt-001",
            "network_diag": {"status": "ok", "started_at": "2026-01-01T00:00:00Z"}
        }))
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("7", str(output_file), str(input_file))
        if not result:
            return False
        output = json.loads(output_file.read_text())
        return output.get("network_diag", {}).get("status") == "ok"

def test_save_missing_fails() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text(json.dumps({
            "event_id": "evt-001",
            "captures": [{"status": "ok", "network_diag": None}]
        }))
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("4", str(output_file), str(input_file))
        return not result

def test_save_no_captures_fails() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text(json.dumps({"event_id": "evt-001"}))
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("5", str(output_file), str(input_file))
        return not result

def test_save_missing_args_fails() -> bool:
    result = save_phase_capture_packet_from_raw_row("", "", "")
    return not result

def test_save_invalid_json_fails() -> bool:
    with tempfile.TemporaryDirectory() as tmpdir:
        input_file = Path(tmpdir) / "input.json"
        input_file.write_text('{"broken": json}')
        output_file = Path(tmpdir) / "output.json"
        result = save_phase_capture_packet_from_raw_row("1", str(output_file), str(input_file))
        return not result


if __name__ == "__main__":
    print("[capture-packet-save] Running tests")
    if run_tests():
        print("\n[capture-packet-save] PASS")
        sys.exit(0)
    else:
        print("\n[capture-packet-save] FAIL", file=sys.stderr)
        sys.exit(1)
