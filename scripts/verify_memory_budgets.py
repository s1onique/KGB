#!/usr/bin/env python3
"""
Verifier for memory budget YAML files.

Validates:
- Budget YAML files exist and are valid YAML
- Schema compliance with embedded-memory-frugality doctrine
- No invalid budget values (strings where integers expected)
- baseline_required values are marked appropriately
"""

import os
import sys
import yaml
import tempfile
import shutil
from typing import List, Tuple, Any, Dict

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BUDGETS_DIR = os.path.join(REPO_ROOT, "docs", "memory", "budgets")

REQUIRED_BUDGET_FILES = [
    "tovarisch-memory-budget.yaml",
    "uvb76-memory-budget.yaml",
]

REQUIRED_FIELDS = [
    "version",
    "service",
    "arch_budgets",
    "enforcement",
]

REQUIRED_ENFORCEMENT_FIELDS = [
    "gate_level",
    "fail_on_budget_exceeded",
]


def validate_budget_yaml(path: str) -> Tuple[List[str], Dict[str, Any]]:
    """Validate a single budget YAML file. Returns (errors, data)."""
    errors = []
    data = None
    
    # Check file exists
    if not os.path.exists(path):
        return [f"Budget file does not exist: {path}"], None
    
    # Parse YAML
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        return [f"Invalid YAML in {path}: {e}"], None
    
    if data is None:
        return [f"Empty YAML file: {path}"], None
    
    # Validate required top-level fields
    for field in REQUIRED_FIELDS:
        if field not in data:
            errors.append(f"Missing required field '{field}' in {path}")
    
    # Validate version
    if "version" in data and data["version"] != "1.0":
        errors.append(f"Unsupported budget version '{data['version']}' in {path}")
    
    # Validate service name matches filename
    filename = os.path.basename(path)
    if "service" in data:
        expected_service = filename.replace("-memory-budget.yaml", "")
        if data["service"] != expected_service:
            errors.append(
                f"Service name '{data['service']}' does not match "
                f"filename '{filename}' (expected '{expected_service}')"
            )
    
    # Validate arch_budgets structure
    if "arch_budgets" in data:
        for arch, budgets in data["arch_budgets"].items():
            if not isinstance(budgets, dict):
                errors.append(f"arch_budgets.{arch} is not a dict")
                continue
            
            # Check for required states
            for state in ["idle", "warm"]:
                if state in budgets:
                    state_data = budgets[state]
                    if not isinstance(state_data, dict):
                        errors.append(f"arch_budgets.{arch}.{state} is not a dict")
                        continue
                    
                    # Check RSS field
                    if "rss_kib" in state_data:
                        val = state_data["rss_kib"]
                        if val != "baseline_required" and not isinstance(val, (int, float)):
                            errors.append(
                                f"arch_budgets.{arch}.{state}.rss_kib must be "
                                f"baseline_required or number, got {type(val).__name__}"
                            )
    
    # Validate enforcement section
    if "enforcement" in data:
        for field in REQUIRED_ENFORCEMENT_FIELDS:
            if field not in data["enforcement"]:
                errors.append(f"Missing required enforcement field '{field}' in {path}")
    
    return errors, data


def check_baseline_required_count(data: Dict[str, Any]) -> Tuple[int, int]:
    """Count baseline_required vs actual values. Returns (baseline_count, actual_count)."""
    baseline_count = 0
    actual_count = 0
    
    def traverse(obj):
        nonlocal baseline_count, actual_count
        if isinstance(obj, dict):
            for key, value in obj.items():
                if key.endswith("_kib") or key.endswith("_bytes") or key.endswith("_max"):
                    if value == "baseline_required":
                        baseline_count += 1
                    elif isinstance(value, (int, float)):
                        actual_count += 1
                else:
                    traverse(value)
        elif isinstance(obj, list):
            for item in obj:
                traverse(item)
    
    traverse(data)
    return baseline_count, actual_count


def run_verifier() -> List[str]:
    """Run the memory budget verifier. Returns list of errors."""
    all_errors = []
    
    print("=== Memory Budget Verifier ===\n")
    
    # Check budgets directory exists
    if not os.path.isdir(BUDGETS_DIR):
        all_errors.append(f"Budgets directory does not exist: {BUDGETS_DIR}")
        return all_errors
    
    print(f"A. Checking required budget files...")
    
    for filename in REQUIRED_BUDGET_FILES:
        path = os.path.join(BUDGETS_DIR, filename)
        print(f"  Checking: {filename}")
        
        errors, data = validate_budget_yaml(path)
        if errors:
            for e in errors:
                print(f"    ERROR: {e}")
            all_errors.extend(errors)
        else:
            print(f"    OK: Valid YAML with correct schema")
            
            # Show baseline vs actual count
            if data:
                baseline_count, actual_count = check_baseline_required_count(data)
                total = baseline_count + actual_count
                pct_baseline = (baseline_count / total * 100) if total > 0 else 0
                print(f"    Baseline required: {baseline_count}/{total} ({pct_baseline:.0f}%)")
    
    print("\nB. Checking budget file schema consistency...")
    
    # Check both files have consistent structure
    budgets_data = {}
    for filename in REQUIRED_BUDGET_FILES:
        path = os.path.join(BUDGETS_DIR, filename)
        _, data = validate_budget_yaml(path)
        if data:
            budgets_data[filename] = data
    
    if len(budgets_data) == 2:
        # Both files loaded; check they have similar hot_path entries
        for filename, data in budgets_data.items():
            if "arch_budgets" in data:
                for arch, budgets in data["arch_budgets"].items():
                    if "hot_paths" in budgets:
                        hot_paths = budgets["hot_paths"]
                        if isinstance(hot_paths, dict):
                            print(f"    {filename}: {arch} hot_paths = {list(hot_paths.keys())}")
    
    return all_errors


def run_self_tests() -> bool:
    """Run self-tests on the verifier."""
    print("\n=== Running Self-Tests ===\n")
    
    fixtures = {
        "test-service-memory-budget.yaml": {
            "version": "1.0",
            "service": "test-service",
            "arch_budgets": {
                "linux/arm64": {
                    "idle": {"rss_kib": 1024, "pss_kib": "baseline_required"},
                }
            },
            "enforcement": {
                "gate_level": "fast_local",
                "fail_on_budget_exceeded": True,
            }
        },
        "missing-version-memory-budget.yaml": {
            "service": "missing-version",
            "arch_budgets": {},
            "enforcement": {"gate_level": "fast_local"},
        },
        "invalid-rss-memory-budget.yaml": {
            "version": "1.0",
            "service": "invalid-rss",
            "arch_budgets": {
                "linux/arm64": {
                    "idle": {"rss_kib": "not_a_number"},
                }
            },
            "enforcement": {"gate_level": "fast_local"},
        },
        "wrong-service-memory-budget.yaml": {
            "version": "1.0",
            "service": "wrong_service",
            "arch_budgets": {
                "linux/arm64": {
                    "idle": {"rss_kib": 1024},
                }
            },
            "enforcement": {
                "gate_level": "fast_local",
                "fail_on_budget_exceeded": True,
            }
        },
    }
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # Test 1: Valid budget should pass
        valid_path = os.path.join(tmpdir, "test-service-memory-budget.yaml")
        with open(valid_path, "w") as f:
            yaml.dump(fixtures["test-service-memory-budget.yaml"], f)
        
        errors, _ = validate_budget_yaml(valid_path)
        if len(errors) == 0:
            print("  PASS: Valid budget passes validation")
            tests_passed += 1
        else:
            print(f"  FAIL: Valid budget failed: {errors}")
            tests_failed += 1
        
        # Test 2: Missing version should fail
        missing_ver_path = os.path.join(tmpdir, "missing-version-memory-budget.yaml")
        with open(missing_ver_path, "w") as f:
            yaml.dump(fixtures["missing-version-memory-budget.yaml"], f)
        
        errors, _ = validate_budget_yaml(missing_ver_path)
        if len(errors) > 0:
            print("  PASS: Missing version correctly fails")
            tests_passed += 1
        else:
            print("  FAIL: Missing version should have failed")
            tests_failed += 1
        
        # Test 3: Invalid RSS type should fail
        invalid_path = os.path.join(tmpdir, "invalid-rss-memory-budget.yaml")
        with open(invalid_path, "w") as f:
            yaml.dump(fixtures["invalid-rss-memory-budget.yaml"], f)
        
        errors, _ = validate_budget_yaml(invalid_path)
        if len(errors) > 0:
            print("  PASS: Invalid RSS type correctly fails")
            tests_passed += 1
        else:
            print("  FAIL: Invalid RSS type should have failed")
            tests_failed += 1
        
        # Test 4: Service mismatch should fail
        mismatch_path = os.path.join(tmpdir, "wrong-service-memory-budget.yaml")
        with open(mismatch_path, "w") as f:
            yaml.dump(fixtures["wrong-service-memory-budget.yaml"], f)
        
        errors, _ = validate_budget_yaml(mismatch_path)
        if len(errors) > 0:
            print("  PASS: Service mismatch correctly fails")
            tests_passed += 1
        else:
            print("  FAIL: Service mismatch should have failed")
            tests_failed += 1
    
    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


def main():
    if "--self-test" in sys.argv:
        sys.exit(0 if run_self_tests() else 1)
    
    errors = run_verifier()
    
    print("\n" + "=" * 50)
    
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        print("Memory budget files are valid.")
        sys.exit(0)


if __name__ == "__main__":
    main()
