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

REQUIRED_SERVICE_FIELDS = {"name": str, "version": str, "commit": str}
REQUIRED_ENVIRONMENT_FIELDS = {"arch": str}
REQUIRED_WORKLOAD_FIELDS = {"type": str, "operations": int, "errors": int, "duration_ms": int}
REQUIRED_MEMORY_FIELDS = {"first": dict, "max": dict, "last": dict, "growth": dict}
REQUIRED_MEMORY_SNAPSHOT_FIELDS = {"rss_kib": int}
REQUIRED_DECISION_FIELDS = {"pass": bool, "reason": str}


def validate_service(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_SERVICE_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}")
    return errors


def validate_environment(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_ENVIRONMENT_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required")
        elif not isinstance(data[field], expected_type):
            errors.append(f"{path}.{field} must be {expected_type.__name__}")
    return errors


def validate_workload(data: Dict, path: str) -> List[str]:
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in REQUIRED_WORKLOAD_FIELDS.items():
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
            errors.append(f"{path}.{field} must be {expected_type.__name__}")
    return errors


def validate_lab_result(data: Dict) -> List[str]:
    errors = []
    if "schema_version" not in data:
        errors.append("schema_version is required")
    elif data["schema_version"] != "1.0":
        errors.append(f"Unsupported schema version: {data['schema_version']}")

    if "service" in data:
        errors.extend(validate_service(data["service"], "service"))
    else:
        errors.append("service is required")

    if "environment" in data:
        errors.extend(validate_environment(data["environment"], "environment"))
    else:
        errors.append("environment is required")

    if "workload" in data:
        errors.extend(validate_workload(data["workload"], "workload"))
    else:
        errors.append("workload is required")

    if "memory" in data:
        errors.extend(validate_memory(data["memory"], "memory"))
    else:
        errors.append("memory is required")

    if "decision" in data:
        errors.extend(validate_decision(data["decision"], "decision"))
    else:
        errors.append("decision is required")

    errors.extend(validate_semantic_consistency(data))
    return errors


def validate_semantic_consistency(data: Dict) -> List[str]:
    """Validate semantic consistency of memory lab artifacts."""
    errors = []
    evidence_kind = data.get("evidence_kind")
    if evidence_kind not in ("schema_fixture", "real_evidence"):
        errors.append(f"evidence_kind must be 'schema_fixture' or 'real_evidence', got '{evidence_kind or 'missing'}'")
        return errors

    workload = data.get("workload", {})
    operations = workload.get("operations", 0)
    errors_count = workload.get("errors", 0)
    if operations < errors_count:
        errors.append(f"workload.operations ({operations}) < workload.errors ({errors_count})")

    duration_ms = workload.get("duration_ms", 0)
    if duration_ms <= 0:
        errors.append(f"workload.duration_ms ({duration_ms}) must be > 0")

    if evidence_kind == "real_evidence":
        memory = data.get("memory", {})
        first_rss = memory.get("first", {}).get("rss_kib")
        last_rss = memory.get("last", {}).get("rss_kib")
        growth_rss = memory.get("growth", {}).get("rss_kib")

        if all(isinstance(x, int) for x in [first_rss, last_rss, growth_rss]):
            expected_growth = last_rss - first_rss
            if growth_rss != expected_growth:
                errors.append(f"memory.growth.rss_kib ({growth_rss}) != expected ({expected_growth})")

        max_rss = memory.get("max", {}).get("rss_kib")
        if all(isinstance(x, int) for x in [max_rss, first_rss]):
            if max_rss < first_rss:
                errors.append(f"memory.max.rss_kib ({max_rss}) < memory.first.rss_kib ({first_rss})")
        if all(isinstance(x, int) for x in [max_rss, last_rss]):
            if max_rss < last_rss:
                errors.append(f"memory.max.rss_kib ({max_rss}) < memory.last.rss_kib ({last_rss})")

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


def run_verifier(repo_root: str, require_real_evidence: bool = False) -> List[str]:
    """Run the memory lab artifact verifier."""
    all_errors = []
    print("=== Memory Lab Artifact Verifier ===\n")

    schema_fixtures = []
    real_evidences = []
    invalid_artifacts = []

    fixture_dirs = [
        os.path.join(repo_root, "docs", "memory", "fixtures"),
        os.path.join(repo_root, "tovarisch", "fixtures"),
    ]
    real_evidence_dirs = [
        os.path.join(repo_root, "artifacts", "memory-labs", "tovarisch"),
        os.path.join(repo_root, "artifacts", "memory-labs", "uvb76"),
    ]

    print("A. Checking schema fixtures (docs/memory/fixtures/)...")
    for fixture_dir in fixture_dirs:
        if not os.path.isdir(fixture_dir):
            continue
        for entry in os.listdir(fixture_dir):
            if entry.endswith("-memory-lab.json") or entry == "lab-result.json":
                path = os.path.join(fixture_dir, entry)
                print(f"  Validating: {path}")
                errors, data = validate_file(path)
                if errors:
                    for e in errors:
                        print(f"    ERROR: {e}")
                    all_errors.extend(errors)
                    invalid_artifacts.append(path)
                else:
                    evidence_kind = data.get("evidence_kind", "unknown")
                    if evidence_kind == "schema_fixture":
                        schema_fixtures.append(path)
                        print(f"    OK: Valid schema_fixture artifact")
                    else:
                        print(f"    WARNING: Fixture has wrong evidence_kind: {evidence_kind}")

    print("\nB. Checking real evidence artifacts (artifacts/memory-labs/)...")
    for evidence_dir in real_evidence_dirs:
        if not os.path.isdir(evidence_dir):
            continue
        for entry in os.listdir(evidence_dir):
            if entry.endswith(".json"):
                path = os.path.join(evidence_dir, entry)
                print(f"  Validating: {path}")
                
                # Reject placeholders immediately
                if "SAMPLE" in entry.upper():
                    print(f"    ERROR: SAMPLE in filename is not real evidence")
                    all_errors.append(f"Placeholder artifact cannot be real evidence: {path}")
                    invalid_artifacts.append(path)
                    continue
                
                errors, data = validate_file(path)
                if errors:
                    for e in errors:
                        print(f"    ERROR: {e}")
                    all_errors.extend(errors)
                    invalid_artifacts.append(path)
                    continue
                
                evidence_kind = data.get("evidence_kind", "unknown")
                if evidence_kind == "schema_fixture":
                    print(f"    ERROR: schema_fixture cannot live in artifacts/memory-labs/")
                    all_errors.append(f"schema_fixture must be in docs/memory/fixtures/: {path}")
                    invalid_artifacts.append(path)
                    continue
                
                if evidence_kind == "real_evidence":
                    # Additional checks for real evidence
                    version = data.get("service", {}).get("version", "")
                    commit = data.get("service", {}).get("commit", "")
                    if "placeholder" in version.lower() or "unknown" in commit.lower():
                        print(f"    ERROR: Real evidence cannot have placeholder version/commit")
                        all_errors.append(f"Placeholder version/commit is not real evidence: {path}")
                        invalid_artifacts.append(path)
                        continue
                    
                    real_evidences.append(path)
                    print(f"    OK: Valid real_evidence artifact")
                else:
                    print(f"    ERROR: Expected 'real_evidence', got '{evidence_kind}'")
                    all_errors.append(f"Artifact must have evidence_kind='real_evidence' in {path}")
                    invalid_artifacts.append(path)

    print("\n" + "=" * 50)
    print("SUMMARY:")
    print(f"  Schema fixtures: {len(schema_fixtures)} valid")
    print(f"  Real evidence artifacts: {len(real_evidences)} valid")
    if invalid_artifacts:
        print(f"  Invalid artifacts: {len(invalid_artifacts)}")
    print("")

    if require_real_evidence and len(real_evidences) == 0:
        print("ERROR: require_real_evidence=True but no valid real_evidence artifacts found!")
        all_errors.append("No real_evidence artifacts found in artifacts/memory-labs/")

    return all_errors


def run_self_tests() -> bool:
    """Run self-tests on the verifier."""
    import tempfile
    print("\n=== Running Self-Tests ===\n")

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

    tests_passed = 0
    tests_failed = 0

    with tempfile.TemporaryDirectory() as tmpdir:
        # Test: valid fixture
        path = os.path.join(tmpdir, "fixture.json")
        with open(path, "w") as f:
            json.dump(valid_fixture, f)
        errors, _ = validate_file(path)
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
        errors, _ = validate_file(path)
        if len(errors) == 0:
            print("  PASS: Valid real_evidence passes")
            tests_passed += 1
        else:
            print(f"  FAIL: {errors}")
            tests_failed += 1

        # Test: bad growth math
        bad = dict(valid_real)
        bad["memory"]["growth"]["rss_kib"] = 999
        path = os.path.join(tmpdir, "bad.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = validate_file(path)
        if len(errors) > 0 and any("growth.rss_kib" in e for e in errors):
            print("  PASS: bad growth math fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: missing evidence_kind
        bad = dict(valid_fixture)
        del bad["evidence_kind"]
        path = os.path.join(tmpdir, "missing.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = validate_file(path)
        if len(errors) > 0 and any("evidence_kind" in e for e in errors):
            print("  PASS: missing evidence_kind fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
            tests_failed += 1

        # Test: operations < errors
        bad = dict(valid_fixture)
        bad["workload"]["operations"] = 5
        bad["workload"]["errors"] = 10
        path = os.path.join(tmpdir, "badops.json")
        with open(path, "w") as f:
            json.dump(bad, f)
        errors, _ = validate_file(path)
        if len(errors) > 0 and any("operations" in e for e in errors):
            print("  PASS: operations < errors fails correctly")
            tests_passed += 1
        else:
            print(f"  FAIL: should have failed")
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
