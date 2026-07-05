#!/usr/bin/env python3
# verify_total_parsers.py — Total parser verifier for tovarisch
"""
Verify that tovarisch external-input parsers follow the total parser doctrine.

Usage:
    python3 scripts/verify_total_parsers.py [--self-test] [--verbose] [--src-root PATH]

The verifier checks that external-input parsers:
- Do not use @panic on malformed input
- Do not use 'unreachable' that could be reached by malformed input
- Do not use 'catch unreachable' to swallow parse errors
- Do not use '.?' optional unwrap without explicit handling
- Do not use @enumFromInt without bounds validation

Classification:
- TOTAL: Parser modules with total public API
- BOUNDARY_TOTAL: Boundary adapters with total external API
- STATEFUL_ADAPTER: Stateful protocol adapters (relaxed checking)
- DEFERRED: Known partial behavior (reports but doesn't fail)
"""

import argparse
import os
import sys
from pathlib import Path
from typing import List

# Add scripts directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from total_parser_verifier import (
    Classification,
    get_all_registered_modules,
    get_module_classification,
    scan_file,
    scan_modules,
    FindingSeverity,
    run_all_self_tests,
    SELF_TEST_CASES,
)


def count_lines(files: List[str]) -> int:
    """Count total lines in given files."""
    total = 0
    for f in files:
        try:
            with open(f, 'r') as fp:
                total += len(fp.readlines())
        except:
            pass
    return total


def print_summary(results, args):
    """Print summary of scan results."""
    total_modules = len(results)
    total_findings = sum(len(r.findings) for r in results)
    total_failures = sum(1 for r in results if r.has_failures)
    total_warnings = sum(1 for r in results if r.has_warnings)
    total_lines = sum(r.scanned_lines for r in results)
    
    print(f"\n{'='*60}")
    print(f"Total Parser Verification Summary")
    print(f"{'='*60}")
    print(f"Modules scanned: {total_modules}")
    print(f"Total lines: {total_lines}")
    print(f"Findings: {total_findings} ({total_failures} failures, {total_warnings} warnings)")
    
    # Group by classification
    by_class = {}
    for r in results:
        cls = r.classification.value
        if cls not in by_class:
            by_class[cls] = {"ok": 0, "issues": 0, "errors": 0}
        if r.errors:
            by_class[cls]["errors"] += 1
        elif r.has_failures:
            by_class[cls]["issues"] += 1
        else:
            by_class[cls]["ok"] += 1
    
    print(f"\nBy classification:")
    for cls in ["TOTAL", "BOUNDARY_TOTAL", "STATEFUL_ADAPTER", "DEFERRED"]:
        stats = by_class.get(cls, {"ok": 0, "issues": 0, "errors": 0})
        if stats["errors"] > 0:
            print(f"  {cls}: {stats['ok']} ok, {stats['issues']} issues, {stats['errors']} errors")
        elif stats["issues"] > 0:
            print(f"  {cls}: {stats['ok']} ok, {stats['issues']} issues")
        elif stats["ok"] > 0:
            print(f"  {cls}: {stats['ok']} ok")
    
    if args.verbose:
        print(f"\nDetailed findings:")
        for r in results:
            if r.findings:
                print(f"\n  {r.module} ({r.classification.value}):")
                for f in r.findings:
                    print(f"    {f}")
    
    return total_failures


def main():
    parser = argparse.ArgumentParser(
        description="Verify total parser discipline in tovarisch"
    )
    parser.add_argument(
        "--self-test",
        action="store_true",
        help="Run self-test cases instead of scanning"
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="Print verbose output"
    )
    parser.add_argument(
        "--src-root",
        default="tovarisch/src",
        help="Root directory for source files (default: tovarisch/src)"
    )
    parser.add_argument(
        "--module",
        help="Scan only a specific module"
    )
    args = parser.parse_args()
    
    # Run self-test if requested
    if args.self_test:
        print("Running total parser verifier self-tests...")
        print(f"Test cases: {len(SELF_TEST_CASES)}")
        
        passed = run_all_self_tests(verbose=args.verbose)
        
        if passed:
            print("\n[OK] All self-tests passed")
            return 0
        else:
            print("\n[FAIL] Some self-tests failed")
            return 1
    
    # Normal verification
    print("Verifying total parser discipline in tovarisch...")
    print(f"Source root: {args.src_root}")
    
    # Scan modules
    if args.module:
        # Scan single module
        file_path = os.path.join(args.src_root, args.module)
        if not os.path.exists(file_path):
            # Try to find it
            for root, _, files in os.walk(args.src_root):
                if args.module in files:
                    file_path = os.path.join(root, args.module)
                    break
            else:
                print(f"Error: Module not found: {args.module}")
                return 1
        
        results, errors = [scan_file(file_path, args.src_root)], []
    else:
        results, errors = scan_modules(args.src_root)
    
    # Report errors (scan errors are fatal)
    if errors:
        print("\nErrors:")
        for error in errors:
            print(f"  {error}")
        print(f"\n[FAIL] {len(errors)} scan error(s) - cannot verify")
        return 1
    
    # Print summary
    failures = print_summary(results, args)
    
    # Print line counts for files created
    verifier_dir = os.path.dirname(os.path.abspath(__file__))
    package_dir = os.path.join(verifier_dir, "total_parser_verifier")
    
    if os.path.exists(verifier_dir) and os.path.isdir(package_dir):
        py_files = []
        for f in os.listdir(package_dir):
            if f.endswith('.py'):
                py_files.append(os.path.join(package_dir, f))
        main_script = os.path.join(verifier_dir, "verify_total_parsers.py")
        if os.path.exists(main_script):
            py_files.append(main_script)
        
        if py_files:
            total_lines = count_lines(py_files)
            print(f"\nVerifier code lines: {total_lines} across {len(py_files)} files")
    
    # Exit with failure count
    if failures > 0:
        print(f"\n[FAIL] {failures} module(s) have forbidden patterns")
        return 1
    else:
        print(f"\n[OK] All modules pass total parser verification")
        return 0


if __name__ == "__main__":
    sys.exit(main())
