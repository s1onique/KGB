"""Self-test suite for status contract verifier.

Regression tests covering contract validation behavior.
"""

import json
import sys
from pathlib import Path

# Import from main verifier (add parent to path)
sys.path.insert(0, str(Path(__file__).parent))
from verify_tovarisch_status_contract import (
    validate_fixture, normalize_for_comparison, CliResult
)


def run_tests() -> bool:
    """Run all self-test cases. Returns True if all pass."""
    results = []
    tests = [
        # Contract validation tests
        ("valid status JSON passes", test_valid_json),
        ("malformed JSON fails", test_malformed_json),
        ("missing required field fails", test_missing_field),
        ("wrong type fails", test_wrong_type),
        ("invalid status value fails", test_invalid_status),
        ("first check not process fails", test_wrong_first_check),
        ("negative PID fails", test_negative_pid),
        ("negative RSS fails", test_negative_rss),
        ("null RSS is valid", test_null_rss),
        ("extra fields ignored", test_extra_fields),
        # Normalization tests
        ("equivalent fixture/CLI after normalization passes", test_normalization_equivalent),
        ("different non-runtime field fails", test_normalization_different_field),
        ("null and numeric rss_kib normalize to same sentinel", test_normalization_rss_sentinel),
        # CliResult tests
        ("CliResult unavailable is distinct from error", test_cli_result_unavailable),
        ("CliResult error contains failure message", test_cli_result_error),
        ("CliResult data contains parsed JSON", test_cli_result_data),
    ]
    
    for name, test_fn in tests:
        print(f"  Test: {name}")
        try:
            if test_fn():
                print("    OK")
                results.append(True)
            else:
                print("    FAIL")
                results.append(False)
        except Exception as e:
            print(f"    FAIL: {e}")
            results.append(False)
    
    return all(results)


def test_valid_json() -> bool:
    """Valid status JSON should pass validation."""
    valid_data = {
        "service": "tovarisch",
        "version": "0.1.2+abc123",
        "node_id": "local-dev",
        "status": "warn",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1234, "rss_kib": None},
    }
    errors = validate_fixture(valid_data)
    return len(errors) == 0


def test_malformed_json() -> bool:
    """Malformed JSON should raise JSONDecodeError."""
    try:
        json.loads('{"broken": json}')
        return False
    except json.JSONDecodeError:
        return True


def test_missing_field() -> bool:
    """Missing required top-level field should fail."""
    missing_service = {
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [],
        "runtime": {"pid": 1, "rss_kib": 0},
    }
    errors = validate_fixture(missing_service)
    return len(errors) > 0 and any("service" in str(e) for e in errors)


def test_wrong_type() -> bool:
    """Wrong type for required field should fail."""
    wrong_type = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": "not-an-array",
        "runtime": {"pid": 1, "rss_kib": 0},
    }
    errors = validate_fixture(wrong_type)
    return len(errors) > 0 and any("checks" in str(e) for e in errors)


def test_invalid_status() -> bool:
    """Invalid status value should fail."""
    invalid_status = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "invalid_value",
        "checks": [],
        "runtime": {"pid": 1, "rss_kib": 0},
    }
    errors = validate_fixture(invalid_status)
    return len(errors) > 0 and any("status" in str(e) for e in errors)


def test_wrong_first_check() -> bool:
    """First check not 'process' should fail."""
    wrong_first = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [{"name": "wrong", "status": "ok", "detail": "test"}],
        "runtime": {"pid": 1, "rss_kib": 0},
    }
    errors = validate_fixture(wrong_first)
    return len(errors) > 0 and any("checks[0].name" in str(e) for e in errors)


def test_negative_pid() -> bool:
    """Negative PID should fail."""
    data = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": -1, "rss_kib": 0},
    }
    errors = validate_fixture(data)
    return len(errors) > 0 and any("pid" in str(e) for e in errors)


def test_negative_rss() -> bool:
    """Negative RSS should fail."""
    data = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1, "rss_kib": -100},
    }
    errors = validate_fixture(data)
    return len(errors) > 0 and any("rss_kib" in str(e) for e in errors)


def test_null_rss() -> bool:
    """Null RSS is valid."""
    data = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1, "rss_kib": None},
    }
    errors = validate_fixture(data)
    return len(errors) == 0


def test_extra_fields() -> bool:
    """Extra fields should be ignored (future-compatible)."""
    data = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1, "rss_kib": 0},
        "extra_field": "ignored",
        "another_extra": 12345,
    }
    errors = validate_fixture(data)
    return len(errors) == 0


def test_normalization_equivalent() -> bool:
    """Equivalent fixture/CLI after runtime normalization should be equal."""
    fixture = {
        "service": "tovarisch",
        "version": "0.1.2+abc123",
        "node_id": "local-dev",
        "status": "warn",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1044345, "rss_kib": None},
    }
    cli = {
        "service": "tovarisch",
        "version": "0.1.2+def456",
        "node_id": "local-dev",
        "status": "warn",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 99999, "rss_kib": 12345},
    }
    fixture_norm = normalize_for_comparison(fixture)
    cli_norm = normalize_for_comparison(cli)
    return fixture_norm == cli_norm


def test_normalization_different_field() -> bool:
    """Different non-runtime field should fail normalization comparison."""
    fixture = {
        "service": "tovarisch",
        "version": "0.1.2+abc123",
        "node_id": "local-dev",
        "status": "warn",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 1044345, "rss_kib": None},
    }
    cli = {
        "service": "tovarisch",
        "version": "0.1.2+def456",
        "node_id": "different-node",  # Different!
        "status": "warn",
        "checks": [{"name": "process", "status": "ok", "detail": "running"}],
        "runtime": {"pid": 99999, "rss_kib": 12345},
    }
    fixture_norm = normalize_for_comparison(fixture)
    cli_norm = normalize_for_comparison(cli)
    return fixture_norm != cli_norm


def test_normalization_rss_sentinel() -> bool:
    """Null and numeric rss_kib should normalize to same sentinel."""
    data_null = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [],
        "runtime": {"pid": 100, "rss_kib": None},
    }
    data_numeric = {
        "service": "tovarisch",
        "version": "0.1.2+abc",
        "node_id": "local-dev",
        "status": "ok",
        "checks": [],
        "runtime": {"pid": 200, "rss_kib": 54321},
    }
    norm_null = normalize_for_comparison(data_null)
    norm_numeric = normalize_for_comparison(data_numeric)
    # Both should have rss_kib == "normalized"
    return (norm_null["runtime"]["rss_kib"] == "normalized" and 
            norm_numeric["runtime"]["rss_kib"] == "normalized" and
            norm_null["runtime"]["pid"] == norm_numeric["runtime"]["pid"] == 1)


def test_cli_result_unavailable() -> bool:
    """CliResult unavailable is distinct from error."""
    result = CliResult(unavailable=True)
    return result.unavailable and result.data is None and result.error is None


def test_cli_result_error() -> bool:
    """CliResult error contains failure message."""
    result = CliResult(error="zig build failed")
    return not result.unavailable and result.data is None and result.error == "zig build failed"


def test_cli_result_data() -> bool:
    """CliResult data contains parsed JSON."""
    data = {"service": "tovarisch", "version": "0.1.2+abc"}
    result = CliResult(data=data)
    return not result.unavailable and result.data == data and result.error is None


if __name__ == "__main__":
    print("[status-contract] Running self-test suite")
    if run_tests():
        print("")
        print("[status-contract] Self-test PASS")
        sys.exit(0)
    else:
        print("")
        print("[status-contract] Self-test FAIL", file=sys.stderr)
        sys.exit(1)
