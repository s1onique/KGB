#!/usr/bin/env python3
"""
verify_effect_boundaries.py — Tovarisch effect boundary verifier

ACT-TOVARISCH-ZIG-HULK20: Functional core / effect boundary register and gate

Verifies that PURE modules do not contain forbidden effect patterns.

This is the executable entrypoint. The actual logic is in effect_boundary_verifier package.
"""

import re
import sys
from pathlib import Path

# Add scripts directory to path for package imports
SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from effect_boundary_verifier.classifications import (
    PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
)
from effect_boundary_verifier.scanner import scan_directory
from effect_boundary_verifier.self_test import run_self_test


def main():
    """Main entry point."""
    args = sys.argv[1:]
    
    if "--self-test" in args:
        success = run_self_test()
        if success:
            print("\n[self-test] All self-tests passed!")
            sys.exit(0)
        else:
            print("\n[self-test] Some self-tests FAILED!")
            sys.exit(1)
    
    # Normal scan mode
    base_dir = Path(__file__).parent.parent
    print(f"[verifier] Scanning {base_dir / 'tovarisch/src'}...")
    
    violations, test_imports, deferred = scan_directory(
        base_dir, PURE_MODULES, BOUNDARY_MODULES, STATEFUL_MODULES, DEFERRED_MODULES
    )
    
    # Report violations
    has_errors = False
    
    if violations:
        print("\n[ERROR] PURE module violations found:")
        for v in violations:
            print(f"  {v.file}:{v.line}: {v.description}")
            print(f"    -> {v.line_content}")
        has_errors = True
    
    if test_imports:
        print("\n[ERROR] Production modules importing test files:")
        for prod_file, test_file, line in test_imports:
            print(f"  {prod_file}:{line} imports {test_file}")
        has_errors = True
    
    if deferred:
        print("\n[INFO] DEFERRED/UNKNOWN modules (report only):")
        for module, reason in deferred:
            print(f"  {module} ({reason})")
    
    if has_errors:
        print("\n[FAIL] Effect boundary verification FAILED")
        sys.exit(1)
    else:
        print("\n[PASS] Effect boundary verification passed")
        sys.exit(0)


if __name__ == "__main__":
    main()
