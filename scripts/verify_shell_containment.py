#!/usr/bin/env python3
"""
Shell Containment Verifier

Fails on new or modified Shell scripts that exceed the allowed wrapper profile
unless annotated with explicit justification.

Risk tokens that indicate non-wrapper shell:
- jq: JSON parsing
- curl with response parsing
- while/until polling loops
- sleep/retry loops
- trap-heavy cleanup
- JSON artifact writes
- gh release commands

The verifier reads from docs/generated/shell_inventory.csv as source of truth.

Usage:
    python3 scripts/verify_shell_containment.py [--self-test] [--check-inventory]
"""

import argparse
import csv
import os
import re
import sys
from pathlib import Path

# Risk token patterns (regex)
# Note: patterns must be specific enough to avoid false positives on legitimate shell usage
RISK_PATTERNS = {
    "jq": r"\bjq\s",
    "curl_parse": r"curl.*\$\|curl.*parse\|curl\s+\$\(",
    "polling_loop": r"while\s+true|until\s+true|while\s+\[\[|while\s+\(\(|until\s+\[\[|until\s+\(\(",
    "retry": r"\bretry\b|\bcooldown\b",
    "gh_release": r"\bgh\s+release\b",
    "trap_cleanup": r"trap.*cleanup|trap.*exit",
    "json_write": r"\$\(.*json|JSON\s*=\s*\$\(",
}

# Thin wrapper max lines
THIN_WRAPPER_MAX_LINES = 50

# Default inventory path
INVENTORY_CSV = "docs/generated/shell_inventory.csv"


def load_inventory(csv_path: str) -> dict:
    """Load inventory from CSV file, skipping comment lines."""
    inventory = {}
    if not os.path.exists(csv_path):
        return inventory

    with open(csv_path, "r", encoding="utf-8") as f:
        # Filter out comment lines and empty lines
        lines = [
            line for line in f
            if line.strip() and not line.strip().startswith("#")
        ]

    if not lines:
        raise ValueError(f"Shell inventory is empty: {csv_path}")

    reader = csv.DictReader(lines)
    required = {"path", "disposition", "risk_flags", "owner", "notes"}
    if set(reader.fieldnames or []) != required:
        raise ValueError(f"Invalid shell inventory header: {reader.fieldnames}")

    for row in reader:
        path = row["path"].strip()
        if path:
            inventory[path] = {
                "disposition": row["disposition"].strip(),
                "risk_flags": row["risk_flags"].strip(),
                "owner": row["owner"].strip(),
                "notes": row["notes"].strip(),
            }

    if not inventory:
        raise ValueError(f"Shell inventory loaded zero entries from: {csv_path}")

    return inventory


def is_thin_wrapper(path: str, lines: int) -> bool:
    """Check if script qualifies as thin wrapper based on line count."""
    if lines <= THIN_WRAPPER_MAX_LINES:
        return True
    return False


def has_header_annotation(content: str) -> tuple[bool, bool, bool]:
    """
    Check if script has explicit shell containment justification headers.
    
    Returns:
        (has_justification, has_role, has_migration_plan)
    """
    has_justification = bool(re.search(r"#\s*ShellJustification:", content, re.IGNORECASE))
    has_role = bool(re.search(r"#\s*ShellRole:", content, re.IGNORECASE))
    has_migration_plan = bool(re.search(r"#\s*MigrationPlan:", content, re.IGNORECASE))
    return has_justification, has_role, has_migration_plan


def check_script(path: str, inventory: dict) -> tuple[bool, list[str]]:
    """
    Check a single shell script for containment violations.
    
    Returns:
        (passed, list of violations)
    """
    violations = []
    
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as f:
            content = f.read()
    except Exception as e:
        return False, [f"Cannot read file: {e}"]
    
    # Count lines
    lines = content.count("\n") + (1 if content and not content.endswith("\n") else 0)
    
    # Get relative path for comparison (use basename for matching in tests)
    rel_path = str(Path(path).as_posix())
    basename = Path(path).name
    
    # Check if in inventory (try full path first, then basename)
    inventory_entry = inventory.get(rel_path) or inventory.get(basename)
    
    # Check risk tokens
    found_risks = []
    for risk_name, pattern in RISK_PATTERNS.items():
        if re.search(pattern, content, re.IGNORECASE):
            found_risks.append(risk_name)
    
    # If no risk tokens, passes
    if not found_risks:
        return True, []
    
    # Check header annotations
    has_justification, has_role, has_migration_plan = has_header_annotation(content)
    
    # Determine disposition from inventory
    if inventory_entry:
        disposition = inventory_entry["disposition"]
        owner = inventory_entry["owner"]
    else:
        disposition = None
        owner = None
    
    # Case 1: Risky script not in inventory - MUST be listed
    if found_risks and not inventory_entry:
        violations.append("Risky shell script must be listed in docs/generated/shell_inventory.csv")
        return False, violations
    
    # Case 2: In inventory as keep_wrapper - but has risky tokens
    if inventory_entry and disposition == "keep_wrapper":
        violations.append(
            f"Inventory says keep_wrapper, but risk tokens were found: {', '.join(found_risks)}"
        )
        return False, violations
    
    # Case 3: In inventory as grandfathered
    if disposition == "grandfathered_needs_owner":
        is_bootstrap = inventory_entry.get("notes", "").strip() == "Bootstrap inventory"
        if owner == "TBD" and not is_bootstrap:
            violations.append("Grandfathered script requires named owner (found: TBD)")
            return False, violations
        return True, []
    
    # Case 4: Only has role annotation (not enough for risky script)
    if has_role and not has_justification:
        violations.append("Risky script has ShellRole but missing ShellJustification")
        return False, violations
    
    # Case 5: Not in inventory, no valid justification - FAIL
    violations.append(f"Risk tokens found: {', '.join(found_risks)}")
    if lines > THIN_WRAPPER_MAX_LINES:
        violations.append(f"Script has {lines} lines (max {THIN_WRAPPER_MAX_LINES} for thin wrapper)")
    violations.append("List in docs/generated/shell_inventory.csv")
    
    return False, violations


def check_inventory_consistency(inventory: dict) -> tuple[bool, list[str]]:
    """
    Check inventory consistency: all paths exist, no drift.
    
    Returns:
        (passed, list of issues)
    """
    issues = []
    
    if not inventory:
        issues.append("Inventory is empty")
        return False, issues
    
    for path, entry in inventory.items():
        # Check if path exists
        if not os.path.exists(path):
            issues.append(f"Inventory path does not exist: {path}")
        
        # Check owner
        owner = entry.get("owner", "")
        disposition = entry.get("disposition", "")
        notes = entry.get("notes", "")
        
        if disposition == "grandfathered_needs_owner" and owner == "TBD" and notes != "Bootstrap inventory":
            issues.append(f"Grandfathered script requires named owner: {path}")
    
    if issues:
        return False, issues
    return True, []


def get_shell_scripts() -> list[str]:
    """Get list of shell scripts in the repository."""
    scripts = []
    
    # Scan scripts directory
    scripts_dir = Path("scripts")
    if scripts_dir.exists():
        for f in scripts_dir.rglob("*.sh"):
            scripts.append(str(f))
    
    # Scan other directories
    for pattern in ["uvb76/scripts/*.sh", "tovarisch/scripts/*.sh"]:
        for f in Path(".").glob(pattern):
            if str(f) not in scripts:
                scripts.append(str(f))
    
    return sorted(scripts)


def run_tests() -> bool:
    """Run self-test with fixture scripts."""
    import tempfile
    import shutil
    
    # Create temp directory for test fixtures
    test_dir = tempfile.mkdtemp()
    all_passed = True
    
    # Create temp inventory
    temp_inventory = {
        "good_thin_launcher.sh": {
            "disposition": "keep_wrapper",
            "risk_flags": "none",
            "owner": "none",
            "notes": "",
        },
        "good_annotated.sh": {
            "disposition": "keep_wrapper",
            "risk_flags": "none",
            "owner": "none",
            "notes": "",
        },
        "grandfathered_example.sh": {
            "disposition": "grandfathered_needs_owner",
            "risk_flags": "jq",
            "owner": "team-platform",
            "notes": "",
        },
        "test_bootstrap.sh": {
            "disposition": "grandfathered_needs_owner",
            "risk_flags": "jq",
            "owner": "TBD",
            "notes": "Bootstrap inventory",
        },
    }
    
    try:
        fixtures = {
            "good_thin_launcher.sh": """#!/bin/sh
# ShellRole: launcher
# Thin launcher that execs a typed binary
exec python3 -m mymodule "$@"
""",
            "good_annotated.sh": """#!/bin/bash
# ShellJustification: CI glue script that orchestrates typed binaries
# ShellRole: ci-glue
# MigrationPlan: Will be replaced when Go lab harness is ready
set -e
./build.sh
./test.sh
""",
            "bad_json_parsing.sh": """#!/bin/bash
# This script has risky JSON parsing without justification
result=$(curl -s http://api.example.com/data)
value=$(echo "$result" | jq '.value')
if [ "$value" = "true" ]; then
    echo "enabled"
fi
""",
            "bad_polling.sh": """#!/bin/bash
# This script has polling without justification
while true; do
    status=$(curl -s http://api.example.com/status)
    if [ "$status" = "ready" ]; then
        break
    fi
    sleep 5
done
""",
            "bad_release.sh": """#!/bin/bash
# This script has gh release commands without justification
gh release create v1.0.0 ./dist/*.zip
gh release upload v1.0.0 ./artifacts.json
""",
            "bad_role_only.sh": """#!/bin/bash
# ShellRole: launcher
# This script uses jq without justification
data=$(echo '{"foo": 1}' | jq '.foo')
""",
            "grandfathered_example.sh": """#!/bin/bash
# grandfathered: Listed in shell_inventory.csv
data=$(curl -s http://api.example.com/data)
jq '.' <<< "$data"
""",
        }
        
        # Write fixtures
        for name, content in fixtures.items():
            path = os.path.join(test_dir, name)
            with open(path, "w") as f:
                f.write(content)
            os.chmod(path, 0o755)
        
        # Test good scripts
        print("[test] Testing good_thin_launcher.sh...")
        passed, violations = check_script(os.path.join(test_dir, "good_thin_launcher.sh"), {})
        if passed:
            print("  PASS")
        else:
            print(f"  FAIL: {violations}")
            all_passed = False
        
        print("[test] Testing good_annotated.sh...")
        passed, violations = check_script(os.path.join(test_dir, "good_annotated.sh"), {})
        if passed:
            print("  PASS")
        else:
            print(f"  FAIL: {violations}")
            all_passed = False
        
        # Test grandfathered (with proper owner)
        print("[test] Testing grandfathered_example.sh (in inventory)...")
        passed, violations = check_script(os.path.join(test_dir, "grandfathered_example.sh"), temp_inventory)
        if passed:
            print("  PASS")
        else:
            print(f"  FAIL: {violations}")
            all_passed = False
        
        # Test bad scripts (should fail)
        print("[test] Testing bad_json_parsing.sh (should fail)...")
        passed, violations = check_script(os.path.join(test_dir, "bad_json_parsing.sh"), {})
        if not passed:
            print("  PASS (correctly rejected)")
        else:
            print("  FAIL: Should have been rejected")
            all_passed = False
        
        print("[test] Testing bad_polling.sh (should fail)...")
        passed, violations = check_script(os.path.join(test_dir, "bad_polling.sh"), {})
        if not passed:
            print("  PASS (correctly rejected)")
        else:
            print("  FAIL: Should have been rejected")
            all_passed = False
        
        print("[test] Testing bad_release.sh (should fail)...")
        passed, violations = check_script(os.path.join(test_dir, "bad_release.sh"), {})
        if not passed:
            print("  PASS (correctly rejected)")
        else:
            print("  FAIL: Should have been rejected")
            all_passed = False
        
        print("[test] Testing bad_role_only.sh (should fail - only has ShellRole)...")
        passed, violations = check_script(os.path.join(test_dir, "bad_role_only.sh"), {})
        if not passed:
            print("  PASS (correctly rejected)")
        else:
            print("  FAIL: Should have been rejected")
            all_passed = False
        
        # Test CSV loader with comment handling
        print("[test] Testing CSV loader with comments...")
        csv_content = """path,disposition,risk_flags,owner,notes
# This is a comment
scripts/test.sh,grandfathered_needs_owner,jq,team-platform,
"""
        csv_path = os.path.join(test_dir, "test_inventory.csv")
        with open(csv_path, "w") as f:
            f.write(csv_content)
        
        try:
            loaded = load_inventory(csv_path)
            if "scripts/test.sh" in loaded:
                print("  PASS (CSV loader correctly handles comments)")
            else:
                print(f"  FAIL: CSV loader missed entry, got: {loaded}")
                all_passed = False
        except Exception as e:
            print(f"  FAIL: CSV loader raised exception: {e}")
            all_passed = False
        
        # Test: risky script with full annotations but NOT in inventory should FAIL
        print("[test] Testing risky script with annotations but not in inventory (should fail)...")
        risky_with_annotations = """#!/bin/bash
# ShellJustification: testing
# ShellRole: test
# MigrationPlan: never
jq '.' <<< '{}'
"""
        risky_path = os.path.join(test_dir, "risky_not_in_inventory.sh")
        with open(risky_path, "w") as f:
            f.write(risky_with_annotations)
        os.chmod(risky_path, 0o755)
        passed, violations = check_script(risky_path, {})  # empty inventory
        if not passed and "must be listed in" in violations[0]:
            print("  PASS (correctly rejected - must be in CSV)")
        else:
            print(f"  FAIL: Should reject risky script not in CSV, got: {violations}")
            all_passed = False
        
        # Test: risky script listed as keep_wrapper should FAIL
        print("[test] Testing risky script listed as keep_wrapper (should fail)...")
        bad_inventory = {
            "misclassified.sh": {
                "disposition": "keep_wrapper",
                "risk_flags": "none",
                "owner": "none",
                "notes": "",
            }
        }
        misclassified_path = os.path.join(test_dir, "misclassified.sh")
        with open(misclassified_path, "w") as f:
            f.write(risky_with_annotations)
        os.chmod(misclassified_path, 0o755)
        passed, violations = check_script(misclassified_path, bad_inventory)
        if not passed and "keep_wrapper" in violations[0]:
            print("  PASS (correctly rejected - keep_wrapper mismatch)")
        else:
            print(f"  FAIL: Should reject misclassified script, got: {violations}")
            all_passed = False
        
    finally:
        shutil.rmtree(test_dir)
    
    return all_passed


def main():
    parser = argparse.ArgumentParser(description="Shell containment verifier")
    parser.add_argument("--self-test", action="store_true", help="Run self-test with fixture scripts")
    parser.add_argument("--check-inventory", action="store_true", help="Check inventory consistency")
    parser.add_argument("--verbose", action="store_true", help="Verbose output")
    args = parser.parse_args()
    
    if args.self_test:
        print("=== Shell Containment Verifier Self-Test ===")
        if run_tests():
            print("\nAll tests passed!")
            return 0
        else:
            print("\nSome tests failed!")
            return 1
    
    # Load inventory from CSV
    try:
        inventory = load_inventory(INVENTORY_CSV)
    except ValueError as e:
        print(f"[gate] FAIL: {e}")
        return 1
    
    if args.check_inventory:
        print("[gate] checking inventory consistency")
        passed, issues = check_inventory_consistency(inventory)
        if not passed:
            print("[gate] FAIL: Inventory consistency issues:")
            for issue in issues:
                print(f"  - {issue}")
            return 1
        print("[gate] inventory consistency PASS")
        return 0
    
    print("[gate] checking shell containment")
    
    # Get git diff for changed/new scripts
    try:
        import subprocess
        result = subprocess.run(
            ["git", "diff", "--name-only", "--cached"],
            capture_output=True, text=True, timeout=30
        )
        staged_scripts = [f for f in result.stdout.strip().split("\n") if f.endswith(".sh")]
        
        result = subprocess.run(
            ["git", "diff", "--name-only"],
            capture_output=True, text=True, timeout=30
        )
        unstaged_scripts = [f for f in result.stdout.strip().split("\n") if f.endswith(".sh")]
        
        result = subprocess.run(
            ["git", "ls-files", "--others", "--exclude-standard"],
            capture_output=True, text=True, timeout=30
        )
        new_scripts = [f for f in result.stdout.strip().split("\n") if f.endswith(".sh")]
        
        changed_scripts = set(staged_scripts + unstaged_scripts + new_scripts)
    except Exception as e:
        print(f"[gate] Warning: Could not get git status: {e}")
        print("[gate] Falling back to scanning all scripts")
        changed_scripts = set(get_shell_scripts())
    
    # Filter to only scripts directory
    changed_scripts = {s for s in changed_scripts if s.startswith("scripts/") or "scripts/" in s}
    
    if args.verbose:
        print(f"[gate] Checking {len(changed_scripts)} changed/new scripts")
    
    failures = []
    
    for script in sorted(changed_scripts):
        if not os.path.exists(script):
            continue
        
        passed, violations = check_script(script, inventory)
        if not passed:
            failures.append((script, violations))
    
    if failures:
        print("[gate] FAIL: Shell containment violations found:")
        for path, violations in failures:
            print(f"\n  {path}:")
            for v in violations:
                print(f"    - {v}")
        print("\n[gate] Fix: Add # ShellJustification: + # ShellRole: + # MigrationPlan: headers, or list in shell_inventory.csv")
        return 1
    
    print("[gate] shell containment PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
