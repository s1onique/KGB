#!/usr/bin/env python3
"""
Self-tests for memory lab artifact verifier.

Run with: python3 scripts/verify_memory_lab_artifact_tests.py
Or: python3 scripts/verify_memory_lab_artifact.py --self-test
"""

import copy
import json
import os
import tempfile

# Import from main verifier
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
import sys
sys.path.insert(0, SCRIPT_DIR)

import importlib.util


def load_verifier_module():
    """Load the main verifier module."""
    spec = importlib.util.spec_from_file_location(
        "verifier", os.path.join(SCRIPT_DIR, "verify_memory_lab_artifact.py")
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def run_self_tests() -> bool:
    """Run self-tests on the verifier."""
    verifier = load_verifier_module()

    print("\n=== Running Self-Tests ===\n")

    # Base fixtures
    valid_fixture = {
        "schema_version": "1.0", "evidence_kind": "schema_fixture",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {"type": "test", "operations": 100, "errors": 0, "duration_ms": 1000},
        "memory": {
            "first": {"rss_kib": 1000}, "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050}, "growth": {"rss_kib": 50, "rss_percent": 5.0}
        },
        "decision": {"pass": True, "reason": "test passed"},
    }

    valid_real = {
        "schema_version": "1.0", "evidence_kind": "real_evidence",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {"type": "test", "operations": 100, "errors": 0, "duration_ms": 1000},
        "memory": {
            "first": {"rss_kib": 1000}, "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050}, "growth": {"rss_kib": 50, "rss_percent": 5.0}
        },
        "decision": {"pass": True, "reason": "test passed"},
    }

    valid_leak_slope = {
        "schema_version": "1.0", "evidence_kind": "real_evidence",
        "service": {"name": "tovarisch", "version": "0.1.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {"type": "tovarisch-leak-slope", "operations": 100, "errors": 0, "duration_ms": 10000},
        "memory": {
            "first": {"rss_kib": 8000}, "max": {"rss_kib": 8200},
            "last": {"rss_kib": 8150}, "growth": {"rss_kib": 150, "rss_percent": 1.875}
        },
        "leak_slope": {
            "sampled_points": 5,
            "duration_seconds": 10.0,
            "rss_first_kib": 8000,
            "rss_max_kib": 8200,
            "rss_last_kib": 8150,
            "pss_first_kib": 6200,
            "pss_max_kib": 6400,
            "pss_last_kib": 6350,
            "rss_growth_kib": 150,
            "pss_growth_kib": 150,
            "rss_slope_kib_per_min": 9.0,
            "pss_slope_kib_per_min": 9.0,
            "request_count": 100,
            "request_errors": 0,
        },
        "decision": {"pass": True, "reason": "test passed"},
    }

    tests_passed = 0
    tests_failed = 0

    with tempfile.TemporaryDirectory() as tmpdir:
        # Test: valid fixture
        path = os.path.join(tmpdir, "fixture.json")
        with open(path, "w") as f:
            json.dump(valid_fixture, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) == 0:
            print("  PASS: Valid schema_fixture passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test: valid real_evidence
        path = os.path.join(tmpdir, "real.json")
        with open(path, "w") as f:
            json.dump(valid_real, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) == 0:
            print("  PASS: Valid real_evidence passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test: valid leak_slope artifact
        path = os.path.join(tmpdir, "leak_slope.json")
        with open(path, "w") as f:
            json.dump(valid_leak_slope, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) == 0:
            print("  PASS: Valid leak_slope artifact passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test: leak-slope with integer-valued duration/slope (int|float acceptance)
        bad = copy.deepcopy(valid_leak_slope)
        bad["leak_slope"]["duration_seconds"] = 5  # Integer instead of float
        bad["leak_slope"]["rss_slope_kib_per_min"] = 2400  # Integer slope value
        path = os.path.join(tmpdir, "leak_slope_int.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) == 0:
            print("  PASS: leak_slope with integer duration/slope passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test: missing PSS field fails
        bad = copy.deepcopy(valid_leak_slope)
        del bad["leak_slope"]["pss_first_kib"]
        path = os.path.join(tmpdir, "no_pss.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("pss_first_kib" in e for e in errors):
            print("  PASS: missing PSS field fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: bad growth math
        bad = copy.deepcopy(valid_real)
        bad["memory"]["growth"]["rss_kib"] = 999
        path = os.path.join(tmpdir, "bad.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("growth.rss_kib" in e for e in errors):
            print("  PASS: bad growth math fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: missing evidence_kind
        bad = copy.deepcopy(valid_fixture)
        del bad["evidence_kind"]
        path = os.path.join(tmpdir, "missing.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("evidence_kind" in e for e in errors):
            print("  PASS: missing evidence_kind fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: operations < errors
        bad = copy.deepcopy(valid_fixture)
        bad["workload"]["operations"] = 5
        bad["workload"]["errors"] = 10
        path = os.path.join(tmpdir, "badops.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("operations" in e for e in errors):
            print("  PASS: operations < errors fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: leak_slope workload missing leak_slope field
        bad = copy.deepcopy(valid_leak_slope)
        del bad["leak_slope"]
        path = os.path.join(tmpdir, "no_leak_slope.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("leak_slope is required" in e for e in errors):
            print("  PASS: leak-slope workload missing leak_slope fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: leak_slope request_count <= 0
        bad = copy.deepcopy(valid_leak_slope)
        bad["leak_slope"]["request_count"] = 0
        path = os.path.join(tmpdir, "bad_req_count.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("request_count" in e for e in errors):
            print("  PASS: leak_slope request_count <= 0 fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: leak_slope request_errors > request_count
        bad = copy.deepcopy(valid_leak_slope)
        bad["leak_slope"]["request_count"] = 10
        bad["leak_slope"]["request_errors"] = 15
        path = os.path.join(tmpdir, "bad_req_errors.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("request_errors" in e and "request_count" in e for e in errors):
            print("  PASS: leak_slope request_errors > request_count fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: duration_seconds=true fails (bool rejected for numeric field)
        bad = copy.deepcopy(valid_leak_slope)
        bad["leak_slope"]["duration_seconds"] = True
        path = os.path.join(tmpdir, "bool_duration.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("duration_seconds" in e and "number" in e for e in errors):
            print("  PASS: duration_seconds=true fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: request_count=true fails (bool rejected for int field)
        bad = copy.deepcopy(valid_leak_slope)
        bad["leak_slope"]["request_count"] = True
        path = os.path.join(tmpdir, "bool_count.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = verifier.validate_file(path)
        if len(errors) > 0 and any("request_count" in e and "int" in e for e in errors):
            print("  PASS: request_count=true fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


if __name__ == "__main__":
    success = run_self_tests()
    sys.exit(0 if success else 1)
