# self_tests.py — Self-test for matrix runner

from pathlib import Path
from .variants import MATRIX_VARIANTS


def run_self_tests() -> bool:
    """Run bounded self-tests for matrix runner module integrity."""
    print("=== Memory Attribution Matrix Runner Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    # Test 1: Verify variants are properly defined
    print("Test 1: Matrix variants defined")
    expected_variants = [
        "all_enabled", "heartbeat_disabled", "wg_disabled", 
        "bgp_disabled", "bfd_disabled", "bgp_bfd_disabled", "no_periodic"
    ]
    if set(MATRIX_VARIANTS.keys()) == set(expected_variants):
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: expected {expected_variants}, got {list(MATRIX_VARIANTS.keys())}")
        tests_failed += 1
    
    # Test 2: Verify variant configs have required fields
    print("Test 2: Variant configs have required fields")
    required_fields = {"description", "disable_heartbeat", "disable_wg_checks", "disable_bgp", "disable_bfd"}
    all_valid = True
    for name, config in MATRIX_VARIANTS.items():
        if required_fields - set(config.keys()):
            print(f"  FAIL: {name} missing fields {required_fields - set(config.keys())}")
            all_valid = False
    if all_valid:
        print("  PASS")
        tests_passed += 1
    else:
        tests_failed += 1
    
    # Test 3: Verify no_periodic disables all subsystems
    print("Test 3: no_periodic variant disables all subsystems")
    no_periodic = MATRIX_VARIANTS.get("no_periodic", {})
    if all(no_periodic.get(f) for f in ["disable_heartbeat", "disable_wg_checks", "disable_bgp", "disable_bfd"]):
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: no_periodic should disable all, got {no_periodic}")
        tests_failed += 1
    
    # Test 4: Verify all_enabled enables all subsystems
    print("Test 4: all_enabled variant enables all subsystems")
    all_enabled = MATRIX_VARIANTS.get("all_enabled", {})
    if not any(all_enabled.get(f) for f in ["disable_heartbeat", "disable_wg_checks", "disable_bgp", "disable_bfd"]):
        print("  PASS")
        tests_passed += 1
    else:
        print(f"  FAIL: all_enabled should enable all, got {all_enabled}")
        tests_failed += 1
    
    print(f"\nResults: {tests_passed} passed, {tests_failed} failed")
    return tests_failed == 0
