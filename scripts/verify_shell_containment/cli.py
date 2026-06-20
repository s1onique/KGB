"""
CLI parsing and mode dispatch for shell containment verifier.
"""

import argparse
import os
import subprocess
from typing import Optional

from .model import INVENTORY_CSV
from .loader import load_inventory
from .rules import check_script, check_inventory_consistency, get_shell_scripts
from .selftest import run_tests


def get_changed_scripts() -> set:
    """Get set of changed/new shell scripts from git."""
    changed_scripts = set()
    
    try:
        # Staged scripts
        result = subprocess.run(
            ["git", "diff", "--name-only", "--cached"],
            capture_output=True, text=True, timeout=30
        )
        staged_scripts = [f for f in result.stdout.strip().split("\n") if f.endswith(".sh")]
        
        # Unstaged scripts
        result = subprocess.run(
            ["git", "diff", "--name-only"],
            capture_output=True, text=True, timeout=30
        )
        unstaged_scripts = [f for f in result.stdout.strip().split("\n") if f.endswith(".sh")]
        
        # Untracked scripts
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
    return {s for s in changed_scripts if s.startswith("scripts/") or "scripts/" in s}


def run_self_test() -> int:
    """Run self-test mode."""
    print("=== Shell Containment Verifier Self-Test ===")
    if run_tests():
        print("\nAll tests passed!")
        return 0
    else:
        print("\nSome tests failed!")
        return 1


def run_inventory_check(inventory_path: Optional[str] = None) -> int:
    """Run inventory consistency check mode."""
    csv_path = inventory_path or INVENTORY_CSV
    
    print("[gate] checking inventory consistency")
    
    try:
        inventory = load_inventory(csv_path)
    except ValueError as e:
        print(f"[gate] FAIL: {e}")
        return 1
    
    result = check_inventory_consistency(inventory)
    if not result.passed:
        print("[gate] FAIL: Inventory consistency issues:")
        for issue in result.violations:
            print(f"  - {issue}")
        return 1
    
    print("[gate] inventory consistency PASS")
    return 0


def run_verifier(verbose: bool = False) -> int:
    """Run normal verifier mode."""
    print("[gate] checking shell containment")
    
    try:
        inventory = load_inventory(INVENTORY_CSV)
    except ValueError as e:
        print(f"[gate] FAIL: {e}")
        return 1
    
    changed_scripts = get_changed_scripts()
    
    if verbose:
        print(f"[gate] Checking {len(changed_scripts)} changed/new scripts")
    
    failures = []
    
    for script in sorted(changed_scripts):
        if not os.path.exists(script):
            continue
        
        result = check_script(script, inventory)
        if not result.passed:
            failures.append((script, result.violations))
    
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


def main() -> int:
    """Main entry point for CLI."""
    parser = argparse.ArgumentParser(description="Shell containment verifier")
    parser.add_argument("--self-test", action="store_true", help="Run self-test with fixture scripts")
    parser.add_argument("--check-inventory", action="store_true", help="Check inventory consistency")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--inventory", help="Path to inventory CSV (default: docs/generated/shell_inventory.csv)")
    args = parser.parse_args()
    
    if args.self_test:
        return run_self_test()
    
    if args.check_inventory:
        return run_inventory_check(args.inventory)
    
    return run_verifier(verbose=args.verbose)
