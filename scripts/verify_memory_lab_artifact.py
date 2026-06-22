#!/usr/bin/env python3
"""
Verifier for memory lab artifacts.

Validates memory lab JSON artifacts conform to the lab artifact schema.
"""

import os
import sys
import json
from typing import List, Tuple, Any, Dict

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

# Required top-level fields
REQUIRED_LAB_RESULT_FIELDS = {
    "schema_version": str,
    "service": dict,
    "environment": dict,
    "workload": dict,
    "memory": dict,
    "decision": dict,
}

REQUIRED_SERVICE_FIELDS = {
    "name": str,
    "version": str,
    "commit": str,
}

REQUIRED_ENVIRONMENT_FIELDS = {
    "arch": str,
}

REQUIRED_WORKLOAD_FIELDS = {
    "type": str,
    "operations": int,
    "errors": int,
    "duration_ms": int,
}

REQUIRED_MEMORY_FIELDS = {
    "first": dict,
    "max": dict,
    "last": dict,
    "growth": dict,
}

REQUIRED_MEMORY_SNAPSHOT_FIELDS = {
    "rss_kib": int,
}

REQUIRED_DECISION_FIELDS = {
    "pass": bool,
    "reason": str,
}


def validate_service(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_SERVICE_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}, got {type(data[field]).__name__}")
    return errors


def validate_environment(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_ENVIRONMENT_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}, got {type(data[field]).__name__}")
    return errors


def validate_workload(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_WORKLOAD_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}, got {type(data[field]).__name__}")
    return errors


def validate_memory_snapshot(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_MEMORY_SNAPSHOT_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}")
    return errors


def validate_memory(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field in REQUIRED_MEMORY_FIELDS:
        if field not in data:
            errors.append(f"{path}.{field} is required")
        else:
            sub = data[field]
            if not isinstance(sub, dict):
                errors.append(f"{path}.{field} must be a dict")
            elif "rss_kib" not in sub:
                errors.append(f"{path}.{field}.rss_kib is required")
            elif not isinstance(sub["rss_kib"], int):
                errors.append(f"{path}.{field}.rss_kib must be int")
    return errors


def validate_decision(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_DECISION_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}, got {type(data[field]).__name__}")
    return errors


def validate_lab_result(data: Dict) -> List[str]:
    errors = []
    
    # Check schema version
    if "schema_version" not in data:
        errors.append("schema_version is required")
    elif data["schema_version"] != "1.0":
        errors.append(f"Unsupported schema version: {data['schema_version']}")
    
    # Validate service
    if "service" in data:
        errors.extend(validate_service(data["service"], "service"))
    else:
        errors.append("service is required")
    
    # Validate environment
    if "environment" in data:
        errors.extend(validate_environment(data["environment"], "environment"))
    else:
        errors.append("environment is required")
    
    # Validate workload
    if "workload" in data:
        errors.extend(validate_workload(data["workload"], "workload"))
    else:
        errors.append("workload is required")
    
    # Validate memory
    if "memory" in data:
        errors.extend(validate_memory(data["memory"], "memory"))
    else:
        errors.append("memory is required")
    
    # Validate decision
    if "decision" in data:
        errors.extend(validate_decision(data["decision"], "decision"))
    else:
        errors.append("decision is required")
    
    # Semantic consistency checks
    errors.extend(validate_semantic_consistency(data))
    
    return errors


def validate_semantic_consistency(data: Dict) -> List[str]:
    """Validate semantic consistency of memory lab artifacts."""
    errors = []
    
    # Require evidence_kind to be one of: schema_fixture, real_evidence
    evidence_kind = data.get("evidence_kind")
    if evidence_kind not in ("schema_fixture", "real_evidence"):
        errors.append(
            f"evidence_kind must be 'schema_fixture' or 'real_evidence', "
            f"got '{evidence_kind or 'missing'}'"
        )
        return errors  # Cannot proceed without valid evidence_kind
    
    # Enforce operations >= errors for every artifact
    workload = data.get("workload", {})
    operations = workload.get("operations", 0)
    errors_count = workload.get("errors", 0)
    if operations < errors_count:
        errors.append(
            f"workload.operations ({operations}) < workload.errors ({errors_count})"
        )
    
    # Enforce duration_ms > 0 for every artifact
    duration_ms = workload.get("duration_ms", 0)
    if duration_ms <= 0:
        errors.append(f"workload.duration_ms ({duration_ms}) must be > 0")
    
    # Only enforce strict RSS math consistency for real evidence
    if evidence_kind == "real_evidence":
        memory = data.get("memory", {})
        
        # growth.rss_kib == last.rss_kib - first.rss_kib
        first_rss = memory.get("first", {}).get("rss_kib")
        last_rss = memory.get("last", {}).get("rss_kib")
        growth_rss = memory.get("growth", {}).get("rss_kib")
        
        if all(isinstance(x, int) for x in [first_rss, last_rss, growth_rss]):
            expected_growth = last_rss - first_rss
            if growth_rss != expected_growth:
                errors.append(
                    f"memory.growth.rss_kib ({growth_rss}) != "
                    f"memory.last.rss_kib ({last_rss}) - memory.first.rss_kib ({first_rss}) "
                    f"(expected {expected_growth})"
                )
        
        # max.rss_kib >= first.rss_kib
        max_rss = memory.get("max", {}).get("rss_kib")
        if all(isinstance(x, int) for x in [max_rss, first_rss]):
            if max_rss < first_rss:
                errors.append(
                    f"memory.max.rss_kib ({max_rss}) < memory.first.rss_kib ({first_rss})"
                )
        
        # max.rss_kib >= last.rss_kib
        if all(isinstance(x, int) for x in [max_rss, last_rss]):
            if max_rss < last_rss:
                errors.append(
                    f"memory.max.rss_kib ({max_rss}) < memory.last.rss_kib ({last_rss})"
                )
    
    return errors


def validate_file(path: str) -> Tuple[List[str], Dict]:
    errors = []
    data = None
    
    if not os.path.exists(path):
        return [f"File does not exist: {path}"], None
    
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON in {path}: {e}"], None
    
    if not isinstance(data, dict):
        return ["lab-result.json must be a JSON object"], None
    
    errors.extend(validate_lab_result(data))
    return errors, data


def run_verifier(repo_root: str) -> List[str]:
    all_errors = []
    
    print("=== Memory Lab Artifact Verifier ===\n")
    
    # Check for fixture artifacts
    fixture_dirs = [
        os.path.join(repo_root, "docs", "memory", "fixtures"),
        os.path.join(repo_root, "tovarisch", "fixtures"),
    ]
    
    lab_result_found = False
    
    for fixture_dir in fixture_dirs:
        if not os.path.isdir(fixture_dir):
            continue
        
        for entry in os.listdir(fixture_dir):
            if entry.endswith("-memory-lab.json") or entry == "lab-result.json":
                lab_result_found = True
                path = os.path.join(fixture_dir, entry)
                print(f"Validating: {path}")
                errors, _ = validate_file(path)
                if errors:
                    for e in errors:
                        print(f"  ERROR: {e}")
                    all_errors.extend(errors)
                else:
                    print(f"  OK: Valid artifact")
    
    if not lab_result_found:
        print("  No memory lab artifacts found (this is OK for initial implementation)")
        print("  Artifacts will be required once memory labs are run")
    
    return all_errors


def run_self_tests() -> bool:
    import tempfile
    
    print("\n=== Running Self-Tests ===\n")
    
    # Valid schema_fixture artifact
    valid_fixture = {
        "schema_version": "1.0",
        "evidence_kind": "schema_fixture",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {
            "type": "test-workload",
            "operations": 100,
            "errors": 0,
            "duration_ms": 1000,
        },
        "memory": {
            "first": {"rss_kib": 1000},
            "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050},
            "growth": {"rss_kib": 50, "rss_percent": 5.0},
        },
        "decision": {"pass": True, "reason": "test passed"},
    }
    
    # Valid real_evidence artifact with correct math
    valid_real_evidence = {
        "schema_version": "1.0",
        "evidence_kind": "real_evidence",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {
            "type": "test-workload",
            "operations": 100,
            "errors": 0,
            "duration_ms": 1000,
        },
        "memory": {
            "first": {"rss_kib": 1000},
            "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050},
            "growth": {"rss_kib": 50, "rss_percent": 5.0},
        },
        "decision": {"pass": True, "reason": "test passed"},
    }
    
    # Invalid: real_evidence with bad growth math
    bad_growth_real_evidence = {
        "schema_version": "1.0",
        "evidence_kind": "real_evidence",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {
            "type": "test-workload",
            "operations": 100,
            "errors": 0,
            "duration_ms": 1000,
        },
        "memory": {
            "first": {"rss_kib": 1000},
            "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050},
            "growth": {"rss_kib": 999, "rss_percent": 99.9},  # Wrong: should be 50
        },
        "decision": {"pass": True, "reason": "test passed"},
    }
    
    # Invalid: missing evidence_kind
    missing_evidence_kind = {
        "schema_version": "1.0",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {
            "type": "test-workload",
            "operations": 100,
            "errors": 0,
            "duration_ms": 1000,
        },
        "memory": {
            "first": {"rss_kib": 1000},
            "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050},
            "growth": {"rss_kib": 50, "rss_percent": 5.0},
        },
        "decision": {"pass": True, "reason": "test passed"},
    }
    
    # Invalid: operations < errors
    bad_operations = {
        "schema_version": "1.0",
        "evidence_kind": "schema_fixture",
        "service": {"name": "test", "version": "1.0.0", "commit": "abc123"},
        "environment": {"arch": "linux/arm64"},
        "workload": {
            "type": "test-workload",
            "operations": 5,
            "errors": 10,  # More errors than operations
            "duration_ms": 1000,
        },
        "memory": {
            "first": {"rss_kib": 1000},
            "max": {"rss_kib": 1100},
            "last": {"rss_kib": 1050},
            "growth": {"rss_kib": 50, "rss_percent": 5.0},
        },
        "decision": {"pass": True, "reason": "test passed"},
    }
    
    tests_passed = 0
    tests_failed = 0
    
    with tempfile.TemporaryDirectory() as tmpdir:
        # Test 1: Valid schema_fixture should pass
        valid_path = os.path.join(tmpdir, "valid_fixture.json")
        with open(valid_path, "w") as f:
            json.dump(valid_fixture, f)
        
        errors, _ = validate_file(valid_path)
        if len(errors) == 0:
            print("  PASS: Valid schema_fixture passes validation")
            tests_passed += 1
        else:
            print(f"  FAIL: Valid schema_fixture failed: {errors}")
            tests_failed += 1
        
        # Test 2: Valid real_evidence should pass
        real_path = os.path.join(tmpdir, "valid_real.json")
        with open(real_path, "w") as f:
            json.dump(valid_real_evidence, f)
        
        errors, _ = validate_file(real_path)
        if len(errors) == 0:
            print("  PASS: Valid real_evidence passes validation")
            tests_passed += 1
        else:
            print(f"  FAIL: Valid real_evidence failed: {errors}")
            tests_failed += 1
        
        # Test 3: real_evidence with bad growth math should fail
        bad_growth_path = os.path.join(tmpdir, "bad_growth.json")
        with open(bad_growth_path, "w") as f:
            json.dump(bad_growth_real_evidence, f)
        
        errors, _ = validate_file(bad_growth_path)
        if len(errors) > 0 and any("growth.rss_kib" in e for e in errors):
            print("  PASS: real_evidence with bad growth correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: real_evidence with bad growth should have failed: {errors}")
            tests_failed += 1
        
        # Test 4: missing evidence_kind should fail
        missing_path = os.path.join(tmpdir, "missing_evidence.json")
        with open(missing_path, "w") as f:
            json.dump(missing_evidence_kind, f)
        
        errors, _ = validate_file(missing_path)
        if len(errors) > 0 and any("evidence_kind" in e for e in errors):
            print("  PASS: missing evidence_kind correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: missing evidence_kind should have failed: {errors}")
            tests_failed += 1
        
        # Test 5: operations < errors should fail
        bad_ops_path = os.path.join(tmpdir, "bad_ops.json")
        with open(bad_ops_path, "w") as f:
            json.dump(bad_operations, f)
        
        errors, _ = validate_file(bad_ops_path)
        if len(errors) > 0 and any("operations" in e for e in errors):
            print("  PASS: operations < errors correctly fails")
            tests_passed += 1
        else:
            print(f"  FAIL: operations < errors should have failed: {errors}")
            tests_failed += 1
    
    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}, Failed: {tests_failed}")
    return tests_failed == 0


def main():
    if "--self-test" in sys.argv:
        sys.exit(0 if run_self_tests() else 1)
    
    errors = run_verifier(REPO_ROOT)
    
    print("\n" + "=" * 50)
    
    if errors:
        print("\nVERIFICATION FAILED:")
        for e in errors:
            print(f"  - {e}")
        sys.exit(1)
    else:
        print("\nVERIFICATION PASSED")
        sys.exit(0)


if __name__ == "__main__":
    main()
