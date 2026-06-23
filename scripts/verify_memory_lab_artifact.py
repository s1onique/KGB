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

# Number type for JSON numeric fields (accepts int or float but not bool)
# Use string sentinel since we can't use a tuple without breaking isinstance checks
Number = "number"


def is_numeric(value) -> bool:
    """Check if value is numeric (int or float) but not bool."""
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def matches_expected_type(value, expected_type) -> bool:
    """Check if value matches the expected type, handling Number specially."""
    if expected_type == Number:
        return is_numeric(value)
    if expected_type is int:
        # Explicitly reject bool since bool is a subclass of int in Python
        return isinstance(value, int) and not isinstance(value, bool)
    return isinstance(value, expected_type)


def type_name_for_error(expected_type) -> str:
    """Get a clean type name for error messages."""
    if expected_type == Number:
        return "number"
    return expected_type.__name__


# Leak-slope specific fields (required for leak-slope workloads)
LEAK_SLOPE_REQUIRED_FIELDS = {
    "sampled_points": int,
    "duration_seconds": Number,
    "rss_first_kib": int,
    "rss_max_kib": int,
    "rss_last_kib": int,
    "pss_first_kib": int,
    "pss_max_kib": int,
    "pss_last_kib": int,
    "rss_growth_kib": int,
    "pss_growth_kib": int,
    "rss_slope_kib_per_min": Number,
    "pss_slope_kib_per_min": Number,
    "request_count": int,
    "request_errors": int,
}

# Leak-slope workload types
LEAK_SLOPE_WORKLOAD_TYPES = {
    "tovarisch-leak-slope",
    "tovarisch-leak-slope-netdiag",
    "uvb76-leak-slope",
    "uvb76-leak-slope-netdiag",
}


def is_runtime_support_file(path: str) -> bool:
    """Check if a file is a runtime support file, not a memory lab artifact.
    
    Runtime support files (*.derived.json, *.runtime.json) are generated during
    memory lab runs but are NOT evidence artifacts. They should be skipped.
    """
    name = os.path.basename(path)
    return (
        name.endswith(".derived.json")
        or name.endswith(".runtime.json")
    )


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


def validate_leak_slope(data: Dict, path: str) -> List[str]:
    """Validate leak-slope metrics if present."""
    errors = []
    if not isinstance(data, dict):
        return [f"{path} must be a dict"]
    for field, expected_type in LEAK_SLOPE_REQUIRED_FIELDS.items():
        if field not in data:
            errors.append(f"{path}.{field} is required for leak-slope workload")
        elif not matches_expected_type(data[field], expected_type):
            errors.append(f"{path}.{field} must be {type_name_for_error(expected_type)}")
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

    # Validate leak-slope metrics if workload is a leak-slope type
    workload = data.get("workload", {})
    workload_type = workload.get("type", "")
    if workload_type in LEAK_SLOPE_WORKLOAD_TYPES:
        if "leak_slope" in data:
            errors.extend(validate_leak_slope(data["leak_slope"], "leak_slope"))
        else:
            errors.append("leak_slope is required for leak-slope workload")

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

        # Validate leak-slope semantics if present
        leak_slope = data.get("leak_slope", {})
        if leak_slope:
            request_count = leak_slope.get("request_count", 0)
            request_errors = leak_slope.get("request_errors", 0)
            if request_count <= 0:
                errors.append("leak_slope.request_count must be > 0")
            if request_errors > request_count:
                errors.append(f"leak_slope.request_errors ({request_errors}) > leak_slope.request_count ({request_count})")

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
                
                # Skip runtime support files - they are not evidence artifacts.
                if is_runtime_support_file(path):
                    print(f"  SKIP (runtime support): {path}")
                    continue
                
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


def main():
    if "--self-test" in sys.argv:
        # Delegate to separate test file for LLM-friendliness
        import subprocess
        test_file = os.path.join(SCRIPT_DIR, "verify_memory_lab_artifact_tests.py")
        result = subprocess.run([sys.executable, test_file], capture_output=True, text=True)
        print(result.stdout)
        if result.stderr:
            print(result.stderr)
        sys.exit(result.returncode)

    require_real_evidence = "--require-real-evidence" in sys.argv
    errors = run_verifier(REPO_ROOT, require_real_evidence=require_real_evidence)

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
