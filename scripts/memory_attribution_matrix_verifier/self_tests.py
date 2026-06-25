# self_tests.py — Self-test suite

import tempfile
from pathlib import Path

from .verify import verify_matrix
from .fixtures import create_matrix_fixture


def run_self_tests():
    print("=== Memory Attribution Matrix Verifier Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        tmppath = Path(tmpdir)
        
        print("Test 1: Valid full matrix passes")
        fixture = create_matrix_fixture(tmppath, "valid_matrix")
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 2: Valid partial matrix (2 variants) passes")
        fixture = create_matrix_fixture(tmppath, "partial", variants=["all_enabled", "no_periodic"])
        valid, error, _ = verify_matrix(fixture)
        if valid:
            print("  PASS")
            tests_passed += 1
        else:
            print(f"  FAIL: {error}")
            tests_failed += 1
        
        print("Test 3: Missing matrix summary fails")
        fixture = create_matrix_fixture(tmppath, "no_summary")
        (fixture / "matrix-summary.md").unlink()
        valid, error, _ = verify_matrix(fixture)
        if not valid and "matrix-summary.md" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 4: Missing declared variant fails")
        fixture = create_matrix_fixture(tmppath, "missing_var", variants=["all_enabled", "no_periodic"])
        import shutil
        shutil.rmtree(fixture / "no_periodic")
        valid, error, _ = verify_matrix(fixture)
        if not valid and "Missing" in error:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
        
        print("Test 5: Missing native events fails")
        fixture = create_matrix_fixture(tmppath, "no_native", variants=["all_enabled"])
        (fixture / "all_enabled" / "native_event_timeline.tsv").unlink()
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "native_event_timeline" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, failed_variants={failed_variants}")
            tests_failed += 1
        
        print("Test 6: Heartbeat leak when disabled fails")
        fixture = create_matrix_fixture(tmppath, "hb_leak", variants=["heartbeat_disabled"])
        native_path = fixture / "heartbeat_disabled" / "native_event_timeline.tsv"
        content = native_path.read_text()
        native_path.write_text(content.rstrip("\n") + "\n" + "2024-01-01T00:00:30\t30000\theartbeat_tick_start\theartbeat\t\t1234\n")
        valid, error, results = verify_matrix(fixture)
        failed_variants = [v for v, r in results.items() if not r["valid"] and "Heartbeat disabled" in r.get("error", "")]
        if not valid and failed_variants:
            print("  PASS (correctly rejected)")
            tests_passed += 1
        else:
            print(f"  FAIL: expected rejection, got valid={valid}, error={error}")
            tests_failed += 1
    
    print(f"\nResults: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0
