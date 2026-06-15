#!/usr/bin/env python3
"""
Verifier for Manifesto Axiom Coverage Matrix.

This script validates the docs/doctrine/manifesto_axiom_coverage.csv file
ensuring it contains valid schema, enums, and coverage for all six axioms.
"""

import csv
import os
import sys
import tempfile
from typing import List, Tuple

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MATRIX_PATH = os.path.join(REPO_ROOT, "docs", "doctrine", "manifesto_axiom_coverage.csv")

REQUIRED_COLUMNS = ["axiom_id", "axiom_name", "repo_area", "enforcement_level", "enforcement_artifact", "status", "notes"]

VALID_AXIOM_IDS = ["AXIOM-1", "AXIOM-2", "AXIOM-3", "AXIOM-4", "AXIOM-5", "AXIOM-6"]

VALID_REPO_AREAS = [
    "source", "tests", "scripts", "docs", "doctrine", "epics",
    "wal", "ci", "gates", "bootstrap"
]

VALID_ENFORCEMENT_LEVELS = ["none", "documented", "advisory", "hard_gate", "not_applicable"]

VALID_STATUSES = ["missing", "present", "partial", "complete", "not_applicable"]

ENFORCEMENT_LEVELS_REQUIRING_ARTIFACT = ["documented", "advisory", "hard_gate"]


def load_matrix(path: str) -> Tuple[List[str], List[dict]]:
    """Load the CSV matrix and return (errors, rows)."""
    errors = []
    rows = []
    
    if not os.path.exists(path):
        errors.append(f"Matrix file not found: {path}")
        return errors, rows
    
    try:
        with open(path, "r", newline="", encoding="utf-8") as f:
            reader = csv.DictReader(f)
            fieldnames = reader.fieldnames or []
            
            # Check required columns
            missing_cols = [col for col in REQUIRED_COLUMNS if col not in fieldnames]
            if missing_cols:
                errors.append(f"Missing required columns: {', '.join(missing_cols)}")
                return errors, rows
            
            for row in reader:
                rows.append(row)
    except csv.Error as e:
        errors.append(f"CSV parse error: {e}")
    except Exception as e:
        errors.append(f"Error reading file: {e}")
    
    return errors, rows


def validate_rows(rows: List[dict]) -> List[str]:
    """Validate rows for schema violations."""
    errors = []
    seen_keys = set()
    
    for i, row in enumerate(rows, 1):
        line_num = i + 1  # +1 for header
        
        # Check for empty required fields
        if not row.get("axiom_id", "").strip():
            errors.append(f"Row {line_num}: axiom_id is empty")
        if not row.get("axiom_name", "").strip():
            errors.append(f"Row {line_num}: axiom_name is empty")
        if not row.get("repo_area", "").strip():
            errors.append(f"Row {line_num}: repo_area is empty")
        
        # Check axiom_id is valid
        axiom_id = row.get("axiom_id", "").strip()
        if axiom_id and axiom_id not in VALID_AXIOM_IDS:
            errors.append(f"Row {line_num}: unknown axiom_id '{axiom_id}' (expected one of {', '.join(VALID_AXIOM_IDS)})")
        
        # Check enforcement_level is valid
        enforcement_level = row.get("enforcement_level", "").strip()
        if enforcement_level and enforcement_level not in VALID_ENFORCEMENT_LEVELS:
            errors.append(f"Row {line_num}: unknown enforcement_level '{enforcement_level}'")
        
        # Check status is valid
        status = row.get("status", "").strip()
        if status and status not in VALID_STATUSES:
            errors.append(f"Row {line_num}: unknown status '{status}'")
        
        # Check repo_area is valid
        repo_area = row.get("repo_area", "").strip()
        if repo_area and repo_area not in VALID_REPO_AREAS:
            errors.append(f"Row {line_num}: unknown repo_area '{repo_area}'")
        
        # Check enforcement_artifact for rows that require it
        if enforcement_level in ENFORCEMENT_LEVELS_REQUIRING_ARTIFACT:
            artifact = row.get("enforcement_artifact", "").strip()
            if not artifact:
                errors.append(f"Row {line_num}: enforcement_artifact is required for enforcement_level '{enforcement_level}'")
        
        # Track duplicates (same axiom_id + repo_area + enforcement_artifact)
        if axiom_id and repo_area:
            artifact = row.get("enforcement_artifact", "").strip()
            key = (axiom_id, repo_area, artifact)
            if key in seen_keys:
                errors.append(f"Row {line_num}: duplicate row (same axiom_id + repo_area + enforcement_artifact)")
            seen_keys.add(key)
    
    return errors


def check_all_axioms_present(rows: List[dict]) -> List[str]:
    """Check that all six axioms are represented."""
    errors = []
    present_ids = set()
    
    for row in rows:
        axiom_id = row.get("axiom_id", "").strip()
        if axiom_id in VALID_AXIOM_IDS:
            present_ids.add(axiom_id)
    
    missing = [aid for aid in VALID_AXIOM_IDS if aid not in present_ids]
    if missing:
        errors.append(f"Missing axiom IDs: {', '.join(missing)}")
    
    return errors


def print_summary(rows: List[dict]):
    """Print a summary by axiom and enforcement level."""
    by_axiom = {}
    for row in rows:
        axiom_id = row.get("axiom_id", "").strip()
        if axiom_id not in by_axiom:
            by_axiom[axiom_id] = {"documented": 0, "advisory": 0, "hard_gate": 0, "none": 0, "not_applicable": 0}
        level = row.get("enforcement_level", "").strip()
        if level in by_axiom[axiom_id]:
            by_axiom[axiom_id][level] += 1
    
    print("\n=== Axiom Coverage Summary ===")
    for axiom_id in sorted(by_axiom.keys()):
        counts = by_axiom[axiom_id]
        print(f"{axiom_id}: documented={counts['documented']}, advisory={counts['advisory']}, hard_gate={counts['hard_gate']}, none={counts['none']}, not_applicable={counts['not_applicable']}")
    
    print(f"\nTotal rows: {len(rows)}")


def run_self_tests():
    """Run self-test sentinels to verify the verifier works correctly."""
    import subprocess
    
    print("\n=== Running Self-Tests ===\n")
    
    tests_passed = 0
    tests_failed = 0
    
    def run_test(name: str, csv_content: str, should_pass: bool) -> bool:
        nonlocal tests_passed, tests_failed
        
        with tempfile.NamedTemporaryFile(mode="w", suffix=".csv", delete=False) as f:
            f.write(csv_content)
            temp_path = f.name
        
        try:
            result = subprocess.run(
                [sys.executable, __file__, "--test-file", temp_path],
                capture_output=True,
                text=True
            )
            passed = result.returncode == 0
            
            if passed == should_pass:
                print(f"  PASS: {name}")
                tests_passed += 1
                return True
            else:
                print(f"  FAIL: {name}")
                print(f"    Expected: {'pass' if should_pass else 'fail'}")
                print(f"    Got: {'pass' if passed else 'fail'}")
                if result.stdout:
                    print(f"    stdout: {result.stdout.strip()}")
                if result.stderr:
                    print(f"    stderr: {result.stderr.strip()}")
                tests_failed += 1
                return False
        finally:
            os.unlink(temp_path)
    
    # Test 1: Valid matrix passes
    run_test(
        "valid matrix passes",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/ai-native-code-discipline-axioms.md,present,Test
""",
        should_pass=True
    )
    
    # Test 2: Missing required column fails
    run_test(
        "missing required column fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,documented,present,Test
""",
        should_pass=False
    )
    
    # Test 3: Unknown axiom ID fails
    run_test(
        "unknown axiom ID fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-99,Unknown Axiom,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    # Test 4: Missing one of the six axioms fails
    run_test(
        "missing one of the six axioms fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    # Test 5: Invalid enforcement_level fails
    run_test(
        "invalid enforcement_level fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,INVALID_LEVEL,docs/doctrine/test.md,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    # Test 6: Invalid status fails
    run_test(
        "invalid status fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,INVALID_STATUS,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    # Test 7: Duplicate row fails
    run_test(
        "duplicate row fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-1,Repo-Local Project Memory,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    # Test 8: hard_gate row without enforcement_artifact fails
    run_test(
        "hard_gate row without enforcement_artifact fails",
        """axiom_id,axiom_name,repo_area,enforcement_level,enforcement_artifact,status,notes
AXIOM-1,Repo-Local Project Memory,doctrine,hard_gate,,present,Test
AXIOM-2,Cold-Resume Checkpoint,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-3,Periodic Health Audit,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-4,Production-Path Parity,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-5,Managed Agent Blocks,doctrine,documented,docs/doctrine/test.md,present,Test
AXIOM-6,Stable Doctrine/ADR Links,doctrine,documented,docs/doctrine/test.md,present,Test
""",
        should_pass=False
    )
    
    print(f"\n=== Self-Test Results ===")
    print(f"  Passed: {tests_passed}")
    print(f"  Failed: {tests_failed}")
    
    return tests_failed == 0


def main():
    # Handle --test-file flag for self-testing
    test_file = None
    args = sys.argv[1:]
    for i, arg in enumerate(args):
        if arg == "--test-file" and i + 1 < len(args):
            test_file = args[i + 1]
        elif arg == "--test-file":
            print("Error: --test-file requires a path argument", file=sys.stderr)
            sys.exit(1)
    
    # Handle --self-test mode
    if "--self-test" in args:
        success = run_self_tests()
        sys.exit(0 if success else 1)
    
    # Determine matrix path
    matrix_path = test_file if test_file else MATRIX_PATH
    
    # Load and validate
    errors, rows = load_matrix(matrix_path)
    
    if errors:
        for e in errors:
            print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)
    
    # Validate rows
    errors = validate_rows(rows)
    if errors:
        for e in errors:
            print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)
    
    # Check all axioms present
    errors = check_all_axioms_present(rows)
    if errors:
        for e in errors:
            print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)
    
    # Print summary
    print_summary(rows)
    
    print("\nVERIFICATION PASSED")
    sys.exit(0)


if __name__ == "__main__":
    main()
